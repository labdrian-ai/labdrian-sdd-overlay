```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:5eaffcb2cdeaa2661608d0a1ef0407c19d105b577f41a21f85c5523a09c0961b
verdict: fail
blockers: 2
critical_findings: 0
requirements: 8/12
scenarios: 26/33
test_command: cd engine && gofmt -l . && go vet ./... && go test -count=1 -race ./...
test_exit_code: 0
test_output_hash: sha256:186893cc033d7cfd1550e5f9ebd3ccc43413352483618eb4496464aeb2431467
build_command: cd engine && go build -o /tmp/claude-1000/-home-labdrian-labdrian-sdd-overlay/54b233bf-75b3-4ffe-9efb-d4c8015a06cf/scratchpad/rv2/build_probe ./cmd
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report (RE-VERIFICATION after Remediation 1 + 2)

**Change**: pi-runtime-target
**Version**: N/A (delta specs under `openspec/changes/pi-runtime-target/specs/`)
**Mode**: Strict TDD
**Worktree**: `/home/labdrian/labdrian-sdd-overlay-worktrees/pi-5`, branch
`feat/pi-runtime-target-5-lifecycle`, HEAD `6718160`, tree clean before and after verification.

> The launch brief named HEAD `14455dd`. The actual HEAD is `6718160`
> (`docs(sdd): record verify remediation size exception for pi-runtime-target`), one commit
> further on. That commit touches `entry.json` only (the granted `verify-remediation` size
> exception); it changes no source. Verification was performed against `6718160`.

**Prior verdict**: FAIL — 2 CRITICAL (C-01, C-02), 7 WARNING, 3 SUGGESTION.
**This verdict**: FAIL (not archive-ready) — **0 CRITICAL**, 5 WARNING, 3 SUGGESTION.
Every prior CRITICAL is fixed and no code defect remains. The verdict is `fail` because
archive-readiness requires complete requirement and scenario evidence, and 7 scenarios are
still only PARTIAL: 4 wait on the unchecked manual live-Pi tasks 6.1-6.3, 2 are owned by
pi-mcp-adapter, and 1 is the unresolved spec-text finding W-02a. This is an evidence-
completeness failure, not an implementation failure.

### Remediation Outcome

| Finding | Remediation claim | Independently verified | Ruling |
|---|---|---|---|
| C-01 status overclaims `supported` | Fixed (`716ccdf`) | Yes — third owned entry probed; runtime proof below | **RESOLVED** |
| C-02 register puts package in permanent drift | Fixed (`a0f6ee8`) | Yes — `VERDICT:pi:IN_SYNC` exit 0 with `.bak` present | **RESOLVED** |
| W-01 `bin` status never called the adapter | Fixed (`186cdcc`) | Yes — delegates; stale text gone | **RESOLVED** |
| W-02 spec names a non-existent `labdrian-overlay uninstall` | Fixed (`f8ae0c6`) | **No — fix landed in the wrong file** | **NOT RESOLVED** (see W-02a) |
| W-03 `--target all` Pi exemption outlived its lifetime | Fixed (`2ffe96e`) | Yes — exemption gone, `[pi]` line present in aggregate | **RESOLVED** |
| W-07 task 6.4 checkbox unchecked | — | Yes — now `[x]` in `tasks.md` | **RESOLVED** |

Four of the five in-scope findings are genuinely fixed and proven at runtime. W-02 is not.

### Completeness

| Metric | Value |
|--------|-------|
| Tasks total (1.1–6.4) | 26 |
| Tasks complete | 23 (1.1–5.4 plus 6.4) |
| Tasks incomplete | 3 (6.1, 6.2, 6.3 — manual-only live-Pi, deferred by design) |
| Requirements | 12 (pi-runtime-target 7, runtime-lifecycle 3, longterm-mem-mcp-registration 2) |
| Scenarios | 33 (15 + 9 + 9) |
| Planned slices (`entry.json` `review_slices`) | 5 |
| Realized slices | 5 — no drift (P = R = 5; the two remediation batches are corrective work against realized slice 5, not new planned slices) |

Requirement and scenario totals were **recounted independently** from the three delta spec files
with `rg -c '^### (Requirement|REQ-)'` and `rg -c '^#### Scenario:'`: 2+7+3 = 12 requirements and
9+15+9 = 33 scenarios. This agrees with the stated 12/33.

**Tasks 6.1–6.3 ruling**: deferred, not failed. `design.md` declared them manual-only from the
start, no headless Pi runner exists, and the orchestrator confirmed post-chain manual live-Pi
verification. They are recorded as an explicit deferral rather than treated as a blocking
incomplete-task condition.

### Build & Tests Execution

**Build**: PASSED

```text
cd engine && go build -o <scratch>/gentle-ai-overlay ./cmd          -> exit 0, 5052173-byte binary
cd longterm-mem && go build -o <scratch>/ltm ./cmd/longterm-mem     -> exit 0
```

**Tests**: PASSED — both modules, all packages

```text
cd engine && gofmt -l .                    -> (empty), exit 0
cd engine && go vet ./...                  -> exit 0
cd engine && go test -count=1 -race ./...  -> exit 0; 13 packages ok: assets cmd gadu gate
                                              installer pipkg prespec propagator runtime
                                              settings shelltest skills synctrigger

