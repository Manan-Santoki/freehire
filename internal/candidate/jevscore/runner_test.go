package jevscore

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/candidate/jeveligible"
	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/jev"
)

type fakeStore struct {
	claimed   []Claimed
	completed []Claimed
	failed    []Claimed
	failErr   error
}

func (f *fakeStore) Reap(context.Context, int) (int, error) { return 0, nil }

func (f *fakeStore) Claim(_ context.Context, batch, _ int) ([]Claimed, error) {
	out := f.claimed
	f.claimed = nil
	if len(out) > batch {
		out = out[:batch]
	}
	return out, nil
}

func (f *fakeStore) Complete(_ context.Context, c Claimed, _ Score, _ string) error {
	f.completed = append(f.completed, c)
	return nil
}

func (f *fakeStore) Fail(_ context.Context, c Claimed, err error) (bool, error) {
	f.failed = append(f.failed, c)
	f.failErr = err
	return false, nil
}

// fakeRunnerDecider counts Decide calls so the test can assert Jev is never asked about
// an ineligible pair. Named distinctly from scorer_test.go's fakeDecider (same package).
type fakeRunnerDecider struct {
	calls int
	resp  jev.Response
	err   error
}

func (f *fakeRunnerDecider) Decide(context.Context, string, map[string]jev.Question) (jev.Response, error) {
	f.calls++
	return f.resp, f.err
}

func TestRunnerScoresEligibleAndSkipsIneligible(t *testing.T) {
	eligible := Claimed{
		ID: 1, UserID: 10, JobID: 100, Version: 1,
		Job: db.Job{Category: "backend"},
		Candidate: jeveligible.Candidate{
			Profile: userprofile.Profile{Specializations: []string{"backend"}},
		},
	}
	ineligible := Claimed{
		ID: 2, UserID: 10, JobID: 101, Version: 1,
		Job: db.Job{Category: "sales"},
		Candidate: jeveligible.Candidate{
			Profile: userprofile.Profile{Specializations: []string{"backend"}},
		},
	}

	store := &fakeStore{claimed: []Claimed{eligible, ineligible}}
	decider := &fakeRunnerDecider{resp: jev.Response{
		Model: "jev-test",
		Answers: map[string]jev.Answer{
			"match_score":        {Type: "score", Score: 3, Confidence: 0.9},
			"role_category":      {Type: "choice", Choice: "backend"},
			"has_required_stack": {Type: "noul", Noul: 0.8},
			"fits_level":         {Type: "noul", Noul: 0.8},
			"hard_blocker":       {Type: "noul", Noul: 0},
		},
	}}
	scorer := &Scorer{decider: decider, th: Thresholds{ApplyMin: 60, MaybeMin: 45, HardBlockMax: 0.5}}

	runner := Runner{Scorer: scorer, Store: store}
	// Runner.Run reuses Concurrency as outbox.RunOptions.BatchSize, so it must cover both
	// claimed entries in one wave — fakeStore.Claim (like every real Claim) drops
	// whatever a wave doesn't ask for.
	stats, err := runner.Run(context.Background(), RunOptions{Concurrency: 2, LeaseSeconds: 60})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if stats.Scored != 1 {
		t.Errorf("Scored = %d, want 1", stats.Scored)
	}
	if stats.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", stats.Skipped)
	}
	if stats.Failed != 0 || stats.DeadLettered != 0 {
		t.Errorf("Failed/DeadLettered = %d/%d, want 0/0", stats.Failed, stats.DeadLettered)
	}
	if decider.calls != 1 {
		t.Errorf("Jev calls = %d, want 1 (ineligible pair must never reach Jev)", decider.calls)
	}
	if len(store.completed) != 2 {
		t.Fatalf("completed = %d, want 2 (both eligible-scored and ineligible-dropped go through Complete)", len(store.completed))
	}
	if len(store.failed) != 0 {
		t.Errorf("failed = %d, want 0", len(store.failed))
	}

	// The eligible entry's Complete call must carry a non-empty model and a scored
	// verdict; the ineligible entry's must carry the empty-model drop signal.
	var gotEligible, gotIneligible bool
	for _, c := range store.completed {
		switch c.ID {
		case eligible.ID:
			gotEligible = true
		case ineligible.ID:
			gotIneligible = true
		}
	}
	if !gotEligible || !gotIneligible {
		t.Fatalf("completed entries = %+v, want both ids 1 and 2", store.completed)
	}
}

func TestRunnerDisabledScorerIsANoOp(t *testing.T) {
	store := &fakeStore{claimed: []Claimed{{ID: 1}}}
	runner := Runner{Scorer: NewScorer(nil, Thresholds{}), Store: store}

	stats, err := runner.Run(context.Background(), RunOptions{Concurrency: 1, LeaseSeconds: 60})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats != (Stats{}) {
		t.Errorf("stats = %+v, want zero value", stats)
	}
	if len(store.claimed) != 1 {
		t.Errorf("Claim must never be called when the Scorer is disabled")
	}
}

func TestRunnerRecordsFailureOnJevError(t *testing.T) {
	c := Claimed{
		ID: 1, UserID: 10, JobID: 100, Version: 1,
		Job: db.Job{Category: "backend"},
		Candidate: jeveligible.Candidate{
			Profile: userprofile.Profile{Specializations: []string{"backend"}},
		},
	}
	store := &fakeStore{claimed: []Claimed{c}}
	decider := &fakeRunnerDecider{err: errBoom}
	scorer := &Scorer{decider: decider, th: Thresholds{}}

	runner := Runner{Scorer: scorer, Store: store}
	stats, err := runner.Run(context.Background(), RunOptions{Concurrency: 1, LeaseSeconds: 60})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Failed != 1 {
		t.Errorf("Failed = %d, want 1", stats.Failed)
	}
	if len(store.failed) != 1 {
		t.Fatalf("failed entries = %d, want 1", len(store.failed))
	}
	if len(store.completed) != 0 {
		t.Errorf("completed = %d, want 0 on a Jev failure", len(store.completed))
	}
}

var errBoom = &boomErr{}

type boomErr struct{}

func (*boomErr) Error() string { return "boom" }
