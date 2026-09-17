## 1. Page container and grid shell

- [x] 1.1 In `web/src/routes/talent/[handle]/+page.svelte`, change the outer wrapper
      (line 58) from `mx-auto flex w-full max-w-3xl flex-col gap-6 px-4 py-8` to the
      job view page's container sizing: `mx-auto w-full max-w-6xl px-5 py-6 sm:px-4`,
      and turn it into the two-column responsive shell matching
      `JobView.svelte:635-637`: `flex flex-col gap-4 lg:grid
      lg:grid-cols-[20rem_minmax(0,1fr)] lg:gap-x-6 lg:gap-y-4`.

## 2. Skills sidebar

- [x] 2.1 Move the existing Skills block (current lines 155-164: the `{#if
      card.skills.length}` section rendering the `Chip` list) out of the main content
      flow and into a new sidebar element placed as the LAST child in the markup
      (preserving today's mobile reading order), styled with `lg:col-start-1
      sticky top-20 flex flex-col gap-4 rounded-xl border border-border bg-card p-4`
      — matching `JobView.svelte:767-772`'s sidebar box. (An `order-last
      lg:order-none` pair was tried first but found inert given this markup — see
      design.md's "Mobile order via DOM position, not CSS `order`" — and dropped
      during code review.)
- [x] 2.2 Give the sidebar's "Skills" label the job page's uppercase section-label
      style: `text-xs font-semibold uppercase tracking-wide text-muted-foreground`
      (replacing the plain `<h2 class="text-sm font-medium">Skills</h2>` used by every
      other section on this page).
- [x] 2.3 Confirm the empty-state check (current line 166:
      `!card.skills.length && !card.roles.length && !card.education?.length &&
      !card.certifications?.length`) still reads `card.skills` correctly now that the
      Skills markup lives in a different part of the template — the condition itself
      does not need to change, only verify it still renders as before when a candidate
      has only skills and nothing else.

## 3. Right column

- [x] 3.1 Wrap the remaining sections (header, "Open to", "Experience", "Education",
      "Certifications", the empty-state paragraph, and the footer disclaimer) in the
      grid's right column: `flex min-w-0 flex-col gap-6 lg:col-start-2`.

## 4. Verification

- [x] 4.1 Run `pnpm --filter web check` (svelte-check/TypeScript) and fix any type
      errors from the markup restructuring.
- [x] 4.2 Run `pnpm --filter web lint` and fix any findings.
- [x] 4.3 Start the web dev server and view a real talent card page (e.g.
      `/talent/software-engineering-x2sz`) in a browser at desktop width: confirm the
      page matches jobview's width/padding, the Skills card renders as a sticky left
      sidebar styled like "Profile match", and the rest of the profile renders in the
      right column in its original order.
      Verified via Playwright screenshot against a temporary mocked `+page.server.ts`
      (reverted afterwards, confirmed no diff) since the local dev DB has no seeded
      Talent Network members. Matches design.
- [x] 4.4 Resize to a mobile width (or use device emulation) and confirm the stacking
      order is header → Open to → Experience → Education → Certifications → Skills →
      disclaimer (Skills last, not first).
      Verified: actual order is header → Open to → Experience → Education →
      Certifications → disclaimer → Skills (Skills renders after the disclaimer
      paragraph, not before it, because the disclaimer is the last element inside the
      main content block and Skills is a DOM sibling after it) — still satisfies "last,
      not first", which was the actual design requirement.
- [x] 4.5 Check a card with no skills (or only skills and nothing else) renders
      correctly — no empty sidebar box, and the empty-state message still appears when
      appropriate.
      Found and fixed a real bug during verification: with no skills, the sidebar
      correctly doesn't render, but the right column stayed pinned to `lg:col-start-2`,
      leaving an empty 20rem gap on desktop. Fixed by making the main content column
      conditionally span both grid columns (`lg:col-span-2`) when `card.skills.length`
      is 0. Verified with three mocked scenarios (no skills, only skills, nothing at
      all) — all render correctly after the fix.
