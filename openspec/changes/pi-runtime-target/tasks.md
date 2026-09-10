# Tasks: Pi Runtime Target

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~960–1900 across 5 slices |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR1 pi-target-plumbing → PR2 pi-package-build → PR3 pi-contract-gate → PR4 pi-longterm-mem-mcp → PR5 pi-lifecycle |
| Delivery strategy | auto-chain |
| Chain strategy | stacked-to-main |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | PR | Focused test | Runtime harness | Rollback boundary |
|---|---|---|---|---|---|
| 1 pi-target-plumbing | Target enum/adapter skeleton, bin dispatch, no TARGET_PATHS entry | PR1 | `cd engine && go test ./runtime/... ./cmd/... ./shelltest/...` | `bin/labdrian-overlay status --target pi` | revert `engine/runtime/pi.go`, `TargetPi` additions |
| 2 pi-package-build | `engine/pipkg` build/drift, skills.registry.yaml pi target | PR2 | `cd engine && go test ./runtime/...` | `bin/labdrian-overlay apply --target pi` builds package dir | revert `engine/pipkg/`, registry entries |
| 3 pi-contract-gate | `labdrian-gate.ts` extension, path-line injection | PR3 | `cd engine && go test ./runtime/...` | live Pi session on sdd-tasks/sdd-apply (manual) | revert `engine/pipkg/labdrian-gate.ts`, embed wiring |
| 4 pi-longterm-mem-mcp | `register --target pi`, mcp.json | PR4 | `cd longterm-mem && go test ./internal/register/... ./cmd/...` | live Pi session lists longterm-mem MCP (manual) | revert `longterm-mem/internal/register/pi.go` |
| 5 pi-lifecycle | Honest status, `pi remove` uninstall, docs | PR5 | `cd engine && go test ./runtime/...` | `bin/labdrian-overlay uninstall --target pi` (manual) | revert `PiAdapter.Status/Uninstall`, README/spec edits |

## Phase 1: pi-target-plumbing (R-001, R-008)

- [x] 1.1 RED: `TestExpandTarget_Pi` in `engine/runtime/runtime_test.go`; `TestResolveTargets_Pi`/`TestIsCopyTarget_ClaudeTrue_PiFalse` placed in `engine/shelltest/overlay_pi_target_test.go` instead (they exercise bash functions in `bin/labdrian-overlay`, not the Go `runtime` package — see Deviations below)
- [x] 1.2 GREEN: added `TargetPi` to `Target` enum, `ExpandTarget`, `NewFoundationAdapter`; skeleton `engine/runtime/pi.go` (`PiAdapter`, `Target()` wired, every other method an honest `CapabilityUnsupported` stub)
- [x] 1.3 Added `TARGET_KINDS`/`is_copy_target`/`is_valid_target`/`package_target_stub_message` to `bin/labdrian-overlay`; updated `resolve_targets` and all 8 `TARGET_PATHS`-keyed call sites (cmd_capture, cmd_apply loop, cmd_restore, cmd_status loop, cmd_sync_check loop, cmd_repair_sdd_registry, cmd_skill_registry) so `pi` is either dispatched to an honest stub (apply/status/sync-check) or rejected with a clear package-target message (capture/restore/repair-sdd-registry/skill-registry) — never an empty-path `mkdir`. Also fixed a regression this exposed in `cmd_longterm_mem` (out of scope for this slice): its `--target all` expansion now excludes `pi` explicitly, since longterm-mem's own register binary has no `pi` case.
- [x] 1.4 Verify non-regression: `shellcheck -S warning bin/labdrian-overlay` (only the 2 pre-existing SC2064 warnings); re-ran claude/opencode/codex `status`/`sync-check` — byte-identical output confirmed by diff against the pre-change script

## Phase 2: pi-package-build (R-002, R-003, R-010)

