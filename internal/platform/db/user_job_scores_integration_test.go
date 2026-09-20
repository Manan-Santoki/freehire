//go:build integration

// Integration tests for the user_job_scores cache and job_score_outbox queue —
// upsert idempotency, the cached round-trip, claim ordering/lease/closed-and-duplicate
// skip, the reaper, and the enqueue fresh-skip gate. These are SQL behaviors (ON
// CONFLICT, FOR UPDATE SKIP LOCKED, IS NOT DISTINCT FROM staleness comparisons) that can
// only be verified against a real Postgres.
// Run with: go test -tags=integration ./internal/platform/db/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestUpsertAndGetUserJobScore(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	uid := insertUser(t, pool, "jevscore-upsert@example.test")
	job, err := ingestUpsert(ctx, q, ingestParams("acme:1", "AI Intern"))
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}

	err = q.UpsertUserJobScore(ctx, UpsertUserJobScoreParams{
		UserID: uid, JobID: job.ID, MatchPct: 80, MatchRaw: 3.2, MatchConfidence: 0.9,
		RoleCategory: "ml_ai", RoleConfidence: 1, HasRequiredStack: 0.8, FitsLevel: 0.9,
		HardBlocker: 0.1, Verdict: "APPLY", Model: "jev-1.13.0", ScoreVersion: 1,
		ProfileFingerprint: "fp1",
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := q.GetUserJobScore(ctx, GetUserJobScoreParams{UserID: uid, JobID: job.ID})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.MatchPct != 80 || got.Verdict != "APPLY" || got.Model != "jev-1.13.0" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	if got.RoleCategory != "ml_ai" || got.ScoreVersion != 1 || got.ProfileFingerprint != "fp1" {
		t.Errorf("round-trip mismatch (extra fields): %+v", got)
	}

	// Upsert again with a different verdict/pct -> overwrite in place, still one row
	// (composite PK is (user_id, job_id): idempotent by construction).
	err = q.UpsertUserJobScore(ctx, UpsertUserJobScoreParams{
		UserID: uid, JobID: job.ID, MatchPct: 40, Verdict: "SKIP",
		Model: "jev-1.13.0", ScoreVersion: 1, ProfileFingerprint: "fp1", RoleCategory: "ml_ai",
	})
	if err != nil {
		t.Fatalf("overwrite upsert: %v", err)
	}
	got2, err := q.GetUserJobScore(ctx, GetUserJobScoreParams{UserID: uid, JobID: job.ID})
	if err != nil {
		t.Fatalf("get after overwrite: %v", err)
	}
	if got2.MatchPct != 40 || got2.Verdict != "SKIP" {
		t.Errorf("overwrite failed: %+v", got2)
	}

	var rowCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM user_job_scores WHERE user_id = $1 AND job_id = $2`,
		uid, job.ID).Scan(&rowCount); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if rowCount != 1 {
		t.Errorf("row count after two upserts = %d, want 1 (ON CONFLICT DO UPDATE)", rowCount)
	}
}

// insertOutboxEntry inserts a live job_score_outbox row directly, mirroring what
// EnqueueJevScoresForProfile would produce, so the claim tests can control which jobs
// are queued without depending on the enqueue gate itself.
func insertOutboxEntry(t *testing.T, pool *pgxpool.Pool, userID, jobID int64) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO job_score_outbox (user_id, job_id, target_version, job_posted_at) VALUES ($1,$2,1,now())`,
		userID, jobID); err != nil {
		t.Fatalf("insert outbox entry: %v", err)
	}
}

