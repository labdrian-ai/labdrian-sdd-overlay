```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:c6899165c39e4d2aaac503f5904afa409c6d4e8a2f6f698e6ee312418382d006
verdict: fail
blockers: 2
critical_findings: 2
requirements: 5/12
scenarios: 22/33
test_command: cd engine && gofmt -l . && go vet ./... && go test -count=1 -race ./...
test_exit_code: 0
test_output_hash: sha256:6052ec68dfc9092947e7851075c3a0f241c6a813e0a30b112e63a16644cfa96c
build_command: cd engine && go build -o /tmp/claude-1000/-home-labdrian-labdrian-sdd-overlay/54b233bf-75b3-4ffe-9efb-d4c8015a06cf/scratchpad/verify/home/.claude/bin/gentle-ai-overlay ./cmd
build_exit_code: 0
build_output_hash: sha256:6f737696d0212af721d0a134cd0c860389afcf20a47b94325c37584d2fd3fa05
```

## Verification Report

**Change**: pi-runtime-target
**Version**: N/A (delta specs under `openspec/changes/pi-runtime-target/specs/`)
**Mode**: Strict TDD
**Worktree**: `/home/labdrian/labdrian-sdd-overlay-worktrees/pi-5`, branch `feat/pi-runtime-target-5-lifecycle`, HEAD `fc610a4`, tree clean before and after verification.

### Completeness

| Metric | Value |
|--------|-------|
| Tasks total (1.1-6.4) | 26 |
| Tasks complete (1.1-5.4) | 22 |
| Tasks incomplete | 4 (6.1, 6.2, 6.3 manual-only live-Pi; 6.4 executed here but checkbox still unchecked) |
| Requirements | 12 (pi-runtime-target 7, runtime-lifecycle 3, longterm-mem-mcp-registration 2) |
| Scenarios | 33 (15 + 9 + 9) |
| Planned slices (entry.json `review_slices`) | 5 |
| Realized slices | 5 (all committed: 9faf011/eb8c14c, acc746b/8be8a55, 9201aaf/a5ab38c, da88436/de435cb, a3fee0e/7bf7abe/fc610a4) |
| Plan-vs-realized drift | None — P = R = 5 |

Scenario totals were recounted from the three delta spec files, not taken from the orchestrator's stated 15+9+9; the recount agrees.

### Build & Tests Execution

**Build**: PASSED

```text
cd engine && go build -o <scratch>/gentle-ai-overlay ./cmd  -> exit 0, 5051582-byte binary
cd longterm-mem && go build -o <scratch>/longterm-mem ./cmd/longterm-mem -> exit 0
```

**Tests**: PASSED (all packages, both modules)

```text
cd engine && gofmt -l .                    -> (empty), exit 0
cd engine && go vet ./...                  -> exit 0
cd engine && go test -count=1 -race ./...  -> exit 0; ok: assets cmd gadu gate installer pipkg
                                              prespec propagator runtime settings shelltest
                                              skills synctrigger

cd longterm-mem && gofmt -l .              -> (empty), exit 0
cd longterm-mem && go vet ./...            -> exit 0
cd longterm-mem && go test -count=1 ./...  -> exit 0; ok: root cmd/longterm-mem durable embed
                                              engram identityledger mcpserver ops projectid
                                              promote query register repohistory staleness
                                              vault vaultreg vecindex

shellcheck -S warning bin/labdrian-overlay -> exit 1; 2 x SC2064 at lines 1448 and 1609
                                              (pre-existing, known environmental; the brief
                                              said 3 — 2 observed)

bin/labdrian-overlay skills validate --registry skills.registry.yaml --manifest overlay.manifest
  --source-root skills                     -> exit 0
                                              "registry and manifest aligned (36 skills)"
                                              "skills/ on disk matches overlay.manifest (74 files)"
                                              (run against a FRESHLY BUILT engine binary placed at
                                              <scratch>/home/.claude/bin/gentle-ai-overlay with
                                              HOME=<scratch>/home; the script derives ENGINE_BINARY
                                              from $HOME and has no override env var)
```

**Coverage**: Not collected — no coverage gate configured for this change.

### Runtime Smoke Evidence (scratch STATE_DIR, scratch HOME, stub `pi` via `LABDRIAN_PI_BIN`)

The real `~/.npm-global/bin/pi` was never invoked. The stub recorded argv and mutated only a
scratch `~/.pi/agent/settings.json`. Nothing was written under the user's real `$HOME`
(verified: `/home/labdrian/.labdrian-overlay/pi` does not exist).

