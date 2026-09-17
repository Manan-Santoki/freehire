## 1. `WebhookSettingsView`

- [x] 1.1 Write a failing test in a new `WebhookSettingsView.spec.ts`: render
      under a mocked `$app/state` with `page.data.locale = 'ru'` (mocking
      `$lib/auth.svelte`'s `isAuthenticated` true and `$lib/api`'s
      `getWebhook`/`createOrUpdateWebhook`/`setWebhookEnabled`/
      `deleteWebhook`, following `ApiKeysView.spec.ts`'s mocking shape) and
      assert: the heading, description, form label, URL placeholder, the
      "Create webhook" button label (no webhook yet), and — once a webhook
      exists — the enabled status line ("Enabled · created …"), the
      Enable/Disable and Delete button labels, and the delete-confirmation
      dialog's title/description/confirm label all render in Russian.
- [x] 1.2 Add `WebhookSettingsView.messages.ts` (en/ru) covering every
      literal string: `headTitle`, the sign-in prompt, heading, description,
      URL field label/placeholder, the three button-label states (Create
      webhook / Save / Saving…), the three form-error messages (invalid
      URL, save failed, update failed, delete failed), "Enabled"/"Disabled",
      the "created"/"last delivered"/"since" status-line connectives (the
      `timeAgo(...)` value itself is not a catalog concern — see the
      date-locale-formatting change), Enable/Disable/Delete button labels,
      and the delete dialog's title/description/confirm label.
- [x] 1.3 Migrate `WebhookSettingsView.svelte` to render through the
      catalog. Confirm 1.1 passes.
- [x] 1.4 Migrate `web/src/routes/my/webhook/+page.svelte`'s `<title>` to
      read `s.headTitle` from the same catalog, mirroring
      `routes/my/submissions/+page.svelte`'s exact pattern (import
      `locale()`/`t()`/the view's `messages`, `const s = $derived(t(messages,
      locale()))`, `<svelte:head><title>{s.headTitle}</title></svelte:head>`).
- [ ] 1.5 Manual verification under `language = ru`, including the no-webhook
      and configured-webhook states and the delete-confirmation dialog, if
      feasible; otherwise note explicitly what was not checked live. **Not
      done live in a browser this session** (no signed-in `language = ru`
      account available) — covered instead by `WebhookSettingsView.spec.ts`
      asserting the real (unmocked) Russian catalog output for the
      no-webhook state, the enabled status line, the action buttons, and the
      delete-confirmation dialog's title/description.

## 2. Wrap-up

- [x] 2.1 Run `pnpm --dir web check`, `pnpm --dir web lint`, and `pnpm --dir
      web test`; fix anything this change introduced.
- [x] 2.2 Repo-wide grep for any other file importing
      `WebhookSettingsView.messages` or relying on the pre-migration string
      shape, to catch a consumer this change's own reading missed (the same
      sweep that caught the `community/*.svelte` gap in the date-locale
      change).
