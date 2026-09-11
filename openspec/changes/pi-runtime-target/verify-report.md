```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:d753678c65d7209629e358c1a082456f3ca57efc442a39e1b8e4447050a64dc8
verdict: fail
blockers: 1
critical_findings: 0
requirements: 9/12
scenarios: 27/33
test_command: cd engine && gofmt -l . && go vet ./... && go test -count=1 -race ./...
test_exit_code: 0
test_output_hash: sha256:ce1019a4b06f228e15cba4f0d8c172bd19d991de5d7b3e8c8f452cbc997f914f
build_command: cd engine && go build -o /tmp/claude-1000/-home-labdrian-labdrian-sdd-overlay/54b233bf-75b3-4ffe-9efb-d4c8015a06cf/scratchpad/rv4/home/.claude/bin/gentle-ai-overlay ./cmd
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report (FOURTH PASS — scoped re-check of the single pass-three blocker)

**Change**: pi-runtime-target
**Version**: N/A (delta specs under `openspec/changes/pi-runtime-target/specs/`)
**Mode**: Strict TDD
**Worktree**: `/home/labdrian/labdrian-sdd-overlay-worktrees/pi-5`, branch
`feat/pi-runtime-target-5-lifecycle`, HEAD `50f2266`, tree clean before and after verification.

**Prior verdict** (HEAD `b2dce9b`): FAIL — 0 CRITICAL, 3 WARNING, 3 SUGGESTION, 1 blocker (N-01).
**This verdict**: FAIL (not archive-ready) — **0 CRITICAL, 4 WARNING, 2 SUGGESTION, 1 blocker.**

N-01, the blocker of pass three, is independently confirmed fixed in source and at runtime, and
both SUGGESTIONs it travelled with (S-01, S-02) were fixed in the same batch. Every suite was
re-run from scratch and every relevant smoke command was re-executed against a stub `pi`. **No new
CRITICAL and no new code defect was found; the implementation is sound.**

The verdict is nevertheless `fail`, and this pass must be explicit about why, because it is a
correction to pass three rather than a new discovery. The remaining blocker is **B-01: six spec
scenarios still lack passing covering evidence (27/33)**. Pass three named these same six PARTIALs
and ruled them "not a blocker". That ruling was never tested, because N-01 was independently
forcing `fail` at the time, so the report was admitted as a `fail` on other grounds. With N-01
gone, the ruling became load-bearing for the first time this pass — and it does not hold.
`gentle-ai sdd-verify-validate` refuses any passing verdict whose completed counts fall short of
its totals: submitting this report at `pass_with_warnings` with `scenarios: 27/33` was denied with
*"passing verdict contradicts failing or incomplete evidence"*. The same report at `33/33` is
admitted, which is precisely the falsification that matters — the only thing standing between this
change and archive is six scenarios' worth of unobserved evidence, and the only way to "pass" today
would be to overstate the count. That is not available. See B-01 for what closes it.

One new non-blocking WARNING (W-07) was also found: the remediation batch that fixed N-01 was never
recorded in `apply-progress.md`.

### Rulings on Findings Carried In

| Finding | Claim | Independently verified | Ruling |
|---|---|---|---|
| **N-01 (the blocker)** — `bin/labdrian-overlay` not-built disclosure said "there is no short alias for either flag" | Fixed (`d12c97f`) | **Yes, three ways.** (a) Diff: `bin/labdrian-overlay:242` now ends `neither flag's use is detected at runtime (short aliases: -ne and -ns)`, byte-for-byte the tail of `piNoDiscoveryFlagsDisclosure` in `engine/runtime/pi.go:59`. (b) Source sweep: `rg "no short alias"` over the whole repo excluding `openspec/` returns exactly one hit, and it is the *negative assertion* inside the new shelltest guard — no live string anywhere claims the aliases do not exist. (c) Runtime: scratch `status --target pi` on an unbuilt package printed the corrected sentence (smoke §1). | **RESOLVED** |
| **S-01** — two stale `"slice 2"` strings in `bin/labdrian-overlay` | Fixed (`d12c97f`) | Yes — line 153 `package_target_stub_message` now prints `"package target without a lifecycle adapter (not implemented)"`, and the sync-check branch (was line 2525) prints `"unsupported -- package target without a lifecycle adapter"`. `rg "slice 2" bin/ engine/ README.md` returns one hit and it is a test-file *comment* naming the historical slice id, not shipped output. The comment above the helper was rewritten to describe the fallback instead of deferring to a landed slice. | **RESOLVED** |
| **S-02** — `supported` status under-reported the third proven entry | Fixed (`d12c97f`) | Yes — `engine/runtime/pi.go:125` now reads `"built, in sync, listed in ~/.pi/agent/settings.json, and longterm-mem is registered in its mcp.json"`, and smoke §6 printed exactly that on a fully-registered scratch package. The change is pinned by `TestPiAdapter_StatusTriangulatesAllThreeOwnedEntries`, whose `wantContains` was tightened from the old prefix to `"longterm-mem is registered in its mcp.json"` — the assertion now fails if the third entry is dropped from the message again. | **RESOLVED** |
| Test coverage for the drift that produced N-01 | Added (`d12c97f`, `50f2266`) | Yes — two independent assertions now pin the bash copy to the adapter's wording. `shelltest/overlay_pi_target_test.go:133` requires `(short aliases: -ne and -ns)` **and** forbids `no short alias` on the not-built branch; `overlay_pi_package_build_test.go:66` was inverted from "must not claim a `-ns` alias exists" to "must name the `-ns`/`-ne` aliases". Both ran green here. | **RESOLVED** — the two copies can no longer drift silently |
| 6 PARTIAL scenarios | Ruled non-blocking in pass three | Nothing in `d12c97f` or `50f2266` touches any of them; no evidence changed on either side. Each was re-read against its spec text this pass and each genuinely has an unobserved THEN/AND clause | **CARRIED, BUT RE-RULED — now the sole blocker (B-01)**; the prior "non-blocking" ruling was never exercised against the validator |
| W-05 (package root `0700`), W-06 (`apply` builds from `main`) | Non-blocking follow-ups | Unchanged this pass | **CARRIED — still non-blocking** |

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

