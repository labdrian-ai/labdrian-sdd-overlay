```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:36a59c9de84063296184b5d69621e69c09d24592eb92409e74784bd2eb56aa8e
verdict: fail
blockers: 1
critical_findings: 0
requirements: 9/12
scenarios: 27/33
test_command: cd engine && gofmt -l . && go vet ./... && go test -count=1 -race ./...
test_exit_code: 0
test_output_hash: sha256:9c9122e0081c2b83f13e7af007d88982dd2befbc64fed7fdd0e66542c755f7b3
build_command: cd engine && go build -o /tmp/claude-1000/-home-labdrian-labdrian-sdd-overlay/54b233bf-75b3-4ffe-9efb-d4c8015a06cf/scratchpad/rv3/home/.claude/bin/gentle-ai-overlay ./cmd
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report (THIRD PASS — after Remediation 1, Remediation 2, and the spec/probe/live-Pi corrections)

**Change**: pi-runtime-target
**Version**: N/A (delta specs under `openspec/changes/pi-runtime-target/specs/`)
**Mode**: Strict TDD
**Worktree**: `/home/labdrian/labdrian-sdd-overlay-worktrees/pi-5`, branch
`feat/pi-runtime-target-5-lifecycle`, HEAD `b2dce9b`, tree clean before and after verification.

**Prior verdict** (HEAD `6718160`): FAIL — 0 CRITICAL, 5 WARNING, 3 SUGGESTION, 2 blockers.
**This verdict**: FAIL (not archive-ready) — **0 CRITICAL, 3 WARNING, 3 SUGGESTION, 1 blocker.**

The entire OpenSpec-artifact blocker from the prior pass is gone: W-02a, W-04, and W-08 are
independently confirmed fixed, all 26 tasks are complete, and every prior manual-deferral PARTIAL
that live Pi could close has been closed. One new WARNING was found in this pass (N-01), and it is
the single remaining blocker because it is a shipped user-facing statement that directly
contradicts the delta spec requirement `sdd-archive` is about to merge into the source of truth.

### Rulings on Findings Carried In

| Finding | Claim | Independently verified | Ruling |
|---|---|---|---|
| W-02a delta still names a `labdrian-overlay uninstall` verb | Fixed (`5534b98`) | Yes — delta line 15 now reads "on `apply`, `status`, and `sync-check`, and to `engine runtime uninstall`"; line 125 reads "`engine runtime uninstall --target pi` (the adapter path; there is no top-level `labdrian-overlay uninstall` verb)" | **RESOLVED** |
| W-04 spec says `extensions/gate.ts`, package ships `labdrian-gate.ts` | Fixed (`5534b98`) | Yes — delta line 88 now names `extensions/labdrian-gate.ts`; the built package ships exactly that file | **RESOLVED** |
| W-08 six requirements duplicated between delta and merged main spec | Fixed (`5534b98`) | Yes — `openspec/specs/runtime-lifecycle/spec.md` lost 161 lines; `rg -ic 'pi'` on it now returns 1 (a `Multi-Contract` incidental), the Pi block is gone, and the delta carries `## ADDED Requirements` so archive merges it cleanly into a new `pi-runtime-target` domain. The `runtime-lifecycle` delta's `## MODIFIED` blocks are complete requirement bodies whose headings match the main spec exactly | **RESOLVED** |
| Relative `packages` entry defeats the listing probes | Fixed (`368b33d`) | Yes — proven at runtime on both probes (see Smoke §8) | **RESOLVED** |
| `go test ./...` could run a real `pi remove` | Fixed (`15016a7`) | Yes — `TestMain` guards present in `engine/runtime/live_guard_test.go` and `engine/cmd/live_guard_test.go`; `TestLiveGuard_IsolatesHomeAndPiBinary` passes; the live `~/.pi/agent/settings.json` still lists `../../.labdrian-overlay/pi/labdrian-pi` after a full `-race ./...` run | **RESOLVED** |
| Disclosure wrongly denied `-ne`/`-ns` aliases | Fixed (`4d3ba7e`, `504e758`) | **Partially — the Go message and README are fixed; the bash not-built branch is not** | **NOT RESOLVED** (see N-01) |
| Tasks 6.1–6.3 unchecked | Ticked (`b2dce9b`) | Yes — `tasks.md` 6.1–6.4 all `[x]`, backed by the dated "Manual Verification (Phase 6, live Pi 0.85.1, 2026-09-11)" evidence block in `apply-progress.md` | **RESOLVED** |

### Completeness

| Metric | Value |
|--------|-------|
| Tasks total (1.1–6.4) | 26 |
| Tasks complete | 26 |
| Tasks incomplete | 0 |
| Requirements | 12 (pi-runtime-target 7, runtime-lifecycle 3, longterm-mem-mcp-registration 2) |
| Scenarios | 33 (15 + 9 + 9) |
| Planned slices (`entry.json` `review_slices`) | 5 |
| Realized slices (`apply-progress` slice tracking) | 5 — no drift (P = R = 5) |

