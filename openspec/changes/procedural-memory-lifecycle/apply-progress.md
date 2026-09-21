# Apply Progress: procedural-memory-lifecycle

## Slice

- Change: `procedural-memory-lifecycle`
- Phase: 6 — `revision`
- Assigned tasks: 6.1 through 6.6 only
- Branch: `feat/procedural-memory-lifecycle-6-revision`
- Artifact store: OpenSpec
- Previous apply-progress: Phase 5 `global-promotion`; this Phase 6 entry is cumulative and preserves the prior evidence.

## Completed tasks and persisted checkboxes

- [x] 5.1 — Added RED/GREEN `AddCore` tests for hard lint refusal with byte-preserved registry and manifest, warnings-only success, and lint-clean existing fixtures. The RED run failed only because the lint gate was absent; the GREEN run passed.
- [x] 5.2 — Added the `LintSkillFile` gate in `AddCore` after SKILL.md existence validation and before serialization or manifest processing. Hard findings are printed and exit 1; warnings do not block.
- [x] 5.3 — Added contract section 12 documenting human-only promotion through the existing `engine skills add`/`AddCore` path, silence-is-not-consent, the `Promoted` hash line, and append-only `History` wiring.
- [x] 5.4 — Confirmed by the hard-lint regression that refusal returns before registry serialization, manifest processing, or either atomic write.
- [x] 5.5 — Completed focused and broad engine verification.
- [x] 6.1 — Added RED coverage for project-revise ownership refusals (`hash-mismatch`, `missing`, `extra-entry`), successful revision metadata, rollback restoration, and project-status owner output.
- [x] 6.2 — Added PlanProjectRevise ownership gating, revision bump/hash update, target and lock backup capture, and execution through the existing atomic temp/rename rollback executor.
- [x] 6.3 — Added project-revise and project-status CLI cores, dispatch, argument handling, and usage output.
- [x] 6.4 — Added contract section 13 covering derived `OccurrencesSincePromotion`, post-registration revision rows, and qualifying-occurrence rules.
- [x] 6.5 — Refactored registration and revision to share `prepareProjectSkill`, preserving stamp → lint → hash → write ordering.
- [x] 6.6 — Completed focused, vet, and full engine verification.

`openspec/changes/procedural-memory-lifecycle/tasks.md` visibly marks 5.1–5.5 and 6.1–6.6 as `[x]`; later tasks remain unchecked.

## TDD cycle evidence

| Cycle | Evidence |
|---|---|
| RED | Added `TestAddCoreRejectsHardLintErrorBeforeWrites` and `TestAddCoreWarningsOnlyProceeds`, then ran `cd engine && go test ./skills/...`; the run failed at the hard-lint test because `AddCore` still accepted the invalid fixture. |
| GREEN | Implemented the `LintSkillFile` gate and ran `cd engine && go test ./skills/...`; the package passed, including existing AddCore and dispatch tests after their fixtures were made lint-clean. |
| TRIANGULATE | Ran `cd engine && go vet ./... && go test ./...`; all engine packages passed. |
| REFACTOR | Corrected AddCore step comments and verified the hard refusal returns before `Serialize`, manifest read/append, and atomic writes; reran focused tests successfully. |
| RED (Phase 6) | Added revision/status tests first; `cd engine && go test ./skills/...` failed to build with undefined `ReviseInput`, `PlanProjectRevise`, `ExecuteProjectRevisePlan`, `RenderProjectReviseCore`, and `RenderProjectStatusCore`. |
| GREEN (Phase 6) | Implemented the revision planner/executor reuse, CLI cores, dispatch, and status rendering; `cd engine && go test ./skills/...` passed. |
| TRIANGULATE (Phase 6) | Ran `cd engine && go vet ./... && go test ./...`; every engine package passed. |
| REFACTOR (Phase 6) | Shared `prepareProjectSkill` between registration and revision and reran focused and full tests; revision rollback restored the pre-run tree byte-for-byte under injected mid-rename failure. |

## Files changed

- `engine/skills/lifecycle.go` — hard lint gate in `AddCore` and updated precondition/step comments.
- `engine/skills/lifecycle_test.go` — lint-clean fixture helper, hard-refusal test, warnings-only test, and existing fixture updates.
- `engine/skills/skills_test.go` — updated the existing AddCore dispatch fixture from invalid Markdown to lint-clean SKILL.md content.
- `skills/_shared/procedural-candidate-detection.md` — section 12 human promotion procedure and section 13 revision trigger/occurrence rules.
- `engine/skills/project_register.go` — shared stamp/lint/hash preparation, revision planning, ownership gate, revision backups, and executor wrapper.
- `engine/skills/project_cli.go` — project-revise and project-status CLI cores.
- `engine/skills/skills.go` — project-revise/project-status dispatch and verb enumeration.
- `engine/cmd/main.go` — project-revise/project-status usage and command documentation.
- `engine/skills/project_register_test.go` — RED/GREEN revision ownership, hash/revision, and rollback tests.
- `engine/skills/project_cli_test.go` — CLI refusal/status/dispatch tests.
- `openspec/changes/procedural-memory-lifecycle/tasks.md` — checked off 5.1–5.5 and 6.1–6.6 with evidence.
- `openspec/changes/procedural-memory-lifecycle/apply-progress.md` — this cumulative progress record.

Unrelated untracked `.agents/`, `.claude/skills/`, `.pi/`, and `skills-lock.json` were preserved and not modified.

## Verification evidence

