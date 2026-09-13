```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:3ba47acf19c8582029ef4a23b12c07ff48f5fb3ddad007f11dcebb9f77aa4748
verdict: fail
blockers: 0
critical_findings: 0
requirements: 17/21
scenarios: 35/41
test_command: cd engine && go test -count=1 -race ./... && cd ../tools/archive-anchor-gate && go test -count=1 ./... && cd ../../longterm-mem && go test -count=1 ./...
test_exit_code: 0
test_output_hash: sha256:48a8f174fddd1b0308b596298f5ef0884e6b6d324da6d4e536c2323624456a4a
build_command: cd engine && gofmt -l . && go vet ./... && cd ../tools/archive-anchor-gate && gofmt -l . && go vet ./... && go build ./... && cd ../../longterm-mem && go vet ./...
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report

**Change**: pi-package-hardening
**Version**: N/A (delta specs)
**Mode**: Strict TDD
**Pass**: 2 (post-remediation re-verification)
**Worktree**: `/home/labdrian/labdrian-sdd-overlay-worktrees/pph-5` @ `fcd46e2` on `feat/pi-package-hardening-5-gadu` (PR #326)
**Supersedes**: pass-1 report `evidence_revision: sha256:dec2d35984887ecf3e676aca867828d7960e90ce30818b3dfea361a1fc1ecfc2` (verdict `fail`, CRIT-1)

### What changed since pass 1

`git diff --stat bf1f4b7..HEAD` is exactly seven files — three code/module files and four SDD documents:

```text
openspec/changes/pi-package-hardening/apply-progress.md | 107 +++++++++++
openspec/changes/pi-package-hardening/design.md         |   2 +-
.../specs/gadu-pi-subagent/spec.md                      |  15 ++-
.../specs/review-receipt-capture/spec.md                |  13 +++
tools/archive-anchor-gate/go.mod                        |   4 +
tools/archive-anchor-gate/receipt.go                    | 103 +++++---------
tools/archive-anchor-gate/receipt_test.go               |  86 +++++++++--
```

No file under `engine/`, `bin/`, `longterm-mem/`, or `skills/` was touched. The pass-1 scratch smoke evidence (S1–S24) therefore still describes the exact bytes under test and is carried forward; S25/S26 are superseded by this pass's direct gate runs below.

### Completeness

| Metric | Value |
|--------|-------|
| Tasks total (automated, 1.x–5.x) | 42 |
| Tasks complete | 42 |
| Tasks incomplete | 0 |
| Manual tasks (Phase M, deferred by design) | 3 (M.1–M.3, unchecked) |
| Planned slices (P, from `entry.json` `review_slices`) | 5 |
| Realized slices (R, from `apply-progress.md`) | 5 |
| Plan vs realized drift | None — every planned slice has a landed counterpart |

Slice landings: 1 `98f8410`, 2 `f4bf3e4`, 3 `9506918`/`307b79e`, 4 `bcb1df7`, 5 `fb27128`/`5d1fec2`. Remediation 1: `89fa4d3`, `eaaac7e`, `fcd46e2`.

`apply-progress.md` now carries Slice 5 and Remediation 1 in the OpenSpec copy (WARN-5 closed — verified by reading the file, not the Engram copy).

**Phase M disclosure**: M.1–M.3 are unchecked. `tasks.md` declares Phase M `MANUAL, not run by sdd-apply`, and this verification session is explicitly forbidden from touching the live `~/.pi` or launching a `pi` session. The generic rule "unchecked tasks are CRITICAL" is therefore ruled down to WARNING here (WARN-8) — but the obligation is real and open, not discharged. See WARN-8.

### Build & Tests Execution

**Build**: PASS — combined build command exit 0, output empty (hash is the sha256 of the empty string, as expected).

```text
cd engine                     && gofmt -l .   -> empty, exit 0
cd engine                     && go vet ./... -> no output, exit 0
cd tools/archive-anchor-gate  && gofmt -l .   -> empty, exit 0
cd tools/archive-anchor-gate  && go vet ./... -> no output, exit 0
cd tools/archive-anchor-gate  && go build ./... -> exit 0 (standalone module build, as CI does)
cd longterm-mem               && go vet ./... -> no output, exit 0
```

**Tests**: PASS — 32 packages `ok`, 0 failed, 0 skipped-as-failure.

```text
cd engine && go test -count=1 -race ./...            -> 14 packages ok, exit 0
cd tools/archive-anchor-gate && go test -count=1 ./... -> ok, exit 0 (41 --- PASS)
cd longterm-mem && go test -count=1 ./...            -> 17 packages ok, exit 0
shellcheck -S warning bin/labdrian-overlay           -> exit 1, 2 findings
   both pre-existing SC2064 at lines 1458 and 1615; byte-identical to pass 1; no new findings
