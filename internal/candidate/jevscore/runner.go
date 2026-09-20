package jevscore

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/candidate/jeveligible"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/outbox"
)

// Claimed is one leased (user, job) queue entry plus the job + candidate the Store
// loaded while claiming it — the runner never touches the database itself. Fingerprint
// and CVUploadedAt are the profile-side staleness stamps Complete's write carries
// forward (see ProfileFingerprint); Version is the outbox row's target_version, carried
// to UpsertUserJobScore's score_version.
type Claimed struct {
	ID           int64
	UserID       int64
	JobID        int64
	Version      int32
	Job          db.Job
	Candidate    jeveligible.Candidate
	Fingerprint  string
	CVUploadedAt pgtype.Timestamptz
}

// Store is the persistence port the Runner drains through. Claim satisfies
// outbox.Claimer[Claimed], so Run hands it straight to outbox.RunPool.
type Store interface {
	// Reap deletes live entries the claim can never take (job gone/closed/duplicate),
	// bounded by max. Called once per Run, before draining.
	Reap(ctx context.Context, max int) (int, error)
	// Claim leases up to batch live entries, loading each one's job and candidate so
	// process needs no further Store call to evaluate eligibility or score it.
	Claim(ctx context.Context, batch, leaseSeconds int) ([]Claimed, error)
	// Complete persists the outcome and removes the queue entry, in one transaction.
	// model=="" (paired with a zero Score) is the ineligible-drop path: the row is
	// simply deleted, no user_job_scores write.
	Complete(ctx context.Context, c Claimed, score Score, model string) error
	// Fail records a scoring failure (a Jev call error), returning whether it crossed
	// into dead-letter.
	Fail(ctx context.Context, c Claimed, cause error) (deadLettered bool, err error)
}

// RunOptions are the per-run knobs, mirroring enrich.RunOptions.
type RunOptions struct {
	Concurrency  int
	LeaseSeconds int
}

// Stats tallies one Run.
type Stats struct {
	Scored, Skipped, Failed, DeadLettered, Reaped int
}

// reapLimit bounds one Run's Reap call, mirroring enrich's own bound on
// DeleteIneligibleEnrichmentOutbox.
const reapLimit = 5000

// Runner drains job_score_outbox: for each eligible (user, job) pair it calls Jev and
// writes user_job_scores; an ineligible pair is dropped without ever reaching Jev.
type Runner struct {
	Scorer *Scorer
	Store  Store
}

// Run reaps ineligible entries, then drains the queue via outbox.RunPool. It is a
// deliberate no-op — before opening the pool or touching the database — when the Scorer
// has no live Jev client: an unconfigured Jev degrades to scoring nothing rather than
// failing every claimed entry.
func (r Runner) Run(ctx context.Context, opt RunOptions) (Stats, error) {
	var stats Stats
	if !r.Scorer.Enabled() {
		return stats, nil
	}

	reaped, err := r.Store.Reap(ctx, reapLimit)
	if err != nil {
		log.Printf("jevscore: reap: %v", err)
	}
	stats.Reaped = reaped

	s, err := outbox.RunPool(ctx, r.Store, outbox.RunOptions{
		BatchSize:    opt.Concurrency,
		LeaseSeconds: opt.LeaseSeconds,
		Concurrency:  opt.Concurrency,
	}, r.process)
	stats.Scored = s.Succeeded
	stats.Failed = s.Failed
	stats.DeadLettered = s.DeadLettered
	stats.Skipped = s.Discarded
	return stats, err
}

// process handles one claimed (user, job) pair: the fine gate (jeveligible.Eligible)
// drops an ineligible pair without ever calling Jev, mirroring the coarse
// EnqueueJevScoresForProfile filter's inability to see the hard-constraint blockers.
func (r Runner) process(ctx context.Context, c Claimed) outbox.Outcome {
	if !jeveligible.Eligible(c.Job, c.Candidate) {
		if err := r.Store.Complete(ctx, c, Score{}, ""); err != nil {
			log.Printf("jevscore: drop ineligible: %v", err)
			return outbox.Failed
		}
		return outbox.Discarded
	}

	score, model, err := r.Scorer.ScoreWithModel(ctx, Input{
		Resume:      c.Candidate.Resume,
		JobText:     c.Job.Description,
		Seniorities: c.Candidate.Profile.Seniorities,
	})
	if err != nil {
		dead, ferr := r.Store.Fail(ctx, c, err)
		if ferr != nil {
			log.Printf("jevscore: fail record: %v", ferr)
		}
		if dead {
			return outbox.DeadLettered
		}
		return outbox.Failed
	}

	if err := r.Store.Complete(ctx, c, score, model); err != nil {
		log.Printf("jevscore: complete: %v", err)
		return outbox.Failed
	}
	return outbox.Succeeded
}
