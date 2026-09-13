## Context

Confirmed live against `boldbusiness.scalis.ai`: the listing page
(`/jobs?page=N&limit=10&sortBy=SORT_BEST_MATCH`) inlines an `"initialData":{"results":[...],
"count":46,"paginationCount":46,...}` object in its RSC flight, where each result already
carries the SAME rich job shape the single-posting detail page carries (title, company,
locations, employment/workplace enums, skills, salary, and `description`/`descriptionHtml`
as `"$<id>"` text-row references) — confirmed across page 1 (10 results), page 5 (6 of 46,
the true last page) and page 6 (empty results, no redirect trap). This means the listing
alone is a `fullBoardListing` source with no per-posting detail fetch needed at all, unlike
`topco` (whose flight only carries a lazy `"$undefined"` body).

## Goals / Non-Goals

**Goals:**
- Crawl a Scalis tenant's open postings to exhaustion from the listing alone, using the
  existing `nextflight.go` primitives exactly as `deel`/`topco`/`micro1` already do.

**Non-Goals:**
- No tenant-discovery/harvest prober — only one live tenant (`boldbusiness`) is known
  today, the same reasoning `selfrecruit-source` and `hrmos-source` already gave.
- No `salary_period` mapping beyond the direct `payment` enum reading (`SALARY`→`year`,
  `HOURLY`→`hour`) — those are the only two values seen in the tenant's own filter facet
  counts, and neither salary bound was populated on any sampled posting, so the period
  mapping is untested against a real non-null example; ship the safe, obviously-correct
  reading rather than guessing further.

## Decisions

- **Page until an empty result list, not until `page*limit >= count`.** The confirmed
  past-the-end behavior (page 6: empty `results`, `count` unchanged) is a strictly simpler
  termination signal than tracking a running total against a count field that could itself
  be stale mid-walk; it also mirrors the empty-page-ends-the-walk convention already used
  by every HTML-listing paginator in this package (`crawlPagedLinks`).
- **A page-fetch/decode failure fails the whole `Fetch`, matching `fullBoardListing`.**
  Manual loop (not `crawlAllPagedLinks`, which is HTML-link-oriented) that returns the error
  immediately on any page — same "whole listing or fail outright" contract as the HTML
  paginators, implemented directly since this walk decodes flight JSON per page rather than
  collecting links.
- **Resolve the `"$<id>"` description reference the same way `deel`/`micro1` do** —
  `strings.CutPrefix(desc, "$")` then a `nextFlightTextRows(flight)` lookup, an unresolved
  reference degrading to an empty description rather than dropping the posting (consistent
  with the existing adapters' posture, and consistent with `micro1.go`'s comment on the
  same pattern).

## Risks / Trade-offs

- [Only one tenant confirmed] → the job object's field set could vary slightly on a
  differently-configured tenant. Mitigation: every field is read defensively (a missing or
  differently-shaped field decodes to its zero value via `encoding/json`, never an error),
  matching this codebase's general posture for a single-tenant-confirmed adapter.

## Migration Plan

No data migration. Ship the adapter, merge, deploy, then add `scalis/boldbusiness` by hand
via `cmd/add-board` to close `board_submissions` id 29.
