# ODD Task — Versioned standalone Goal contract

## Objective
Design and implement a runtime-neutral, versioned `Goal` contract for the standalone platform, using strict TDD and preserving the approved phase order.

## Problem and why
The standalone platform needs a canonical, explicit representation of user intent before workflow profiles, shaper decisions, memory integration, or runtime adapters consume it. The contract must be versioned and must not imply that a prompt or model output grants execution authority.

## Authorized scope
- Add the Goal contract package and tests under `engine/goal/`.
- Keep the work limited to the Goal model, parsing/validation, canonical serialization, and package-level tests.
- Treat `project_id` as a required top-level Goal field, per the user's D1 follow-up decision.
- Do not implement workflow profiles, shaper, runtime adapters, permissions, memory persistence, runtime execution, migration, installation, or release behavior.
- Do not alter the pre-existing dirty/untracked procedural-memory archive, runtime-roster change, Archify assets, or other user work.
- The user later authorized work-unit commits and PR creation for Phase 1 and selected `feature-branch-chain`; the latest instruction now explicitly authorizes merging the requested Goal PR when ready and requests notification afterward. Follow repository checks/protection, and clarify before merging additional chain PRs if scope is ambiguous. The destination is `https://github.com/labdrian-ai/labdrian-sdd-overlay`, and the user authorized using the GitHub credentials available in this environment for that repo only; do not inspect or expose credential values. The base branch is confirmed as `main`. The user authorized creating a Goal tracking issue; follow the repo's YAML Issue Forms, search duplicates, and obtain review of exact content before one write. Do not apply `status:approved` without target-host-verified authority and exact instruction. Do not change runtime state.

## Contract direction
The canonical Goal fields from `openspec/decisions/standalone-platform-roadmap.md` are `objective`, `scope`, `constraints`, `non_goals`, `acceptance_criteria`, `memory_scope`, `runtime_scope`, and `delivery_boundary`. Version 1 also has required `project_id` at the top level, as explicitly approved by the user. Use JSON with integer `version: 1`; `project_id`, `objective`, `scope`, `memory_scope`, `runtime_scope`, and `delivery_boundary` are required non-blank strings; `constraints`, `non_goals`, and `acceptance_criteria` are required non-null string arrays, with at least one acceptance criterion (the other two arrays may be empty). Preserve authored values rather than rewriting them during parsing.

Validation is deterministic and structural: reject missing/null/blank required values, blank array elements, duplicate JSON keys, unknown fields, malformed/trailing JSON, and unsupported versions. A version-1 golden fixture is the compatibility baseline; no earlier Goal format exists in this checkout, so do not invent a v0 migration. Do not claim to detect natural-language contradictions; unresolved semantic ambiguity belongs to the later shaper/human decision.

Goal is declarative data only. It does not authorize actions, grant permissions, dispatch work, persist memory, or claim runtime enforcement. Runtime capabilities remain classified as `enforced`, `advisory`, or `unavailable`; prompt-only guidance is not enforcement. Goal validation should be deterministic and structural; it must not pretend to infer natural-language contradictions. Unresolved semantic questions remain for a later shaper/human decision.

## Constraints
- Required reading completed before code: `openspec/decisions/standalone-platform-roadmap.md`, `odd/tasks/standalone-platform-roadmap.md`, `docs/architecture/standalone-product-roadmap.md`, and `openspec/config.yaml`.
- Strict TDD is enabled by `openspec/config.yaml` (`testing.strict_tdd: true`). Tests for the Goal contract must be authored and run before implementing the model; record the actual RED result, then GREEN, then REFACTOR evidence. Do not invent RED evidence.
- Existing `engine` conventions favor strict JSON decoding, unknown/trailing input rejection, version checks, deterministic indented JSON, and a final newline. Confirm source conventions in the writer's preparation.
- Targeted checks of this checkout found no `engine/goal/` or `engine/standalone/`. Older project-scoped Engram entries mention standalone packages without matching files in this tree; do not transplant that cross-checkout memory as source evidence.
- No pre-existing Goal wire version was found in this checkout. Preserve a version-1 golden fixture as the compatibility baseline; do not invent a legacy migration format without evidence.
- Technical artifacts and comments remain in English.
- Initial planning estimate was approximately 250 lines; isolated candidate measurement is 641 added lines across the three Goal files and this task document (authoritative for delivery planning). The 400-line budget is a slicing heuristic, not permission to omit tests or docs.
- Delivery strategy: `ask-on-risk`; the user selected `feature-branch-chain` and wants Phase 1 marked closed only after merge. Commit/PR work and merging the requested Goal PR are now authorized, subject to repository policy and required checks; notify the user after merge.

