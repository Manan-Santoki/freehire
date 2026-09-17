## Why

A candidate can already tell freehire which skills to avoid seeing associated with a
job (`excluded_skills` on their profile), but has no way to say "don't show me this as
coming from source X" or "I don't want anything from company Y" in the same durable,
profile-level way. The candidate-facing surfaces that already read `excluded_skills`
(the filter-subscription digest exclusion, the match-analysis "avoid" action, the
in-app assistant's profile tool) would benefit from the same signal for sources and
companies, and today there is nowhere on `/my/profile` to record it.

## What Changes

- Add two new profile fields, `excluded_sources` and `excluded_companies`, stored and
  validated exactly like `excluded_skills` today: a normalized (trim, lowercase,
  dedup), capped, order-preserving list with no relation to a "have" counterpart list
  (there is no "sources I have" or "companies I have" concept).
- `excluded_sources` holds values from the existing `jobs.source` vocabulary (the
  crawl adapter/board a posting came from — greenhouse, workday, djinni, etc.), the
  same vocabulary the general job-search `source` facet already filters on.
- `excluded_companies` holds `company_slug` values, the same identity the general
  job-search `company_slug` facet and the company-search picker already use.
- `GET`/`PUT /api/v1/me/profile` read and write the two new fields alongside the
  existing ones.
- On the frontend, move the existing "Skills to avoid" control out of the Skills tab
  and introduce a new **Avoid** tab on `/my/profile` (`/my/profile/avoid`) that hosts
  all three exclusion lists: skills to avoid, sources to avoid, companies to avoid.
  The Skills tab keeps only the "skills I have" control.
- No change to general job search/filtering behavior: these lists are stored
  preferences only, with the same scope of use `excluded_skills` has today (digest
  exclusion, match-analysis avoid action, assistant tool) — extending that scope to
  sources/companies is out of scope for this change.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `search-profiles`: the profile now stores `excluded_sources` and
  `excluded_companies` in addition to `excluded_skills`, validated the same way
  (normalized, deduped, capped, no cross-list exclusivity requirement since there is
  no corresponding "have" list for sources/companies); `GET`/`PUT /api/v1/me/profile`
  carry the two new fields; the profile UI moves "skills to avoid" into a new
  dedicated "Avoid" tab and adds "sources to avoid" / "companies to avoid" controls
  there.

## Impact

- **DB**: new migration adding `excluded_sources text[]` and
  `excluded_companies text[]` (`NOT NULL DEFAULT '{}'::text[]`) to `user_profiles`,
  mirroring migration 0039.
- **sqlc**: `internal/platform/db/queries/user_profile.sql` gains the two columns;
  `make sqlc` regenerates `internal/platform/db`.
- **Backend**: `internal/identity/userprofile` (`Profile`, `Repository.Upsert`/
  `UpsertIfUnchanged`, `Service.Save`, new normalization helper, new sentinel errors)
  and `internal/api/handler/me_profile.go` (`saveProfileRequest`, `profileResponse`,
  `profileError`).
- **Frontend**: `web/src/routes/my/profile/+layout.svelte` (new nav tab), new route
  `web/src/routes/my/profile/avoid/+page.svelte`, `web/src/lib/components/profile/
  SkillsPicker.svelte` (remove the avoid block), new `AvoidCard.svelte`,
  `web/src/lib/profile.svelte.ts` (new store methods), `web/src/lib/types.ts`
  (`UserProfile` fields), `web/src/lib/api.ts` (payload fields).
- **No search/matching behavior change**: `internal/search` is untouched.