```text
engine runtime install --target pi
  -> exit 0; "[pi] install: restart_required - ran `pi install <pkg>`; start a new Pi session"
  -> stub argv log: "install <pkg>"; settings.json packages: ["<pkg>"]
  -> package tree: package.json, mcp.json, agents/GADU.md, extensions/labdrian-gate.ts, skills/

bin/labdrian-overlay apply --target pi
  -> exit 0, deploy complete. NOTE: the script switches the worktree to `main`, merges upstream,
     builds, then switches back. The package therefore reflects `main`'s tree, not the current
     branch, so the immediately following sync-check reported DRIFT (many skills "missing").
     Pre-existing script behaviour, not introduced by this change, but it makes
     `apply --target pi` on a feature branch produce a package that does not match that branch.

bin/labdrian-overlay status --target pi
  -> exit 0
  -> "pi: note -- 'pi --no-extensions' disables the before_agent_start contract-gate extension
      for that session, and 'pi --no-skills' disables skill discovery, for that session only;
      neither flag's use is detected at runtime, and there is no short alias for either flag"
  -> "pi: built at <pkg>, no drift (partial -- lifecycle proof lands in a later slice)"

bin/labdrian-overlay sync-check --target pi   (freshly built, no MCP registration)
  -> exit 0; "SYNC_CHECK:pi: no drift"; "VERDICT:pi:IN_SYNC"

engine runtime status --target pi             (built + `pi install`ed, mcp.json = {"mcpServers": {}})
  -> exit 0; "[pi] status: supported - labdrian-pi package is built, in sync, and listed in
     ~/.pi/agent/settings.json. ..."                                <-- CRITICAL-1 evidence

longterm-mem register --target pi --config-root <pkg>   (package NOT listed in settings.json)
  -> exit 1; "register: pi: not installed (no package at <pkg>, or not listed in
     ~/.pi/agent/settings.json); build it ... and run pi install first"

longterm-mem register --target pi --config-root <pkg>   (package listed)
  -> exit 0; "register: pi: ok"
  -> <pkg>/mcp.json: {"mcpServers": {"longterm-mem": {"type":"stdio","command":"<ltm>","args":["mcp"]}}}
  -> a sibling <pkg>/mcp.json.bak is created                        <-- CRITICAL-2 evidence
  -> pre-seeded ~/.pi/agent/mcp.json (pi-engram-owned) sha256 before == after: BYTE-IDENTICAL
  -> ~/.pi/agent/agents/ never created

bin/labdrian-overlay sync-check --target pi   (immediately after the register above)
  -> exit 1; "SYNC_CHECK:pi: drift -- pipkg check: labdrian-pi package drift:"
  -> "VERDICT:pi:DRIFT"                                             <-- CRITICAL-2 evidence
engine runtime status --target pi             (same state)
  -> exit 1; "[pi] status: partial - labdrian-pi status is unproven: in sync (labdrian-pi
     package drift:\n  mcp.json.bak: extra)."

pipkg build (rebuild over a registered package)
  -> exit 0; mcp.json content preserved byte-for-byte across the atomic swap

longterm-mem unregister --target pi --config-root <pkg>
  -> exit 0; "unregister: pi: removed"; mcp.json back to {"mcpServers": {}}
     (leaves mcp.json.bak behind, same CRITICAL-2 mechanism)

engine runtime uninstall --target pi
  -> exit 0; "[pi] uninstall: supported - removed via `pi remove <pkg>`; package directory deleted"
  -> stub argv log: "remove <pkg>" (verb is `remove`, never `uninstall`)
  -> settings.json packages: []; package directory gone; ~/.pi/agent/mcp.json untouched
```

### Spec Compliance Matrix

Statuses: COMPLIANT (covering test passed at runtime), PARTIAL (partially covered / deferred to
manual live-Pi), FAILING (covering evidence contradicts the scenario).

#### `specs/pi-runtime-target/spec.md` (7 requirements, 15 scenarios)

