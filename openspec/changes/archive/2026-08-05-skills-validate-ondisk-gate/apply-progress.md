# Apply Progress: skills-validate-ondisk-gate

Mode: Strict TDD. Both slices complete. 12/12 tasks done.

> Mirrored from Engram `sdd/skills-validate-ondisk-gate/apply-progress` (obs #2780). Written to disk after
> delivery consumed the review receipt — writing it earlier would have changed the frozen candidate tree and
> voided receipt `sha256:787b2d753fdd57c19c66bdc8161331621a5f0911e95704d6fba7c8a2389c1edf`.

## Branch / commit chain

Originally planned as a `feature-branch-chain`. Delivered as a **single PR** instead: the native review froze and
approved all five commits as ONE candidate, so no sub-range matches the receipt, and merging a first slice would
have moved `main` off the frozen `base_tree` and voided the receipt for the second.

- Tracker: `feat/skills-validate-ondisk-gate` (base `origin/main` @ `be2c3ca`)
  - `749f028` docs(sdd): add skills-validate-ondisk-gate planning artifacts — 932 lines, prior-phase output, not authored by apply
  - `c005a48` docs(sdd): add skills-validate-ondisk-gate tasks baseline — 45 lines, not authored by apply
  - `c986230` feat(skills): pin on-disk exclusion rules and name working remediation — **slice 1, 122+16 = 138 authored lines**
  - `955fc9a` feat(skills): wire the on-disk cross-check into skills validate — **slice 2, 308+18 = 326 authored lines**
  - `c780d5e` fix(skills): print registry divergences before on-disk stages can exit — **review correction R4-001, 42+5 = 47 lines**

Merged to `main` as `79995ea` (PR #130). `origin/main` tree is `b654d2d006d4d151145fd6452817f97d1ddc9ebb`, byte-identical
to the receipt's `final_candidate_tree`.

## TDD Cycle Evidence

| Task | RED | GREEN |
|---|---|---|
| 1.1 R-004 pin test | Test asserts `_shared` deployable in `ondisk.go` and excluded in `manifest.go`, both directions | Passed immediately — regression pin, not driving new code |
| 1.2 R-009 remediation text | `TestOnDiskDiagnosticsNameManifestRowEdit` failed: `Detail` lacked the manifest-row wording | Rewrote both `Detail` strings; verified no `sync-manifest` citation remains |
| 2.1 / 2.2 `--source-root` + scan seam | `TestRenderValidateCoreOnDiskGate` failed to compile (signature change), then 2 subtests failed on their own test bugs | Implemented flag parse, fail-loud, `scanSkills` param and wiring; fixed both test bugs |
| 2.3 `verb_validate` fixture | `TestSkillsCore/verb_validate` regressed (exit 1, missing `--source-root`) | Added a dedicated `dir/skills/sdd-spec/SKILL.md` subdirectory plus the flag; zero assertion-statement edits |
| R4-001 correction | `registry_divergence_survives_scan_failure` failed: only the scan error reached stderr, the registry divergence was discarded | Hoisted the registry-divergence print above the three on-disk stages |

## Files Changed

| File | Slice | Action |
|---|---|---|
| `engine/skills/ondisk.go` | 1 | `Detail` strings name the manifest-row edit (R-009) |
| `engine/skills/ondisk_test.go` | 1 | `TestInfraExclusionRulesArePinnedIndependently`, `TestOnDiskDiagnosticsNameManifestRowEdit` |
| `openspec/changes/skill-manifest-gen/proposal.md` | 1 | Scoped the `sync-manifest` claim to `*/SKILL.md` rows (it cannot clear an on-disk divergence) |
| `README.md`, `bin/labdrian-overlay`, `engine/cmd/main.go` | 1 | R-010 documentation sites |
| `engine/skills/skills.go` | 2 + correction | `--source-root` flag, injected `scanSkills` param, on-disk wiring; correction hoists the registry print |
| `engine/skills/skills_test.go` | 2 + correction | `TestRenderValidateCoreOnDiskGate`; `verb_validate` fixture; `registry_divergence_survives_scan_failure` |
| `.github/workflows/ci.yml` | 2 | `--source-root ../skills` on the validate step |
| `openspec/changes/deterministic-verification-evidence/specs/verification-evidence-capture/spec.md` | 2 | Decision 5: amended the scenario's GIVEN and WHEN |
| `openspec/changes/skills-validate-ondisk-gate/tasks.md` | both | All 12 tasks marked complete |

## Verification

Focused and broad Go suites green across `engine`, `tui`, and all three `tools/*` modules. `gofmt` 0, `go vet` 0,
`staticcheck` 0. Real-tree gate: `skills validate --registry ../skills.registry.yaml --manifest ../overlay.manifest
--source-root ../skills` exits 0 with "registry and manifest aligned (20 skills)" and "skills/ on disk matches
overlay.manifest (57 files)" — reproduced in CI on PR #130.

## Measurement

- Slice 1 authored: 138 lines · Slice 2 authored: 326 lines · Review correction: 47 lines
- Total authored: **511 lines** (464 pre-review + 47 correction)
- `git diff --stat be2c3ca..c780d5e`: 17 files changed, including 932 lines of prior-phase SDD planning artifacts
  persisted in this run and not authored by apply

## Deviations from Design

None in implementation. One delivery deviation: single PR instead of the planned chained PRs, forced by the
receipt's single tree-pair binding (see the branch section above).

## Status

12/12 tasks complete, review approved, merged to `main`. Ready for archive.