## Route and authorized surfaces
- Route: delegated direct.
- Trigger evidence: implementation requires multiple non-trivial package/test files and reading that prepares the write; one writer owns the source changes.
- Writer's allowed edit surface: `engine/goal/**` only.
- Parent-owned tracking surface: this task document and its Engram mirror.

## Acceptance criteria
- The package exposes a versioned Goal representation that includes all approved fields and required top-level `project_id`.
- Contract tests cover valid decoding, required fields, invalid/ambiguous structural input, unsupported versions, strict input handling, stable canonical serialization, and a version-1 compatibility fixture.
- The test-first sequence is observable: tests are written first and run RED before the model implementation; implementation then reaches GREEN and receives a focused refactor without behavior regression.
- Parsing and serialization are deterministic and fail closed on malformed or unsupported wire data.
- No runtime, permission, memory, install, review, delivery, or release behavior is added.
- Applicable focused and configured integration checks are run; failed, skipped, unavailable, and passing results are recorded separately.

## Checks
- RED: `cd engine && go test ./goal` immediately after adding contract tests and before production model code. Capture the actual failure; a compile-only failure is not sufficient final RED evidence, so establish behavioral failing assertions before implementing semantics where feasible.
- GREEN/refactor: `cd engine && go test ./goal` after implementation and after any refactor.
- Integration: `cd engine && go test ./... && go vet ./...`.
- Full configured checks from `openspec/config.yaml`: Go tests and `go vet` for `longterm-mem`, `engine`, `tui`, and every `tools/*` module; `bash -n bin/overlay && shellcheck bin/overlay`.
- Baseline before source changes: all configured tests, vets, and shell checks passed in this session; post-test Git status matched the initial dirty state.

## Progress
- [x] Read all four required roadmap/configuration files.
- [x] Close P0 evidence pass and resolve D1: user approved Claude Code first with explicit project identity, native runtime authorization, and capability labels `enforced/advisory/unavailable`; prompt-only gate is not enforcement.
- [x] Confirm strict TDD configuration and configured test runners.
- [x] G1 — Added Goal contract tests first. Initial test-only run failed because `Goal` and `Parse` did not yet exist; after a minimal API scaffold, the behavioral assertions failed as expected.
- [x] G2 — Implemented Goal v1 parser/validator and canonical serializer; focused tests reached GREEN, then refactor/strictness cases passed.
- [x] G3 — Focused/configured verification passed; isolated native review approved and acknowledged for the exact Goal/task candidate. No commit was made.

## Evidence and decisions

### TDD cycle observed
- Test-only RED: `cd engine && go test ./goal` failed before any production model file existed, reporting undefined `Goal` and `Parse` and package build failure.
- Behavioral RED: after only the API scaffold, the same command failed valid parsing, allowed empty arrays, invalid-value validation, canonical marshaling, and v1 fixture assertions.
- GREEN: `cd engine && go test ./goal` passed (`ok .../engine/goal 0.003s`).
- Refactor/strictness: added nested and escaped-equivalent duplicate-key, invalid UTF-8, non-string array item, and case-variant field tests; the latter exposed case-insensitive standard decoder matching and was fixed with exact field-name validation. Final focused rerun passed (`ok .../engine/goal 0.008s`).
- `gofmt` completed; `git diff --check -- engine/goal` emitted no output, but Git does not include untracked files in that diff check.
- Independent verifier: `cd engine && go test ./goal` passed; the full configured Go test/vet matrix for longterm-mem, engine, tui, and five tools passed; `bash -n bin/overlay && shellcheck bin/overlay` passed; `gofmt -d engine/goal/*.go` and `git diff --check` emitted no output. No edits were made by verification.
- Native ASSESS returned `unassessable` because the native command returned empty output. After the native review closed, ASSESS reported `nativeReviewOutcome: closed`, with `writerSelfVerification: true` and `independentVerifier: false`; the configured independent verifier had also passed.
- Native review was run only in an isolated worktree at `/tmp/labdrian-goal-review-01a0cc27-45e3-7004-82f4-381be26c5cce`, selecting exactly the three `engine/goal/` files and this task document. It closed approved and was acknowledged (lineage `review-018cae353c58f2c3`, target `sha256:72e2b7532fe24df65dff0b68a9925667a8a84ad0ab10f12c205bdc2e1a2c3338`). The receipt reports one non-blocking informational warning `R3-001` at `engine/goal/goal.go:47`; no correction was opened and the review stands.
- The original worktree still has 29 pre-existing tracked changes under `openspec/changes/procedural-memory-lifecycle/` and `openspec/specs/`, plus unrelated untracked work; they were excluded from native review and preserved.
- Delivery update: user authorized work-unit commits and PRs, selected `feature-branch-chain`, and wants Phase 1 closed only after merge. The latest user instruction explicitly authorizes merging the requested Goal PR when ready and asks for notification after merge; repository checks/protection still apply. The isolated candidate totals 641 added lines (goal.go 215, goal_test.go 325, fixture 18, task doc 83; all four paths are untracked relative to HEAD). One-pass slice plan: tracker foundation = Goal type/validation with its tests plus the 83-line ODD task document (~220 lines); child 1 = strict JSON parsing with parser tests (~340 lines); child 2 = canonical serialization, fixture, and serialization/round-trip tests (~90 lines). Each child is estimated under 400; confirm exact diff counts at commit/PR time. The user initially confirmed no approved issue, then authorized creating a Goal tracking issue. The issue-first workflow still blocks branch creation and commits until an issue has `status:approved` and is verified. Issue #380 (`https://github.com/labdrian-ai/labdrian-sdd-overlay/issues/380`) was created exactly once and read back with matching title/body; at creation it was OPEN with `type:feature` only. After the user reported approval, a fresh target-host read confirmed it is OPEN with both `status:approved` and `type:feature`. No delivery branch, commit, PR, or merge has occurred. The exact slice boundaries still require a read-only mapper; repeated mapping-worker launches, including the user-requested GADU call, failed before work started. PRs require an approved linked issue and exactly one `type:*` label. The user supplied destination `https://github.com/labdrian-ai/labdrian-sdd-overlay` and explicitly authorized the GitHub credential source in the current environment for this repo. Do not inspect or expose its values. The base branch is confirmed as `main`. Read-only GitHub discovery confirmed issues enabled, Discussions disabled, and the `Feature` YAML form `.github/ISSUE_TEMPLATE/feature.yml` with title prefix `feat: `, create label `type:feature`, five required text fields, and two required acknowledgements. The `status:approved` label exists but is protected and will not be applied without target-host-verified authority and exact instruction. One complete open+closed duplicate search for `Goal standalone` returned zero matches. The user reviewed and confirmed the exact proposed title/body and both required acknowledgements for one `type:feature` issue; the final privacy scan passed and issue #380 was created/read back with matching title/body. It was initially OPEN with `type:feature`; a later target-host read confirms `status:approved` and `type:feature`.