| Requirement | Scenario | Test / Evidence | Result |
|---|---|---|---|
| Pi Accepted as a Valid CLI Target | Pi target is recognized (apply, status, sync-check, uninstall) | `shelltest.TestResolveTargets_Pi`, `TestOverlayStatusAndSyncCheck_PiStubNoEmptyMkdir`, `TestPipkgHelpers_BuildStatusSyncCheck` + smoke apply/status/sync-check | PARTIAL — `bin/labdrian-overlay` has no `uninstall` command at all (see W-02) |
| Pi Accepted as a Valid CLI Target | Target all includes Pi without masking other failures | `cmd.TestRunRuntimeCore_AllTargetsIncludesPiWithoutMaskingOtherFailures`, `TestRunRuntimeCore_AllTargetsNonStatusActionsFailWhenPiUnsupported` | COMPLIANT |
| Package-Delivered Skills and Agents | Local-path install registers the package idempotently | Smoke: stub `pi install` recorded, single entry in settings.json. Real `pi` idempotency is Pi's own behaviour | PARTIAL — deferred to manual task 6.1 |
| Package-Delivered Skills and Agents | Skills and agents are visible in a Pi session; nothing written to `~/.pi/agent/agents/` | Smoke: `~/.pi/agent/agents/` never created; package ships `skills/`, `agents/GADU.md`. Session discovery needs a live Pi | PARTIAL — deferred to manual task 6.1 |
| Deterministic Contract Gate | sdd-tasks and sdd-apply receive both contract path lines | `runtime.TestLabdrianGateInjectsPathLine_SddTasksSddApply` (node runs the real embedded source; Go-side oracle `runtime.CanonicalEntry`/`InjectPrompt`) | COMPLIANT |
| Deterministic Contract Gate | Every other agent is excluded | same test, excluded-agent and unnamed-agent cases | COMPLIANT |
| Deterministic Contract Gate | Composition with gentle-pi's own handler | `runtime.TestLabdrianGateChainsAfterGentlePi` (node v22 present, ran for real) | COMPLIANT |
| Deterministic Contract Gate | Contract paths stay contained; malformed frontmatter yields no injection | `runtime.TestLabdrianGatePathContainment_RejectsTraversal` (4 cases) + malformed-sibling case | COMPLIANT |
| Deterministic Contract Gate | Extension is discovered from the package extensions directory | `pipkg.TestPipkgBuild_*` assert the embedded extension is copied; built package contains `extensions/labdrian-gate.ts`. Pi's own jiti discovery is manual | PARTIAL — deferred to 6.1/6.2; filename deviates from the spec text `extensions/gate.ts` (W-04) |
| Honest Status for Unproven Activation | Unproven entry forces partial | `runtime.TestPiAdapter_StatusPartialOnUnprovenEntry` passes, but covers only the settings.json-listing entry. Smoke proves `supported` is returned with longterm-mem MCP unregistered | FAILING — C-01 |
| Honest Status for Unproven Activation | All entries proven report supported | `engine runtime status --target pi` can report `supported`, but only because MCP visibility is unproven; after the documented register flow it reports `partial`. `bin/labdrian-overlay status --target pi` can never report `supported` | FAILING — C-01, C-02, W-01 |
| `--no-extensions` Limitation Disclosure | Disclosure text is present | `runtime.TestPiAdapter_StatusDisclosesNoExtensionsNoSkills`, shelltest disclosure assertion, smoke output | COMPLIANT |
| Pi-Scoped Uninstall | Only the owned package entry is removed | `runtime.TestPiAdapter_UninstallUsesRemoveNotUninstall`, `TestPiAdapter_UninstallNeverTouchesGentlePiFiles` + smoke (verb `remove`, settings entry gone, `~/.pi/agent/mcp.json` untouched, package dir deleted so the MCP registration goes with it) | COMPLIANT |
| Pi Drift Detection via Sync-Check | No drift is reported when unchanged | `pipkg.TestPipkgCheck_DetectsDrift` and shelltest pass for a freshly built package; smoke proves DRIFT after the design's own `register --target pi` step | FAILING — C-02 |
| Pi Drift Detection via Sync-Check | A source edit is detected as drift | `pipkg.TestPipkgCheck_DetectsDrift`, `shelltest.TestPipkgHelpers_BuildStatusSyncCheck` tamper case | COMPLIANT |

#### `specs/runtime-lifecycle/spec.md` (3 requirements, 9 scenarios)

