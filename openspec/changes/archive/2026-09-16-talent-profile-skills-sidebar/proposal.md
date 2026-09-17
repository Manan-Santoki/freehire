## Why

The Talent Network public card page (`/talent/<handle>`) is a narrow, single-column
layout (`max-w-3xl`) that buries the candidate's Skills list at the bottom, after
Experience, Education and Certifications. The job view page (`/jobs/<slug>`) already
establishes this product's pattern for a wide, two-column detail page with a sticky
facts sidebar. Bringing the talent card page to the same width and giving Skills a
sidebar position (mirroring the job page's "Profile match" box) makes the candidate's
stack scannable at a glance instead of requiring a scroll to the bottom of the page.

## What Changes

- Widen the talent card page container from `max-w-3xl` to `max-w-6xl` and match the
  job view page's padding (`px-5 py-6 sm:px-4`).
- Introduce a two-column responsive layout on the talent card page, matching the job
  view page's grid pattern (`lg:grid-cols-[20rem_minmax(0,1fr)]`), stacking to a single
  column below the `lg` breakpoint.
- Add a left-column "Skills" card, styled to match the job view page's sidebar box
  (`sticky top-20`, `rounded-xl border border-border bg-card p-4`, uppercase section
  label), containing the candidate's existing skill chips.
- Remove the old bottom-of-page "Skills" section — the data moves into the new sidebar
  card, it is not duplicated.
- On narrow viewports, the Skills card renders after the rest of the profile content
  (header, Open to, Experience, Education, Certifications) rather than before it, which
  is a deliberate deviation from the job view page's own mobile stacking order (where
  the sidebar renders first) — a Skills list is not the first thing a visitor should see
  on a candidate's profile.

No data, API, or capability-level behavior changes: `card.skills` is already present in
the `CandidateCard` payload and already rendered on this page today. This is purely a
front-end layout change on one route.

Avatar display/blurring is explicitly out of scope for this change; the user asked to
defer it to a later change.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None — this change alters presentation/layout only. No requirement in
`talent-network-catalog` or `talent-network-membership` changes: the same data
(`card.skills`, everything else on the page) is served and rendered, only repositioned.

## Impact

- `web/src/routes/talent/[handle]/+page.svelte` — layout restructuring only (container
  width/padding, two-column grid, new Skills sidebar card, removal of the old bottom
  Skills section).
- No backend, API, or database changes.
- No other routes or components affected.