**Correction to the prior three passes**: the task total is **28**, not 26. Enumerated directly
from `tasks.md` this pass: 1.1–1.4 (4), 2.1–2.5 (5), 3.1–3.6 (6), 4.1–4.5 (5), 5.1–5.4 (4),
6.1–6.4 (4) = 28. `rg '^\s*- \[ \]'` returns nothing, so all 28 are `[x]`. The earlier "26" was an
undercount; it never changed the verdict (complete is complete either way), but it is corrected
here so archive inherits the real number.

Requirement and scenario totals were recounted independently this pass with
`rg -c '^### (Requirement|REQ-)'` and `rg -c '^#### Scenario:'` against the three delta files:
`pi-runtime-target` 7/15, `runtime-lifecycle` 3/9, `longterm-mem-mcp-registration` 2/9 →
**12 requirements, 33 scenarios**, unchanged.

Entry-contract slice comparison performed, not skipped: `entry.json` `review_slices` names
`pi-target-plumbing`, `pi-package-build`, `pi-contract-gate`, `pi-longterm-mem-mcp`,
`pi-lifecycle`; `apply-progress.md` carries a realized batch for each. No planned slice lacks a
realized counterpart.

### Build & Tests Execution

**Build**: PASSED

```text
cd engine && go build -o <scratch>/home/.claude/bin/gentle-ai-overlay ./cmd  -> exit 0, 5052589 bytes
cd longterm-mem && go build -o <scratch>/state/bin/longterm-mem ./cmd/longterm-mem -> exit 0
```

**Tests**: PASSED — both modules, every package, re-run from scratch this pass

```text
cd engine && gofmt -l .                    -> (empty), exit 0
cd engine && go vet ./...                  -> exit 0
cd engine && go test -count=1 -race ./...  -> exit 0; 13 packages ok: assets cmd gadu gate
                                              installer pipkg prespec propagator runtime
                                              settings shelltest skills synctrigger
                                              (sha256 of captured output:
                                              ce1019a4b06f228e15cba4f0d8c172bd19d991de5d7b3e8c8f452cbc997f914f)

cd longterm-mem && gofmt -l .              -> (empty), exit 0
cd longterm-mem && go vet ./...            -> exit 0
cd longterm-mem && go test -count=1 ./...  -> exit 0; 17 packages ok

shellcheck -S warning bin/labdrian-overlay -> exit 1; exactly 2 x SC2064, now at lines 1451 and
                                              1612 (were 1450/1611; shifted +1 by the one-line
                                              comment d12c97f added above package_target_stub_message)
                                              pre-existing, known environmental
```

The shellcheck line shift is itself corroboration that the only bash change in this batch is the
one N-01 asked for: two message strings and a comment, nothing structural.