| Requirement | Scenario | Test / Evidence | Result |
|---|---|---|---|
| Target Aggregation | Target all includes Codex support | `cmd.TestRunRuntimeCore_AllTargetsRunsCodexLifecycleTogether` | COMPLIANT |
| Target Aggregation | Target all preserves non-Codex failures | `cmd.TestRunRuntimeCore_AllTargetsStatusFailsWhenClaudeOrOpenCodeFails` | COMPLIANT |
| Target Aggregation | Target all includes Pi support | `cmd.TestRunRuntimeCore_AllTargetsStatusAllowsCodexPartialWithoutFailing` exits 0 with Pi present, but Pi is `unsupported` and masked by the status-only aggregate exemption, not "succeeding or honest partial"; no test asserts a `[pi]` line in a succeeding aggregate | PARTIAL — W-03 |
| Target Aggregation | Target all preserves non-Pi failures | `cmd.TestRunRuntimeCore_AllTargetsIncludesPiWithoutMaskingOtherFailures` | COMPLIANT |
| Existing Runtime Behavior Non-Regression | Legacy Claude commands still work | `cmd.TestRunRuntimeCore_ClaudeUpdateAndUninstallPreserveLegacyBehavior`; full engine suite green under `-race` | COMPLIANT |
| Existing Runtime Behavior Non-Regression | OpenCode lifecycle remains unchanged | `cmd.TestRunRuntimeCore_OpenCode*` (4 tests) | COMPLIANT |
| Existing Runtime Behavior Non-Regression | Codex lifecycle remains unchanged | `cmd.TestRunRuntimeCore_Codex*`, `shelltest.TestOverlayStatusAndSyncCheckAll_ClaudeOpenCodeCodexOutputUnchanged` | COMPLIANT |
| Pi Target Validation and Dispatch Without a Per-File Copy Path | Pi passes target validation without a copy-path entry | `shelltest.TestIsCopyTarget_ClaudeTrue_PiFalse`, `TestOverlayStatusAndSyncCheck_PiStubNoEmptyMkdir` | COMPLIANT |
| Pi Target Validation and Dispatch Without a Per-File Copy Path | Pi is present in the enum, aggregation, and adapter construction | `runtime.TestExpandTarget_Pi` + `NewFoundationAdapter` adapter-type assertion in `runtime_test.go` | COMPLIANT |

#### `specs/longterm-mem-mcp-registration/spec.md` (2 requirements, 9 scenarios)

| Requirement | Scenario | Test / Evidence | Result |
|---|---|---|---|
| MCP Registration — Pi | pi-engram's mcp.json stays untouched | `register.TestRegisterPi_*` (golden writer harness, 5 fixtures) + smoke: pre-seeded `~/.pi/agent/mcp.json` sha256 identical before and after register | COMPLIANT |
| MCP Registration — Pi | The registration target is a package-relative mcp.json file | Smoke: `package.json` = `"pi":{"mcp":"./mcp.json"}`; `mcp.json` top-level `mcpServers` with exactly one `longterm-mem` entry, no nesting. Ownership is the shared install-state fingerprint model (`register/doc.go` D9), the same "ownership-tagged" idiom claude/opencode/codex use | COMPLIANT |
| MCP Registration — Pi | The server name is package-prefixed | pi-mcp-adapter's own load-time prefixing; nothing in this change controls it | PARTIAL — external, deferred to manual 6.1 |
| MCP Registration — Pi | User/project config takes precedence | pi-mcp-adapter's own precedence rule; the overlay correctly implements nothing here | PARTIAL — external, satisfied by non-action |
| MCP Registration — Pi | pi-engram init does not drop the registration | The registration lives inside the package, not `~/.pi/agent/mcp.json`, so `pi-engram init` cannot reach it. No automated proof | PARTIAL — external, deferred to manual 6.1 |
| MCP Registration — Pi | Registration is skipped on expansion when the package is absent | `cmd/longterm-mem.TestCmdRegister_TargetAll_SkipsAbsentPi` | COMPLIANT |
| MCP Registration — Pi | Registration fails when Pi is named explicitly and the package is absent | `cmd/longterm-mem.TestCmdRegister_TargetPiExplicit_FailsWhenPackageAbsent` + smoke (exit 1, "not installed") | COMPLIANT |
| Multi-Target Expansion Treats Pi Like the Other Runtimes | An expansion skips Pi when its package is not installed | `TestCmdRegister_TargetAll_SkipsAbsentPi`, `shelltest.TestLongtermMemUninstall_TargetAllIncludesPi` | COMPLIANT |
| Multi-Target Expansion Treats Pi Like the Other Runtimes | Pi named explicitly still fails without the package installed | `TestCmdRegister_TargetPiExplicit_FailsWhenPackageAbsent`, `shelltest.TestLongtermMemUninstall_TargetPiNoLongerDies` | COMPLIANT |

**Compliance summary**: 22/33 scenarios COMPLIANT, 8 PARTIAL, 3 FAILING. 5/12 requirements fully compliant.

### Correctness (Static Evidence)

