```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:dec2d35984887ecf3e676aca867828d7960e90ce30818b3dfea361a1fc1ecfc2
verdict: fail
blockers: 1
critical_findings: 1
requirements: 15/21
scenarios: 32/40
test_command: cd engine && go test -count=1 -race ./... && cd ../tools/archive-anchor-gate && go test ./... && cd ../../longterm-mem && go test -count=1 ./...
test_exit_code: 0
test_output_hash: sha256:a2d72c6aab0ccec9d491ceb106e1d0e000449b528c8ce1456d9b0d6c35100637
build_command: cd engine && gofmt -l . && go vet ./...
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report

**Change**: pi-package-hardening
**Version**: N/A (delta specs)
**Mode**: Strict TDD
**Worktree**: `/home/labdrian/labdrian-sdd-overlay-worktrees/pph-5` @ `d1cf988` on `feat/pi-package-hardening-5-gadu`

### Completeness

| Metric | Value |
|--------|-------|
| Tasks total (automated, 1.x–5.x) | 42 |
| Tasks complete | 42 |
| Tasks incomplete | 0 |
| Manual tasks (Phase M, deferred by design) | 3 |
| Planned slices (P, from `entry.json` `review_slices`) | 5 |
| Realized slices (R, from `apply-progress`) | 5 |
| Plan vs realized drift | None — every planned slice has a realized counterpart |

Slice landings: 1 `98f8410`, 2 `f4bf3e4`, 3 `9506918`/`307b79e`, 4 `bcb1df7`, 5 `fb27128`/`5d1fec2`. Working tree clean.

### Build & Tests Execution

**Build**: PASS

```text
cd engine && gofmt -l .                         -> empty (clean), exit 0
cd engine && go vet ./...                       -> no output, exit 0
cd longterm-mem && go vet ./...                 -> no output, exit 0
```

**Tests**: PASS — 32 packages `ok`, 0 failed

```text
cd engine && go test -count=1 -race ./...                 -> 14 packages ok, exit 0
cd tools/archive-anchor-gate && go test ./...             -> ok, exit 0
cd longterm-mem && go test -count=1 ./...                 -> 15 packages ok, exit 0
shellcheck -S warning bin/labdrian-overlay                -> 2 findings, exit 1
   both pre-existing SC2064 (lines 1458, 1615), no new findings
```

**Gate runs**

```text
cd tools/archive-anchor-gate && go run . --repo <worktree> --known-gaps known-gaps.txt
   -> exit 0, "ok: 7 report(s) checked, 1 known gap(s)"

cd tools/archive-anchor-gate && go run . --repo <worktree> --known-gaps known-gaps.txt \
     --change pi-package-hardening
   -> exit 1
   -> FAIL no verified review receipt for change "pi-package-hardening"
   *** CRITICAL: four approved receipts ARE persisted. See CRIT-1. ***