```

The gate module was re-run with `-count=1` after an initial cached `ok`, so the recorded result is a real execution, not a cache hit.

### Gate runs (CRIT-1 verification)

```text
cd tools/archive-anchor-gate && go run . --repo <worktree> --known-gaps known-gaps.txt
   -> exit 0, "ok: 7 report(s) checked, 1 known gap(s)"
      (7 archive reports checked: 1 verified, 5 self-asserted, 1 absent; the 2026-09-02
       longterm-mem gap is the single declared known gap — unchanged from pass 1)

cd tools/archive-anchor-gate && go run . --repo <worktree> --known-gaps known-gaps.txt \
     --change pi-package-hardening
   -> exit 0
   -> verified review receipt found for "pi-package-hardening"
      (approved_tree=ec56969bdb2a7ade90b83e47d971beb857ddadab)
```

**CRIT-1 is fixed and the fix is load-bearing — proven by reversion, not by assertion.** I rebuilt the gate in the session scratchpad from `git show 89fa4d3^:tools/archive-anchor-gate/receipt.go` plus the pre-fix `go.mod`, against the unchanged `gate.go`/`main.go`, and ran that pre-fix binary against this same worktree and the same four real receipts:

```text
<scratch>/gate-prefix --repo <worktree> --known-gaps known-gaps.txt --change pi-package-hardening
   -> exit 1
   -> FAIL no verified review receipt for change "pi-package-hardening"
