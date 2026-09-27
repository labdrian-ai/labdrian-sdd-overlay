# Verification Report: `procedural-memory-lifecycle`

- **Verification date:** 2026-09-21
- **Branch:** `feat/procedural-memory-lifecycle-7b-retirement-detector`
- **HEAD:** `f983e74` (`feat(skills): add read-only retirement detector`)
- **Verification status:** **PARTIAL — not a clean PASS**

## Executive summary

The implemented slices through Phase 7b are behaviorally green: the engine and longterm-mem vet/test suites, the configured cross-module test command, shell checks, focused detector/retirement tests, temporary-repository acceptance procedures, Engram fixture checks, and diff-integrity checks passed. The verifier made no source, commit, or unrelated-tree changes.

This report remains **PARTIAL**, not a clean PASS. Final verification tasks F.1–F.6 were executed and persisted as checked, with F.5 and F.6 explicitly recording their partial outcomes. The retirement rollback-of-rollback diagnostic branch was not exercised, and the Codex smoke test verifies project-skill discovery but not skill-body loading on this host. A canonical strict-TDD evidence table for slices 5–7b was appended to `apply-progress.md`; historical baseline counts that were not recorded remain disclosed rather than reconstructed.

## Native structured status

Consumed the authoritative `gentle-ai.sdd-status` v2 projection before verification and again after checks:

| Field | Observed value |
|---|---|
| `changeName` | `procedural-memory-lifecycle` |
| `artifactStore` | `openspec` |
| `artifacts` | proposal/specs/design/tasks/apply-progress `done`; verify report was missing at start |
| `taskProgress` at verification start | `112/118` complete; `6` pending; `allComplete: false` |
| `dependencies` | proposal/specs/design/tasks `all_done`; apply/verify/archive `ready` |
| `nextRecommended` | `apply` |
| `blockedReasons` | `[]` |
| `actionContext.mode` | `repo-local` |
| `actionContext.workspaceRoot` | `/home/labdrian/labdrian-sdd-overlay` |
| `actionContext.allowedEditRoots` | `/home/labdrian/labdrian-sdd-overlay` |

Native status, not this report, remains the readiness authority. Verification did not invoke lifecycle mutation, apply, archive, commit, push, or PR actions.

## Inputs and implementation ownership

Read the native-resolved proposal, all six specs, design, tasks, cumulative `apply-progress.md`, `codex-smoke.md`, `openspec/config.yaml`, and `skills/_shared/procedural-candidate-detection.md`. Implementation ownership is inside the authoritative workspace and allowed edit root, including:

- `engine/skills/project_register.go`
- `engine/skills/project_cli.go`
- `engine/skills/lifecycle.go`
- `longterm-mem/internal/staleness/staleness.go`
- `longterm-mem/internal/skillstale/skillstale.go`
- `longterm-mem/cmd/longterm-mem/cmd_skills_stale.go`

The branch history shows item-30 candidate matching before this change's slices, followed by registration, promotion, revision, retirement, and detector commits.

## Final native status read

After the task checkbox and artifact reconciliation, a fresh read-only `gentle-ai sdd-status procedural-memory-lifecycle --cwd /home/labdrian/labdrian-sdd-overlay --json --instructions` returned:

| Field | Final value |
|---|---|
| `schemaName` / `schemaVersion` | `gentle-ai.sdd-status` / `2` |
| `taskProgress` | `118/118` complete; `0` pending; `allComplete: true` |
| `applyState` | `all_done` |
| `nextRecommended` | `archive` |
| `blockedReasons` | `[]` |
| `artifactStore` | `openspec` |
| `actionContext.mode` | `repo-local` |

This native recommendation is authoritative. The report remains partial because it preserves the two explicit verification limitations above; no apply or archive mutation was invoked.

## Spec and requirement coverage

| Capability/spec | Result | Evidence and limitations |
|---|---|---|
| `skill-lint` | PASS | `LintSkill` rule/contract tests and `AddCore` hard-gate tests pass; warnings remain non-blocking. |
| `procedural-skill-drafting` | PASS with recorded warning | Contract tests pass. Three invalid lesson-shape forms were refused before draft persistence; a clean imperative/why summary persisted and registered. `incident-log-shape` remained advisory. |
| `procedural-skill-registration` | PASS | Temporary Git repositories covered dry-run/write output, exact staging, scoped commit/revert, ignored targets, detached `HEAD`, merge state, root selection, staged-set mismatch recovery, hook-rewrite ownership, and second-registration crash recovery. |
| `procedural-skill-maintenance` | PASS with known gap | Revision ownership/hash, revision metadata, absorbed-target existence, retirement ownership, rollback, and report-only behavior passed. The rollback-of-rollback diagnostic branch was not exercised. |
| `skill-lifecycle` | PASS | `AddCore` refuses hard lint before registry/manifest writes; global promotion remains human-gated. |
| `procedural-candidate-detection` | PASS with scoped Codex limitation | Candidate/draft records, history transitions, status/hash fields, contract sections, and `MatchCandidate` behavior were verified. Codex discovery is recorded as a scoped PASS for `.agents/skills/<id>/`; body loading was inconclusive because the host filesystem sandbox failed. |

