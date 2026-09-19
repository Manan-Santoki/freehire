## 1. Source adapter skeleton

- [x] 1.1 Add `internal/ingest/sources/dover.go`: `NewDover(c JSONGetter) Source` constructor,
      `Provider() string` returning `"dover"`, implementing `HydratingSource`.
- [x] 1.2 Write failing unit test for slug→client-id resolution (`careers-page-slug/{slug}`) against
      an `httptest.NewServer` fixture, then implement the resolve step to pass it.

## 2. Listing and pagination

- [x] 2.1 Write failing test: a board with results split across two pages of
      `careers-page/{client_id}/jobs?limit=...&offset=...` yields postings from both pages.
- [x] 2.2 Implement pagination-following list fetch to pass it.
- [x] 2.3 Write failing test: a listing entry with `is_sample=true` or `is_published=false` is
      excluded from the returned postings.
- [x] 2.4 Implement the listing-stage filter to pass it.

## 3. Per-posting hydration

- [x] 3.1 Write failing test: `FetchNew` calls the detail endpoint
      (`inbound/application-portal-job/{id}`) only for ids the `seen` callback reports unseen, and
      marks seen ones `SeenRefresh = true` without a detail call.
- [x] 3.2 Implement hydration via the shared `fetchDetails` worker-pool helper (per `getro.go`'s
      shape) to pass it.
- [x] 3.3 Write failing test: a posting whose detail marks it `active=false` or `is_private=true` is
      dropped even though it passed the listing filters.
- [x] 3.4 Implement the detail-stage filter to pass it.
- [x] 3.5 Write failing test: a posting whose detail fetch errors still yields a list-only `Job`
      (identity from the listing, no description) without aborting the rest of the board's
      hydration — per `getro.go`'s convention, not a drop.
- [x] 3.6 Implement the list-only fallback to pass it.

## 4. Posting normalization

- [x] 4.1 Write failing test mapping a full detail fixture to `Job`: `ExternalID` from the job id,
      `URL` built as `https://app.dover.com/apply/<slug>/<jobId>`, `Title`, `Company` from
      `e.Company` (the configured board's employer name, per every other single-employer
      adapter — never Dover's own `client_name`), `Description` from
      `sanitizeHTML(user_provided_description)`.
- [x] 4.2 Implement the base mapping to pass it.
- [x] 4.3 Write failing test: `locations[]` with a `COUNTRY`-typed `location_option` maps its ISO2
      code via `countriesFromCodes` into `Countries` (a `REGION`-typed entry is excluded, having no
      country); `workplace_type` maps via `workplaceTypeMode` into `WorkMode`/`Remote`.
- [x] 4.4 Implement location/work-mode mapping to pass it.
- [x] 4.5 Write failing test: `compensation.employment_type` maps onto `EmploymentType` via
      `vocab.EmploymentTypeValues`, and an unrecognized value leaves it unset rather than erroring.
- [x] 4.6 Implement the employment-type mapping to pass it.

## 5. Salary gating and folded facts

- [x] 5.1 Write failing test: `SalaryMin/Max/Currency` are set only when
      `compensation.open_to_sharing_comp == true`, and left unset (even with non-null bounds
      present internally) otherwise.
- [x] 5.2 Implement the salary gate to pass it.
- [x] 5.3 Write failing test: a posting with `offers_equity`/equity bounds set and/or
      `visa_support == true` has those facts appended to `Description` text.
- [x] 5.4 Implement the description-folding for equity and visa sponsorship to pass it.
- [x] 5.5 Write failing test: a posting with none of these facts gets no extra text appended.
- [x] 5.6 Confirm the no-op case passes without change.

## 6. Registration

- [x] 6.1 Register `dover` in `internal/ingest/sources/registry.go`'s `All(c)` as a plain
      `JSONGetter`-based entry (no special transport).
- [x] 6.2 Run `make gen-contracts` and confirm `'dover'` appears in generated `SOURCE_VALUES`
      without hand-editing `contracts.ts`.

## 7. Board recognition

- [x] 7.1 Write failing test in `internal/ingest/atsboard/board_test.go`: `/apply/<slug>/<jobId>` and
      `/jobs/<slug>` on `app.dover.com` recognize as `(dover, <slug>)`.
- [x] 7.2 Write failing test: any other `app.dover.com` path (`/pricing`, `/login`, `/crm`,
      `/docs/api`) is NOT recognized as a board.
- [x] 7.3 Implement the allow-list first-segment check in `Recognize` (per design.md — new logic,
      not a reuse of `modePath`/`reservedSegments`) to pass both.

## 8. Application-form capture

- [x] 8.1 Add `internal/ingest/applyform/dover.go`: a mapping from Dover's `application_questions[]`
      (`input_type`, `required`, `multiple_choice_options`) to `applyform.Field`, mirroring
      `FromAshby`'s shape.
- [x] 8.2 Write failing test for the field-shape mapping against a fixture built from the live
      `qompyl` sample's `application_questions[]`.
- [x] 8.3 Implement the mapping to pass it.
- [x] 8.4 Register a `doverFetcher` in `internal/ingest/applyform/fetch.go`'s `fetcherFor`, hitting
      `inbound/application-portal-job/{postingID}`.

## 9. Live verification

- [x] 9.1 Run the adapter by hand against the live `qompyl` board (`go run ./cmd/ingest dover`,
      after step 10) and confirm postings, descriptions, and locations look correct end to end.
- [x] 9.2 Check `gofmt -l .`, `go vet ./...`, `go test ./...` are clean.
- [x] 9.3 Resolve the design's Open Questions if a second live Dover board becomes available during
      review (compensation-sharing gate, additional `workplace_type`/`employment_type` spellings);
      otherwise leave them open for a future adapter fix.

## 10. Board catalog entry

- [x] 10.1 Add the `qompyl` board via `cmd/add-board` (or the `board_submissions` intake path) so the
      triggering posting actually enters the live catalogue on the next `dover` crawl.
