```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:b3debb83e73d90e34c73be59abb56668507beb1bf9e4143fc939fff51760eca7
verdict: pass_with_warnings
blockers: 0
critical_findings: 0
requirements: 12/12
scenarios: 33/33
test_command: cd engine && gofmt -l . && go vet ./... && go test -count=1 -race ./...
test_exit_code: 0
test_output_hash: sha256:20d401f814ef879b195efd53541e8117004b91f0543d4037e141b70e1c5ce6a4
build_command: cd engine && go build -o /tmp/claude-1000/-home-labdrian-labdrian-sdd-overlay/54b233bf-75b3-4ffe-9efb-d4c8015a06cf/scratchpad/rv5/bin/gentle-ai-overlay ./cmd
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report (FIFTH PASS — closure of the pass-four blocker B-01)

**Change**: pi-runtime-target
**Version**: N/A (delta specs under `openspec/changes/pi-runtime-target/specs/`)
**Mode**: Strict TDD
**Worktree**: `/home/labdrian/labdrian-sdd-overlay-worktrees/pi-5`, branch
`feat/pi-runtime-target-5-lifecycle`, HEAD `3f91b4b`, tree clean before and after verification.

**Prior verdict** (HEAD `50f2266`): FAIL — 0 CRITICAL, 4 WARNING, 2 SUGGESTION, 1 blocker (B-01).
**This verdict**: **PASS WITH WARNINGS — 0 CRITICAL, 2 WARNING, 4 SUGGESTION, 0 blockers.**

B-01 — six scenarios with unobserved THEN/AND clauses — is closed. Five were closed by evidence
landed in `3f91b4b` and re-executed independently here; the sixth was closed by correcting the
scenario to what the platform actually permits. No code changed between `50f2266` and `3f91b4b`
(`git show --stat` lists only three files, all under `openspec/`), so this pass re-ran both suites
from scratch, re-executed the deterministic external probes, and re-read every scenario against
its current text rather than inheriting pass four's matrix.

**Not one of the six closures rests on the prior report's summary.** Each was re-observed here:
the package-prefixed server name and the precedence rule were produced by running
`pi-mcp-adapter`'s own code; the jiti load was produced by handing the shipped `.ts` to Pi's own
jiti; the agent-shipping and non-mutation clauses were read off the installed package and the live
`~/.pi/agent/agents/` directory.

### Rulings on Findings Carried In

| Finding | Claim in `3f91b4b` | Independently verified this pass | Ruling |
|---|---|---|---|
| **B-01 (the blocker)** — 6 scenarios at PARTIAL (27/33) | Five closed by live evidence recorded in `apply-progress` "Remediation 3 and live evidence closure"; one closed by correcting the delta scenario | **Yes, each one separately — see the Evidence Closure table below.** Three of the five were re-produced from scratch here rather than read; the agent-scenario correction was checked against Pi 0.85.1's own `docs/packages.md` and gentle-pi's reader | **RESOLVED — 33/33** |
| **W-07** — the N-01 remediation batch was missing from `apply-progress.md` | Recorded as "Remediation 3 (verify pass three N-01, S-01, S-02) and live evidence closure (2026-09-11)" | Yes — `apply-progress.md:1112-1121` names `d12c97f`, `50f2266`, the inverted shelltest, the native review lineage, and the live evidence for all six scenarios. The ledger no longer stops a batch short of the delivered tree | **RESOLVED** |
| **N-01 / S-01 / S-02** (pass three) | Fixed in `d12c97f`/`50f2266` | Re-confirmed at runtime this pass: a scratch-`HOME` `runtime status --target pi` on an unbuilt package printed the corrected disclosure ending `(short aliases: -ne and -ns)`, with no "slice 2" string | **STILL RESOLVED** |
| **W-05** (package root `0700`), **W-06** (`apply` builds from `main`) | Untouched | Unchanged this pass | **CARRIED — still non-blocking** |
| **S-03** (live pass observed 3 of 12 shipped skills) | Untouched | Package ships 12 skill directories plus `_shared`; the live session listed 3 | **CARRIED as SUGGESTION** |

### Evidence Closure for the Six Former PARTIALs

| # | Scenario clause that was unobserved | Evidence now | Re-executed here? |
|---|---|---|---|
| 1 | "re-running install leaves a single package entry, not a duplicate" | Live: `engine runtime install --target pi` run twice against the real machine (`apply-progress` Remediation 3); the package `mcp.json` registration survived both | Corroborated — `~/.pi/agent/settings.json` `packages` carries exactly one `../../.labdrian-overlay/pi/labdrian-pi` entry among five |
| 2 | "the custom agent files are present under the package's `agents/`" (scenario corrected) | Installed package ships `agents/GADU.md` (7639 bytes) and `package.json` declares `"agents":["./agents"]` | **Yes** — directory listing and manifest read this pass |
| 3 | "it loads `labdrian-gate.ts` via jiti; no `.js` extension file is required" | Pi's own jiti (`@earendil-works/pi-coding-agent/node_modules/jiti`) loaded the shipped `extensions/labdrian-gate.ts` and returned its real exports: `default`, `injectContractLine`, `injectContractsForEvent`, `parseFrontmatter`, `resolveContractPath`. `fd -e js` over the whole installed package returns nothing | **Yes — direct observation, replacing the doc-plus-no-error inference** |
| 4 | "visible under a name prefixed by the sanitized package name" | `pi-mcp-adapter`'s own `loadPackageMcpConfigs(HOME)`, run through jiti against the live settings, returns `mcpServers["labdrian-pi__longterm-mem"]` with our stdio command | **Yes** — probe re-run this pass |
| 5 | "the user/project entry wins, per `pi-mcp-adapter`'s own precedence rule" | **Observed, not inferred.** A scratch cwd with `.pi/settings.json` pointing at the real package and a `.pi/mcp.json` declaring `labdrian-pi__longterm-mem` with command `/user/project/OVERRIDE` was passed to the adapter's exported `loadMcpConfig`: the package-only loader returned our `longterm-mem` command, and the merged effective config returned `/user/project/OVERRIDE`. Corroborated in source (`config.ts:336-341`, package servers are the merge base) and in `README.md:143` | **Yes — this pass built the two-config case the prior passes said only a live session could show** |
| 6 | "the registration remains intact" after `pi-engram init` | Live: `pi-engram init` re-run reported "Kept existing Engram MCP server in mcp.json"; `~/.pi/agent/mcp.json` and the package `mcp.json` byte-identical before and after | Corroborated — both hashes still match the recorded values (`302d5494…`, `25a17cf3…`) at the start and end of this pass |

On (2), the spec correction is the right call and not a weakening dressed as a fix: Pi 0.85.1's
`docs/packages.md` enumerates the package resource kinds as extensions, skills, prompt templates
and themes — agents are not among them — and gentle-pi's reader enumerates
`join(PI_AGENT_DIR, "agents")` only (`startup-banner.ts:494`). A package therefore *cannot* make a
custom agent session-visible, and the only mechanism that could is the one the same requirement
forbids: writing into `~/.pi/agent/agents/`. The scenario as written demanded two contradictory
things. Follow-up #316 tracks the opt-in link action. Both checks were performed against the
installed sources this pass, not taken from the commit message.

### Completeness

| Metric | Value |
|--------|-------|
| Tasks total (1.1–6.4) | **28** |
| Tasks complete | 28 |
| Tasks incomplete | 0 |
| Requirements | 12 (pi-runtime-target 7, runtime-lifecycle 3, longterm-mem-mcp-registration 2) |
| Scenarios | 33 (15 + 9 + 9) |
| Planned slices (`entry.json` `review_slices`) | 5 |
| Realized slices (`apply-progress` slice tracking) | 5 — no drift (P = R = 5) |

Totals recounted independently this pass: `rg -o '^### Requirement:'` → 12,
`rg -o '^#### Scenario:'` → 33 across the three delta files. `rg '^\s*- \[ \]' tasks.md` returns
nothing, so all 28 tasks are `[x]`.

Entry-contract slice comparison performed, not skipped: `entry.json` `review_slices` names
`pi-target-plumbing`, `pi-package-build`, `pi-contract-gate`, `pi-longterm-mem-mcp`,
`pi-lifecycle`; `apply-progress.md` carries a realized batch for each, plus three remediation
batches and a manual-verification block. No planned slice lacks a realized counterpart.

### Build & Tests Execution

**Build**: PASSED

```text
cd engine && go build -o <scratch>/rv5/bin/gentle-ai-overlay ./cmd      -> exit 0, 5052589 bytes
cd longterm-mem && go build -o <scratch>/rv5/bin/longterm-mem ./cmd/... -> exit 0
```

**Tests**: PASSED — both modules, every package, re-run from scratch this pass

```text
cd engine && gofmt -l .                    -> (empty), exit 0
cd engine && go vet ./...                  -> exit 0
cd engine && go test -count=1 -race ./...  -> exit 0; 13 packages ok: assets cmd gadu gate
                                              installer pipkg prespec propagator runtime
                                              settings shelltest skills synctrigger
                                              (sha256 of captured output:
                                              20d401f814ef879b195efd53541e8117004b91f0543d4037e141b70e1c5ce6a4)

