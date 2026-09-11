# Tasks: Pi Package Hardening and GADU as a Real Pi Subagent

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 1380–1850 total across 5 slices |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR1 pipkg-integrity → PR2 sync-check-provenance → PR3 review-receipt-capture → PR4 receipt-anchor-gate → PR5 gadu-pi-subagent |
| Delivery strategy | auto-chain |
| Chain strategy | stacked-to-main |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | pipkg-integrity (R-001..R-004) | PR 1 | `cd engine && go test ./pipkg/...` | N/A — pure filesystem/parse unit tests | `engine/pipkg/pipkg.go`, `engine/skills/parse.go` revert |
| 2 | sync-check-provenance (R-005..R-007) | PR 2 | `cd engine && go test ./pipkg/... ./cmd/...` | git-in-TempDir scenarios (skip `-short`) | `engine/pipkg/pipkg.go` provenance fields, `cmd_sync_check` revert |
| 3 | review-receipt-capture (R-008) | PR 3 | `cd engine && go test ./cmd/... ./shelltest/...` | fake `pi`/hook invocation via scratch repo | `engine/reviewreceipt/`, hook registration revert |
| 4 | receipt-anchor-gate (R-009..R-011) | PR 4 | `cd tools/archive-anchor-gate && go test ./...` | scratch archived-change fixture | `tools/archive-anchor-gate` changes, inception-pipeline Plan edit revert |
| 5 | gadu-pi-subagent (R-012..R-016) | PR 5 | `cd engine && go test ./runtime/...` | scratch HOME + `LABDRIAN_PI_BIN` stub script | `engine/runtime/pi.go` extension/link logic revert |

## Phase 1: pipkg-integrity (PR 1, R-001..R-004)

- [x] 1.1 RED: `engine/pipkg/pipkg_test.go` TestCheck_ModeDrift — 0644→0755 reported; symlinked destDir root refused.
- [x] 1.2 RED: `engine/skills/parse_test.go` TestValidateEntry_PathTraversal — `../`, non-relative, unclean path rejected.
- [x] 1.3 RED: `engine/pipkg/pipkg_test.go` TestBuildInto_NameMismatch — SKILL.md `name` ≠ dir rejected; live registry passes.
- [x] 1.4 GREEN: `engine/pipkg/pipkg.go` — `listFiles` returns `{data,perm}`; `Check` diffs `Perm()`; `copyFile` keeps 0644; `Chmod(tmpDir,0755)` build root.
- [x] 1.5 GREEN: `engine/skills/parse.go` `validateEntry` — non-empty, relative, no `..`, `Clean(path)==path`.
- [x] 1.6 GREEN: `engine/pipkg/pipkg.go` `buildInto` — SKILL.md frontmatter `name==Base(e.Path)`; `Rel(skillsDir,dst)` no `..` prefix.
- [x] 1.7 Docs: `engine/pipkg/pipkg.go` doc comment — mode-drift + containment behavior.

## Phase 2: sync-check-provenance (PR 2, R-005..R-007)

- [ ] 2.1 RED: `engine/pipkg/pipkg_test.go` TestBuiltFrom_RecordedAtBuild — matches `git rev-parse HEAD` in t.TempDir (skip `-short`).
- [ ] 2.2 RED: TestCheck_RefBasis — sha `^[0-9a-f]{40}$` + `cat-file -e` resolvable → `git archive` compare, Basis=ref.
- [ ] 2.3 RED: TestCheck_MainFallback — unresolvable sha → Basis="main" with disclosure message; TestCheck_NonGitRoot → Basis="worktree".
- [ ] 2.4 RED: TestCheck_RejectsNonHexBuiltFrom — non-hex/`--option` never spawns git.
- [ ] 2.5 GREEN: `engine/pipkg/pipkg.go` `packageManifest.Labdrian{BuiltFrom}`, `resolvePackageVersion(root,rev)`.
- [ ] 2.6 GREEN: `Check` returns `(CheckReport{Basis,Ref}, error)` — ref/main/worktree basis logic per 2.2–2.4.
- [ ] 2.7 GREEN: `bin/labdrian-overlay` `cmd_sync_check` — print basis for `--target pi` / `SYNC_CHECK:pi:`.
- [ ] 2.8 Docs: `cmd_sync_check` usage note — basis disclosure line.

## Phase 3: review-receipt-capture (PR 3, R-008)

