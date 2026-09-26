# ODD Task — Goal v2 identity

## Objective and problem
Add an explicit `goal_id` to version 2 of the runtime-neutral Goal contract so two concurrent Goals in the same project can be distinguished by `(project_id, goal_id)`. Goal v1 carries only `project_id`; the Phase 3 Shaper readiness positive path cannot identify a specific concurrent Goal using v1 alone.

## Authorized scope and limits
- Isolated worktree `/home/labdrian/labdrian-sdd-overlay-goal-v2`, branch `feat/goal-v2-identity`, starting from committed Goal v1 `574ef923922313eebbdf0b7b96faae4c59cb8f65`.
- Source/test scope: `engine/goal/**`; preserve the immutable v1 JSON fixture and existing v1 behavior. Tracker: this document and Engram mirror.
- No Shaper implementation, worktree lifecycle/identity resolution, persistent Goal registry, RDD semantics, memory integration, Phase 4/5 roles, runtime authority, source migration, remote action or PR in this unit. The user subsequently authorized a local Goal v2 commit.
- The original dirty worktree and its untracked Goal copy must remain untouched. A Goal ID is not a worktree ID, a revision/currentness proof, or a closure certificate.

## Requirements and acceptance
- [x] Goal v2 accepts a caller-supplied, nonblank `goal_id` as a distinct Goal identity within its `project_id`. It does not synthesize an ID or infer one from path/branch.
- [x] Version 1 parsing, validation, canonical marshaling and golden fixture remain compatible, with no new field present in v1 output and no acceptance of a v2 field in v1 input.
- [x] Version 2 rejects missing/null/blank `goal_id`, unknown/duplicate fields and malformed input under existing strict parsing rules; canonical marshaling retains authored values, deterministic order, indentation and trailing newline.
- [x] Two v2 Goals with one project_id and different goal_id round-trip distinctly; no claim of global uniqueness without an authoritative store.
- [x] Strict TDD RED (behavioral when feasible), GREEN and refactor observed; focused `cd engine && go test ./goal -count=1`, integration `cd engine && go test ./... && go vet ./...`, `gofmt` check. Other configured module checks not run for this scoped Goal package change.

## Route and checks
- Delegated direct: changes to production code, tests and fixture are multiple nontrivial files; preparation belongs to one bounded writer. 400 authored changed lines is advisory only; forecast ~150–300 lines (actual measure after implementation).
- TDD: enabled, source `openspec/config.yaml` (`testing.strict_tdd: true`, `apply.rules.tdd: true`). Runner `cd engine && go test ./...`; focused runner above. Never invent failing test evidence.
- The worktree was created only after the user authorized it. The user later authorized a local Goal v2 commit; no push or PR is authorized.

## Tasks and progress
- [x] G2-1 (delegated) — Added v2 identity tests/fixture first; observed compile RED, then behavioral RED after the API field scaffold.
- [x] G2-2 (delegated) — Implemented version-aware Goal parser/validator/serializer; observed focused GREEN and post-refactor checks plus independent verification.
- [x] G2-3 (parent) — Read back code/check evidence; Shaper Goal/worktree binding is still needed. Goal v2 alone does not close Phase 3.

## Evidence
- Worktree creation: `git worktree add -b feat/goal-v2-identity /home/labdrian/labdrian-sdd-overlay-goal-v2 574ef923922313eebbdf0b7b96faae4c59cb8f65` succeeded; isolated worktree initially clean.
- TDD RED: `cd engine && go test ./goal -count=1` failed first on absent `GoalID`; after adding the field scaffold, it failed behaviorally for rejected v2 `goal_id` and unsupported v2 marshaling. The writer then made the v2 tests GREEN and reran focused tests after final formatting/comment changes.
- TDD GREEN/refactor: `cd engine && go test ./goal -count=1` passed; `cd engine && go test ./... && go vet ./...` passed; `gofmt -d engine/goal/goal.go engine/goal/goal_test.go` and `git diff --check` produced no diagnostics. Fresh independent verifier observed the same commands passing and reviewed both fixtures. Non-engine configured module/shell checks were not run, because this slice changes only `engine/goal/`.
- Source delta: `goal.go` +21/-5, `goal_test.go` +136/-1, `testdata/v2.json` +19. Source work-unit commit `8c98646918df5688c6e67ec8546b01fa72a228b8` (`feat(goal): add versioned goal identity`) includes exactly the three reviewed Goal paths; post-commit tree blob hashes matched pre-commit bytes. Post-commit focused test spot-check passed. No remote action or Shaper implementation. Engine-only search found no non-test consumer of `Goal` struct literals; external unkeyed composite literal consumers remain unverified and could be source-incompatible with a new exported field.
- Native RDD review on the exact three-file Goal v2 candidate (tracker excluded) froze target `sha256:607ba4af196bab5b51f79302de16fd1ee49d6bc01575a2ad54422969cdad62f3`, lineage `review-765ab27af5e5b4a3`, tier medium, one reliability lens. The last capture closed approved; bound STATUS offered the exact acknowledgement and `acknowledge-approved` reported authority burned. This is review evidence, not permission to commit or deliver.
- Read-only Shaper map: no implemented Goal store/lookup by `(project_id, goal_id)`, persistent `worktree_id`, or Shaper readiness evaluator exists. `longterm-mem/internal/projectid/projectid.go` distinguishes Git `WorktreeRoot`/`CommonDir` from project identity but does not bind a Goal. The Phase 3 spec remains untracked in the original checkout, deliberately not copied here. Binding and readiness tests remain pending a product decision; no positive `ready` claim is made by Goal v2.
- Remaining integration: Goal v2 supplies `(project_id, goal_id)` only; authoritative Goal selection, worktree association, currentness and positive Shaper readiness evidence are not implemented. No Goal completion or worktree isolation is implied by identity alone.

## Next step
Clarify whether worktree identity must be durable across sessions for Shaper readiness or whether observed Git provenance is sufficient at plan time; separately identify the authoritative Goal source and flag/scope evidence before implementing a positive readiness test. The original dirty checkout remains unchanged; no push or PR.
