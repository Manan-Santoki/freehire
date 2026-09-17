## Why

The Work History view (`/my/profile/experience`) renders every achievement bullet for
every employment inline, all the time. On an account with even a modest history this is a
wall of bordered boxes, hand-rolled tag pills that bypass the design system, and a dense
icon cluster on every row — hard to scan, and the "N not confirmed" banner is plain text
with no way to jump to what actually needs a decision.

## What Changes

- Each employment's achievement list collapses to a summary count ("5 achievements") by
  default; an in-place expand (accordion, no route change) reveals the full list. Achievement
  selection for Merge / "Tailor with assistant" keeps working across employments regardless
  of which are expanded or collapsed — selection state is not scoped to the accordion.
- The "N not confirmed" banner becomes a link: activating it expands the employment (or the
  unplaced section) holding the first unconfirmed achievement and scrolls to/highlights it.
- Card and row borders are removed; grouping comes from spacing and background instead.
- Hand-rolled skill/stack tag markup (`bg-brand-muted rounded-full` spans) is replaced with
  the design system's `Chip` component.
- The add/edit forms for an employment and an achievement move from raw `<input>`/`<textarea>`
  markup to the design system's `FormField`/`Input`.
- Visual hierarchy pass: clearer title/company emphasis, more breathing room, quieter
  secondary text — no change to what information is shown, only how it reads.

No API or data-model changes. `freehire-cli` and the in-app assistant talk to the same
`GET/POST/PUT /me/experience/*` endpoints regardless of how the web view renders them, so
neither needs a change for this proposal; revisit only if implementation surfaces an actual
gap.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `experience-bank`: adds requirements for how an employment's achievements are disclosed
  (collapsed-by-default with an in-place expand) and for the unconfirmed-achievements banner
  linking to the specific achievement it is about.

## Impact

- `web/src/lib/components/ExperienceBankView.svelte` — split into smaller, focused pieces
  (employment card, achievement row, forms) as part of the rework.
- Design-system consumption only (`Chip`, `FormField`, `Input`) — no changes to
  `design-system/` itself expected.
- No backend/API changes, no changes to `freehire-cli` or `internal/ai/assistant`.