- [ ] 3.1 RED: `engine/reviewreceipt/reviewreceipt_test.go` TestCapture_SchemaAndTerminalState — `gentle-ai.review-receipt/v2` + `terminal_state==approved` required.
- [ ] 3.2 RED: TestCapture_AtomicWrite — byte-identical write to `review-receipts/<lineage_id>.json`.
- [ ] 3.3 RED: hook test — TestHook_PassThrough_NoOpenspecChanges; TestHook_Deny_MultipleActiveChanges (exit 2 naming `review-receipt capture --change <name>`); TestHook_Capture_SingleActiveChange.
- [ ] 3.4 GREEN: `engine/reviewreceipt/reviewreceipt.go` `Capture(repo,change)`.
- [ ] 3.5 GREEN: `bin/labdrian-overlay` `gentle-ai-overlay review-receipt hook` — fail-closed PreToolUse Bash hook matching `gentle-ai review acknowledge-approved`.
- [ ] 3.6 GREEN: install-hooks registers hook under third settings identity.
- [ ] 3.7 Docs: `engine/reviewreceipt/` package doc — fail-closed semantics, single-active-change rule.

## Phase 4: receipt-anchor-gate (PR 4, R-009..R-011; depends on Phase 3)

- [ ] 4.1 RED: `tools/archive-anchor-gate` TestApprovedTree_FromReceipt — verified via `final_candidate_tree` prefix match.
- [ ] 4.2 RED: TestArchiveBlock_NoReceiptPostConvention — blocks on/after `2026-09-12` without receipt; TestOverride_RecordedSelfAsserted — `override.json` + Cycle Timestamps `override` passes.
- [ ] 4.3 RED: TestPreArchiveFlag — `--change <name>` exits 1 when receipt missing pre-archive.
- [ ] 4.4 RED: closure-feedback test — reads `approved_tree`/`review_lens_count` from receipt file, not live state.
- [ ] 4.5 GREEN: archive-anchor-gate reads `review-receipts/*.json`, compares `approved_tree` prefix.
- [ ] 4.6 GREEN: `ReceiptConventionDate = "2026-09-12"` const + `override.json` schema check.
- [ ] 4.7 GREEN: `--change` flag wiring for pre-archive check.
- [ ] 4.8 GREEN: inception-pipeline closure-feedback reads receipt file; remove `.git/...` path from Plan.
- [ ] 4.9 Docs: inception-pipeline Gate Compliance list — add pre-archive receipt check before sdd-archive.

## Phase 5: gadu-pi-subagent (PR 5, R-012..R-016; depends on Phase 2)

- [ ] 5.1 RED: `engine/runtime/pi_test.go` TestSubagentsExtension_InstallWhenAbsent (via `LABDRIAN_PI_BIN` stub, disclosure line); TestSubagentsExtension_Noop (either prefix present); TestSubagentsExtension_SkipEnv (`LABDRIAN_PI_SKIP_SUBAGENTS=1`).
- [ ] 5.2 RED: TestGaduLinkState_Matrix — missing/current/stale/conflict, 4×2.
- [ ] 5.3 RED: TestGaduLink_SurvivesOverwrite — gentle-pi-style overwrite leaves our symlink intact.
- [ ] 5.4 RED: TestUninstall_OwnedLinkOnly — removes only our symlink, then `pi remove`; extension/conflict entries untouched.
- [ ] 5.5 RED: TestFrontmatter_InlineToolsScalar — `tools: '*'` stays a single inline scalar.
- [ ] 5.6 GREEN: `engine/runtime/pi.go` extension probe (`settings.json packages[]` prefix `npm:pi-subagents-j0k3r`/`npm:pi-subagents`) + `runPiCommand(bin,"install",...)`.
- [ ] 5.7 GREEN: symlink `~/.pi/agent/agents/GADU.md` → `<destDir>/agents/GADU.md`; `gaduLinkState(home,destDir,expected)`.
- [ ] 5.8 GREEN: status reports `subagents_extension`/`gadu_link`; uninstall removes only owned link, then `pi remove`.
- [ ] 5.9 GREEN: wire extension install inside `apply --target pi`.
- [ ] 5.10 Docs: pi-runtime-target status docs — two new status reasons.

## Phase M: Manual live-Pi checkpoints (MANUAL, not run by `sdd-apply`)

- [ ] M.1 MANUAL: On real machine, run `apply --target pi`; confirm `pi-subagents-j0k3r` installs and `GADU.md` links.
- [ ] M.2 MANUAL: Dispatch GADU via gentle-pi `subagent_*` from the overlay-linked file; confirm response.
- [ ] M.3 MANUAL: Run `pi remove npm:pi-subagents-j0k3r`; confirm byte-identical extension removal, overlay link handled per uninstall scope.
