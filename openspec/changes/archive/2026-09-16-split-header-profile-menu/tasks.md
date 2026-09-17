## 1. Shared account-links data

- [x] 1.1 Write a failing source-text-audit test (`web/src/lib/headerAccountLinks.test.ts`) asserting `headerAccountLinks.ts` exports an `accountLinks` array with exactly the current eight items in order (Activity, Tracking, Inbox, Agent, Tailor, Search alerts, API keys, My submissions), each with `href`/`label`/`icon`.
- [x] 1.2 Create `web/src/lib/headerAccountLinks.ts` (moving the `accountLinks` const and its icon imports out of `HeaderMenu.svelte`) to satisfy 1.1; update `HeaderMenu.svelte` to import it instead of declaring it locally (no behavior change yet — mobile drawer still renders the same items the same way).

## 2. Desktop profile menu component

- [x] 2.1 Write a failing source-text-audit test (`web/src/lib/components/HeaderProfileMenu.test.ts`, following the pattern in `jobActionStrip.test.ts`/`PlanView.test.ts` — `web/` has no mounted-component test infra) asserting `HeaderProfileMenu.svelte`: renders the profile icon as a `aria-haspopup="menu"` dropdown trigger; the panel lists Profile, then the `headerAccountLinks.ts` items in order, then Submit a job, then a moderator-gated Moderation item, then Log out; closes via outside click, `Escape`, and `afterNavigate`; calls `openedOverlay`/`closedOverlay` from `$lib/headerOverlay`; and renders the existing direct Sign-in icon action (no dropdown) when signed out.
- [x] 2.2 Implement `web/src/lib/components/HeaderProfileMenu.svelte` to satisfy 2.1, reusing `HeaderMenu.svelte`'s existing row/icon-button styling and importing `accountLinks` from `$lib/headerAccountLinks`.

## 3. Split HeaderMenu's desktop dropdown

- [x] 3.1 Write a failing source-text-audit test (extend `web/src/lib/components/HeaderMenu.test.ts`, creating it if absent) asserting `HeaderMenu.svelte`: renders `<HeaderProfileMenu />` in place of the current inline profile-icon/sign-in block; wraps the account-items block (Profile link + `accountLinks` + divider + Submit a job + Moderation) in a `sm:hidden` container so it stays in the mobile drawer only; and no longer renders `{@render authButton()}` in the desktop-only (`hidden sm:block`) tail, while still rendering `{@render themeButton()}` there.
- [x] 3.2 Implement the `HeaderMenu.svelte` edits to satisfy 3.1.

## 4. Open link in the site nav

- [x] 4.1 Write a failing source-text-audit test (extend `web/src/lib/siteNav.test.ts`, creating it if absent) asserting `siteNav.ts` exports a `NAV.open` entry targeting `/open` labelled "Open", and that `HEADER_LINKS` is unchanged (still the same five entries).
- [x] 4.2 Add `NAV.open` to `siteNav.ts` and render it immediately after `NAV.about` in `HeaderMenu.svelte`'s nav-links list (both viewports), to satisfy 4.1.

## 5. Verify

- [x] 5.1 Run `pnpm --filter web test` and confirm the new/updated tests pass alongside the existing suite.
- [x] 5.2 Run `pnpm --filter web check` and `pnpm --filter web lint` on the touched files.
- [x] 5.3 Manually verify in a browser at desktop width. Done for the signed-out state via `pnpm --filter web dev` + Playwright (no backend running, so this covers everything reachable signed-out): the `☰` menu shows only site nav (Jobs, Companies, Collections, How it works, CV tailoring, Job notifications, Analytics, Trends, Discussions, About, Open, Dark theme) with no account items, no Submit a job, no Moderation, no Sign in/Log out; the bar shows the direct Sign-in icon with no dropdown. The signed-in profile dropdown (Log out visibility, overlay mutual-exclusion) was not live-browser-verified — that needs a real session, which needs the full `make up` stack (Docker Postgres + Go backend) — and instead relies on `HeaderProfileMenu.test.ts`'s source-text-audit coverage of that exact structure.
- [x] 5.4 Manually verify the mobile drawer (narrow viewport) is unchanged except for the added Open link. Done for the signed-out state (same tooling/caveat as 5.3): single drawer with nav links ending in About, Open, and the pinned bottom bar (GitHub, Discord, Dark theme, Sign in). The signed-in account section (Profile, Activity, Tracking, Inbox, Agent, Tailor, Search alerts, API keys, My submissions, Submit a job, Moderation) was not live-browser-verified for the same reason as 5.3 — it is untouched markup (only newly wrapped in `sm:hidden`), covered by `HeaderMenu.test.ts`.