**Live-machine safety**: the guard held again. Before and after the full `-race ./...` run and
after every smoke command, `/home/labdrian/.labdrian-overlay/pi/labdrian-pi` still exists and the
real `~/.pi/agent/settings.json` still lists `../../.labdrian-overlay/pi/labdrian-pi`. Every smoke
command ran with `HOME` and `STATE_DIR` under the session scratchpad and `LABDRIAN_PI_BIN` pointing
at a stub script that logged argv and edited only the scratch `settings.json`; the real `pi` binary
was never invoked by this phase, and no uninstall/remove ran against the real `HOME`.

**Coverage**: Not collected — no coverage tool or threshold is configured for this repository.

### Runtime Smoke Evidence (scratch `HOME`, scratch `STATE_DIR`, stub `pi` via `LABDRIAN_PI_BIN`)

```text
1) bin/labdrian-overlay status --target pi   (package NEVER built)              <-- the N-01 branch
   -> exit 1
   -> "pi: note -- 'pi --no-extensions' disables the before_agent_start contract-gate extension
       for that session, and 'pi --no-skills' disables skill discovery, for that session only;
       neither flag's use is detected at runtime (short aliases: -ne and -ns)"
   -> "pi: not built (run: labdrian-overlay apply --target pi)"
   -> contains "(short aliases: -ne and -ns)"  YES
   -> contains "no short alias"                NO
   -> contains "slice 2"                       NO

2) engine pipkg build --dest-dir <pkg>       -> exit 0; package written

3) engine runtime install --target pi        -> exit 0
   -> "[pi] install: restart_required - ran `pi install <pkg>`; start a new Pi session to load it"
   -> stub argv log: "install <pkg>"; settings.json packages = ["npm:gentle-pi", "<pkg>"]

4) engine runtime status --target pi         (built + listed, MCP NOT registered)
   -> exit 1; "[pi] status: partial - labdrian-pi status is unproven: longterm-mem registered in
      mcp.json (not registered; run: longterm-mem register --target pi)" + the corrected disclosure

5) longterm-mem register --target pi --config-root <pkg>   -> exit 0; "register: pi: ok"
   -> <pkg>/mcp.json: mcpServers.longterm-mem = {"type":"stdio","command":"<ltm>","args":["mcp"]}
   -> pre-seeded pi-engram-owned ~/.pi/agent/mcp.json sha256 BYTE-IDENTICAL before == after
      (3c239f998dd1368e164e8fc73a830ab2aa026dc9829ceb9c2533ed29ee1f0653)
   -> ~/.pi/agent/agents/ never created

6) engine runtime status --target pi         (built + listed + registered)      <-- the S-02 branch
   -> exit 0; "[pi] status: supported - labdrian-pi package is built, in sync, listed in
      ~/.pi/agent/settings.json, and longterm-mem is registered in its mcp.json."
   -> the success message now names all three proven entries, matching the partial branch

7) bin/labdrian-overlay sync-check --target pi
   -> exit 0; "SYNC_CHECK:pi: no drift"; "VERDICT:pi:IN_SYNC"; no "slice 2" in the output
```

The pass-three smoke set (uninstall via `pi remove`, the `.bak` preservation probe, the relative
`packages` entry probe, `--target all` aggregation) was not re-executed: none of its inputs changed
between `b2dce9b` and `50f2266`, the two commits touch only three message strings and three test
assertions, and each of those behaviours remains pinned by a Go test that ran green in this pass's
suite (`TestPiAdapter_UninstallUsesRemoveNotUninstall`, `TestPipkgCheck_IgnoresMcpJSONBak`,
`TestPiAdapter_StatusAcceptsRelativePackageListing`,
`TestRunRuntimeCore_AllTargetsIncludesPiWithoutMaskingOtherFailures`).

### Spec Compliance Matrix

Statuses: COMPLIANT (covering evidence passed at runtime), PARTIAL (part of the scenario proven,
part unobserved), FAILING (evidence contradicts the scenario).

#### `specs/pi-runtime-target/spec.md` (7 requirements, 15 scenarios)