- Canonical Goal shape: `openspec/decisions/standalone-platform-roadmap.md`, Phase 1.
- D1 decision: Claude Code first; capability assertions must not exceed source/runtime evidence.
- Project identity: required top-level `project_id` (user selected this placement).
- P0 static review found `engine/reviewreceipt` coupled to the Gentle command/schema; it stays outside Goal and behind optional compatibility.
- Current repository state is intentionally dirty. Branch `feat/procedural-memory-lifecycle-7b-retirement-detector`, HEAD `f983e7435494452d6bed40ef683ac5024e4e3cd8`; preserve all pre-existing changes.
- P0 follow-ups remain: no root `LICENSE`; Laya weights/license are unverified; Codex clean body-loading proof, retirement rollback-of-rollback proof, clean release smoke, and a historical `overlay-versioned-releases` verification blocker remain recorded. These do not authorize scope expansion in this Goal task.

## Delivery progress
- [x] DL1 — One honest split mapped: tracker foundation with type/validation/tests and task doc (~220 lines), child 1 strict parser plus tests (~340), child 2 canonical serializer/fixture/tests (~90). Exact changeset counts must be checked at each commit; no size-only code/test/doc reductions.
- [x] DL1a — Discovered the Feature form/policy, completed one open+closed duplicate search, obtained human confirmation of the exact issue and acknowledgements, and created/read back issue #380 (`https://github.com/labdrian-ai/labdrian-sdd-overlay/issues/380`); a later fresh target-host read confirmed it OPEN with `status:approved` and `type:feature`.
- [ ] DL2 — Feature Branch Chain in progress: read-only mapper resolved exact Goal source/test boundaries; isolated tracker worktree `feat/goal-v1-tracker` starts from remote `main` at `fe2df1bde5dfb16e4a17789d22499d3a5f9ec7b3`. Issue #380 is verified `status:approved` for core/validation and this task document. Child parser and serializer PRs need their own approved issues before remote submission. Record commit and PR identities only after creation.
- [ ] DL3 — Merge the requested Goal PR once repository policy/checks pass, notify the user, then close Phase 1. The user explicitly authorized this merge in the latest instruction; no PR exists yet.

## Next step
Implementation and prior native review of the complete Goal candidate are recorded above. The delivery mapper completed, and an isolated `main`-based tracker worktree now exists at `/tmp/labdrian-goal-delivery`; original mixed worktree remains intact. Create tracker/core work-unit commits and PRs only within issue #380's approved scope, then obtain approved in-scope issues for the parser and serializer child PRs. Follow native RDD checks per actual committed candidate. Merge the requested Goal PR only after the full chain meets repository policy and checks; notify the user and close Phase 1 after actual merge.
