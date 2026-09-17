## 1. Frontend: apply a title suggestion as a phrase, scoped to the title field

- [x] 1.1 In `web/src/lib/apiSuggestions.test.ts`, add failing tests for `applyParams`:
      a bare title part produces a quoted `plan.q` and a title-only field
      restriction; an embedded `"` in the title text is stripped before quoting; a
      composed suggestion (title + company) still applies the company facet
      alongside the quoted, field-restricted title.
- [x] 1.2 Extend the `ApplyPlan` shape in `apiSuggestions.ts` to carry the field
      restriction (e.g. `qFields?: string[]`), and update the title branch of
      `applyParams` to strip embedded `"` characters and wrap the result in quotes.
- [x] 1.3 In `web/src/lib/browseTarget.test.ts`, add a failing test asserting
      `browseQuery` threads the plan's field restriction through as `q_fields` on the
      `/jobs` target.
- [x] 1.4 Update `browseTarget.ts`'s `browseQuery` to set `q_fields` from the plan
      when present.
- [x] 1.5 Run `pnpm --filter web test` (or the project's equivalent) and confirm all
      four new/updated tests pass.

## 2. Backend: keep demand tracking unaffected by the new quoting

- [x] 2.1 In `internal/api/handler/search_test.go`, add a failing test for
      `recordQuery`: a raw query wrapped in a single matching pair of `"` is recorded
      under the same normalised key as the same words without quotes; a query with
      only one quote, or quotes not at both ends, is left untouched before
      normalisation (it is not the suggestion-click shape and should not be
      guessed at).
- [x] 2.2 Implement the quote-stripping in `recordQuery` (`internal/api/handler/
      search.go`), before the existing `suggest.Title(raw)` call — strip one matching
      leading and trailing `"` only, not every quote character in the string.
- [x] 2.3 Run `go test ./internal/api/handler/...` and confirm the new test passes
      alongside the existing suite.

## 3. In-place list search + pagination (found in code review)

Code review found the launcher-only fix (sections 1-2) does not cover applying a
title suggestion while already on a list page (`/jobs`, a company's job list, a
role/country page, a collection), nor pagination/facet/sort changes afterward, both
of which reproduce the original bug. See design.md's Addendum.

- [x] 3.1 In `web/src/lib/facetModel.test.ts`, add failing tests: `filtersToParams`
      serializes `qFields` as `q_fields`; `filtersFromParams` parses it back (and
      reads its absence as `null`, not `[]`); the two round-trip; `filtersWithParts`
      sets `qFields` when given and clears a previous one when not.
- [x] 3.2 Add `qFields: string[] | null` to `JobFilters` (`facetModel.ts`), wire it
      through `emptyFilters`, `filtersToParams`, `filtersFromParams`, and
      `filtersWithParts` (replace, not merge — a suggestion with no title part clears
      any prior restriction).
- [x] 3.3 Update `FilterStore.applyParts` (`filters.ts`) to accept and thread
      `qFields` through to `filtersWithParts`; update `setQuery`/`commitQuery` to
      clear `qFields` when a query is set independently of a suggestion pick.
- [x] 3.4 Update `JobsView.svelte`'s `applyParts` callback to pass `plan.qFields`
      through to `filters.applyParts`.
- [x] 3.5 Run `npx svelte-check` across `web/` to confirm the new required
      `JobFilters.qFields` field breaks no existing call site.
- [x] 3.6 Run the full web test suite and confirm no regressions.
- [x] 3.7 (Found in re-review) `browseTarget.ts`'s `browseQuery` set `q_fields` by
      manually calling `params.set` after `filtersToParams`, with a comment claiming
      `qFields` stays out of `JobFilters`/`filtersToParams` on purpose — stale as of
      3.1-3.2, and a second, divergent mechanism for the one concern the module's own
      header comment says must go through one serializer. Fixed: `browseQuery` now
      sets `f.qFields` before calling `filtersToParams`, like every other field.

`FilterStore`'s methods and the `JobsView.svelte` call site are not independently
unit-tested (the class depends on Svelte 5 runes and SvelteKit modules this
project's plain-Node vitest config cannot load — the same pre-existing boundary the
rest of `FilterStore` already sits behind). The logic they carry is a thin
pass-through to `filtersWithParts`, which IS fully covered above.

## 4. Keep the quoting wrapper out of human-facing text (found in independent review)

An independently-run review pass found `JobFilters.q`/`ApplyPlan.q` read as plain
text in three places that never anticipated the new quoting: `FilterSummary.svelte`'s
filter chip, `HeaderSearch.svelte`'s displayed search-box text, and
`JobsView.svelte`'s `role_suggestion` analytics fallback. All three would show/record
literal quote marks for a title suggestion. See design.md's Addendum 2.

- [x] 4.1 In `web/src/lib/facetModel.test.ts`, add failing tests for a new
      `displayQuery` function: strips a matching wrapping quote pair; leaves
      unquoted text, a lone quote character, a one-sided quote, and the empty string
      untouched.
- [x] 4.2 Add `displayQuery(q: string): string` to `facetModel.ts` — the inverse of
      `apiSuggestions.ts`'s `quoteForTitleSearch`.
- [x] 4.3 Apply `displayQuery` at every point `q` is read for a human, not sent to
      the search API: `FilterSummary.svelte`'s chip text, `HeaderSearch.svelte`'s
      displayed/reconciled search-box value, `JobsView.svelte`'s analytics `role`
      fallback, and (found in a follow-up sweep for every `track()` call reading
      `.q`) `JobsView.svelte`'s separate `search` analytics event's `q` field.
- [x] 4.4 Run `npx svelte-check` and the full web test suite; confirm no regressions.

## 5. Verification

- [x] 5.1 `go build ./... && go vet ./...` and `go test ./...` clean (one pre-existing,
      unrelated failure: `cmd/billing-sync`'s `TestTheStoreProviderAloneKeepsTheWorkerRunning`
      fails in this environment because an ambient local Postgres on :5432, not this
      change, answers when the test clears `DATABASE_URL` — not touched by this diff).
- [x] 5.2 `gofmt -l .` prints nothing for touched Go files; `npx eslint` clean on
      touched frontend files.
- [x] 5.3 Verified against production (freehire.me) after deploy (commit `9b383c6f0`,
      merged and released to host2 as part of `9c2a438ad`). `GET /api/v1/suggest?
      q=Founding+Engineer` shows a "Founding Engineer" title suggestion with
      `jobs: 730`. `GET /api/v1/jobs/search?q=Founding+Engineer` (the old, unscoped
      shape) returns `total: 17831` — confirms the bug was real and reproducible at
      this scale. `GET /api/v1/jobs/search?q=%22Founding+Engineer%22&q_fields=title`
      (what the fix now sends) returns `total: 1548` — an ~11.5x reduction, closing
      the large majority of the gap. Not exact parity with 730 (expected — see
      design.md's Non-Goals: quoting is AND-of-tokens-in-title, not exact-phrase
      adjacency, so e.g. "Senior Founding Engineer" now matches too). Did not
      separately browser-verify the display-quote fix (chip/search-box/analytics) live
      — covered deterministically by `displayQuery`'s unit tests and direct reading of
      each call site; the API-level fix was the load-bearing risk to confirm live.
- [x] 5.4 Confirm a suggestion click's search still appears under its plain
      (unquoted) form in `search_queries` rather than under a quoted variant —
      proven deterministically by `TestDemandKey_StripsMatchingQuotePairBeforeNormalising`
      (2.1); no live-DB spot-check needed.