| Requirement | Scenario | Test / Evidence | Result |
|---|---|---|---|
| Pi Accepted as a Valid CLI Target | Pi target is recognized | `shelltest.TestResolveTargets_Pi`, `TestOverlayStatusAndSyncCheck_PiStubNoEmptyMkdir`, `TestPipkgHelpers_BuildStatusSyncCheck`, `runtime.TestPiAdapter_UninstallUsesRemoveNotUninstall` + smoke §1,§7 | COMPLIANT |
| Pi Accepted as a Valid CLI Target | Target all includes Pi without masking other failures | `cmd.TestRunRuntimeCore_AllTargetsIncludesPiWithoutMaskingOtherFailures` | COMPLIANT |
| Package-Delivered Skills and Agents | Local-path install registers the package idempotently | Live 6.1 + smoke §3 prove the package is listed in `~/.pi/agent/settings.json` packages. The second THEN clause — re-running install leaves a single entry — has no evidence on either side: the live pass installed once, and the smoke stub's dedup is our own code, not Pi's | PARTIAL — listing proven, idempotency unobserved |
| Package-Delivered Skills and Agents | Skills and agents are visible in a Pi session | Live 6.1 proves skill discovery (3 packaged skills listed by a real session) and proves the AND clause (`~/.pi/agent/agents/` never written, re-confirmed in smoke §5). "Custom agents are present" is evidenced only structurally: the package ships `agents/GADU.md` and declares `"agents":["./agents"]` | PARTIAL — skills and the non-mutation clause proven live; agent presence in a live session not observed |
| Deterministic Contract Gate | sdd-tasks and sdd-apply receive both contract path lines | `runtime.TestLabdrianGateInjectsPathLine_SddTasksSddApply` (node runs the real embedded source against the Go `InjectPrompt` oracle) | COMPLIANT |
| Deterministic Contract Gate | Every other agent is excluded | same test, excluded-agent and unnamed-agent cases | COMPLIANT |
| Deterministic Contract Gate | Composition with gentle-pi's own handler | `runtime.TestLabdrianGateChainsAfterGentlePi` (node present, ran for real) | COMPLIANT |
| Deterministic Contract Gate | Contract paths stay contained; malformed frontmatter yields no injection | `runtime.TestLabdrianGatePathContainment_RejectsTraversal` (4 cases) + malformed-sibling case | COMPLIANT |
| Deterministic Contract Gate | Extension is discovered from the package extensions directory | `pipkg` build tests + smoke §2 prove `extensions/labdrian-gate.ts` ships and `package.json` declares `"extensions":["./extensions"]`; live 6.2 proves `pi list` reports it and that a session with extensions starts normally. The THEN clause "it loads `labdrian-gate.ts` via jiti" was not directly observed — 6.2 records that the `--no-extensions` comparison was impossible on this machine | PARTIAL — shipped, declared and listed; jiti load not directly observed |
| Honest Status for Unproven Activation | Unproven entry forces partial | `runtime.TestPiAdapter_StatusTriangulatesAllThreeOwnedEntries` (all three owned entries) + smoke §4 | COMPLIANT |
| Honest Status for Unproven Activation | All entries proven report supported | same test's `all_three_entries_proven_reports_supported` (assertion tightened in `d12c97f` to require the mcp.json clause) + `TestPiAdapter_StatusAcceptsRelativePackageListing` + smoke §6 | COMPLIANT |
| `--no-extensions` Limitation Disclosure | Disclosure text is present | `runtime.TestPiAdapter_StatusDisclosesNoExtensionsNoSkills`, `shelltest` assertions on **both** the built and not-built branches — both now pin the exact `(short aliases: -ne and -ns)` wording — plus smoke §1/§4/§6 | COMPLIANT — **and the requirement-text contradiction that made this row's footnote necessary in pass three is gone** |
| Pi-Scoped Uninstall | Only the owned package entry is removed | `runtime.TestPiAdapter_UninstallUsesRemoveNotUninstall`, `TestPiAdapter_UninstallNeverTouchesGentlePiFiles` (both green this pass) + pass-three smoke §10 + live 6.3 (byte-identical gentle-pi/pi-engram state on a real machine) | COMPLIANT |
| Pi Drift Detection via Sync-Check | No drift is reported when unchanged | `pipkg.TestPipkgCheck_IgnoresMcpJSONBak`, `TestPipkgBuild_PreservesRegisteredMcpJSONBak` + smoke §7 | COMPLIANT |
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
| MCP Registration — Pi | pi-engram's mcp.json stays untouched | `register.TestRegisterPi_*` + smoke §5 (pre-seeded `~/.pi/agent/mcp.json` sha256 identical through register) + live 6.3 on the real machine | COMPLIANT |
| MCP Registration — Pi | The registration target is a package-relative mcp.json file | Smoke §2/§5: `package.json` `"pi":{"mcp":"./mcp.json"}`; `<pkg>/mcp.json` top-level `mcpServers` with exactly one `longterm-mem` entry, no nesting | COMPLIANT |
| MCP Registration — Pi | The server name is package-prefixed | pi-mcp-adapter's own load-time prefixing; nothing in this change controls it, and the live pass did not inspect the served name | PARTIAL — external, unobserved |
| MCP Registration — Pi | User/project config takes precedence | pi-mcp-adapter's own precedence rule; the overlay correctly implements nothing here, and precedence was not exercised live | PARTIAL — external, satisfied by non-action |
| MCP Registration — Pi | pi-engram init does not drop the registration | The registration lives inside the package, not `~/.pi/agent/mcp.json`, so `pi-engram init` structurally cannot reach it. No automated or live proof | PARTIAL — external, unobserved |
| MCP Registration — Pi | Registration is skipped on expansion when the package is absent | `cmd/longterm-mem.TestCmdRegister_TargetAll_SkipsAbsentPi` | COMPLIANT |
| MCP Registration — Pi | Registration fails when Pi is named explicitly and the package is absent | `cmd/longterm-mem.TestCmdRegister_TargetPiExplicit_FailsWhenPackageAbsent` | COMPLIANT |
| Multi-Target Expansion Treats Pi Like the Other Runtimes | An expansion skips Pi when its package is not installed | `TestCmdRegister_TargetAll_SkipsAbsentPi`, `shelltest.TestLongtermMemUninstall_TargetAllIncludesPi` | COMPLIANT |
| Multi-Target Expansion Treats Pi Like the Other Runtimes | Pi named explicitly still fails without the package installed | `TestCmdRegister_TargetPiExplicit_FailsWhenPackageAbsent`, `shelltest.TestLongtermMemUninstall_TargetPiNoLongerDies` | COMPLIANT |

