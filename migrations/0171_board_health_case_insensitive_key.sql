-- board_health: replace the case-SENSITIVE PRIMARY KEY (provider, board, region) with a
-- case-INSENSITIVE UNIQUE INDEX, matching how `boards` — the catalog this table shadows —
-- has built its own identity since migration 0123:
--
--     CREATE UNIQUE INDEX boards_identity_key ON boards (provider, lower(board), region);
--
-- WHAT WAS WRONG. board_health's key stayed case-sensitive through the catalog's move out
-- of YAML (0006, 0077), so a board id that reaches this table under two different casings
-- (the catalogue's own casing on one code path, a lowercased legacy id on another — see
-- 0170) does not upsert onto its existing row: it INSERTS a second one, and the two age
-- independently from then on. The abandoned twin then looks exactly like a board that has
-- genuinely gone unreachable. 0170 deleted 325 such twins, but only the ones that had
-- already gone silent for 60+ days — its own comment says outright that the cause (a
-- case-sensitive key) was left standing, and that the only guard against a NEW twin
-- forming was a NOT EXISTS subquery inside ListChronicBoards
-- (internal/platform/db/queries/board_health.sql) hiding a stale twin from the chronic
-- report. That guard lived in exactly one query; any other reader of this table (
-- ListUnhealthyBoards, ProviderHealthRollup, a future one) had no such protection, because
-- an application-code workaround is not an invariant.
--
-- THIS MIGRATION, in three steps:
--
--  1. Merge any case-duplicate rows that still exist. 0170 reached only twins silent for
--     60+ days; a pair where both sides are still being crawled, or where the abandoned
--     side is younger than that, survived it. For every (provider, lower(board), region)
--     group of more than one row, keep the row with the newest evidence
--     (coalesce(last_success_at, last_error_at, first_seen_at)) and drop the rest.
--
--     Unlike 0170's pairwise `DELETE ... USING` (deliberately narrower than "keep the
--     newest": it left a tie alone, which was fine for a one-off cleanup that only had to
--     remove OBVIOUS twins), the UNIQUE INDEX two steps below requires every group to end
--     up with exactly one row, ties included — so this ranks each group with
--     row_number() and breaks a tie deterministically on `board` (a coalesce() collision
--     to the microsecond is already implausible; this only makes the outcome
--     reproducible rather than leaving it to Postgres's row order).
--
--  2. Drop the case-sensitive primary key.
--
--  3. Add the case-insensitive unique index. A plain UNIQUE INDEX, not a PRIMARY KEY:
--     nothing holds a foreign key against board_health — it is a runtime-state sidecar,
--     per 0006's own comment, not a table anything else joins to require one — and
--     boards_identity_key already sets the precedent for expressing exactly this kind of
--     identity as an index instead of a key.
--
-- The Go side moves in the same change: RecordBoardSuccess/RecordBoardFailure now upsert
-- on (provider, lower(board), region) and additionally write board = EXCLUDED.board, so a
-- future rename converges onto the existing row under its NEW casing instead of freezing
-- the first one ever seen; and the NOT EXISTS guard in ListChronicBoards is removed, since
-- the schema itself now makes the twin it was compensating for impossible to create.
WITH ranked AS (
    SELECT provider, board, region,
           row_number() OVER (
               PARTITION BY provider, lower(board), region
               ORDER BY coalesce(last_success_at, last_error_at, first_seen_at) DESC, board DESC
           ) AS rn
    FROM board_health
)
DELETE FROM board_health AS stale
USING ranked
WHERE stale.provider = ranked.provider
  AND stale.board = ranked.board
  AND stale.region = ranked.region
  AND ranked.rn > 1;

ALTER TABLE board_health DROP CONSTRAINT board_health_pkey;

-- squawk-ignore require-concurrent-index-creation -- board_health is a per-(provider, board, region) sidecar, not a jobs-scale table: low thousands of rows at most (0170 measured 353 chronic boards across the whole fleet), so the build is milliseconds and blocking writes for it costs less than CONCURRENTLY's own failure mode — it cannot run in this transaction, it waits on unrelated open transactions, and an aborted build leaves an INVALID index behind (same reasoning as 0129, 0135, 0137 for similarly small tables).
CREATE UNIQUE INDEX board_health_identity_key ON board_health (provider, lower(board), region);