- `cd engine && go test ./skills/...` — PASS after GREEN and REFACTOR.
- `cd engine && go vet ./... && go test ./...` — PASS; all engine packages passed.
- `git diff --check` — PASS after Phase 6 edits.
- `git status --short` — nine expected modified slice/artifact files plus the pre-existing unrelated untracked paths listed above; no commit, push, or PR was performed.

## Deviations and warnings

- Updating `engine/skills/skills_test.go` was required because its existing AddCore dispatch fixture used `# new-skill`, which is now correctly rejected by the new gate; no dispatch assertions changed.
- CodeGraph was present, but `gentle-ai codegraph explore` was unavailable in this environment and returned the init-only usage error. Targeted file reads were used after that failed CodeGraph attempt.
- No design or scope deviation was introduced. The review forecast's owner-granted `size:exception` for Phase 6 remains the recorded delivery boundary; this worker implemented only 6.1–6.6 and did not commit, push, or publish.
- `project-status` currently reports `superseded-by:-`; supersession matching and retirement remain explicitly deferred to Phase 7a.

## Structured native status

### Consumed

- Schema: `gentle-ai.sdd-status` v2.
- Change: `procedural-memory-lifecycle`.
- Native state at apply start: `applyState: ready`, `nextRecommended: apply`, `blockedReasons: []`.
- Artifact store: `openspec`; proposal, specs, design, and tasks were read from the native-resolved paths.
- Task progress at the previous Phase 5 apply start: 85/118 complete, 33 pending; Phase 6 native preflight reported 90/118 complete and 28 pending.
- Action context: `mode: repo-local`, workspace `/home/labdrian/labdrian-sdd-overlay`, allowed edit root `/home/labdrian/labdrian-sdd-overlay`.

### Produced

- Persisted OpenSpec task checkboxes: 5.1–5.5 and 6.1–6.6 are checked; task progress is now 96/118 complete and 22 pending.
- Persisted apply-progress artifact at `openspec/changes/procedural-memory-lifecycle/apply-progress.md`.
- No native lifecycle mutation, PR, commit, push, or archive action was performed. Native status consumed at apply start was `applyState: ready`, `nextRecommended: apply`, `blockedReasons: []`; after this worker the next unchecked slice begins at task 7a-i.1.

## Remaining tasks (exact unchecked lines)

```text
- [ ] 7a-i.1 RED `engine/skills/project_register_test.go` (extend): `project-retire` removes target files and the lock entry in one operation for an agent-owned skill; refused for a human-owned skill (reason named); `AbsorbedInto` write succeeds only when the named id exists (global: `MatchCandidate` against the overlay registry as an existence lookup, not coverage proof; project: an entry in the project lock) and is refused with the unverified target named otherwise; a failure injected mid-retirement (via the `projectFS` fake) triggers rollback of the partial delete, leaving the pre-retirement tree byte-identical, and a failure during that rollback itself prints `error: rollback incomplete: <rel-path>` and exits 1
- [ ] 7a-i.2 GREEN `engine/skills/project_register.go`: `project-retire` planning/execution (delete targets + lock entry, ownership-gated); `AbsorbedInto` existence verification helper
- [ ] 7a-i.3 Verify: `cd engine && go vet ./... && go test ./skills/...`
- [ ] 7a-ii.1 RED `engine/skills/project_cli_test.go` (extend): `project-retire` CLI dispatch over the core planning/execution from retirement-engine-core; `project-status` reports `superseded-by:<path>` when `MatchCandidate(registry, id)` or `MatchCandidate(registry, <last candidate slug>)` matches a global skill
- [ ] 7a-ii.2 GREEN `engine/skills/project_cli.go`: `project-retire` CLI wiring; `project-status` supersession reporting via `MatchCandidate`
- [ ] 7a-ii.3 GREEN `skills/_shared/procedural-candidate-detection.md`: section 14 — retirement decision procedure, `RetirementReason` vocabulary (`stale-reference | superseded | absorbed | promoted | quiet | human-request`), `AbsorbedInto` verification rule, global-tier `RemoveCore` path (human, unmodified)
- [ ] 7a-ii.4 REFACTOR: confirm `project-retire` never invokes removal automatically from a detector result — it is always a separate, explicitly invoked command
- [ ] 7a-ii.5 Verify: `cd engine && go vet ./... && go test ./skills/...`
- [ ] 7b.1 RED `longterm-mem/internal/staleness/staleness_test.go` (extend): `ReferencedPaths(text)` extracts repo-shaped tokens from inline code spans and fenced blocks using the existing strict `filePath` pattern; `ClassifyPaths(repoRoot, paths)` reuses `indexTree`/`repohistory.Inspect` to classify each as removed-in-history vs still-present vs moved; `Detect`'s existing behavior and tests are unchanged
- [ ] 7b.2 GREEN `longterm-mem/internal/staleness/staleness.go`: export `ReferencedPaths`, `ClassifyPaths`, sitting beside `Detect` and sharing its private helpers, with no change to `Detect` itself
- [ ] 7b.3 RED `longterm-mem/internal/skillstale/skillstale_test.go` (new), fixture `longterm-mem/internal/skillstale/testdata/procedural-skills.lock.json`: detector over a temp git repo + fixture Engram DB — a skill naming a git-removed path is flagged `REMOVED <path> (by <commit>)` regardless of ordering; a renamed path yields `MOVED <path> -> <new>` only, never `REMOVED`; a clean skill (all paths present, all commands resolvable, `LastObserved` recent, no supersession) is not flagged; an unresolvable fenced command's first word yields `UNRESOLVED-COMMAND <name>` (resolved via `os.Stat` scan of `$PATH`, no `os/exec`); `LastObserved` older than 180 days yields `QUIET since <LastObserved>`; the detector performs zero file/record mutation (tree snapshot identical before/after, Engram store opened read-only); the fixture lock parses and `engine/skills/project_lock_test.go`'s `SerializeProjectLock` reproduces the shared fixture byte-for-byte (cross-module pinning, verified in 7b.6)
- [ ] 7b.4 GREEN `longterm-mem/internal/skillstale/skillstale.go`: the detector over the lock, the first target SKILL.md of each entry, `staleness.ReferencedPaths`/`ClassifyPaths`, `os.Stat`-based `$PATH` command resolution, and `engram.Store.ListObservations` filtered by `TopicKey` for `LastObserved`/`Status` (read-only)
- [ ] 7b.5 GREEN `longterm-mem/cmd/longterm-mem/cmd_skills_stale.go`, `main.go`: `longterm-mem skills-stale --project-root <abs> [--project <P>]`, report-only subcommand
- [ ] 7b.6 GREEN `engine/skills/project_lock_test.go`: add the pinning test asserting `SerializeProjectLock` reproduces `longterm-mem/internal/skillstale/testdata/procedural-skills.lock.json` byte-for-byte
- [ ] 7b.7 REFACTOR: confirm no new `os/exec`-importing package was introduced in `longterm-mem` (the module's existing `allowedExecImporters`/equivalent guard, if any, is unchanged) and that `skillstale` and `cmd_skills_stale.go` perform no write to the lock, the SKILL.md files, or Engram
- [ ] 7b.8 Verify: `cd longterm-mem && go vet ./... && go test ./internal/staleness/... ./internal/skillstale/... ./cmd/...`
- [ ] F.1 `cd engine && go vet ./... && go test ./...`
- [ ] F.2 `cd longterm-mem && go vet ./... && go test ./...`
- [ ] F.3 Confirm no file was written under `skills/` by any project-tier code path outside `AddCore`'s own tests (`git diff --stat` review across all 13 merged PRs)
- [ ] F.4 Confirm no test in either module touched a live `.claude/skills/`, `.agents/skills/`, `.pi/`, `skills-lock.json`, the user's home directory, or spawned a live runtime binary (temp-dir-only rule)
- [ ] F.5 Run the full acceptance checklist from sections 11 (Phase 4), and the sdd-verify checklists implied by Phases 2, 6, 7a-i, 7a-ii against real Engram records and a temp git repo; record results honestly, including any scenario that could not be exercised (e.g. the rollback-failure print path, or the Codex smoke test if `UNVERIFIED`)
- [ ] F.6 Confirm every Success Criteria checkbox in `proposal.md` is satisfied or explicitly recorded as a known gap (in particular the Codex-support claim, gated strictly by the recorded smoke-test verdict)
```

