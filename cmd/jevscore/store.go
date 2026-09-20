package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/candidate/jeveligible"
	"github.com/strelov1/freehire/internal/candidate/jevscore"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/db"
)

// candidateEntry is one user's loaded jeveligible.Candidate plus the profile-side
// staleness stamps the write carries forward, cached per Claim wave so a wave with many
// jobs for the same user pays for the profile/résumé lookups once.
type candidateEntry struct {
	candidate    jeveligible.Candidate
	fingerprint  string
	cvUploadedAt pgtype.Timestamptz
}

// cachedCandidate wraps candidateEntry with an explicit found flag (candidateEntry holds
// slices, so it cannot be compared with == to detect a cached "no profile" miss).
type cachedCandidate struct {
	entry candidateEntry
	found bool
}

// dbStore adapts the generated queries + connection pool to jevscore.Store. It is also
// where the profile/résumé/geography needed to build each Claimed's Candidate get
// loaded — Claim is the only place that touches userProfile/resumeStore, so the runner
// itself never sees the database.
type dbStore struct {
	pool    *pgxpool.Pool
	q       *db.Queries
	version int32
	// model is the configured Jev model (config.Jev.Model), used both for
	// EnqueuePending's freshness check and as the staleness stamp Complete writes to
	// user_job_scores.model — the two must agree or every pair re-scores every run. A
	// model upgrade behind the same alias is invalidated by bumping JEVSCORE_VERSION,
	// not by the resolved response model (see jevscore.Scorer.ScoreWithModel).
	model             string
	maxAttempts       int32
	upstreamGraceDays int32

	profiles *userprofile.Service
	resumes  *resume.Store
}

func newDBStore(pool *pgxpool.Pool, cfg config.JevScore) *dbStore {
	queries := db.New(pool)
	return &dbStore{
		pool:              pool,
		q:                 queries,
		version:           int32(cfg.Version),
		model:             cfg.Model,
		maxAttempts:       int32(cfg.MaxAttempts),
		upstreamGraceDays: int32(cfg.UpstreamGraceDays),
		profiles:          userprofile.New(userprofile.NewQueriesRepository(queries)),
		// blobs is nil: this worker never touches the object store, only the résumé's
		// derived structure/geography/upload-stamp columns, none of which gate on
		// Store.Enabled() (see internal/candidate/resume.Store).
		resumes: resume.New(nil, resume.NewQueriesRepository(queries)),
	}
}

// Reap deletes live queue entries the claim can never take (job gone/closed/duplicate).
func (s *dbStore) Reap(ctx context.Context, maxRows int) (int, error) {
	n, err := s.q.DeleteIneligibleJevScoreOutbox(ctx, int32(maxRows))
	return int(n), err
}

// Claim leases a wave of live entries and, for each, loads the job and the claiming
// user's candidate (profile + résumé + geography), caching the candidate per user_id for
// the life of this wave so a wave holding several jobs for the same user pays for that
// load once. A row whose job or profile cannot be loaded is left leased rather than
// forced into a Claimed the runner cannot score correctly; a transient failure retries
// after the lease expires, and a genuinely gone job is later swept by Reap.
func (s *dbStore) Claim(ctx context.Context, batch, leaseSeconds int) ([]jevscore.Claimed, error) {
	rows, err := s.q.ClaimJevScoreBatch(ctx, db.ClaimJevScoreBatchParams{
		LeaseSeconds: int32(leaseSeconds),
		BatchSize:    int32(batch),
	})
	if err != nil {
		return nil, err
	}

	cache := make(map[int64]cachedCandidate, len(rows))
	out := make([]jevscore.Claimed, 0, len(rows))
	for _, r := range rows {
		job, err := s.q.GetJob(ctx, r.JobID)
		if err != nil {
			log.Printf("jevscore: claim: load job %d: %v", r.JobID, err)
			continue
		}

		cached, seen := cache[r.UserID]
		if !seen {
			loaded, found := s.loadCandidate(ctx, r.UserID)
			cached = cachedCandidate{entry: loaded, found: found}
			cache[r.UserID] = cached
			if !found {
				// No profile for this user any more (deleted after enqueue, before
				// drain) — this pair can never be scored. Drop it outright rather than
				// leaving it stuck: Complete's ineligible-drop path is the closest fit,
				// but Store.Complete needs a Claimed, so delete the row directly.
				if err := s.q.DeleteJevScoreEntries(ctx, []int64{r.ID}); err != nil {
					log.Printf("jevscore: claim: drop entry for missing profile (user %d): %v", r.UserID, err)
				}
				continue
			}
		} else if !cached.found {
			// A previous row in this wave already found no profile for this user; skip
			// without repeating the lookup or the delete attempt.
			continue
		}
		entry := cached.entry

		out = append(out, jevscore.Claimed{
			ID:           r.ID,
			UserID:       r.UserID,
			JobID:        r.JobID,
			Version:      r.TargetVersion,
			Job:          job,
			Candidate:    entry.candidate,
			Fingerprint:  entry.fingerprint,
			CVUploadedAt: entry.cvUploadedAt,
		})
	}
	return out, nil
}