```

**Coverage**: `tools/archive-anchor-gate` 88.3% (reported by apply). Not re-measured repo-wide — informational only.

### Scratch Smoke Evidence (isolated `HOME`, stub `pi`, scratch `--state-dir`)

Live-machine safety held: `LABDRIAN_PI_BIN` pointed at a recording stub script, `HOME` was a scratch directory, no `pi install`/`pi remove` ever ran for real, and `/home/labdrian/.pi/agent/agents/` was confirmed unmodified (mtimes unchanged, no `GADU.md` created there). No `pi` session was launched.

| # | Command / scenario | Observed result |
|---|---|---|
| S1 | `pipkg build` then `stat -c %a <dest>` | `755` |
| S2 | `chmod 777 skills/roadmap-maker/SKILL.md` then `pipkg check` | exit 1, `skills/roadmap-maker/SKILL.md: mode 0644 -> 0777` |
| S3 | symlink `skills/evil-link.json` inside the package, `pipkg check` | refused: `refusing symlink at .../skills/evil-link.json` |
| S4 | registry patched to `path: "../../outside"`, `pipkg build` | exit 1, `entry "sdd-spec": path "../../outside" must not contain a ".." component`; dest dir never created; no file written outside the root |
| S5 | `jq .labdrian <dest>/package.json` | `{"builtFrom": "d1cf988612ba4fd287f508a5c24cfc211607f66d"}` == `git rev-parse HEAD` |
| S6 | `pipkg check` basis line | `pipkg check: compared against main (5aa5209b2958); package built from d1cf988612ba4fd287f508a5c24cfc211607f66d` |
| S7 | fixture: build at main, then commit a source change on main, `pipkg check` | exit 1, `package built from e4931a18b804... but main is at b843fea76418: rebuild with apply --target pi` plus `skills/pi-skill/SKILL.md: changed` |
| S8 | fixture: build on a feature branch ahead of main, `pipkg check` | exit 0, `OK`, no stale line — no false drift |
| S9 | non-git overlay root, `pipkg check` | `compared against the worktree (overlay root is not a git repository)` |
| S10 | unresolvable 40-hex `builtFrom`, `pipkg check` | `compared against main (b843fea76418); package built from 0123456789abcdef...` — deploy ref named, recorded ref disclosed, but NOT stated as unusable (see WARN-1) |
| S11 | `builtFrom: "--upload-pack=evil"`, `pipkg check` | falls back cleanly, no git error — the value never reached a git argv |
| S12 | `engine review-receipt capture --cwd <fake repo>` with one active change and an approved `review-state.json` | `1 receipt(s) captured for "alpha"`, destination byte-identical to source (`cmp` clean) |
| S13 | hook with 2 active changes and an unpersisted approved receipt | exit 2, `multiple active changes (alpha, beta); run \`review-receipt capture --change <name>\` before acknowledging` |
| S14 | `capture --change beta`, then re-run the hook | capture exit 0; hook exit 0 (documented fix-forward path, see WARN-4) |
| S15 | hook against a repo with no `openspec/changes/` | exit 0 (pass-through) |
| S16 | hook with `echo acknowledge-approved` (look-alike) | exit 0 (pass-through) |
| S17 | non-approved (`reviewing`) `review-state.json`, `capture` | `0 receipt(s) captured`, no file written |
| S18 | `engine runtime install --target pi` with the stub | recorded argv: `install <destDir>` then `install npm:pi-subagents-j0k3r`; message carries `installing third-party Pi extension pi-subagents-j0k3r (npm) required for GADU dispatch` |
| S19 | `readlink ~/.pi/agent/agents/GADU.md` (scratch HOME) | `<destDir>/agents/GADU.md`; content byte-identical to the repo's `agents/GADU.md` |
| S20 | foreign regular-file `GADU.md` present, then `install` | `exists and is not the overlay-owned symlink; leaving it untouched`; file byte-identical before/after; still a regular file |
| S21 | `runtime status --target pi` | `partial`, naming five entries separately including `Pi Subagents extension installed (not installed; ...)` AND `GADU.md linked at ~/.pi/agent/agents/GADU.md (conflict: a foreign file already exists there)` |
| S22 | `runtime uninstall --target pi` with a sibling gentle-pi managed file | `GADU.md link removed`; sibling `gentle-ai-explore.md` byte-identical; argv only `remove <destDir>` — never `remove npm:pi-subagents-j0k3r` |
| S23 | `uninstall` with a foreign `GADU.md` | `is not the overlay-owned symlink; leaving it untouched`; file survives byte-identical |
| S24 | `LABDRIAN_PI_SKIP_SUBAGENTS=1 runtime install --target pi` | `Pi Subagents extension check skipped (LABDRIAN_PI_SKIP_SUBAGENTS=1)`; argv contains no `npm:pi-subagents-j0k3r` |
| S25 | `reviewreceipt.ApprovedSummary` over all four persisted receipts | all four resolved: lineage, `final_candidate_tree`, `base_tree`, lens count, risk level |
| S26 | gate `--change demo` with the FLAT fixture shape vs the REAL nested shape | flat: exit 0 `verified review receipt found`; real: exit 1 `no verified receipt` (see CRIT-1) |

### Spec Compliance Matrix

