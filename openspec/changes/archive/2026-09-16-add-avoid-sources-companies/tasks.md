## 1. Data model

- [x] 1.1 Add migration `migrations/0167_user_profile_excluded_sources_companies.sql` adding `excluded_sources text[] NOT NULL DEFAULT '{}'::text[]` and `excluded_companies text[] NOT NULL DEFAULT '{}'::text[]` to `user_profiles`, mirroring `migrations/0039_user_profile_excluded_skills.sql`.
- [x] 1.2 Update `internal/platform/db/queries/user_profile.sql` (the upsert / upsert-if-unchanged queries) to read/write the two new columns; run `make sqlc` and check in the regenerated `internal/platform/db` output.

## 2. Backend domain (`internal/identity/userprofile`)

- [x] 2.1 Add `ExcludedSources []string` and `ExcludedCompanies []string` to `Profile`.
- [x] 2.2 Add `maxExcludedLen = 100` and reuse `maxSkills` (200) as the shared cap; add `ErrTooManySources`/`ErrTooManyCompanies` sentinels.
- [x] 2.3 Write `normalizeExcludedSet(values []string, maxLen, maxCount int) ([]string, error)` (trim, lowercase, dedup preserving first-seen order, drop over-`maxLen` values, error past `maxCount`) with unit tests covering: normalization, dedup, an over-long value dropped not rejected, a set at the cap stored whole, a set past the cap rejected.
- [x] 2.4 Extend `Repository` interface (`Upsert`, `UpsertIfUnchanged`) with `excludedSources, excludedCompanies []string` positional params; update the real implementation and any test fakes.
- [x] 2.5 Extend `Service.Save` with `excludedSources, excludedCompanies []string` params, calling `normalizeExcludedSet` for each (no wanted/avoided subtraction — there is no corresponding "have" list); thread the values into `s.repo.Upsert`. Update `MergeSkills`'s two `Upsert`/`UpsertIfUnchanged` call sites to pass through `profile.ExcludedSources`/`profile.ExcludedCompanies` unchanged.
- [x] 2.6 Update `Service.Save`'s doc comment to describe the two new fields.

## 3. Backend API (`internal/api/handler/me_profile.go`)

- [x] 3.1 Add `ExcludedSources`/`ExcludedCompanies` (`[]string`, `json:"excluded_sources"`/`json:"excluded_companies"`) to `saveProfileRequest` and `profileResponse`; wire them through `toProfileResponse` and the `h.userProfile.Save(...)` call in `PutProfile`.
- [x] 3.2 Map `userprofile.ErrTooManySources`/`ErrTooManyCompanies` to `400` in `profileError`.
- [x] 3.3 Handler-level tests, alongside the existing profile handler tests (these are plain unit tests against `fakeProfileRepo`, not integration-tagged — following the file's actual existing convention rather than the task's original wording): save with excluded sources/companies round-trips on GET; over-cap list rejected with 400; values normalized (case/whitespace/dedup) same as `excluded_skills`.

## 4. Frontend data layer

- [x] 4.1 Add `excluded_sources: string[]` and `excluded_companies: string[]` to `UserProfile` in `web/src/lib/types.ts`.
- [x] 4.2 Extend the save-profile payload builder in `web/src/lib/api.ts` to send the two new fields.
- [x] 4.3 Add `avoidSource()`/`unavoidSource()` and `avoidCompany()`/`unavoidCompany()` to `web/src/lib/profile.svelte.ts`, mirroring `avoidSkill()`/`unavoidSkill()`. (Backed by a new pure-function module `profileExclusions.ts` — sources/companies have no "wanted" counterpart list, so the toggle logic is simpler than `profileSkills.ts`'s and didn't fit reusing it.)

## 5. Frontend UI — move skills-to-avoid, add the Avoid tab

- [x] 5.1 Add the "Avoid" tab entry to the nav array in `web/src/routes/my/profile/+layout.svelte`, pointing at `/my/profile/avoid`.
- [x] 5.2 Create the thin route `web/src/routes/my/profile/avoid/+page.svelte` rendering a new `AvoidCard.svelte`, mirroring `skills/+page.svelte`.
- [x] 5.3 Remove the "Skills to avoid" block from `web/src/lib/components/profile/SkillsPicker.svelte` (and its `excludedSkills`/`toggleExcludedSkill` plumbing that only served that block) so the Skills tab shows only the wanted-skills control; `ProfileForm.svelte` also used the removed block (during first-time set-up) and was updated to drop it; `onboarding/SkillsStep.svelte` never had it ("no avoid half" by its own header comment) — nothing to change there.
- [x] 5.4 Build `AvoidCard.svelte`: a "Skills to avoid" block reusing the same `RemoteSearchSelect` + `skillDictionary` pattern moved from `SkillsPicker.svelte`, backed by `profileStore.avoidSkill()`/`unavoidSkill()`.
- [x] 5.5 Add the "Sources to avoid" block to `AvoidCard.svelte`, using a new `sourceDictionary.ts` (`loadSourceDistribution`, mirroring `skillDictionary.ts`, backed by the same `source` facet distribution `sourceLabel()` labels), backed by `avoidSource()`/`unavoidSource()`.
- [x] 5.6 Add the "Companies to avoid" block to `AvoidCard.svelte`, using the existing `companySearch()` (exported from `facets.ts` for this reuse) / `api.listCompanies`, backed by `avoidCompany()`/`unavoidCompany()`.

## 6. Verification

- [x] 6.1 `go build ./... && go vet ./... && go test ./...`; `go vet -tags=integration ./...`; run the tagged profile handler tests (`go test -tags=integration ./internal/api/handler/...` or the narrower package as appropriate). (The `internal/api/handler` profile tests are plain unit tests, not integration-tagged — see 3.3.)
- [x] 6.2 `gofmt -l .` clean on touched Go files.
- [x] 6.3 Frontend: `pnpm check` (svelte-check, 0 errors) and `pnpm lint` (0 errors) clean; `pnpm test` (2066 tests) green, including new `profileExclusions.test.ts` and `AvoidCard.spec.ts`.
- [x] 6.4 Live-verified in the browser (isolated throwaway Postgres+Redis, real `go run ./cmd/server` + `pnpm dev`, Playwright-driven): the "Avoid" tab renders with three correctly-labeled sections; seeded `excluded_skills/sources/companies` render as destructive-styled chips; clicking a chip un-avoids it and the removal round-trips through `PUT /me/profile` (confirmed via a direct `GET /me/profile` after the click); the Skills tab shows only the wanted-skills control, no avoid block.

## 7. Spec sync

- [ ] 7.1 `/opsx:archive` then `/opsx:sync` once all tasks above are done and reviewed, to fold the `search-profiles` delta into `openspec/specs/search-profiles/spec.md`.