## Phase 7a-i: `retirement-engine-core`

- Change: `procedural-memory-lifecycle`
- Assigned tasks: 7a-i.1 through 7a-i.3 only
- Branch: `feat/procedural-memory-lifecycle-7a-i-retirement-engine-core`
- Artifact store: OpenSpec
- Previous progress: Phase 6 `revision` entry above; this entry is cumulative and preserves all prior evidence.

## Completed tasks and persisted checkboxes

- [x] 7a-i.1 — Added RED/GREEN coverage in `engine/skills/project_register_test.go` for agent-owned retirement, human-owned refusal, global and project `AbsorbedInto` existence checks, unverified target naming, partial-delete rollback, and rollback-of-rollback diagnostics.
- [x] 7a-i.2 — Added `RetireInput`, `VerifyAbsorbedInto`, `PlanProjectRetire`, and `ExecuteProjectRetirePlan` in `engine/skills/project_register.go`. Retirement proves ownership from the project lock, stages the updated lock, deletes target files, commits the lock last, and restores captured target/lock bytes on failure. `ExecuteProjectPlan` dispatches retirement plans through the delete-aware executor for compatibility with the existing executor seam.
- [x] 7a-i.3 — Ran the required vet and focused skills verification successfully.

`openspec/changes/procedural-memory-lifecycle/tasks.md` visibly marks 7a-i.1–7a-i.3 as `[x]`; 7a-ii and 7b remain unchecked.

## TDD cycle evidence

| Cycle | Evidence |
|---|---|
| RED | Added retirement tests before production code, then ran `cd engine && go test ./skills/...`; compilation failed with undefined `RetireInput`, `PlanProjectRetire`, and `ExecuteProjectRetirePlan`, proving the new tests were initially red. |
| GREEN | Implemented ownership-gated planning, existence-only `AbsorbedInto` verification through `MatchCandidate` or an exact lock entry, staged-lock/delete execution, and rollback; `cd engine && go test ./skills/...` passed. |
| TRIANGULATE | Ran `cd engine && go test ./...`; all engine packages passed. The required `cd engine && go vet ./... && go test ./skills/...` also passed. |
| REFACTOR | Kept retirement behind the existing `projectFS` and plan/executor seam, added generic `ExecuteProjectPlan` dispatch for retirement plans, ran `gofmt`, reran focused tests, and ran `git diff --check`. |

## Files changed in this phase

- `engine/skills/project_register.go` — retirement input, `AbsorbedInto` existence helper, planner, delete/lock executor, rollback state, and executor dispatch.
- `engine/skills/project_register_test.go` — retirement RED/GREEN behavior, ownership, consolidation target, rollback, and rollback-failure tests.
- `openspec/changes/procedural-memory-lifecycle/tasks.md` — checked only 7a-i.1–7a-i.3.
- `openspec/changes/procedural-memory-lifecycle/apply-progress.md` — appended this cumulative phase entry.