| Requirement | Scenario | Test / evidence | Result |
|-------------|----------|-----------------|--------|
| R-001 Mode Drift Detection | Mode-only drift is reported | `TestPipkgCheck_ModeDrift`; smoke S2 | COMPLIANT |
| R-001 | Identical files report no drift | `pipkg` suite; smoke S6 (exit 0) | COMPLIANT |
| R-002 Build Root Permissions | Fresh build root is 0755 | `TestPipkgBuild_RootPermissions`; smoke S1 | COMPLIANT |
| R-003 Registry Path Containment | Traversal path is rejected | `parse_test.go` `path_traversal_rejected` (4 cases); smoke S4 | COMPLIANT |
| R-003 | In-root path builds normally | `path_in_root_passes`; smoke S1 | COMPLIANT |
| R-004 SKILL.md Name Matches Directory | Mismatch is rejected | `TestPipkgBuild_RejectsNameMismatch` | COMPLIANT |
| R-004 | Match passes | `TestPipkgBuild_LiveRegistryNamesMatch` (real registry) | COMPLIANT |
| R-005 builtFrom Recorded at Build | builtFrom equals the resolved build commit | `TestBuiltFrom_RecordedAtBuild`; smoke S5 | COMPLIANT |
| R-006 Sync-Check Compares Against Deploy Ref | Unrelated feature-branch diffs do not cause drift | `TestCheck_FeatureBranchDoesNotFalseDrift`, `TestCheck_BuildOnFeatureBranchIsNotStale`; smoke S8 | COMPLIANT |
| R-006 | Real divergence from the deploy ref is still reported | `TestCheck_StaleAfterCommittedSourceChange`; smoke S7 | COMPLIANT |
| R-007 Unresolvable Ref Discloses a Main-Only Comparison | Fallback comparison is disclosed | `TestCheck_MainFallback`; smoke S10 | PARTIAL |
| R-008 Receipt Persisted Before Acknowledge | Receipt file exists before acknowledge is invoked | `TestCapture_*`, `TestApprovedSummary`, `TestHook_*`; smoke S12–S17, S25 | COMPLIANT |
| R-009 approved_tree Sourced Only From the Persisted Receipt | approved_tree equals the receipt's final_candidate_tree | `TestApprovedTree_FromReceipt` (legacy shape only); `TestApprovedTree_FromReviewStateShape` passes only on a fabricated flat payload; smoke S26 | FAILING |
| R-010 Closure-Feedback Reads the Persisted Receipt | Post-acknowledge closure still produces a verified value | `TestApprovedTree_NeverReadFromGitTransactionStore` (proxy) + `skills/inception-pipeline/SKILL.md` prose; no executable harness; blocked in practice by CRIT-1 | PARTIAL |
| R-011 Archive Blocks Without a Receipt Unless Overridden | Missing receipt blocks archive | `TestArchiveBlock_NoReceiptPostConvention`; gate run exit 1 | COMPLIANT |
| R-011 | Recorded owner override permits archive | `TestOverride_RecordedSelfAsserted`, `TestPreArchiveFlag` | COMPLIANT |
| R-012 Subagents Extension Installed When Missing | Neither package present triggers install | `TestSubagentsExtension_InstallWhenAbsent`; smoke S18 | COMPLIANT |
| R-012 | Alternate package already installed is treated as satisfied | `TestSubagentsExtension_Noop` (4 sub-cases) | COMPLIANT |
| R-013 GADU.md Linked as an Overlay-Owned Asset | Linked content matches the current generation | `TestInstall_WiresSubagentsExtensionAndGaduLink`; smoke S19 | COMPLIANT |
| R-013 | gentle-pi's own asset management does not touch it | `TestGaduLink_SurvivesOverwrite`; smoke S22 | COMPLIANT |
| R-014 Frontmatter Verified Compatible | GADU dispatches with working tool access | `TestFrontmatter_InlineToolsScalar`, `TestInstall_RejectsAmbiguousGaduFrontmatter` guard the emitted form; actual dispatch is Phase M (manual, deferred) | PARTIAL |
| R-015 Honest, Distinct Extension and Link Status | Stale link is reported independent of extension state | `TestGaduLinkState_Matrix`; `stale` is reachable only as a broken symlink target, never as "content predates body.md" (DEV-3) | PARTIAL |
| R-015 | Missing extension does not collapse link status | `TestStatus_ReportsUnprovenSubagentsAndGaduLinkEntries`; smoke S21 | COMPLIANT |
| R-016 Uninstall Removes Only the Overlay-Owned Link | Only GADU.md is removed | `TestUninstall_OwnedLinkOnly`, `TestUninstall_LeavesForeignGaduFileUntouched`; smoke S22, S23 | COMPLIANT |
| PRT Package-Delivered Skills and Agents | Local-path install registers the package idempotently | `engine/runtime` pi suite; smoke S18 argv | COMPLIANT |
| PRT | Skills are visible in a Pi session and agents ship as package content | `engine/runtime` pi suite; smoke S18/S19 — only `GADU.md` written under `~/.pi/agent/agents/` | COMPLIANT |
| PRT Honest Status for Unproven Activation | Unproven entry forces partial | smoke S21 (five entries named) | COMPLIANT |
| PRT | All entries proven report supported | `TestPiAdapter_StatusTriangulatesAllOwnedEntries/all_five_entries_proven_reports_supported` | COMPLIANT |
| PRT Pi-Scoped Uninstall | Only the owned package entry is removed | `TestUninstall_OwnedLinkOnly`; smoke S22 argv `remove <destDir>` | COMPLIANT |
| PRT | GADU link is removed without touching the extension package | smoke S22 — no `remove npm:pi-subagents-j0k3r` in argv | COMPLIANT |
| PRT Pi Drift Detection via Sync-Check | No drift is reported when unchanged | smoke S6 | COMPLIANT |
| PRT | A source edit is detected as drift | smoke S7 | COMPLIANT |
| PRT | Feature-branch checkout does not cause false drift | smoke S8; `TestCheck_BuildOnFeatureBranchIsNotStale` | COMPLIANT |
| AI Boundary Anchors | Anchors resolve and are legible in both stores | the verified path requires the gate to read the persisted receipt; it cannot on the real shape (CRIT-1) | FAILING |
| AI | A missing receipt blocks archive unless an owner override is recorded | `TestArchiveBlock_NoReceiptPostConvention`, `TestOverride_RecordedSelfAsserted`; gate run exit 1 | COMPLIANT |
| AI | An unrelated change touching the folder does not become the anchor | `TestAbsenceProseElsewhereDoesNotSilenceARealLandingCommit`, `TestLabelledCommitIsStillReadWhenAbsenceProseIsPresent` | COMPLIANT |
| AI | A mis-recorded anchor is rejected, not trusted | `TestScanArchiveRejectsAnUndisclosedTreeMismatch`, `TestScanArchiveAcceptsADisclosedRejection` | COMPLIANT |
| AI | A change predating the convention omits rather than guesses | `TestScanArchiveIgnoresReportsPredatingTheConvention`; gate run reports `anchor absent` for 2026-09-02 | COMPLIANT |
| AI | A change that skipped inception-pipeline still measures | `skills/inception-pipeline/SKILL.md` prose only; no executable harness | PARTIAL |
| AI | Neither anchor resolves | `skills/inception-pipeline/SKILL.md` prose only; no executable harness | PARTIAL |

