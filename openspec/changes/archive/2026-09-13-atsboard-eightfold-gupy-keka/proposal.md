## Why

While bulk-seeding freehire's board catalog from an external ATS-company inventory
(kalil0321/ats-scrapers), several providers freehire already has a working ingest adapter for
— Eightfold, Gupy, and Keka — turned out to be **entirely unrecognized** by
`internal/ingest/atsboard.Recognize`: 0% of each provider's inventory URLs resolved. This is a
plain gap, not a design choice: all three serve every tenant from the vendor's own multi-tenant
domain (`<tenant>.eightfold.ai`, `<tenant>.gupy.io`, `<tenant>.keka.com`), the exact shape
`atsboard`'s `subdomain` mode already handles for ~15 other platforms — it was simply never
added.

`atsboard` is also the accept-set for the paid crowdsourced board-contribution flow
(`internal/ingest/contribution`, see `openspec/specs/link-contributions/spec.md`'s "Recording a
novel board and awarding AI credits"), so widening it is a deliberate, reviewable decision
rather than a silent side effect of unrelated harvesting work — hence its own change, per the
guard `internal/ingest/atsdetect`'s `TestLocalShapesStayOutsideTheSharedTable` enforces.

## What Changes

- Add three `subdomain`-mode entries to `atsBoards`: `eightfold.ai` → `eightfold`, `gupy.io` →
  `gupy`, `keka.com` → `keka`.
- A pasted link on any of these three platforms now resolves to a board the same way an
  existing Recruitee/BambooHR/Personio link already does — accepted (and rewarded) as a
  contribution instead of falling through to manual review, and usable by `cmd/harvest-boards`
  seed-based discovery.
- **Explicitly excluded from this change**: Paycom. Its URL shape
  (`paycomonline.net/v4/ats/web.php/portal/<clientkey>/…`) is currently owned by
  `internal/ingest/atsdetect` as one of the five shapes deliberately kept outside the shared
  table, and moving it in is its own decision this change does not make.

## Capabilities

### Modified Capabilities
- `link-contributions`: the "Supported-ATS board recognition" requirement's accept-set now
  includes Eightfold, Gupy, and Keka.

## Impact

- `internal/ingest/atsboard/board.go`: three new `atsBoards` rows, no new extraction mode.
- No API, schema, or migration changes.
- Downstream effects, all through the existing shared table (per its own package doc — one
  definition, three consumers): `internal/ingest/contribution` (board contributions on these
  three platforms are now accepted and rewarded), `internal/ingest/linksource` (board coverage
  recognizes these hosts), `internal/ingest/boardresolve` (a company's own careers page
  embedding one of these ATSes is now detected).
