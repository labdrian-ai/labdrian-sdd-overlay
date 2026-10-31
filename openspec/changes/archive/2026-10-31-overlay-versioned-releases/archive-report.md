# Archive Report: overlay-versioned-releases

- **Change**: `overlay-versioned-releases`
- **Archived**: 2026-10-31
- **Archive status**: archived by Phase 9 item C11 (odd/tasks/phase9-c11-openspec-archive.md, T1), moved as-is.
- **Verification status**: verify-report.md verdict fail (31/35 scenarios compliant, 3 UNTESTED, 1 PARTIAL, 0 FAILING), unchanged.

## Cycle Timestamps

| Phase | Timestamp | Notes |
|-------|-----------|-------|
| t1 (landing commit) | n/a | landing_commit is absent: the work of this change reached main through its own earlier pull requests, and this archive moves only its record, so it records no landing commit. |

## Canonical Spec Composition

None. C11 archives this change as-is (owner decision Q9, odd/tasks/consolidation-phase9.md:24): its seven delta specs (overlay-backup-restore, overlay-release-identity, overlay-release-state, overlay-release-surfacing and overlay-update-check, which have no canonical spec, the ADDED requirements of sync-check-verdicts and the MODIFIED requirements of tui-self-update) were not composed into openspec/specs/.

## Archive Move

`openspec/changes/overlay-versioned-releases/` → `openspec/changes/archive/2026-10-31-overlay-versioned-releases/`, by `git mv` in commit 735992bf; no file of the folder was edited.