**Compliance summary**: 32/40 scenarios compliant, 2 failing, 6 partial. 15/21 requirements fully compliant.

### Correctness (Static Evidence)

| Requirement | Status | Notes |
|------------|--------|-------|
| R-001..R-004 | Implemented | `fileEntry{data,perm}`, `Chmod(tmpDir,0755)`, `validateEntry` containment, `checkSkillNameMatchesPath` |
| R-005..R-007 | Implemented | `labdrianField{BuiltFrom}`, `CheckReport{Basis,DeployRef,DeployTip,BuiltFrom}`, `Disclosure()`, `git archive` + `archive/tar` export |
| R-008 | Implemented | `Capture`, `DetectActiveChange`, `ApprovedSummary`, `RunHook`, fourth settings identity |
| R-009 | Defective | `persistedReviewState` models a flat payload gentle-ai never writes (CRIT-1) |
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
| D6 comparison basis | Superseded, then followed | the design's "compare against builtFrom" was superseded during review; `Check` now resolves the deploy ref (`main` -> `origin/main` -> `HEAD`, or `LABDRIAN_PI_DEPLOY_REF`), treats `builtFrom` as provenance, and reports `Stale` when `builtFrom` is strictly behind the deploy tip. Verified at runtime (smoke S7/S8). Basis values are `deploy`/`worktree`/`dirty`, not the design's `ref`/`main`/`worktree` — a rename consistent with the superseding decision. |
| D7 receipt capture | Mostly | fail-closed hook, atomic byte-identical write, third/fourth settings identity — all present. Two disclosed deviations (DEV-1, DEV-2) plus one undisclosed refinement (WARN-4). |
| D8 receipt consumption | Broken for the live shape | dual-shape dispatch exists but the review-state branch parses the wrong nesting (CRIT-1) |
| D9 missing receipt | Yes | `ReceiptConventionDate = "2026-09-12"`, override schema, `--change` flag, documented gate |
| D10 GADU placement | Yes | symlink; ownership proven by `Readlink` equality; no state file |
| D11 extension probe/install | Yes | prefix match on both package names, fixed argv, disclosure, skip env |
| D12 frontmatter | Yes | `tools: '*'` inline scalar emitted and guarded |
| D13 status/uninstall | Partially | four link states exist and are independently reported; `stale` is narrower than the design's parenthetical (DEV-3) |

