## 1. Search document: compute the facet from `source`

- [ ] 1.1 Add a small, documented provider-set constant in
      `internal/search/search` (e.g. `autoApplyProviders`) listing
      `greenhouse`, `lever`, `ashby`, `workable`. Doc comment states it is a
      manually-synced mirror of `atsapply.fillProviders` ∪
      `atsapply.browserUseProviders` (cannot import `internal/api/atsapply` —
      layering forbids `search`, layer 6, importing `api`, layer 8).
- [ ] 1.2 Add a unit test asserting the constant's exact expected 4-provider
      set (fails loudly if the list is ever edited to something unexpected).
- [ ] 1.3 Add `AutoApplyAvailable bool` to `JobDocument`
      (`internal/search/search/document.go`, after the existing `AIInterview`
      field), tagged `json:"auto_apply_available,omitempty"`.
- [ ] 1.4 Compute it inline in `search.FromJob`
      (`internal/search/search/document.go`) as a lookup of `view.Source`
      against the provider set from 1.1.
- [ ] 1.5 Add test cases (in `document_test.go` or a new
      `auto_apply_available_test.go`, mirroring `ai_interview` coverage) for:
      each of the four eligible providers marks the document; `recruitee` and
      an arbitrary non-ATS source do not; the JSON omits the key when false.

## 2. Meilisearch settings and query-filter plumbing

- [ ] 2.1 Add `"auto_apply_available"` to `FilterableAttributes` in
      `facetSettings()` (`internal/search/search/client.go`), with a comment
      mirroring the `ai_interview` one about settings-before-binary ordering.
- [ ] 2.2 Add `AutoApplyAvailableParam` exported constant
      (`internal/search/search/query_filter.go`), mirroring
      `AIInterviewParam`/`RequiresClearanceParam`.
- [ ] 2.3 Add `"auto_apply_available": "auto_apply_available"` to
      `StringFacets`, and register it as a true-or-absent param (alongside
      `requires_clearance`/`ai_interview`).
- [ ] 2.4 Add test cases (mirroring `clearance_filter_test.go` /
      `ai_interview_integration_test.go`) covering:
      `auto_apply_available=true` filters to marked postings only; omitting
      the param changes nothing; the param is included in the `/jobs/facets`
      distribution.
- [ ] 2.5 Add the new attribute to the settings-drift expectations
      (`settings_test.go`/`settings_drift_test.go`) so
      `search-settings-drift` tracks it like every other filterable
      attribute.

## 3. Frontend: filter model

- [ ] 3.1 Add `autoApplyAvailable: boolean` to the filter type in
      `web/src/lib/facetModel.ts` (near `visa`/`hideAIInterview`, ~line
      32/41), with a `false` default (~line 214/215).
- [ ] 3.2 Add serialize logic in `filtersToParams`: when true, set
      `auto_apply_available=true`; when false, omit the param entirely
      (never serialize `=false`) — mirrors the true-or-absent contract from
      the spec, not the `hideAIInterview` negated-serialization shape.
- [ ] 3.3 Add deserialize logic in `filtersFromParams`:
      `f.autoApplyAvailable = p.get('auto_apply_available') === 'true'`.
- [ ] 3.4 Include it in `activeFilterCount` (~lines 185-186/338-339).
- [ ] 3.5 Add dedicated serialize/deserialize/round-trip test cases in
      `facetModel.test.ts` for `autoApplyAvailable` (none exist yet for
      `visa`/`hideAIInterview` to mirror — follow the shape of the
      `experienceYearsMax`-style dedicated tests instead).

## 4. Frontend: checkbox UI

- [ ] 4.1 Add the "Auto-apply available" checkbox to
      `web/src/lib/components/filters/FilterModal.svelte`, alongside the
      existing "Offers visa sponsorship"/"Hide employers reported to
      interview with AI" checkboxes (~lines 482-501), wired to
      `staged.value.autoApplyAvailable` / a new `staged.setAutoApplyAvailable`
      setter.
- [ ] 4.2 Add the "Best-effort — successful submission isn't guaranteed"
      caption under the checkbox.
- [ ] 4.3 Manually verify in the running app: checking the box stages the
      filter, "Show results" applies it, the sidebar chip reflects it, and
      removing the chip clears it — per `run` skill, drive the actual UI, not
      just the unit tests.

## 5. Rollout

- [ ] 5.1 Confirm deploy order in the release checklist/notes: Meilisearch
      settings patch reaches the live index before the binary carrying the
      new `StringFacets` entry (same rule `clearance-facet`/`ai_interview`
      already follow) — verify `/api/v1/jobs/facets` does not 500 during
      rollout.
- [ ] 5.2 After the binary is live, run one full `make reindex` so
      pre-existing open postings from eligible providers carry the facet.
- [ ] 5.3 Spot-check post-reindex: `GET /api/v1/jobs?auto_apply_available=true`
      returns only postings whose `source` is one of the four eligible
      providers, including postings that predate this change.
