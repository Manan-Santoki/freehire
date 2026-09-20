package handler

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/candidate/hardconstraint"
	"github.com/strelov1/freehire/internal/candidate/jevscore"
	"github.com/strelov1/freehire/internal/candidate/jobmatch"
	"github.com/strelov1/freehire/internal/platform/db"
)

// jobMatchStore is the data JobMatch reads. *db.Queries satisfies it; tests inject a fake so
// the fallback between the Jev score and the deterministic coverage is exercised without
// Postgres.
type jobMatchStore interface {
	GetJobBySlug(ctx context.Context, publicSlug string) (db.Job, error)
	GetUserJobScore(ctx context.Context, arg db.GetUserJobScoreParams) (db.GetUserJobScoreRow, error)
}

// jobMatchResponse is the profile-match payload: the deterministic skill coverage plus the
// advisory hard-constraint blockers, always present so the bar degrades to coverage-only when
// there is no cached Jev score or no structured résumé. Jev carries the richer server-owned
// verdict when a scored row exists for this (user, job); JevStale marks one whose job text has
// since changed (re-ingest moved job_content_hash) — the SPA may still show it, badged as
// stale, rather than nothing.
type jobMatchResponse struct {
	jobmatch.JobMatch
	Blockers []hardconstraint.Blocker `json:"blockers"`
	Jev      *jevscore.Score          `json:"jev,omitempty"`
	JevStale bool                     `json:"jev_stale,omitempty"`
}

// JobMatch serves the per-job profile match: how well the open job's skills are
// covered by the authenticated caller's profile skills (exact/adjacent/missing +
// coverage percent), plus deterministic hard-constraint blockers (years, education,
// certifications, work authorization, location/work-mode) as advisory warnings —
// never hiding or downranking the job. The deterministic coverage is ALWAYS the body;
// when the caller has a cached Jev score for this job it rides along as the richer verdict,
// best-effort — a missing row or a read failure never errors the request. Cookie or API
// key; an unknown slug is a 404 (pgx.ErrNoRows via the central handler) and a caller
// without a profile is a 404 (profileError). The SPA only calls this once the viewer is
// signed in with a non-empty profile, so the not-found paths are defensive.
func (h *matchHandlers) JobMatch(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	job, err := h.store.GetJobBySlug(c.Context(), c.Params("slug"))
	if err != nil {
		return err
	}
	profile, err := h.userProfile.Get(c.Context(), userID)
	if err != nil {
		return profileError(err)
	}
	blockers := h.jobBlockers(c.Context(), userID, job, profile)
	if blockers == nil {
		blockers = []hardconstraint.Blocker{}
	}
	resp := jobMatchResponse{
		JobMatch: jobmatch.Compute(job.Skills, profile.Skills),
		Blockers: blockers,
	}
	h.attachJevScore(c.Context(), userID, job, &resp)
	return c.JSON(fiber.Map{"data": resp})
}

// attachJevScore best-effort loads the caller's cached Jev score for job and attaches it to
// resp, alongside a staleness flag comparing the stored job_content_hash stamp against the
// job's live one. No row (never scored, or the score read fails) leaves resp exactly as the
// deterministic-coverage fallback built it — this never errors the request.
func (h *matchHandlers) attachJevScore(ctx context.Context, userID int64, job db.Job, resp *jobMatchResponse) {
	row, err := h.store.GetUserJobScore(ctx, db.GetUserJobScoreParams{UserID: userID, JobID: job.ID})
	if err != nil {
		return
	}
	score := jevScoreFromRow(row)
	resp.Jev = &score
	resp.JevStale = !sameJobContentHash(row.JobContentHash, job.ContentHash)
}

// jevScoreFromRow maps the generated score row to the sanitized wire type.
func jevScoreFromRow(row db.GetUserJobScoreRow) jevscore.Score {
	return jevscore.Score{
		MatchPct:         int(row.MatchPct),
		MatchRaw:         float64(row.MatchRaw),
		MatchConfidence:  float64(row.MatchConfidence),
		RoleCategory:     row.RoleCategory,
		RoleConfidence:   float64(row.RoleConfidence),
		HasRequiredStack: float64(row.HasRequiredStack),
		FitsLevel:        float64(row.FitsLevel),
		HardBlocker:      float64(row.HardBlocker),
		Verdict:          row.Verdict,
	}
}

// sameJobContentHash reports whether a stored score's job_content_hash stamp still matches
// the job's live one. Absent on both sides counts as unchanged (a job whose hash was never
// computed is never re-crawled, so its text is stable and a NULL stamp must not force a
// permanent stale flag); present on only one side is a change — the same rule
// fitanalysis.Stamps.Fresh applies to the same column.
func sameJobContentHash(stored, live pgtype.Text) bool {
	if stored.Valid != live.Valid {
		return false
	}
	return !stored.Valid || stored.String == live.String
}
