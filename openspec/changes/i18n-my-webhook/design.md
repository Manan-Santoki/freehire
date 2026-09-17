## Context

The catalog mechanism and scope-boundary rule are already established (see
`i18n-profile-small-cards/design.md` for the fuller rationale). This change
is small and mechanical: one self-contained component, one catalog, one
route title.

## Goals / Non-Goals

**Goals:** every literal string `WebhookSettingsView.svelte` and its route's
`<title>` own renders in the resolved locale.

**Non-Goals:** translating `States.svelte` (shared with public routes,
renders no text in this view's own usage) or any other `/my/**` page.

## Decisions

### The route's `<title>` becomes a `headTitle` catalog key, not a separate mechanism

Matches the precedent `MySubmissionsView`'s migration set in the archived
`i18n-my-account-fanout` change: the page's `<svelte:head><title>` reads
from the same component catalog rather than staying hardcoded in the route
file, so the title and the body can never drift into different locales.

## Risks / Trade-offs

None beyond what `i18n-profile-small-cards` already documents for this
pattern.

## Migration Plan

No data migration. Frontend-only, one PR.
