# Apply Progress: Pi Package Hardening

## Slice 1: pipkg-integrity (PR 1, R-001..R-004)

**Change**: pi-package-hardening
**Mode**: Strict TDD (RED → GREEN → REFACTOR)
**Branch**: `feat/pi-package-hardening` (stacked-to-main)

### Completed Tasks

- [x] 1.1 RED: `engine/pipkg/pipkg_test.go` — `TestPipkgCheck_ModeDrift` (0644→0755 reported), `TestPipkgCheck_SymlinkedDestRootRefused` (approval test — already correct via `listFiles`' `WalkDir` Lstat-based root entry).
- [x] 1.2 RED: `engine/skills/parse_test.go` — `path_traversal_rejected` table (`../../outside`, `/etc/passwd`, `foo/../bar`, empty) + `path_in_root_passes`.
- [x] 1.3 RED: `engine/pipkg/pipkg_test.go` — `TestPipkgBuild_RejectsNameMismatch`, `TestPipkgBuild_LiveRegistryNamesMatch` (approval test against the repo's real `skills.registry.yaml`).
- [x] 1.4 GREEN: `engine/pipkg/pipkg.go` — `listFiles` now returns `map[string]fileEntry{data,perm}`; `Check` diffs `perm` in addition to `data` and reports `"<rel>: mode 0NNN -> 0NNN"`; `Build` chmods the build root (`tmpDir`) to 0755 right after `MkdirTemp` (which defaults to 0700).
- [x] 1.5 GREEN: `engine/skills/parse.go` `validateEntry` — rejects empty, absolute, unclean, or `..`-bearing `path`.
- [x] 1.6 GREEN: `engine/pipkg/pipkg.go` `buildInto` — re-checks `filepath.Rel(skillsDir,dst)` has no `..` prefix (defense in depth) and calls the new `checkSkillNameMatchesPath` (line-oriented frontmatter scan, no YAML dependency) before `copyTree`.
- [x] 1.7 Docs: `engine/pipkg/pipkg.go` package doc comment documents the four integrity guarantees (mode drift, build-root perms, path containment, name/dir match).

### Files Changed

| File | Action | What Was Done |
|------|--------|----------------|
| `engine/pipkg/pipkg.go` | Modified | `listFiles`→`fileEntry{data,perm}`; `Check` mode-diff; `Chmod(tmpDir,0755)`; `buildInto` dest-containment re-check + `checkSkillNameMatchesPath`/`frontmatterName`; package doc comment. |
| `engine/pipkg/pipkg_test.go` | Modified | Added `TestPipkgCheck_ModeDrift`, `TestPipkgCheck_SymlinkedDestRootRefused`, `TestPipkgBuild_RootPermissions`, `TestPipkgBuild_RejectsNameMismatch`, `TestPipkgBuild_LiveRegistryNamesMatch`. |
| `engine/skills/parse.go` | Modified | `validateEntry` gains the R-003 path-containment checks; added `path/filepath` import. |
| `engine/skills/parse_test.go` | Modified | Added `path_traversal_rejected` table + `path_in_root_passes` subtests, plus `escapeYAMLPath` helper. |
| `openspec/changes/pi-package-hardening/tasks.md` | Modified | Phase 1 tasks 1.1–1.7 marked `[x]`. |

### TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 1.1/1.4 | `engine/pipkg/pipkg_test.go` | Unit | ✅ 14/14 (pre-existing pipkg suite) | ✅ Written — confirmed failing (`want error, got nil` for mode drift and root perms) | ✅ Passed | ✅ mode-drift + symlink-refusal + root-perms (3 cases) | ➖ None needed |
| 1.2/1.5 | `engine/skills/parse_test.go` | Unit | ✅ full `skills` package suite green before edit | ✅ Written — confirmed failing (4/4 traversal subtests) | ✅ Passed | ✅ 4 traversal cases + 1 passing case | ➖ None needed |
| 1.3/1.6 | `engine/pipkg/pipkg_test.go` | Unit | ✅ (same run as 1.1) | ✅ Written — confirmed failing (`want error, got nil`) | ✅ Passed | ✅ mismatch case + live-registry approval case | ➖ None needed |

### Test Summary

- **Total tests written**: 8 new test functions (2 pipkg approval tests already passed pre-GREEN; 3 pipkg genuinely RED→GREEN; 2 parse.go subtests tables covering 5 cases).
- **Total tests passing**: full `engine/skills` and `engine/pipkg` packages green; full repo `go test -race ./...` green.
- **Layers used**: Unit only (pure filesystem/parse, no runtime harness applicable to this slice).
- **Approval tests**: `TestPipkgCheck_SymlinkedDestRootRefused`, `TestPipkgBuild_LiveRegistryNamesMatch` — pin already-correct or newly-correct behavior against the real registry.
- **Pure functions created**: `frontmatterName` (line-oriented `name:` scan, no I/O).

### Work Unit Evidence

| Evidence | Value |
|---|---|
| Focused test command and exact result | `cd engine && go test ./pipkg/... ./skills/...` → `ok  .../pipkg  0.072s`, `ok  .../skills  0.113s`, all tests pass, 0 failures. |
| Runtime harness command/scenario and exact result | N/A — pure filesystem/parse unit tests, no runtime boundary (per tasks.md Suggested Work Units row 1). Supplemental scratch smoke (outside repo, under `/tmp/claude-1000/.../scratchpad/smoke`, deleted after use): built a fixture package via the `pipkg` CLI subcommand, confirmed `stat -c %a dest` = `755`; `chmod 777` a built file → `pipkg check` exit 1 naming `skills/demo-skill/SKILL.md: mode 0644 -> 0777`; symlinked file inside the built package → `pipkg check` exit 1 naming `refusing symlink at .../link.json`. |
| Rollback boundary | Revert `engine/pipkg/pipkg.go`, `engine/pipkg/pipkg_test.go`, `engine/skills/parse.go`, `engine/skills/parse_test.go` — no other files touched. Each is independently revertible; no cross-slice coupling. |

### Broad Verification

| Command | Result |
|---|---|
| `cd engine && gofmt -l .` | empty (clean) |
| `cd engine && go vet ./...` | no output (clean) |
| `cd engine && go test -count=1 -race ./...` | all 13 packages `ok` (assets, cmd, gadu, gate, installer, pipkg, prespec, propagator, runtime, settings, shelltest, skills, synctrigger) |
| `shellcheck -S warning bin/labdrian-overlay` | 2 findings, both pre-existing `SC2064` at lines 1451 and 1608 (unrelated to this slice; no new findings) |
| `git diff --shortstat 98f8410..HEAD -- engine bin` (working tree, pre-commit) | `4 files changed, 322 insertions(+), 13 deletions(-)` — well under the 400-line budget |

### Deviations from Design

None — implementation matches design decisions D1–D4 exactly:
- D1: mode diff via `listFiles`'s new `fileEntry{data,perm}`; `copyFile` unchanged (still writes 0644).
- D2: `os.Chmod(tmpDir, 0755)` right after `os.MkdirTemp`.
- D3: containment enforced in `validateEntry` (shared boundary for both source and destination joins) plus a `buildInto`-side `filepath.Rel` defense-in-depth re-check.
- D4: SKILL.md `name:` scanned via a minimal line-oriented parser living in `engine/pipkg` (not `engine/skills`), keeping `validateEntry` filesystem-free per the zero-dependency ADR.

### Issues Found

None.

### Remaining Tasks (out of this slice's scope)

- [ ] Phase 2: sync-check-provenance (PR 2, R-005..R-007)
- [ ] Phase 3: review-receipt-capture (PR 3, R-008)
- [ ] Phase 4: receipt-anchor-gate (PR 4, R-009..R-011)
- [ ] Phase 5: gadu-pi-subagent (PR 5, R-012..R-016)
- [ ] Phase M: Manual live-Pi checkpoints (not run by `sdd-apply`)

### Workload / PR Boundary

- Mode: stacked-to-main PR slice (PR 1 of 5)
- Current work unit: pipkg-integrity (R-001..R-004)
- Boundary: starts from `98f8410` (docs: proposal/spec/design/tasks/entry contract), ends with Phase 1 GREEN + docs, all tests passing, tasks 1.1–1.7 marked `[x]`.
- Estimated review budget impact: 322 authored lines (well under 400); no further slicing needed for this unit.
- Slices planned=5 realized=1 (within tolerance).

### Status

7/7 Phase 1 tasks complete. Ready for the next SDD phase (verify or the next apply batch for Phase 2).