**Compliance summary**: 27/33 scenarios COMPLIANT, 6 PARTIAL, **0 FAILING**.
9/12 requirements fully compliant. The counts are unchanged from pass three by design: N-01 was a
contradiction between shipped text and *requirement* text, not a failure of any scenario's THEN
clauses, so fixing it removes the blocker without moving a scenario. What it does move is the
correctness table (R-007) and the `--no-extensions` row's footnote.

The 6 PARTIALs, named exactly, all carried unchanged:

1. *Local-path install registers the package idempotently* — the re-install-leaves-one-entry clause.
2. *Skills and agents are visible in a Pi session* — the custom-agent-presence clause.
3. *Extension is discovered from the package extensions directory* — the jiti-load clause.
4. *The server name is package-prefixed* — pi-mcp-adapter behaviour, unobserved.
5. *User/project config takes precedence* — pi-mcp-adapter behaviour, satisfied by non-action.
6. *pi-engram init does not drop the registration* — structurally unreachable, unobserved.

None is a code defect. (1)–(3) are narrow observational gaps in an otherwise-completed live pass;
(4)–(6) are owned by pi-mcp-adapter and pi-engram, not by this change. Each of the six was re-read
against its spec text this pass rather than inherited from the prior report, and each really does
carry an unobserved clause: "re-running install leaves a single package entry"; "custom agents are
present"; "it loads `labdrian-gate.ts` via jiti"; "visible under a name prefixed by the sanitized
package name"; "the user/project entry wins"; "the registration remains intact" after
`pi-engram init`. Not one of them can be closed by a Go test, because each asserts what a real Pi
session does. They are the blocker B-01.

### Correctness (Static Evidence)

| Requirement | Status | Notes |
|---|---|---|
| R-001 Pi as CLI target | Implemented | `TARGET_KINDS`/`is_copy_target`/`is_valid_target`; the delta names the adapter verb correctly |
| R-002 Package delivery | Implemented | `engine/pipkg.Build`: registry-selected skills, `agents/`, embedded extension, temp-dir + atomic swap, symlink refusal, overlap guard |
| R-003/R-004 Contract gate | Implemented | `engine/pipkg/labdrian-gate.ts`, strict frontmatter parse, bare path line, chained `event.systemPrompt`, root containment |
| R-005 MCP registration | Implemented | `longterm-mem/internal/register/pi.go` reuses unmodified `jsonInstall(containerKey="mcpServers")`; `register_paths.go` resolves relative settings entries |
| R-006 Honest status | Implemented | `PiAdapter.Status` builds `problems` from all three owned entries; the `supported` message now names all three too (S-02) |
| R-007 Disclosure | **Implemented** — was "Implemented with a defect" | Go adapter, README, delta spec and the bash not-built branch now carry one identical claim: the aliases are `-ne` and `-ns`. Two test assertions pin the bash copy to the adapter's wording |
| R-008 Non-copy dispatch | Implemented | No `TARGET_PATHS` entry for pi; no empty-path `mkdir` |
| R-009 Uninstall | Implemented | `pi remove <path>`, fixed argv, `exec.LookPath`/`LABDRIAN_PI_BIN`, package dir removed |
| R-010 Drift detection | Implemented | `Check` excludes `mcp.json` and `mcp.json.bak`; `Build` preserves both |

