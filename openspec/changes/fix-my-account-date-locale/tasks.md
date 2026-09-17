## 1. `utils.ts` core change

- [x] 1.1 Write failing tests in `web/src/lib/utils.test.ts`: format the same
      fixed instant under `locale: 'ru'` and `locale: 'en'` for `formatDate`,
      `formatDateTime`, `timeAgo` (both styles) and `formatDateOrAgo`,
      asserting the Russian output differs from the English output and
      contains no Latin-script month/relative-unit words. Also assert that
      two `timeAgo` calls with different locales in the same test do not leak
      one locale's formatter into the other (the cache-keying regression this
      change exists to prevent).
- [x] 1.2 Add `locale: Locale` as a required parameter to `formatDate`,
      `formatDateTime`, `timeAgo`, `formatDateOrAgo` in `web/src/lib/utils.ts`
      and pass it to the underlying `Intl` calls. Import `Locale` from
      `$lib/locale`. (`locale` placed before the pre-existing optional
      `style` param, since a required param cannot follow an optional one —
      call convention is now `timeAgo(ts, locale, style?)`.)
- [x] 1.3 Rekey the `relativeTime` cache from `Partial<Record<TimeAgoStyle,
      Intl.RelativeTimeFormat>>` to `Partial<Record<\`${Locale}:${TimeAgoStyle}\`,
      Intl.RelativeTimeFormat>>` and update the cache comment (the
      "runtime locale cannot change within a session" justification no longer
      holds once locale is a parameter).
- [x] 1.4 Run the new tests; confirm GREEN. (17/17 passing.)

## 2. `/my/**` call sites

- [x] 2.1 `lib/components/BoardList.svelte` — pass `locale()` to its
      `timeAgo` call.
- [x] 2.2 `lib/components/JobDrawer.svelte` — pass `locale()` to its three
      `timeAgo` calls.
- [x] 2.3 `lib/components/WebhookSettingsView.svelte` — pass `locale()` to its
      three `timeAgo` calls.
- [x] 2.4 `lib/components/ApiKeysView.svelte` — pass `locale()` to its three
      `timeAgo` calls.
- [x] 2.5 `lib/components/InboxView.svelte` — pass `locale()` to its two
      `timeAgo` calls.
- [x] 2.6 `lib/components/MySubmissionsView.svelte` — pass `locale()` to its
      `timeAgo` call.
- [x] 2.7 `lib/components/ReferralsView.svelte` — pass `locale()` to its two
      `timeAgo` calls.
- [x] 2.8 `lib/components/ContributeView.svelte` — pass `locale()` to its two
      `timeAgo` calls.

## 3. Shared header notification call site

- [x] 3.1 `lib/components/NotificationCard.svelte` — pass `locale()` to its
      `timeAgo` call. Confirm (per design.md) it renders on every route via
      `NotificationBell.svelte`/`TopBar.svelte`, so this one call site covers
      both `/my/**` and public routes with the same rule already applied
      elsewhere — no route-specific branching needed here.

## 4. Public and moderation call sites

- [x] 4.1 `lib/ghost.ts` — add a required `locale: Locale` parameter to
      `detailFor`, `ghostChecklist`, `ghostBadge`, `ghostGauge`,
      `ghostUnobserved` and `supersedesReality`. Only `detailFor`'s
      `ats_absent` branch actually formats locale-sensitive text via
      `timeAgo`; the rest thread `locale` through purely because they call
      into (or gate on) `ghostChecklist`/`ghostBadge` structurally —
      documented inline on `ghostBadge` rather than left unexplained.
      Updated callers: `GhostBadge.svelte`, `GhostChecklist.svelte` (direct
      calls), `JobRow.svelte`, `JobView.svelte` (`supersedesReality`), and
      `ghost.test.ts` (every call site, plus one new test asserting the
      cross-check date actually differs between `'en'` and `'ru'`).