Requirement and scenario totals were **recounted independently this pass** with
`rg -c '^### (Requirement|REQ-)'` and `rg -c '^#### Scenario:'` against the three delta files:
`pi-runtime-target` 7/15, `runtime-lifecycle` 3/9, `longterm-mem-mcp-registration` 2/9 →
**12 requirements, 33 scenarios**, unchanged from the prior pass.

Entry-contract slice comparison performed, not skipped: `entry.json` `review_slices` names
`pi-target-plumbing`, `pi-package-build`, `pi-contract-gate`, `pi-longterm-mem-mcp`,
`pi-lifecycle`; `apply-progress.md` carries a realized batch for each. No planned slice lacks a
realized counterpart. The two remediation batches and the manual pass are corrective/verification
work against realized slice 5, not unplanned slices.

### Build & Tests Execution

**Build**: PASSED

```text
cd engine && go build -o <scratch>/home/.claude/bin/gentle-ai-overlay ./cmd  -> exit 0, 5052589 bytes
cd longterm-mem && go build -o <scratch>/ltm ./cmd/longterm-mem              -> exit 0
```

**Tests**: PASSED — both modules, every package

```text
cd engine && gofmt -l .                    -> (empty), exit 0
cd engine && go vet ./...                  -> exit 0
cd engine && go test -count=1 -race ./...  -> exit 0; 13 packages ok: assets cmd gadu gate
                                              installer pipkg prespec propagator runtime
                                              settings shelltest skills synctrigger
                                              (sha256 of captured output:
                                              9c9122e0081c2b83f13e7af007d88982dd2befbc64fed7fdd0e66542c755f7b3)

cd longterm-mem && gofmt -l .              -> (empty), exit 0
cd longterm-mem && go vet ./...            -> exit 0
cd longterm-mem && go test -count=1 ./...  -> exit 0; 17 packages ok
                                              (sha256 3a38a4eae7d8cb574873ac29abfa0f5604697d40ead726b3e0d50b4dde96decd)

shellcheck -S warning bin/labdrian-overlay -> exit 1; exactly 2 x SC2064 at lines 1450 and 1611
                                              (pre-existing, known environmental)

bin/labdrian-overlay skills validate --registry skills.registry.yaml --manifest overlay.manifest
  --source-root skills                     -> exit 0
                                              "registry and manifest aligned (36 skills)"
                                              "skills/ on disk matches overlay.manifest (74 files)"
                                              (HOME overridden to a scratch tree holding a FRESHLY
                                              BUILT engine binary)
```

**Live-machine safety**: the guard held. After the full `-race ./...` run,
`~/.pi/agent/settings.json` still lists `../../.labdrian-overlay/pi/labdrian-pi` and
`/home/labdrian/.labdrian-overlay/pi/labdrian-pi` still exists. Every smoke command below ran with
a scratch `HOME`, a scratch `STATE_DIR`, and `LABDRIAN_PI_BIN` pointing at a stub script that
recorded argv; the real `pi` was never invoked by this phase.

**Coverage**: Not collected — no coverage tool or threshold is configured for this repository.

### Pi-Specific Tests — Independently Re-Executed

```text
engine/runtime    TestLiveGuard_IsolatesHomeAndPiBinary                        PASS
                  TestPiAdapter_ApplyInstallSyncCheck_WiredToPipkg             PASS
                  TestPiAdapter_ApplyWithoutOverlayRoot_StaysHonestlyUnsupported PASS
                  TestLabdrianGatePathContainment_RejectsTraversal             PASS
                  TestLabdrianGateInjectsPathLine_SddTasksSddApply             PASS
                  TestLabdrianGateChainsAfterGentlePi                          PASS (node ran; not skipped)
                  TestPiAdapter_InstallNoShellInjection                        PASS
                  TestPiAdapter_StatusPartialOnUnprovenEntry                   PASS
                  TestPiAdapter_StatusAcceptsRelativePackageListing            PASS  <-- new (368b33d)
                  TestPiAdapter_StatusTriangulatesAllThreeOwnedEntries         PASS
                  TestPiAdapter_StatusDisclosesNoExtensionsNoSkills            PASS
                  TestPiAdapter_UninstallUsesRemoveNotUninstall                PASS
                  TestPiAdapter_UninstallNeverTouchesGentlePiFiles             PASS
                  TestExpandTarget_Pi                                          PASS
                  TestPiAdapter_UnbuiltDefaultReportsConcreteReasons           PASS
engine/pipkg      TestPipkgBuild_SelectsPiTargetedSkills / RejectsSymlinks /
                  AtomicSwap / OverlapAndStaleDirSafety / RefusesSpecialFiles  PASS
                  TestPipkgCheck_DetectsDrift                                  PASS
                  TestPipkgBuild_PreservesRegisteredMcpJSON                    PASS
                  TestPipkgCheck_IgnoresMcpJSONBak                             PASS
                  TestPipkgBuild_PreservesRegisteredMcpJSONBak                 PASS
engine/cmd        TestRunRuntimeCore_PiExplicitTargetReportsUnsupportedHonestly PASS
                  TestRunRuntimeCore_AllTargetsIncludesPiWithoutMaskingOtherFailures PASS
                  TestRunRuntimeCore_AllTargetsNonStatusActionsFailWhenPiUnsupported PASS
                  TestRunRuntimeCore_AllTargetsStatusFailsWhenPiIsHonestlyUnsupported PASS
engine/shelltest  TestPipkgHelpers_BuildStatusSyncCheck                        PASS
                  TestPipkgStatusAndReport_DelegatesHonestPartialThenSupported PASS
                  TestResolveTargets_Pi / TestIsCopyTarget_ClaudeTrue_PiFalse  PASS
                  TestOverlayStatusAndSyncCheck_PiStubNoEmptyMkdir             PASS
                  TestOverlayVersionAndUpdate_ListPiNeverDeployed              PASS
                  TestLongtermMemUninstall_TargetPiNoLongerDies                PASS
                  TestLongtermMemUninstall_TargetAllIncludesPi                 PASS
longterm-mem      TestRegisterPi_WritesMcpServersLongtermMem                   PASS
                  TestPi_ReinstallIsIdempotent / UntaggedSameNamedEntryRefused /
                  UninstallRemovesOwnedEntry                                   PASS
                  TestCmdRegister_TargetAll_SkipsAbsentPi                      PASS
                  TestCmdRegister_TargetPiExplicit_FailsWhenPackageAbsent      PASS
                  TestCmdRegister_TargetPi_RegistersWhenInstalled              PASS
                  TestCmdRegister_TargetPi_AcceptsRelativePackageListing       PASS  <-- new (368b33d)
```

