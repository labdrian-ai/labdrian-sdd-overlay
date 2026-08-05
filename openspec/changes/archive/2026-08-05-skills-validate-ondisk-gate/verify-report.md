```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:1a01b69a3855a954faf4bfd33a344163ff36bcecf6b92b3e3663c8f1b5de2d35
verdict: pass
blockers: 0
critical_findings: 0
requirements: 11/11
scenarios: 11/11
test_command: cd engine && go test -count=1 ./... && cd ../tui && go test -count=1 ./... && for m in tools/*/go.mod; do (cd "$(dirname "$m")" && go test -count=1 ./...) || exit 1; done
test_exit_code: 0
test_output_hash: sha256:99b6567a222580f61ddc6a856f0f034abd406e4482703124ec6301b043792885
build_command: cd engine && go build ./... && go vet ./... && cd ../tui && go build ./... && go vet ./...
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report

**Change**: skills-validate-ondisk-gate
**Version**: spec revision 2 / design revision 2
**Mode**: Strict TDD
**Candidate**: branch `ondisk-gate-wiring`, HEAD `c780d5e`, tree `b654d2d006d4d151145fd6452817f97d1ddc9ebb` (matches the approved receipt binding)
**Working tree**: clean before and after verification (`git status --porcelain` = 0 entries)

### Completeness
| Metric | Value |
|--------|-------|
| Tasks total | 12 |
| Tasks complete | 12 |
| Tasks incomplete | 0 |

`openspec/changes/skills-validate-ondisk-gate/tasks.md`: 12 `[x]`, 0 `[ ]`. Matches the Engram tasks artifact and the code state.

### Build & Tests Execution

**Build**: PASSED — `go build ./...` and `go vet ./...` clean in `engine/` and `tui/`; `gofmt -l .` clean.

**Tests**: PASSED — 14/14 packages ok, 0 failed, 0 skipped.

```text
ok  engine/assets  engine/cmd  engine/gadu  engine/gate  engine/installer
ok  engine/prespec  engine/propagator  engine/runtime  engine/settings  engine/skills
ok  tui
ok  tools/deterministic-check-runner  tools/entry-contract-validator  tools/review-preflight
```

**Go version trap check**: PASSED. `engine/go.mod` declares `go 1.21`; local toolchain is `go1.26.5`. Audited every changed Go file for post-1.21 stdlib APIs (`t.Chdir`, `t.Context`, `b.Loop`, `os.Root`, `slices.Sorted/Collect`, `strings.SplitSeq/Lines`, `maps.Collect`, `cmp.Or`, `math/rand/v2`, `errors.ErrUnsupported`, integer `for range`) — none present. Changed test files import only `bytes`, `errors`, `os`, `path/filepath`, `strings`, `testing`. CI-safe.

**Coverage**: not run — no coverage threshold configured for this repo.

### Runtime Acceptance Evidence (real binary, not claims)

Built `engine/cmd` to a scratch path and executed the real binary. No repository file was created, modified, or moved; the two probes that required a real file used an untracked scratch file that was removed, with `git status --porcelain` confirmed at 0 entries afterwards.

| # | Acceptance criterion | Command / vector | Observed | Verdict |
|---|---|---|---|---|
| 1 | R-011 real tree exits 0 | `cd engine && skillsbin skills validate --registry ../skills.registry.yaml --manifest ../overlay.manifest --source-root ../skills` | `registry and manifest aligned (20 skills)` + `skills/ on disk matches overlay.manifest (57 files)`, exit 0 | PASS |
| 2 | R-002 absent `--source-root` fails loud | same, flag omitted | `error: skills validate requires --source-root <skills-dir>`, exit 1 | PASS |
| 3 | R-002 same directory resolved from 4 cwds | repo root `--source-root skills`; `engine/` `--source-root ../skills`; `engine/skills/` `--source-root ../../skills`; outside the repo, absolute root | identical output all four times (`20 skills`, `57 files`), exit 0 | PASS |
| 4 | R-002 scanned dir follows the flag, not cwd | repo root, `--source-root engine/skills` (the 28-file Go package) | 102 `UNREGISTERED_ON_DISK` lines, exit 1 — same cwd, different flag, different result | PASS |
| 5 | R-005 unregistered file blocks | untracked probe `skills/_shared/sdd-verify-probe-DELETEME.md` | `[UNREGISTERED_ON_DISK] _shared/sdd-verify-probe-DELETEME.md: ... add a row for ... to overlay.manifest to register it`, exit 1 | PASS |
| 6 | R-005 adding the row clears it | same file, scratch manifest copy with the row appended | `skills/ on disk matches overlay.manifest (58 files)`, exit 0 | PASS |
| 7 | R-005 removing the file restores green | probe deleted | exit 0, `57 files`, `git status --porcelain` = 0 | PASS |
| 8 | R-006 orphan row blocks | scratch manifest copy + `ghost-ref/notes.md managed`, real `skills/` | `[MISSING_ON_DISK] ghost-ref/notes.md: ... remove the "ghost-ref/notes.md" row from overlay.manifest, or restore the missing file`, exit 1 | PASS |
| 9 | R-007 full-scan in one run | probe file + `ghost-ref/notes.md` + `ghost-skill/SKILL.md` rows | 4 divergence lines across 3 classes (`MISSING_IN_REGISTRY`, `UNREGISTERED_ON_DISK`, 2x `MISSING_ON_DISK`) in a single run, exit 1 | PASS |
| 10 | R-008 four existing classes unchanged | built the pre-change binary from `git archive be2c3ca`, ran both binaries on one fixture producing all four classes | `diff` of the four registry lines: **byte-identical**; both exit 1; aligned fixture: base stdout line reproduced byte-for-byte by HEAD plus one new on-disk line, both exit 0 | PASS |
| 11 | R-009 diagnostics name the row edit, never `sync-manifest` | inspected every live diagnostic above | both classes name the exact `overlay.manifest` row edit; zero occurrences of `sync-manifest` | PASS |
| 12 | R-004 infra exclusions pinned bidirectionally | `engine/skills/manifest.go:13` vs `engine/skills/ondisk.go:66-72` | `infraPrefixes = []string{"engine/", "_shared/"}` vs on-disk excluding only first segment `== "engine"`; probe 5 confirms a `_shared/*` file IS scanned by the on-disk check | PASS |
| 13 | R-010 documented contract | live `skillsbin` usage output | `validate [--registry <path>] [--manifest <path>] --source-root <path>  cross-check registry vs manifest and skills/ on disk; exit 1 on divergence` | PASS |

Probe 4 is the decisive R-002 evidence the unit suite does not supply: with the working directory held constant and only `--source-root` varied, the result flips from 0 divergences to 102. Combined with probe 3 (four different working directories, equivalent relative and absolute roots, identical result), the resolved directory demonstrably tracks the flag and never the process cwd.

### Spec Compliance Matrix

| Requirement | Scenario | Test / evidence | Result |
|---|---|---|---|
| R-001 | Enumeration precedes the merge | `openspec/changes/skills-validate-ondisk-gate/design.md` consumer table, 11 rows, committed `749f028` (slice 1) before the emitting code in `955fc9a` (slice 2) | COMPLIANT |
| R-002 | cwd-independent, no implicit fallback | `skills_test.go > TestRenderValidateCoreOnDiskGate/absent_source_root_fails_loud_naming_the_flag` (PASS, fully covers the no-fallback half) + `cwd_independence_with_absolute_source_root` (PASS, but stubbed — see W1) + runtime probes 2/3/4 (cover the resolution half in full) | COMPLIANT* |
| R-003 | Core test runs without touching the repo tree | `TestRenderValidateCoreOnDiskGate` — 5 of 6 subtests inject `stubScan`; no subtest passes a real repo path to the scan seam | COMPLIANT |
| R-004 | Unifying either direction fails the pin test | `ondisk_test.go > TestInfraExclusionRulesArePinnedIndependently` (both subtests PASS) + probe 12 | COMPLIANT |
| R-005 | Blocks then clears | `TestRenderValidateCoreOnDiskGate/unregistered_file_fails_then_row_added_passes` (PASS) + probes 5/6/7 | COMPLIANT |
| R-006 | Orphan row blocks validation | `TestRenderValidateCoreOnDiskGate/orphan_row_fails_loud` (PASS) + probe 8 | COMPLIANT |
| R-007 | Three divergences in one run | `TestRenderValidateCoreOnDiskGate/mixed_divergences_all_reported_in_one_run` (PASS) + `registry_divergence_survives_scan_failure` (PASS) + probe 9 | COMPLIANT |
| R-008 | No regression | full suite green; zero deleted assertion statements in existing tests; probe 10 byte-identical base-vs-HEAD | COMPLIANT |
| R-009 | Diagnostic names the working fix | `ondisk_test.go > TestOnDiskDiagnosticsNameManifestRowEdit` (2 table cases, PASS) + probe 11 | COMPLIANT |
| R-010 | Docs match the real contract | five doc sites diffed (`README.md` x2, `bin/labdrian-overlay:149`, `engine/cmd/main.go` doc comment + usage line) + probe 13 live help | COMPLIANT |
| R-011 | Real tree is clean | `ondisk_test.go > TestRepositorySkillsAreFullyRegistered` (PASS, walks the real tree) + probes 1/3 | COMPLIANT |

**Compliance summary**: 11/11 scenarios COMPLIANT, 0 UNTESTED, 0 FAILING.

\* R-002 is counted COMPLIANT because its scenario was proven in full at runtime during this phase (probes 2, 3 and 4) and because its no-implicit-fallback conjunct is durably enforced by a passing unit test. The resolution conjunct rests on this phase's runtime evidence rather than on a durable unit guard — that gap is W1 below, a WARNING, not an unproven requirement.

### Correctness (Static Evidence)

| Requirement | Status | Notes |
|---|---|---|
| R-002 | Implemented | `sourceRoot` in `RenderValidateCore` (`skills.go:69`) is assigned only from `--source-root`; default `""` short-circuits to a usage error at `skills.go:90-94`. There is no code path that derives a directory from cwd. |
| R-003 | Implemented | `scanSkills func(string) ([]string, error)` parameter (`skills.go:66`); `SkillsCore` passes `ScanSkillFiles` (`skills.go:21`). |
| R-004 | Implemented | Two rule sets deliberately divergent and both pinned. |
| R-005/R-006 | Implemented | `DiffOnDisk(diskPaths, manifestPaths)` at `skills.go:141`; both classes printed at `skills.go:145-147`; exit 1 at `skills.go:149-152`. |
| R-007 | Implemented | Registry divergences printed at `skills.go:117-121` **before** the three fatal on-disk stages (the R4-001 correction); on-disk divergences printed in full at `skills.go:145-147`; no first-error-stop anywhere. |
| R-008 | Implemented | `engine/skills/validate.go` is untouched by the diff; the four classes' `Detail` format strings are unchanged. |
| R-009 | Implemented | Both `Detail` strings rewritten in `ondisk.go:147-150,166-169`. |
| R-010 | Implemented | Five doc sites. |
| R-011 | Implemented | Real tree at zero divergences: 57 deployable rows, 57 files. |

### Coherence (Design revision 2)

| Decision | Followed? | Notes |
|---|---|---|
| D1: `--source-root` required, absent fails loud | Yes | Exact message as designed; no default, no derivation. |
| D2: inject `scanSkills`; manifest rows via existing `readFile` seam | Yes | Manifest read twice, exactly as the design accepted (see S1). |
| D3: check lives in `RenderValidateCore`, not `Validate` | Yes | `Validate`'s signature and its 10 test call sites are untouched; no other command gained the gate. |
| D4: the design table is the persisted R-001 enumeration | Yes | 11 rows on disk and in Engram; committed before the emitting code. |
| D5: amend the motivating spec scenario's invocation vector | Yes | `verification-evidence-capture/spec.md` GIVEN + WHEN amended in slice 2; edited in `openspec/changes/`, never under `archive/`. |
| Consumer row 4 (TUI): "No change" | Yes | The diff touches no `tui/` file (see W2 for the stale proposal text). |
| `ci.yml:158` gains `--source-root ../skills` | Yes | Verified in the diff; probe 1 runs that exact vector and exits 0. |
| Never `t.Chdir` (Go 1.24 API) | Yes | `os.Chdir` + `t.Cleanup` used instead; no `t.Chdir` anywhere. |

### TDD Compliance

| Check | Result | Details |
|---|---|---|
| TDD Evidence reported | PASS | "TDD Cycle Evidence" table present in apply-progress with 4 rows |
| All tasks have tests | PASS | 12/12 tasks; behavior-bearing tasks map to named tests that exist |
| RED confirmed (tests exist) | PASS | `TestInfraExclusionRulesArePinnedIndependently`, `TestOnDiskDiagnosticsNameManifestRowEdit`, `TestRenderValidateCoreOnDiskGate` all exist at the reported paths |
| GREEN confirmed (tests pass) | PASS | All named tests re-executed with `-count=1 -v`; every subtest PASS |
| Triangulation adequate | PASS | `TestRenderValidateCoreOnDiskGate` 6 subtests; `TestOnDiskDiagnosticsNameManifestRowEdit` 2 table cases; `TestInfraExclusionRulesArePinnedIndependently` 2 directions |
| Safety net for modified files | PASS | Slice 1 landed a zero-behavior-change pin before slice 2's emitting code; existing suites re-run at each step |
| Correction test present | PASS | `registry_divergence_survives_scan_failure` covers the R4-001 correction and asserts both the surviving registry divergence and the scan error |

**TDD Compliance**: 7/7 checks passed.

### Test Layer Distribution

| Layer | Tests | Files | Tools |
|---|---|---|---|
| Unit (stubbed seams, temp dirs) | 3 top-level, 10 subtests/cases | 2 | `go test` |
| Integration (real `ScanSkillFiles` / real tree) | 2 (`TestSkillsCore/verb_validate`, `TestRepositorySkillsAreFullyRegistered`) | 2 | `go test` |
| Command-level (real binary) | 13 runtime probes | — | built `engine/cmd` binary |
| E2E | 0 | 0 | not installed |

`ScanSkillFiles` is exercised for real by `TestSkillsCore/verb_validate` (absolute temp source root) and `TestRepositorySkillsAreFullyRegistered` (the real repo tree). It is not exercised through `RenderValidateCore` from more than one cwd by any unit test — that gap is W1.

### Assertion Quality

| File | Test | Issue | Severity |
|---|---|---|---|
| `engine/skills/skills_test.go` | `TestRenderValidateCoreOnDiskGate/cwd_independence_with_absolute_source_root` | Passes `stubScan(...)`, whose closure is `func(string) ([]string, error) { return files, nil }` — it discards the directory argument. Combined with the placeholder `--source-root "/does-not-matter-because-stubbed"`, the subtest asserts only `exitCode == 0`, a value that no `os.Chdir` in the loop can influence. The subtest cannot observe the property named in its own title. | WARNING |
| `engine/skills/skills_test.go` | `cwd_independence...` and the second half of `unregistered_file_fails_then_row_added_passes` | Initialise `exitCode := 0` and assert `exitCode == 0`, so "exit 0" means "`exit()` was never called". No stdout assertion accompanies it (unlike `verb_validate`, which asserts `aligned`/`OK`). | SUGGESTION |

No tautologies, no ghost loops, no assertions that skip production code, no mock-heavy tests. Every other assertion checks a real observable: exit code plus specific stderr class or remediation substring.

### Quality Metrics

**Linter**: `go vet ./...` clean in `engine/` and `tui/`.
**Formatter**: `gofmt -l .` clean.
**Type checker**: `go build ./...` clean (exit 0, empty output).

### Issues Found

**CRITICAL**: None.

**WARNING (2)**:

- **W1 — R-002's positive property has no unit-test regression guard (reviewer finding 1, CONFIRMED).** I verified the claim directly against `engine/skills/skills_test.go:380-417` and the `stubScan` helper at `:196-198`. The helper signature is `func(string) ([]string, error)` and its body ignores the argument entirely, so `cwd_independence_with_absolute_source_root` never invokes `ScanSkillFiles` and never observes which directory was resolved; the three `os.Chdir` calls cannot affect the asserted value. The relative `--source-root ../skills` vector shipped at `.github/workflows/ci.yml:158` likewise has no unit equivalent.
  **Assessment**: the *requirement* is satisfied — the no-cwd-fallback half is genuinely enforced by `absent_source_root_fails_loud_naming_the_flag` (its `neverScan` stub calls `t.Fatal` if the scan is reached) and structurally by the code, where `sourceRoot` has no assignment other than the flag; and the positive half is proven at runtime by probes 3 and 4. What is missing is a durable guard: a recording stub asserting `scanSkills` received the resolved `--source-root`, plus one relative-path vector. Roughly three lines. This is a coverage debt, not a defect, so it does not block archive — but it should be logged as a follow-up, because the regression it fails to catch is precisely the one this change exists to prevent.

- **W2 — `proposal.md` contradicts design revision 2 on the R-001 enumeration (reviewer finding 2, CONFIRMED and slightly broader than reported).** `openspec/changes/skills-validate-ondisk-gate/proposal.md` says "Exit-code blast radius is **9 consumers**, not 3" (line 35), lists `tui/run.go` under **Modified** (line 51), and carries an unchecked criterion "R-001: enumeration of **9 consumers** ... persisted" (line 77). Design revision 2 closes the table at **11 rows** and dispositions the TUI as "No change"; the shipped diff touches no `tui/` file (verified: `git diff --name-only be2c3ca HEAD | rg '^tui/'` is empty).
  **Assessment**: R-001 itself is COMPLIANT — its scenario requires a persisted enumeration with path/line and per-consumer disposition to exist in change history before the emitting code, and `design.md`'s 11-row table satisfies that, committed in slice 1 ahead of slice 2. The defect is traceability: two persisted artifacts in the same change folder state different consumer counts, and one advertises a file modification that never shipped. A reviewer re-deriving the enumeration from the proposal — which is exactly what R-001's minimum PASS evidence asks for — would search for 9 and find 11. Non-blocking, but it should be corrected at archive time when the change folder is finalised.

**SUGGESTION (3)**:

- **S1 — double manifest read (reviewer finding 3): the rationale holds.** `RenderValidateCore` reads the manifest through `Validate` → `LoadManifestView` → `os.Open` and again through the injected `readFile` seam. Design Decision 2 accepted this to keep `Validate` untouched, and Decision 3 justifies why `Validate` must stay untouched: it has 1 non-test caller and 10 test call sites, and widening its contract would structurally hand the gate to any future caller, violating skill-package-manager R-034. The cost is one extra small file read per invocation. One residual note worth recording: only one of the two reads goes through the injected seam, so a test could in principle feed divergent content to the registry check and the on-disk check. No current test does, and no requirement is affected.

- **S2 — the `skills/` prefix in on-disk diagnostics is hardcoded.** `DiffOnDisk` formats `"skills/%s ..."` regardless of the actual `--source-root`. Probe 4 shows the cosmetic consequence: scanning `engine/skills` reports `skills/install.go` rather than `engine/skills/install.go`. The `Path` field carries the correct relative path, and production always passes the real skills directory, so no requirement is affected.

- **S3 — tighten the two `exitCode := 0` success assertions** by also asserting the expected stdout line, matching the pattern already used in `TestSkillsCore/verb_validate`.

### Verdict

**PASS WITH WARNINGS** — all 11 requirements are implemented and empirically verified against the real binary, and all 11 spec scenarios are proven by passing covering tests and/or this phase's runtime probes. Nothing is left unproven. The single substantive caveat is a durability gap: R-002's resolution property is proven here at runtime but has no unit-test regression guard (W1), and the change folder's proposal text contradicts the design's consumer enumeration (W2). Zero CRITICAL findings, zero blockers, 12/12 tasks complete, entire test suite green, working tree unchanged.
