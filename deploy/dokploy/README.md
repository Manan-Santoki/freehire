# Dokploy operations

The web deployment needs a populated board catalog and scheduled workers. Starting
Postgres and Meilisearch alone creates neither job postings nor a search index.

`docker-compose.dokploy.yml` runs the API, frontend, database, search, object storage,
Redis, the CPU PII-filter service, a crawl scheduler, and periodic application workers. The scheduler uses the
existing `ingest_schedule` and `ingest_run_state` tables and launches bounded Docker
containers. It mounts the Docker socket and runs as root to manage those containers;
the crawlers themselves run as the image's nonroot user on the dedicated crawl
network, with one CPU, 2 GiB RAM, and a per-run timeout. Do not mount the socket in
the API or frontend.

Set `INGEST_SCHEDULER_CAP` to control simultaneous crawls (default 4). Each scheduled
provider must be enabled and managed in `ingest_schedule`; `schedule-board` is the
operator interface. New providers need the same handover, matching upstream's
current scheduler rollout gate.

## Initial catalog

`boards.json.gz` contains 157,551 historical entries from 235 providers, converted
from upstream's last committed YAML catalog at
[`96b53a2c7b1e545bbad0868e16ec85a2eba4f36a`](https://github.com/strelov1/freehire/tree/96b53a2c7b1e545bbad0868e16ec85a2eba4f36a/sources).
The source files are covered by the repository's MIT license. Three entries using
adapters absent from this checkout (`globalpayments`, `justjoin`, and `wantedkr`)
were excluded. Per-entry provider overrides, region, hub, and tenant metadata were
preserved. This is a bootstrap snapshot, not a copy of upstream's live job database.

In the scheduler container terminal:

```sh
/app/import-board-catalog /app/deploy/dokploy/boards.json.gz
/app/import-board-catalog --apply /app/deploy/dokploy/boards.json.gz
/app/schedule-board --provider=remoteok --manage --apply
```

Validation happens before writes. The import uses the domain catalog inserter, skips
existing live boards, and inserts new boards as pending. It can resume after an
interruption. Run it deliberately, not on every deployment: a retired historical
board can otherwise be reintroduced by a later import.

**A provider that was ever seen without its credentials is disabled in `ingest_schedule`
with a reason, and adding the key later does NOT re-enable it.** After setting
`ADZUNA_APP_ID/KEY`, `REED_API_KEY`, `USAJOBS_API_KEY` or `WHATJOBS_PUBLISHER_IDS`, run once
in the scheduler container (the Dokploy schedule `enable-adzuna` does exactly this; edit
`--provider` for the others):

```sh
/app/schedule-board --provider=adzuna --enable --apply
```

Adzuna's API returns a ~500-character snippet; `hydrate-adzuna-description` (every 30 min in
[workers.crontab](workers.crontab)) fetches the full posting for jobs crawled from then on.
Postings that existed before it was enabled need `/app/seed-adzuna-description-queue` once.

Configure other providers with `schedule-board`. Large providers need shards so
their complete board list can be visited within a crawl budget. Sources that require
missing API credentials should be disabled with an explicit reason until configured.
The scheduler reports disabled providers, completed runs, failures, and saturation.
Running crawl containers keep their exit status until the scheduler records it.

## Upgrading from upstream

`main` on this fork tracks `strelov1/freehire`; `dokploy` is `main` plus the deployment
commits (this directory, `docker-compose.dokploy.yml`, the Dockerfile worker list, the
Docker crawl launcher, and the migrate search_path fix). To ship a newer upstream:

`.github/workflows/sync-upstream.yml` does this every three days (and on demand from the
Actions tab): it fast-forwards `main`, merges it into `dokploy` and pushes, which Dokploy
deploys. A conflict fails the run and opens a `sync-conflict` issue. By hand, with
`origin` = this fork and `upstream` = strelov1/freehire:

```sh
git fetch upstream && git checkout main && git merge --ff-only upstream/main && git push origin main
git checkout dokploy && git merge main && git push origin dokploy   # Dokploy auto-deploys
```

