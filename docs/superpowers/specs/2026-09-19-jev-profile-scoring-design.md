# Jev profile scoring — design

**Date:** 2026-09-19
**Status:** approved for planning
**Author:** Manan (with Claude)

## Problem

The "Profile match" the app shows a signed-in candidate is skill-set overlap and
nothing else. Two independent surfaces both compute it, and both share the same blind
spot:

1. **The badge** — `internal/api/handler/job_match.go` calls
   `jobmatch.Compute(job.Skills, profile.Skills)`: exact hold = 1, curated adjacency = 0.5,
   over the job's skill count. Plus a handful of deterministic `hardconstraint` blockers.
2. **The feed `?sort=match`** — a Meilisearch *vector* sort over `internal/dict/skillvec`
   position vectors (`userProvided` embedder, no model). It ranks the whole index by
   skill-cosine.

Neither knows **seniority fit, role intent, or hard blockers beyond a degree check.**
Observed failures on real postings:

| Posting | Current badge | Reality |
|---|---|---|
| "Founding AI Engineer" — 3+ yrs *excluding internships* | **100%** (12/12 skills) | Wrong seniority for a Master's student targeting intern/new-grad |
| GenAI Research Intern — *PhD preferred* | 58% | Actually a reasonable maybe |

A tuned Jev script (`~/Downloads/resume-match/skills/resume-match/scripts/jd_match.py`)
already scores these correctly: Founding Eng → **SKIP** (`fits_new_grad` 12%,
`hard_blocker` 33%), Research Intern → **MAYBE** (47/100). Jev's `fits_new_grad`,
`hard_blocker`, and `role_category` are exactly the axes the skill signals lack.

## Goal

Replace the profile-match signal (badge + a new personalized feed) with a Jev-backed
score that understands seniority, role intent, and hard blockers — computed ahead of
time so the feed is instantly sortable and filterable.

### Non-goals

- Not replacing Meilisearch. Keyword search, facets, filters, autocomplete, and
  anonymous/public browsing stay on Meili. The anon `?sort=match` vector sort stays.
- Not reconciling Jev's 8-value `role_category` with freehire's ~46-value category
  vocabulary. `role_category` is an advisory display field only.
- No credit/metering plumbing (unlike `fitanalysis`). Scoring is a background batch, not
  an on-demand charged action.
- Not writing per-user scores into the shared Meili index (does not scale; pollutes the
  public index).

## What Jev is (constraints it imposes)

TypeSafe's Jev ("System One") takes unstructured `state` plus a set of typed `questions`
and returns, in one parallel pass, a typed calibrated answer per question. Primitives:
`score` (numeric over a rubric), `choice` (categorical), `noul` (yes/no probability).
Every answer carries a `confidence`. Output space is bounded by the schema, so a job
description cannot inject arbitrary output — but we still clamp/coerce. Vendor-reported
70–500 ms latency and ~$0.042 / M input tokens (output free), which is what makes
batch-scoring a large eligible set affordable. It is **not** OpenAI-compatible, so it
gets its own client, separate from `internal/platform/llm`.

Wire shape (from the validated script):

```
POST {JEV_BASE_URL}                      # default https://api.typesafe.ai/v1/systemone
Authorization: Bearer {JEV_API_KEY}
{ "model": "jev-latest", "state": "...", "questions": { ... } }
→ { "answers": { "<name>": {"score"|"choice"|"noul": ..., "confidence": ...} }, "usage": {...} }
```

## Chosen approach

**Postgres-driven personalized scoring, with a cheap deterministic eligibility gate in
front of Jev.** Jev never scores the full user × job Cartesian product — only the
(user, job) pairs that already pass the gate (the pairs that would appear in that user's
feed). Scores live in Postgres, per (user, job). The feed reads only stored rows, and
only eligible pairs are ever stored, so the feed *is* the eligible set by construction —
no drift between "what is scored" and "what is shown."

Rejected alternatives: (B) Meili prefilter + per-page re-sort — only an approximate
global order, fights Meili pagination, undercuts the point of pre-scoring; (C) per-user
verdict as a Meili filter attribute — does not scale on a shared index.

```
new/updated job ──enrich (existing)──► jeveligible.Eligible(profile, job)?
                                              │ yes, per active profile
                                              ▼
                                   job_score_outbox (user_id, job_id)
                                              │  cmd/jevscore cron
                                              ▼
                              jevscore.Score → Jev /v1/systemone
                                              ▼
                                    user_job_scores (Postgres)
                              ┌───────────────┴───────────────┐
                              ▼                                ▼
                     GET /jobs/for-you                 GET /jobs/:slug/match
                     ORDER BY match_pct                score + verdict badge
```