## Task completion

At the start of verification, implementation tasks were checked through 7b.8 and the six final verification lines below were still open. This is preserved as the historical snapshot:

```text
- [ ] F.1 `cd engine && go vet ./... && go test ./...`
- [ ] F.2 `cd longterm-mem && go vet ./... && go test ./...`
- [ ] F.3 Confirm no file was written under `skills/` by any project-tier code path outside `AddCore`'s own tests (`git diff --stat` review across all 13 merged PRs)
- [ ] F.4 Confirm no test in either module touched a live `.claude/skills/`, `.agents/skills/`, `.pi/`, `skills-lock.json`, the user's home directory, or spawned a live runtime binary (temp-dir-only rule)
- [ ] F.5 Run the full acceptance checklist from sections 11 (Phase 4), and the sdd-verify checklists implied by Phases 2, 6, 7a-i, 7a-ii against real Engram records and a temp git repo; record results honestly, including any scenario that could not be exercised (e.g. the rollback-failure print path, or the Codex smoke test if `UNVERIFIED`)
- [ ] F.6 Confirm every Success Criteria checkbox in `proposal.md` is satisfied or explicitly recorded as a known gap (in particular the Codex-support claim, gated strictly by the recorded smoke-test verdict)
```

The final reconciliation then executed F.1–F.6, recorded the partial/skipped outcomes required by those task definitions, and updated the persisted task artifact. All six final verification tasks are now visibly checked; F.5 and F.6 being checked records completion of the evidence-gathering and reconciliation work, not a claim that the unavailable scenarios passed.

| Task | Final checkbox | Final result |
|---|---|---|
| F.1 | `[x]` | PASS; engine vet and full tests exited 0 |
| F.2 | `[x]` | PASS; longterm-mem vet and full tests exited 0 |
| F.3 | `[x]` | PASS; project-tier diff review found no runtime writes under `skills/` |
| F.4 | `[x]` | PASS; temporary-fixture and no-live-runtime rule reconciled |
| F.5 | `[x]` | PARTIAL; acceptance evidence passed where exercised, with rollback-failure and Codex body-loading limits recorded |
| F.6 | `[x]` | PARTIAL; every criterion maps to evidence or an explicit known gap |

## Final verification task evidence

| Task | Result | Evidence |
|---|---|---|
| F.1 | PASS; checkbox persisted `[x]` | `cd engine && go vet ./... && go test ./...` — exit 0. |
| F.2 | PASS; checkbox persisted `[x]` | `cd longterm-mem && go vet ./... && go test ./...` — exit 0. |
| F.3 | PASS; checkbox persisted `[x]` | `git diff --stat main...HEAD -- skills` shows only the shared contract and expected style/skill documentation changes; no project-tier runtime target write was introduced. |
| F.4 | PASS; checkbox persisted `[x]` | Changed tests use temporary fixtures; acceptance used isolated temporary repositories/Engram databases. No live `.claude/skills/`, `.agents/skills/`, `.pi/`, `skills-lock.json`, home directory, or live runtime was used by the verified scenarios. |
| F.5 | PARTIAL; checkbox persisted `[x]` | Registration checklist, revision/retirement acceptance, drafting acceptance, and real Engram records passed where exercised. The retirement rollback-of-rollback diagnostic branch is explicitly **SKIPPED/UNAVAILABLE**; Codex body loading remains **UNVERIFIED/INCONCLUSIVE**. |
| F.6 | PARTIAL with explicit scoped gap; checkbox persisted `[x]` | Every proposal criterion maps to passing implementation evidence or an explicit limitation below. Codex discovery is verified; body loading is **UNVERIFIED/INCONCLUSIVE**. |

## Proposal success criteria mapping

