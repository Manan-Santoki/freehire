-- name: GetUserJobScore :one
-- The caller's cached Jev score for one job, with the five staleness stamps. No row =
-- never scored (handler falls back to deterministic coverage). The handler compares the
-- stamps to live values to decide the stale flag.
SELECT match_pct, match_raw, match_confidence, role_category, role_confidence,
       has_required_stack, fits_level, hard_blocker, verdict,
       model, score_version, profile_fingerprint, cv_uploaded_at, job_content_hash, scored_at
FROM user_job_scores
WHERE user_id = $1 AND job_id = $2;

-- name: UpsertUserJobScore :exec
-- Create-or-replace the score for a (user, job). Composite PK makes it idempotent.
INSERT INTO user_job_scores (
    user_id, job_id, match_pct, match_raw, match_confidence, role_category, role_confidence,
    has_required_stack, fits_level, hard_blocker, verdict,
    model, score_version, profile_fingerprint, cv_uploaded_at, job_content_hash, scored_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16, now())
ON CONFLICT (user_id, job_id) DO UPDATE SET
    match_pct           = EXCLUDED.match_pct,
    match_raw           = EXCLUDED.match_raw,
    match_confidence    = EXCLUDED.match_confidence,
    role_category       = EXCLUDED.role_category,
    role_confidence     = EXCLUDED.role_confidence,
    has_required_stack  = EXCLUDED.has_required_stack,
    fits_level          = EXCLUDED.fits_level,
    hard_blocker        = EXCLUDED.hard_blocker,
    verdict             = EXCLUDED.verdict,
    model               = EXCLUDED.model,
    score_version       = EXCLUDED.score_version,
    profile_fingerprint = EXCLUDED.profile_fingerprint,
    cv_uploaded_at      = EXCLUDED.cv_uploaded_at,
    job_content_hash    = EXCLUDED.job_content_hash,
    scored_at           = now();

-- name: ForYouFeed :many
-- The caller's personalized feed: eligible scored jobs, best first. Optional verdict
-- filter ('' = all) and minimum match_pct. Paginated.
SELECT j.public_slug, j.title, j.company, j.company_slug, j.location, j.work_mode,
       j.posted_at, j.closed_at, j.skills,
       s.match_pct, s.verdict, s.has_required_stack, s.fits_level, s.hard_blocker, s.role_category
FROM user_job_scores s
JOIN jobs j ON j.id = s.job_id
WHERE s.user_id = $1
  AND j.closed_at IS NULL AND j.duplicate_of IS NULL
  AND (sqlc.arg(verdict)::text = '' OR s.verdict = sqlc.arg(verdict)::text)
  AND s.match_pct >= sqlc.arg(min_pct)::int
ORDER BY s.match_pct DESC, j.id DESC
LIMIT sqlc.arg(lim)::int OFFSET sqlc.arg(off)::int;

-- name: CountForYou :one
SELECT count(*)
FROM user_job_scores s
JOIN jobs j ON j.id = s.job_id
WHERE s.user_id = $1
  AND j.closed_at IS NULL AND j.duplicate_of IS NULL
  AND (sqlc.arg(verdict)::text = '' OR s.verdict = sqlc.arg(verdict)::text)
  AND s.match_pct >= sqlc.arg(min_pct)::int;

-- name: InvalidateUserScores :exec
-- Drop a user's scores after a profile edit or CV upload so they re-score fresh.
DELETE FROM user_job_scores WHERE user_id = $1;

-- name: DeleteUserScoreOutbox :exec
-- Drop a user's pending queue entries so re-enqueue stamps them with fresh values.
DELETE FROM job_score_outbox WHERE user_id = $1;

