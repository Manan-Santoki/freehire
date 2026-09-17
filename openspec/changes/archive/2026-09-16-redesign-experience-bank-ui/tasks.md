## 1. Pure helper for the banner-jump target (TDD)

- [x] 1.1 Write a failing vitest test for a `findFirstUnconfirmed(bank)` helper: given an
      `ExperienceBank`, returns the bank-order-first unconfirmed atom's location
      (`{ employmentId: string | null; atomId: string }`, `employmentId: null` for `unplaced`),
      or `null` when nothing is unconfirmed. Cover: unconfirmed under the first job, unconfirmed
      only under `unplaced`, unconfirmed under a later employment when earlier ones have none,
      and no unconfirmed atoms anywhere.
- [x] 1.2 Implement `findFirstUnconfirmed` (e.g. `web/src/lib/experienceBank.ts`) to pass 1.1.

## 2. Extract `ExperienceAchievementRow.svelte`

- [x] 2.1 Move the `achievement` snippet out of `ExperienceBankView.svelte` into
      `web/src/lib/components/ExperienceAchievementRow.svelte`: same three modes (display /
      editing / promote-to-project), same props-driven callbacks (select, edit, save, cancel,
      promote, confirm, remove), same behavior.
- [x] 2.2 Remove the row's border; keep background only for the selected/unconfirmed states, at
      reduced visual weight per design.md.
- [x] 2.3 Replace the hand-rolled skill-tag `<span>` markup with `<Chip variant="brand">`, and
      the metrics/flag `<span>` markup with `<Chip variant="default">` (or the borderless muted
      style design.md calls for), preserving the `SkillIcon` inside the skill chips.
- [x] 2.4 Rework the achievement edit form (claim / context / metrics) onto `FormField` +
      `Input`/`textarea`, keeping the same fields and the "Save — this makes it yours" copy.
- [x] 2.5 Add a `scrollToAtomId` prop: when it equals this row's atom id, scroll the row into
      view and move focus to it.

## 3. Extract `ExperienceEmploymentCard.svelte`

- [x] 3.1 Move the `employmentSection` snippet into
      `web/src/lib/components/ExperienceEmploymentCard.svelte`: logo, header, summary, stack,
      edit-in-place form, remove action — same behavior, using `<ExperienceAchievementRow>` for
      each atom.
- [x] 3.2 Add local `expanded = $state(false)`; render a collapsed summary (achievement count,
      or "nothing recorded yet") by default, and the achievement list only when expanded.
- [x] 3.3 Accept `forceExpanded?: boolean` and `scrollToAtomId?: string` props; expand the card
      when `forceExpanded` is true and forward `scrollToAtomId` to the matching row.
- [x] 3.4 Replace the stack `<span>` tag markup with `<Chip variant="brand">`.
- [x] 3.5 Rework the employment edit-in-place form onto `FormField` + `Input`/`textarea`.
- [x] 3.6 Remove the card's border.

## 4. Wire the parent view

- [x] 4.1 Replace the inline `employmentSection`/`achievement` snippet usages in
      `ExperienceBankView.svelte` with the two new components; keep `bank`, `selected`, and all
      mutation functions in the parent unchanged.
- [x] 4.2 Add `bannerTarget = $state<{ employmentId: string | null; atomId: string } | null>`;
      make the "N not confirmed" banner an activatable control (button/link) that sets it via
      `findFirstUnconfirmed(bank)`.
- [x] 4.3 Pass `forceExpanded`/`scrollToAtomId` down to the matching `ExperienceEmploymentCard`
      (by `employmentId`) or to the unplaced section's rows (when `employmentId` is `null`),
      derived from `bannerTarget`.
- [x] 4.4 Rework the "Add experience" / "Add project" forms onto `FormField` + `Input`.

## 5. Visual hierarchy pass

- [x] 5.1 Review title/company/date/meta typography and spacing across the two new components
      against design.md's "quieter secondary text, more breathing room" goal; no information
      added or removed, styling only.
- [x] 5.2 Confirm the unconfirmed/selected background treatments still read clearly without the
      borders they used to sit inside.

## 6. Verify

- [x] 6.1 `pnpm --filter web test` (vitest) — the new helper test from task 1 passes.
- [x] 6.2 `pnpm --filter web check` and `pnpm --filter web lint` on the changed files.
- [x] 6.3 `pnpm --filter web dev` and manually verify every scenario in
      `specs/experience-bank/spec.md`: a job with several achievements collapses to a count and
      expands in place with no URL change; selecting achievements in two different employments
      and collapsing both still offers "Tailor with assistant" for the two-item selection;
      activating the unconfirmed banner expands the right employment (and the unplaced section,
      for an atom with no employment) and scrolls to the right achievement; add/edit forms for
      jobs, projects, and achievements still save correctly; no borders remain; skill/stack tags
      render as `Chip`.

      Verified via a scratch fixture-data preview route driven with Playwright (screenshotted,
      then deleted) rather than the full authenticated app — standing up Postgres/auth for this
      presentation-only change risked colliding with another session's shared dev database (see
      `docker ps` showing an already-running `hire-db-1`). Confirmed: no borders, `Chip`
      rendering with icons, unconfirmed-first sort, collapse/expand with no URL change,
      selection surviving collapse across employments, and the banner jump expanding the right
      card and highlighting the right row. No console/page errors.
- [x] 6.4 Remove now-dead code left in `ExperienceBankView.svelte` after the extraction (unused
      imports, the old inline snippets).