1. **Item-30 dependency before application — PASS.** Branch history contains the item-30 candidate-store/matching commits before this change's implementation slices.
2. **Codex discovery recorded before targeting — PASS with scoped limitation.** `codex-smoke.md` records Codex 0.148.0 commands and output; `.agents/skills/<id>/` discovery passed. Body loading is **UNVERIFIED/INCONCLUSIVE** due the disclosed `bwrap: loopback: Failed RTM_NEWADDR` host limitation.
3. **Lint rules and boundaries — PASS.** Rule-table, hard-rule, warning, generated-style, and contract tests pass.
4. **`AddCore` hard refusal before writes — PASS.** Focused and full engine tests pass, including byte-preservation/refusal coverage.
5. **Registration targets/provenance/hash/traversal — PASS.** Temporary-repository acceptance and project-register tests cover all configured targets, lock metadata, provenance, hash, and traversal refusal.
6. **Exactly one scoped registration commit and staged refusal — PASS.** Acceptance covered exact staging, unrelated staged-file refusal, commit, and revert.
7. **Human-owned revision refusal — PASS.** Hash mismatch, missing target, and extra-entry cases refuse without agent mutation and name ownership.
8. **Report-only retirement detector — PASS.** Detector tests and static review show no writes to skills, lock, or Engram; removed paths are reported.
9. **History append-only behavior — PASS.** Contract tests and Engram candidate/draft fixtures verified the prior history as a prefix after registration.
10. **No project-tier writes under `skills/`; no live-directory tests — PASS.** Diff review and temporary-fixture acceptance support the restriction.
11. **Engine and longterm-mem suites — PASS.** Exact commands and exit codes are listed below.

## Commands and observed results

All commands below exited 0 unless stated otherwise:

```text
cd /home/labdrian/labdrian-sdd-overlay/engine && go vet ./... && go test ./...
cd /home/labdrian/labdrian-sdd-overlay/longterm-mem && go vet ./... && go test ./...
cd /home/labdrian/labdrian-sdd-overlay && cd longterm-mem && go test ./... && cd .. && cd engine && go test ./... && cd ../tui && go test ./... && cd .. && for m in tools/*/go.mod; do (cd "$(dirname "$m")" && go test ./...) || exit 1; done
cd /home/labdrian/labdrian-sdd-overlay && bash -n bin/overlay && shellcheck bin/overlay
cd /home/labdrian/labdrian-sdd-overlay/longterm-mem && go test ./internal/staleness/... ./internal/skillstale/... ./cmd/...
cd /home/labdrian/labdrian-sdd-overlay/engine && go test ./skills -run TestSerializeProjectLock_MatchesSharedSkillstaleFixture
cd /home/labdrian/labdrian-sdd-overlay && git diff --check
```

The configured cross-module command passed for `longterm-mem`, `engine`, `tui`, and all actual `tools/*` Go modules. The focused detector, fixture-pinning, engine, shell, and diff checks also passed.

## Strict TDD compliance

Strict TDD is active in `openspec/config.yaml` (`testing.strict_tdd: true`) with the configured cross-module Go test runner. The global verify guidance was applied.

### TDD evidence audit

The final reconciliation appended a canonical task-level `TDD Cycle Evidence` table to `apply-progress.md` for every task in slices 5, 6, 7a-i, 7a-ii, and 7b. The table uses the support contract's `Safety Net`, `RED`, `GREEN`, `TRIANGULATE`, and `REFACTOR` columns and explicit `✅`/`➖` markers. It preserves historical RED/GREEN observations without reverting implementation, and uses `N/A` for documentation-only or verification-only tasks.

All reported changed test files exist: `engine/skills/lifecycle_test.go`, `engine/skills/skills_test.go`, `engine/skills/project_register_test.go`, `engine/skills/project_cli_test.go`, `engine/skills/procedural_candidate_contract_test.go`, `engine/skills/project_lock_test.go`, `longterm-mem/internal/staleness/staleness_test.go`, `longterm-mem/internal/skillstale/skillstale_test.go`, and `longterm-mem/cmd/longterm-mem/cmd_skills_stale_test.go`. Current focused and full test execution is GREEN for those files/packages.

| Check | Result | Details |
|---|---|---|
| TDD evidence reported | ✅ Reconciled | A task-level table with the required columns and markers is persisted in `apply-progress.md`. |
| All implementation tasks have a mapping | ✅ Reconciled | Slices 5–7b map each task to its test or explicit document/verification evidence surface. |
| RED confirmed | ✅ Historical evidence retained | Historical compile/test failures are recorded for behavior tasks; structural/document and verification-only tasks are explicitly marked `➖` rather than given invented RED runs. |
| GREEN confirmed | ✅ | Relevant focused, vet, and full suites pass now. |
| Triangulation adequate | ✅ With explicit structural exceptions | Behavioral tasks list distinct happy/edge-path coverage; structural and verification-only tasks state why no second behavior case applies. |
| Safety net for modified files | ⚠️ Historical counts incomplete | Earlier apply entries recorded baseline commands but not numeric test counts; the table says “count not captured” and does not fabricate them. |

**TDD compliance:** **PARTIAL, with evidence reconciled.** The required table and historical cycle evidence now exist, but exact historical safety-net counts were not preserved and no RED run is claimed for prose-only or verification-only tasks. Functional behavior remains GREEN.

