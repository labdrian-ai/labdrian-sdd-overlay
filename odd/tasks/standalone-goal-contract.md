# ODD Task — Versioned standalone Goal contract

> **Roadmap authority:** [Standalone Platform Master Roadmap](../../openspec/decisions/standalone-platform-roadmap.md), canonical item **Phase 1 — Canonical Goal model**. This file is the detailed execution/evidence ledger, not a second roadmap. Phase 1 status is **complete as recorded in this ledger only**; this reconciliation does not re-audit its evidence or authorize further work. Do not use this ledger as evidence for Workflow Profile semantic values or status.

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
- The user authorized work-unit commits and PR creation for Phase 1 and selected `feature-branch-chain`; the original tracker chain and follow-up PRs #385/#386 were explicitly authorized for merge and are complete. No merge authorization is established for future Phase 2 work. Follow repository checks/protection. The destination is `https://github.com/labdrian-ai/labdrian-sdd-overlay`, and the user authorized using the GitHub credentials available in this environment for that repo only; do not inspect or expose credential values. The base branch is `main`. The user authorized Goal issue creation; follow the repo's YAML Issue Forms, search duplicates, and obtain review of exact content before one write. Do not apply `status:approved` without target-host-verified authority and exact instruction. Do not change runtime state.

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
- Delivery strategy: `ask-on-risk`; the user selected `feature-branch-chain` and wanted Phase 1 marked closed only after merge. Commit/PR work and merging the requested Phase 1 Goal PRs were authorized and are complete, subject to repository policy and required checks. Future Phase 2 delivery needs its own authorization.

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
- [x] G3 — Focused/configured verification passed; isolated native review approved and acknowledged for the exact Goal/task candidate. Core cleanup committed as `a506afac910d078f599a41806f278910a05253b1` after restoring the exact approved candidate tree.

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
- Delivery update: user authorized and completed the original Goal tracker chain, then explicitly authorized merging follow-up PRs #385/#386 to close Phase 1. The 641-line core candidate was split into tracker, parser, and serialization slices. Issues #383/#384 and PRs #385/#386 are closed/merged; main is now at `e95fc9d66c1bc42eccca6c3ea99961eb9e0c1a5c`. Future Phase 2 merges require their own authorization. PRs require an approved linked issue and exactly one `type:*` label. Target is `https://github.com/labdrian-ai/labdrian-sdd-overlay`; credential scope remains this repository only. Discovery confirmed the Feature YAML issue form; the completed open+closed duplicate search found no earlier follow-up. Do not inspect or expose credential values.

- Canonical Goal shape: `openspec/decisions/standalone-platform-roadmap.md`, Phase 1.
- D1 decision: Claude Code first; capability assertions must not exceed source/runtime evidence.
- Project identity: required top-level `project_id` (user selected this placement).
- P0 static review found `engine/reviewreceipt` coupled to the Gentle command/schema; it stays outside Goal and behind optional compatibility.
- Current repository state is intentionally dirty. Branch `feat/procedural-memory-lifecycle-7b-retirement-detector`, HEAD `f983e7435494452d6bed40ef683ac5024e4e3cd8`; preserve all pre-existing changes.
- P0 follow-ups remain: no root `LICENSE`; Laya weights/license are unverified; Codex clean body-loading proof, retirement rollback-of-rollback proof, clean release smoke, and a historical `overlay-versioned-releases` verification blocker remain recorded. These do not authorize scope expansion in this Goal task.

