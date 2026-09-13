```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:6aa546e2cb9869035db1b8217d79820b54cca5bd8901d0f08b4a0fb4fe026e57
verdict: fail
blockers: 0
critical_findings: 0
requirements: 17/21
scenarios: 35/41
test_command: cd engine && go test -count=1 -race ./... && cd ../tools/archive-anchor-gate && go test -count=1 ./... && cd ../../longterm-mem && go test -count=1 ./...
test_exit_code: 0
test_output_hash: sha256:4501f81aba2327010555fe970f9433774442565600fda0411133407c1ebcd309
build_command: cd engine && gofmt -l . && go vet ./... && cd ../tools/archive-anchor-gate && gofmt -l . && go vet ./... && cd ../../longterm-mem && go vet ./...
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report

**Change**: pi-package-hardening
**Version**: N/A (delta specs)
**Mode**: Strict TDD
**Pass**: 3 (post-merge, post-Phase-M re-verification)
**Worktree**: `/home/labdrian/labdrian-sdd-overlay` @ `d385311` on `docs/pi-package-hardening-closure` (main `1952772` plus the manual-evidence commit)
**Supersedes**: pass-2 report `evidence_revision: sha256:3ba47acf19c8582029ef4a23b12c07ff48f5fb3ddad007f11dcebb9f77aa4748` (verdict `fail`, 0 CRITICAL, 5 WARNING)

`evidence_revision` is `sha256(HEAD tree `c31cbad3762a3c1597a6f08be455cf28b6ce70b9` + build_output_hash + test_output_hash, newline-separated)`, recomputed this pass.

### What changed since pass 2

All five slices are merged (PRs #321–#326); `main` is at `1952772`. The working tree is clean. Three things moved since the pass-2 report was written:

1. **PR #326** landed the CRIT-1 remediation with the review-state fixture pinned as real captured data in `tools/archive-anchor-gate/testdata/real-review-state-record-v2.json` (48 KB, byte-for-byte copy of a production receipt), and `.gitignore:27` gained `/tools/*/archive-anchor-gate`.
2. **`ac87563`** persisted the fifth review receipt (`review-6e01e9510e00f068`).
3. **`d385311`** recorded live-Pi manual verification: `apply-progress.md` gained a "Manual Verification (Phase M, live Pi 0.85.1, 2026-09-13)" section and `tasks.md` M.1–M.3 are now `[x]`.

### Completeness

| Metric | Value |
|--------|-------|
| Tasks total (automated, 1.x–5.x) | 42 |
| Tasks complete | 42 |
| Tasks incomplete | 0 |
| Manual tasks (Phase M) | 3 (M.1–M.3) — **all now run and checked `[x]`** |
| Planned slices (P, from `entry.json` `review_slices`) | 5 (`pipkg-integrity`, `sync-check-provenance`, `review-receipt-capture`, `receipt-anchor-gate`, `gadu-pi-subagent`) |
| Realized slices (R, from `apply-progress.md`) | 5 |
| Plan vs realized drift | None — every planned slice has a landed, merged counterpart |
| Review receipts persisted | 5 (was 4 at pass 2) |

**Phase M is discharged as an obligation.** Pass 2 recorded M.1–M.3 as unrun and open (WARN-8). They have now been executed against the real machine and recorded. Every task in `tasks.md` is checked; zero unchecked tasks remain.

### Build & Tests Execution

**Build**: PASS — exit 0, output empty (`sha256` of the empty string, as expected).

```text
cd engine                    && gofmt -l .   -> empty, exit 0
cd engine                    && go vet ./... -> no output, exit 0
cd tools/archive-anchor-gate && gofmt -l .   -> empty, exit 0
cd tools/archive-anchor-gate && go vet ./... -> no output, exit 0
cd longterm-mem              && go vet ./... -> no output, exit 0
```

`go build ./...` inside `tools/archive-anchor-gate` was deliberately NOT run this pass: it drops a 4 MB binary next to its source. The module's compilability is proven instead by `go test -count=1 ./...` and two `go run .` invocations, all exit 0 — the same linker work, without the artifact.

**Tests**: PASS — 32 packages `ok`, 0 failed, 0 skipped-as-failure, exit 0.

```text
cd engine && go test -count=1 -race ./...              -> 14 packages ok, exit 0
cd tools/archive-anchor-gate && go test -count=1 ./... -> ok 0.674s, exit 0 (59 PASS lines incl. subtests)
cd longterm-mem && go test -count=1 ./...              -> 17 packages ok, exit 0
shellcheck -S warning bin/labdrian-overlay             -> exit 1, 2 findings
   both pre-existing SC2064 at lines 1458 and 1615; byte-identical to passes 1 and 2; no new findings
```

### Gate runs (pre-archive gate, re-run against merged `main`)

```text
cd tools/archive-anchor-gate && go run . --repo /home/labdrian/labdrian-sdd-overlay --known-gaps known-gaps.txt
   -> exit 0, "ok: 7 report(s) checked, 1 known gap(s)"
      (1 verified, 5 self-asserted, 1 absent; the 2026-09-02 longterm-mem gap is the single declared known gap)

cd tools/archive-anchor-gate && go run . --repo /home/labdrian/labdrian-sdd-overlay --known-gaps known-gaps.txt \
     --change pi-package-hardening
   -> exit 0
   -> verified review receipt found for "pi-package-hardening"
      (approved_tree=ec56969bdb2a7ade90b83e47d971beb857ddadab)
```

The change passes the pre-archive gate it introduced, now against the merged repository rather than a slice worktree. The pass-2 reversion proof (pre-fix gate binary exits 1 on the same receipts) stands unchallenged and is not re-run.

### Manual Verification (Phase M) — evidence and ruling

`apply-progress.md` records three live-machine checkpoints against Pi 0.85.1. This session may not launch `pi`, so the Phase M results are **reported manual evidence**, corroborated by the read-only observations below, not re-executed.

| Checkpoint | Reported result | Independent read-only corroboration this pass |
|---|---|---|
| M.1 real `apply --target pi` | installed `npm:pi-subagents-j0k3r` with third-party disclosure, rebuilt the package, linked `GADU.md`; `status --target pi` reported `supported` naming all entries; gentle-pi `settings.json` (minus `packages`) and pi-engram `mcp.json` byte-identical | `readlink ~/.pi/agent/agents/GADU.md` → `/home/labdrian/.labdrian-overlay/pi/labdrian-pi/agents/GADU.md` (overlay-owned target, as D10 specifies). `settings.json` `packages` contains both `npm:pi-subagents-j0k3r` and `../../.labdrian-overlay/pi/labdrian-pi`, alongside four pre-existing foreign packages left intact |
| M.2 GADU dispatch | headless session lists `gadu` in `subagent_list_agents` beside gentle-pi's agents and exposes the `subagent_*` tools; `subagent_run gadu` fails "provider auth error"; control run of gentle-pi's own `gentle-ai-explore` fails identically | `ls ~/.pi/agent/agents` → `GADU.md` present beside 22 gentle-pi agent files. Issue **#327** confirmed OPEN: "pi: subagent_run fails with provider auth error when the parent uses the Claude bridge" |
| M.3 uninstall scope | removed only the overlay link and package; extension, every gentle-pi agent file, `settings.json` (minus our entry), and `mcp.json` unchanged | The live machine is currently in the re-installed state, so M.3's removal is not observable now; its non-destructive scope is independently proven by `TestUninstall_OwnedLinkOnly` and `TestUninstall_LeavesForeignGaduFileUntouched`, both green this pass |

**M.2's ruling, stated plainly.** The control run is what makes this evidence load-bearing: `subagent_run` fails identically for gentle-pi's *own* agent, so the failure is not a property of our GADU.md, our frontmatter, or our symlink. It is the extension spawning a child on a real provider (`default_model` `provider/model-id`) on a machine whose parent session runs through the pi-claude-bridge with no provider key. That is an environment gap, filed as #327, not an overlay defect.

What this **does** newly prove, and pass 2 explicitly could not: the real Pi Subagents extension read our real overlay-linked `GADU.md`, accepted it, and registered `gadu` as a dispatchable agent. Pass 2's stated residual — "'the extension parses our frontmatter' remains an inference from a Go port of the extension's parser" — is now a direct observation. What it still does not prove is the second half of R-014's THEN clause: that the dispatched agent receives a non-empty, correctly-expanded tool set. No child ever started, so no tool set was ever observed.

### Spec Compliance Matrix

Authoritative counts, recounted this pass from the six delta spec files: **21 requirements, 41 scenarios** (`rg -c '^### (Requirement:|REQ-[0-9]+:)'` → 4+3+4+1+5+4 = 21; `rg -c '^#### Scenario:'` → 9+4+7+7+8+6 = 41). Unchanged from pass 2.

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
| R-009 approved_tree Sourced Only From the Persisted Receipt | approved_tree equals the receipt's final_candidate_tree | `TestApprovedTree_FromReceipt`, `TestApprovedTree_FromReviewStateShape`, `TestApprovedTree_FromRealCapturedReceipt` (now reading committed `testdata/real-review-state-record-v2.json`); live gate run exit 0 naming `approved_tree=ec56969b…` | COMPLIANT |
| R-010 Closure-Feedback Reads the Persisted Receipt | Post-acknowledge closure still produces a verified value | `TestApprovedTree_NeverReadFromGitTransactionStore` (proxy) + `skills/inception-pipeline/SKILL.md` prose; no executable harness | PARTIAL (WARN-2) |
| R-011 Archive Blocks Without a Receipt Unless Overridden | Missing receipt blocks archive | `TestArchiveBlock_NoReceiptPostConvention`; pass-2 pre-fix binary reproduced the exact block message | COMPLIANT |
| R-011 | Recorded owner override permits archive | `TestOverride_RecordedSelfAsserted`, `TestPreArchiveFlag/override_present_passes` | COMPLIANT |
| R-012 Subagents Extension Installed When Missing | Neither package present triggers install | `TestSubagentsExtension_InstallWhenAbsent`; smoke S18; **M.1 live install observed** | COMPLIANT |
| R-012 | Alternate package already installed is treated as satisfied | `TestSubagentsExtension_Noop` (4 sub-cases) | COMPLIANT |
| R-013 GADU.md Linked as an Overlay-Owned Asset | Linked content matches the current generation | `TestInstall_WiresSubagentsExtensionAndGaduLink`; smoke S19; **live `readlink` confirms the overlay-owned target** | COMPLIANT |
| R-013 | gentle-pi's own asset management does not touch it | `TestGaduLink_SurvivesOverwrite`; smoke S22; **live `ls ~/.pi/agent/agents` shows GADU.md intact beside 22 gentle-pi files** | COMPLIANT |
| R-014 Frontmatter Verified Compatible | GADU dispatches with working tool access | `TestFrontmatter_InlineToolsScalar`, `TestInstall_RejectsAmbiguousGaduFrontmatter` guard the emitted form; **M.2 observed the real extension parse our file and register `gadu`**; tool-set expansion unobserved — `subagent_run` blocked by provider auth (#327), identically for gentle-pi's own agent | PARTIAL (WARN-8) |
| R-015 Honest, Distinct Extension and Link Status | Stale link is reported independent of extension state | `TestGaduLinkState_Matrix/stale (broken target)` | COMPLIANT |
| R-015 | Missing extension does not collapse link status | `TestStatus_ReportsUnprovenSubagentsAndGaduLinkEntries`; smoke S21 | COMPLIANT |
| R-016 Uninstall Removes Only the Overlay-Owned Link | Only GADU.md is removed | `TestUninstall_OwnedLinkOnly`, `TestUninstall_LeavesForeignGaduFileUntouched`; smoke S22, S23; **M.3 live uninstall left the extension and every foreign agent untouched** | COMPLIANT |
| PRT Package-Delivered Skills and Agents | Local-path install registers the package idempotently | `engine/runtime` pi suite; smoke S18 argv; **live `settings.json` `packages` carries the overlay entry once** | COMPLIANT |
| PRT | Skills are visible in a Pi session and agents ship as package content | `engine/runtime` pi suite; smoke S18/S19; **M.1 live: only `GADU.md` written under `~/.pi/agent/agents/`** | COMPLIANT |
| PRT Honest Status for Unproven Activation | Unproven entry forces partial | smoke S21 (five entries named separately) | COMPLIANT |
| PRT | All entries proven report supported | `TestPiAdapter_StatusTriangulatesAllOwnedEntries/all_five_entries_proven_reports_supported`; **M.1 live `status --target pi` reported `supported` naming every entry** | COMPLIANT |
| PRT Pi-Scoped Uninstall | Only the owned package entry is removed | `TestUninstall_OwnedLinkOnly`; smoke S22 argv `remove <destDir>`; **M.3 live** | COMPLIANT |
| PRT | GADU link is removed without touching the extension package | smoke S22 — no `remove npm:pi-subagents-j0k3r` in argv; **M.3 live: extension survived** | COMPLIANT |
| PRT Pi Drift Detection via Sync-Check | No drift is reported when unchanged | smoke S6 | COMPLIANT |
| PRT | A source edit is detected as drift | smoke S7 | COMPLIANT |
| PRT | Feature-branch checkout does not cause false drift | smoke S8; `TestCheck_BuildOnFeatureBranchIsNotStale` | COMPLIANT |
| AI Boundary Anchors | Anchors resolve and are legible in both stores | gate half runtime-proven (exit 0, verified `approved_tree`); the t0/Engram + archive-report half is agent prose with no harness | PARTIAL (WARN-2) |
| AI | A missing receipt blocks archive unless an owner override is recorded | `TestArchiveBlock_NoReceiptPostConvention`, `TestOverride_RecordedSelfAsserted` | COMPLIANT |
| AI | An unrelated change touching the folder does not become the anchor | `TestAbsenceProseElsewhereDoesNotSilenceARealLandingCommit`, `TestLabelledCommitIsStillReadWhenAbsenceProseIsPresent` | COMPLIANT |
| AI | A mis-recorded anchor is rejected, not trusted | `TestScanArchiveRejectsAnUndisclosedTreeMismatch`, `TestScanArchiveAcceptsADisclosedRejection` | COMPLIANT |
| AI | A change predating the convention omits rather than guesses | `TestScanArchiveIgnoresReportsPredatingTheConvention`; gate run reports `anchor absent` for 2026-09-02 | COMPLIANT |
| AI | A change that skipped inception-pipeline still measures | `skills/inception-pipeline/SKILL.md` prose only; no executable harness | PARTIAL (WARN-2) |
| AI | Neither anchor resolves | `skills/inception-pipeline/SKILL.md` prose only; no executable harness | PARTIAL (WARN-2) |

**Compliance summary**: 35/41 scenarios COMPLIANT, 0 FAILING, 0 UNTESTED, 6 PARTIAL. 17/21 requirements fully compliant. The 4 requirements short of full are R-007 (WARN-1), R-010 (WARN-2), R-014 (WARN-8), and the `actuals-instrumentation` boundary-anchor requirement (WARN-2).

**Why R-014 stays PARTIAL and was not promoted.** Its THEN clause has two halves: "no parse error" *and* "access to the tools the frontmatter declares, with no silently-empty tool set". M.2 observed the first directly and could not observe the second, because no child process ever started. Promoting it to COMPLIANT would mean reading "the extension listed the agent" as "the agent had working tools" — an inference, and precisely the kind of inference pass 2 flagged. The honest position is that R-014 is now *better evidenced and environment-blocked* rather than *unrun*, which is a real improvement that does not reach compliance.

### Correctness (Static Evidence)

| Requirement | Status | Notes |
|------------|--------|-------|
| R-001..R-004 | Implemented | `fileEntry{data,perm}`, `Chmod(tmpDir,0755)`, `validateEntry` containment, `checkSkillNameMatchesPath` |
| R-005..R-006 | Implemented | `labdrianField{BuiltFrom}`, `CheckReport{Basis,DeployRef,DeployTip,BuiltFrom}`, `git archive` + `archive/tar` export |
| R-007 | Implemented with a gap | `Disclosure()` (`engine/pipkg/pipkg.go:226`) re-read this pass and unchanged: the `deploy` branch returns `"compared against " + DeployRef + " (" + DeployTip + "); package built from " + builtFrom`, varying only when `BuiltFrom == ""` → `"unrecorded"`. A well-formed but unresolvable SHA renders identically to a resolvable one (WARN-1) |
| R-008 | Implemented | `Capture`, `DetectActiveChange`, `ApprovedSummary`, `RunHook` + `AllSurvivingApprovedPersisted`, fourth settings identity |
| R-009 | Fixed and merged | gate's private `persistedReviewState` deleted; `loadApprovedTreeFromReceipts` delegates to `reviewreceipt.ApprovedSummary`; fixture is now committed production data |
| R-010 | Implemented (docs) | `skills/inception-pipeline/SKILL.md` reads the persisted file, both shapes; `.git/...` path removed from the Plan |
| R-011 | Implemented | `ReceiptConventionDate`, `loadReceiptOverride`, `CheckPreArchive`, `--change` flag, Gate Compliance bullet |
| R-012..R-016 | Implemented and live-confirmed | `isSubagentsExtensionListed`, `ensureSubagentsExtension`, `gaduLinkState`, `linkGaduAgent`, `unlinkGaduAgent`, `validateGaduFrontmatter`; M.1/M.3 exercised the install and uninstall paths on a real Pi |

### Coherence (Design)

| Decision | Followed? | Notes |
|----------|-----------|-------|
| D1 mode drift | Yes | `copyFile` still writes 0644; symlinked dest root refused |
| D2 build root 0755 | Yes | `os.Chmod(tmpDir, 0755)` after `MkdirTemp` |
| D3 path containment | Yes | `validateEntry` + `filepath.Rel` defense in depth |
| D4 SKILL.md name in pipkg | Yes | line-oriented scan, zero-dependency ADR preserved |
| D5 provenance | Yes | `resolveBuildRev` + `resolvePackageVersion(root, rev)` |
| D6 comparison basis | Superseded, then followed | `Check` resolves the deploy ref and treats `builtFrom` as provenance; runtime-proven (S7/S8) |
| D7 receipt capture | Yes | DEV-1 corrected in `eaaac7e`; multi-change allow path is a named scenario with two covering tests |
| D8 receipt consumption | Yes | single correct reader; verified against all real receipts, now including a committed production fixture |
| D9 missing receipt | Yes | `ReceiptConventionDate = "2026-09-12"`, override schema, `--change` flag, documented gate |
| D10 GADU placement | Yes | symlink; **live `readlink` confirms the overlay-owned target on the real machine**; no state file |
| D11 extension probe/install | Yes | prefix match on both package names, fixed argv, disclosure, skip env; M.1 observed the disclosure |
| D12 frontmatter | Yes | `tools: '*'` inline scalar emitted and guarded; **the real extension accepted it and registered `gadu`** |
| D13 status/uninstall | Yes | DEV-3 resolved by reworking the spec; M.1/M.3 confirm status honesty and uninstall scope live |

### Deviation Rulings (pass 3)

| # | Pass-2 status | Pass-3 status |
|---|---|---|
| DEV-1 | CLOSED | Unchanged |
| DEV-2 | ACCEPTABLE (fail-closed is the spec's requirement) | Unchanged |
| DEV-3 | CLOSED | Unchanged |
| DEV-4 | ACCEPTABLE (gate-side proxy for agent prose) | Unchanged; residual risk remains WARN-2 |
| DEV-5 | ACCEPTABLE but regrettable | Unchanged |
| DEV-6 | ACCEPTABLE (`HOME` isolation is load-bearing) | Unchanged |
| DEV-7 | ACCEPTABLE (wiring is transitive) | Unchanged |

### TDD Compliance

| Check | Result | Details |
|-------|--------|---------|
| TDD Evidence reported | PASS | TDD Cycle Evidence tables present in `apply-progress.md` for slices 1–5 and Remediation 1 |
| All tasks have tests | PASS | all 31 task-named test functions verified present by name; zero missing |
| RED confirmed (tests exist) | PASS | Remediation 1's RED independently reproduced at pass 2 by rebuilding the pre-fix gate binary (exit 1 on the same real receipts where HEAD exits 0); the fixture it needs is now committed under `testdata/` |
| GREEN confirmed (tests pass) | PASS | all named tests re-executed at `-count=1` this pass; 32 packages `ok`, 59 PASS lines in the gate module alone |
| Triangulation adequate | PASS | 4 traversal cases, 4 extension no-op cases, 5 link-state cases, 3 non-hex builtFrom cases, 4 `TestPreArchiveFlag` sub-cases, legacy + nested + real-production receipt shapes |
| Safety Net for modified files | PASS | Remediation 1 records the full gate suite green pre-fix; earlier slices record pre-existing suites green before each modification |

**TDD Compliance**: 6/6 checks passed.

### Test Layer Distribution

| Layer | Tests | Files | Tools |
|-------|-------|-------|-------|
| Unit | 62 | 6 | `go test` |
| Integration (git-in-TempDir, scratch `HOME` + stub `pi`, real `bash`, committed real-production-data fixture) | 17 | 3 | `go test`, `git`, `bash` |
| Manual / live-system (Phase M) | 3 | — | real Pi 0.85.1, real `~/.pi` |
| E2E (automated) | 0 | 0 | not installed |
| **Total (automated)** | **79** | **9** | |

Phase M is listed as its own layer rather than folded into E2E: it is real-system evidence, but it is human-executed and unrepeatable by CI, so it must not be counted as automated coverage.

### Changed File Coverage

`tools/archive-anchor-gate`: 88.3% (as reported by apply; CI re-measures via `go test ./... -cover`). Repo-wide per-file coverage not re-measured — informational, non-blocking.

### Assertion Quality

Re-confirmed the pass-1 and pass-2 audits across all nine test files. No tautologies, no orphan empty-collection checks, no ghost loops, no smoke-test-only patterns, no mock-heavy files. Pass 1's single defect (a fabricated review-state payload shape) is fixed twice over: the helper emits the real nested `gentle-ai.review-state-record/v2` wrapper, and `TestApprovedTree_FromRealCapturedReceipt` now reads a committed 48 KB byte-for-byte copy of a production receipt from `testdata/`, so the suite cannot drift back into agreeing only with itself.

**Assertion quality**: 0 CRITICAL, 0 WARNING.

### Quality Metrics

**Linter (`go vet`)**: no errors (engine, longterm-mem, archive-anchor-gate).
**Formatter (`gofmt -l`)**: clean (engine, archive-anchor-gate).
**Shell (`shellcheck -S warning`)**: 2 pre-existing SC2064 warnings at `bin/labdrian-overlay:1458` and `:1615`. No new findings; `bin/labdrian-overlay` is unchanged since pass 1.

### Issues Found

**CRITICAL**

None.

**WARNING**

- **WARN-1 (R-007) — unresolvable `builtFrom` is disclosed but not flagged as unusable. Ruling: FOLLOW-UP, not blocking.** Re-read `Disclosure()` this pass: unchanged. R-007's requirement body demands the output state "that the recorded build ref could not be used as provenance"; the `deploy` branch never says that, so a well-formed-but-unresolvable SHA prints the same sentence as a resolvable one. The scenario's own THEN clause is satisfied and the operator is never misled about *what was compared*, so no safety property depends on it — but the requirement body is not met, and the scenario is not promoted. This is a source edit to `engine/pipkg/pipkg.go`, out of scope for a closure pass; it pairs naturally with SUG-2 as one branch in one function.
- **WARN-2 (R-010 and three `actuals-instrumentation` scenarios) — agent-prose behaviour has no executable harness. Ruling: FOLLOW-UP, not blocking.** Closure-feedback lives in `skills/inception-pipeline/SKILL.md` and is executed by an agent, not by code. Compliance rests on a gate-side proxy test plus documentation. The repo has no harness for agent prose; this is a structural limit of the codebase, not an omission of this change. Residual risk: prose and gate could drift apart with nothing to catch it.
- **WARN-6 — `apply-progress.md` still opens three slice narratives with "NOT committed". Ruling: FIX BEFORE ARCHIVE.** Lines 100 (Slice 2), 194 (Slice 3), and 309 (Slice 4) still read `**Delivery status**: implemented and fully verified, **NOT committed**`. All three landed and are merged (`f4bf3e4`, `9506918`/`307b79e`, `bcb1df7`). This is the only finding I rule fix-needed rather than follow-up, and the reason is timing, not severity: `apply-progress.md` is the file that travels into `openspec/changes/archive/` and becomes the permanent record. A future reader opening it cold meets three stale "blocked" headers before reaching any correction. The cost is three sentences; the cost of not doing it is a permanently misleading audit trail. It changes no code and no count, so it does not affect this verdict.
- **WARN-8 (R-014) — live GADU tool-set expansion remains unobserved. Ruling: FOLLOW-UP, not blocking; obligation narrowed, not discharged.** Materially improved since pass 2: Phase M ran, M.1–M.3 are checked, and the real extension demonstrably parsed our real GADU.md and registered `gadu`. What remains unproven is that a dispatched GADU receives a non-empty tool set, because `subagent_run` never reaches a child on this machine — a provider-auth gap that reproduces identically for gentle-pi's own agent and is filed as issue #327. This closes only when someone runs M.2 on a machine with a provider key configured; it is not work this repository can do.

**Closed since pass 2**

- **WARN-N1 CLOSED** — `.gitignore:27` now carries `/tools/*/archive-anchor-gate`; `git check-ignore -v` resolves to that line, and no stray binary is present in the module.
- **WARN-3, WARN-4, WARN-5, WARN-7** — closed at pass 2, re-confirmed here.
- **SUG-1 CLOSED** — five review receipts are now persisted for five slices (`ac87563` added `review-6e01e9510e00f068`); the archive report no longer needs to explain a missing fifth.
- **Phase M unchecked-tasks concern CLOSED** — every task in `tasks.md`, including M.1–M.3, is checked.

**SUGGESTION**

- **SUG-2** — `pipkg check` echoes a raw `builtFrom` value (e.g. `--upload-pack=evil`) verbatim into operator-facing output. It never reaches a git argv (`builtFromPattern` gates it first), but consider quoting or eliding a value that fails the pattern. Same function as WARN-1.
- **SUG-3** — Add a gate self-test that runs `--change` against this repository's own real `review-receipts/` directory. `TestApprovedTree_FromRealCapturedReceipt` now pins the parser against committed production data, which is most of the value; a full end-to-end self-test would additionally pin the wiring.
- **SUG-4 (new)** — Issue #327 is an environment gap that will block *every* future subagent dispatch verification on this machine, not just GADU's. Worth resolving before the next change whose acceptance depends on observing a child agent reply, so the same scenario does not go unproven a third time.

### Verdict

**FAIL (no defects; not fully spec-proven)** — 0 blockers, 0 CRITICAL, 4 WARNING, 3 SUGGESTION, 17/21 requirements and 35/41 scenarios complete.

The verdict word needs its meaning stated, because "fail" with zero blockers and zero critical findings reads like a contradiction. `gentle-ai sdd-verify-validate` ties any passing verdict to *complete* requirement and scenario counts: `pass` and `pass_with_warnings` are admitted only at 21/21 and 41/41. Six scenarios have no covering test that passed at runtime, so the counts cannot honestly reach complete, so the verdict cannot honestly be passing. **The counts were not adjusted to reach a passing verdict**, and the Phase M evidence — genuinely good evidence — was not stretched to cover a clause it does not reach.

**The exact blocker**: six scenarios have no covering test that passed at runtime.

| Scenario | Why not runtime-proven | Closable by |
|---|---|---|
| R-014 — GADU dispatches with working tool access | Parse and registration now observed live; tool-set expansion unobserved because `subagent_run` cannot spawn a child without a provider key (#327, reproduces for gentle-pi's own agent) | Re-running M.2 on a machine with a provider key; nothing in this repository |
| R-010 — Post-acknowledge closure produces a verified value | Closure-feedback is agent prose; only a gate-side proxy test exists | Building a harness for agent prose, or accepting the proxy explicitly |
| AI — Anchors resolve and are legible in both stores | Gate half proven; the t0-from-Engram and archive-report half is agent prose | as above |
| AI — A change that skipped inception-pipeline still measures | Agent prose only | as above |
| AI — Neither anchor resolves | Agent prose only | as above |
| R-007 — Fallback comparison is disclosed | Test passes; the requirement body's "could not be used as provenance" statement is absent from `Disclosure()` | A one-branch edit to `Disclosure()` (WARN-1, pairs with SUG-2) |

Only R-007 is closable by work in this repository. R-014 is now blocked by a machine configuration gap rather than by unrun work — a different and much smaller thing than pass 2 faced, but still not proof. The middle four describe behaviour executed by an agent reading `skills/inception-pipeline/SKILL.md`, for which this repo has no harness: a structural limit, not an omission of this change. Ruling R-007 COMPLIANT instead would yield 36/41 and 18/21, still short of passing, so the grade does not turn on that judgement call.

**Everything this change actually built is green and merged.** 42 automated tasks plus 3 manual checkpoints complete across 5 planned and 5 realized slices with zero drift; PRs #321–#326 merged; 32 packages pass at `-count=1 -race` with exit 0; `gofmt` and `go vet` clean; the change passes the pre-archive gate it introduced, run against merged `main` (`--change pi-package-hardening` → exit 0, verified `approved_tree=ec56969b…`). Five review receipts are persisted for five slices. Pass 1's CRIT-1 stays fixed and its fixture is now committed production data. WARN-N1 and SUG-1 closed this pass.

**Archive readiness is the orchestrator's call, not this report's.** Validity and archive readiness are separate decisions, and no finding here blocks archive on correctness grounds. One item genuinely wants attention first: WARN-6's three stale "NOT committed" headers, because archiving is the moment that text stops being fixable cheaply. R-014 and the four agent-prose scenarios should be carried forward as named open obligations in the archive report rather than quietly dropped — the change is done, but it is not fully spec-proven, and those are different claims.