cd longterm-mem && gofmt -l .              -> (empty), exit 0
cd longterm-mem && go vet ./...            -> exit 0
cd longterm-mem && go test -count=1 ./...  -> exit 0; 17 packages ok
                                              (sha256 a3f48dc3865250a487e05e10827d7c801a3ad9a87ba8a5fd011bda04f900e0d3)

shellcheck -S warning bin/labdrian-overlay -> exit 1; exactly 2 x SC2064 at lines 1451 and 1612,
                                              pre-existing, known environmental
```

The engine test-output hash differs from pass four's while the package set and result are
identical: `go test` prints per-package timings, which are not reproducible across runs. The hash
pins *this* pass's observed output, which is its purpose.

**Live-machine safety**: the guard held. `/home/labdrian/.labdrian-overlay/pi/labdrian-pi` exists
before and after; `~/.pi/agent/settings.json` (`b601fbaa…`) and `~/.pi/agent/mcp.json`
(`302d5494…`) are byte-identical before and after the full `-race` run, every probe, and the smoke
command. The one smoke command ran with `HOME`, `STATE_DIR` under the session scratchpad and
`LABDRIAN_PI_BIN` pointing at a stub that only logs argv. No `pi` session was launched, and no
uninstall or remove ran against the real `HOME`. All external probes are read-only: they load
`pi-mcp-adapter` and `labdrian-gate.ts` through jiti and call pure loader functions.

**Coverage**: Not collected — no coverage tool or threshold is configured for this repository.

### Runtime Evidence Executed This Pass

```text
A) runtime status --target pi (scratch HOME, package NEVER built)   <-- the old N-01 branch
   -> exit 1
   -> "[pi] status: unsupported — labdrian-pi package is not built at <scratch>/state/pi/labdrian-pi
       (run: labdrian-overlay apply --target pi). 'pi --no-extensions' disables the
       before_agent_start contract-gate extension for that session, and 'pi --no-skills' disables
       skill discovery, for that session only; neither flag's use is detected at runtime
       (short aliases: -ne and -ns)"
   -> contains "(short aliases: -ne and -ns)"  YES     contains "no short alias"  NO

