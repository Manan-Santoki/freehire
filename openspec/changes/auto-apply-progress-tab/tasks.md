## 1. Pause marker storage

- [x] 1.1 `web/src/lib/autoApplyPauseStorage.test.ts`: failing tests for `isAutoApplyPaused`/`setAutoApplyPaused` — defaults to `false` for an unset queue id, `true` after `setAutoApplyPaused(id, true)`, back to `false` after `setAutoApplyPaused(id, false)`, and independent across two different queue ids.
- [x] 1.2 `web/src/lib/autoApplyPauseStorage.ts`: implement, mirroring `filterStorage.ts`'s shape (feature-detect `localStorage`, wrap every access, swallow failures), keyed `hire.autoApplyPaused:<queueId>`.

## 2. Progress-stage mapping

- [x] 2.1 `web/src/lib/autoApplyProgress.test.ts`: failing tests for `autoApplyProgressSteps(status, autoApplied)` — one case per status (`tailoring`, `tailor_failed`, `pending_review`, `declined`, `approved`, `blocked`, `failed`) asserting the 3-step state tuple from design.md's mapping table, plus `autoApplyProgressSteps(null, true)` (all three `done`) and `autoApplyProgressSteps(null, false)` (asserts the function's "nothing to show" signal, however the implementation represents it — decide the exact return shape while writing the first failing test, then hold it).
- [x] 2.2 `web/src/lib/autoApplyProgress.ts`: implement to pass. Returns `AutoApplyProgressStep[] | null` (null = nothing to show).

## 3. JobDrawer: dedicated Progress tab

- [x] 3.1 Add `autoApplied = $derived(events.some((e) => e.kind === 'applied' && e.source === 'auto_apply'))` to `JobDrawer.svelte`.
- [x] 3.2 Add `'auto_apply'` to the `Tab` type; extend `TABS` to include `{ id: 'auto_apply', label: 'Progress' }` only when `autoApply != null || autoApplied`, following the existing `canSeeMail`/`emails` conditional-tab pattern.
- [x] 3.3 Move the existing `{#if autoApplyBanner?.kind === ...}` chain (currently inside the `application` tab body) into a new `{:else if tab === 'auto_apply'}` branch, unchanged — same markup, same handlers (`decideAutoApply`, `saveBankedAnswer`, answer-bank state). The `application` tab keeps everything else that shares its wrapping `<div>` today (pending-outcome picker, ledger History list, Stage select, Notes editor). Added one further `{:else if autoApplied}` branch for the post-submission case (no live `autoApply`, ledger confirms it went through) with its own short confirmation line.
- [x] 3.4 Render the progress bar at the top of the new tab branch, built from `autoApplyProgressSteps(autoApply?.status, autoApplied)`.
- [x] 3.5 Add the Pause/Continue control in the new tab branch, visible only when `autoApply?.status` is `tailoring`, `pending_review`, `approved`, or `blocked`; wire it to `isAutoApplyPaused`/`setAutoApplyPaused` keyed on `autoApply.queue_id`.

## 4. Verification

- [x] 4.1 `pnpm --filter web test` — the two new test files plus any existing JobDrawer component test must stay green. Ran the full `web` suite: 164 files, 1899 tests, all passing.
- [x] 4.2 `pnpm --filter web check` (svelte-check) over the touched `.svelte`/`.ts` files. 0 errors, 39 pre-existing warnings unrelated to this change (JobDrawer.svelte, autoApplyProgress.ts, autoApplyPauseStorage.ts all clean).
- [ ] 4.3 Manually exercise the drawer (dev server) for a job in at least one live status (e.g. `pending_review`) and confirm the Progress tab shows the moved banner, the progress bar, and the pause toggle; confirm the `application` tab no longer shows the banner.
