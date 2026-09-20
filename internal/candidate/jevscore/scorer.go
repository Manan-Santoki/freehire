package jevscore

import (
	"context"

	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/platform/jev"
)

// decider is the subset of *jev.Client the Scorer needs, so tests can inject a fake.
type decider interface {
	Decide(ctx context.Context, state string, questions map[string]jev.Question) (jev.Response, error)
}

// Input is one scoring request.
type Input struct {
	Resume      resumeextract.Professional
	JobText     string
	Seniorities []string
}

// Scorer runs one Jev decision and sanitizes it into a Score.
type Scorer struct {
	decider decider
	th      Thresholds
}

// NewScorer returns a Scorer; a nil client yields a disabled Scorer (Enabled()==false).
func NewScorer(c *jev.Client, th Thresholds) *Scorer {
	if c == nil {
		return &Scorer{th: th}
	}
	return &Scorer{decider: c, th: th}
}

// Enabled reports whether the Scorer has a live decider (a non-nil Jev client).
func (s *Scorer) Enabled() bool { return s != nil && s.decider != nil }

// Score builds the Jev state, sends the typed questions, and returns the sanitized Score.
func (s *Scorer) Score(ctx context.Context, in Input) (Score, error) {
	resp, err := s.decider.Decide(ctx, BuildState(in.Resume, in.JobText), Questions(in.Seniorities))
	if err != nil {
		return Score{}, err
	}
	return FromAnswers(resp, s.th), nil
}

// ScoreWithModel is like Score but also returns the response's resolved model, which the
// worker stores as the staleness stamp (so a Jev upgrade auto-invalidates).
func (s *Scorer) ScoreWithModel(ctx context.Context, in Input) (Score, string, error) {
	resp, err := s.decider.Decide(ctx, BuildState(in.Resume, in.JobText), Questions(in.Seniorities))
	if err != nil {
		return Score{}, "", err
	}
	return FromAnswers(resp, s.th), resp.Model, nil
}
