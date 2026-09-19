-- Heavy is a curator's explicit override on top of the shard-count heuristic
-- (ingestsched.Settings.IsHeavy): a provider is heavy when it is SHARDED (more than one row
-- in ingest_run_state — workday, eightfold, oracle, paylocity, join, dayforce, workstream,
-- adp and adpmyjobs already are, through that alone) or when this flag says so. It gives a
-- future single-shard-but-costly provider somewhere to land in the reserved pool without
-- having to be sharded just to get there.
--
-- This is the reservation deploy/bin/ingest-slot.sh's HEAVY_SLOTS split already makes for
-- the flock semaphore this scheduler replaces (that script's own comments: "4 -> 5 on
-- 2026-09-15") and that ingestsched.DefaultCap flagged as a SEAM not yet ported: the
-- scheduler claimed the whole fleet from one shared budget, so a burst of long sharded
-- crawls could starve the ~230-provider tail exactly the way the pre-split flock semaphore
-- measured it doing (42% of cycles skipped while average utilisation sat near half).
ALTER TABLE ingest_schedule ADD COLUMN heavy boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN ingest_schedule.heavy IS
    'Explicit heavy-pool override. A provider is ALSO heavy when ingest_run_state holds '
    'more than one row for it (sharded) -- see ingestsched.Settings.IsHeavy, which ORs the '
    'two the same way the scheduler''s claim queries do.';