Migrations apply on start through the `migrate` service. Meilisearch settings do NOT:
a release that adds a filterable or sortable attribute (upstream ships these in
`internal/search/search/client.go`) makes `/api/v1/jobs/facets` and the affected filters
return 400 until the next `reindex`, which builds a fresh index with the new settings and
swaps it in. After such a deploy, trigger the disabled Dokploy schedule `reindex` (runs
`run-worker.sh reindex` in the `workers` container, so it shares the search lock) rather
than waiting for the six-hourly cron. `search-settings-drift` reports the same gap as a
metric. With `DOKPLOY_URL` and `DOKPLOY_API_KEY` repository secrets set, the sync workflow
runs that schedule itself once the deploy is done.

## Environment

[env.template](env.template) lists every variable the binaries read, grouped by feature,
with the ones already set on this deployment marked. The compose file passes Dokploy's
whole `.env` through to `app`, `workers`, `scheduler` and `migrate`, so a variable pasted
into the Dokploy Environment tab takes effect on the next deploy.

Outbound mail (verification codes, password reset, alerts, digests) needs
`NOTIFY_EMAIL_FROM` plus either `RESEND_API_KEY` (Resend, no AWS account needed; the
sender's domain must be verified in the Resend dashboard) or AWS SES credentials. Until
one is set, sign-up works but the "confirm your email" prompt cannot be satisfied.
The `notify` (saved-search alerts, every 5 min), `remind` and `nudge` workers are in
[workers.crontab](workers.crontab) but `run-worker.sh` skips them until a delivery channel
(`NOTIFY_EMAIL_FROM` or `TELEGRAM_BOT_TOKEN`) exists; `onboarding` also needs
`ONBOARDING_REPLY_TO`.

## Search and maintenance

After initial ingestion, run in the workers container:

```sh
/app/deploy/dokploy/run-worker.sh reindex
/app/deploy/dokploy/run-worker.sh recount-companies
/app/deploy/dokploy/run-worker.sh rollup-stats
```

[workers.crontab](workers.crontab) maintains incremental search, periodic full
rebuilds, company indexes, suggestions, catalog rollups, related jobs, duplicate markers,
posting liveness, captured
application forms, public Telegram channel crawling and extraction, and auth cleanup.
Telegram channels are already seeded by migration 0130; no Telegram login is needed. Enrichment runs only with complete LLM settings.
All Meilisearch writers share a lock; rebuilds wait for an existing drain to finish,
and incremental pushes defer while a rebuild holds it. Suggestions run at 04:45 UTC,
after the midnight rebuild's four-hour budget and fifteen-minute lock wait; company
rebuilds run in odd hours so they cannot occupy that window. The real search volume is
mounted read-only in workers so the rebuild's disk-space guard measures its disk.

The scheduler image must match `INGEST_DOCKER_IMAGE`; the Compose image settings do
this by default. Set distinct `FREEHIRE_IMAGE`, `FREEHIRE_CRAWL_NETWORK`, and
`FREEHIRE_CRAWL_PREFIX` values for multiple freehire deployments on one Docker host.

## External integrations

An LLM endpoint, API key, and model enable AI processing. CV extraction also needs
the [PII filter](../../services/pii-filter/README.md), included in this Compose stack
with weights pinned to the official model revision. Its health check waits for model
loading, and it is accessible only inside the deployment network. OAuth applications, SES sending
and receiving identities, Gmail consent, messaging integrations, and Inngest each
need their own service configuration; their credentials are not part of the source
code. The image includes their workers, but unconfigured outbound integrations are
not scheduled. Configure and verify each before adding it to the cron schedule.

Job coverage grows as crawling completes. Source API limits, retired boards, blocked
egress, and upstream-only credentials can prevent exact catalog parity. Monitor
`ingest_run_state`, scheduler logs, `search_outbox`, and the public search results
instead of treating a green web deployment as proof that data collection works.
