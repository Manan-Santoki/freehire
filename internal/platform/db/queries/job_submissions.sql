-- name: CreateSubmission :one
-- Insert a user-contributed vacancy into the moderation queue as 'pending'. The partial
-- unique index on lower(url) WHERE status='pending' rejects a second pending submission of
-- the same URL (the repository maps that unique violation to a 409).
INSERT INTO job_submissions (
    submitted_by, url, source, title, company, location, remote, description, posted_at,
    skills, regions, cities, work_mode, employment_type, seniority,
    salary_min, salary_max, salary_currency, salary_period
) VALUES (
    sqlc.arg(submitted_by)::bigint, sqlc.arg(url), sqlc.arg(source), sqlc.arg(title),
    sqlc.arg(company), sqlc.arg(location), sqlc.arg(remote), sqlc.arg(description),
    sqlc.arg(posted_at),
    COALESCE(sqlc.arg(skills)::text[], '{}'), COALESCE(sqlc.arg(regions)::text[], '{}'),
    COALESCE(sqlc.arg(cities)::text[], '{}'), sqlc.arg(work_mode),
    sqlc.arg(employment_type), sqlc.arg(seniority),
    sqlc.arg(salary_min), sqlc.arg(salary_max), sqlc.arg(salary_currency), sqlc.arg(salary_period)
)
RETURNING *;

-- name: GetSubmission :one
-- Load a single submission by id for the review path. The approve/reject flow guards the
-- status in the service; the Mark* queries are additionally scoped to status='pending' as
-- defense-in-depth against a concurrent second decision.
SELECT * FROM job_submissions WHERE id = $1;

-- name: ListPendingSubmissions :many
-- The moderator review queue: pending submissions, newest first, with the submitter's
-- email so the moderator can judge provenance. Capped at 500 as a runaway-growth
-- guard — far above any plausible backlog; a queue that deep needs bulk triage,
-- not a longer page.
-- sqlc.embed keeps the submission row as one db.JobSubmission instead of a flat row type
-- unrelated to it, so the adapter maps it once (fromRow) rather than re-assembling it here
-- (see mentorship.sql's ListBookingsByMentor for the same shape).
SELECT sqlc.embed(s), u.email AS submitter_email
FROM job_submissions s
JOIN users u ON u.id = s.submitted_by
WHERE s.status = 'pending'
ORDER BY s.created_at DESC
LIMIT 500;

-- name: ListSubmissionsByUser :many
-- "My submissions": one user's submissions, newest first, whatever their status.
-- LEFT JOIN the minted job (present only once approved) to surface its public_slug,
-- so the UI can link an approved submission straight to its live vacancy page.
-- sqlc.embed, see ListPendingSubmissions above.
SELECT sqlc.embed(s), j.public_slug AS job_slug
FROM job_submissions s
LEFT JOIN jobs j ON j.id = s.job_id
WHERE s.submitted_by = $1
ORDER BY s.created_at DESC;

-- name: ClaimSubmissionForApproval :one
-- Claim-first half of approval: atomically flips a pending submission to 'approved' and
-- records the reviewing moderator, leaving job_id NULL until AttachSubmissionJob records
-- the mint. Scoped to status='pending', so this is the guarded transition a concurrent
-- Reject on the same row always loses (whichever call flips the status first wins; the
-- other affects 0 rows, mapped to ErrAlreadyDecided by the service). Running this BEFORE
-- the mint — rather than marking approved only after, as the single MarkSubmissionApproved
-- update used to — closes the race where a concurrent Reject could flip the status between
-- the mint and the mark: the job would exist live while the submission stayed 'rejected'
-- with no job_id pointing at it.
UPDATE job_submissions
SET status      = 'approved',
    reviewed_by = sqlc.arg(reviewed_by)::bigint,
    reviewed_at = now()
WHERE id = sqlc.arg(id) AND status = 'pending'
RETURNING *;

-- name: AttachSubmissionJob :one
-- Records the minted job on a submission ClaimSubmissionForApproval already claimed.
-- Scoped to status='approved', not 'pending' — by this point the claim has already moved
-- it there, and the guard exists so this never resurrects a submission some other path
-- moved on (in practice unreachable, since only Approve's own claim reaches this status).
UPDATE job_submissions
SET job_id = sqlc.arg(job_id)::bigint
WHERE id = sqlc.arg(id) AND status = 'approved'
RETURNING *;

-- name: MarkSubmissionRejected :one
-- Mark a pending submission rejected with an optional reason, recording the deciding
-- moderator. Scoped to status='pending' (see ClaimSubmissionForApproval). No job is created.
UPDATE job_submissions
SET status        = 'rejected',
    reviewed_by   = sqlc.arg(reviewed_by)::bigint,
    reviewed_at   = now(),
    review_reason = sqlc.arg(review_reason)
WHERE id = sqlc.arg(id) AND status = 'pending'
RETURNING *;

-- name: ListPendingSubmissionURLs :many
-- id+url of every pending submission, used only to find which OTHER pending rows share the
-- host being blocked (see RejectAndBlockHost in the submission package): host matching needs
-- Go's net/url normalization (see submission.normalizeHost), so this fetches the candidates
-- and the caller filters in Go rather than duplicating that normalization in SQL.
SELECT id, url FROM job_submissions WHERE status = 'pending';

-- name: MarkSubmissionsRejectedByIDs :many
-- Bulk-reject every given id still pending, recording the same moderator and reason on
-- each — the sibling half of RejectAndBlockHost. Scoped to status='pending' like the
-- single-row Mark* queries, so a row already decided by the time this runs is left alone.
UPDATE job_submissions
SET status        = 'rejected',
    reviewed_by   = sqlc.arg(reviewed_by)::bigint,
    reviewed_at   = now(),
    review_reason = sqlc.arg(review_reason)
WHERE id = ANY(sqlc.arg(ids)::bigint[]) AND status = 'pending'
RETURNING *;