// loadCandidate builds one user's jeveligible.Candidate plus their profile fingerprint
// and CV upload stamp. found is false when the user has no saved profile (a data race
// against enqueue, not a transient error).
func (s *dbStore) loadCandidate(ctx context.Context, userID int64) (candidateEntry, bool) {
	profile, err := s.profiles.Get(ctx, userID)
	if errors.Is(err, userprofile.ErrNotFound) {
		return candidateEntry{}, false
	}
	if err != nil {
		log.Printf("jevscore: load profile for user %d: %v", userID, err)
		return candidateEntry{}, false
	}

	var loc userprofile.LocationPreferences
	if len(profile.LocationPreferences) > 0 {
		_ = json.Unmarshal(profile.LocationPreferences, &loc) // best-effort; empty loc simply skips geo checks
	}

	var pro resumeextract.Professional
	if st, ok, err := s.resumes.Structured(ctx, userID); err == nil && ok {
		pro = st.Professional()
	}

	var derived []string
	if geo, ok, err := s.resumes.Geography(ctx, userID); err == nil && ok {
		derived = geo.Countries
	}

	var cvUploadedAt pgtype.Timestamptz
	if t, err := s.resumes.UploadedAt(ctx, userID); err == nil && t != nil {
		cvUploadedAt = pgtype.Timestamptz{Time: *t, Valid: true}
	}

	fp := jevscore.ProfileFingerprint(profile.Specializations, profile.Seniorities, profile.Skills, profile.LocationPreferences)

	return candidateEntry{
		candidate: jeveligible.Candidate{
			Profile:          profile,
			Resume:           pro,
			Loc:              loc,
			DerivedCountries: derived,
		},
		fingerprint:  fp,
		cvUploadedAt: cvUploadedAt,
	}, true
}

// Complete writes the score and removes the queue entry in one transaction. resolvedModel
// (the response model ScoreWithModel returned) is used only to detect the ineligible-drop
// path — resolvedModel=="" is paired with a zero jevscore.Score by the runner, so only the
// outbox row is deleted, no user_job_scores write. The write itself always stamps the
// CONFIGURED model (s.model), not resolvedModel: EnqueueJevScoresForProfile's freshness
// check compares against s.model too, so the two must agree or every pair re-scores every
// run (see the model field comment above). A model upgrade behind the same alias is
// invalidated by bumping JEVSCORE_VERSION instead.
func (s *dbStore) Complete(ctx context.Context, c jevscore.Claimed, score jevscore.Score, resolvedModel string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)
	if resolvedModel != "" {
		if err := qtx.UpsertUserJobScore(ctx, db.UpsertUserJobScoreParams{
			UserID:             c.UserID,
			JobID:              c.JobID,
			MatchPct:           int16(score.MatchPct),
			MatchRaw:           float32(score.MatchRaw),
			MatchConfidence:    float32(score.MatchConfidence),
			RoleCategory:       score.RoleCategory,
			RoleConfidence:     float32(score.RoleConfidence),
			HasRequiredStack:   float32(score.HasRequiredStack),
			FitsLevel:          float32(score.FitsLevel),
			HardBlocker:        float32(score.HardBlocker),
			Verdict:            score.Verdict,
			Model:              s.model,
			ScoreVersion:       c.Version,
			ProfileFingerprint: c.Fingerprint,
			CvUploadedAt:       c.CVUploadedAt,
			JobContentHash:     c.Job.ContentHash,
		}); err != nil {
			return fmt.Errorf("upsert user job score: %w", err)
		}
	}
	if err := qtx.DeleteJevScoreEntries(ctx, []int64{c.ID}); err != nil {
		return fmt.Errorf("delete outbox entry: %w", err)
	}
	return tx.Commit(ctx)
}

// Fail records a scoring failure. posting_at_fault is always false: every failure this
// worker sees is a Jev call error (transport/model), never a defect in the posting
// itself, so dead-lettering follows the upstream-grace clock rather than an attempt cap.
func (s *dbStore) Fail(ctx context.Context, c jevscore.Claimed, cause error) (bool, error) {
	row, err := s.q.RecordJevScoreFailure(ctx, db.RecordJevScoreFailureParams{
		LastError:         cause.Error(),
		PostingAtFault:    false,
		MaxAttempts:       s.maxAttempts,
		UpstreamGraceDays: s.upstreamGraceDays,
		ID:                c.ID,
	})
	if err != nil {
		return false, err
	}
	return row.FailedAt.Valid, nil
}

// EnqueuePending walks every saved profile and issues one coarse
// EnqueueJevScoresForProfile per user, stamping each with the profile fingerprint and CV
// upload time computed the same way Claim's candidate loader does (jevscore.
// ProfileFingerprint), so a profile or CV edit invalidated via InvalidateUserScores/
// DeleteUserScoreOutbox re-enqueues fresh the very next run.
func (s *dbStore) EnqueuePending(ctx context.Context) (int64, error) {
	profiles, err := s.q.ListAllUserProfiles(ctx)
	if err != nil {
		return 0, err
	}

	var total int64
	for _, p := range profiles {
		var cvUploadedAt pgtype.Timestamptz
		if t, err := s.resumes.UploadedAt(ctx, p.UserID); err != nil {
			log.Printf("jevscore: enqueue: résumé stamp for user %d: %v", p.UserID, err)
		} else if t != nil {
			cvUploadedAt = pgtype.Timestamptz{Time: *t, Valid: true}
		}

		fp := jevscore.ProfileFingerprint(p.Specializations, p.Seniorities, p.Skills, p.LocationPreferences)
		n, err := s.q.EnqueueJevScoresForProfile(ctx, db.EnqueueJevScoresForProfileParams{
			UserID:             p.UserID,
			TargetVersion:      s.version,
			Specializations:    p.Specializations,
			Seniorities:        p.Seniorities,
			ExcludedCompanies:  p.ExcludedCompanies,
			ExcludedSources:    p.ExcludedSources,
			Model:              s.model,
			ProfileFingerprint: fp,
			CvUploadedAt:       cvUploadedAt,
		})
		if err != nil {
			log.Printf("jevscore: enqueue: user %d: %v", p.UserID, err)
			continue
		}
		total += n
	}
	return total, nil
}