## Components

### `internal/platform/jev` — API client

The only unit that knows Jev's wire shape.

```go
type QuestionType string // "score" | "choice" | "noul"

type Question struct {
    Type         QuestionType
    Instructions string
    Criteria     any // []string rubric for score; map[string]string for choice; nil for noul
}

type Answer struct {
    Score      float64 // score questions
    Choice     string  // choice questions
    Noul       float64 // noul questions
    Confidence float64
}

type Response struct {
    Answers map[string]Answer
    Usage   map[string]any
}

type Client interface {
    Decide(ctx context.Context, state string, questions map[string]Question) (Response, error)
}
```

- Config: `JEV_BASE_URL` (default `https://api.typesafe.ai/v1/systemone`), `JEV_API_KEY`,
  `JEV_MODEL` (default `jev-latest`), `JEV_TIMEOUT` (per call, default 30s).
- A nil/unconfigured client is a first-class state: callers degrade, they do not error.
- **The API key is never committed.** `.env.example` gets `JEV_*` placeholders; the real
  key is an env/secret on the deploy.
- Tested against an `httptest.Server`: request body shape, response parse, HTTP-error and
  timeout mapping.

### `internal/candidate/jevscore` — decision model + scoring

Ports `jd_match.py`'s model verbatim, with one multi-user change.

Questions (built per profile):

- `match_score` — `score`, 5-level rubric (no overlap → excellent), mapped raw/4 → 0–100.
- `role_category` — `choice` over `{swe, ml_ai, robotics, qa_test, data, devops_infra,
  other_tech, non_technical}`.