### Runtime Smoke Evidence (scratch `HOME`, scratch `STATE_DIR`, stub `pi` via `LABDRIAN_PI_BIN`)

```text
1) engine runtime install --target pi
   -> exit 0; "[pi] install: restart_required - ran `pi install <pkg>`; start a new Pi session"
   -> stub argv log: "install <pkg>"; settings.json packages gains exactly one entry
   -> package tree: package.json, mcp.json, agents/, extensions/, skills/
   -> package.json "pi": {"skills":["./skills"],"agents":["./agents"],
                          "extensions":["./extensions"],"mcp":"./mcp.json"}
   -> extensions/labdrian-gate.ts present (matches the corrected spec text)

2) engine runtime status --target pi        (installed, MCP NOT registered)
   -> exit 1; "[pi] status: partial - ... longterm-mem registered in mcp.json (not registered;
      run: longterm-mem register --target pi)"                        C-01 still fixed

2b) bin/labdrian-overlay status --target pi (same state)
   -> exit 1; prints the identical delegated "[pi] status: partial ..." line   W-01 still fixed

3) bin/labdrian-overlay sync-check --target pi (built, not registered)
   -> exit 0; "SYNC_CHECK:pi: no drift"; "VERDICT:pi:IN_SYNC"

4) longterm-mem register --target pi --config-root <pkg>
   -> exit 0; "register: pi: ok"
   -> <pkg>/mcp.json: mcpServers.longterm-mem = {"type":"stdio","command":"<ltm>","args":["mcp"]}
   -> sibling <pkg>/mcp.json.bak created
   -> pre-seeded pi-engram-owned ~/.pi/agent/mcp.json sha256 BYTE-IDENTICAL before == after
      (f251cc50d0a2fba8e6c08473fa0013f5a46feed7a7f2ed5684891a30e9cdb144)
   -> ~/.pi/agent/agents/ never created

5) engine runtime status --target pi        (built + listed + registered)
   -> exit 0; "[pi] status: supported - labdrian-pi package is built, in sync, and listed in
      ~/.pi/agent/settings.json."

6) bin/labdrian-overlay sync-check --target pi (with .bak present)
   -> exit 0; "VERDICT:pi:IN_SYNC"                                    C-02 still fixed

7) engine pipkg build ... --dest-dir <pkg>  (rebuild over a registered package)
   -> exit 0; mcp.json     sha256 c95e1d6f... before == after (preserved)
              mcp.json.bak sha256 372a7f8c... before == after (preserved)
   -> subsequent sync-check: VERDICT:pi:IN_SYNC exit 0

8) RELATIVE packages entry probe (the 368b33d regression)
   settings.json packages entry rewritten to "../../../state/pi/labdrian-pi"
   -> engine runtime status --target pi     -> exit 0, "supported"    both probes resolve
   -> longterm-mem register --target all    -> "register: pi: ok" (pi NOT skipped as absent)
   Before 368b33d both probes compared an absolute path against this relative string and
   never matched.

9) longterm-mem unregister --target pi --config-root <pkg>
   -> exit 0; "unregister: pi: removed"; <pkg>/mcp.json back to {"mcpServers":{}}
   -> status returns to honest `partial` exit 1; sync-check stays IN_SYNC exit 0

10) engine runtime uninstall --target pi
   -> exit 0; "[pi] uninstall: supported - removed via `pi remove <pkg>`; package directory deleted"
   -> stub argv log: "remove <pkg>" (verb is `remove`, never `uninstall`)
   -> settings.json packages back to ["npm:gentle-pi"]; package directory gone
   -> ~/.pi/agent/mcp.json byte-identical; ~/.pi/agent/agents/ never created

11) engine runtime status --target all
   -> exit 1 (honest: claude unsupported, opencode unsupported, codex partial)
   -> "[pi] status: supported - ..." IS present in the aggregate while the non-Pi failures still
      fail the overall command                                        W-03 still fixed
```

