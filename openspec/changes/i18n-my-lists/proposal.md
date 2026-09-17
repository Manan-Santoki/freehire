## Why

freehire#2005 (item 1) lists `/my/lists` among the still-untranslated
`/my/**` pages. It is the fourth PR of that fan-out, following the same
`defineMessages`/`t()`/`locale()` pattern already proven on
`i18n-profile-small-cards` and `i18n-my-webhook`.

## What Changes

- `web/src/lib/components/JobListsView.svelte` gains a colocated
  `JobListsView.messages.ts` catalog (en/ru) and is migrated to render every
  literal string through it: the sign-in prompt and button, heading,
  description, the load-error and empty-state messages, the create-list
  form (label placeholders, Create/Creating…/Cancel/New list buttons), the
  per-list row (the job-count plural, the "Shared" badge, the
  rename/edit-description/share/delete action buttons' `aria-label`/`title`
  attributes — each carrying the list's own name via the `format()`
  placeholder helper), the shared-list controls (Copy link/Copied/Unshare),
  the two `window.prompt()` calls' message text (rename, edit description),
  and the delete-confirmation dialog's title (also interpolated) and confirm
  label.
- `web/src/routes/my/lists/+page.svelte`'s `<title>Job lists —
  freehire</title>` moves into the same catalog as `headTitle`, matching
  the `i18n-my-webhook`/`MySubmissionsView` precedent.
- The job-count line ("N job"/"N jobs") uses `plural()`/`plurals()` from
  `$lib/i18n/t.ts` rather than a hand-rolled `n === 1 ? … : …` ternary — the
  same reasoning `PlanView.messages.ts`'s `model call(s)` catalog leaf
  already documents (Russian takes more than two forms).
- The seven distinct error-fallback messages (create, rename,
  description-edit, share, unshare, copy-link, delete) are keyed into the
  catalog via an `errorKind` state variable, not captured as frozen strings
  at the moment of failure — the same fix `i18n-my-webhook`'s
  `formErrorKind` pattern applied, avoiding the locale-staleness bug two
  earlier reviews caught in `i18n-profile-small-cards`. Where the original
  code also surfaces the server's own `ApiError.message`, that raw message
  is kept as-is (not a catalog concern) and takes priority over the
  generic fallback, exactly as today.
- **Out of scope**: `States.svelte` (shared with public routes, unchanged
  from the `i18n-my-webhook` precedent) and every other `/my/**` page.

## Capabilities

### New Capabilities
- `account-interface-i18n`: extends the same not-yet-synced capability the
  three earlier fan-out changes in this series declared.

### Modified Capabilities
(none)

## Impact

- 1 component file, 1 new catalog file, 1 route file (title only).
- No backend, API, or database changes.
- No changes to `$lib/i18n/t.ts` — this change USES the `format()`/`plural()`
  helpers already added there, it does not modify them.
