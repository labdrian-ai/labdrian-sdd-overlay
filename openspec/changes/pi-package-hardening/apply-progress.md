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

## Slice 2: sync-check-provenance (PR 2, R-005..R-007)

**Change**: pi-package-hardening
**Mode**: Strict TDD (RED → GREEN → REFACTOR)
**Branch**: `feat/pi-package-hardening-2-provenance` (stacked on `feat/pi-package-hardening` @ `a1f2bc0`)
**Delivery status**: implemented and fully verified, **NOT committed** — exceeds the 400-authored-line budget (see Workload / PR Boundary below). Working tree left uncommitted and unstaged so the orchestrator can decide `size:exception` vs. a further split before this lands.

### Implemented (uncommitted) — tasks 2.1–2.8

All Phase 2 RED/GREEN/Docs work is implemented and green, but **tasks.md checkboxes are intentionally left `[ ]`** because nothing was committed this batch (see Delivery status above). Do not treat this as "not started" — the code, on disk in this worktree, is complete and passing:

- 2.1 RED: `engine/pipkg/pipkg_provenance_test.go` `TestBuiltFrom_RecordedAtBuild` — `labdrian.builtFrom` equals `git rev-parse HEAD` in a `gitFixtureOverlay` (git-in-`t.TempDir()`, skipped under `-short`).
- 2.2 RED: `TestCheck_RefBasis` — resolvable builtFrom SHA → `Basis="ref"`; an uncommitted working-tree edit does NOT cause drift; a genuinely tampered deployed file still reports drift naming the entry.
- 2.3 RED: `TestCheck_MainFallback` — unresolvable-but-well-formed SHA → `Basis="main"`, `Ref` carries the disclosed unresolvable value; `TestCheck_NonGitRoot` — non-git `overlayRoot` → `Basis="worktree"`.
- 2.4 RED: `TestCheck_RejectsNonHexBuiltFrom` — `--upload-pack=evil`, `not-hex-at-all`, and `""` all fall back to `main` cleanly; `builtFromPattern` (`^[0-9a-f]{40}$`) is checked BEFORE any of these values ever reaches a `git` argv.
- 2.5 GREEN: `engine/pipkg/pipkg.go` — `packageManifest.Labdrian *labdrianField{BuiltFrom}`; `resolveBuildRev(overlayRoot)` (`git rev-parse HEAD`, `""` on any failure); `resolvePackageVersion(overlayRoot, rev)` gained the `rev` argument (D5 "single source").
- 2.6 GREEN: `Check` now returns `(CheckReport{Basis, Ref}, error)`. `resolveComparisonSource` picks the basis (worktree / ref / main), `exportGitTree` + `extractTar` (`archive/tar`) export `skills/ agents/ skills.registry.yaml` at the resolved rev via `git archive --format=tar`, and `buildInto` gained `(provenanceRoot, rev string)` parameters so the comparison build's `package.json` version/builtFrom are resolved against the ORIGINAL `overlayRoot` (which has full tag history) even when the file-source root is a throwaway export (which has no `.git`). `stripBuiltFrom` normalizes `package.json`'s `labdrian.builtFrom` (and canonicalizes JSON formatting) out of the content diff so a legitimately different recorded ref never reads as drift by itself.
- 2.7 GREEN: `bin/labdrian-overlay` `pipkg_sync_check_and_report` now echoes `$drift_output` (which always carries the engine's `pipkg check: compared against ...` basis line) in the no-drift branch too, not just the drift branch — `SYNC_CHECK:pi:`/`VERDICT:pi:` lines unchanged.
- 2.8 Docs: doc comment added above `cmd_sync_check` describing the three basis-disclosure forms; `CheckReport.Disclosure()` is the single rendering function all four surfaces (`pipkg check` CLI, `PiAdapter.SyncCheck`/`Status`, the bash helper) call.

### Files Changed (uncommitted)

| File | Action | What Was Done |
|------|--------|----------------|
| `engine/pipkg/pipkg.go` | Modified | `CheckReport`/`Disclosure`; `labdrianField`; `resolveBuildRev`; `resolvePackageVersion(root, rev)`; `Check` signature → `(CheckReport, error)`; `resolveComparisonSource`, `exportGitTree`, `extractTar`, `readBuiltFrom`, `stripBuiltFrom`; `buildInto(overlayRoot, reg, dir, provenanceRoot, rev)`; package doc comment extended. |
| `engine/pipkg/pipkg_provenance_test.go` | Created | `gitFixtureOverlay`/`runGit`/`corruptBuiltFrom` helpers; `TestBuiltFrom_RecordedAtBuild`, `TestCheck_RefBasis`, `TestCheck_MainFallback`, `TestCheck_NonGitRoot`, `TestCheck_RejectsNonHexBuiltFrom`. |
| `engine/pipkg/pipkg_test.go` | Modified | All ~10 pre-existing `pipkg.Check(...)` call sites updated for the new `(CheckReport, error)` return signature — no behavior change (all still exercise the "worktree" basis since `fixtureOverlay` is a plain non-git temp dir). |
| `engine/runtime/pi.go` | Modified | `SyncCheck`/`Status` updated for `Check`'s new signature; both messages now append `report.Disclosure()`. |
| `engine/cmd/main.go` | Modified | `runPipkgCore`'s `check` verb prints `report.Disclosure()` to stdout before the OK/error line, always, on both success and failure. |
| `bin/labdrian-overlay` | Modified | `pipkg_sync_check_and_report` echoes `$drift_output` (carries the basis line) on the no-drift path too; doc comment on `cmd_sync_check`. |

### TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 2.1 | `engine/pipkg/pipkg_provenance_test.go` | Unit (git-in-`t.TempDir`) | ✅ full `pipkg` suite green before edit | ✅ Written — confirmed failing (`assignment mismatch: 2 variables but pipkg.Check returns 1 value`, i.e. the whole provenance test file failed to compile against the old `Check` signature, proving the RED state) | ✅ Passed | ➖ Single scenario (builtFrom == HEAD) | ➖ None needed |
| 2.2 | `engine/pipkg/pipkg_provenance_test.go` | Unit (git-in-`t.TempDir`) | same run | ✅ Written — confirmed failing | ✅ Passed | ✅ uncommitted edit / committed edit (still ref) / tampered deployed file (real drift) — 3 cases | ✅ Fixed two real bugs surfaced by triangulation (see Issues Found) |
| 2.3 | `engine/pipkg/pipkg_provenance_test.go` | Unit (git-in-`t.TempDir`) | same run | ✅ Written — confirmed failing | ✅ Passed | ✅ main-fallback + non-git-root — 2 cases | ➖ None needed |
| 2.4 | `engine/pipkg/pipkg_provenance_test.go` | Unit (git-in-`t.TempDir`) | same run | ✅ Written — confirmed failing | ✅ Passed | ✅ `--option`-shaped / non-hex / empty — 3 cases, all proven to never reach `git` (regex gate runs before any `exec.Command` using the value) | ➖ None needed |

### Test Summary

- **Total tests written**: 5 new test functions (`pipkg_provenance_test.go`) plus ~10 pre-existing `pipkg_test.go` call sites mechanically updated for the new signature (no new assertions there).
- **Total tests passing**: full `engine/pipkg`, `engine/runtime`, `engine/cmd`, `engine/shelltest` packages green; full repo `go test -count=1 -race ./...` green (13 packages).
- **Layers used**: Unit (git-in-`t.TempDir`, real `git` subprocess, skipped under `-short`) — no runtime harness applicable beyond the existing `shelltest` bash-helper suite, which was re-run and confirmed unaffected.
- **Approval tests**: none new this slice.
- **Pure functions created**: `stripBuiltFrom` (JSON normalization, no I/O); `CheckReport.Disclosure` (pure rendering).

### Issues Found (during TRIANGULATE — fixed before REFACTOR)

1. **package.json comparison bug (1)**: the first `stripBuiltFrom` implementation only re-marshaled `package.json` when a `labdrian` key was present, so a side with no `labdrian` field stayed pretty-printed (`json.MarshalIndent`) while the other side (post-strip) became compact — a pure formatting difference misreported as `package.json: changed`. Fixed by always re-marshaling both sides through the same canonical (compact) path regardless of whether `labdrian` is present.
2. **package.json comparison bug (2)**: the real deployed package (built from the actual repo, which has reachable `v*` tags) resolved a real semantic `version`, while the comparison rebuild — built from a `git archive` export with no `.git` directory — always fell back to `"0.0.0-dev"`, a genuine (but spurious) version mismatch. Fixed by threading a separate `provenanceRoot` (always the original, tag-history-bearing `overlayRoot`) and an explicit `rev` into `buildInto`, decoupled from the file-source root used for reading `skills/`/`agents/`.

Both were caught by triangulating against the REAL overlay repo (`TestPipkgBuild_LiveRegistryNamesMatch`'s pattern, and the `engine/shelltest` suite which builds against this worktree's actual git history) rather than only the synthetic `fixtureOverlay` fixture (which is never a git repo and would not have exposed either bug).

### Work Unit Evidence

| Evidence | Value |
|---|---|
| Focused test command and exact result | `cd engine && go test ./pipkg/... ./runtime/... ./cmd/... ./shelltest/...` → all four packages `ok`, 0 failures. |
| Runtime harness command/scenario and exact result | `engine/shelltest` `TestPipkgHelpers_BuildStatusSyncCheck` and `TestPipkgStatusAndReport_DelegatesHonestPartialThenSupported` — real `bash` + a freshly `go build`'t `gentle-ai-overlay` binary, against this worktree's real git history (basis=ref against this worktree's actual HEAD): both pass, `SYNC_CHECK:pi:`/`VERDICT:pi:` lines unchanged from Slice 1. |
| Rollback boundary | Revert `engine/pipkg/pipkg.go`, delete `engine/pipkg/pipkg_provenance_test.go`, revert `engine/pipkg/pipkg_test.go`, `engine/runtime/pi.go`, `engine/cmd/main.go`, `bin/labdrian-overlay` — independently revertible; no cross-slice coupling (Phase 1 files untouched this batch). |

### Broad Verification

| Command | Result |
|---|---|
| `cd engine && gofmt -l .` | empty (clean) |
| `cd engine && go vet ./...` | no output (clean) |
| `cd engine && go test -count=1 -race ./...` | all 13 packages `ok` |
| `shellcheck -S warning bin/labdrian-overlay` | 2 findings, both pre-existing `SC2064` (unrelated to this slice; no new findings) |
| `git diff --shortstat a1f2bc0..HEAD -- engine bin` (working tree, uncommitted) | `5 files changed, 628 insertions(+), 49 deletions(-)` — plus one new file `engine/pipkg/pipkg_provenance_test.go` (237 lines) → **677 authored lines total, over the 400-line budget** |

### Deviations from Design

None functionally — matches D5/D6/D7's intent exactly (`labdrian.builtFrom`, `CheckReport{Basis,Ref}`, `git archive`+`archive/tar` export, always-disclosed basis). One implementation detail beyond what D6's prose states explicitly: `buildInto` needed a `provenanceRoot` parameter (distinct from the file-source root) that D6 does not call out, because a `git archive` export has no `.git` and cannot resolve `resolvePackageVersion`'s tag lookup itself — without this, comparing against a real, tagged repository would misreport `package.json.version` as drift (Issues Found #2 above). This is a strict subset of D6's existing "export at that sha... buildInto from it" description, not a deviation from it.

### Workload / PR Boundary — BLOCKED on budget, size:exception recommended

- Mode: stacked-to-main PR slice (PR 2 of 5), per the entry contract's `review_slices` (P=5).
- Current work unit: sync-check-provenance (R-005..R-007), tasks 2.1–2.8 (all of Phase 2).
- Boundary: starts from `a1f2bc0` (Phase 1, merged into this stack), would end with Phase 2 GREEN + docs, all tests passing, tasks 2.1–2.8 marked `[x]` — **not yet landed**.
- **Estimated review budget impact: 677 authored lines (`git diff --shortstat a1f2bc0..HEAD -- engine bin`), well over the 400-line budget** (design's own slice table estimated 280–380 for this slice; the actual `git archive`/`archive/tar` export plumbing plus `resolveComparisonSource`'s three-basis logic plus the git-fixture RED test file exceeded that estimate).
- One honest slicing pass was completed (this is that pass) with no further split attempted, per `work-unit-commits`' "Splitting is bounded" rule and the explicit "never shrink a diff by deleting comments, blank lines, docs, or tests... to fit the review budget" guard — no code-golfing was applied.
- **STOPPED before committing per this batch's explicit instruction** ("Budget 400 authored lines... STOP `partial` before committing if exceeded"). The worktree is left with all Phase 2 changes present but uncommitted and unstaged (`git status` shows 5 modified files + 1 untracked new test file), so a size:exception decision or a further slice split (e.g., separating the `CheckReport`/basis-resolution core from the `bin/labdrian-overlay` disclosure wiring — though the latter alone is only ~20 lines and would not meaningfully reduce the total) can be made before this lands.
- `tasks.md` Phase 2 checkboxes (2.1–2.8) are intentionally left `[ ]` — the work is implemented and verified, but not delivered.
- Slices planned=5 realized=1 (unchanged this batch — no new slice was delivered/committed; Slice 1 remains the only realized slice. Within tolerance.)

### Status

Phase 2 (sync-check-provenance, R-005..R-007) is implemented and fully verified (all focused, runtime, and broad checks pass) but **blocked from committing by the 400-line review budget** (677 authored lines). Returning `partial`. The orchestrator should choose one of: (a) grant `size:exception` and have a follow-up apply batch commit this exact diff as-is, or (b) direct a further split (e.g., a separate PR for the `bin/labdrian-overlay` disclosure-line plumbing vs. the `engine/pipkg` core) before landing. No code was reverted; the worktree retains the full, tested Phase 2 implementation uncommitted.