### Live-Pi Evidence Accepted from `apply-progress.md` (Phase 6, Pi 0.85.1, 2026-09-11)

This phase did not re-run the live pass (it mutates the real machine). The recorded evidence was
read and each claim was matched against a scenario:

- **6.1** — `pi install` was recorded by Pi *relative* to `~/.pi/agent/`; a headless
  `pi --no-session -p` session listed `gadu-operator`, `gadu-orchestrate`,
  `sdd-time-estimation`; nothing was written under `~/.pi/agent/agents/`.
- **6.2** — `pi list` shows the package with its `extensions/labdrian-gate.ts`; a session with
  extensions enabled answers a trivial prompt in 2.7s. The with/without prompt comparison could
  **not** be observed, because `--no-extensions` also disables the pi-claude-bridge provider on
  this machine.
- **6.3** — `pi remove <package>` left `~/.pi/agent/settings.json` (minus `packages`),
  `~/.pi/agent/mcp.json`, and `~/.pi/agent/agents/` byte-identical; only the owned entry vanished.

### Spec Compliance Matrix

Statuses: COMPLIANT (covering evidence passed at runtime), PARTIAL (part of the scenario proven,
part unobserved), FAILING (evidence contradicts the scenario).

#### `specs/pi-runtime-target/spec.md` (7 requirements, 15 scenarios)

| Requirement | Scenario | Test / Evidence | Result |
|---|---|---|---|
| Pi Accepted as a Valid CLI Target | Pi target is recognized | `shelltest.TestResolveTargets_Pi`, `TestOverlayStatusAndSyncCheck_PiStubNoEmptyMkdir`, `TestPipkgHelpers_BuildStatusSyncCheck`, `runtime.TestPiAdapter_UninstallUsesRemoveNotUninstall` + smoke §1,2b,3,10 covering all four named surfaces | COMPLIANT — **was PARTIAL (W-02a)** |
| Pi Accepted as a Valid CLI Target | Target all includes Pi without masking other failures | `cmd.TestRunRuntimeCore_AllTargetsIncludesPiWithoutMaskingOtherFailures` + smoke §11 | COMPLIANT |
| Package-Delivered Skills and Agents | Local-path install registers the package idempotently | Live 6.1 + smoke §1 prove the package is listed in `~/.pi/agent/settings.json` packages. The second THEN clause — re-running install leaves a single entry — has no evidence on either side: the live pass installed once, and the smoke stub's dedup is my own code, not Pi's | PARTIAL — listing proven, idempotency unobserved |
| Package-Delivered Skills and Agents | Skills and agents are visible in a Pi session | Live 6.1 proves skill discovery (3 packaged skills listed by a real session) and proves the AND clause (`~/.pi/agent/agents/` never written). "Custom agents are present" is evidenced only structurally: the package ships `agents/GADU.md` and declares `"agents":["./agents"]` | PARTIAL — skills and the non-mutation clause proven live; agent presence in a live session not observed |
| Deterministic Contract Gate | sdd-tasks and sdd-apply receive both contract path lines | `runtime.TestLabdrianGateInjectsPathLine_SddTasksSddApply` (node runs the real embedded source against the Go `InjectPrompt` oracle) | COMPLIANT |
| Deterministic Contract Gate | Every other agent is excluded | same test, excluded-agent and unnamed-agent cases | COMPLIANT |
| Deterministic Contract Gate | Composition with gentle-pi's own handler | `runtime.TestLabdrianGateChainsAfterGentlePi` (node present, ran for real) | COMPLIANT |
| Deterministic Contract Gate | Contract paths stay contained; malformed frontmatter yields no injection | `runtime.TestLabdrianGatePathContainment_RejectsTraversal` (4 cases) + malformed-sibling case | COMPLIANT |
| Deterministic Contract Gate | Extension is discovered from the package extensions directory | `pipkg` build tests + smoke §1 prove `extensions/labdrian-gate.ts` ships and `package.json` declares `"extensions":["./extensions"]`; live 6.2 proves `pi list` reports it and that a session with extensions starts normally. The THEN clause "it loads `labdrian-gate.ts` via jiti" was not directly observed — 6.2 records that the `--no-extensions` comparison was impossible on this machine | PARTIAL — shipped, declared and listed; jiti load not directly observed |
| Honest Status for Unproven Activation | Unproven entry forces partial | `runtime.TestPiAdapter_StatusTriangulatesAllThreeOwnedEntries` (all three owned entries) + smoke §2, §9 | COMPLIANT |
| Honest Status for Unproven Activation | All entries proven report supported | same test's `all_three_entries_proven_reports_supported` + `TestPiAdapter_StatusAcceptsRelativePackageListing` + smoke §5, §8 | COMPLIANT |
| `--no-extensions` Limitation Disclosure | Disclosure text is present | `runtime.TestPiAdapter_StatusDisclosesNoExtensionsNoSkills`, `shelltest` assertions on both built and not-built branches, smoke §2/§2b/§5 output | COMPLIANT — the scenario's two THEN clauses hold on every branch (see N-01 for the requirement-text contradiction, which is outside this scenario's THEN clauses) |
| Pi-Scoped Uninstall | Only the owned package entry is removed | `runtime.TestPiAdapter_UninstallUsesRemoveNotUninstall`, `TestPiAdapter_UninstallNeverTouchesGentlePiFiles` + smoke §10 + live 6.3 (byte-identical gentle-pi/pi-engram state on a real machine) | COMPLIANT |
| Pi Drift Detection via Sync-Check | No drift is reported when unchanged | `pipkg.TestPipkgCheck_IgnoresMcpJSONBak`, `TestPipkgBuild_PreservesRegisteredMcpJSONBak` + smoke §3, §6, §7 | COMPLIANT |
| Pi Drift Detection via Sync-Check | A source edit is detected as drift | `pipkg.TestPipkgCheck_DetectsDrift`, `shelltest.TestPipkgHelpers_BuildStatusSyncCheck` tamper case | COMPLIANT |