B) pi-mcp-adapter loadPackageMcpConfigs(HOME) via jiti  (cwd = the installed adapter)
   -> {"mcpServers":{"labdrian-pi__longterm-mem":{"type":"stdio",
       "command":"/home/labdrian/.labdrian-overlay/bin/longterm-mem","args":["mcp"]}}}

C) pi-mcp-adapter loadMcpConfig(undefined, <scratch cwd>) with a competing user/project entry
   -> package-declared: command /home/labdrian/.labdrian-overlay/bin/longterm-mem
   -> merged effective: command /user/project/OVERRIDE, args ["from-user-config"]
   -> the user/project entry wins; the package declaration is the merge base

D) Pi's jiti loading the shipped extension
   -> loaded /home/labdrian/.labdrian-overlay/pi/labdrian-pi/extensions/labdrian-gate.ts
   -> exports: default, injectContractLine, injectContractsForEvent, parseFrontmatter,
      resolveContractPath
   -> fd -e js over the package: no matches

E) Installed package shape
   -> package.json: {"name":"labdrian-pi","version":"1.19.1","pi":{"skills":["./skills"],
      "agents":["./agents"],"extensions":["./extensions"],"mcp":"./mcp.json"}}
   -> agents/GADU.md present; skills/ = 12 skill dirs + _shared; mcp.json = one top-level
      mcpServers.longterm-mem entry, no nesting
   -> ~/.pi/agent/agents/ contains only gentle-pi's agents; no GADU.md was written there