```

Same repository, same receipts, same command — exit 1 before the fix, exit 0 after. The remediation's RED claim is independently confirmed, and the change now passes the pre-archive gate it introduced.

The root cause is also confirmed directly against production data: `jq 'keys, (.state|keys)'` on `review-receipts/review-42353e66bae50ad8.review-state.json` returns top-level `["revision","schema","state"]` with `lineage_id`, `state`, `current_snapshot`, `initial_snapshot`, `selected_lenses`, `risk_level` all nested inside the `state` object — the `gentle-ai.review-state-record/v2` wrapper the pre-fix `persistedReviewState` modeled as a flat `state string`.

The fix deletes the gate's private model entirely and delegates to `engine/reviewreceipt.ApprovedSummary` (`tools/archive-anchor-gate/receipt.go:11` import; `go.mod` `require` + relative `replace ../../engine`). The `engine` module is stdlib-only with no `go.sum`, and `go build ./...` inside the gate module succeeds standalone, so CI's `test-archive-anchor-gate` job (`.github/workflows/ci.yml:245`, which runs `gofmt`/`vet`/`go test ./... -cover` with `working-directory: tools/archive-anchor-gate` and then `go run -C tools/archive-anchor-gate .`) resolves the same way. Verified here by running exactly those four commands.

**WARN-3 closed as a consequence**: `receipt_test.go`'s `writeReviewStateFile` now emits the real nested `gentle-ai.review-state-record/v2` wrapper, and the new `TestApprovedTree_FromRealCapturedReceipt` is seeded from the committed production receipt rather than a fabricated payload. Both verified by reading the test source and running it (`--- PASS`).

**Coverage**: `tools/archive-anchor-gate` 88.3% (as reported by apply). Not re-measured repo-wide — informational, non-blocking.

### Spec Compliance Matrix

Authoritative counts, recounted this pass from the six delta spec files: **21 requirements, 41 scenarios** (`rg -c '^### (Requirement:|REQ-[0-9]+:)'` and `rg -c '^#### Scenario:'`). Pass 1 counted 40 scenarios; `review-receipt-capture` gained one in `eaaac7e`.

| Requirement | Scenario | Test / evidence | Result |
|-------------|----------|-----------------|--------|
| R-001 Mode Drift Detection | Mode-only drift is reported | `TestPipkgCheck_ModeDrift`; smoke S2 | COMPLIANT |
| R-001 | Identical files report no drift | `pipkg` suite; smoke S6 | COMPLIANT |
| R-002 Build Root Permissions | Fresh build root is 0755 | `TestPipkgBuild_RootPermissions`; smoke S1 | COMPLIANT |
| R-003 Registry Path Containment | Traversal path is rejected | `parse_test.go` `path_traversal_rejected` (4 cases); smoke S4 | COMPLIANT |
| R-003 | In-root path builds normally | `path_in_root_passes`; smoke S1 | COMPLIANT |
| R-004 SKILL.md Name Matches Directory | Mismatch is rejected | `TestPipkgBuild_RejectsNameMismatch` | COMPLIANT |
| R-004 | Match passes | `TestPipkgBuild_LiveRegistryNamesMatch` (real registry) | COMPLIANT |
| R-005 builtFrom Recorded at Build | builtFrom equals the resolved build commit | `TestBuiltFrom_RecordedAtBuild`; smoke S5 | COMPLIANT |
| R-006 Sync-Check Compares Against Deploy Ref | Unrelated feature-branch diffs do not cause drift | `TestCheck_FeatureBranchDoesNotFalseDrift`, `TestCheck_BuildOnFeatureBranchIsNotStale`; smoke S8 | COMPLIANT |
| R-006 | Real divergence from the deploy ref is still reported | `TestCheck_StaleAfterCommittedSourceChange`; smoke S7 | COMPLIANT |
| R-007 Unresolvable Ref Discloses a Main-Only Comparison | Fallback comparison is disclosed | `TestCheck_MainFallback`; smoke S10 | PARTIAL (WARN-1) |
| R-008 Receipt Persisted Before Acknowledge | Receipt file exists before acknowledge is invoked | `TestCapture_*`, `TestApprovedSummary`, `TestHook_*`; smoke S12–S17 | COMPLIANT |
| R-008 | Multiple active changes deny, unless every surviving receipt is already persisted | `TestHook_Deny_MultipleActiveChanges`, `TestHook_MultiChange_DeniesWhenAnyUncaptured`, `TestHook_MultiChange_PassesAfterExplicitCapture`; smoke S13/S14 | COMPLIANT |
| R-009 approved_tree Sourced Only From the Persisted Receipt | approved_tree equals the receipt's final_candidate_tree | `TestApprovedTree_FromReceipt`, `TestApprovedTree_FromReviewStateShape`, `TestApprovedTree_FromRealCapturedReceipt`; live gate run exit 0 naming `approved_tree=ec56969b…` | COMPLIANT |
| R-010 Closure-Feedback Reads the Persisted Receipt | Post-acknowledge closure still produces a verified value | `TestApprovedTree_NeverReadFromGitTransactionStore` (proxy) + `skills/inception-pipeline/SKILL.md` prose; no executable harness | PARTIAL (WARN-2) |
| R-011 Archive Blocks Without a Receipt Unless Overridden | Missing receipt blocks archive | `TestArchiveBlock_NoReceiptPostConvention`; pre-fix binary run reproduced the exact block message | COMPLIANT |
| R-011 | Recorded owner override permits archive | `TestOverride_RecordedSelfAsserted`, `TestPreArchiveFlag/override_present_passes` | COMPLIANT |
| R-012 Subagents Extension Installed When Missing | Neither package present triggers install | `TestSubagentsExtension_InstallWhenAbsent`; smoke S18 | COMPLIANT |
| R-012 | Alternate package already installed is treated as satisfied | `TestSubagentsExtension_Noop` (4 sub-cases) | COMPLIANT |
| R-013 GADU.md Linked as an Overlay-Owned Asset | Linked content matches the current generation | `TestInstall_WiresSubagentsExtensionAndGaduLink`; smoke S19 | COMPLIANT |
| R-013 | gentle-pi's own asset management does not touch it | `TestGaduLink_SurvivesOverwrite`; smoke S22 | COMPLIANT |
| R-014 Frontmatter Verified Compatible | GADU dispatches with working tool access | `TestFrontmatter_InlineToolsScalar`, `TestInstall_RejectsAmbiguousGaduFrontmatter` guard the emitted form; actual dispatch is Phase M (manual, deferred) | PARTIAL (WARN-8) |
| R-015 Honest, Distinct Extension and Link Status | Stale link is reported independent of extension state | `TestGaduLinkState_Matrix/stale (broken target)` — scenario reworded in `eaaac7e` to the state the design makes reachable; test and spec now agree | COMPLIANT |
| R-015 | Missing extension does not collapse link status | `TestStatus_ReportsUnprovenSubagentsAndGaduLinkEntries`; smoke S21 | COMPLIANT |
| R-016 Uninstall Removes Only the Overlay-Owned Link | Only GADU.md is removed | `TestUninstall_OwnedLinkOnly`, `TestUninstall_LeavesForeignGaduFileUntouched`; smoke S22, S23 | COMPLIANT |
| PRT Package-Delivered Skills and Agents | Local-path install registers the package idempotently | `engine/runtime` pi suite; smoke S18 argv | COMPLIANT |
| PRT | Skills are visible in a Pi session and agents ship as package content | `engine/runtime` pi suite; smoke S18/S19 — only `GADU.md` written under `~/.pi/agent/agents/` | COMPLIANT |
| PRT Honest Status for Unproven Activation | Unproven entry forces partial | smoke S21 (five entries named separately) | COMPLIANT |
| PRT | All entries proven report supported | `TestPiAdapter_StatusTriangulatesAllOwnedEntries/all_five_entries_proven_reports_supported` | COMPLIANT |
| PRT Pi-Scoped Uninstall | Only the owned package entry is removed | `TestUninstall_OwnedLinkOnly`; smoke S22 argv `remove <destDir>` | COMPLIANT |
| PRT | GADU link is removed without touching the extension package | smoke S22 — no `remove npm:pi-subagents-j0k3r` in argv | COMPLIANT |
| PRT Pi Drift Detection via Sync-Check | No drift is reported when unchanged | smoke S6 | COMPLIANT |
| PRT | A source edit is detected as drift | smoke S7 | COMPLIANT |
| PRT | Feature-branch checkout does not cause false drift | smoke S8; `TestCheck_BuildOnFeatureBranchIsNotStale` | COMPLIANT |
| AI Boundary Anchors | Anchors resolve and are legible in both stores | gate half now runtime-proven (exit 0, verified `approved_tree`); the t0/Engram + archive-report half is agent prose with no harness | PARTIAL (WARN-2) |
| AI | A missing receipt blocks archive unless an owner override is recorded | `TestArchiveBlock_NoReceiptPostConvention`, `TestOverride_RecordedSelfAsserted`; pre-fix binary reproduced the block | COMPLIANT |
| AI | An unrelated change touching the folder does not become the anchor | `TestAbsenceProseElsewhereDoesNotSilenceARealLandingCommit`, `TestLabelledCommitIsStillReadWhenAbsenceProseIsPresent` | COMPLIANT |
| AI | A mis-recorded anchor is rejected, not trusted | `TestScanArchiveRejectsAnUndisclosedTreeMismatch`, `TestScanArchiveAcceptsADisclosedRejection` | COMPLIANT |
| AI | A change predating the convention omits rather than guesses | `TestScanArchiveIgnoresReportsPredatingTheConvention`; gate run reports `anchor absent` for 2026-09-02 | COMPLIANT |
| AI | A change that skipped inception-pipeline still measures | `skills/inception-pipeline/SKILL.md` prose only; no executable harness | PARTIAL (WARN-2) |
| AI | Neither anchor resolves | `skills/inception-pipeline/SKILL.md` prose only; no executable harness | PARTIAL (WARN-2) |

**Compliance summary**: 35/41 scenarios COMPLIANT, 0 FAILING, 0 UNTESTED, 6 PARTIAL. 17/21 requirements fully compliant (every scenario COMPLIANT). The 4 requirements short of full are R-007 (WARN-1), R-010 (WARN-2), R-014 (WARN-8), and the `actuals-instrumentation` boundary-anchor requirement (WARN-2). Pass 1's 2 FAILING scenarios are both resolved.

### Correctness (Static Evidence)

| Requirement | Status | Notes |
|------------|--------|-------|
| R-001..R-004 | Implemented | `fileEntry{data,perm}`, `Chmod(tmpDir,0755)`, `validateEntry` containment, `checkSkillNameMatchesPath` |
| R-005..R-006 | Implemented | `labdrianField{BuiltFrom}`, `CheckReport{Basis,DeployRef,DeployTip,BuiltFrom}`, `git archive` + `archive/tar` export |
| R-007 | Implemented with a gap | `Disclosure()` (`engine/pipkg/pipkg.go:226`) branches only on `Basis`; on `deploy` it renders one sentence whose only variation is `BuiltFrom == ""` → `"unrecorded"`. A well-formed but unresolvable SHA renders identically to a resolvable one (WARN-1) |
| R-008 | Implemented | `Capture`, `DetectActiveChange`, `ApprovedSummary`, `RunHook` + `AllSurvivingApprovedPersisted`, fourth settings identity |
| R-009 | **Fixed** | gate's private `persistedReviewState` deleted; `loadApprovedTreeFromReceipts` delegates to `reviewreceipt.ApprovedSummary` — one reader for both on-disk shapes |
| R-010 | Implemented (docs) | `skills/inception-pipeline/SKILL.md` reads the persisted file, both shapes; `.git/...` path removed from the Plan |
| R-011 | Implemented | `ReceiptConventionDate`, `loadReceiptOverride`, `CheckPreArchive`, `--change` flag, Gate Compliance bullet |
| R-012..R-016 | Implemented | `isSubagentsExtensionListed`, `ensureSubagentsExtension`, `gaduLinkState`, `linkGaduAgent`, `unlinkGaduAgent`, `validateGaduFrontmatter` |

### Coherence (Design)

| Decision | Followed? | Notes |
|----------|-----------|-------|
| D1 mode drift | Yes | `copyFile` still writes 0644; symlinked dest root refused |
| D2 build root 0755 | Yes | `os.Chmod(tmpDir, 0755)` after `MkdirTemp` |
| D3 path containment | Yes | `validateEntry` + `filepath.Rel` defense in depth |
| D4 SKILL.md name in pipkg | Yes | line-oriented scan, zero-dependency ADR preserved |
| D5 provenance | Yes | `resolveBuildRev` + `resolvePackageVersion(root, rev)` |
| D6 comparison basis | Superseded, then followed | `Check` resolves the deploy ref and treats `builtFrom` as provenance; basis values `deploy`/`worktree`/`dirty` rename is consistent with the superseding decision. Runtime-proven (S7/S8) |
| D7 receipt capture | Yes | DEV-1 corrected in the design text (`eaaac7e`): `DetectActiveChange` is documented as artifact-bearing, not `state.yaml`-bearing, matching `activeChangeMarkers`. The previously-undisclosed multi-change allow path is now a named spec scenario with two covering tests (WARN-4 closed) |
| D8 receipt consumption | Yes | dual-shape dispatch now routed through the single correct reader; verified against all four real receipts |
| D9 missing receipt | Yes | `ReceiptConventionDate = "2026-09-12"`, override schema, `--change` flag, documented gate |
| D10 GADU placement | Yes | symlink; ownership proven by `Readlink` equality; no state file |
| D11 extension probe/install | Yes | prefix match on both package names, fixed argv, disclosure, skip env |
| D12 frontmatter | Yes | `tools: '*'` inline scalar emitted and guarded |
| D13 status/uninstall | Yes | DEV-3 resolved by reworking the spec, not the code: R-015's stale scenario now describes the broken-target state D10's stable-path symlink actually reaches, and `TestGaduLinkState_Matrix/stale (broken target)` covers it (WARN-7 closed) |

### Deviation Rulings (pass 2)

| # | Pass-1 ruling | Pass-2 status |
|---|---|---|
| DEV-1 | Spec is wrong; correct D7's `state.yaml` wording before archive | **CLOSED** — `design.md` D7 reworded in `eaaac7e` |
| DEV-2 | ACCEPTABLE (fail-closed is the spec's requirement) | Unchanged |
| DEV-3 | SPEC GAP — reword the unsatisfiable stale scenario | **CLOSED** — `specs/gadu-pi-subagent/spec.md` R-015 reworded in `eaaac7e`; test agrees |
| DEV-4 | ACCEPTABLE (gate-side proxy for agent prose) | Unchanged; residual risk remains WARN-2 |
| DEV-5 | ACCEPTABLE but regrettable | Unchanged |
| DEV-6 | ACCEPTABLE (`HOME` isolation is load-bearing) | Unchanged |
| DEV-7 | ACCEPTABLE (wiring is transitive) | Unchanged |

### TDD Compliance

| Check | Result | Details |
|-------|--------|---------|
| TDD Evidence reported | PASS | TDD Cycle Evidence tables present in `apply-progress.md` for slices 1–5 **and** Remediation 1 (WARN-5 closed) |
| All tasks have tests | PASS | all 31 task-named test functions verified present by name across the repo; zero missing |
| RED confirmed (tests exist) | PASS | Remediation 1's RED independently reproduced: the pre-fix gate binary exits 1 on the same real receipts where HEAD exits 0 |
| GREEN confirmed (tests pass) | PASS | all named tests re-executed at `-count=1`; 32 packages `ok`, 41 `--- PASS` in the gate module alone |
| Triangulation adequate | PASS | 4 traversal cases, 4 extension no-op cases, 5 link-state cases, 3 non-hex builtFrom cases, 4 `TestPreArchiveFlag` sub-cases, legacy + fabricated-nested + real-production receipt shapes |
| Safety Net for modified files | PASS | Remediation 1 records the full gate suite green pre-fix; earlier slices record pre-existing suites green before each modification |

**TDD Compliance**: 6/6 checks passed.

### Test Layer Distribution

| Layer | Tests | Files | Tools |
|-------|-------|-------|-------|
| Unit | 62 | 6 | `go test` |
| Integration (git-in-TempDir, scratch `HOME` + stub `pi`, real `bash`, real-production-data fixture) | 17 | 3 | `go test`, `git`, `bash` |
| E2E | 0 | 0 | not installed (deferred to Phase M) |
| **Total** | **79** | **9** | |

`TestApprovedTree_FromRealCapturedReceipt` is classified as integration: it reads a committed production artifact from the repository rather than a constructed fixture, which is exactly the property that makes it able to catch CRIT-1.

### Changed File Coverage

`tools/archive-anchor-gate`: 88.3% (as reported by apply; CI re-measures via `go test ./... -cover`). Repo-wide per-file coverage not re-measured — informational, non-blocking.

### Assertion Quality

Re-audited the two files changed since pass 1 (`tools/archive-anchor-gate/receipt_test.go`, and the gate sources it exercises), and re-confirmed the pass-1 audit of the other seven test files. No tautologies, no orphan empty-collection checks, no ghost loops, no smoke-test-only patterns, no mock-heavy files.

Pass 1's single assertion-quality defect — `writeReviewStateFile` fabricating a payload shape gentle-ai never writes — is **fixed**: the helper now emits the real nested wrapper, and a second test pins behaviour against a committed production receipt so the suite can no longer agree only with itself.

**Assertion quality**: 0 CRITICAL, 0 WARNING.

### Quality Metrics

**Linter (`go vet`)**: no errors (engine, longterm-mem, archive-anchor-gate).
**Formatter (`gofmt -l`)**: clean (engine, archive-anchor-gate).
**Shell (`shellcheck -S warning`)**: 2 pre-existing SC2064 warnings at `bin/labdrian-overlay:1458` and `:1615`. No new findings; `bin/labdrian-overlay` is unchanged since pass 1.

### Issues Found

**CRITICAL**

None. Pass 1's CRIT-1 is fixed, and the fix was verified by reversion rather than by trusting the remediation report.

**WARNING**

- **WARN-N1 (new this pass) — the gate's compiled binary is untracked and not gitignored.** `tools/archive-anchor-gate/archive-anchor-gate` (4.0 MB) is present in the worktree and `git check-ignore` exits 1 for it. `.gitignore` already carries a block titled "Compiled guard binaries: `go build ./...` inside a tools/ module writes the binary next to its source, where `git add -A` will sweep it in", listing `/tools/*/archive-reconcile`, `/tools/*/deterministic-check-runner`, and `/tools/*/entry-contract-validator` — `/tools/*/archive-anchor-gate` is missing from that list. This change is the one that makes the gate a routinely-built module (CI builds it; the new pre-archive step runs `go run -C tools/archive-anchor-gate .`), so it inherits exactly the hazard that block was written to prevent. **Follow-up, not blocking** — nothing is committed and archive does not run `git add -A` — but it should be one line in `.gitignore` before the next commit touching that directory.
- **WARN-1 (R-007) — unresolvable `builtFrom` is disclosed but not flagged as unusable.** `CheckReport.Disclosure()` (`engine/pipkg/pipkg.go:226`) switches only on `Basis`; on `deploy` the only variation is `BuiltFrom == ""` rendering as `"unrecorded"`. A well-formed-but-unresolvable SHA prints the same sentence as a resolvable one, so a reader cannot tell. R-007's requirement body asks the output to state "that the recorded build ref could not be used as provenance". **Ruling: follow-up, not blocking.** The scenario's own THEN clause ("compared against the deploy ref, with the unresolvable recorded build ref disclosed only as provenance") is satisfied, the operator is never misled about what was compared, and no safety property depends on it — but the requirement body is not fully met, so the scenario stays PARTIAL rather than being quietly promoted.
- **WARN-2 (R-010 and three `actuals-instrumentation` scenarios) — agent-prose behaviour has no executable harness.** closure-feedback lives in `skills/inception-pipeline/SKILL.md` and is executed by an agent, not by code. Compliance rests on a gate-side proxy test (`TestApprovedTree_NeverReadFromGitTransactionStore`) plus documentation. **Ruling: follow-up, not blocking** — the repo has no harness for agent prose, the proxy asserts the real invariant, and the gate half is now runtime-proven. The residual risk is that the prose and the gate could drift apart with nothing to catch it.
- **WARN-6 — `apply-progress.md`'s Slice 2 and Slice 3 narratives still read "NOT committed / blocked on budget".** Both landed (`f4bf3e4`, `9506918`/`307b79e`). Slice 4 gained a correcting landing note during Remediation 1; Slices 2 and 3 were explicitly deferred. **Ruling: follow-up, not blocking.** The Workload/PR Boundary sections and the Slice 5 / Remediation 1 sections state the true landed state, and the Completeness table above is authoritative. But this file is the one that travels into the archive, and a future reader opening it cold will read three stale "blocked" headers before reaching the correction. One sentence per section would close it.
- **WARN-8 (R-014, and Phase M M.1–M.3) — live GADU dispatch is unproven.** The frontmatter *form* is verified by two tests; the *dispatch* through `subagent_*` is Phase M, which requires a real `pi` session against the real `~/.pi` and is forbidden in this session. **Ruling: follow-up, not blocking for archive** — Phase M is declared manual and out of `sdd-apply` scope by `tasks.md` itself. But it is an open obligation: until M.1–M.3 run on a real machine, R-014's only scenario is unproven, and "the extension parses our frontmatter" remains an inference from a Go port of the extension's parser, not an observation.

**SUGGESTION**

- **SUG-1** — Four receipts are persisted for five slices; Slice 1's lineage predates the capture path. Worth one line in the archive report so a reader does not go hunting for a fifth.
- **SUG-2** — `pipkg check` echoes a raw `builtFrom` value (e.g. `--upload-pack=evil`) verbatim into operator-facing output. It never reaches a git argv (`builtFromPattern` gates it first), but consider quoting or eliding a value that fails the pattern. Naturally pairs with WARN-1, since both are edits to the same disclosure path.
- **SUG-3** — Add a gate self-test that runs `--change` against this repository's own real `review-receipts/` directory. `TestApprovedTree_FromRealCapturedReceipt` now pins the parser against production data, which is most of the value; a full end-to-end self-test would additionally pin the wiring. This is the test that would have caught CRIT-1 on the first pass.

### Verdict

**FAIL (no defects; not fully spec-proven)** — 0 blockers, 0 CRITICAL, 5 WARNING, 3 SUGGESTION, 17/21 requirements and 35/41 scenarios complete.

This verdict needs its plain meaning stated, because "fail" with zero blockers and zero critical findings reads like a contradiction. `gentle-ai sdd-verify-validate` ties any passing verdict to *complete* requirement and scenario counts: probed directly this pass, `pass` and `pass_with_warnings` are admitted only at 21/21 and 41/41, and are denied at 17/21 and 35/41 with "passing verdict contradicts failing or incomplete evidence". Six scenarios are not runtime-proven, so the counts cannot honestly reach complete, so the verdict cannot honestly be passing. The counts were not adjusted to reach a passing verdict.

**The exact blocker**: six scenarios have no covering test that passed at runtime.

| Scenario | Why not runtime-proven | Closable by |
|---|---|---|
| R-014 — GADU dispatches with working tool access | Requires a real `pi` session against the real `~/.pi`; forbidden this session | Running Phase M M.1–M.3 on a real machine |
| R-010 — Post-acknowledge closure produces a verified value | closure-feedback is agent prose; only a gate-side proxy test exists | Building a harness for agent prose, or accepting the proxy explicitly |
| AI — Anchors resolve and are legible in both stores | Gate half proven; t0-from-Engram and archive-report half is agent prose | as above |
| AI — A change that skipped inception-pipeline still measures | Agent prose only | as above |
| AI — Neither anchor resolves | Agent prose only | as above |
| R-007 — Fallback comparison is disclosed | Test passes; the requirement body's "could not be used as provenance" statement is absent from `Disclosure()` | A one-branch edit to `Disclosure()` (WARN-1, pairs with SUG-2) |

Only the first and last are closable by work in this repository. The middle four describe behaviour executed by an agent reading `skills/inception-pipeline/SKILL.md`, for which this repo has no harness — a structural limit, not an omission of this change. Ruling R-007 COMPLIANT instead would yield 36/41 and 18/21, still short of a passing verdict, so the grade does not turn on that judgement call.

**Everything this change actually built is green.** All 42 automated tasks complete across 5 planned and 5 realized slices with no drift; 32 packages pass at `-count=1 -race`; `gofmt`, `go vet`, and the standalone gate module build are clean; 35 scenarios are runtime-proven with 0 FAILING and 0 UNTESTED. Pass 1's CRIT-1 is fixed, and I proved the fix load-bearing by rebuilding the pre-fix gate and watching it exit 1 on the same four real receipts where HEAD exits 0. WARN-3, WARN-4, WARN-5, WARN-7, DEV-1 and DEV-3 are all closed.

**Archive readiness is the orchestrator's call, not this report's.** Validity and archive readiness are separate decisions. No finding here blocks archive on correctness grounds: the change passes the pre-archive gate it introduced (`--change pi-package-hardening` exits 0 with a verified `approved_tree`). What a reader must not do is mistake this for full spec proof — R-014 in particular rests on a Go port of the extension's parser rather than on an observed dispatch, and that gap closes only when a human runs Phase M.