| Requirement | Status | Notes |
|---|---|---|
| R-001 Pi as CLI target | Implemented (partial) | `TARGET_KINDS`/`is_copy_target`/`is_valid_target` in `bin/labdrian-overlay`; no `uninstall` verb exists |
| R-002 Package delivery | Implemented | `engine/pipkg.Build`: registry-selected skills, `agents/`, embedded extension, temp-dir + atomic swap, symlink/special-file refusal, overlap guard |
| R-003/R-004 Contract gate | Implemented | `engine/pipkg/labdrian-gate.ts`, strict frontmatter parse, bare path line under `injection_point`, chained `event.systemPrompt`, root containment |
| R-005 MCP registration | Implemented | `longterm-mem/internal/register/pi.go` reuses unmodified `jsonInstall(containerKey="mcpServers")`; `cmd_register.go` skip/fail probe |
| R-006 Honest status | Partially implemented | `PiAdapter.Status` (engine/runtime/pi.go:103) proves build+in-sync and settings.json listing only; MCP visibility never probed; the `bin` surface does not use it at all |
| R-007 Disclosure | Implemented | Static note, no runtime detection claim, no `-ns` alias, present on both surfaces |
| R-008 Non-copy dispatch | Implemented | No `TARGET_PATHS`/`AGENT_TARGET_PATHS` entry for pi; no empty-path `mkdir` |
| R-009 Uninstall | Implemented | `pi remove <path>`, fixed argv, `exec.LookPath`/`LABDRIAN_PI_BIN`, package dir removed |
| R-010 Drift detection | Partially implemented | Content-diff `Check` works; excludes `mcp.json` but not the `mcp.json.bak` its own registration writer creates |

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
| Security F1-F3 — containment, strict parse, symlink refusal, atomic swap, 0644/0755 | Partial | Files 0644 and subdirectories 0755 as designed, but the package ROOT ships `0700` (`os.MkdirTemp` mode survives the rename). Design says 0755 dirs. Matches issue #312 |
| Threat matrix: process integration (fixed argv, LookPath) | Yes | `TestPiAdapter_InstallNoShellInjection` |
| Threat matrix: path containment (F1) | Yes | `TestLabdrianGatePathContainment_RejectsTraversal` |
| Threat matrix: documentation-like paths / git selection / commit-push-PR = N/A | Yes | No git operations introduced by the change; the `apply` branch switch is pre-existing script behaviour |
| 5-slice table (Slice 1..5 file lists) | Yes, with disclosed deviations | Every slice's file list landed; deviations are all recorded in apply-progress |
| Open question — registry opt-in set for `install.targets: [pi]` | Resolved | `skills validate` exit 0, 36 skills aligned; the package ships the pi-targeted subset |

### Disclosed Deviations — Rulings

| # | Disclosed deviation | Ruling |
|---|---|---|
| D1 | Slice 1: RED tests split into `engine/shelltest/` because `resolve_targets`/`is_copy_target` are bash | Acceptable deviation — the tests exist, run the real script, and are green |
| D2 | Slice 1: `cmd_longterm_mem` regression fix outside the 8-site list | Acceptable deviation — a genuine bug this slice caused, fixed and covered |
| D3 | Slice 1: `--target all` aggregate exemption for Pi in `cmd/main.go` | **Now a gap** — the code comment says the exemption "is removed once pi-lifecycle" lands. pi-lifecycle landed in slice 5; the exemption is still live. See W-03 |
| D4 | Slice 2: `Build` resolves version internally, no 4th parameter | Acceptable deviation — matches the design's own 3-arg Data Flow signature |
| D5 | Slice 2: `Apply`/`Install` report `CapabilityPartial` on build-only | Superseded by slice 5, which makes `Apply() == Install()` and runs `pi install` |
| D6 | Slice 3: `extensions/labdrian-gate.ts` vs the spec's `extensions/gate.ts` | Acceptable deviation, but the spec text is now wrong. See W-04 |
| D7 | Slice 3: `skills/_shared/*.md` copied unconditionally, not registry-driven | Acceptable deviation — gate infrastructure, not an installable skill; no spec scenario governs it |
| D8 | Slice 3: containment/injection tests are node-driven against the real source instead of a Go mirror | Acceptable deviation, and better: it tests the artifact Pi actually loads |
| D9 | **Slice 4: `status` parity scoped down** (`cmd_longterm_mem status` does not consult `$targets`) | Acceptable for the longterm-mem `status` subcommand specifically. **But** the related `bin/labdrian-overlay status --target pi` surface is a genuine **spec gap** — see W-01 |
| D10 | Slice 4: `piInstalled` probe lives in `longterm-mem`, not `engine` | Acceptable deviation — D4 module boundary, the caller owns it |
| D11 | Slice 4: two slice-1 shelltests rewritten to assert the new behaviour | Acceptable — corrective, not scope creep; the old assertions would have lied |
| D12 | **Slice 5: no top-level `uninstall` command in `bin/labdrian-overlay`** | Acceptable deviation in substance (no target has such a command; `engine runtime uninstall --target pi` is real and proven), but R-001 literally names `uninstall` as a `labdrian-overlay` verb, so it is also a **spec-text gap** — see W-02 |
| D13 | Slice 5: `Update`/`Rollback` stay rebuild-only partial | Acceptable deviation — no spec scenario covers them |