- `has_required_stack` — `noul`.
- `fits_level` — `noul`. **This is the multi-user change:** the instruction is built from
  `profile.Seniorities` (e.g. "the role's seniority fits a candidate targeting: intern,
  junior, middle — not senior, staff, lead, or principal"). With no seniorities set it
  degrades to a generic "fits the candidate's stated experience level."
- `hard_blocker` — `noul` (citizenship, clearance, a degree not held, a required license).

State string: `"RESUME:\n{structured}\n\n---\n\nJOB DESCRIPTION:\n{jd}"`, where
`{structured}` is a rendering of `resumeextract.Professional` (the contact-free structured
projection the existing AI analysis uses — never the raw CV). `{jd}` is the job's
description (+ `company_info` where present).

`Score` result (sanitized):

```go
type Score struct {
    MatchPct         int     // 0..100
    MatchRaw         float64
    MatchConfidence  float64 // 0..1
    RoleCategory     string  // coerced to the known set, else "other_tech"
    RoleConfidence   float64
    HasRequiredStack float64 // 0..1
    FitsLevel        float64 // 0..1
    HardBlocker      float64 // 0..1
    Verdict          string  // "APPLY" | "MAYBE" | "SKIP"
}
```

Sanitize: clamp `MatchPct` to 0..100, clamp every `noul`/confidence to 0..1, coerce
`RoleCategory` to the known set. Verdict (ported from `summarize()`, thresholds env-tunable):

```
if HardBlocker >= HARDBLOCK_MAX (0.5) or RoleCategory == "non_technical": SKIP
elif MatchPct >= APPLY_MIN (60) and HasRequiredStack >= 0.5 and FitsLevel >= 0.5: APPLY
elif MatchPct >= MAYBE_MIN (45): MAYBE
else: SKIP
```

`Scorer.Score(ctx, in) (Score, error)` composes the client, the question builder, and
sanitize. Pure over the `jev.Client` interface — table-tested without the network.

### `internal/candidate/jeveligible` — deterministic gate

`Eligible(profile userprofile.Profile, job db.Job) bool` (the enriched job domain row, as
the read/ingest layers already carry it), reusing existing logic:

- **Specialization**: job's enrichment category ∈ `profile.Specializations`.
- **Seniority**: job seniority overlaps `profile.Seniorities` (empty profile set = no
  seniority constraint).
- **Location / work-mode**: `dict/location` + `profile.LocationPreferences`.
- **Obvious hard blockers**: `hardconstraint.Evaluate` — a `blocking`-severity result
  (wrong degree, work-auth) skips the pair before Jev. Jev's `hard_blocker` catches the
  subtler ones.
- **Exclusions**: `profile.ExcludedCompanies`, `profile.ExcludedSources`,
  `profile.ExcludedSkills`.

Only open, tech, enriched, non-duplicate jobs are candidates (same base predicate as
enrichment). Table-driven tests.

### Schema — migration `0175_user_job_scores.sql`

Mirrors `user_job_analysis` (cache) and `semantic_outbox` (queue).

```sql
CREATE TABLE public.user_job_scores (
    user_id             bigint      NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    job_id              bigint      NOT NULL REFERENCES public.jobs(id)  ON DELETE CASCADE,
    match_pct           smallint    NOT NULL,   -- 0..100
    match_raw           real        NOT NULL,
    match_confidence    real        NOT NULL,   -- 0..1
    role_category       text        NOT NULL,
    role_confidence     real        NOT NULL,
    has_required_stack  real        NOT NULL,   -- 0..1
    fits_level          real        NOT NULL,   -- 0..1
    hard_blocker        real        NOT NULL,   -- 0..1
    verdict             text        NOT NULL,   -- APPLY | MAYBE | SKIP
    -- staleness stamps (mirror user_job_analysis's model/cv/content_hash + version)
    model               text        NOT NULL,
    score_version       integer     NOT NULL,
    profile_fingerprint text        NOT NULL,   -- hash(specializations+seniorities+skills+location)
    cv_uploaded_at      timestamptz,
    job_content_hash    text,
    scored_at           timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, job_id)
);
CREATE INDEX user_job_scores_feed_idx   ON public.user_job_scores (user_id, match_pct DESC);
CREATE INDEX user_job_scores_verdict_idx ON public.user_job_scores (user_id, verdict);

CREATE TABLE public.job_score_outbox (
    user_id       bigint      NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    job_id        bigint      NOT NULL REFERENCES public.jobs(id)  ON DELETE CASCADE,
    target_version integer    NOT NULL,
    job_posted_at timestamptz,                 -- freshest-first ordering
    claimed_at    timestamptz,                 -- lease
    attempts      integer     NOT NULL DEFAULT 0,
    failed_at     timestamptz,                 -- dead-letter marker
    enqueued_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, job_id)
);
CREATE INDEX job_score_outbox_claim_idx
    ON public.job_score_outbox (COALESCE(job_posted_at, enqueued_at) DESC, job_id DESC)
    WHERE claimed_at IS NULL AND failed_at IS NULL;
```

Staleness: a row is stale when any of `model`, `score_version`, `profile_fingerprint`,
`cv_uploaded_at`, `job_content_hash` differs from live. Same "absent on both sides = 
unchanged" rule as `user_job_analysis` for `job_content_hash`. sqlc queries live in
`internal/platform/db/queries/user_job_scores.sql`.

### `cmd/jevscore` — batch worker

Mirrors `cmd/enrich`:

1. **Reap** outbox rows whose pair is no longer eligible (job closed / became duplicate /
   profile changed so the pair fell out of eligibility) — bounded, best-effort, never
   aborts the run.
2. **EnqueuePending** — for each active profile, insert outbox rows for eligible open jobs
   that lack a *fresh* score (staleness compared to the live stamps). This is the
   self-healing net.
3. **Drain** — claim a wave freshest-first `FOR UPDATE SKIP LOCKED` + `claimed_at` lease,
   run across `JEV_CONCURRENCY` workers (default 4). Each: build state, call Jev, sanitize,
   upsert `user_job_scores`, delete the outbox row in one transaction. The lease is the
   built-in reaper — no separate process.

Dead-letter split like enrichment: a payload/row fault is bounded by `JEVSCORE_MAX_ATTEMPTS`
(default 3); a transport/upstream fault is bounded by queue age (`JEVSCORE_UPSTREAM_GRACE_DAYS`,
default 14) so a Jev outage does not permanently bury the queue.

**Enqueue triggers:** the periodic `EnqueuePending` is the baseline. On **profile Save**
(`userprofile.Save`) and **CV upload** the user's score rows are invalidated (fingerprint /
cv_uploaded_at will mismatch), so the next run re-scores promptly.

### Read / serve

- **Badge** (`internal/api/handler/job_match.go`): return the Jev `Score` (pct, verdict,
  three flags, confidence) from `user_job_scores` when a fresh row exists; **degrade to
  `jobmatch.Compute` + hardconstraint blockers** when there is no fresh row or Jev is
  unconfigured — the same best-effort posture as `matchanalysis`. The skill breakdown
  (matched / adjacent / missing) stays as secondary detail in the payload. Wire shape
  regenerated to TS via `cmd/gen-contracts`.