Unrelated untracked `.agents/`, `.claude/skills/`, `.pi/`, and `skills-lock.json` were preserved and not modified.

## Verification evidence

- `cd engine && go test ./skills/...` — PASS after GREEN and REFACTOR.
- `cd engine && go vet ./... && go test ./skills/...` — PASS.
- `cd engine && go test ./...` — PASS; all engine packages passed.
- `git diff --check` — PASS.
- `git status --short` — only the two phase source/test files and the two OpenSpec artifact files are modified; unrelated untracked paths remain unchanged.

## Deviations, workload, and risks

- CodeGraph was present, but `gentle-ai codegraph explore` was unavailable and returned the init-only usage error; targeted artifact and source reads were used after that failed attempt.
- The authored phase diff is 590 insertions plus 10 deletions (600 additions-plus-deletions), above the 400-line review budget. The tests and rollback implementation form one cohesive retirement-engine work unit; no code, comments, or tests were compressed or omitted. This remains the assigned PR 11 boundary on the feature-branch chain and should receive a `size:exception` recommendation if the maintainer requires an explicit budget decision.
- No CLI wiring, project-status supersession reporting, contract section 14, detector work, commit, push, or PR was performed; those remain later tasks.
- Retirement leaves the now-empty per-skill directories in place and removes the target files plus the lock entry, matching the task's file-level removal contract; the lock file itself remains with an empty `skills` list for symmetric git revert.

## Structured native status

### Consumed

- Schema: `gentle-ai.sdd-status` v2.
- Change: `procedural-memory-lifecycle`.
- Native state at apply start: `applyState: ready`, `nextRecommended: apply`, `blockedReasons: []`.
- Artifact store: `openspec`; proposal, six specs, design, tasks, and prior apply-progress were read from the native-resolved paths.
- Action context: `mode: repo-local`, workspace `/home/labdrian/labdrian-sdd-overlay`, allowed edit root `/home/labdrian/labdrian-sdd-overlay`.
- Review context: assigned slice 7a-i on the feature-branch chain; no source path was outside the authoritative workspace.

### Produced

- Persisted OpenSpec task checkboxes: 7a-i.1–7a-i.3 are checked; task progress is now 99/118 complete and 19 pending.
- Persisted cumulative apply-progress at `openspec/changes/procedural-memory-lifecycle/apply-progress.md`.
- No native lifecycle mutation, commit, push, PR, or archive action was performed. The next unchecked slice begins at 7a-ii.1.

## Remaining tasks (exact unchecked lines after this phase)

```text
- [ ] 7a-ii.1 RED `engine/skills/project_cli_test.go` (extend): `project-retire` CLI dispatch over the core planning/execution from retirement-engine-core; `project-status` reports `superseded-by:<path>` when `MatchCandidate(registry, id)` or `MatchCandidate(registry, <last candidate slug>)` matches a global skill
- [ ] 7a-ii.2 GREEN `engine/skills/project_cli.go`: `project-retire` CLI wiring; `project-status` supersession reporting via `MatchCandidate`
- [ ] 7a-ii.3 GREEN `skills/_shared/procedural-candidate-detection.md`: section 14 — retirement decision procedure, `RetirementReason` vocabulary (`stale-reference | superseded | absorbed | promoted | quiet | human-request`), `AbsorbedInto` verification rule, global-tier `RemoveCore` path (human, unmodified)
- [ ] 7a-ii.4 REFACTOR: confirm `project-retire` never invokes removal automatically from a detector result — it is always a separate, explicitly invoked command
- [ ] 7a-ii.5 Verify: `cd engine && go vet ./... && go test ./skills/...`
- [ ] 7b.1 RED `longterm-mem/internal/staleness/staleness_test.go` (extend): `ReferencedPaths(text)` extracts repo-shaped tokens from inline code spans and fenced blocks using the existing strict `filePath` pattern; `ClassifyPaths(repoRoot, paths)` reuses `indexTree`/`repohistory.Inspect` to classify each as removed-in-history vs still-present vs moved; `Detect`'s existing behavior and tests are unchanged
- [ ] 7b.2 GREEN `longterm-mem/internal/staleness/staleness.go`: export `ReferencedPaths`, `ClassifyPaths`, sitting beside `Detect` and sharing its private helpers, with no change to `Detect` itself
- [ ] 7b.3 RED `longterm-mem/internal/skillstale/skillstale_test.go` (new), fixture `longterm-mem/internal/skillstale/testdata/procedural-skills.lock.json`: detector over a temp git repo + fixture Engram DB — a skill naming a git-removed path is flagged `REMOVED <path> (by <commit>)` regardless of ordering; a renamed path yields `MOVED <path> -> <new>` only, never `REMOVED`; a clean skill (all paths present, all commands resolvable, `LastObserved` recent, no supersession) is not flagged; an unresolvable fenced command's first word yields `UNRESOLVED-COMMAND <name>` (resolved via `os.Stat` scan of `$PATH`, no `os/exec`); `LastObserved` older than 180 days yields `QUIET since <LastObserved>`; the detector performs zero file/record mutation (tree snapshot identical before/after, Engram store opened read-only); the fixture lock parses and `engine/skills/project_lock_test.go`'s `SerializeProjectLock` reproduces the shared fixture byte-for-byte (cross-module pinning, verified in 7b.6)
- [ ] 7b.4 GREEN `longterm-mem/internal/skillstale/skillstale.go`: the detector over the lock, the first target SKILL.md of each entry, `staleness.ReferencedPaths`/`ClassifyPaths`, `os.Stat`-based `$PATH` command resolution, and `engram.Store.ListObservations` filtered by `TopicKey` for `LastObserved`/`Status` (read-only)
- [ ] 7b.5 GREEN `longterm-mem/cmd/longterm-mem/cmd_skills_stale.go`, `main.go`: `longterm-mem skills-stale --project-root <abs> [--project <P>]`, report-only subcommand
- [ ] 7b.6 GREEN `engine/skills/project_lock_test.go`: add the pinning test asserting `SerializeProjectLock` reproduces `longterm-mem/internal/skillstale/testdata/procedural-skills.lock.json` byte-for-byte
- [ ] 7b.7 REFACTOR: confirm no new `os/exec`-importing package was introduced in `longterm-mem` (the module's existing `allowedExecImporters`/equivalent guard, if any, is unchanged) and that `skillstale` and `cmd_skills_stale.go` perform no write to the lock, the SKILL.md files, or Engram
- [ ] 7b.8 Verify: `cd longterm-mem && go vet ./... && go test ./internal/staleness/... ./internal/skillstale/... ./cmd/...`
- [ ] F.1 `cd engine && go vet ./... && go test ./...`
- [ ] F.2 `cd longterm-mem && go vet ./... && go test ./...`
- [ ] F.3 Confirm no file was written under `skills/` by any project-tier code path outside `AddCore`'s own tests (`git diff --stat` review across all 13 merged PRs)
- [ ] F.4 Confirm no test in either module touched a live `.claude/skills/`, `.agents/skills/`, `.pi/`, `skills-lock.json`, the user's home directory, or spawned a live runtime binary (temp-dir-only rule)
- [ ] F.5 Run the full acceptance checklist from sections 11 (Phase 4), and the sdd-verify checklists implied by Phases 2, 6, 7a-i, 7a-ii against real Engram records and a temp git repo; record results honestly, including any scenario that could not be exercised (e.g. the rollback-failure print path, or the Codex smoke test if `UNVERIFIED`)
- [ ] F.6 Confirm every Success Criteria checkbox in `proposal.md` is satisfied or explicitly recorded as a known gap (in particular the Codex-support claim, gated strictly by the recorded smoke-test verdict)
```


