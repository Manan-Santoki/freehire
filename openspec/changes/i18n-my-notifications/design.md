## Context

The catalog mechanism is already established (four earlier PRs in this
series). This page is unusually small: no dedicated view component, just a
route file with three literal strings.

## Goals / Non-Goals

**Goals:** the three literal strings this route owns render in the
resolved locale.

**Non-Goals:** `NotificationCard.svelte` (shared with the header bell,
already out of scope per `i18n-my-webhook`/`i18n-my-lists` precedent for
shared components) and `$lib/ui`'s `LoadMore`/`States`.

## Decisions

### The catalog is a plain `messages.ts` beside the route, not a `$lib/components/*.messages.ts`

There is no `$lib/components` view for this page to colocate a catalog
with — the route file is the whole UI. `routes/my/profile/messages.ts` and
`routes/my/security/messages.ts` (from the archived `i18n-my-account`
change) already establish this exact pattern for a route with no separate
component; this change follows it rather than inventing a new
`NotificationsPage.svelte` wrapper purely to have somewhere to put a
`.messages.ts` file next to.

## Risks / Trade-offs

None beyond what the four earlier fan-out changes already document.

## Migration Plan

No data migration. Frontend-only, one PR.