### Coherence (Design)

| Decision (design.md) | Followed? | Notes |
|---|---|---|
| A1 — `pi.mcp` is a package-relative path; unmodified `jsonInstall` | Yes | Verified in the built package and in `register/pi.go` |
| A2 — `pi remove <source>`, not `pi uninstall <name>` | Yes | Stub argv log and live 6.3 both prove the verb |
| A3 — `--no-extensions` / `--no-skills` | **Yes, after correction** | The design's "no short alias" premise was wrong; Pi 0.85.1 documents `-ne`/`-ns`. Research, spec, README, the Go message (`4d3ba7e`) and finally the bash branch (`d12c97f`) all now agree. `design.md` itself still records the original premise — that is correct as a historical design record; the spec, not the design, is what archive merges |
| A4 — chained `before_agent_start`, additive `systemPrompt` | Yes | `TestLabdrianGateChainsAfterGentlePi` |
| A5 — type-annotation-free `.ts`, tested via `.mjs` copy under node | Yes | Ships as `.ts`; node tests ran for real |
| C4/C5 — bare path line, reuse `runtime.InjectPrompt` as oracle | Yes | Byte-identical to the Go mirror |
| `TARGET_KINDS[pi]=package` + 8 call sites | Yes | Plus a disclosed 9th (`cmd_longterm_mem`) fixed in slice 1 |
| Honest status across package, extension and MCP | Yes | All three owned entries probed, and now all three are named in both the partial and the supported message |
| Security F1–F3 — containment, strict parse, symlink refusal, atomic swap, 0644/0755 | Partial | Package ROOT ships `0700` rather than `0755`. See W-05 |
| Threat matrix: process integration (fixed argv, LookPath) | Yes | `TestPiAdapter_InstallNoShellInjection` |
| Threat matrix: path containment (F1) | Yes | `TestLabdrianGatePathContainment_RejectsTraversal` |
| 5-slice table | Yes, with disclosed deviations | Every slice's file list landed |
| Open question — registry opt-in set for `install.targets: [pi]` | Resolved | `skills validate` exit 0, 36 skills aligned; 12 skills ship in the package |

### Issues Found

**CRITICAL**: None. No code defect was found in this pass.

**BLOCKER**

- **B-01 — six spec scenarios lack passing covering evidence (scenarios 27/33, requirements 9/12).**
  The six are enumerated above. Three (`install idempotency`, `custom agents present`,
  `jiti load`) need one more live Pi session; three (`package-prefixed server name`,
  `user/project precedence`, `pi-engram init does not drop the registration`) assert behaviour owned
  by `pi-mcp-adapter` and `pi-engram` and need either a live observation or an explicit spec
  decision that they are out of this change's scope.

  Why this is a blocker now and was not called one in pass three: the skill's own gate is that *a
  spec scenario is compliant only when a covering test passed at runtime*, and none of these six
  has one. Pass three recorded that accurately but ruled it non-blocking — a ruling that cost
  nothing at the time, because N-01 was already forcing `fail`. This pass removed N-01 and the
  ruling became the deciding one, at which point it failed: `gentle-ai sdd-verify-validate` denied
  the `pass_with_warnings` candidate with *"passing verdict contradicts failing or incomplete
  evidence"*, and admitted the byte-identical report only when the counts were raised to `12/12`
  and `33/33`. The counts are not raisable honestly, so the verdict is `fail`.

  Two exits, both legitimate, and the choice is the user's rather than this phase's:
  1. **Close the evidence.** One live Pi session closes (1)–(3): re-run `pi install` on an
     already-installed package and show one entry; list a package agent in-session; observe the
     gate firing with extensions on and absent with `-ne` on a machine where the provider survives
     that flag. For (4)–(6), inspect the served MCP name in a live session, shadow it from a
     user-level config, and re-run `pi-engram init`. This is S-03's suggestion promoted to the
     critical path.
  2. **Narrow the spec.** If (4)–(6) are genuinely pi-mcp-adapter's contract and not this change's,
     the delta should say so — as a note on the requirement, or by removing scenarios this overlay
     cannot and should not prove — so the merged source of truth claims only what it owns. That is
     an `sdd-spec` decision, not something verification may assume.