---

## Phase 7a-ii: `retirement-cli-and-docs`

- Change: `procedural-memory-lifecycle`
- Assigned tasks: 7a-ii.1 through 7a-ii.5 only
- Branch: `feat/procedural-memory-lifecycle-7a-ii-retirement-cli-docs`
- Artifact store: OpenSpec
- Previous progress: Phase 7a-i `retirement-engine-core`; this entry is cumulative and preserves all prior evidence.

## Completed tasks and persisted checkboxes

- [x] 7a-ii.1 — Added CLI RED coverage for explicit `project-retire` dispatch and execution, plus `project-status` supersession reporting through both the project id and the final candidate slug.
- [x] 7a-ii.2 — Added `RenderProjectRetireCore`, `project-retire` dispatch/usage wiring, dry-run plan output, ownership-gated execution and exit-1 failure handling; extended `project-status` to parse the overlay registry and print `superseded-by:<path>`.
- [x] 7a-ii.3 — Added contract section 14 documenting report-then-decision retirement, the six `RetirementReason` values, existence-only `AbsorbedInto` verification, project-tier cleanup, and the human/unmodified global `RemoveCore` path; pinned the section with contract assertions.
- [x] 7a-ii.4 — Confirmed retirement has no detector import or automatic trigger: only the explicit `project-retire` verb reaches the retirement planner/executor, while section 14 makes detector output report-only.
- [x] 7a-ii.5 — Completed focused, vet, full-engine and diff-integrity verification.

`openspec/changes/procedural-memory-lifecycle/tasks.md` visibly marks 7a-ii.1–7a-ii.5 as `[x]`; Phase 7b and final verification tasks remain unchecked.

## TDD cycle evidence

| Task | Test file | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| 7a-ii.1 | `engine/skills/project_cli_test.go` | Unit/FS CLI | `cd engine && go test ./skills/...` passed before edits | Added dispatch, id-match, candidate-slug-match and rollback-exit tests; the first focused run failed to compile with undefined `RenderProjectRetireCore` | Implemented the CLI core and dispatch; focused tests passed | Ran two supersession match cases plus successful and rollback-incomplete retirement paths | Ran gofmt, full focused package tests, and diff checks with no behavior change |
| 7a-ii.2 | `engine/skills/project_cli.go`, `engine/skills/skills.go`, `engine/cmd/main.go` | Unit/CLI | Existing skills package passed before edits | Covered by the same RED CLI tests | `cd engine && go test ./skills/...` passed | Exercised explicit dispatch, actual target/lock removal, and nonzero rollback failure | Consolidated supersession formatting in `projectStatusSupersededBy`; retained injected I/O and executor seams |
| 7a-ii.3 | `skills/_shared/procedural-candidate-detection.md`, `engine/skills/procedural_candidate_contract_test.go` | Contract/document | Existing contract tests passed before edits | Added section-14 assertions; the first focused assertion run failed on literal matching across wrapped prose | Normalized contract whitespace and pinned the complete ordered section; focused contract test passed | Checked both project and global retirement branches, all six reason values, and the detector boundary | Ran gofmt and the full skills package after the contract assertion refactor |
| 7a-ii.4 | `engine/skills/project_cli.go`, contract section 14 | Structural | Existing source behavior passed before edits | N/A — refactor/inspection task | Explicit-only command path and report-only documentation verified | Covered successful explicit invocation and detector-independent code path | No additional production behavior required; existing tests remained green |
| 7a-ii.5 | `engine/skills/...` | Verification | Focused baseline passed | N/A — verification task | Vet and tests passed | Full engine suite passed | `git diff --check` passed |