cd longterm-mem && gofmt -l .              -> (empty), exit 0
cd longterm-mem && go vet ./...            -> exit 0
cd longterm-mem && go test -count=1 ./...  -> exit 0; 17 packages ok: root cmd/longterm-mem
                                              durable embed engram identityledger mcpserver
                                              ops projectid promote query register repohistory
                                              staleness vault vaultreg vecindex

shellcheck -S warning bin/labdrian-overlay -> exit 1; exactly 2 x SC2064 at lines 1450 and 1611
                                              (pre-existing, known environmental; content
                                              unchanged, line numbers shifted by the W-01 fix)

bin/labdrian-overlay skills validate --registry skills.registry.yaml --manifest overlay.manifest
  --source-root skills                     -> exit 0
                                              "registry and manifest aligned (36 skills)"
                                              "skills/ on disk matches overlay.manifest (74 files)"
                                              (HOME overridden to a scratch tree holding a FRESHLY
                                              BUILT engine binary at
                                              <scratch>/home/.claude/bin/gentle-ai-overlay)
```

**Coverage**: Not collected — no coverage tool or threshold is configured for this repository.

### Remediation Tests — Independently Re-Executed

All five tests the two remediation batches claim to have added were located in the tree and
re-run verbosely. All pass.

```text
TestPiAdapter_StatusTriangulatesAllThreeOwnedEntries          PASS (3 subtests)
  /listed_but_MCP_unregistered_stays_partial_and_names_the_register_command   PASS
  /unlisted_and_MCP_unregistered_stays_partial                                PASS
  /all_three_entries_proven_reports_supported                                 PASS
TestPipkgCheck_IgnoresMcpJSONBak                              PASS
TestPipkgBuild_PreservesRegisteredMcpJSONBak                  PASS
TestRunRuntimeCore_AllTargetsStatusFailsWhenPiIsHonestlyUnsupported   PASS
TestRunRuntimeCore_AllTargetsStatusAllowsCodexPartialWithoutFailing   PASS (updated)
TestPipkgStatusAndReport_DelegatesHonestPartialThenSupported  PASS
```

C-01's prior gap was precisely that `TestPiAdapter_StatusPartialOnUnprovenEntry` triangulated
over only one of three named entries. The replacement now triangulates all three, which is the
correct structural answer to that finding rather than a single added case.

### Runtime Smoke Evidence (scratch `STATE_DIR`, scratch `HOME`, stub `pi` via `LABDRIAN_PI_BIN`)

The real `~/.npm-global/bin/pi` was never invoked. The stub recorded argv and mutated only a
scratch `~/.pi/agent/settings.json`. Confirmed after the run: `/home/labdrian/.labdrian-overlay/pi`
does not exist — nothing was written under the user's real `$HOME`.

```text
engine runtime install --target pi
  -> exit 0; "[pi] install: restart_required - ran `pi install <pkg>`; start a new Pi session"
  -> stub argv log: "install <pkg>"; settings.json packages: ["<pkg>"]
  -> package tree: package.json, mcp.json, agents/, extensions/, skills/

engine runtime status --target pi        (installed, MCP NOT yet registered)
  -> exit 1; "[pi] status: partial - labdrian-pi status is unproven: longterm-mem registered
     in mcp.json (not registered; run: longterm-mem register --target pi)."   <-- C-01 FIXED
     (pre-remediation this same state returned `supported`, exit 0)

bin/labdrian-overlay status --target pi  (same state)
  -> exit 1; prints the identical delegated "[pi] status: partial ..." line   <-- W-01 FIXED
     The hard-coded "no drift (partial -- lifecycle proof lands in a later slice)" text is gone.

longterm-mem register --target pi --config-root <pkg>
  -> exit 0; "register: pi: ok"
  -> <pkg>/mcp.json: mcpServers.longterm-mem = {"type":"stdio","command":"<ltm>","args":["mcp"]}
  -> sibling <pkg>/mcp.json.bak created (unchanged jsonInstall behaviour)
  -> pre-seeded ~/.pi/agent/mcp.json (pi-engram-owned) sha256 before == after: BYTE-IDENTICAL
     (e3ea3099ed2f471bf31c19bee22e0458ec52948d4cbcb546a5a6dec4d316a68e)
  -> ~/.pi/agent/agents/ never created

