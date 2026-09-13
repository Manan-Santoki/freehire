## Context

See proposal.md for the discovery and the motivation for a dedicated change rather than
folding this into the harvesting work that found it. `internal/ingest/atsboard/board.go`'s
`atsBoards` table already lists ~15 `subdomain`-mode platforms following the exact shape
Eightfold, Gupy, and Keka need (`<tenant>.<apex>` → board `<tenant>`, canonical collapses to
the bare tenant host).

## Goals / Non-Goals

**Goals:**
- Recognize the three platforms' standard multi-tenant hosts, matching how every other
  `subdomain`-mode entry already behaves.

**Non-Goals:**
- Paycom (see proposal.md — deliberately excluded, a separate decision).
- Any vanity/custom-domain instance of these platforms — none of the three offer one; every
  tenant observed in the source inventory (and in each adapter's own board-catalog rows) is on
  the vendor's own domain.

## Decisions

**Mode: `subdomain`, no new extraction mode.** All three platforms serve every tenant at
`<tenant>.<apex>` with no path segment carrying additional identity — the same shape
`recruitee.com`, `bamboohr.com`, and a dozen others already use. Adding a new mode for a shape
an existing one already covers would just be a second implementation of the same rule.

**Source keys**: `eightfold`, `gupy`, `keka` — verified against each adapter's own
`Provider()` (`internal/ingest/sources/{eightfold,gupy,keka}.go`), per `atsboard`'s own
load-bearing rule that the source MUST be the provider key the catalogue uses.

## Risks / Trade-offs

- **Widens what the paid contribution flow rewards.** This is the change's whole point, not a
  side effect — see proposal.md. The risk it displaces (a silent widening bundled into
  unrelated work) is exactly what `atsdetect`'s own guard test exists to catch, and this change
  is the deliberate response that guard asks for.
