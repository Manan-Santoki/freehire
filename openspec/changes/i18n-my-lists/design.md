## Context

The catalog mechanism, the `formErrorKind`-style reactive-error pattern,
the `format()` placeholder helper, and `plural()`/`plurals()` are all
already established (`i18n-profile-small-cards`, `i18n-my-webhook`,
`$lib/i18n/t.ts`). This change applies them to one page with more moving
parts than the webhook page: seven distinct error fallbacks instead of
four, two `window.prompt()` calls, and interpolated `aria-label`/`title`
attributes.

## Goals / Non-Goals

**Goals:** every literal string `JobListsView.svelte` and its route's
`<title>` own renders in the resolved locale, including prompts and
interpolated attributes; the job-count line uses a real plural rule.

**Non-Goals:** `States.svelte` (public-reachable, unchanged); any other
`/my/**` page.

## Decisions

### Seven error fallbacks become one `errorKind` state, keyed per action

Alternative considered: seven separate boolean/string flags (one per
action), mirroring `LocationCard`'s single-flag approach. Rejected as
needless duplication — a single `errorKind: 'create' | 'rename' |
'description' | 'share' | 'unshare' | 'copyLink' | 'delete' | null` plus a
`errorMessage: string | null` (the server's own message, when the action
provides one) is one shape that covers every action:
`const error = $derived(errorKind ? (errorMessage ?? s.errors[errorKind]) :
null);`. `s.errors` is a nested catalog section — one key per action —
rather than seven top-level keys, matching the "nested per section"
convention `$lib/i18n/t.ts`'s own `Messages` type doc comment describes.

Three actions (`unshare`, `copyLink`, `remove`) never surface an
`ApiError.message` today (their `catch` blocks take no error parameter) —
their `errorMessage` is always cleared to `null` when they set
`errorKind`, so the derived `error` always reads the catalog fallback for
them, preserving current behavior exactly.

### The original per-action `error = null` reset is preserved exactly, not made uniform

Every action but `copyLink` resets the error at the start (`confirmCreate`,
`rename`, `editDescription`, `share`, `unshare`, `remove` all do;
`copyLink` does not). That one asymmetry is pre-existing and unrelated to
i18n — reproducing it exactly (resetting `errorKind`/`errorMessage` at the
same points the original reset `error`, and nowhere else) keeps this
change surgical. Fixing the asymmetry, if it is worth fixing, is a
separate change.

### `window.prompt()` messages are catalog keys like any other string

`window.prompt('Rename job list', l.name)` and `window.prompt('Edit
description', l.description)` both show their first argument as visible
text. There is no mechanism-level reason to treat a native prompt's message
differently from a `<label>`'s text — both are catalog keys read through
`s`.

### Interpolated `aria-label`/`title`/dialog-title strings use `format()`

`aria-label="Rename “{l.name}”"` and its three siblings, plus the delete
dialog's `` `Delete job list "${name}"?` ``, all place a value in the
middle of a sentence — exactly what `format()` (added in
`i18n-profile-small-cards` for `SkillsCard`'s save-failure message) exists
for, rather than reintroducing a prefix/suffix split.

## Risks / Trade-offs

None beyond what the two earlier fan-out changes already document for this
pattern.

## Migration Plan

No data migration. Frontend-only, one PR.