engine runtime status --target pi        (built + installed + registered)
  -> exit 0; "[pi] status: supported - labdrian-pi package is built, in sync, and listed in
     ~/.pi/agent/settings.json."

bin/labdrian-overlay sync-check --target pi   (immediately after register, .bak present)
  -> exit 0; "SYNC_CHECK:pi: no drift"; "VERDICT:pi:IN_SYNC"                  <-- C-02 FIXED
     (pre-remediation this exact state returned exit 1 / VERDICT:pi:DRIFT / "mcp.json.bak: extra")

engine pipkg build --overlay-root <w> --registry <reg> --dest-dir <pkg>   (rebuild over registered)
  -> exit 0; mcp.json     sha256 ce666c86... before == after (preserved)
             mcp.json.bak sha256 372a7f8c... before == after (preserved)     <-- C-02 Build side
  -> subsequent sync-check: VERDICT:pi:IN_SYNC exit 0

engine runtime status --target all
  -> exit 1 (honest: claude unsupported, opencode unsupported, codex partial)
  -> "[pi] status: supported - ..." line IS present in the aggregate         <-- W-03 FIXED
     Pi is no longer exempted; it is reported like every other target, and a genuinely
     supported Pi coexists with non-Pi failures that correctly fail the aggregate.

longterm-mem unregister --target pi --config-root <pkg>
  -> exit 0; "unregister: pi: removed"; mcp.json back to {"mcpServers": {}}
  -> status returns to honest `partial` exit 1; sync-check stays IN_SYNC exit 0 (.bak tolerated)

engine runtime uninstall --target pi
  -> exit 0; "[pi] uninstall: supported - removed via `pi remove <pkg>`; package directory deleted"
  -> stub argv log: "remove <pkg>" (verb is `remove`, never `uninstall`)
  -> settings.json packages: []; package directory gone; ~/.pi/agent/mcp.json byte-identical