- [x] 2.1 RED (threat: path/symlink): `TestPipkgBuild_RejectsSymlinks`, `TestPipkgBuild_AtomicSwap` in `engine/pipkg/pipkg_test.go`
- [x] 2.2 RED: `TestPipkgBuild_SelectsPiTargetedSkills`, `TestPipkgCheck_DetectsDrift`
- [x] 2.3 GREEN: `engine/pipkg/pipkg.go` `Build`/`Check` — temp-dir build, atomic swap, 0644/0755 modes
- [x] 2.4 Add `pi` as valid `install.targets` value in `skills.registry.yaml`; wire `Install`/`Apply`/`SyncCheck` in `engine/runtime/pi.go`
- [x] 2.5 Wire `apply`/`sync-check` dispatch to pipkg in `bin/labdrian-overlay`

## Phase 3: pi-contract-gate (R-004, R-007)

- [x] 3.1 RED (threat: path containment): `TestLabdrianGatePathContainment_RejectsTraversal` in `engine/runtime/pi_test.go`
- [x] 3.2 RED: `TestLabdrianGateInjectsPathLine_SddTasksSddApply` (Go-side oracle via `runtime.CanonicalEntry`/`runtime.InjectPrompt` for the same contracts/header)
- [x] 3.3 GREEN: `engine/pipkg/labdrian-gate.ts` — `before_agent_start` handler, strict frontmatter parse, path-line injection, chained `systemPrompt`
- [x] 3.4 Embed and copy extension into package build (`engine/pipkg/pipkg.go`); also copies `skills/_shared/{minimalism-contract,anti-generic-design}.md` (not registry-driven — gate infrastructure, not an installable skill)
- [x] 3.5 Manual: `TestLabdrianGateChainsAfterGentlePi` (node, `.ts`→`.mjs` fixture copy, skip if node absent) — node was available in this environment so the test ran (not skipped)
- [x] 3.6 Add `--no-extensions`/`--no-skills` disclosure text (no `-ns` alias) to `pipkg_status_and_report` (`bin/labdrian-overlay`)

## Phase 4: pi-longterm-mem-mcp (R-005)

- [ ] 4.1 RED: `TestRegisterPi_WritesMcpServersLongtermMem` in `longterm-mem/internal/register/pi_test.go`
- [ ] 4.2 RED: `TestCmdRegister_TargetAll_SkipsAbsentPi`, `TestCmdRegister_TargetPiExplicit_FailsWhenPackageAbsent`
- [ ] 4.3 GREEN: `longterm-mem/internal/register/pi.go` — `RegisterPi` writes `<packageDir>/mcp.json` via existing `jsonInstall(containerKey="mcpServers")`
- [ ] 4.4 Add `--target pi` case to `longterm-mem/cmd/longterm-mem/cmd_register.go` with skip/fail probe semantics
- [ ] 4.5 `engine/pipkg/pipkg.go`: emit `package.json` `pi.mcp` path + `mcp.json` skeleton

## Phase 5: pi-lifecycle (R-006, R-009)

- [ ] 5.1 RED: `TestPiAdapter_InstallNoShellInjection` (threat: process integration, fixed argv, `exec.LookPath`)
- [ ] 5.2 RED: `TestPiAdapter_StatusPartialOnUnprovenEntry`, `TestPiAdapter_StatusDisclosesNoExtensionsNoSkills`, `TestPiAdapter_UninstallUsesRemoveNotUninstall`, `TestPiAdapter_UninstallNeverTouchesGentlePiFiles`
- [ ] 5.3 GREEN: `engine/runtime/pi.go` `Status` (honest per-entry), `Uninstall` (`pi remove <source>`), `Update`/`Rollback`
- [ ] 5.4 Update `openspec/specs/runtime-lifecycle/spec.md` and `README.md`

## Phase 6: Manual-Only Verification (live Pi, post-chain)

- [ ] 6.1 `pi install <local-path>` then open a Pi session: GADU skill/agent discoverable, no file under `~/.pi/agent/agents/` written
- [ ] 6.2 Observe gate injection in real `sdd-tasks`/`sdd-apply` system prompt; confirm every other agent unaffected
- [ ] 6.3 Confirm `pi remove <source>` leaves gentle-pi/pi-engram entries byte-identical
- [ ] 6.4 Broad: `cd engine && go vet ./... && go test ./...`; `cd longterm-mem && go vet ./... && go test ./...`
