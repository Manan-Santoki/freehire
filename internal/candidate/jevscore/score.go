package jevscore

import (
	"math"
	"slices"

	"github.com/strelov1/freehire/internal/platform/jev"
)

// Score is the sanitized, server-owned decision for one (candidate, job).
type Score struct {
	MatchPct         int     `json:"match_pct"`
	MatchRaw         float64 `json:"match_raw"`
	MatchConfidence  float64 `json:"match_confidence"`
	RoleCategory     string  `json:"role_category"`
	RoleConfidence   float64 `json:"role_confidence"`
	HasRequiredStack float64 `json:"has_required_stack"`
	FitsLevel        float64 `json:"fits_level"`
	HardBlocker      float64 `json:"hard_blocker"`
	Verdict          string  `json:"verdict"`
}

type Thresholds struct {
	ApplyMin     int
	MaybeMin     int
	HardBlockMax float64
}

// FromAnswers maps a Jev response to a sanitized Score and computes the verdict.
func FromAnswers(resp jev.Response, th Thresholds) Score {
	a := resp.Answers
	levels := float64(len(matchRubric) - 1)
	raw := a["match_score"].Score
	pct := 0
	if levels > 0 {
		pct = int(math.Round(raw / levels * 100))
	}
	role := a["role_category"].Choice
	if !slices.Contains(RoleCategories, role) {
		role = "other_tech"
	}
	s := Score{
		MatchPct:         clampInt(pct, 0, 100),
		MatchRaw:         raw,
		MatchConfidence:  clamp01(a["match_score"].Confidence),
		RoleCategory:     role,
		RoleConfidence:   clamp01(a["role_category"].Confidence),
		HasRequiredStack: clamp01(a["has_required_stack"].Noul),
		FitsLevel:        clamp01(a["fits_level"].Noul),
		HardBlocker:      clamp01(a["hard_blocker"].Noul),
	}
	s.Verdict = verdict(s, th)
	return s
}

func verdict(s Score, th Thresholds) string {
	switch {
	case s.HardBlocker >= th.HardBlockMax || s.RoleCategory == "non_technical":
		return "SKIP"
	case s.MatchPct >= th.ApplyMin && s.HasRequiredStack >= 0.5 && s.FitsLevel >= 0.5:
		return "APPLY"
	case s.MatchPct >= th.MaybeMin:
		return "MAYBE"
	default:
		return "SKIP"
	}
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
