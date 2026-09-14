## 1. The shared window

- [x] 1.1 `maxPageWindow` in `handler.go`; `pageParamsWindowed` refuses `offset+limit > maxPageWindow` with 400 "pagination too deep".
- [x] 1.2 `search.go` and `swipe.go` drop their own `maxSearchWindow` check and call the helper, so one constant decides for every store.
- [x] 1.3 Unit test: the helper allows the boundary (`offset+limit == maxPageWindow`), refuses one past it, and still clamps a 64-bit offset into int32 range rather than wrapping negative. Mutation-checked — disabling the guard fails three of the five.

## 2. The five unbounded Postgres lists

- [x] 2.1 `GET /api/v1/jobs` (`jobs.go`).
- [x] 2.2 `GET /api/v1/companies` (`companies.go`).
- [x] 2.3 `GET /api/v1/companies/:slug` — the embedded job list (`companies.go`). The window moved ABOVE the company lookup: a refused request must cost no query at all.
- [x] 2.4 `GET /api/v1/jobs/:slug/copies` (`copies.go`) — keeps its own limit ceiling, gains the window, same reordering.
- [x] 2.5 `GET /api/v1/companies/:slug/feedback` (`company_feedback.go`).
- [x] 2.6 `pagination_window_routes_test.go` drives all five through their real registers with zero-valued handlers, so a 400 PROVES nothing was queried. Carries its own control (a shallow request must reach the handler) so a path typo cannot make it pass on nothing. Mutation-checked — all five fail without the guard.

## 3. The missing limiter

- [x] 3.1 `GET /api/v1/companies/:slug/feedback` mounts `publicReadLimiter` — the one public list that had none.
- [x] 3.2 Split into `registerPublic`, following `mentorshipHandlers`: the existing guard drives every GET a register mounts and requires the limiter to LEAD each chain, which this feature's cookie-gated and moderator reads cannot satisfy. Added to `publicReadRoutes`, so its key is now checked against what the mounted chain can actually see.

## 4. The backstop: a query may not hold a connection forever

- [x] 4.1 `database.WithStatementTimeout`, applied by `cmd/server` only at 30s. Opt-in, because the cron workers share the package and `backfill-derive` legitimately runs for hours.
- [x] 4.2 Test: the option reaches `RuntimeParams` as milliseconds (a unitless "30" would be 30ms — set-looking and cutting every real query), a pool without it carries none, and applying it does not disturb the connection cap.

## 5. The status page sees a saturated pool

- [x] 5.1 `currentSiteHealth` reads `pool.Stat()`; at or above 90% of the pool held, the site reads `degraded`.
- [x] 5.2 Tests at both boundaries, on an idle pool, on a pool reporting no capacity (the division guard), and that the signal can never outrank `down`. Exempt from the traffic floor on purpose — that floor is a sampling argument about a FRACTION, and during the outage almost nothing completed.
- [x] 5.3 `pool_pressure` on the wire; `StatusBoard.svelte` names it only when it is what makes the site degraded.

## 6. Docs

- [x] 6.1 `docs/API.md` and `web/static/openapi.yaml`: the window applies to every list, the refusal is deliberate, and it is not a substitute for the rate limit.
- [x] 6.2 `internal/api/handler/AGENTS.md`: a caller-controlled offset is a caller-controlled COST, and a rate limiter cannot bound it.

## 7. Metrics and alerts

The outage was found by a person noticing the site was slow. Every signal that could have
named it existed somewhere and none of it was watched, so this section is what turns the
change from "this specific hole is closed" into "the next one is visible".

- [x] 7.1 `observability.NewPoolCollector` publishes acquired / idle / max and
      `EmptyAcquireCount` — the counter that rises only when a caller found nothing free and
      had to WAIT, i.e. the bottleneck rather than the workload. Nothing read `pool.Stat()`
      before this. A Collector, not a polled gauge: it reads at scrape time, so there is no
      interval to choose.
- [x] 7.2 `freehire_http_request_duration_seconds`, by route pattern. `requestBucket` carries
      `minute`/`total`/`errors` and had no field a duration could go into, so p95 was
      unanswerable — which is why "slow" was invisible until it became "down". Buckets stop at
      30s because that is now the pool's `statement_timeout`.
- [x] 7.3 Three rules in `freehire-ops` (`freehire-api-saturation`): pool starvation
      (critical), p95 latency (warning), and deep-pagination refusals (warning, 15m — the
      guard working is worth knowing about and must never wake anybody).
- [x] 7.4 `scripts/check-alert-rules.py` passes: 23 rules, UIDs within 40 chars, no `<` in an
      annotation. The pool rule's runbook query uses `!=` for exactly that reason.

## 8. Verification

- [x] 8.1 `gofmt -l .` clean, `go vet ./...` clean, `go vet -tags=integration ./...` clean,
      `go test ./...` — 226 packages ok, 0 failures.
- [ ] 8.2 k6: reproduce the attack against the **idle** colour's API port on prod and record
      before/after. `perf/k6` already carries the siting rule and the `FORCE_SCRAPER` latch.
- [ ] 8.3 Deploy; confirm `/api/v1/jobs?offset=179500` answers 400 and the ordinary list still
      serves.
- [ ] 8.4 Ship the alert rules to litellm-host AFTER the binary, per the deploy-order note in
      the rules file: the pool rule's `noDataState: Alerting` pages immediately if the series
      is not being published yet.