### Test layer distribution

| Layer | Files | Result |
|---|---:|---|
| Go unit/contract/FS tests | 6 primary engine test files | PASS; production calls and behavioral outputs are asserted. |
| Go temp-repository/Engram/CLI integration tests | 3 primary longterm-mem/CLI test files | PASS; isolated fixtures exercise Git, lock, Engram, and command boundaries. |
| Browser/E2E tests | 0 | Not applicable to this change. |

### Assertion quality

**Assertion quality: ✅ All audited assertions verify real behavior.** No tautologies, ghost loops, assertion-only tests, smoke-only tests, CSS/implementation-detail assertions, or mock-heavy test smell was found in the changed test coverage. Empty-result assertions in staleness/retirement cases are paired with non-empty behavior cases and test an explicit clean/refusal contract.

### Coverage and quality metrics

Coverage analysis was skipped because no configured coverage tool was available; `coverage_threshold` is `0`. `go vet ./...`, focused/full Go tests, `bash -n`, and ShellCheck passed. No separate type-checker or Go linter was configured.

## Acceptance and Engram evidence

- Registration acceptance passed against isolated temporary Git repositories, including dry-run/write output, exact target staging, one scoped commit, `git revert`, ignored targets, detached `HEAD`, merge state, root selection, staged-set mismatch recovery, hook rewrite ownership, and second-registration crash recovery. The retirement rollback-of-rollback output branch was **SKIPPED/UNAVAILABLE** in final acceptance; no pass is claimed for it.
- Revision/retirement acceptance passed for human-owned refusals, hash-matched revision to revision 2, absorbed-target existence validation, dry-run, scoped retirement commit, and `git revert`. The unexercised rollback-of-rollback branch remains a documented limitation.
- Drafting acceptance rejected three lesson-shape violations before persistence and accepted a clean imperative/why summary; advisory `incident-log-shape` did not block.
- Engram fixtures were saved and read back: candidate/draft records `3531`/`3532`, rejected candidate `3535`; the registered candidate history was updated and no rejected draft was persisted.
- Codex smoke verdict remains **PASS only for `.agents/skills/<id>/` discovery**; body loading is **UNVERIFIED/INCONCLUSIVE** on this host because the filesystem sandbox failed.

## Review workload and boundary

The recorded delivery strategy is `auto-chain` with `feature-branch-chain`. Prior slices above the nominal review budget carry the documented owner-granted `size:exception`; Phase 7b is the assigned PR 13 boundary. Verification did not implement scope beyond the assigned final checks, did not create a new PR slice, and did not alter unrelated untracked trees.

## Working-tree integrity

- `git diff --check` — PASS.
- Branch and HEAD remained `feat/procedural-memory-lifecycle-7b-retirement-detector` / `f983e74`.
- No source, commit, push, or PR mutation was performed by verification.
- Existing unrelated untracked content remained present, including `.agents/`, `.claude/skills/archify`, `.pi/`, `skills-lock.json`, and `docs/architecture/laya-system-one-overlay-plan.md`. These paths were not modified.
- The intended verification artifact updates are this report, the cumulative `apply-progress.md` reconciliation, and the six final task checkboxes; no source file was changed.

## Findings and exact blockers

### WARNING

1. The retirement rollback-of-rollback diagnostic path (`error: rollback incomplete: <rel-path>`) was **SKIPPED/UNAVAILABLE** during final acceptance; no pass is claimed for that scenario.
2. Codex skill-body loading remains **UNVERIFIED/INCONCLUSIVE** because the host filesystem sandbox failed; only discovery/name/description exposure is verified.
3. Historical apply entries did not preserve exact numeric safety-net counts. The canonical table records that limitation explicitly and does not invent counts.

## Recommended next action

Run a fresh native status read. Preserve the partial findings when proceeding to the native archive recommendation; do not claim a clean verification PASS unless the two documented scenario limitations are independently resolved.

## Phase envelope

```yaml
status: partial
executive_summary: >-
  Implementation and functional verification are green through Phase 7b, and
  F.1-F.6 are persisted as checked with partial outcomes recorded. The
  rollback-of-rollback scenario remains unexercised and Codex body loading is
  unverified on this host; strict-TDD evidence is reconciled with disclosed
  historical baseline-count limits.
artifacts:
  verify_report: openspec/changes/procedural-memory-lifecycle/verify-report.md
next_recommended: archive
risks:
  - WARNING: retirement rollback-of-rollback branch skipped/unavailable
  - WARNING: Codex body loading unverified/inconclusive on this host
  - INFO: historical safety-net counts were not captured
skill_resolution: paths-injected
```
