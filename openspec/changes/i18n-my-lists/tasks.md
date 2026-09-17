## 1. `JobListsView`

- [x] 1.1 Write failing tests in a new `JobListsView.spec.ts`: mock
      `$app/state` (`locale = 'ru'`), `$lib/auth.svelte`'s `isAuthenticated`
      true, and `$lib/jobLists.svelte`'s `jobLists` store (following
      `SkillsCard.spec.ts`'s mocking shape for a similar store). Assert, in
      Russian: the heading/description, the empty-state message, the create
      form (label placeholders, New list/Create list/Cancel), the job-count
      plural for at least two counts that exercise different Russian forms
      (e.g. 1 and 5), the "Shared" badge, and the delete-confirmation
      dialog's interpolated title. (The confirm label reuses `s.deleteTitle`
      — exercised by the icon button's own `title` assertion, not asserted
      a second time independently on the dialog.)
- [x] 1.2 Add `JobListsView.messages.ts` (en/ru) covering every literal
      string identified in design.md: `headTitle`, sign-in prompt/button,
      heading, description, load-error message, empty-state message, create
      form strings, the job-count `plurals()` leaf, "Shared" badge, the four
      action buttons' `aria-label`/`title` templates (with a `{name}`
      placeholder for `format()`), the shared-list controls (Copy
      link/Copied/Unshare), the two `window.prompt()` message strings, the
      delete-dialog title template and confirm label, and an `errors` nested
      section with the seven action-keyed fallback messages.
- [x] 1.3 Migrate `JobListsView.svelte` to render through the catalog:
      - Replace the single `error: string | null` state with `errorKind`
        (a key into `s.errors`) + `errorMessage` (the server's own message,
        when present) and a `$derived error` combining them, per design.md.
        Preserve the exact reset points the original `error = null` calls
        had — do not add or remove a reset.
      - Replace the job-count ternary with `plural(locale(), l.job_count,
        s.jobCount)`.
      - Replace every `aria-label`/`title`/dialog-title template literal
        with `format(s.<key>, { name: l.name })`.
      - Replace the two `window.prompt()` message arguments with `s.<key>`.
      Confirm 1.1 passes.
- [x] 1.4 Migrate `web/src/routes/my/lists/+page.svelte`'s `<title>` to read
      `s.headTitle` from the same catalog, mirroring
      `routes/my/webhook/+page.svelte`'s exact pattern.
- [ ] 1.5 Manual verification under `language = ru`, including triggering at
      least one action failure and one `window.prompt()`, if feasible;
      otherwise note explicitly what was not checked live. **Not done live
      in a browser this session** (no signed-in `language = ru` account
      available) — covered instead by `JobListsView.spec.ts` asserting the
      real (unmocked) Russian catalog output for the empty state, the
      create form, both exercised plural forms, the delete dialog's
      interpolated title, and a create-failure fallback message.

## 2. Wrap-up

- [x] 2.1 Run `pnpm --dir web check`, `pnpm --dir web lint`, and `pnpm --dir
      web test`; fix anything this change introduced.
- [x] 2.2 Repo-wide grep for any other file importing `JobListsView` or
      `JobListsView.messages`, to catch a consumer this change's own
      reading missed.