#### `specs/runtime-lifecycle/spec.md` (3 requirements, 9 scenarios)

| Requirement | Scenario | Test / Evidence | Result |
|---|---|---|---|
| Target Aggregation | Target all includes Codex support | `cmd.TestRunRuntimeCore_AllTargetsRunsCodexLifecycleTogether` | COMPLIANT |
| Target Aggregation | Target all preserves non-Codex failures | `cmd.TestRunRuntimeCore_AllTargetsStatusFailsWhenClaudeOrOpenCodeFails` | COMPLIANT |
| Target Aggregation | Target all includes Pi support | `cmd.TestRunRuntimeCore_AllTargetsStatusAllowsCodexPartialWithoutFailing`, `TestRunRuntimeCore_AllTargetsStatusFailsWhenPiIsHonestlyUnsupported` + smoke §11 (`[pi] supported` in the aggregate) | COMPLIANT |
| Target Aggregation | Target all preserves non-Pi failures | `cmd.TestRunRuntimeCore_AllTargetsIncludesPiWithoutMaskingOtherFailures` + smoke §11 (claude/opencode unsupported still fail the aggregate at exit 1) | COMPLIANT |
| Existing Runtime Behavior Non-Regression | Legacy Claude commands still work | `cmd.TestRunRuntimeCore_ClaudeUpdateAndUninstallPreserveLegacyBehavior`; full engine suite green under `-race` | COMPLIANT |
| Existing Runtime Behavior Non-Regression | OpenCode lifecycle remains unchanged | `cmd.TestRunRuntimeCore_OpenCode*` | COMPLIANT |
| Existing Runtime Behavior Non-Regression | Codex lifecycle remains unchanged | `cmd.TestRunRuntimeCore_Codex*`, `shelltest.TestOverlayStatusAndSyncCheckAll_ClaudeOpenCodeCodexOutputUnchanged` | COMPLIANT |
| Pi Target Validation and Dispatch Without a Per-File Copy Path | Pi passes target validation without a copy-path entry | `shelltest.TestIsCopyTarget_ClaudeTrue_PiFalse`, `TestOverlayStatusAndSyncCheck_PiStubNoEmptyMkdir` | COMPLIANT |
| Pi Target Validation and Dispatch Without a Per-File Copy Path | Pi is present in the enum, aggregation, and adapter construction | `runtime.TestExpandTarget_Pi` + `NewFoundationAdapter` adapter-type assertion | COMPLIANT |

#### `specs/longterm-mem-mcp-registration/spec.md` (2 requirements, 9 scenarios)

| Requirement | Scenario | Test / Evidence | Result |
|---|---|---|---|
| MCP Registration — Pi | pi-engram's mcp.json stays untouched | `register.TestRegisterPi_*` + smoke §4/§10 (pre-seeded `~/.pi/agent/mcp.json` sha256 identical through register and uninstall) + live 6.3 on the real machine | COMPLIANT |
| MCP Registration — Pi | The registration target is a package-relative mcp.json file | Smoke §1/§4: `package.json` `"pi":{"mcp":"./mcp.json"}`; `<pkg>/mcp.json` top-level `mcpServers` with exactly one `longterm-mem` entry, no nesting | COMPLIANT |
| MCP Registration — Pi | The server name is package-prefixed | pi-mcp-adapter's own load-time prefixing; nothing in this change controls it, and the live pass did not inspect the served name | PARTIAL — external, unobserved |
| MCP Registration — Pi | User/project config takes precedence | pi-mcp-adapter's own precedence rule; the overlay correctly implements nothing here, and precedence was not exercised live | PARTIAL — external, satisfied by non-action |
| MCP Registration — Pi | pi-engram init does not drop the registration | The registration lives inside the package, not `~/.pi/agent/mcp.json`, so `pi-engram init` structurally cannot reach it. No automated or live proof | PARTIAL — external, unobserved |
| MCP Registration — Pi | Registration is skipped on expansion when the package is absent | `cmd/longterm-mem.TestCmdRegister_TargetAll_SkipsAbsentPi` + smoke §11-adjacent run | COMPLIANT |
| MCP Registration — Pi | Registration fails when Pi is named explicitly and the package is absent | `cmd/longterm-mem.TestCmdRegister_TargetPiExplicit_FailsWhenPackageAbsent` + smoke (observed the explicit-target failure message when the settings entry was missing) | COMPLIANT |
| Multi-Target Expansion Treats Pi Like the Other Runtimes | An expansion skips Pi when its package is not installed | `TestCmdRegister_TargetAll_SkipsAbsentPi`, `shelltest.TestLongtermMemUninstall_TargetAllIncludesPi` | COMPLIANT |
| Multi-Target Expansion Treats Pi Like the Other Runtimes | Pi named explicitly still fails without the package installed | `TestCmdRegister_TargetPiExplicit_FailsWhenPackageAbsent`, `shelltest.TestLongtermMemUninstall_TargetPiNoLongerDies` | COMPLIANT |

