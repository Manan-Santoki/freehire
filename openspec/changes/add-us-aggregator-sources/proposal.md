## Why

The catalogue's US coverage comes almost entirely from per-employer ATS boards. Three
open, keyless, US-centric sources that other aggregators (trawl, career-ops, Job-Ops) already
read have no adapter here: hiring.cafe (an index of employer career sites with the employer's
own apply link on every hit), the curated SimplifyJobs / vanshb03 GitHub lists (the reference
US internship and new-grad lists, rendered from one machine-readable file), and Hacker News'
monthly "Who is hiring?" threads. Each was probed live on 2026-09-14 and each is readable
with transport this repository already has — hiring.cafe through the Chrome-fingerprint
client, the other two through the plain one.

## What Changes

- Add a `hiringcafe` adapter: board = search keyword, region = country; a paced
  `fingerprintHTTP` crawl of the server-rendered search page's `__NEXT_DATA__`, newest-first,
  hydrating bodies from each posting's own page (`HydratingSource`, 14-day `sweepGrace`).
- Add a `githublists` adapter: board = `owner/repo`, streaming read of
  `.github/scripts/listings.json`, bodies from the employer's page via the shared ld+json
  parser, `CoverageGated` so covered employers' bodies are never bought, `fullCatalog`.
- Add a `hackernews` adapter: boardless, the two newest hiring threads through the Algolia HN
  API, one Job per top-level comment that follows the thread's header convention,
  `fullCatalog`.
- `fingerprintHTTP` returns `*StatusError` for a non-2xx so its refusals are readable
  structurally (message unchanged); hiringcafe retries a burst refusal on a short ladder.
- Register all three in `sources.All` and regenerate the web source facet.

## Capabilities

### New Capabilities
- `hiringcafe-source`, `githublists-source`, `hackernews-source`: one per adapter.

### Modified Capabilities
(none — `source-ingest`'s existing requirements already cover "a new provider is an adapter
plus a registry line plus boards via `cmd/add-board`".)

## Impact

- **New files**: `internal/ingest/sources/{hiringcafe,githublists,hackernews}.go` and tests.
- **Modified**: `registry.go` (three registrations), `fingerprinthttp.go` (typed status
  error), `AGENTS.md` (trap entries), `web/src/lib/generated/contracts.ts` (regenerated).
- **Boards**: added afterward via `cmd/add-board` — hiring.cafe keyword boards with
  `--region=US`, the three GitHub repos, one boardless `hackernews` row. This change seeds no
  catalogue rows.
- **Risk**: hiring.cafe was measured served from residential egress only; if the production
  datacenter IP is refused, the fallback is a `firecrawlProviders` entry (as `wellfound`).
- **Out of scope**: WTTJ, Built In, Naukri, Torre and the other non-US gaps found in the
  same survey.
