# Apply Progress: procedural-memory-lifecycle

## Slice

- Change: `procedural-memory-lifecycle`
- Phase: 5 — `global-promotion`
- Assigned tasks: 5.1 through 5.5 only
- Branch: `feat/procedural-memory-lifecycle-5-global-promotion`
- Artifact store: OpenSpec
- Previous apply-progress: none; this is the initial apply-progress artifact.

## Completed tasks and persisted checkboxes

- [x] 5.1 — Added RED/GREEN `AddCore` tests for hard lint refusal with byte-preserved registry and manifest, warnings-only success, and lint-clean existing fixtures. The RED run failed only because the lint gate was absent; the GREEN run passed.
- [x] 5.2 — Added the `LintSkillFile` gate in `AddCore` after SKILL.md existence validation and before serialization or manifest processing. Hard findings are printed and exit 1; warnings do not block.
- [x] 5.3 — Added contract section 12 documenting human-only promotion through the existing `engine skills add`/`AddCore` path, silence-is-not-consent, the `Promoted` hash line, and append-only `History` wiring.
- [x] 5.4 — Confirmed by the hard-lint regression that refusal returns before registry serialization, manifest processing, or either atomic write.
- [x] 5.5 — Completed focused and broad engine verification.

`openspec/changes/procedural-memory-lifecycle/tasks.md` visibly marks 5.1–5.5 as `[x]`; later tasks remain unchecked.

## TDD cycle evidence

| Cycle | Evidence |
|---|---|
| RED | Added `TestAddCoreRejectsHardLintErrorBeforeWrites` and `TestAddCoreWarningsOnlyProceeds`, then ran `cd engine && go test ./skills/...`; the run failed at the hard-lint test because `AddCore` still accepted the invalid fixture. |
| GREEN | Implemented the `LintSkillFile` gate and ran `cd engine && go test ./skills/...`; the package passed, including existing AddCore and dispatch tests after their fixtures were made lint-clean. |
| TRIANGULATE | Ran `cd engine && go vet ./... && go test ./...`; all engine packages passed. |
| REFACTOR | Corrected AddCore step comments and verified the hard refusal returns before `Serialize`, manifest read/append, and atomic writes; reran focused tests successfully. |

## Files changed

- `engine/skills/lifecycle.go` — hard lint gate in `AddCore` and updated precondition/step comments.
- `engine/skills/lifecycle_test.go` — lint-clean fixture helper, hard-refusal test, warnings-only test, and existing fixture updates.
- `engine/skills/skills_test.go` — updated the existing AddCore dispatch fixture from invalid Markdown to lint-clean SKILL.md content.
- `skills/_shared/procedural-candidate-detection.md` — section 12 human promotion procedure.
- `openspec/changes/procedural-memory-lifecycle/tasks.md` — checked off 5.1–5.5 with evidence.
- `openspec/changes/procedural-memory-lifecycle/apply-progress.md` — this cumulative progress record.

Unrelated untracked `.agents/`, `.claude/skills/`, `.pi/`, and `skills-lock.json` were preserved and not modified.

## Verification evidence

- `cd engine && go test ./skills/...` — PASS.
- `cd engine && go vet ./... && go test ./...` — PASS; all engine packages passed.
- `git diff --check` — PASS.
- `git status --short` — expected five modified slice files plus the pre-existing unrelated untracked paths listed above; no commit, push, or PR was performed.

## Deviations and warnings

- Updating `engine/skills/skills_test.go` was required because its existing AddCore dispatch fixture used `# new-skill`, which is now correctly rejected by the new gate; no dispatch assertions changed.
- CodeGraph was present, but `gentle-ai codegraph explore` was unavailable in this environment and returned the init-only usage error. Targeted file reads were used after that failed CodeGraph attempt.
- No design or scope deviation was introduced. The review forecast's owner-granted `size:exception` for Phase 5 remains the recorded delivery boundary; this worker implemented only 5.1–5.5 and did not commit or publish.

## Structured native status

### Consumed

- Schema: `gentle-ai.sdd-status` v2.
- Change: `procedural-memory-lifecycle`.
- Native state at apply start: `applyState: ready`, `nextRecommended: apply`, `blockedReasons: []`.
- Artifact store: `openspec`; proposal, specs, design, and tasks were read from the native-resolved paths.
- Task progress at apply start: 85/118 complete, 33 pending.
- Action context: `mode: repo-local`, workspace `/home/labdrian/labdrian-sdd-overlay`, allowed edit root `/home/labdrian/labdrian-sdd-overlay`.

### Produced

- Persisted OpenSpec task checkboxes: 5.1–5.5 are checked; task progress is now 90/118 complete and 28 pending.
- Persisted apply-progress artifact at `openspec/changes/procedural-memory-lifecycle/apply-progress.md`.
- No native lifecycle mutation, PR, commit, push, or archive action was performed. Fresh native status after apply reports `applyState: ready`, `nextRecommended: apply`, `blockedReasons: []`, and 90/118 tasks complete; the next unchecked slice begins at task 6.1.

## Remaining tasks (exact unchecked lines)

```text
- [ ] 6.1 RED `engine/skills/project_register_test.go`, `project_cli_test.go` (extend): `project-revise` refused per each `EvaluateOwnership` reason (`hash-mismatch`, `missing`, `extra-entry`) with the reason named in output and nothing written; a hash-matching revision bumps `revision`, recomputes `sha256`, and restores backup bytes on an injected mid-revision failure (rollback); `project-status` reports `owner:agent|human (<reason>)`
- [ ] 6.2 GREEN `engine/skills/project_register.go`: `project-revise` planning/execution path — ownership gate via `EvaluateOwnership`, revision-number bump, backup-capture-then-restore-on-failure (temp+rename pattern, same as new registration)
- [ ] 6.3 GREEN `engine/skills/project_cli.go`: `project-status` (ownership reporting) and `project-revise` CLI wiring
- [ ] 6.4 GREEN `skills/_shared/procedural-candidate-detection.md`: section 13 — revision trigger (`OccurrencesSincePromotion >= 2`, derived not stored), new emission-table rows for post-registration occurrences, the "qualifying occurrence" rule (failure-recovery recurrence, or repeated-success where the instruction was missing/wrong/rediscovered — simply following the skill does not count)
- [ ] 6.5 REFACTOR: confirm `project-revise` reuses the exact same stamp→lint→hash→write ordering as new registration (no divergent code path)
- [ ] 6.6 Verify: `cd engine && go vet ./... && go test ./skills/...`
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