**Compliance summary**: 27/33 scenarios COMPLIANT, 6 PARTIAL, **0 FAILING**.
9/12 requirements fully compliant (was 8/12, and 5/12 two passes ago).

The 6 PARTIALs, named exactly:

1. *Local-path install registers the package idempotently* — the re-install-leaves-one-entry clause.
2. *Skills and agents are visible in a Pi session* — the custom-agent-presence clause.
3. *Extension is discovered from the package extensions directory* — the jiti-load clause.
4. *The server name is package-prefixed* — pi-mcp-adapter behaviour, unobserved.
5. *User/project config takes precedence* — pi-mcp-adapter behaviour, satisfied by non-action.
6. *pi-engram init does not drop the registration* — structurally unreachable, unobserved.

None is a code defect. (1)–(3) are narrow observational gaps in an otherwise-completed live pass;
(4)–(6) are owned by pi-mcp-adapter and pi-engram, not by this change.

### Correctness (Static Evidence)

| Requirement | Status | Notes |
|---|---|---|
| R-001 Pi as CLI target | Implemented | `TARGET_KINDS`/`is_copy_target`/`is_valid_target`; the delta now names the adapter verb correctly |
| R-002 Package delivery | Implemented | `engine/pipkg.Build`: registry-selected skills, `agents/`, embedded extension, temp-dir + atomic swap, symlink refusal, overlap guard |
| R-003/R-004 Contract gate | Implemented | `engine/pipkg/labdrian-gate.ts`, strict frontmatter parse, bare path line, chained `event.systemPrompt`, root containment |
| R-005 MCP registration | Implemented | `longterm-mem/internal/register/pi.go` reuses unmodified `jsonInstall(containerKey="mcpServers")`; `register_paths.go` now resolves relative settings entries |
| R-006 Honest status | Implemented | `PiAdapter.Status` builds `problems` from all three owned entries; `isPiPackageListed` resolves relative entries against `~/.pi/agent` |
| R-007 Disclosure | **Implemented with a defect** | The Go message and README now name `-ne`/`-ns`; the bash not-built branch still denies them (N-01) |
| R-008 Non-copy dispatch | Implemented | No `TARGET_PATHS` entry for pi; no empty-path `mkdir` |
| R-009 Uninstall | Implemented | `pi remove <path>`, fixed argv, `exec.LookPath`/`LABDRIAN_PI_BIN`, package dir removed |
| R-010 Drift detection | Implemented | `Check` excludes `mcp.json` and `mcp.json.bak`; `Build` preserves both |

### Coherence (Design)

| Decision (design.md) | Followed? | Notes |
|---|---|---|
| A1 — `pi.mcp` is a package-relative path; unmodified `jsonInstall` | Yes | Verified in the built package and in `register/pi.go` |
| A2 — `pi remove <source>`, not `pi uninstall <name>` | Yes | Stub argv log and live 6.3 both prove the verb |
| A3 — `--no-extensions` / `--no-skills` | Partial | The design's "no short alias" premise was wrong; Pi 0.85.1 documents `-ne`/`-ns`. Research, spec, README and the Go message were corrected (`4d3ba7e`); the bash branch was missed (N-01) |
| A4 — chained `before_agent_start`, additive `systemPrompt` | Yes | `TestLabdrianGateChainsAfterGentlePi` |
| A5 — type-annotation-free `.ts`, tested via `.mjs` copy under node | Yes | Ships as `.ts`; node tests ran for real |
| C4/C5 — bare path line, reuse `runtime.InjectPrompt` as oracle | Yes | Byte-identical to the Go mirror |
| `TARGET_KINDS[pi]=package` + 8 call sites | Yes | Plus a disclosed 9th (`cmd_longterm_mem`) fixed in slice 1 |
| Honest status across package, extension and MCP | Yes | All three owned entries probed; extension load is still proxied by the in-sync check |
| Security F1–F3 — containment, strict parse, symlink refusal, atomic swap, 0644/0755 | Partial | Re-confirmed observationally this run: files `644`, subdirectories `755`, package ROOT `700` (`drwx------` on both the live package and a freshly built scratch package). See W-05 |
| Threat matrix: process integration (fixed argv, LookPath) | Yes | `TestPiAdapter_InstallNoShellInjection` |
| Threat matrix: path containment (F1) | Yes | `TestLabdrianGatePathContainment_RejectsTraversal` |
| 5-slice table | Yes, with disclosed deviations | Every slice's file list landed |
| Open question — registry opt-in set for `install.targets: [pi]` | Resolved | `skills validate` exit 0, 36 skills aligned; 12 skills ship in the package |