func TestClaimSkipsClosedAndDuplicateAndRespectsLease(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	uid := insertUser(t, pool, "jevscore-claim@example.test")
	open, err := ingestUpsert(ctx, q, ingestParams("acme:open", "Open"))
	if err != nil {
		t.Fatalf("ingest open: %v", err)
	}
	closed, err := ingestUpsert(ctx, q, ingestParams("acme:closed", "Closed"))
	if err != nil {
		t.Fatalf("ingest closed: %v", err)
	}
	dup, err := ingestUpsert(ctx, q, ingestParams("acme:dup", "Duplicate"))
	if err != nil {
		t.Fatalf("ingest dup: %v", err)
	}
	canonical, err := ingestUpsert(ctx, q, ingestParams("acme:canonical", "Canonical"))
	if err != nil {
		t.Fatalf("ingest canonical: %v", err)
	}

	if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at = now() WHERE id = $1`, closed.ID); err != nil {
		t.Fatalf("close job: %v", err)
	}
	// duplicate_of is a derived column (see migrations/0115_jobs_derive_duplicate_of.sql):
	// a trigger overwrites any direct write to it from the three owned marker columns, so
	// the fixture must set one of those (duplicate_of_role) rather than duplicate_of itself.
	if _, err := pool.Exec(ctx, `UPDATE jobs SET duplicate_of_role = $1 WHERE id = $2`, canonical.ID, dup.ID); err != nil {
		t.Fatalf("mark duplicate: %v", err)
	}

	for _, jobID := range []int64{open.ID, closed.ID, dup.ID} {
		insertOutboxEntry(t, pool, uid, jobID)
	}

	rows, err := q.ClaimJevScoreBatch(ctx, ClaimJevScoreBatchParams{LeaseSeconds: 180, BatchSize: 10})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(rows) != 1 || rows[0].JobID != open.ID {
		t.Fatalf("want only the open job claimed, got %+v", rows)
	}
	if rows[0].UserID != uid || rows[0].TargetVersion != 1 {
		t.Errorf("claimed row fields mismatch: %+v", rows[0])
	}

	// Immediately re-claiming returns nothing: the open job's entry is now leased,
	// and the closed/duplicate entries are permanently ineligible (no matching job).
	again, err := q.ClaimJevScoreBatch(ctx, ClaimJevScoreBatchParams{LeaseSeconds: 180, BatchSize: 10})
	if err != nil {
		t.Fatalf("re-claim: %v", err)
	}
	if len(again) != 0 {
		t.Errorf("leased entry should not re-claim: %+v", again)
	}

	// Claiming with a lease of 0 seconds re-takes the same entry: the lease has
	// already expired the instant it was set.
	expired, err := q.ClaimJevScoreBatch(ctx, ClaimJevScoreBatchParams{LeaseSeconds: 0, BatchSize: 10})
	if err != nil {
		t.Fatalf("claim after lease expiry: %v", err)
	}
	if len(expired) != 1 || expired[0].JobID != open.ID {
		t.Errorf("want the open job reclaimable once its lease expires, got %+v", expired)
	}
}

func TestClaimBatchSizeAndFreshnessOrder(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	uid := insertUser(t, pool, "jevscore-claim-order@example.test")
	older, err := ingestUpsert(ctx, q, ingestParams("acme:older", "Older"))
	if err != nil {
		t.Fatalf("ingest older: %v", err)
	}
	newer, err := ingestUpsert(ctx, q, ingestParams("acme:newer", "Newer"))
	if err != nil {
		t.Fatalf("ingest newer: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET posted_at = now() - interval '2 days' WHERE id = $1`, older.ID); err != nil {
		t.Fatalf("set older posted_at: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET posted_at = now() WHERE id = $1`, newer.ID); err != nil {
		t.Fatalf("set newer posted_at: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO job_score_outbox (user_id, job_id, target_version, job_posted_at)
		 SELECT $1, id, 1, posted_at FROM jobs WHERE id = ANY($2::bigint[])`,
		uid, []int64{older.ID, newer.ID}); err != nil {
		t.Fatalf("insert outbox entries: %v", err)
	}

	// BatchSize 1 must take the freshest-posted job first.
	rows, err := q.ClaimJevScoreBatch(ctx, ClaimJevScoreBatchParams{LeaseSeconds: 180, BatchSize: 1})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(rows) != 1 || rows[0].JobID != newer.ID {
		t.Fatalf("want the newer-posted job claimed first, got %+v", rows)
	}

	rest, err := q.ClaimJevScoreBatch(ctx, ClaimJevScoreBatchParams{LeaseSeconds: 180, BatchSize: 10})
	if err != nil {
		t.Fatalf("claim rest: %v", err)
	}
	if len(rest) != 1 || rest[0].JobID != older.ID {
		t.Fatalf("want the older job claimed next, got %+v", rest)
	}
}

// TestDeleteIneligibleJevScoreOutboxReaps verifies the reaper drops live (not yet
// dead-lettered) entries whose job is gone, closed, or a duplicate, and leaves entries
// for open canonical jobs and already dead-lettered entries alone.
func TestDeleteIneligibleJevScoreOutboxReaps(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	uid := insertUser(t, pool, "jevscore-reaper@example.test")
	open, err := ingestUpsert(ctx, q, ingestParams("acme:reaper-open", "Open"))
	if err != nil {
		t.Fatalf("ingest open: %v", err)
	}
	closed, err := ingestUpsert(ctx, q, ingestParams("acme:reaper-closed", "Closed"))
	if err != nil {
		t.Fatalf("ingest closed: %v", err)
	}
	deadLettered, err := ingestUpsert(ctx, q, ingestParams("acme:reaper-dead", "DeadLettered"))
	if err != nil {
		t.Fatalf("ingest dead-lettered: %v", err)
	}

	if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at = now() WHERE id = $1`, closed.ID); err != nil {
		t.Fatalf("close job: %v", err)
	}
	// closed's entry is still live (failed_at NULL) -> reapable.
	insertOutboxEntry(t, pool, uid, closed.ID)
	// open's entry is live and its job is eligible -> must survive.
	insertOutboxEntry(t, pool, uid, open.ID)
	// deadLettered's job also gets closed, but its entry is already failed -> the
	// reaper must leave dead-lettered rows alone even when they'd otherwise qualify.
	if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at = now() WHERE id = $1`, deadLettered.ID); err != nil {
		t.Fatalf("close dead-lettered job: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO job_score_outbox (user_id, job_id, target_version, job_posted_at, failed_at) VALUES ($1,$2,1,now(),now())`,
		uid, deadLettered.ID); err != nil {
		t.Fatalf("insert dead-lettered outbox entry: %v", err)
	}

	reaped, err := q.DeleteIneligibleJevScoreOutbox(ctx, 100)
	if err != nil {
		t.Fatalf("reap: %v", err)
	}
	if reaped != 1 {
		t.Errorf("reaped = %d, want 1 (only the closed job's live entry)", reaped)
	}

	var remaining []int64
	rows, err := pool.Query(ctx, `SELECT job_id FROM job_score_outbox ORDER BY job_id`)
	if err != nil {
		t.Fatalf("query remaining: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan: %v", err)
		}
		remaining = append(remaining, id)
	}
	if len(remaining) != 2 {
		t.Fatalf("remaining entries = %v, want exactly open + dead-lettered (2 rows)", remaining)
	}
}