```

### Spec Compliance Matrix

Statuses: COMPLIANT (covering test passed at runtime), PARTIAL (partially covered / deferred to
manual live-Pi), FAILING (covering evidence contradicts the scenario).

#### `specs/pi-runtime-target/spec.md` (7 requirements, 15 scenarios)

| Requirement | Scenario | Test / Evidence | Result |
|---|---|---|---|
| Pi Accepted as a Valid CLI Target | Pi target is recognized (apply, status, sync-check, uninstall) | `shelltest.TestResolveTargets_Pi`, `TestOverlayStatusAndSyncCheck_PiStubNoEmptyMkdir`, `TestPipkgHelpers_BuildStatusSyncCheck` + smoke | PARTIAL — this delta still names `uninstall` as a `labdrian-overlay` verb; no such command exists (W-02a) |
| Pi Accepted as a Valid CLI Target | Target all includes Pi without masking other failures | `cmd.TestRunRuntimeCore_AllTargetsIncludesPiWithoutMaskingOtherFailures` + smoke `status --target all` exit 1 with `[pi] supported` | COMPLIANT |
| Package-Delivered Skills and Agents | Local-path install registers the package idempotently | Smoke: stub `pi install` recorded, single entry in settings.json. Real `pi` idempotency is Pi's own behaviour | PARTIAL — deferred to manual 6.1 |
| Package-Delivered Skills and Agents | Skills and agents visible in a Pi session; nothing written to `~/.pi/agent/agents/` | Smoke: `~/.pi/agent/agents/` never created; package ships `skills/`, `agents/`. Session discovery needs a live Pi | PARTIAL — deferred to manual 6.1 |
| Deterministic Contract Gate | sdd-tasks and sdd-apply receive both contract path lines | `runtime.TestLabdrianGateInjectsPathLine_SddTasksSddApply` (node runs the real embedded source) | COMPLIANT |
| Deterministic Contract Gate | Every other agent is excluded | same test, excluded-agent and unnamed-agent cases | COMPLIANT |
| Deterministic Contract Gate | Composition with gentle-pi's own handler | `runtime.TestLabdrianGateChainsAfterGentlePi` (node present, ran for real) | COMPLIANT |
| Deterministic Contract Gate | Contract paths stay contained; malformed frontmatter yields no injection | `runtime.TestLabdrianGatePathContainment_RejectsTraversal` (4 cases) + malformed-sibling case | COMPLIANT |
| Deterministic Contract Gate | Extension is discovered from the package extensions directory | `pipkg.TestPipkgBuild_*` assert the embedded extension is copied; built package contains `extensions/labdrian-gate.ts`. Pi's own jiti discovery is manual | PARTIAL — deferred to 6.1/6.2; filename deviates from the spec text `extensions/gate.ts` (W-04) |
| Honest Status for Unproven Activation | Unproven entry forces partial | `runtime.TestPiAdapter_StatusTriangulatesAllThreeOwnedEntries` (3 cases, all three owned entries) + smoke: `partial` exit 1 with MCP unregistered | COMPLIANT — **was FAILING (C-01)** |
| Honest Status for Unproven Activation | All entries proven report supported | same test `all_three_entries_proven_reports_supported` + smoke: `supported` exit 0 only after register; `bin` surface now reports it too | COMPLIANT — **was FAILING (C-01/C-02/W-01)** |
| `--no-extensions` Limitation Disclosure | Disclosure text is present | `runtime.TestPiAdapter_StatusDisclosesNoExtensionsNoSkills`, shelltest assertion, smoke output on both surfaces | COMPLIANT |
| Pi-Scoped Uninstall | Only the owned package entry is removed | `runtime.TestPiAdapter_UninstallUsesRemoveNotUninstall`, `TestPiAdapter_UninstallNeverTouchesGentlePiFiles` + smoke (verb `remove`, settings entry gone, `~/.pi/agent/mcp.json` byte-identical, package dir deleted) | COMPLIANT |
| Pi Drift Detection via Sync-Check | No drift is reported when unchanged | `pipkg.TestPipkgCheck_IgnoresMcpJSONBak`, `TestPipkgBuild_PreservesRegisteredMcpJSONBak` + smoke: `VERDICT:pi:IN_SYNC` exit 0 with `.bak` present, and again after rebuild | COMPLIANT — **was FAILING (C-02)** |
| Pi Drift Detection via Sync-Check | A source edit is detected as drift | `pipkg.TestPipkgCheck_DetectsDrift`, `shelltest.TestPipkgHelpers_BuildStatusSyncCheck` tamper case | COMPLIANT |

#### `specs/runtime-lifecycle/spec.md` (3 requirements, 9 scenarios)

| Requirement | Scenario | Test / Evidence | Result |
|---|---|---|---|
| Target Aggregation | Target all includes Codex support | `cmd.TestRunRuntimeCore_AllTargetsRunsCodexLifecycleTogether` | COMPLIANT |
| Target Aggregation | Target all preserves non-Codex failures | `cmd.TestRunRuntimeCore_AllTargetsStatusFailsWhenClaudeOrOpenCodeFails` | COMPLIANT |
| Target Aggregation | Target all includes Pi support | `cmd.TestRunRuntimeCore_AllTargetsStatusAllowsCodexPartialWithoutFailing` (rewritten to prove Pi genuinely reaching `supported`), `TestRunRuntimeCore_AllTargetsStatusFailsWhenPiIsHonestlyUnsupported` + smoke `[pi] supported` in the aggregate | COMPLIANT — **was PARTIAL (W-03)** |
| Target Aggregation | Target all preserves non-Pi failures | `cmd.TestRunRuntimeCore_AllTargetsIncludesPiWithoutMaskingOtherFailures` + smoke (claude/opencode unsupported still fail the aggregate) | COMPLIANT |
| Existing Runtime Behavior Non-Regression | Legacy Claude commands still work | `cmd.TestRunRuntimeCore_ClaudeUpdateAndUninstallPreserveLegacyBehavior`; full engine suite green under `-race` | COMPLIANT |
| Existing Runtime Behavior Non-Regression | OpenCode lifecycle remains unchanged | `cmd.TestRunRuntimeCore_OpenCode*` (4 tests) | COMPLIANT |
| Existing Runtime Behavior Non-Regression | Codex lifecycle remains unchanged | `cmd.TestRunRuntimeCore_Codex*`, `shelltest.TestOverlayStatusAndSyncCheckAll_ClaudeOpenCodeCodexOutputUnchanged` | COMPLIANT |
| Pi Target Validation and Dispatch Without a Per-File Copy Path | Pi passes target validation without a copy-path entry | `shelltest.TestIsCopyTarget_ClaudeTrue_PiFalse`, `TestOverlayStatusAndSyncCheck_PiStubNoEmptyMkdir` | COMPLIANT |
| Pi Target Validation and Dispatch Without a Per-File Copy Path | Pi is present in the enum, aggregation, and adapter construction | `runtime.TestExpandTarget_Pi` + `NewFoundationAdapter` adapter-type assertion | COMPLIANT |

#### `specs/longterm-mem-mcp-registration/spec.md` (2 requirements, 9 scenarios)

| Requirement | Scenario | Test / Evidence | Result |
|---|---|---|---|
| MCP Registration — Pi | pi-engram's mcp.json stays untouched | `register.TestRegisterPi_*` (golden writer harness) + smoke: pre-seeded `~/.pi/agent/mcp.json` sha256 identical before and after register AND after uninstall | COMPLIANT |
| MCP Registration — Pi | The registration target is a package-relative mcp.json file | Smoke: `package.json` `"pi":{"mcp":"./mcp.json"}`; `mcp.json` top-level `mcpServers` with exactly one `longterm-mem` entry, no nesting | COMPLIANT |
| MCP Registration — Pi | The server name is package-prefixed | pi-mcp-adapter's own load-time prefixing; nothing in this change controls it | PARTIAL — external, deferred to manual 6.1 |
| MCP Registration — Pi | User/project config takes precedence | pi-mcp-adapter's own precedence rule; the overlay correctly implements nothing here | PARTIAL — external, satisfied by non-action |
| MCP Registration — Pi | pi-engram init does not drop the registration | The registration lives inside the package, not `~/.pi/agent/mcp.json`, so `pi-engram init` cannot reach it. No automated proof | PARTIAL — external, deferred to manual 6.1 |
| MCP Registration — Pi | Registration is skipped on expansion when the package is absent | `cmd/longterm-mem.TestCmdRegister_TargetAll_SkipsAbsentPi` | COMPLIANT |
| MCP Registration — Pi | Registration fails when Pi is named explicitly and the package is absent | `cmd/longterm-mem.TestCmdRegister_TargetPiExplicit_FailsWhenPackageAbsent` + smoke | COMPLIANT |
| Multi-Target Expansion Treats Pi Like the Other Runtimes | An expansion skips Pi when its package is not installed | `TestCmdRegister_TargetAll_SkipsAbsentPi`, `shelltest.TestLongtermMemUninstall_TargetAllIncludesPi` | COMPLIANT |
| Multi-Target Expansion Treats Pi Like the Other Runtimes | Pi named explicitly still fails without the package installed | `TestCmdRegister_TargetPiExplicit_FailsWhenPackageAbsent`, `shelltest.TestLongtermMemUninstall_TargetPiNoLongerDies` | COMPLIANT |

**Compliance summary**: 26/33 scenarios COMPLIANT, 7 PARTIAL, **0 FAILING**.
8/12 requirements fully compliant (was 5/12).

The 7 remaining PARTIALs are: 4 awaiting manual live-Pi tasks 6.1/6.2 (package idempotency,
session skill/agent discovery, jiti extension discovery, pi-engram init non-destruction),
2 governed entirely by pi-mcp-adapter and correctly not implemented here, and 1 spec-text
mismatch (W-02a). None is a code defect.

### Correctness (Static Evidence)

| Requirement | Status | Notes |
|---|---|---|
| R-001 Pi as CLI target | Implemented (partial) | `TARGET_KINDS`/`is_copy_target`/`is_valid_target`; no `uninstall` verb exists at the `bin` surface by design |
| R-002 Package delivery | Implemented | `engine/pipkg.Build`: registry-selected skills, `agents/`, embedded extension, temp-dir + atomic swap, symlink refusal, overlap guard |
| R-003/R-004 Contract gate | Implemented | `engine/pipkg/labdrian-gate.ts`, strict frontmatter parse, bare path line, chained `event.systemPrompt`, root containment |
| R-005 MCP registration | Implemented | `longterm-mem/internal/register/pi.go` reuses unmodified `jsonInstall(containerKey="mcpServers")` |
| R-006 Honest status | **Implemented** | `PiAdapter.Status` now builds `problems` from all three owned entries: in-sync (`pipkg.Check`), `isPiPackageListed`, `isPiMcpRegistered` (`engine/runtime/pi.go:110-128`). Both surfaces use it |
| R-007 Disclosure | Implemented | Static note, no runtime detection claim, no `-ns` alias, present on both surfaces |
| R-008 Non-copy dispatch | Implemented | No `TARGET_PATHS` entry for pi; no empty-path `mkdir` |
| R-009 Uninstall | Implemented | `pi remove <path>`, fixed argv, `exec.LookPath`/`LABDRIAN_PI_BIN`, package dir removed |
| R-010 Drift detection | **Implemented** | `Check` excludes both `mcp.json` and `mcp.json.bak` (`pipkg.go:185-196`); `Build` preserves both via the shared `preserveIfExists` helper (`pipkg.go:122-134`) |

### Coherence (Design)

| Decision (design.md) | Followed? | Notes |
|---|---|---|
| A1 — `pi.mcp` is a package-relative path; unmodified `jsonInstall` | Yes | Verified in the built package and in `register/pi.go` |
| A2 — `pi remove <source>`, not `pi uninstall <name>` | Yes | Stub argv log proves the verb |
| A3 — `--no-extensions` / `--no-skills`, no `-ns` alias | Yes | Disclosure text matches |
| A4 — chained `before_agent_start`, additive `systemPrompt` | Yes | `TestLabdrianGateChainsAfterGentlePi` |
| A5 — type-annotation-free `.ts`, tested via `.mjs` copy under node | Yes | File ships as `.ts`; node tests ran for real |
| C4/C5 — bare path line, reuse `runtime.InjectPrompt` as oracle | Yes | Byte-identical to the Go mirror |
| `TARGET_KINDS[pi]=package` + 8 call sites | Yes | Plus a disclosed 9th (`cmd_longterm_mem`) fixed in slice 1 |
| Honest status across package, extension and MCP (slice 5 goal) | Yes | Package + settings listing + MCP all probed. Extension load itself is still proxied by the in-sync check rather than probed directly — acceptable, since a drift-free package necessarily carries the extension file |
| Security F1–F3 — containment, strict parse, symlink refusal, atomic swap, 0644/0755 | Partial | Files 0644 and subdirectories 0755 as designed; package ROOT still ships `0700`. Re-confirmed observationally this run: `drwx------ labdrian-pi`. See W-05 |
| Threat matrix: process integration (fixed argv, LookPath) | Yes | `TestPiAdapter_InstallNoShellInjection` |
| Threat matrix: path containment (F1) | Yes | `TestLabdrianGatePathContainment_RejectsTraversal` |
| 5-slice table (Slice 1..5 file lists) | Yes, with disclosed deviations | Every slice's file list landed; deviations recorded in apply-progress |
| Open question — registry opt-in set for `install.targets: [pi]` | Resolved | `skills validate` exit 0, 36 skills aligned |

### Rulings on Findings Deferred from the Prior Verify

The orchestrator asked for an explicit follow-up-or-blocking ruling on W-04, W-05, W-06.

| # | Finding | Ruling |
|---|---|---|
| W-04 | Spec text says `extensions/gate.ts`; package ships `extensions/labdrian-gate.ts` | **Follow-up, archive-time.** The behavioural requirement (jiti-loaded `.ts`, not `.js`) is met and tested. Amend the delta spec sentence when `sdd-archive` merges the delta so the merged main spec is not wrong. Not blocking. |
| W-05 | Package root ships `0700`, design says `0755` for directories | **Follow-up.** Already filed as GitHub issue #312. Re-confirmed present this run. No spec scenario names file modes, so it is a design-coherence deviation only; `0700` is more restrictive than designed, so it is not a security regression. Not blocking. |
| W-06 | `apply --target pi` switches the worktree to `main` before building | **Follow-up, and worth its own issue.** Pre-existing script behaviour shared by every target, not introduced by this change. Pi is simply the first target whose `sync-check` can detect the resulting mismatch. Not blocking this change, but it should be filed so it is not later mistaken for a pipkg bug. Not re-exercised this run (running `apply` would have mutated the branch checkout, which this phase is forbidden to do). |

### Issues Found

**CRITICAL**: None. Both prior criticals (C-01, C-02) are independently verified as fixed.

**WARNING**

- **W-02a — The W-02 spec fix landed in the wrong file; the authoritative delta is unchanged.**
  Commit `f8ae0c6` corrected `openspec/specs/runtime-lifecycle/spec.md` (the already-merged main
  spec). Its message states that the change's own delta "does not contain these two requirements".
  That is true of `openspec/changes/pi-runtime-target/specs/runtime-lifecycle/spec.md`, which was
  the only delta file checked — but the two requirements live in a **different** delta file,
  `openspec/changes/pi-runtime-target/specs/pi-runtime-target/spec.md`, which still reads:
  - line 15: "MUST accept `pi` as a valid value for `--target` on `apply`, `status`, `sync-check`,
    and `uninstall`"
  - line 125: "`uninstall --target pi` MUST remove only the `labdrian-pi` package entry ... and
    MUST deregister the longterm-mem MCP entry"

  Two consequences. First, the finding W-02 named is not actually fixed in the artifact that
  governs this change. Second, the delta and the main spec now **disagree**: main says uninstall
  is `engine runtime uninstall --target pi` and that it removes the package directory; the delta
  says it is a `labdrian-overlay` verb that deregisters the MCP entry. Fix direction: apply the
  same correction to the delta at lines 14-18 and 123-136.

- **W-08 — Six Pi requirements exist in duplicate, and archive would create a contradictory copy.**
  `openspec/specs/runtime-lifecycle/spec.md` already contains `Pi Accepted as a Valid CLI Target`,
  `Package-Delivered Skills and Agents`, `Honest Status for Unproven Activation`,
  `` `--no-extensions` Limitation Disclosure ``, `Pi-Scoped Uninstall`, and
  `Pi Drift Detection via Sync-Check` — written directly into the merged main spec by task 5.4
  rather than through a delta. The delta `specs/pi-runtime-target/spec.md` carries its own copies
  of all six (plus the delta-only `Deterministic Contract Gate`), and it has **no**
  `## ADDED`/`## MODIFIED` section headers, so `sdd-archive` will merge it into a new
  `openspec/specs/pi-runtime-target/spec.md`. The result would be the same six requirements in two
  domain files, disagreeing on the uninstall verb (W-02a). `sdd-archive` must reconcile this
  deliberately — either amend the delta and let it own the Pi domain while removing the Pi block
  from `runtime-lifecycle`, or drop the duplicated requirements from the delta. This is
  bookkeeping, not a code defect, but archiving without a decision produces a self-contradictory
  source of truth.

