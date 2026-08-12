# Archive Report: Runtime-wire the anti-generic-design contract

**Change**: anti-generic-design-runtime-wiring | **Project**: labdrian-sdd-overlay | **Archived**: 2026-08-11 | **Artifact store**: hybrid (OpenSpec + Engram)

## Executive Summary

Change is complete, verified, and archived. `anti-generic-design` is now wired as a third engine-owned embedded contract (marker pair, `embeddedContract()` case, embedded asset, Merger hook pair, `checkRegistry`/lifecycle-state recognition), mirroring the `skill-discovery-safety` mechanism. All 6 requirements (R-101..R-106) and 11/11 scenarios are compliant; verdict PASS WITH WARNINGS, 0 CRITICAL. All 28/28 tasks are complete, including Phase 6's live rebuild/install/verify actions.

This archive is **retroactive**: the implementation merged to `main` on 2026-07-08 via PR #87 and PR #88 (merge commit `98abdf4`), roughly 34 days before this archive ran. The archive step itself was never executed at the time — the change was found stranded by a guard, not surfaced by a human, and is being closed now on branch `chore/archive-stranded-changes`.

## Specs Synced

| Domain | Action | Details |
|--------|--------|---------|
| `anti-generic-design` | Merged (append) | Appended 6 added requirements (R-101..R-106, 11 scenarios) into the existing `## ADDED Requirements` section of `openspec/specs/anti-generic-design/spec.md` |

**Critical pre-condition handled**: `openspec/specs/anti-generic-design/spec.md` was already occupied by a **different, previously archived change** (`2026-07-08-anti-generic-ai-design-skill`, title `# Spec: anti-generic-ai-design-skill`), carrying requirements R-002, R-006, R-007, R-008, R-009 plus 3 unnumbered requirements. `rg 'R-10[1-6]' openspec/specs/` returned no matches before this archive ran — R-101..R-106 were unpromoted precisely because archive never ran for this change. The merge was performed as a pure **append** into the existing `## ADDED Requirements` section, preserving the original title and every byte of the five pre-existing numbered requirements (R-002, R-006, R-007, R-008, R-009) and the three unnumbered ones, plus the trailing `## OUT OF SCOPE` section, all byte-identical before and after. Verified by full re-read of the file post-edit: R-002 (line 22), R-006 (line 41), R-007 (line 57), R-008 (line 68), R-009 (line 79) are present unchanged, alongside R-101 (line 143), R-102 (line 168), R-103 (line 193), R-104 (line 207), R-105 (line 229), R-106 (line 256).

## Task Completion Gate

`openspec/changes/anti-generic-design-runtime-wiring/tasks.md` (on-disk, authoritative) shows **28/28** implementation tasks checked `[x]` across Phase 1 (5/5), Phase 2 (6/6), Phase 3 (4/4), Phase 4 (5/5), Phase 5 (5/5), Phase 6 (3/3) — including Phase 6 (6.1 rebuild+install-hooks, 6.2 propagate against the live registry, 6.3 status/check + full test suite). No stale unchecked checkboxes; no exceptional reconciliation was required.

## Final-State Authority — reconciling stale Engram snapshots

Per the Final-State Authority hierarchy, the on-disk artifacts and the orchestrator's explicit final-state facts outrank the intermediate Engram snapshots below, which were never refreshed after Phase 6 executed:

- **Engram `#2000` (tasks)** — last written 2026-07-08 16:54. Frames Phase 6 as **"NOT STARTED (PR-4, the ONLY remaining phase)"** with all 3 Phase-6 tasks `[ ]`, and states a 29-task total. **This is stale.** The on-disk `tasks.md` is authoritative: Phase 6 is executed and complete, 28/28 tasks (not 29).
- **Engram `#2003` (apply-progress)** — last written 2026-07-08 17:01, upserted mid-PR-4 before the live actions ran. States **"Phase 6 (6.1-6.3): 0/3 marked [x] — BLOCKED pending explicit orchestrator/user go-ahead."** **This is stale.** A later revision of the same on-disk `apply-progress.md` (not separately persisted to Engram due to the topic-key upsert, see gap below) records that go-ahead was given and Phase 6 executed for real: `install-hooks` rebuilt `~/.claude/bin/gentle-ai-overlay` and installed the third hook pair into `~/.claude/settings.json` (backup confirmed), `propagate --embedded-contract anti-generic-design` inserted the `anti-generic-design-scope` block into the live `.atl/skill-registry.md`, and `status-hooks` reported all `[OK]`. This was independently re-verified read-only against live state during `sdd-verify` (see below) — not merely re-asserted from the stale prose.
- **`verify-report.md` (Engram `#2005`, on-disk, most recent authoritative artifact before this archive)** — re-verified all three Phase 6 claims read-only against current live state at verification time: hook pair present in `~/.claude/settings.json` matching R-105's shape, `anti-generic-design-scope` block present in `.atl/skill-registry.md` coexisting with the other two blocks, and `bin/labdrian-overlay status-hooks` returning all `[OK]`.

**Documented, unrecoverable gap (not reconstructed)**: No `apply-progress` artifact survives in Engram for PR-1, PR-2, or PR-3 — only PR-4's remains, because the `sdd/anti-generic-design-runtime-wiring/apply-progress` topic key upserts (each save replaces the prior content under the same key). Per-PR TDD cycle evidence for PR-1..PR-3 survives only as inline annotations inside `tasks.md` itself, which was independently corroborated during verification (named test functions located on disk, RED/GREEN evidence concrete). This gap is recorded here honestly, as instructed, and is not reconstructed.

## Review Gate

