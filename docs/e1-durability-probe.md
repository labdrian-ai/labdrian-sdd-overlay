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
| `gentle-ai upgrade` availability (2026-06-26 finding — SUPERSEDED, see 2026-08-11 correction below) | ~~**Not available** (no `upgrade` subcommand found)~~ |
| `gentle-ai upgrade` availability (2026-08-11 correction) | **Available** — `gentle-ai upgrade --help` prints an `Upgrade` help section describing it as upgrading managed tool binaries; installed `gentle-ai 2.3.0`. Supersedes the 2026-06-26 "not available" finding above. |
| `gentle-ai upgrade` durability probe | **Not executed** (deliberate). `gentle-ai upgrade` mutates installed tool binaries; running it purely to close this evidence gap was judged unsafe and was not attempted. Only the `sync` half of this probe (rows above) was executed and passed. |

## 2026-08-11 Correction — `gentle-ai upgrade` availability

The 2026-06-26 finding that `gentle-ai upgrade` was "not available" is stale. Verified
directly on 2026-08-11: `gentle-ai upgrade --help` now prints an `Upgrade` help section
and describes the command as upgrading managed tool binaries. Installed version at time
of verification: `gentle-ai 2.3.0`.

The durability probe against `upgrade` itself was **not executed**, deliberately.
`gentle-ai upgrade` mutates installed tool binaries on this machine — an unattended run
already broke this repository's review authority store earlier in this same working
session, costing hours of recovery. Running it again solely to close this evidence gap
was judged an unacceptable risk. Only the `sync` half of this probe was executed (see
table above: `gentle-ai sync` ran, 103 files synced, 0 under `~/.claude/agents/`,
sentinel and `GADU.md` both survived unmodified). The classification and D4 guard
selection below remain based on that `sync` evidence only; they do not rest on any
observation of `gentle-ai upgrade` behavior.

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
- ~~AC-5 (simulated sync → reapply → both artifacts present + managed) is still
  the regression test for the agent route, exercised in `engine/installer/route_test.go`.~~
  (SUPERSEDED, see 2026-08-11 correction below — that sentence named a file, not a
  test, and no test performing a reapply cycle existed at the time it was written.)

## 2026-08-11 Correction — AC-5 regression test now exists

The sentence above named `engine/installer/route_test.go` as a whole but pointed to no
specific test; a direct check (`rg 'AC-5|reapply|simulated sync' engine/`) found zero
matches, confirming no test in that file exercised a re-application cycle. AC-5 was an
untested claim.

This is now closed: `TestSyncCheck_DetectsMissingAgentFile` in
`engine/installer/route_test.go` was extended to add the restoration half. After its
existing removal step (which already asserted `OVERLAY_NOT_DEPLOYED`), the test now
reapplies the overlay, asserts the agent file is present again at
`~/.claude/agents/GADU.md`, and asserts `cmd_sync_check` reports `IN_SYNC` again. Run
with `go test -count=1 -v -run TestSyncCheck_DetectsMissingAgentFile ./installer/...`
from `engine/`: **PASS**, first run, no RED phase — this is a characterization test of
already-existing `cmd_apply` behavior (unconditional copy-if-absent-or-differs; see
`bin/labdrian-overlay` cmd_apply's deploy loop), not a red-green TDD cycle.
AC-5 (simulated sync → reapply → both artifacts present + managed) is now the
regression test named above, exercised by `TestSyncCheck_DetectsMissingAgentFile` in
`engine/installer/route_test.go`.