**WARNING** (none blocking)

- **W-05 — Package root ships `0700`, design says `0755` for directories.** Follow-up, already
  filed as GitHub issue #312. Unchanged this pass. No spec scenario names file modes, and `0700` is
  more restrictive than designed, so this is a design-coherence deviation, not a security
  regression. Not blocking.

- **W-06 — `apply` switches the worktree to `main` before building.** Follow-up, to be filed.
  `bin/labdrian-overlay:1612` runs `git checkout main` under an EXIT trap that restores the original
  branch. Pre-existing behaviour shared by every target, not introduced by this change; Pi is simply
  the first target whose `sync-check` can detect the resulting build/branch mismatch. This is also
  why the smoke pass above drives `engine pipkg build` directly instead of `labdrian-overlay apply`.
  Not blocking.

- **W-07 (new) — the N-01 remediation batch is absent from `apply-progress.md`.** Every earlier
  corrective batch got its own section ("Remediation 1", "Remediation 2", "Manual Verification");
  `d12c97f` and `50f2266` got none. `rg 'N-01|S-01|S-02|d12c97f|50f2266'` over `apply-progress.md`
  returns nothing, and the file's mtime predates both commits. The implementation is correct and
  independently verified here, so this is an audit-ledger gap rather than a code or spec defect, and
  this report now carries that evidence in its place. It does not block archive, but a future reader
  of the archived `apply-progress.md` will see a work history that stops one batch short of the
  delivered tree. Not blocking.

**SUGGESTION**

- **S-03 (carried) — The live pass observed 3 of the 12 skills the package ships.**
  `apply-progress` 6.1 records `gadu-operator`, `gadu-orchestrate`, `sdd-time-estimation`; the built
  package contains twelve skill directories plus `_shared`. Discovery is proven, coverage is not. A
  single follow-up live session that lists the full skill set, names one package agent, and runs one
  `--no-extensions` comparison on a machine where the provider survives that flag would close
  PARTIAL scenarios 1–3 in one pass.

- **S-04 (new) — `tasks.md` 3.6 still describes the wrong premise.** The task text reads
  "Add `--no-extensions`/`--no-skills` disclosure text (no `-ns` alias)". That was an accurate
  record of what was believed when the task was written, and `tasks.md` is an audit artifact rather
  than a merged source of truth, so it is deliberately left alone. Flagged only so nobody later
  mistakes it for a live requirement: the spec, the README, the Go adapter and the bash branch all
  say the aliases exist.

### TDD Compliance

| Check | Result | Details |
|---|---|---|
| TDD Evidence reported | Yes | 5 slice tables, 2 remediation tables, and a dated manual-verification block in `apply-progress` — minus the final batch (W-07) |
| All tasks have tests | Yes | 28/28 tasks complete; every code task maps to a named test file; 5.4, W-02 and 6.1–6.3 are docs/spec/manual by design |
| RED confirmed (test files exist) | Yes | Every named file located. The N-01 fix carries its own RED shape: the pre-existing assertion in `overlay_pi_package_build_test.go` actively asserted the *wrong* claim (`must not claim a '-ns' alias exists`), so it had to be inverted before the fix could be green — a falsifiable transition, not an added-after-the-fact assertion |
| RED evidence is real, not asserted | Yes | Remediations record falsifiable pre-fix strings (`got: supported, want partial`; `mcp.json.bak: extra`; `got 0, want 1`). For N-01 the pre-fix string is the shipped sentence itself, quoted in the pass-three report and now absent from the tree |
| GREEN confirmed (tests pass now) | Yes | Both full suites re-run from scratch this pass, engine under `-race`; 13 + 17 packages ok |
| Triangulation adequate | Yes | The disclosure claim is now pinned from two directions (positive `(short aliases: -ne and -ns)` and negative `no short alias`) across two independent shelltest files, plus the Go constant both must match |
| Safety net for modified files | Yes | Each batch records the relevant suite green before edit |
| Live-machine isolation | Yes | `TestMain` guards in `engine/runtime/live_guard_test.go` and `engine/cmd/live_guard_test.go`; the real package and the real settings entry survived this pass intact |

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

This pass re-audited only the assertions that changed, since the rest were audited in pass three
and their files are untouched:

- `overlay_pi_target_test.go:133` — a conjunction of a positive and a negative on the same string.
  The positive alone would pass on a message that contained both the new clause and the old denial;
  the negative alone would pass on a message that dropped the subject entirely. Together they pin
  the sentence. Correct shape for a claim that already drifted once.