### Issues Found

**CRITICAL**: None.

**WARNING**

- **N-01 (new, blocking) — the bash not-built disclosure still denies the `-ne`/`-ns` aliases.**
  `bin/labdrian-overlay:241` prints
  `"... neither flag's use is detected at runtime, and there is no short alias for either flag"`.
  Commit `4d3ba7e` corrected exactly this claim in `engine/runtime/pi.go:59`
  (`"(short aliases: -ne and -ns)"`), in `README.md:69`, and in the delta spec, whose R-007
  requirement text now states plainly: *"The short aliases are `-ne` and `-ns`."* The bash copy of
  the same sentence was missed. This branch is reachable — it is what
  `labdrian-overlay status --target pi` prints before the package has ever been built, i.e. the
  first Pi status a new user sees. It ships a statement that the spec `sdd-archive` is about to
  merge into the source of truth directly contradicts. The scenario's own THEN clauses still hold
  (the note is present; it claims no runtime detection), which is why this is a WARNING and not a
  CRITICAL — but it is the one blocker, because it is a one-line fix and archiving it produces a
  source of truth that shipped code contradicts. Fix: replace the tail of that string with the
  same `(short aliases: -ne and -ns)` wording used in `pi.go`, and add a shelltest assertion that
  the not-built branch's disclosure matches the adapter's.

- **W-05 — Package root ships `0700`, design says `0755` for directories.** Follow-up, already
  filed as GitHub issue #312. Re-confirmed twice this run: `drwx------` on the live
  `~/.labdrian-overlay/pi/labdrian-pi` and `700` on a freshly built scratch package, while
  `extensions/` is `755` and `package.json` is `644`. No spec scenario names file modes, and
  `0700` is more restrictive than designed, so this is a design-coherence deviation, not a
  security regression. Not blocking.

- **W-06 — `apply` switches the worktree to `main` before building.** Follow-up, to be filed.
  Confirmed present this run by inspection: `bin/labdrian-overlay:1613` runs `git checkout main`
  under an EXIT trap that restores the original branch (`:1611`). Pre-existing behaviour shared by
  every target, not introduced by this change; Pi is simply the first target whose `sync-check`
  can detect the resulting build/branch mismatch. Not re-exercised (running `apply` would mutate
  the branch checkout, which this phase must not do). Not blocking.

**SUGGESTION**

- **S-01 — Two stale "slice 2" strings remain in `bin/labdrian-overlay`.**
  `package_target_stub_message` (line 153) and the sync-check branch (line 2525) both print
  `"package target, handled by slice 2 (pi-package-build)"`. Slice 2 landed long ago, and slice 5
  replaced the very behaviour these lines defer. Same class as the W-01 text already fixed.
- **S-02 — The `supported` status message under-reports what it proved.**
  `engine/runtime/pi.go:124-125` reads "built, in sync, and listed in `~/.pi/agent/settings.json`"
  while `Status` now also proves the longterm-mem MCP entry. The `partial` branch names that third
  entry correctly; the success branch does not.
- **S-03 — The live pass observed 3 of the 12 skills the package ships.**
  `apply-progress` 6.1 records `gadu-operator`, `gadu-orchestrate`, `sdd-time-estimation`; the
  built package contains twelve skill directories plus `_shared`. Discovery is proven, coverage is
  not. If a follow-up live session is run for the PARTIALs above, listing the full set (and one
  package agent) would close scenario gaps 1–3 in a single pass.

### TDD Compliance

| Check | Result | Details |
|---|---|---|
| TDD Evidence reported | Yes | 5 slice tables, 2 remediation tables, and a dated manual-verification block in `apply-progress` |
| All tasks have tests | Yes | 26/26 tasks complete; every code task maps to a named test file; 5.4, W-02 and 6.1–6.3 are docs/spec/manual by design |
| RED confirmed (test files exist) | Yes | Every named file located, including the two new relative-path tests from `368b33d` and the live guards from `15016a7` |
| RED evidence is real, not asserted | Yes | Remediations record falsifiable pre-fix strings (`got: supported, want partial`; `mcp.json.bak: extra`; `got 0, want 1`). The `368b33d` RED is the strongest kind — a live-machine observation that the probe never matched a real Pi-written relative entry |
| GREEN confirmed (tests pass now) | Yes | 39 pi-specific tests re-executed verbosely here; both full suites green, engine under `-race` |
| Triangulation adequate | Yes | Status triangulates all three owned entries; drift uses two independent RED tests (Check side, Build side); the relative-path fix is covered on both probes independently |
| Safety net for modified files | Yes | Each batch records the relevant suite green before edit |
| Live-machine isolation | Yes | `TestMain` guards added after a real `pi remove` fired during `go test ./...`; `TestLiveGuard_IsolatesHomeAndPiBinary` makes the guard itself falsifiable rather than a comment |

**TDD Compliance**: 8/8 checks passed.

