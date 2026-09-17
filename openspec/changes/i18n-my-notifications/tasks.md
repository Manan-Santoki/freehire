## 1. `/my/notifications`

- [x] 1.1 Write a failing test in a new `web/src/routes/my/notifications/page.spec.ts`
      (mirroring `routes/my/security/page.spec.ts`'s naming, mocking
      `$app/state` for `locale = 'ru'`, `$lib/api`'s `getNotifications`, and
      `$lib/notificationCenter.svelte`'s `notificationCenter`) asserting the
      "Mark all read" button (given at least one unread item) and the
      "No notifications yet." empty-state message (given zero items) render
      in Russian.
- [x] 1.2 Add `web/src/routes/my/notifications/messages.ts` (en/ru)
      covering `headTitle`, `markAllRead`, and `empty`.
- [x] 1.3 Migrate `web/src/routes/my/notifications/+page.svelte` to render
      through the catalog. Confirm 1.1 passes.
- [ ] 1.4 Manual verification under `language = ru`, including the
      mark-all-read action if feasible; otherwise note explicitly what was
      not checked live. **Not done live in a browser this session** (no
      signed-in `language = ru` account available) — covered instead by
      `page.spec.ts` asserting the real (unmocked) Russian catalog output
      for both the mark-all-read button and the empty state.

## 2. Wrap-up

- [x] 2.1 Run `pnpm --dir web check`, `pnpm --dir web lint`, and `pnpm --dir
      web test`; fix anything this change introduced.