- **Feed** — `GET /jobs/for-you` (RequireAuth): reads `user_job_scores JOIN jobs`,
  `ORDER BY match_pct DESC`, optional `?verdict=` and `?min=` filters, Postgres-paginated.
  **Default: show all eligible jobs sorted by score** (SKIP sinks to the bottom, never
  hidden by default). Meili's `/jobs/search`, facets, autocomplete, and anon `?sort=match`
  are untouched.

### Frontend

- Badge component (replaces the current "Profile match" block on card + detail): match
  score, verdict chip (APPLY / MAYBE / SKIP color), the three flags (stack %, level fit %,
  blocker risk %), and confidence. Falls back to the coverage breakdown when no Jev score.
- Feed: "For You" ranked view with verdict filter chips (show/hide APPLY/MAYBE/SKIP) and an
  optional minimum-score control. Card shows the verdict chip + score.

## Config / env

| Var | Default | Meaning |
|---|---|---|
| `JEV_BASE_URL` | `https://api.typesafe.ai/v1/systemone` | Jev endpoint |
| `JEV_API_KEY` | — | Secret; never committed |
| `JEV_MODEL` | `jev-latest` | Model route (also a staleness stamp) |
| `JEV_TIMEOUT` | `30s` | Per-call timeout |
| `JEV_CONCURRENCY` | `4` | Worker fan-out |
| `JEVSCORE_VERSION` | `1` | Bump to force re-score |
| `JEVSCORE_APPLY_MIN` | `60` | Verdict threshold |
| `JEVSCORE_MAYBE_MIN` | `45` | Verdict threshold |
| `JEVSCORE_HARDBLOCK_MAX` | `0.5` | Verdict threshold |
| `JEVSCORE_MAX_ATTEMPTS` | `3` | Payload-fault dead-letter bound |
| `JEVSCORE_UPSTREAM_GRACE_DAYS` | `14` | Outage dead-letter bound |

## Failure modes

- **Jev unconfigured / down:** best-effort everywhere. Badge and feed fall back to today's
  skill signals; nothing hard-depends on Jev. The worker leaves stamps untouched so a
  recovered Jev re-scores on the next run.
- **Prompt injection via `description`/`company_info`:** Jev's output space is bounded by
  the question schema, so it cannot emit arbitrary output; results are still clamped and
  `role_category` coerced.
- **Out-of-vocabulary / out-of-range answers:** clamped (`match_pct`, nouls, confidence)
  and coerced (`role_category`), never persisted raw — same invariant as enrichment.
- **Cross-product blow-up:** impossible by construction — only eligible pairs are enqueued,
  and the periodic enqueue is bounded by each profile's eligible open-job set.

## Testing

- `internal/platform/jev`: `httptest` — request shape, parse, HTTP-error + timeout mapping,
  nil-client degrade.
- `internal/candidate/jevscore`: table-driven verdict + sanitize tests, seeded with the two
  real cases (Research Intern → MAYBE, Founding Eng → SKIP) and the rubric→pct mapping;
  `fits_level` instruction built from varying `Seniorities`.
- `internal/candidate/jeveligible`: table-driven predicate (specialization / seniority /
  location / blocker / exclusion combinations).
- Store + worker: integration tests (claim / lease / reap / dead-letter / staleness), in the
  style of the existing `*_integration_test.go`.
- Handler: badge returns Jev then falls back; feed ordering + verdict/min filtering.
- One optional **live** Jev test behind a build tag, using `JEV_API_KEY` from env.

## Migration / deploy notes

- Migration `0175` applied by initdb on a fresh volume; on the live volume it must be run
  manually before deploying code that reads the new tables (no versioned runner — the
  standing freehire migration caveat).
- New cron service `cmd/jevscore` added to the compose/schedule alongside `cmd/enrich`.
- `.env` on the deploy gains `JEV_API_KEY` (secret) and any non-default `JEV_*`.

## Decisions log

- **Scope:** multi-user / generic (seniority framing derived per profile), though the fork
  runs at a handful of profiles.
- **Placement:** Jev replaces the profile-match signal on badge + a new Postgres feed; Meili
  keeps keyword/facets/autocomplete/anon.
- **Timing:** batch-score eligible pairs ahead of time (outbox + cron), bounded by the
  eligibility gate.
- **Résumé input:** structured contact-free projection (`resumeextract.Professional`), not
  raw CV markdown.
- **Feed default:** show all eligible jobs sorted by score; verdict filtering is opt-in.
- **role_category:** advisory display only; not mapped to freehire's category vocabulary.
