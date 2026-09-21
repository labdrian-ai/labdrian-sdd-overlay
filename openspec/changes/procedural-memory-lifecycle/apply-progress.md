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
