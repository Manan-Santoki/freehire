## Why

The header's single consolidated menu mixes site navigation (Jobs, Companies,
feature pages, About) with the signed-in user's own account items (Profile,
Activity, Tracking, Inbox, API keys, My submissions, Log out). On desktop the
account items sit at the bottom of a long list, below every site nav link, so
`Log out` is often off-screen and requires scrolling the dropdown to find.
Splitting the two concerns into separate desktop controls puts account actions
one click away, right under the existing profile icon, instead of buried at
the end of an unrelated list.

## What Changes

- On desktop (`sm` breakpoint and up), the profile icon (currently a plain
  link to `/my/profile`) becomes a second dropdown trigger, opening a
  profile-only menu: Profile, Activity, Tracking, Inbox, Agent, Tailor, Search
  alerts, API keys, My submissions, then Submit a job / Moderation (moderators
  only), then Log out.
- On desktop, the existing ☰ menu keeps only site-wide navigation (Jobs,
  Companies, the rest of the site nav, About) and the theme toggle; the
  signed-in account items and the Log out action are removed from it (moved to
  the new profile menu). Sign in for a signed-out visitor stays as the
  existing direct icon action next to the menu trigger, unchanged.
- The two desktop dropdowns are mutually exclusive with each other and with
  the existing header overlays (notification bell), via the existing
  `headerOverlay` coordination.
- Mobile (below `sm`) is unchanged: the single full-screen drawer still lists
  everything, including the account section and Log out, exactly as today.
- The menu's site nav also gains an `Open` link (the open-startup transparency
  page, already linked from the footer), placed next to `About` — the one
  destination the footer already reaches that the header menu had no path to
  at all.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `header-navigation`: the desktop layout gains a second, profile-scoped menu
  trigger alongside the existing consolidated menu trigger; the consolidated
  menu's contents differ by viewport (mobile keeps everything, desktop keeps
  only site nav + theme toggle); account items and Log out move to the new
  desktop profile menu; the site nav gains an `Open` link next to `About`.

## Impact

- `web/src/lib/components/HeaderMenu.svelte`: removes the inline profile
  icon/sign-in block and the desktop-only rendering of account items and Log
  out from its dropdown panel; mobile drawer markup unchanged.
- New `web/src/lib/components/HeaderProfileMenu.svelte`: desktop-only profile
  dropdown trigger and panel.
- `web/src/lib/headerOverlay.ts`: reused as-is, no changes.
- `web/src/lib/siteNav.ts`: adds a `NAV.open` entry (`/open`, "Open") for the
  header menu to render next to `About`. `HEADER_LINKS` (the homepage's own
  row) is unaffected — that list stays the five entries it names today.
- `web/src/lib/components/TopBar.svelte`: no changes (still renders a single
  `<HeaderMenu />` in the menu slot; the new profile trigger is composed
  inside `HeaderMenu.svelte`).