### Independent Validator Follow-ups (GitHub issue #312) — Rulings

| Finding | Ruling |
|---|---|
| `pipkg.Check` is mode-blind | **Follow-up, not a spec violation.** R-010 requires drift detection "using the same idiom as the GADU generator's drift check", which is content-based. No scenario names file modes |
| Registry `path` is never validated (`path: ../x` escapes `skills/`) | **Follow-up, not a spec violation.** The only containment scenario in the specs governs contract paths inside the gate extension, which IS enforced. Security-relevant; recommend it is not left open indefinitely |
| Package root ships 0700 | **Follow-up and a design deviation** (design says 0755 dirs). No spec scenario names modes, so not a spec violation. Confirmed observationally: `drwx------ labdrian-pi` |
| SKILL.md `name` != directory name ships silently | **Follow-up, not a spec violation.** No scenario requires build-time name validation |
| Release coupling (registry + engine must ship together) | **Follow-up, not a spec violation.** Release-process concern |

### TDD Compliance

| Check | Result | Details |
|---|---|---|
| TDD Evidence reported | Yes | 5 "TDD Cycle Evidence" tables in apply-progress, one per slice |
| All tasks have tests | Yes | 22/22 implementation tasks map to a named test file; 5.4 is docs-only |
| RED confirmed (test files exist) | Yes | Every named file exists: `engine/runtime/pi_test.go`, `engine/pipkg/pipkg_test.go`, `engine/cmd/pipkg_test.go`, `engine/shelltest/overlay_pi_target_test.go`, `engine/shelltest/overlay_pi_package_build_test.go`, `longterm-mem/internal/register/pi_test.go`, `longterm-mem/cmd/longterm-mem/main_test.go` |
| RED evidence is real, not asserted | Yes | Slice 1 reverted `runtime.go`/`pi.go` to HEAD and recorded the build failure; slice 2 recorded "no non-test Go files"; slice 5 recorded `git stash` on `pi.go` with all 5 new tests failing. These are falsifiable RED records, not "I wrote the test first" claims |
| GREEN confirmed (tests pass now) | Yes | Every named test re-executed here under `go test -count=1 -race ./...`, exit 0 |
| Triangulation adequate | Yes | Multi-case tables throughout (4 targets, 4 containment cases, 6 gate states, 5 pipkg states, 3 status cases). The one "single" case (skills `validTargets` enum addition) is genuinely structural |
| Safety net for modified files | Yes | Each slice records the pre-existing suite green before modification |
| Independent RED verification | Not attempted | Re-running RED would require reverting source; the recorded evidence is specific and falsifiable enough to accept |

**TDD Compliance**: 7/7 checks passed.

### Test Layer Distribution

| Layer | Tests | Files | Tools |
|---|---|---|---|
| Unit (Go) | 20 pi-specific | `engine/pipkg/pipkg_test.go` (7), `engine/runtime/pi_test.go` (4 adapter), `engine/cmd/pipkg_test.go` (3), `engine/cmd/runtime_test.go` (3 pi), `longterm-mem/internal/register/pi_test.go` (4, golden harness) | `go test` |
| Integration — bash | 7 | `engine/shelltest/overlay_pi_target_test.go` (6), `overlay_pi_package_build_test.go` (1) — source the real `bin/labdrian-overlay` and run a real built engine binary | `go test` + bash |
| Integration — node | 3 | `engine/runtime/pi_test.go` — run the real embedded `labdrian-gate.ts` under node v22 | `go test` + node |
| Integration — CLI | 3 | `longterm-mem/cmd/longterm-mem/main_test.go` pi cases | `go test` |
| E2E (live Pi) | 0 | Tasks 6.1-6.3 — no headless Pi runner exists | Manual |
| **Total pi-specific** | **33** | **8 files** | |