// TestEnqueueJevScoresForProfileSkipsFreshScores verifies the coarse enqueue gate:
// an eligible, never-scored job is queued; a job with a FRESH cached score (matching
// model/version/fingerprint/cv-stamp/content-hash) is skipped; and a second enqueue
// call is a no-op once the row exists (ON CONFLICT DO NOTHING keeps one live entry).
func TestEnqueueJevScoresForProfileSkipsFreshScores(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	uid := insertUser(t, pool, "jevscore-enqueue@example.test")

	fresh, err := ingestUpsert(ctx, q, ingestParams("acme:fresh", "Fresh"))
	if err != nil {
		t.Fatalf("ingest fresh: %v", err)
	}
	stale, err := ingestUpsert(ctx, q, ingestParams("acme:stale", "Stale"))
	if err != nil {
		t.Fatalf("ingest stale: %v", err)
	}

	// Both jobs must clear the coarse eligibility gate: open, not a duplicate, tech,
	// described, enriched, and in a matching category.
	for _, id := range []int64{fresh.ID, stale.ID} {
		if _, err := pool.Exec(ctx,
			`UPDATE jobs SET is_tech = true, enriched_at = now(), category = 'backend' WHERE id = $1`, id); err != nil {
			t.Fatalf("mark job eligible: %v", err)
		}
	}

	// fresh already has a score matching every staleness stamp the enqueue will use
	// below -> must be skipped. stale has no score at all -> must be queued.
	if err := q.UpsertUserJobScore(ctx, UpsertUserJobScoreParams{
		UserID: uid, JobID: fresh.ID, MatchPct: 90, Verdict: "APPLY",
		Model: "jev-1.13.0", ScoreVersion: 1, ProfileFingerprint: "fp1", RoleCategory: "backend",
	}); err != nil {
		t.Fatalf("seed fresh score: %v", err)
	}

	enqueueParams := EnqueueJevScoresForProfileParams{
		UserID:             uid,
		TargetVersion:      1,
		Specializations:    []string{"backend"},
		Seniorities:        []string{},
		ExcludedCompanies:  []string{},
		ExcludedSources:    []string{},
		Model:              "jev-1.13.0",
		ProfileFingerprint: "fp1",
	}

	n, err := q.EnqueueJevScoresForProfile(ctx, enqueueParams)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if n != 1 {
		t.Fatalf("enqueued rows = %d, want 1 (only the stale job)", n)
	}

	var queuedJobID int64
	if err := pool.QueryRow(ctx, `SELECT job_id FROM job_score_outbox WHERE user_id = $1`, uid).Scan(&queuedJobID); err != nil {
		t.Fatalf("query queued row: %v", err)
	}
	if queuedJobID != stale.ID {
		t.Errorf("queued job = %d, want the stale job %d", queuedJobID, stale.ID)
	}

	// Re-running the same enqueue is a no-op: the unique (user_id, job_id) constraint
	// already holds a live entry for the stale job.
	n2, err := q.EnqueueJevScoresForProfile(ctx, enqueueParams)
	if err != nil {
		t.Fatalf("re-enqueue: %v", err)
	}
	if n2 != 0 {
		t.Errorf("re-enqueue rows = %d, want 0 (ON CONFLICT DO NOTHING)", n2)
	}
}