No native review transaction, frozen ledger, terminal receipt, or `reviews/` artifact exists for this change (no `reviews/receipt.json` under the change folder, no review topic in Engram). This is consistent with the change's actual delivery path: it merged via PRs #87/#88 on 2026-07-08, before this repository's receipt-driven review gate was consistently enforced on every change, and archive is running retroactively and out-of-band via an explicit guard-driven cleanup pass (`chore/archive-stranded-changes`), not through the normal apply→verify→archive pipeline. There is no CRITICAL verification finding and no pending/invalidated/escalated review state to block on — there is simply no review artifact for this change to validate against. This absence is recorded as a limitation, not silently treated as `allow`.

## Engram Traceability

| Artifact | Topic | Observation ID |
|---|---|---:|
| Proposal | `sdd/anti-generic-design-runtime-wiring/proposal` | 1996 |
| Spec (delta) | `sdd/anti-generic-design-runtime-wiring/spec` | 1997 |
| Design | `sdd/anti-generic-design-runtime-wiring/design` | 1998 |
| Tasks | `sdd/anti-generic-design-runtime-wiring/tasks` | 2000 (stale re: Phase 6 — see Final-State Authority above) |
| Apply progress | `sdd/anti-generic-design-runtime-wiring/apply-progress` | 2003 (PR-4 only, stale re: Phase 6 go-ahead — see Final-State Authority above; no surviving record for PR-1/2/3) |
| Verification report | `sdd/anti-generic-design-runtime-wiring/verify-report` | 2005 |

## Verification Summary

**Verdict**: PASS WITH WARNINGS (per verify-report obs #2005, evidence_revision: `sha256:42781317dc065e734a2f67a22e4e8c7eeb6b487389d35cc741a77a9a6b5735f3`)

| Metric | Result |
|--------|--------|
| Blockers | 0 |
| Critical findings | 0 |
| Requirements | 6/6 compliant |
| Scenarios | 11/11 passing |
| Tasks checked | 28/28 |
| Build | PASS — `go build ./... && go vet ./...` clean in `engine/` and `tui/` |
| Tests | PASS — 11 packages, 0 failures, 0 skips, run with `-count=1` |

### Warnings carried from verify-report (informational, non-blocking, addressed or recorded here)

- **W1** — canonical spec path collision with a sibling change. **Resolved by this archive** via append-only merge (see Specs Synced above).
- **W2** — Engram tasks/apply-progress staleness relative to on-disk state. **Recorded** in Final-State Authority above; on-disk files treated as authoritative per the phase gate hierarchy.
- **W3** — no surviving per-PR apply-progress for PR-1/2/3 (topic-key upsert overwrote them). **Recorded honestly above; not reconstructed.**

### Suggestions carried from verify-report (non-blocking, for future work, not re-litigated here)

S1 (hardcoded absent-path test fixture), S2 (generic vs. contract-specific negative-scenario fixture), S3 (cosmetic open-question checkbox in design.md), S4 (cosmetic Success Criteria checkboxes in proposal.md), S5 (R-106 scenario 1 coverage inherited from a later change, not change-owned).

## Archive Contents

- `proposal.md`
- `design.md`
- `tasks.md`
- `apply-progress.md`
- `verify-report.md`
- `specs/anti-generic-design/spec.md`
- `archive-report.md` (this file)

## Archive Verification

- Main specification updated before archival: confirmed — R-101..R-106 appended into `openspec/specs/anti-generic-design/spec.md`, all 5 pre-existing numbered requirements (R-002, R-006, R-007, R-008, R-009) and 3 unnumbered ones preserved byte-for-byte, original title `# Spec: anti-generic-ai-design-skill` unchanged.
- Archived artifacts include proposal, design, tasks, apply-progress, verification report, delta spec, and this archive report: confirmed.
- Archived `tasks.md` has 28/28 checked implementation tasks, no unchecked items, no reconciliation needed: confirmed.
- **Change folder physical move**: **NOT completed by this execution.** This executor's toolset for this run did not include a shell/Bash/file-delete capability, so `git mv openspec/changes/anti-generic-design-runtime-wiring openspec/changes/archive/2026-07-08-anti-generic-design-runtime-wiring` could not be run. Instead, full copies of all six source artifacts plus this new archive report were written directly into the archive destination via the Write tool. **The original folder `openspec/changes/anti-generic-design-runtime-wiring/` still exists on disk and was not removed.** A follow-up mechanical step — `git mv` (to preserve rename history) or an equivalent delete of the original folder now that its content is duplicated at the archive destination — is required from an actor with shell access to complete the move cleanly and avoid the change appearing in both the active and archived directories simultaneously. No content was lost: the archive destination is a complete, verified copy.

## Notes

- No CRITICAL verification issues were present at any point.
- No commit, stage, or push was performed by this executor; all writes are unstaged changes in the working tree, per explicit instruction.
- `openspec/changes/gadu-portable-operator/` was not touched, per explicit instruction.
- No existing folder under `openspec/changes/archive/` was modified; only this new folder was added.

---

**Archive report persisted**: OpenSpec (this file) + Engram topic `sdd/anti-generic-design-runtime-wiring/archive-report`

**Next recommended**: A follow-up action (by an actor with shell/git access) to `git mv` or delete the original `openspec/changes/anti-generic-design-runtime-wiring/` folder now that its content is duplicated at the archive destination. Otherwise: none — the change itself is complete and closed.

**Risks**: (1) Source folder not removed — see "Change folder physical move" above. (2) No native review receipt exists for this retroactive change — recorded as a limitation, not blocking, given 0 CRITICAL findings and explicit final-state facts. (3) Engram `#2000`/`#2003` remain stale in place (not overwritten) — future readers of those specific observation IDs must be pointed to this archive report or the on-disk files for the true final state.
