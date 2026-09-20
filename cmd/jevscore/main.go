// Command jevscore is the standalone Jev scoring worker. It enqueues eligible (user,job)
// pairs, then drains the outbox queue: for each claimed pair it calls Jev and writes
// user_job_scores. Run it on a schedule (e.g. cron); it processes a bounded batch and
// exits. It exits non-zero when the run finished with any failures or dead-letters, so
// cron can alert. A missing/misconfigured JEV_* leaves the Scorer disabled, in which
// case Runner.Run is a no-op — the enqueue pass above still runs (harmless: nothing ever
// drains it) so turning Jev on later needs no separate backfill step.
package main

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/candidate/jevscore"
	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/jev"
	"github.com/strelov1/freehire/internal/platform/worker"
)

func main() {
	worker.Main(run)
}

func run() int {
	// Jev config is loaded first so a misconfigured worker fails before it opens the pool.
	cfg, err := config.LoadJevScore()
	if err != nil {
		log.Printf("config: %v", err)
		return 1
	}
	client := jev.NewFromConfig(cfg.Jev)
	scorer := jevscore.NewScorer(client, jevscore.Thresholds{
		ApplyMin:     cfg.ApplyMin,
		MaybeMin:     cfg.MaybeMin,
		HardBlockMax: cfg.HardBlockMax,
	})

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	store := newDBStore(pool, cfg)
	if n, err := store.EnqueuePending(ctx); err != nil {
		log.Printf("jevscore: enqueue: %v", err)
	} else {
		log.Printf("jevscore: enqueued=%d", n)
	}

	stats, err := jevscore.Runner{Scorer: scorer, Store: store}.Run(ctx, jevscore.RunOptions{
		Concurrency:  cfg.Concurrency,
		LeaseSeconds: cfg.LeaseSeconds,
	})
	if err != nil {
		log.Printf("jevscore: %v", err)
		return 1
	}
	log.Printf("jevscore done: scored=%d skipped=%d failed=%d dead=%d reaped=%d",
		stats.Scored, stats.Skipped, stats.Failed, stats.DeadLettered, stats.Reaped)
	return worker.ExitCode(stats.Failed, stats.DeadLettered)
}