- **W-04 — Spec text `extensions/gate.ts` vs shipped `extensions/labdrian-gate.ts`.** Ruled
  follow-up (archive-time) above. Carried forward unresolved; explicitly out of remediation scope.

- **W-05 — Package root ships `0700`, not the design's `0755`.** Ruled follow-up above
  (issue #312). Carried forward unresolved; explicitly out of remediation scope.

- **W-06 — `apply --target pi` builds from `main`, not the checked-out branch.** Ruled follow-up
  above. Carried forward unresolved; explicitly out of remediation scope.

**SUGGESTION**

- **S-01 — Two stale "slice 2" strings remain in `bin/labdrian-overlay`.**
  `package_target_stub_message` (line 153) and the sync-check branch (line 2525) both print
  "package target, handled by slice 2 (pi-package-build)". Slice 2 landed long ago. Reword to
  describe what a package target actually is. Same class as the W-01 text that was just fixed.
- **S-02 — The `supported` status message does not mention the MCP entry it now proves.**
  `engine/runtime/pi.go:124-125` still reads "built, in sync, and listed in
  ~/.pi/agent/settings.json". Now that MCP registration is a third probed entry, the success
  message under-reports what was verified, while the `partial` message names it correctly.
- **S-03 — Manual tasks 6.1–6.3 remain deferred to post-chain manual verification, not failures.**
  Three PARTIAL scenarios rest on them. With C-01 and C-02 now closed, that manual pass is
  meaningful rather than a debugging session — this is the right moment to run it.

### TDD Compliance

| Check | Result | Details |
|---|---|---|
| TDD Evidence reported | Yes | 5 slice tables plus 2 remediation tables in apply-progress |
| All tasks have tests | Yes | 23/23 completed tasks map to a named test file; 5.4 and W-02 are docs/spec-only |
| RED confirmed (test files exist) | Yes | Every named file located, including all 5 remediation tests |
| RED evidence is real, not asserted | Yes | Remediation 1 records the pre-fix failure strings (`got: supported, want partial`; `mcp.json.bak: extra`; `no such file`); Remediation 2 records `got 0, want 1` for the aggregate exemption and the pre-fix "no drift"/exit 0 for the bash delegation. These are falsifiable, specific records |
| GREEN confirmed (tests pass now) | Yes | All 5 remediation tests re-executed verbosely here; full suites green under `-race` |
| Triangulation adequate | Yes | C-01's replacement test triangulates all 3 owned entries — the exact structural gap the prior verify identified. C-02 uses 2 independent RED tests (Check side and Build side). W-01 uses a 2-case delegation proof. W-02's skipped triangulation is correctly justified (spec-text-only) |
| Safety net for modified files | Yes | Both remediation batches record the relevant suite green before edit |
| Budget discipline honest | Yes | Remediation 1 reverted a proven-correct W-03 fix rather than commit 6 lines over budget, disclosed it, and Remediation 2 re-applied it under a fresh budget. That is the correct behaviour and it was reported transparently |

**TDD Compliance**: 8/8 checks passed.

### Test Layer Distribution

| Layer | Tests | Files | Tools |
|---|---|---|---|
| Unit (Go) | 25 pi-specific | `engine/pipkg/pipkg_test.go` (9), `engine/runtime/pi_test.go` (5 adapter), `engine/cmd/pipkg_test.go` (3), `engine/cmd/runtime_test.go` (4 pi), `longterm-mem/internal/register/pi_test.go` (4) | `go test` |
| Integration — bash | 8 | `engine/shelltest/overlay_pi_target_test.go` (6), `overlay_pi_package_build_test.go` (2) — source the real `bin/labdrian-overlay` against a real built engine binary | `go test` + bash |
| Integration — node | 3 | `engine/runtime/pi_test.go` — run the real embedded `labdrian-gate.ts` under node | `go test` + node |
| Integration — CLI | 3 | `longterm-mem/cmd/longterm-mem/main_test.go` pi cases | `go test` |
| E2E (live Pi) | 0 | Tasks 6.1–6.3 — no headless Pi runner exists | Manual |
| **Total pi-specific** | **39** | **8 files** | |

### Changed File Coverage

Coverage analysis skipped — no coverage tool or threshold is configured for this repository, and
none was required by `entry.json`. Not a failure.

### Assertion Quality

Re-audited the pi-related test files, with focus on the 5 tests added by remediation. No
tautologies, no orphan empty-collection assertions, no type-only assertions used alone, no ghost
loops, no smoke-test-only cases, no mock-heavy files. The suite uses real filesystem temp trees,
real subprocesses, and real node execution rather than mocks. Assertions compare concrete expected
strings, exit codes, file bytes, sha256 digests, and recorded argv.

The prior verify raised two assertion-quality notes. Both are now addressed:
- `TestPiAdapter_StatusPartialOnUnprovenEntry`'s single-entry coverage is superseded by
  `TestPiAdapter_StatusTriangulatesAllThreeOwnedEntries`, which varies expectations across cases
  (two distinct `partial` reasons and one `supported`) rather than asserting one value repeatedly.
- The missing register-against-a-built-package interaction is now covered from both sides by
  `TestPipkgCheck_IgnoresMcpJSONBak` and `TestPipkgBuild_PreservesRegisteredMcpJSONBak`, and end
  to end by the smoke sequence above.

**Assertion quality**: All assertions verify real behavior.

### Quality Metrics

**Formatter**: `gofmt -l` clean in both modules.
**Vet**: `go vet ./...` clean in both modules.
**Linter (shell)**: `shellcheck -S warning` reports only the 2 pre-existing SC2064 warnings.
**Race detector**: `go test -race` clean across all 13 engine packages.

### Verdict

**FAIL — not archive-ready.** 0 CRITICAL, 5 WARNING, 3 SUGGESTION, 2 blockers to archive.

Read the verdict precisely: **nothing in the implementation is broken.** Both prior CRITICAL
findings are independently verified as fixed, not merely claimed. The Pi status surface now probes
all three owned entries and returns an honest `partial` in the exact state that previously returned
`supported`, and a correctly-registered package now reports `VERDICT:pi:IN_SYNC` where it
previously reported permanent `DRIFT`. W-01 and W-03 are likewise genuinely fixed and proven at
runtime. Scenario compliance rose from 22/33 to 26/33 and **zero FAILING scenarios remain**. All
tests, vet, gofmt, race detector, shellcheck, and skills validation pass.

The `fail` verdict records that the change is not yet archive-ready, for two blockers:

1. **Evidence completeness.** 7 of 33 scenarios are still PARTIAL, and tasks 6.1-6.3 are unchecked.
   Four PARTIAL scenarios (package install idempotency, live-session skill/agent discovery, jiti
   extension discovery, pi-engram init non-destruction) can only be closed by the deliberate manual
   live-Pi pass those tasks describe. Two more are governed entirely by pi-mcp-adapter and are
   correctly satisfied by non-action here. These were deferred by design from the start, not
   failed — but a passing verification cannot assert evidence that does not yet exist.
2. **OpenSpec artifact reconciliation.** W-02 was fixed in the merged main spec but not in the
   delta that governs this change (W-02a), the delta duplicates six requirements that already
   exist in the main runtime-lifecycle spec (W-08), and the delta's `extensions/gate.ts` text still
   contradicts the shipped filename (W-04). `sdd-archive` merges deltas into the source of truth,
   so merging as-is would produce a self-contradictory spec.

W-05 (root `0700`, issue #312) and W-06 (`apply` builds from `main`) are confirmed follow-ups and
do not block.

**Recommended next step**: this does not need a return to `sdd-apply` for code. It needs (a) the
three delta spec-text corrections, which are edits to
`openspec/changes/pi-runtime-target/specs/pi-runtime-target/spec.md`, and (b) the manual live-Pi
pass for tasks 6.1-6.3. With C-01 and C-02 now closed, that manual pass is finally meaningful
rather than a debugging session. Re-verify afterwards to convert the remaining PARTIALs.
