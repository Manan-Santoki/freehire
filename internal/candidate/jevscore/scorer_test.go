package jevscore

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/platform/jev"
)

type fakeDecider struct {
	resp     jev.Response
	gotState string
}

func (f *fakeDecider) Decide(_ context.Context, state string, _ map[string]jev.Question) (jev.Response, error) {
	f.gotState = state
	return f.resp, nil
}

func TestScorerScore(t *testing.T) {
	fd := &fakeDecider{resp: resp(3.2, 0.9, "swe", 1, 0.8, 0.8, 0.1)}
	s := &Scorer{decider: fd, th: defaultTh}
	got, err := s.Score(context.Background(), Input{JobText: "Backend Engineer", Seniorities: []string{"junior"}})
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if got.Verdict != "APPLY" {
		t.Errorf("verdict=%s", got.Verdict)
	}
	if fd.gotState == "" {
		t.Error("state not built/sent")
	}
}

func TestScorerNilIsDisabled(t *testing.T) {
	if NewScorer(nil, defaultTh).Enabled() {
		t.Error("nil client scorer must be disabled")
	}
}