-- name: EnqueueJevScoresForProfile :execrows
-- Coarse enqueue for one profile: open/tech/enriched jobs whose category and seniority
-- match, not excluded, and lacking a FRESH score. The fine hard-blocker gate runs in the
-- worker before Jev is called. ON CONFLICT keeps one live entry per (user, job).
INSERT INTO job_score_outbox (user_id, job_id, target_version, job_posted_at)
SELECT sqlc.arg(user_id)::bigint, j.id, sqlc.arg(target_version)::int, COALESCE(j.posted_at, j.created_at)
FROM jobs j
WHERE j.closed_at IS NULL
  AND j.duplicate_of IS NULL
  AND j.is_tech IS TRUE
  AND j.description <> ''
  AND j.enriched_at IS NOT NULL
  AND j.category = ANY(sqlc.arg(specializations)::text[])
  AND (cardinality(sqlc.arg(seniorities)::text[]) = 0 OR j.seniority = ANY(sqlc.arg(seniorities)::text[]))
  AND NOT (j.company_slug = ANY(sqlc.arg(excluded_companies)::text[]))
  AND NOT (j.source = ANY(sqlc.arg(excluded_sources)::text[]))
  AND NOT EXISTS (
      SELECT 1 FROM user_job_scores s
      WHERE s.user_id = sqlc.arg(user_id)::bigint
        AND s.job_id = j.id
        AND s.score_version = sqlc.arg(target_version)::int
        AND s.model = sqlc.arg(model)::text
        AND s.profile_fingerprint = sqlc.arg(profile_fingerprint)::text
        AND s.cv_uploaded_at IS NOT DISTINCT FROM sqlc.arg(cv_uploaded_at)
        AND s.job_content_hash IS NOT DISTINCT FROM j.content_hash
  )
ON CONFLICT (user_id, job_id) DO NOTHING;

-- name: ClaimJevScoreBatch :many
-- Claim a wave of live, unleased entries for open canonical jobs, freshest first.
WITH claimable AS (
    SELECT o.id, o.user_id, o.job_id, o.target_version
    FROM job_score_outbox o
    WHERE o.failed_at IS NULL
      AND (o.claimed_at IS NULL
           OR o.claimed_at < now() - make_interval(secs => sqlc.arg(lease_seconds)::int))
      AND EXISTS (
          SELECT 1 FROM jobs j
          WHERE j.id = o.job_id AND j.closed_at IS NULL AND j.duplicate_of IS NULL
      )
    ORDER BY o.job_posted_at DESC NULLS LAST, o.job_id DESC
    FOR UPDATE OF o SKIP LOCKED
    LIMIT sqlc.arg(batch_size)::int
)
UPDATE job_score_outbox o
SET claimed_at = now()
FROM claimable c
WHERE o.id = c.id
RETURNING o.id, o.user_id, o.job_id, o.target_version;

-- name: DeleteJevScoreEntries :exec
DELETE FROM job_score_outbox WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- name: DeleteIneligibleJevScoreOutbox :execrows
-- Reap live entries the claim can never take: job gone/closed/duplicate. Leaves
-- dead-lettered rows (failed_at set) alone. Bounded by max_rows.
DELETE FROM job_score_outbox
WHERE id IN (
    SELECT o.id
    FROM job_score_outbox o
    LEFT JOIN jobs j ON j.id = o.job_id
    WHERE o.failed_at IS NULL
      AND (j.id IS NULL OR j.closed_at IS NOT NULL OR j.duplicate_of IS NOT NULL)
    LIMIT sqlc.arg(max_rows)::int
);

-- name: RecordJevScoreFailure :one
-- Count a failed attempt; dead-letter at max_attempts (posting-fault) or past the
-- upstream grace window (outage). Lease left in place as the crash reaper.
UPDATE job_score_outbox
SET attempts   = attempts + 1,
    last_error = sqlc.arg(last_error),
    failed_at  = CASE
                     WHEN sqlc.arg(posting_at_fault)::boolean
                         THEN CASE WHEN attempts + 1 >= sqlc.arg(max_attempts)::int THEN now() END
                     ELSE CASE
                              WHEN sqlc.arg(upstream_grace_days)::int > 0
                                  AND created_at < now() - make_interval(days => sqlc.arg(upstream_grace_days)::int)
                                  THEN now()
                          END
                 END
WHERE id = sqlc.arg(id)
RETURNING attempts, failed_at;