### Deviation Rulings

| # | Deviation (disclosed in apply-progress) | Ruling |
|---|---|---|
| DEV-1 | `DetectActiveChange` does not require `state.yaml` (this repo never writes one); it accepts any non-`archive` dir carrying `tasks.md`/`design.md`/`proposal.md`/`entry.json` | **ACCEPTABLE — and the spec is the thing that is wrong.** Requiring `state.yaml` would make R-008 a permanent no-op here. Recorded as a spec gap: D7's `state.yaml` wording should be corrected to "artifact-bearing" before archive. |
| DEV-2 | Fail-closed hook guard (`command -v <bin> \|\| exit 0`) instead of the fail-safe `\|\| true` used by the other three families | **ACCEPTABLE.** R-008 requires fail-closed; the deviation is from sibling convention, not from the spec. Verified at runtime: exit 2 propagates (smoke S13). |
| DEV-3 | Symlink ownership collapses the "stale" state: `stale` is reachable only when the link target file is gone, never when linked content predates `body.md` | **SPEC GAP (WARNING).** The intent — honest, non-collapsed status — is met, and a stale *deployment* is still caught by `pipkg check` drift on `agents/GADU.md`. But R-015's scenario as written ("GADU.md predates the current body.md ... link state SHALL read `stale`") is unsatisfiable under D10's stable-path symlink. Reword the scenario or change the design; do not leave a scenario no implementation can satisfy. |
| DEV-4 | Task 4.4's "closure-feedback test" implemented as a gate-side Go test, not a test of agent prose | **ACCEPTABLE.** closure-feedback has no harness in this repo; the proxy asserts the real invariant (never falls back to the live git store). Residual risk recorded as WARN-2. |
| DEV-5 | Four extra RED tests written then removed to fit budget; scenarios verified ad hoc in scratch fixtures | **ACCEPTABLE but regrettable.** Their scenarios are covered by remaining tests plus the `AnchorRejected` message fix. No assigned test was removed. |
| DEV-6 | `Uninstall()` unlinks `GADU.md` before `pi remove`; `writeStubPiScript` recorder changed to append; two pre-existing tests gained isolated `HOME` | **ACCEPTABLE.** The `HOME` isolation fix is load-bearing and correct. |
| DEV-7 | Slice 5 added no `bin/labdrian-overlay` edits; wiring is transitive through `engine runtime install/status --target pi` | **ACCEPTABLE.** Verified at runtime (smoke S18–S24). |