### Test Layer Distribution

| Layer | Tests | Files | Tools |
|---|---|---|---|
| Unit (Go) | 28 pi-specific | `engine/pipkg/pipkg_test.go` (9), `engine/runtime/pi_test.go` (6 adapter), `engine/runtime/live_guard_test.go` (1), `engine/cmd/pipkg_test.go` (3), `engine/cmd/runtime_test.go` (4 pi), `longterm-mem/internal/register/pi_test.go` (4), `engine/runtime/runtime_test.go` (1) | `go test` |
| Integration — bash | 8 | `engine/shelltest/overlay_pi_target_test.go` (6), `overlay_pi_package_build_test.go` (2) — source the real `bin/labdrian-overlay` against a real built engine binary | `go test` + bash |
| Integration — node | 3 | `engine/runtime/pi_test.go` — run the real embedded `labdrian-gate.ts` under node | `go test` + node |
| Integration — CLI | 4 | `longterm-mem/cmd/longterm-mem/main_test.go` pi cases | `go test` |
| E2E (live Pi) | 3 (manual) | Tasks 6.1–6.3, executed 2026-09-11 against Pi 0.85.1; no headless runner exists | Manual |
| **Total pi-specific** | **46** | **9 files + 1 manual pass** | |

### Changed File Coverage

Coverage analysis skipped — no coverage tool or threshold is configured for this repository, and
none was required by `entry.json`. Not a failure.

### Assertion Quality

Re-audited the pi-related test files, focusing on the four tests added since the prior pass
(`TestPiAdapter_StatusAcceptsRelativePackageListing`,
`TestCmdRegister_TargetPi_AcceptsRelativePackageListing`,
`TestLiveGuard_IsolatesHomeAndPiBinary`, and the updated disclosure test). No tautologies, no
orphan empty-collection assertions, no type-only assertions used alone, no ghost loops, no
smoke-test-only cases, no mock-heavy files. The suite uses real filesystem temp trees, real
subprocesses, and real node execution rather than mocks; assertions compare concrete expected
strings, exit codes, file bytes, sha256 digests, and recorded argv.

`TestLiveGuard_IsolatesHomeAndPiBinary` deserves a specific note: it asserts a *negative*
(`os.Stat` on the real `~/.pi/agent/settings.json` must fail, `LABDRIAN_PI_BIN` must not exist),
which is normally the weakest assertion shape. Here it is the right one — it makes the `TestMain`
guard falsifiable, so a future edit that drops the guard fails a test instead of silently
uninstalling a developer's Pi package again.

One gap worth naming: `TestPiAdapter_StatusDisclosesNoExtensionsNoSkills` asserts only that
`--no-extensions` and `--no-skills` appear in the message, not what the message says *about* them.
That is precisely why N-01 survived the alias correction on the bash side.

**Assertion quality**: All assertions verify real behavior.

### Quality Metrics

**Formatter**: `gofmt -l` clean in both modules.
**Vet**: `go vet ./...` clean in both modules.
**Linter (shell)**: `shellcheck -S warning` reports only the 2 pre-existing SC2064 warnings.
**Race detector**: `go test -race` clean across all 13 engine packages.

### Verdict

**FAIL — not archive-ready.** 0 CRITICAL, 3 WARNING, 3 SUGGESTION, **1 blocker**.

The implementation is sound and the gap to archive is now one line of bash. Everything the prior
pass blocked on in the OpenSpec artifacts is fixed and independently confirmed: the delta names
the adapter uninstall verb, names the real extension filename, carries `## ADDED Requirements`,
and no longer duplicates six requirements against a hand-edited main spec. All 26 tasks are
complete, the live-Pi pass ran against Pi 0.85.1 and found a real bug (relative `packages` entries)
that was fixed and covered on both probes, and a second live incident produced permanent
`TestMain` guards with a falsifiable test. Scenario compliance rose 26/33 → 27/33 with zero
FAILING scenarios, and every suite, vet, gofmt, race, shellcheck and skills check passes.

The `fail` verdict records one blocker:

**N-01.** `bin/labdrian-overlay:241` still tells users "there is no short alias for either flag"
while the delta spec that `sdd-archive` will merge states the aliases are `-ne` and `-ns`, and
while the Go adapter and README now say so too. Archiving this merges a source of truth that the
shipped CLI contradicts on its own output — the same failure mode this change already paid two
remediation batches to avoid. It is a one-line string fix plus a shelltest assertion.

The 6 remaining PARTIAL scenarios are **not** counted as a second blocker this pass. Three are
narrow observational gaps in an otherwise-completed manual pass (install idempotency, agent
presence in a live session, direct jiti-load observation) and three are owned entirely by
pi-mcp-adapter and pi-engram. They are recorded here so archive inherits an accurate evidence
ledger, and S-03 describes how a single further live session would close the first three.

W-05 (root `0700`, issue #312) and W-06 (`apply` builds from `main`) are confirmed follow-ups and
do not block.

**Recommended next step**: `sdd-apply` for N-01 only — one string in `bin/labdrian-overlay`, plus
a test that pins the not-built disclosure to the adapter's wording so the two copies cannot drift
again. Then re-verify and archive.