## Delivery progress
- [x] DL1 — One honest split mapped: tracker foundation with type/validation/tests and task doc (~220 lines), child 1 strict parser plus tests (~340), child 2 canonical serializer/fixture/tests (~90). Exact changeset counts must be checked at each commit; no size-only code/test/doc reductions.
- [x] DL1a — Discovered the Feature form/policy, completed one open+closed duplicate search, obtained human confirmation of the exact issue and acknowledgements, and created/read back issue #380 (`https://github.com/labdrian-ai/labdrian-sdd-overlay/issues/380`); a later fresh target-host read confirmed it OPEN with `status:approved` and `type:feature`.
- [x] DL2 — Native review reliability result was quarantined through the approved maintainer path; the fresh reviewer capture approved and was acknowledged for exact target `sha256:3bf6a495baf6c095837ef64ffe2b4900069978b93d06a0a631b2e57ab0b0f81b` (lineage `review-ec30f092180bcc5c`). The exact unused-helper cleanup was restored, focused tests/vet passed, and commit `a506afac910d078f599a41806f278910a05253b1` was pushed to the core branch. PR #382’s required checks passed.
- [x] DL3 — PR #382 merged into tracker PR #381 at `b4370496b9f07ac2b7cd59637d0b98e6398a2c1c`; PR #381 merged to `main` at `bed6bf811826859d627550e6614e26a891789818`. After the user explicitly authorized closing Phase 1, PR #385 merged at `a9db5c0aaff49614a81d2e17e506495795f931ce` and PR #386 merged at `e95fc9d66c1bc42eccca6c3ea99961eb9e0c1a5c`; all required checks passed before their respective merges. Issues #380, #383, and #384 are closed. Phase 1 is complete.

## Follow-up delivery — parser and canonical serialization
- User explicitly requested the remaining issue/PR delivery. The target-host Feature form says one issue per implementation slice, and the previous accepted delivery plan keeps parser and serialization as separate slices; prepare two issue drafts and two linked, chained PRs.
- At the time follow-up delivery was prepared, local `engine/goal/` contained strict parsing, validation, canonical serialization, tests, and the v1 fixture, while target-host `main` contained only `Goal.Validate`. There was no local code-level gap; the follow-up delivered existing behavior, not new semantics. Both parser and serialization are now merged to `main`.
- The parent executed a complete read-only `gh issue list --state all --search 'Goal' --limit 1000` against the authorized target. It returned closed #380 for the core slice and unrelated issues #338/#120; no parser/serialization follow-up issue exists. This parent-host result supersedes the mapper's inability to run GitHub commands.
- Delivery scope: split the already-present implementation into two independently reviewable issue-bound PRs: parser first (strict `Parse` and its tests), then canonical serialization (ordered indented JSON, one final newline, fixture and round-trip tests). Serializer PR was based on parser PR. This delivered existing behavior without adding new semantics.
- Route: delegated direct for multi-file branch-slice preparation. Existing G1/G2 evidence records strict TDD RED/GREEN/refactor for the combined local implementation; preserve that evidence and rerun `cd engine && go test ./goal` plus `cd engine && go vet ./goal` on each isolated PR candidate. Do not create branches or write source until the respective issue is target-host approved.
- [x] F1 — Presented both exact YAML-form issue drafts; user confirmed the exact content before publication.
- [x] F2 — Created and read back both issues once: parser #383 and serialization #384, each OPEN with `type:feature` and exact body/title match.
- [x] F3 — After the user explicitly authorized the exact #383/#384 approval action and confirmed the authenticated account, target-host pre-read verified `labdrian-ai` ADMIN, then the protected label was applied atomically and read back on both issues. #383 and #384 are OPEN with `status:approved` and `type:feature`.
- [x] F4 — Issue #383 was approved and read back. Parser candidate passed strict TDD, focused tests/vet, independent verification, parent spot-check, and native review/acknowledgement. Commit `b22ebcc342df0516bddd5cd16223aeada70a0054`; PR #385 merged to `main` at `a9db5c0aaff49614a81d2e17e506495795f931ce`; all required CI checks passed. Issue #383 closed.
- [x] F5 — Issue #384 was approved and read back. Serialization child candidate passed strict TDD, focused tests/vet, independent verification, parent spot-check, and native review/acknowledgement. Commit `574ef923922313eebbdf0b7b96faae4c59cb8f65`; after all CI checks passed, PR #386 merged to `main` at `e95fc9d66c1bc42eccca6c3ea99961eb9e0c1a5c`. Issue #384 closed; Phase 1 complete.

## Next step
Phase 1 is closed: both follow-up PRs #385/#386 are merged to `main` at `e95fc9d66c1bc42eccca6c3ea99961eb9e0c1a5c`; all required CI checks passed and issues #383/#384 are closed. Begin Phase 2 only in a fresh Pi session, first reading the approved roadmap/task source and deriving the exact Phase 2 scope; no Phase 2 implementation or merge authorization has been assumed here. The original mixed worktree and unrelated changes remain untouched.