### TDD Compliance

| Check | Result | Details |
|-------|--------|---------|
| TDD Evidence reported | PASS | TDD Cycle Evidence tables present for slices 1–4 in `apply-progress.md`; slice 5's evidence is in the Engram `sdd/pi-package-hardening/apply-progress` revision only (WARN-5) |
| All tasks have tests | PASS | every 1.x–5.x task names a concrete test file/function; all exist |
| RED confirmed (tests exist) | PASS | all named test files verified present; RED states recorded as compile failures or `want error, got nil` |
| GREEN confirmed (tests pass) | PASS | all named tests re-executed and passing (208 `--- PASS` across the five changed packages) |
| Triangulation adequate | PASS | 4 traversal cases, 4 extension no-op cases, 5 link-state cases, 3 non-hex builtFrom cases, 3 basis cases |
| Safety Net for modified files | PASS | pre-existing suites recorded green before each modification |

**TDD Compliance**: 6/6 checks passed.

### Test Layer Distribution

| Layer | Tests | Files | Tools |
|-------|-------|-------|-------|
| Unit | 61 | 6 | `go test` |
| Integration (git-in-TempDir, scratch `HOME` + stub `pi`, real `bash`) | 16 | 3 | `go test`, `git`, `bash` |
| E2E | 0 | 0 | not installed (deferred to Phase M) |
| **Total** | **77** | **9** | |

### Changed File Coverage

`tools/archive-anchor-gate`: 88.3% (as reported by apply). Repo-wide per-file coverage not re-measured — informational, non-blocking.

### Assertion Quality

No tautologies, no orphan empty-collection checks, no ghost loops, no smoke-test-only patterns found across the change's nine test files. Assertion density is healthy (e.g. `engine/runtime/pi_test.go`: 22 tests / 95 assertions; `engine/skills/parse_test.go`: 88 assertions).

One quality defect, which is the root of CRIT-1: `tools/archive-anchor-gate/receipt_test.go:39` `writeReviewStateFile` fabricates a payload shape gentle-ai has never written. The test passes, the production path fails. This is a fixture-fidelity failure, not a trivial assertion — but it is exactly the class of green test that proves nothing.

**Assertion quality**: 0 CRITICAL, 1 WARNING (WARN-3).

### Quality Metrics

**Linter (`go vet`)**: no errors (engine, longterm-mem, archive-anchor-gate).
**Formatter (`gofmt -l`)**: clean.
**Shell (`shellcheck -S warning`)**: 2 pre-existing SC2064 warnings at `bin/labdrian-overlay:1458` and `:1615`. No new findings.

### Issues Found

**CRITICAL**

- **CRIT-1 — `archive-anchor-gate` cannot read the review-state receipts it is supposed to verify (R-009, R-010, and the actuals "verified" path).**
  `tools/archive-anchor-gate/receipt.go:49` declares:

  ```go
  type persistedReviewState struct {
      State           string   `json:"state"`
      LineageID       string   `json:"lineage_id"`
      SelectedLenses  []string `json:"selected_lenses"`
      RiskLevel       string   `json:"risk_level"`
      CurrentSnapshot struct{ CandidateTree string `json:"candidate_tree"` } `json:"current_snapshot"`
      InitialSnapshot struct{ BaseTree string `json:"base_tree"` } `json:"initial_snapshot"`
  }
  ```

  What gentle-ai 2.7.0 actually persists is a `gentle-ai.review-state-record/v2` wrapper whose top-level `state` is an **object**, with `lineage_id`, `selected_lenses`, `risk_level`, `current_snapshot` and `initial_snapshot` nested one level inside it. `json.Unmarshal` therefore fails on `State string` and `approvedTreeFromReviewState` returns `ok == false` for every real file.

  Consequences, all observed:
  - `go run . --repo <worktree> --known-gaps known-gaps.txt --change pi-package-hardening` exits **1** with `no verified review receipt`, despite four approved receipts sitting in `openspec/changes/pi-package-hardening/review-receipts/`.
  - The same command is the pre-archive Gate Compliance bullet this change itself added to `skills/inception-pipeline/SKILL.md:130`, so **this change cannot pass its own archive gate**.
  - R-009's `approved_tree` can never be sourced from a real receipt, so the "verified" anchor outcome in `actuals-instrumentation` is unreachable; every post-convention archive would fall to "blocked" or "self-asserted with a recorded override".
  - `engine/reviewreceipt.ApprovedSummary` gets the **same four files right** (smoke S25) — the two packages disagree about the on-disk shape, and only the gate is wrong. `engine/reviewreceipt/reviewreceipt.go:101` models the nested shape correctly.

  Fix: make the gate parse the record wrapper (nested `state` object), or better, delete `persistedReviewState` and call `reviewreceipt.ApprovedSummary`, which is already the single correct reader and is what the spec's R-008 scenario names. Then replace `writeReviewStateFile`'s fabricated payload with a real captured file (one of the four in this tree is a ready-made fixture).

