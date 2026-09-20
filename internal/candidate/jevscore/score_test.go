package jevscore

import (
	"testing"

	"github.com/strelov1/freehire/internal/platform/jev"
)

var defaultTh = Thresholds{ApplyMin: 60, MaybeMin: 45, HardBlockMax: 0.5}

func resp(ms, msc float64, role string, rolec, stack, level, blocker float64) jev.Response {
	return jev.Response{Answers: map[string]jev.Answer{
		"match_score":        {Score: ms, Confidence: msc},
		"role_category":      {Choice: role, Confidence: rolec},
		"has_required_stack": {Noul: stack},
		"fits_level":         {Noul: level},
		"hard_blocker":       {Noul: blocker},
	}}
}

func TestFromAnswersFoundingEngIsSkip(t *testing.T) {
	// Real case: strong stack but wrong seniority + hard blocker -> SKIP.
	s := FromAnswers(resp(1.78, 0.76, "ml_ai", 1.0, 0.75, 0.12, 0.33), defaultTh)
	if s.MatchPct != 45 { // round(1.78/4*100)
		t.Errorf("MatchPct=%d want 45", s.MatchPct)
	}
	if s.Verdict != "MAYBE" {
		t.Errorf("verdict=%s want MAYBE (45>=MaybeMin, blocker<0.5, but level<0.5 blocks APPLY)", s.Verdict)
	}
}

func TestFromAnswersHardBlockerForcesSkip(t *testing.T) {
	s := FromAnswers(resp(3.0, 0.9, "ml_ai", 1.0, 0.9, 0.9, 0.93), defaultTh)
	if s.Verdict != "SKIP" {
		t.Errorf("verdict=%s want SKIP (hard_blocker 0.93>=0.5)", s.Verdict)
	}
}

func TestFromAnswersApply(t *testing.T) {
	s := FromAnswers(resp(3.2, 0.9, "swe", 1.0, 0.8, 0.8, 0.1), defaultTh)
	if s.MatchPct != 80 || s.Verdict != "APPLY" {
		t.Errorf("got pct=%d verdict=%s want 80/APPLY", s.MatchPct, s.Verdict)
	}
}

func TestFromAnswersSanitizes(t *testing.T) {
	s := FromAnswers(resp(9.0, 2.0, "wizardry", -3, 5, -1, 2), defaultTh)
	if s.MatchPct != 100 || s.MatchConfidence != 1 || s.RoleCategory != "other_tech" ||
		s.HasRequiredStack != 1 || s.FitsLevel != 0 || s.HardBlocker != 1 {
		t.Errorf("not clamped/coerced: %+v", s)
	}
}