- [x] 4.2 `lib/components/JobRow.svelte` — pass `locale()` to its `timeAgo`
      call and to `supersedesReality`.
- [x] 4.3 `lib/components/JobView.svelte` — pass `locale()` to its five
      `formatDateOrAgo`/`formatDateTime`/`formatDate` calls and to its two
      `supersedesReality` calls.
- [x] 4.4 `lib/components/RecentJobsFeed.svelte` — pass `locale()` to its
      `timeAgo` call.
- [x] 4.5 `lib/components/CompanyFeedbackListDialog.svelte` — pass `locale()`
      to its `formatDate` call.
- [x] 4.6 `lib/components/StatusBoard.svelte` — pass `locale()` to its
      `timeAgo` call.
- [x] 4.7 `lib/components/ModerationView.svelte` and its children
      (`ReportQueue.svelte`, `ReportedFeedbackQueue.svelte`,
      `ReferralReviewView.svelte`, `MentorReviewView.svelte`) — pass
      `locale()` to each `timeAgo`/`formatDate` call. These resolve to
      `'en'` today and after this change (per design.md, Risks); confirm the
      diff here is signature-only with no rendered-output change.

## 5. Wrap-up

- [x] 5.1 Run `pnpm --dir web check` (svelte-check/tsc — this is the
      completeness gate for every call site enumerated above) and fix
      anything it flags. (Worktree needed its own `design-system` install
      first — a separate pnpm package the worktree tooling does not install
      automatically, per the repo's own AGENTS.md.) **Code review caught 4
      call sites this task originally missed and mis-reported as
      pre-existing/unrelated**: `community/DiscussionThread.svelte:85`,
      `community/DiscussionFeed.svelte:120`, `community/ReplyNode.svelte:74`,
      `community/DiscussionIndex.svelte:67` — all four call `timeAgo` with
      only the timestamp, compiled fine before `locale` became required, and
      back live public discussion routes the original proposal never
      enumerated. Fixed with the same `locale()` pattern as every other
      call site. A follow-up repo-wide grep for every touched function name
      (`timeAgo`/`formatDate`/`formatDateTime`/`formatDateOrAgo`/
      `ghostBadge`/`ghostChecklist`/`ghostGauge`/`ghostUnobserved`/
      `supersedesReality`) found nothing else missed — the two remaining
      `formatDate(...)` hits (`routes/blog/+page.svelte`,
      `routes/blog/[slug]/+page.svelte`) are a locally-defined `formatDate`
      const unrelated to `$lib/utils.ts`'s export. **`pnpm --dir web check`
      now reports 0 errors** (39 pre-existing warnings, none in this diff's
      files).
- [x] 5.2 Run `pnpm --dir web lint` and `pnpm --dir web test`; fix anything
      this change introduced. (lint: exit 0, only pre-existing warnings
      outside this diff's files, including one at `ghost.ts:113` well
      outside every hunk this change touches. test: 162/162 files, 1889/1889
      tests passing, rerun clean after the 5.1 fix above.)
- [x] 5.3 Manual verification, partial: started the dev server against the
      already-running local stack (`hire-app-1`/`hire-db-1`/Meilisearch) and
      loaded `/status` (one of the components this change touches) — 200,
      no runtime error. `/jobs` and `/jobs/[slug]` 404'd against that stack's
      backend for reasons unrelated to this change (search/job-lookup itself
      returning 404, before any date formatting runs) and a signed-in
      `language = ru` account was not available in this session to click
      through `/my/webhook`/`/my/api-keys` live. Not claiming a full
      browser walkthrough — the locale-aware behavior itself is verified by
      `utils.test.ts`/`ghost.test.ts` against real (unmocked) `Intl` output
      for both `'en'` and `'ru'`, and every call site is type-checked via
      `pnpm --dir web check`.
- [ ] 5.4 Update freehire#2005: note the date gap is fixed, per the issue
      author's comment asking for it to be addressed first among the
      remaining items.