**WARNING**

- **WARN-1 (R-007)** — `CheckReport.Disclosure()` (`engine/pipkg/pipkg.go:226`) has no branch for an unresolvable `builtFrom`. Output is identical whether the recorded ref resolves or not, so a reader cannot tell. The scenario requires stating "the recorded build ref could not be used as provenance". `TestCheck_MainFallback` only asserts the value is disclosed, not that it is flagged. Scenario PARTIAL.
- **WARN-2 (R-010)** — closure-feedback is agent-executed prose with no harness; compliance rests on a gate-side proxy test plus documentation. Acceptable given the repo's shape, but the scenario is not runtime-proven.
- **WARN-3** — `receipt_test.go`'s `writeReviewStateFile` fixture does not match reality; this is what let CRIT-1 ship green. Any future receipt-shape test must be seeded from a real captured file.
- **WARN-4 (D7)** — `RunHook` has an **undisclosed** refinement beyond D7: with multiple active changes it allows (exit 0) when `AllSurvivingApprovedPersisted` is true, rather than always denying. The behaviour is safe and well-documented in code, and verified (smoke S13 denies, S14 allows after explicit capture), but it is a design change that `apply-progress.md` never lists under Deviations from Design.
- **WARN-5** — `openspec/changes/pi-package-hardening/apply-progress.md` stops at Slice 4. Slice 5's progress, TDD evidence and deviations exist **only** in the Engram copy. Under hybrid mode both stores must carry the artifact; the OpenSpec file is the one that travels into the archive, so slice 5's evidence would be lost at archive time.
- **WARN-6** — `apply-progress.md`'s slice 2/3/4 sections still read "NOT committed / blocked on budget" with landing notes appended. All four did land. The narrative is confusing for a future reader; the Engram copy is accurate.
- **WARN-7 (R-015)** — see DEV-3: the stale-link scenario is unsatisfiable as written.
- **WARN-8 (R-014)** — actual GADU dispatch through `subagent_*` is Phase M (manual, deferred). The frontmatter *form* is verified; the *dispatch* is not. Not a failure — deferred by design — but the scenario is not runtime-proven.

**SUGGESTION**

- Four receipts are persisted for five slices. Slice 1's lineage has no receipt (it predates the capture path). Harmless, but the count is worth a line in the archive report so a reader does not go looking for a fifth.
- `pipkg check` echoes a raw `builtFrom` value (e.g. `--upload-pack=evil`) verbatim into operator-facing output. It never reaches a git argv, but consider quoting or eliding a value that fails `builtFromPattern`.
- Consider making `ReceiptConventionDate` reachable in `--change` mode as a nearer-term guard, and adding a gate self-test that runs against the repository's own real `review-receipts/` directory — that single test would have caught CRIT-1.

### Verdict

**FAIL** — 1 CRITICAL: `archive-anchor-gate` parses a review-state shape gentle-ai never writes, so R-009 and the verified-anchor path are inoperative and this change cannot pass the pre-archive gate it introduced; 8 WARNING, 3 SUGGESTION. Everything else — all 42 automated tasks, the full test suite, and 32/40 scenarios — is green and runtime-proven.
