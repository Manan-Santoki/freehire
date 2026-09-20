package handler

import (
	"context"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/platform/db"
)

// forYouStore is the data ForYou reads: the personalized feed page and its total, both
// scoped to (user, verdict, min match_pct). *db.Queries satisfies it; tests inject a fake
// so the ordering, filtering and pagination envelope are exercised without Postgres.
type forYouStore interface {
	ForYouFeed(ctx context.Context, arg db.ForYouFeedParams) ([]db.ForYouFeedRow, error)
	CountForYou(ctx context.Context, arg db.CountForYouParams) (int64, error)
}

// ForYouJob is one row of the caller's personalized feed: the job's public listing fields
// alongside the cached Jev verdict that ranked it. Named ForYouJob (not FeedRow/Job/Score)
// so it does not collide on the wire with jobmatch.JobMatch or jevscore.Score, the two
// other match-surface types already generated into contracts.ts.
type ForYouJob struct {
	Slug             string   `json:"slug"`
	Title            string   `json:"title"`
	Company          string   `json:"company"`
	CompanySlug      string   `json:"company_slug"`
	Location         string   `json:"location"`
	WorkMode         string   `json:"work_mode"`
	PostedAt         *string  `json:"posted_at"`
	ClosedAt         *string  `json:"closed_at"`
	Skills           []string `json:"skills"`
	MatchPct         int      `json:"match_pct"`
	Verdict          string   `json:"verdict"`
	HasRequiredStack float64  `json:"has_required_stack"`
	FitsLevel        float64  `json:"fits_level"`
	HardBlocker      float64  `json:"hard_blocker"`
	RoleCategory     string   `json:"role_category"`
}

// ForYou serves the caller's personalized feed: Postgres-ranked scored jobs (best match
// first), optionally filtered by verdict (APPLY/MAYBE/SKIP, "" = all) and a minimum
// match_pct, paginated with the shared list envelope. Cookie-auth; a caller with no scored
// jobs yet (never enqueued, or the worker hasn't caught up) gets an empty page rather than
// an error — the feed is additive to Meili search, never a hard dependency.
func (h *matchHandlers) ForYou(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	limit, offset, err := pageParams(c)
	if err != nil {
		return err
	}
	verdict := c.Query("verdict") // "", APPLY, MAYBE, SKIP
	minPct := c.QueryInt("min", 0)

	rows, err := h.forYou.ForYouFeed(c.Context(), db.ForYouFeedParams{
		UserID:  userID,
		Verdict: verdict,
		MinPct:  int32(minPct),
		Lim:     int32(limit),
		Off:     int32(offset),
	})
	if err != nil {
		return err
	}
	total, err := h.forYou.CountForYou(c.Context(), db.CountForYouParams{
		UserID:  userID,
		Verdict: verdict,
		MinPct:  int32(minPct),
	})
	if err != nil {
		return err
	}
	return listResponse(c, toForYouJobs(rows), total, limit, offset)
}

// toForYouJobs maps the generated feed rows to the wire type, never returning nil so an
// empty page serves `"data":[]` rather than `"data":null`.
func toForYouJobs(rows []db.ForYouFeedRow) []ForYouJob {
	out := make([]ForYouJob, 0, len(rows))
	for _, r := range rows {
		out = append(out, ForYouJob{
			Slug:             r.PublicSlug,
			Title:            r.Title,
			Company:          r.Company,
			CompanySlug:      r.CompanySlug,
			Location:         r.Location,
			WorkMode:         r.WorkMode,
			PostedAt:         isoOrNil(r.PostedAt),
			ClosedAt:         isoOrNil(r.ClosedAt),
			Skills:           r.Skills,
			MatchPct:         int(r.MatchPct),
			Verdict:          r.Verdict,
			HasRequiredStack: float64(r.HasRequiredStack),
			FitsLevel:        float64(r.FitsLevel),
			HardBlocker:      float64(r.HardBlocker),
			RoleCategory:     r.RoleCategory,
		})
	}
	return out
}