- `overlay_pi_package_build_test.go:66` — inverted from a negative to a positive. The previous form
  is worth naming as a lesson: it was a *correct-looking* assertion encoding a false premise, and it
  actively defended the bug. A test can be green, meaningful, and wrong at the same time.
- `pi_test.go:506` — `wantContains` narrowed from `"labdrian-pi package is built, in sync, and
  listed"` (a prefix that survives dropping the third entry) to `"longterm-mem is registered in its
  mcp.json"` (the clause that is actually at risk). Strictly stronger.

The gap named in pass three — `TestPiAdapter_StatusDisclosesNoExtensionsNoSkills` asserting only
that the flag names appear, not what is said about them — is now covered at the shelltest layer on
both branches. The Go-side test was left as-is, which is acceptable: the shared constant it reads is
the same one the shelltests pin.

**Assertion quality**: All assertions verify real behavior.

### Quality Metrics

**Formatter**: `gofmt -l` clean in both modules.
**Vet**: `go vet ./...` clean in both modules.
**Linter (shell)**: `shellcheck -S warning` reports only the 2 pre-existing SC2064 warnings.
**Race detector**: `go test -race` clean across all 13 engine packages.

### Verdict

**FAIL — not archive-ready.** 0 CRITICAL, 4 WARNING, 2 SUGGESTION, **1 blocker (B-01).**

This is the unusual case where a `fail` verdict records no defect. Every line of code this pass
examined is correct, every suite is green, and the blocker of the previous pass is genuinely gone.

N-01, the single blocker of pass three, is fixed and confirmed three independent ways: the diff,
a repo-wide sweep proving no live string still denies the aliases, and a scratch runtime execution
of the exact branch a new user hits first. It arrived with the two SUGGESTIONs it was related to —
S-01's stale "slice 2" deferrals and S-02's under-reporting `supported` message — both also
confirmed fixed in source and at runtime. Crucially, the fix came with the assertion that was
missing: two shelltest checks now pin the bash disclosure to the Go adapter's wording from both
directions, so the two copies of that sentence cannot drift apart silently again, which is exactly
how N-01 survived the first alias correction.

Everything else held. Both suites were re-run from scratch and are green, engine under `-race`
across 13 packages and longterm-mem across 17. `gofmt`, `go vet`, and `shellcheck` are clean apart
from the two known pre-existing SC2064 warnings, whose line numbers moved by exactly one — itself
evidence that the bash change was the one-line string fix it claimed to be. All 28 tasks are
complete (corrected from the 26 the prior three passes reported). Planned and realized slice counts
match at 5. The live machine was untouched: the real `labdrian-pi` package and the real settings
entry survived every suite and every smoke command.

What blocks archive is **B-01**: 27 of 33 scenarios have passing covering evidence, and six do not.
This report must be equally clear that B-01 is not a new finding — it is a correction of pass
three's own ruling. Those six PARTIALs have been named in every pass; pass three declared them
"not a blocker" while a different blocker was already forcing `fail`, so the declaration was never
tested. Removing N-01 tested it, and it broke immediately: the validator refuses a passing verdict
at 27/33 and admits the identical bytes at 33/33. The counts cannot honestly be raised — each of
the six scenarios asserts something only a real Pi session can show, and this phase re-read all six
against their spec text rather than trusting the inherited summary. So the honest report is `fail`,
and the change is one live session (or one scoped spec decision) away from archive rather than one
line of code away.

A verification phase that had carried pass three's ruling forward unexamined would have written
`pass_with_warnings`, had it denied by the validator, and then been tempted to reconcile the
denial by adjusting the counts. Naming the ruling as the blocker is the same fix applied to
process that `d12c97f` applied to the disclosure string: two copies of a claim had drifted, and
the cheaper one had been winning.

Three WARNINGs besides B-01's evidence gap remain, none blocking: W-05 (package root `0700` vs the
design's `0755`, already issue #312), W-06 (`apply` builds from `main`, pre-existing and shared by
every target), and W-07, new this pass — the batch that fixed N-01 was never written into
`apply-progress.md`. W-07 is an audit-ledger gap, not a defect: the work is real, verified here,
and this report carries the evidence the ledger is missing.

**Recommended next step**: **not** `sdd-archive`. Take one of B-01's two exits first — a live Pi
session that closes the six scenarios, or an `sdd-spec` pass that narrows the three
pi-mcp-adapter-owned scenarios to what this overlay actually owns. Then re-verify. Nothing in the
implementation needs to change; `sdd-apply` has no work to do.