```

The stub-driven smoke set of passes three and four (install, register, supported-status,
sync-check, uninstall, `--target all`) was not re-executed: no code changed between `50f2266` and
`3f91b4b`, and each of those behaviours is pinned by a Go test that ran green in this pass's suite.

### Spec Compliance Matrix

Statuses: COMPLIANT (covering evidence passed at runtime), PARTIAL, FAILING.

#### `specs/pi-runtime-target/spec.md` (7 requirements, 15 scenarios)

| Requirement | Scenario | Test / Evidence | Result |
|---|---|---|---|
| Pi Accepted as a Valid CLI Target | Pi target is recognized | `shelltest.TestResolveTargets_Pi`, `TestOverlayStatusAndSyncCheck_PiStubNoEmptyMkdir`, `TestPipkgHelpers_BuildStatusSyncCheck`, `runtime.TestPiAdapter_UninstallUsesRemoveNotUninstall` + runtime §A | COMPLIANT |
| Pi Accepted as a Valid CLI Target | Target all includes Pi without masking other failures | `cmd.TestRunRuntimeCore_AllTargetsIncludesPiWithoutMaskingOtherFailures` | COMPLIANT |
| Package-Delivered Skills and Agents | Local-path install registers the package idempotently | Live Remediation 3: two consecutive `runtime install --target pi` runs left one entry; corroborated here — `settings.json` `packages` carries exactly one `labdrian-pi` entry | COMPLIANT |
| Package-Delivered Skills and Agents | Skills are visible in a Pi session and agents ship as package content | Live 6.1: a real headless session listed packaged skills; runtime §E: `agents/GADU.md` ships and is declared; `~/.pi/agent/agents/` holds only gentle-pi's agents, byte-identical through install and remove (live 6.3) | COMPLIANT |
| Deterministic Contract Gate | sdd-tasks and sdd-apply receive both contract path lines | `runtime.TestLabdrianGateInjectsPathLine_SddTasksSddApply` (node runs the real embedded source against the Go `InjectPrompt` oracle) | COMPLIANT |
| Deterministic Contract Gate | Every other agent is excluded | same test, excluded-agent and unnamed-agent cases | COMPLIANT |
| Deterministic Contract Gate | Composition with gentle-pi's own handler | `runtime.TestLabdrianGateChainsAfterGentlePi` (node present, ran for real) | COMPLIANT |
| Deterministic Contract Gate | Contract paths stay contained; malformed frontmatter yields no injection | `runtime.TestLabdrianGatePathContainment_RejectsTraversal` (4 cases) + malformed-sibling case | COMPLIANT |
| Deterministic Contract Gate | Extension is discovered from the package extensions directory | Runtime §D: Pi's own jiti loaded the shipped `labdrian-gate.ts` and returned its handler exports; no `.js` exists in the package; live 6.2 `pi list` shows the package with its extension | COMPLIANT |
| Honest Status for Unproven Activation | Unproven entry forces partial | `runtime.TestPiAdapter_StatusTriangulatesAllThreeOwnedEntries` + runtime §A (unbuilt branch) | COMPLIANT |
| Honest Status for Unproven Activation | All entries proven report supported | same test's `all_three_entries_proven_reports_supported` + `TestPiAdapter_StatusAcceptsRelativePackageListing` + pass-four smoke §6 | COMPLIANT |
| `--no-extensions` Limitation Disclosure | Disclosure text is present | `runtime.TestPiAdapter_StatusDisclosesNoExtensionsNoSkills`, shelltest assertions on both the built and not-built branches pinning `(short aliases: -ne and -ns)`, + runtime §A | COMPLIANT |
| Pi-Scoped Uninstall | Only the owned package entry is removed | `runtime.TestPiAdapter_UninstallUsesRemoveNotUninstall`, `TestPiAdapter_UninstallNeverTouchesGentlePiFiles` + live 6.3 (byte-identical gentle-pi/pi-engram state on the real machine) | COMPLIANT |
| Pi Drift Detection via Sync-Check | No drift is reported when unchanged | `pipkg.TestPipkgCheck_IgnoresMcpJSONBak`, `TestPipkgBuild_PreservesRegisteredMcpJSONBak` + pass-four smoke §7 | COMPLIANT |
| Pi Drift Detection via Sync-Check | A source edit is detected as drift | `pipkg.TestPipkgCheck_DetectsDrift`, `shelltest.TestPipkgHelpers_BuildStatusSyncCheck` tamper case | COMPLIANT |

#### `specs/runtime-lifecycle/spec.md` (3 requirements, 9 scenarios)

| Requirement | Scenario | Test / Evidence | Result |
|---|---|---|---|
| Target Aggregation | Target all includes Codex support | `cmd.TestRunRuntimeCore_AllTargetsRunsCodexLifecycleTogether` | COMPLIANT |
| Target Aggregation | Target all preserves non-Codex failures | `cmd.TestRunRuntimeCore_AllTargetsStatusFailsWhenClaudeOrOpenCodeFails` | COMPLIANT |
| Target Aggregation | Target all includes Pi support | `cmd.TestRunRuntimeCore_AllTargetsStatusAllowsCodexPartialWithoutFailing`, `TestRunRuntimeCore_AllTargetsStatusFailsWhenPiIsHonestlyUnsupported` + pass-three smoke §11 | COMPLIANT |
| Target Aggregation | Target all preserves non-Pi failures | `cmd.TestRunRuntimeCore_AllTargetsIncludesPiWithoutMaskingOtherFailures` | COMPLIANT |
| Existing Runtime Behavior Non-Regression | Legacy Claude commands still work | `cmd.TestRunRuntimeCore_ClaudeUpdateAndUninstallPreserveLegacyBehavior`; full engine suite green under `-race` | COMPLIANT |
| Existing Runtime Behavior Non-Regression | OpenCode lifecycle remains unchanged | `cmd.TestRunRuntimeCore_OpenCode*` | COMPLIANT |
| Existing Runtime Behavior Non-Regression | Codex lifecycle remains unchanged | `cmd.TestRunRuntimeCore_Codex*`, `shelltest.TestOverlayStatusAndSyncCheckAll_ClaudeOpenCodeCodexOutputUnchanged` | COMPLIANT |
| Pi Target Validation and Dispatch Without a Per-File Copy Path | Pi passes target validation without a copy-path entry | `shelltest.TestIsCopyTarget_ClaudeTrue_PiFalse`, `TestOverlayStatusAndSyncCheck_PiStubNoEmptyMkdir` | COMPLIANT |
| Pi Target Validation and Dispatch Without a Per-File Copy Path | Pi is present in the enum, aggregation, and adapter construction | `runtime.TestExpandTarget_Pi` + `NewFoundationAdapter` adapter-type assertion | COMPLIANT |

#### `specs/longterm-mem-mcp-registration/spec.md` (2 requirements, 9 scenarios)

| Requirement | Scenario | Test / Evidence | Result |
|---|---|---|---|
| MCP Registration — Pi | pi-engram's mcp.json stays untouched | `register.TestRegisterPi_*` + pass-four smoke §5 (pre-seeded `~/.pi/agent/mcp.json` sha256 identical through register) + live 6.3; the real file is still `302d5494…` after this pass | COMPLIANT |
| MCP Registration — Pi | The registration target is a package-relative mcp.json file | Runtime §E: `package.json` `"pi":{"mcp":"./mcp.json"}`; `<pkg>/mcp.json` holds exactly one top-level `mcpServers.longterm-mem` entry, no nesting | COMPLIANT |
| MCP Registration — Pi | The server name is package-prefixed | Runtime §B: the adapter's own loader returns `labdrian-pi__longterm-mem` | COMPLIANT |
| MCP Registration — Pi | User/project config takes precedence | Runtime §C: with a competing `.pi/mcp.json` entry, the adapter's own `loadMcpConfig` resolves to the user/project command, not ours; `config.ts:336-341` makes package servers the merge base; the overlay writes only the package `mcp.json` | COMPLIANT |
| MCP Registration — Pi | pi-engram init does not drop the registration | Live Remediation 3: `pi-engram init` re-run kept the existing server; `~/.pi/agent/mcp.json` and the package `mcp.json` byte-identical (`302d5494…`, `25a17cf3…`), both re-confirmed here | COMPLIANT |
| MCP Registration — Pi | Registration is skipped on expansion when the package is absent | `cmd/longterm-mem.TestCmdRegister_TargetAll_SkipsAbsentPi` | COMPLIANT |
| MCP Registration — Pi | Registration fails when Pi is named explicitly and the package is absent | `cmd/longterm-mem.TestCmdRegister_TargetPiExplicit_FailsWhenPackageAbsent` | COMPLIANT |
| Multi-Target Expansion Treats Pi Like the Other Runtimes | An expansion skips Pi when its package is not installed | `TestCmdRegister_TargetAll_SkipsAbsentPi`, `shelltest.TestLongtermMemUninstall_TargetAllIncludesPi` | COMPLIANT |
| Multi-Target Expansion Treats Pi Like the Other Runtimes | Pi named explicitly still fails without the package installed | `TestCmdRegister_TargetPiExplicit_FailsWhenPackageAbsent`, `shelltest.TestLongtermMemUninstall_TargetPiNoLongerDies` | COMPLIANT |

**Compliance summary**: 33/33 scenarios COMPLIANT, 0 PARTIAL, 0 FAILING. 12/12 requirements
fully compliant.

### Correctness (Static Evidence)

| Requirement | Status | Notes |
|---|---|---|
| R-001 Pi as CLI target | Implemented | `TARGET_KINDS`/`is_copy_target`/`is_valid_target`; the delta names the adapter verb correctly |
| R-002 Package delivery | Implemented | `engine/pipkg.Build`: registry-selected skills, `agents/`, embedded extension, temp-dir + atomic swap, symlink refusal, overlap guard |
| R-003/R-004 Contract gate | Implemented | `engine/pipkg/labdrian-gate.ts`, strict frontmatter parse, bare path line, chained `event.systemPrompt`, root containment; loads under Pi's jiti as shipped |
| R-005 MCP registration | Implemented | `longterm-mem/internal/register/pi.go` reuses unmodified `jsonInstall(containerKey="mcpServers")`; served as `labdrian-pi__longterm-mem`, below user/project precedence |
| R-006 Honest status | Implemented | `PiAdapter.Status` builds `problems` from all three owned entries; the `supported` message names all three |
| R-007 Disclosure | Implemented | Go adapter, README, delta spec and the bash not-built branch carry one identical claim; two shelltest assertions pin the bash copy to the adapter's wording |
| R-008 Non-copy dispatch | Implemented | No `TARGET_PATHS` entry for pi; no empty-path `mkdir` |
| R-009 Uninstall | Implemented | `pi remove <path>`, fixed argv, `exec.LookPath`/`LABDRIAN_PI_BIN`, package dir removed |
| R-010 Drift detection | Implemented | `Check` excludes `mcp.json` and `mcp.json.bak`; `Build` preserves both |

### Coherence (Design)

| Decision (design.md) | Followed? | Notes |
|---|---|---|
| A1 — `pi.mcp` is a package-relative path; unmodified `jsonInstall` | Yes | Verified in the built package and in `register/pi.go` |
| A2 — `pi remove <source>`, not `pi uninstall <name>` | Yes | Stub argv log and live 6.3 both prove the verb |
| A3 — `--no-extensions` / `--no-skills` | Yes, after correction | The design's "no short alias" premise was wrong; Pi 0.85.1 documents `-ne`/`-ns`. `design.md` still records the original premise, correctly, as a historical design record |
| A4 — chained `before_agent_start`, additive `systemPrompt` | Yes | `TestLabdrianGateChainsAfterGentlePi` |
| A5 — type-annotation-free `.ts`, tested via `.mjs` copy under node | Yes | Ships as `.ts`; Pi's jiti loads that exact file (runtime §D) |
| C4/C5 — bare path line, reuse `runtime.InjectPrompt` as oracle | Yes | Byte-identical to the Go mirror |
| `TARGET_KINDS[pi]=package` + 8 call sites | Yes | Plus a disclosed 9th (`cmd_longterm_mem`) fixed in slice 1 |
| Honest status across package, extension and MCP | Yes | All three owned entries probed and named in both messages |
| Agents delivered as package content | Deviation, disclosed and spec-corrected | The design assumed a package could publish agents; Pi 0.85.1 has no agent resource kind. Spec corrected, follow-up #316 filed. See S-05 |
| Security F1–F3 — containment, strict parse, symlink refusal, atomic swap, 0644/0755 | Partial | Package ROOT ships `0700` rather than `0755`. See W-05 |
| Threat matrix: process integration (fixed argv, LookPath) | Yes | `TestPiAdapter_InstallNoShellInjection` |
| Threat matrix: path containment (F1) | Yes | `TestLabdrianGatePathContainment_RejectsTraversal` |
| 5-slice table | Yes, with disclosed deviations | Every slice's file list landed |
| Open question — registry opt-in set for `install.targets: [pi]` | Resolved | `skills validate` exit 0; 12 skills ship in the package |

### Issues Found

**CRITICAL**: None.

**BLOCKER**: None. B-01 is closed at 33/33.

**WARNING** (none blocking)

- **W-05 — Package root ships `0700`, design says `0755` for directories.** Follow-up, already
  filed as GitHub issue #312. Unchanged. No spec scenario names file modes, and `0700` is more
  restrictive than designed, so this is a design-coherence deviation, not a security regression.

- **W-06 — `apply` switches the worktree to `main` before building.** Follow-up, to be filed.
  `bin/labdrian-overlay:1612` runs `git checkout main` under an EXIT trap that restores the
  original branch. Pre-existing behaviour shared by every target; Pi is simply the first target
  whose `sync-check` can detect the resulting build/branch mismatch.

**SUGGESTION**

- **S-03 (carried) — the live session observed 3 of the 12 skills the package ships.** Discovery
  is proven, per-skill coverage is not. A follow-up session that lists the full skill set would
  close the gap. Not a spec clause: the scenario asks only that overlay skills are discoverable.

- **S-04 (carried) — `tasks.md` 3.6 still describes the wrong premise** ("no `-ns` alias").
  Deliberately left alone: `tasks.md` is an audit artifact, not a merged source of truth. Flagged
  so nobody later mistakes it for a live requirement.

- **S-05 (new) — `package.json` declares `"agents":["./agents"]`, which Pi 0.85.1 does not read.**
  The key is inert: Pi's package resource kinds are extensions, skills, prompt templates and
  themes, and the installed package loads without complaint. It is harmless and arguably useful as
  intent, but it reads like a working declaration to anyone who has not checked the platform docs.
  Worth a comment in the build, or a note on the requirement, alongside #316.

- **S-06 (new) — Remediation 3 is narrative where earlier batches are tabular.** It records the
  commits, the review lineage and the live evidence, which is what W-07 asked for, but it carries
  no TDD Cycle Evidence table like Remediations 1 and 2. Since the batch's RED shape (an inverted
  assertion) is described in prose and independently re-verified across passes four and five, this
  is a formatting inconsistency in the ledger, not a missing-evidence finding.

### TDD Compliance

| Check | Result | Details |
|---|---|---|
| TDD Evidence reported | Yes | 5 slice tables, 3 remediation batches, and a dated manual-verification block in `apply-progress`; the ledger now reaches the delivered tree (W-07 resolved) |
| All tasks have tests | Yes | 28/28 tasks complete; every code task maps to a named test file; 5.4 and 6.1–6.3 are docs/spec/manual by design |
| RED confirmed (test files exist) | Yes | Every named file located and executed this pass |
| RED evidence is real, not asserted | Yes | Remediations record falsifiable pre-fix strings (`got: supported, want partial`; `mcp.json.bak: extra`; `got 0, want 1`); for N-01 the pre-fix string was the shipped sentence itself, quoted in pass three and absent from the tree now |
| GREEN confirmed (tests pass now) | Yes | Both full suites re-run from scratch this pass, engine under `-race`; 13 + 17 packages ok |
| Triangulation adequate | Yes | The disclosure claim is pinned positively and negatively across two shelltest files plus the Go constant both must match |
| Safety net for modified files | Yes | Each batch records the relevant suite green before edit |
| Live-machine isolation | Yes | `TestMain` guards in `engine/runtime/live_guard_test.go` and `engine/cmd/live_guard_test.go`; the real package, settings and mcp.json survived this pass byte-identical |

**TDD Compliance**: 8/8 checks passed.

### Test Layer Distribution

| Layer | Tests | Files | Tools |
|---|---|---|---|
| Unit (Go) | 28 pi-specific | `engine/pipkg/pipkg_test.go` (9), `engine/runtime/pi_test.go` (6 adapter), `engine/runtime/live_guard_test.go` (1), `engine/cmd/pipkg_test.go` (3), `engine/cmd/runtime_test.go` (4 pi), `longterm-mem/internal/register/pi_test.go` (4), `engine/runtime/runtime_test.go` (1) | `go test` |
| Integration — bash | 8 | `engine/shelltest/overlay_pi_target_test.go` (6), `overlay_pi_package_build_test.go` (2) | `go test` + bash |
| Integration — node | 3 | `engine/runtime/pi_test.go` — run the real embedded `labdrian-gate.ts` under node | `go test` + node |
| Integration — CLI | 4 | `longterm-mem/cmd/longterm-mem/main_test.go` pi cases | `go test` |
| E2E (live Pi) | 3 (manual) + 6 evidence closures | Tasks 6.1–6.3 and Remediation 3, executed 2026-09-11 against Pi 0.85.1; no headless runner exists | Manual + jiti probes |
| **Total pi-specific** | **46 automated** | **9 files + 1 manual pass + 3 external probes** | |

### Changed File Coverage

Coverage analysis skipped — no coverage tool or threshold is configured for this repository, and
none was required by `entry.json`. Not a failure.

### Assertion Quality

No test file changed since pass four (`3f91b4b` touches only `openspec/`), so the assertion audit
of passes three and four stands unaltered. The three assertions rewritten in `d12c97f`/`50f2266`
were audited in pass four and re-executed green here:
`overlay_pi_target_test.go:133` (positive-and-negative conjunction on the disclosure string),
`overlay_pi_package_build_test.go:66` (inverted from a false-premise negative to a positive), and
`pi_test.go:506` (`wantContains` narrowed to the clause actually at risk).

**Assertion quality**: All assertions verify real behavior.

### Quality Metrics

**Formatter**: `gofmt -l` clean in both modules.
**Vet**: `go vet ./...` clean in both modules.
**Linter (shell)**: `shellcheck -S warning` reports only the 2 pre-existing SC2064 warnings.
**Race detector**: `go test -race` clean across all 13 engine packages.

### Verdict

**PASS WITH WARNINGS — archive-ready.** 0 CRITICAL, 0 blockers, 2 WARNING, 4 SUGGESTION.
33/33 scenarios and 12/12 requirements compliant.

B-01, the sole blocker of pass four, is closed — and closed the way the previous report demanded,
by producing the missing observations rather than by relaxing the count. Five of the six scenarios
now have runtime evidence: two from live Pi runs recorded in `apply-progress` and corroborated
here against current on-disk state, and three produced from scratch in this pass by running
`pi-mcp-adapter`'s and Pi's own code — the package-prefixed name, the precedence rule, and the
jiti load of the shipped `.ts`. The precedence scenario deserves particular note: passes three and
four both recorded it as external and unobservable, yet the adapter exports `loadMcpConfig`, and
handing it a scratch config directory with a competing entry settles the question in one command.
"Owned by another component" is a statement about who must fix a bug, not about whether a claim
can be checked.

The sixth scenario was corrected rather than satisfied, and that is the right outcome, not a
convenient one. It asked for custom agents to be session-visible *and* for nothing to be written
into `~/.pi/agent/agents/` — and on Pi 0.85.1 those two clauses cannot both hold, because packages
declare no agent resource and gentle-pi reads agents only from that directory. Both halves of that
were re-verified here against the installed `docs/packages.md` and `startup-banner.ts`, not taken
from the commit message. A spec that demands an impossibility is a spec defect; the delta now
claims what the overlay actually delivers, and #316 tracks the opt-in link action that would make
the original intent reachable.

W-07 is resolved: `apply-progress.md` now carries Remediation 3 with commits, native review
lineage, and the live evidence, so the archived ledger reaches the delivered tree.

Everything else held. Both suites re-ran from scratch and are green — engine under `-race` across
13 packages, longterm-mem across 17 — with `gofmt`, `go vet` and `shellcheck` clean apart from the
two known pre-existing SC2064 warnings. All 28 tasks are complete. Planned and realized slice
counts match at 5. The live machine was untouched: the real package, `settings.json` and
`mcp.json` are byte-identical before and after every command this phase ran.

Two WARNINGs remain and neither blocks: W-05 (package root `0700` vs the design's `0755`, issue
#312) and W-06 (`apply` builds from `main`, pre-existing and shared by every target). Four
SUGGESTIONs are recorded for follow-up, including the inert `pi.agents` manifest key (S-05).

**Recommended next step**: `sdd-archive`. No implementation work remains.
