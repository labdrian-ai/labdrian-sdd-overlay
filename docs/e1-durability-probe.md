# E1 Durability Probe — Result and Classification

> **Date**: 2026-06-26
> **Phase**: A0 prerequisite — must precede any D4 guard code (R-013, D4)

## Classification

**append-only / replace-catalog-only**

`gentle-ai sync` is APPEND-ONLY: it never touches `~/.claude/agents/`. Zero files
under `~/.claude/agents/` were modified, renamed, or deleted by the sync operation.

## Evidence

| Observation | Value |
|---|---|
| Sentinel planted | `~/.claude/agents/_overlay-durability-probe.md` |
| Sentinel sha256 (before sync) | `(planted with known content)` |
| GADU.md sha256 (before sync) | `4b654a5a6ea6...` (mtime 2026-06-23) |
| `gentle-ai sync` total files synced | 103 |
| Files synced under `~/.claude/agents/` | **0** |
| Sentinel present after sync | **Yes — unmodified** |
| GADU.md present after sync | **Yes — identical sha256** |
| `gentle-ai upgrade` availability | **Not available** (no `upgrade` subcommand found) |

## Probe method

1. Recorded sha256 + mtime of `~/.claude/agents/GADU.md`.
2. Planted `~/.claude/agents/_overlay-durability-probe.md` with a known sentinel.
3. Ran `gentle-ai sync` (103 files synced to `~/.claude/` targets).
4. Re-inspected `~/.claude/agents/`: sentinel survived with identical sha256 + mtime;
   `GADU.md` survived with identical sha256; no files in `~/.claude/agents/` were
   touched by the sync operation.

## D4 Guard Branch Selected

Per design D4, the E1 outcome selects the **MINIMAL** guard branch:

> **append-only / replace-catalog-only → MINIMAL guard**: the existing apply
> mechanism extended to the agent route. `overlay apply` re-deploys `agents/GADU.md`
> after any drift. `overlay sync-check` + `overlay apply` are the durability path.
> NO pre-sync backup needed. NO new destructive-protection code.

Durability is delivered by **reproducibility + re-apply**: GADU is generated from
`engine/gadu/persona/body.md`, committed in the overlay, and re-deployable by
`overlay apply` at any time. Because `gentle-ai sync` does not touch
`~/.claude/agents/`, re-apply after sync is a maintenance step only, not an
emergency recovery path.

## Implications for Slice B

- No pre-sync backup function is needed.
- No filesystem immutability or protection hooks are needed.
- `bin/labdrian-overlay apply` (with the agent route from Slice B) is the
  complete durability mechanism.
- AC-5 (simulated sync → reapply → both artifacts present + managed) is still
  the regression test for the agent route, exercised in `engine/installer/route_test.go`.