## Files changed in this phase

- `engine/skills/project_cli.go` — explicit retirement CLI parser/core, dry-run ordering, executor failure mapping, registry-backed supersession output.
- `engine/skills/project_cli_test.go` — retirement dispatch/removal, supersession-by-id/slug, and rollback-incomplete exit tests.
- `engine/skills/procedural_candidate_contract_test.go` — section 14 heading, ordering, reason vocabulary, and ownership-boundary assertions.
- `engine/skills/skills.go` — project-retire dispatch and supported-verb enumeration.
- `engine/cmd/main.go` — project-retire usage/help and supported-verb text.
- `skills/_shared/procedural-candidate-detection.md` — section 14 retirement procedure and top-level slice index update.
- `openspec/changes/procedural-memory-lifecycle/tasks.md` — checked only 7a-ii.1–7a-ii.5.
- `openspec/changes/procedural-memory-lifecycle/apply-progress.md` — appended this cumulative phase entry.

Unrelated untracked `.agents/`, `.claude/skills/`, `.pi/`, and `skills-lock.json` were preserved and not modified.

## Verification evidence

- `cd engine && go test ./skills -run 'Test(SkillsCore_DispatchesProjectRetireAndRemovesRegisteredSkill|RenderProjectStatusCoreReportsSupersededByIDOrCandidateSlug|RenderProjectRetireCoreMapsRollbackIncompleteToExitOne)$'` — PASS after GREEN.
- `cd engine && go test ./skills -run 'Project(Register|Revise|Status|Retire)|SkillsCore|RenderProject'` — PASS.
- `cd engine && go test ./skills -run TestProceduralCandidateContractArtifact` — PASS after the contract assertion refactor.
- `cd engine && go test ./skills/...` — PASS.
- `cd engine && go vet ./...` — PASS.
- `cd engine && go test ./...` — PASS; all engine packages passed.
- `git diff --check` — PASS.
- `git status --short` — eight expected modified files plus pre-existing unrelated untracked paths; no commit, push, or PR was performed.

## Deviations, workload, and risks

- The authored Phase 7a-ii implementation/documentation diff is 519 additions plus 17 deletions (536 authored changed lines, excluding OpenSpec bookkeeping) across the CLI, tests, help text, contract assertions, and section 14. It is above the nominal 400-line review budget but remains the owner-decided cohesive PR 12 slice on the `feature-branch-chain`; no comments, tests, docs, or safety handling were compressed to reduce the count. A `size:exception` should be recorded if the maintainer requires an explicit budget acknowledgement.
- `project-retire` accepts optional `--reason` and `--absorbed-into` metadata flags in addition to the design's short command form; the engine still performs existence verification only for `AbsorbedInto`, and candidate-record persistence remains agent-driven as specified.
- No Phase 7b detector, commit, push, PR, archive, or native lifecycle mutation was performed.

## Structured native status

### Consumed

- Schema: `gentle-ai.sdd-status` v2.
- Change: `procedural-memory-lifecycle`.
- Native state at apply start: `applyState: ready`, `nextRecommended: apply`, `blockedReasons: []`.
- Artifact store: `openspec`; proposal, all six specs, design, tasks, and cumulative apply-progress were read from native-resolved paths.
- Action context: `mode: repo-local`, workspace `/home/labdrian/labdrian-sdd-overlay`, allowed edit root `/home/labdrian/labdrian-sdd-overlay`.
- Review workload: `auto-chain` with `feature-branch-chain`; Phase 7a-ii was the assigned slice, so no new delivery decision was needed.

### Produced

- Persisted OpenSpec task checkboxes: 7a-ii.1–7a-ii.5 are checked; task progress is now 104/118 complete and 14 pending.
- Persisted cumulative apply-progress at `openspec/changes/procedural-memory-lifecycle/apply-progress.md`.
- No native lifecycle mutation, commit, push, PR, or archive action was performed. The next unchecked slice begins at 7b.1.

## Remaining tasks (exact unchecked lines after this phase)

