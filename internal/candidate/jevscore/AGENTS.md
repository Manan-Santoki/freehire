# Jev scoring conventions

## Scope
The Jev-backed (TypeSafe System One) job-fit decision model for one (user, job) pair:
the typed questions and state builder (`model.go`), the sanitized `Score` + verdict
(`score.go`), the profile-side staleness stamp (`fingerprint.go`), the `Scorer` client
wrapper (`scorer.go`), and the outbox-draining `Runner` (`runner.go`). The deterministic
gate that bounds which pairs ever reach this package lives in the sibling
`internal/candidate/jeveligible`. The worker entrypoint is `cmd/jevscore`; the two
surfaces that read the stored score are the profile-match badge
(`internal/api/handler/job_match.go`) and the personalized feed
(`internal/api/handler/for_you.go`).

## Always true
- **`jeveligible.Eligible` bounds the (user, job) cross product, not this package.** It
  reproduces the profile's own feed filter (specialization, seniority, exclusions) plus
  the two obvious hard-constraint blockers (work authorization, location/work-mode).
  Only a pair that survives it is ever scored — a pair a candidate would never see in
  their feed anyway is never worth a Jev call.
- **The gate runs twice, coarse then fine, deliberately.** `EnqueueJevScoresForProfile`
  (SQL) is the coarse pass: category/seniority/exclusions plus a freshness check, cheap
  enough to run per-profile over the whole job table. `jeveligible.Eligible` (Go) is the
  fine pass the `Runner` re-applies per claimed row before ever calling Jev, because it
  reaches the hard-constraint blockers (visa, degree, certifications, remote-only)
  that the SQL filter has no reasonable way to express. A pair the SQL let through but
  the Go gate rejects is dropped with `Complete(ctx, c, Score{}, "")` — model `""` paired
  with a zero `Score` is the ineligible-drop path, no `user_job_scores` write, entry just
  deleted.
- **The stored `model` stamp is the RESOLVED Jev response model, not the configured
  one.** `Scorer.ScoreWithModel` returns `resp.Model` and the worker stamps
  `user_job_scores.model` with it. `EnqueueJevScoresForProfile`'s freshness check compares
  against the configured `JEV_MODEL` instead (the only value known before the call), so a
  Jev-side model upgrade behind the same alias naturally produces a stamp mismatch on the
  next enqueue pass and re-scores every affected pair with no manual invalidation step.
- **Best-effort degradation throughout — Jev is never a hard dependency.**
  `Runner.Run` is a deliberate no-op when `Scorer.Enabled()` is false (no live Jev
  client), before it opens the pool or touches the database. On the read side, the
  profile-match badge (`job_match.go`) always builds the deterministic `jobmatch.Compute`
  coverage first and only best-effort attaches a cached Jev score on top
  (`attachJevScore`); a missing row or read failure leaves the response exactly as the
  coverage-only fallback built it, never an error. The personalized feed
  (`for_you.go`) degrades the same way at the account level: a caller with no scored
  rows yet (never enqueued, or the worker hasn't caught up) gets an empty page, not an
  error — the feed is additive to Meili search, never a replacement for it.
- **Postgres owns the personalized feed; Meili stays for keyword search and anonymous
  browsing.** `ForYouFeed`/`CountForYou` read straight off `user_job_scores` joined to
  `jobs`, ordered by `match_pct DESC` — there is no Meili involvement in `/jobs/for-you`
  at all. Meili continues to serve the keyword-searchable, non-personalized catalogue
  (including for signed-out visitors), which is why a Jev outage or an unscored account
  never blocks search, only the personalized ranking.
- **Every stored score carries a quintuple staleness stamp**: `model`, `score_version`,
  `profile_fingerprint`, `cv_uploaded_at`, `job_content_hash`. `ProfileFingerprint`
  (`fingerprint.go`) hashes the profile-side inputs that decide the fine gate
  (specializations, skills, seniorities, then the raw `location_preferences` JSONB) —
  it is called identically by `EnqueueJevScoresForProfile`'s freshness check and by
  `Complete`'s write, and both call sites MUST hash the same fields the same way or a
  coarse re-enqueue pass either never fires (a real profile edit hashes the same) or
  fires every run (the two sites disagree). `cv_uploaded_at` and `job_content_hash` are
  compared the same way the badge and `fitanalysis` stamps are (`sameJobContentHash`:
  absent on both sides counts as unchanged, present on only one side is a change).
  `score_version` is a manual escape hatch (`JEVSCORE_VERSION`) to force a full
  recompute independent of any of the other four.
- **Scoring is server-owned, not model-trusted.** `FromAnswers` clamps every confidence
  to `[0,1]`, coerces an out-of-vocabulary `role_category` to `other_tech`, and computes
  `MatchPct` and `Verdict` itself from the raw rubric score and the configured
  thresholds — the model answers typed questions, it never states a verdict. Same
  "never persist an out-of-vocabulary value" invariant as `enrich.Enrichment.Sanitize`
  and `matchanalysis`.
- **`fits_level`'s question text is derived per candidate**, not a fixed instruction —
  `fitsLevelInstruction` folds the profile's own `Seniorities` into the prompt so the
  model judges level fit against what this candidate is targeting, not a generic notion
  of seniority.
- Never hard-code a Jev endpoint or model — `JEV_BASE_URL`/`JEV_API_KEY`/`JEV_MODEL`
  configure it, and Jev is intentionally NOT OpenAI-compatible, so it has its own
  client/config (`internal/platform/jev`, `internal/platform/config/jev.go`) entirely
  separate from `LLM_*`.
- The lease expiry is the built-in reaper for crashed workers — no separate process.
  `Runner.Run` also reaps entries whose job stopped being claimable (closed/duplicate/
  gone) at the start of every run, mirroring `cmd/enrich`'s own reap-before-drain.

## How it works
`cmd/jevscore` first enqueues: it walks every saved profile and issues one
`EnqueueJevScoresForProfile` per user (the coarse SQL gate + freshness check), then drains
`job_score_outbox` via `outbox.RunPool`, claiming a wave of live entries freshest-first
(`ClaimJevScoreBatch`, `FOR UPDATE OF o SKIP LOCKED` + a lease). For each claimed pair the
`Runner` re-applies the fine `jeveligible.Eligible` gate; a pair that fails it is dropped
with no Jev call. A pair that passes is scored via `Scorer.ScoreWithModel`, which builds
the Jev state (`BuildState`: the candidate's de-identified structured résumé JSON plus the
raw job text) and typed questions (`Questions`), sends them through the `decider`
interface (the real `*jev.Client`, or a fake in tests), and sanitizes the response into a
`Score` via `FromAnswers`. On success the worker writes `user_job_scores` and deletes the
outbox row in one transaction (`Complete`); on a Jev call error it records the failure
(`Fail`), dead-lettering on `JEVSCORE_MAX_ATTEMPTS` or `JEVSCORE_UPSTREAM_GRACE_DAYS` —
every failure here is a Jev call error (`posting_at_fault` is always `false`), never a
defect in the posting, so dead-lettering follows the upstream-grace clock, not an attempt
cap.

## Limitations
- `EnqueueJevScoresForProfile` walks every saved profile on every run rather than only
  profiles that changed — fine at current scale, but the first thing to revisit if the
  profile table grows large enough to make the per-run enqueue pass itself the
  bottleneck.
- The `0175` migration (schema for `user_job_scores` / `job_score_outbox`) must be
  applied to prod manually before the worker is ever deployed; nothing here runs it.