### Changed File Coverage

Coverage analysis skipped — no coverage tool or threshold is configured for this repository, and none was required by `entry.json`.

### Assertion Quality

Audited all 8 pi-related test files. No tautologies, no orphan empty-collection assertions, no
type-only assertions used alone, no ghost loops, no smoke-test-only cases, no mock-heavy files
(the suite uses real filesystem temp trees and real subprocesses instead of mocks). Assertions
compare concrete expected strings, exit codes, file bytes, sha256 digests, and recorded argv.

**Assertion quality**: All assertions verify real behavior.

Two quality notes, not violations:
- `runtime.TestPiAdapter_StatusPartialOnUnprovenEntry` asserts `partial` for exactly one unproven
  entry (settings.json listing). It passes honestly, but its name promises coverage of the
  requirement's full entry set; it is the test that would have caught C-01 had it triangulated
  over all three named entries.
- `pipkg.TestPipkgCheck_DetectsDrift` and `TestPipkgBuild_PreservesRegisteredMcpJSON` both exercise
  `mcp.json`, but neither runs a real `register` call against a built package, which is why the
  `mcp.json.bak` interaction (C-02) escaped both.

### Quality Metrics

**Formatter**: `gofmt -l` clean in both modules.
**Vet**: `go vet ./...` clean in both modules.
**Linter (shell)**: `shellcheck -S warning` reports only the 2 pre-existing SC2064 warnings.
**Race detector**: `go test -race` clean across the engine module.

### Issues Found

**CRITICAL**

- **C-01 — `status --target pi` reports `supported` while longterm-mem MCP visibility is unproven.**
  Spec requirement "Honest Status for Unproven Activation" names three owned entries: package
  presence, extension load, and **longterm-mem MCP visibility**. `PiAdapter.Status`
  (`engine/runtime/pi.go:103`) probes only (a) package built, (b) `pipkg.Check` in-sync, and
  (c) listed in `~/.pi/agent/settings.json`. MCP visibility is never probed — and it cannot be
  probed by the in-sync check, because `pipkg.Check` deliberately deletes `mcp.json` from both
  sides of its diff (`engine/pipkg/pipkg.go:154-163`). Observed: a package built and `pi install`ed
  with `mcp.json` still `{"mcpServers": {}}` returns
  `[pi] status: supported - labdrian-pi package is built, in sync, and listed in ~/.pi/agent/settings.json`.
  That is exactly the overclaim the requirement exists to prevent. Scenario "Unproven entry forces
  partial" is FAILING; scenario "All entries proven report supported" is unsound because `supported`
  does not mean all entries are proven.
  Fix direction: add an `mcp.json` `mcpServers.longterm-mem` presence probe to `Status`'s `problems`
  list, and extend `TestPiAdapter_StatusPartialOnUnprovenEntry` to triangulate over all three entries.

- **C-02 — The documented install flow immediately puts the package into permanent drift.**
  `jsonInstall` writes a `mcp.json.bak` sibling on any content-changing register/unregister.
  `pipkg.Check` excludes `mcp.json` from its content diff but not `mcp.json.bak`, so the `.bak`
  falls through to the "extra" branch. Observed, in the exact order design.md's own Data Flow
  prescribes (`pipkg build` -> `pi install` -> `longterm-mem register --target pi`):
  `bin/labdrian-overlay sync-check --target pi` -> exit 1, `VERDICT:pi:DRIFT`; and
  `engine runtime status --target pi` -> exit 1, `partial - ... mcp.json.bak: extra`.
  The drift persists until the next `pipkg build`. Spec scenario "No drift is reported when
  unchanged" is FAILING, and it makes scenario "All entries proven report supported" unreachable
  on a correctly-registered installation. This is the mirror image of the bug slice 4 explicitly
  set out to avoid ("or every `register --target pi` call would show as permanent drift") — the
  exclusion was written for the file but not for the sibling the same writer creates.
  Fix direction: exclude `mcp.json.bak` in `pipkg.Check` alongside `mcp.json` (and consider having
  `jsonInstall` clean its own `.bak`, or having `Build` preserve it the way it preserves `mcp.json`).

**WARNING**