```text
- [ ] 7b.1 RED `longterm-mem/internal/staleness/staleness_test.go` (extend): `ReferencedPaths(text)` extracts repo-shaped tokens from inline code spans and fenced blocks using the existing strict `filePath` pattern; `ClassifyPaths(repoRoot, paths)` reuses `indexTree`/`repohistory.Inspect` to classify each as removed-in-history vs still-present vs moved; `Detect`'s existing behavior and tests are unchanged
- [ ] 7b.2 GREEN `longterm-mem/internal/staleness/staleness.go`: export `ReferencedPaths`, `ClassifyPaths`, sitting beside `Detect` and sharing its private helpers, with no change to `Detect` itself
- [ ] 7b.3 RED `longterm-mem/internal/skillstale/skillstale_test.go` (new), fixture `longterm-mem/internal/skillstale/testdata/procedural-skills.lock.json`: detector over a temp git repo + fixture Engram DB — a skill naming a git-removed path is flagged `REMOVED <path> (by <commit>)` regardless of ordering; a renamed path yields `MOVED <path> -> <new>` only, never `REMOVED`; a clean skill (all paths present, all commands resolvable, `LastObserved` recent, no supersession) is not flagged; an unresolvable fenced command's first word yields `UNRESOLVED-COMMAND <name>` (resolved via `os.Stat` scan of `$PATH`, no `os/exec`); `LastObserved` older than 180 days yields `QUIET since <LastObserved>`; the detector performs zero file/record mutation (tree snapshot identical before/after, Engram store opened read-only); the fixture lock parses and `engine/skills/project_lock_test.go`'s `SerializeProjectLock` reproduces the shared fixture byte-for-byte (cross-module pinning, verified in 7b.6)
- [ ] 7b.4 GREEN `longterm-mem/internal/skillstale/skillstale.go`: the detector over the lock, the first target SKILL.md of each entry, `staleness.ReferencedPaths`/`ClassifyPaths`, `os.Stat`-based `$PATH` command resolution, and `engram.Store.ListObservations` filtered by `TopicKey` for `LastObserved`/`Status` (read-only)
- [ ] 7b.5 GREEN `longterm-mem/cmd/longterm-mem/cmd_skills_stale.go`, `main.go`: `longterm-mem skills-stale --project-root <abs> [--project <P>]`, report-only subcommand
- [ ] 7b.6 GREEN `engine/skills/project_lock_test.go`: add the pinning test asserting `SerializeProjectLock` reproduces `longterm-mem/internal/skillstale/testdata/procedural-skills.lock.json` byte-for-byte
- [ ] 7b.7 REFACTOR: confirm no new `os/exec`-importing package was introduced in `longterm-mem` (the module's existing `allowedExecImporters`/equivalent guard, if any, is unchanged) and that `skillstale` and `cmd_skills_stale.go` perform no write to the lock, the SKILL.md files, or Engram
- [ ] 7b.8 Verify: `cd longterm-mem && go vet ./... && go test ./internal/staleness/... ./internal/skillstale/... ./cmd/...`
- [ ] F.1 `cd engine && go vet ./... && go test ./...`
- [ ] F.2 `cd longterm-mem && go vet ./... && go test ./...`
- [ ] F.3 Confirm no file was written under `skills/` by any project-tier code path outside `AddCore`'s own tests (`git diff --stat` review across all 13 merged PRs)
- [ ] F.4 Confirm no test in either module touched a live `.claude/skills/`, `.agents/skills/`, `.pi/`, `skills-lock.json`, the user's home directory, or spawned a live runtime binary (temp-dir-only rule)
- [ ] F.5 Run the full acceptance checklist from sections 11 (Phase 4), and the sdd-verify checklists implied by Phases 2, 6, 7a-i, 7a-ii against real Engram records and a temp git repo; record results honestly, including any scenario that could not be exercised (e.g. the rollback-failure print path, or the Codex smoke test if `UNVERIFIED`)
- [ ] F.6 Confirm every Success Criteria checkbox in `proposal.md` is satisfied or explicitly recorded as a known gap (in particular the Codex-support claim, gated strictly by the recorded smoke-test verdict)
```

## Phase 7b: `retirement-detector`

- Change: `procedural-memory-lifecycle`
- Phase: 7b — `retirement-detector`
- Assigned tasks: 7b.1 through 7b.8 only
- Branch: `feat/procedural-memory-lifecycle-7b-retirement-detector`
- Artifact store: OpenSpec
- Previous apply-progress: Phase 7a-ii `retirement-cli-and-docs`; this entry is cumulative and preserves all prior evidence.

## Completed tasks and persisted checkboxes

- [x] 7b.1 — Added RED coverage for code-span and fenced-block path extraction plus RED/GREEN coverage for present, deleted, renamed, and unknown path classification; existing `Detect` tests remain unchanged.
- [x] 7b.2 — Exported `staleness.ReferencedPaths` and `staleness.ClassifyPaths`, sharing the strict path matcher, tree index, and `repohistory.Inspect` without changing `Detect`.
- [x] 7b.3 — Added a temporary-git-repository and fixture-Engram test matrix for removed, moved, clean, unresolved-command, quiet, ordering-independent removal, lock parsing, and zero-mutation behavior.
- [x] 7b.4 — Implemented the read-only detector over the project lock, first target `SKILL.md`, staleness classifications, PATH `os.Stat` command scans, and TopicKey-filtered candidate records.
- [x] 7b.5 — Added `longterm-mem skills-stale --project-root <abs> [--project <P>]` and main dispatch; findings are report-only and exit successfully.
- [x] 7b.6 — Added the engine cross-module byte-for-byte pinning test against the shared lock fixture.
- [x] 7b.7 — Confirmed the production additions do not import `os/exec` and perform no writes to project skill files, the lock, or Engram; the existing two-entry allowlist remains unchanged.
- [x] 7b.8 — Ran the required focused vet/test verification successfully.

`openspec/changes/procedural-memory-lifecycle/tasks.md` visibly marks 7b.1–7b.8 as `[x]`; final verification tasks F.1–F.6 remain unchecked.

## TDD cycle evidence

| Cycle | Evidence |
|---|---|
| RED (7b.1) | Added `ReferencedPaths`/`ClassifyPaths` tests first; `cd longterm-mem && go test ./internal/staleness/...` failed to compile with undefined exported functions. |
| GREEN (7b.1–7b.2) | Implemented the two staleness exports; the focused staleness suite passed. |
| RED (7b.3) | Added the new detector tests and fixture before production code; `cd longterm-mem && go test ./internal/skillstale/...` failed because the package had no non-test Go files. |
| GREEN (7b.3–7b.4) | Implemented lock parsing, detector signals, read-only Engram lookup, path safety, and PATH scanning; the skillstale package passed. |
| RED (7b.5) | Added dispatch and absolute-root tests; before wiring, the focused command tests returned the unknown-subcommand usage path and failed their expected dispatch/error assertions. |
| GREEN (7b.5) | Added `cmd_skills_stale.go` and main dispatch; focused command tests passed. |
| TRIANGULATE | Ran `cd longterm-mem && go test ./internal/staleness/... ./internal/skillstale/... ./cmd/...`, `cd longterm-mem && go vet ./...`, and `cd engine && go vet ./...`; all passed. |
| REFACTOR | Ran gofmt, full module tests, engine focused tests, `git diff --check`, and reviewed the unchanged `Detect` path, read-only calls, and import allowlist. |

## Files changed in this phase

- `longterm-mem/internal/staleness/staleness.go` — exported code-span path extraction and tree/history classification.
- `longterm-mem/internal/staleness/staleness_test.go` — RED/GREEN path extraction and classification coverage.
- `longterm-mem/internal/skillstale/skillstale.go` — report-only lock, skill, command, quietness, and candidate detector.
- `longterm-mem/internal/skillstale/skillstale_test.go` — temporary git/Engram fixture matrix and mutation checks.
- `longterm-mem/internal/skillstale/testdata/procedural-skills.lock.json` — shared lock fixture.
- `longterm-mem/cmd/longterm-mem/cmd_skills_stale.go` — report-only CLI implementation.
- `longterm-mem/cmd/longterm-mem/cmd_skills_stale_test.go` — dispatch and absolute-root tests.
- `longterm-mem/cmd/longterm-mem/main.go` — `skills-stale` dispatch.
- `engine/skills/project_lock_test.go` — shared fixture serialization pin.
- `openspec/changes/procedural-memory-lifecycle/tasks.md` — checked only 7b.1–7b.8.
- `openspec/changes/procedural-memory-lifecycle/apply-progress.md` — appended this cumulative phase entry.

Unrelated untracked `.agents/`, `.claude/skills/`, `.pi/`, and `skills-lock.json` were preserved and not modified.

## Verification evidence

- `cd longterm-mem && go test ./internal/staleness/... ./internal/skillstale/... ./cmd/...` — PASS.
- `cd longterm-mem && go vet ./...` — PASS.
- `cd longterm-mem && go test ./...` — PASS.
- `cd engine && go vet ./...` — PASS.
- `cd engine && go test ./skills/... ./cmd/...` — PASS.
- `cd engine && go test ./...` — PASS.
- `cd engine && go test ./skills -run TestSerializeProjectLock_MatchesSharedSkillstaleFixture` — PASS.
- `git diff --check` — PASS.
- `git status --short` — expected Phase 7b source/test/artifact changes plus preserved unrelated untracked paths; no commit, push, or PR was performed.

## Deviations, workload, and risks

- CodeGraph was present, but `gentle-ai codegraph explore` was unavailable and returned the init-only usage error; targeted reads were used after that failed CodeGraph attempt.
- The Phase 7b implementation and test/fixture matrix is the owner-granted `size:exception` PR 13 boundary on the feature-branch chain; no comments or tests were compressed to reduce the diff.
- Supersession evidence is accepted through the detector `Config.SupersededBy` read-only input; the CLI has no registry argument and therefore does not invent a global registry lookup for arbitrary consumer roots. Existing engine `project-status` remains the registry-backed supersession surface.
- Final F.1/F.2 checkboxes remain intentionally unchecked because this worker owns only 7b; their equivalent vet/full-test commands were run as triangulation. F.3–F.6, commit, push, PR, native lifecycle mutation, and archive action were not performed.

## Structured native status

### Consumed

- Schema: `gentle-ai.sdd-status` v2.
- Change: `procedural-memory-lifecycle`.
- Native state at apply start: `applyState: ready`, `nextRecommended: apply`, `blockedReasons: []`.
- Artifact store: `openspec`; proposal, all six specs, design, tasks, and cumulative apply-progress were read from native-resolved paths.
- Action context: `mode: repo-local`, workspace `/home/labdrian/labdrian-sdd-overlay`, allowed edit root `/home/labdrian/labdrian-sdd-overlay`.
- Workload context: 7b has an owner-granted `size:exception` and remains the assigned PR 13 boundary; no new delivery decision was required.

### Produced

- Persisted OpenSpec task checkboxes: 7b.1–7b.8 are checked; task progress is now 112/118 complete and 6 pending.
- Persisted cumulative apply-progress at `openspec/changes/procedural-memory-lifecycle/apply-progress.md`.
- No native lifecycle mutation, commit, push, PR, or archive action was performed. The next native implementation work is final verification / later lifecycle handling.

## Remaining tasks (exact unchecked lines)

```text
- [ ] F.1 `cd engine && go vet ./... && go test ./...`
- [ ] F.2 `cd longterm-mem && go vet ./... && go test ./...`
- [ ] F.3 Confirm no file was written under `skills/` by any project-tier code path outside `AddCore`'s own tests (`git diff --stat` review across all 13 merged PRs)
- [ ] F.4 Confirm no test in either module touched a live `.claude/skills/`, `.agents/skills/`, `.pi/`, `skills-lock.json`, the user's home directory, or spawned a live runtime binary (temp-dir-only rule)
- [ ] F.5 Run the full acceptance checklist from sections 11 (Phase 4), and the sdd-verify checklists implied by Phases 2, 6, 7a-i, 7a-ii against real Engram records and a temp git repo; record results honestly, including any scenario that could not be exercised (e.g. the rollback-failure print path, or the Codex smoke test if `UNVERIFIED`)
- [ ] F.6 Confirm every Success Criteria checkbox in `proposal.md` is satisfied or explicitly recorded as a known gap (in particular the Codex-support claim, gated strictly by the recorded smoke-test verdict)
```