- **W-01 — `bin/labdrian-overlay status --target pi` never calls `PiAdapter.Status`.**
  `pipkg_status_and_report` (`bin/labdrian-overlay:230-250`) only runs `engine pipkg check` and
  prints a hard-coded
  `"built at <dest>, no drift (partial -- lifecycle proof lands in a later slice)"`.
  Three consequences: the message is stale (the "later slice", pi-lifecycle, has landed); the
  `labdrian-overlay` surface can never report `supported`, so R-006's scenario "All entries proven
  report supported" is unmet there; and it never names package-listing or MCP visibility as
  unproven entries. This is the wider consequence of the slice-4 disclosed "status parity scoped
  down" deviation. Ruling: acceptable as scoping for `cmd_longterm_mem status`, **spec gap** for
  `status --target pi`.
- **W-02 — No `uninstall` command exists in `bin/labdrian-overlay`** for any target, so R-001's
  "MUST accept `pi` as a valid `--target` on ... `uninstall`" cannot be satisfied at that surface.
  The behaviour itself is implemented and proven at `engine runtime uninstall --target pi`.
  Ruling: acceptable deviation in substance (disclosed by slice 5, and adding the command for pi
  alone would be inconsistent with the other three targets), but either the spec sentence should
  be amended to name the engine verb, or a generic `uninstall --target <t>` should be added for
  all four targets as a separate change.
- **W-03 — The `--target all` Pi status exemption outlived its own stated lifetime.**
  `piStatusUnsupportedInAggregate` (`engine/cmd/main.go:421`, comment at 403-408: "removed once
  pi-lifecycle") still masks an unsupported Pi from `status --target all`. `TestRunRuntimeCore_AllTargetsStatusAllowsCodexPartialWithoutFailing`
  exits 0 with Pi unsupported and does not assert any `[pi]` line. Now that `PiAdapter.Status` is
  real, the exemption hides genuine Pi breakage rather than an honest stub, and it leaves the
  runtime-lifecycle scenario "Target all includes Pi support" without a test where Pi is genuinely
  supported or honestly partial.
- **W-04 — Spec text says `extensions/gate.ts`; the package ships `extensions/labdrian-gate.ts`.**
  Disclosed slice-3 deviation. The behavioural requirement (jiti-loaded `.ts`, not `.js`) is met.
  Amend the spec scenario text at archive time so the merged main spec is not wrong.
- **W-05 — Package root directory ships `0700`**, not the `0755` design.md specifies for
  directories (`os.MkdirTemp`'s mode survives the atomic rename). Everything below it is correct.
  Already filed as an issue #312 follow-up; recorded here as a design-coherence deviation.
- **W-06 — `bin/labdrian-overlay apply --target pi` switches the worktree to `main`** (merging
  upstream) before building, then switches back, so on a feature branch it builds the package from
  `main`'s tree. Observed here: the package built by `apply` immediately failed `sync-check` against
  the branch's own registry with dozens of "missing" skills. This is pre-existing script behaviour
  shared with every target, not introduced by this change, but Pi is the first target whose
  `sync-check` can detect and report the mismatch. Recorded so it is not mistaken for a pipkg bug.
- **W-07 — Task 6.4's checkbox is unchecked** although its two broad commands pass (executed in this
  verification). Cosmetic bookkeeping; mark it complete at archive time.

**SUGGESTION**

- S-01 — `package_target_stub_message` (`bin/labdrian-overlay:152`) still says "handled by slice 2
  (pi-package-build)". Slice 2 landed; reword to describe what a package target actually is.
- S-02 — Issue #312's four open follow-ups (mode-blind `Check`, unvalidated registry `path`, root
  0700, `name` != directory) are confirmed as follow-ups, not spec violations. The registry-`path`
  containment one is the only security-relevant member of that set and deserves priority.
- S-03 — Manual tasks 6.1-6.3 remain **deferred to post-chain manual verification**, not failures:
  no headless Pi runner exists, and design.md declared them manual-only from the start. Three
  spec scenarios (package-prefixed server name, pi-engram init non-destruction, live session skill
  and agent discovery) rest on them. Closing C-01 and C-02 first would make that manual pass
  meaningful rather than a debugging session.

### Verdict

**FAIL** — 2 CRITICAL, 7 WARNING, 3 SUGGESTION. Both criticals are in the same seam: the Pi status
and drift surfaces do not yet account for the longterm-mem MCP registration that the change's own
design puts inside the built package. Everything else — package build, contract-gate injection,
target plumbing, MCP registration semantics, and `pi remove` uninstall — is implemented, tested,
and proven at runtime. Both fixes are small and local (`engine/runtime/pi.go` `Status`,
`engine/pipkg/pipkg.go` `Check`), each needs one new triangulated test, and neither disturbs a
landed slice's public shape. Return to `sdd-apply` for a scoped fix, then re-verify; do not archive.
