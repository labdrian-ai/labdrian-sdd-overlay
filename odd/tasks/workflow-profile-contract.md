# ODD Task — Workflow Profile contract

> **Roadmap authority:** [Standalone Platform Master Roadmap](../../openspec/decisions/standalone-platform-roadmap.md), canonical item **Phase 2 — Workflow Profile**. This file tracks detailed execution checks and evidence; it is not a second roadmap. Status is **active as recorded in this ledger**. The ledger does not establish profile semantic values: the compatibility modes `standalone` / `Gentle-compatible` remain unmapped to the five Workflow Profiles, and that question is unresolved in the master. Do not create a duplicate Phase 2 task.

## Objective
Define and implement the canonical standalone Workflow Profile contract and concrete initial values for `odd`, `sdd`, `standalone-minimal`, `maintenance`, and `incident-recovery`, with fail-closed validation and contract tests.

## Problem and why
The standalone platform needs a runtime-neutral declarative description of workflow stages, roles, checks, and memory/review/delivery policies before later Shaper and execution phases consume it. The canonical roadmap requires testing dependencies, mandatory gates, restart/resume, incompatibility, and fail-closed resolution.

## Authorized scope
- Phase 2 of `openspec/decisions/standalone-platform-roadmap.md` only.
- Define the profile model, profile validation/resolution contract, five named profile values, and tests necessary to establish that contract.
- Derive concrete values from canonical repository sources and observed conventions; do not invent runtime capabilities or imply that profile declarations enforce execution.
- Preserve all pre-existing dirty/untracked files, especially procedural-memory-lifecycle work, `engine/goal/`, and unrelated local assets.
- No Shaper, role executor, memory integration/persistence, runtime adapters, installer/migration, release work, issue/label, push, PR, merge, or runtime-state mutation. The user later explicitly authorized a dedicated branch/worktree and one local Phase 2 work-unit commit.
- Worktree started on `feat/procedural-memory-lifecycle-7b-retirement-detector` at `f983e7435494452d6bed40ef683ac5024e4e3cd8`; preserve it. Do not clean, reset, or switch branches.

## Requirements and boundaries
- Canonical shape: `WorkflowProfile { name, stages, roles, checks, memory_policy, review_policy, delivery_policy }`.
- Initial named profiles: `odd`, `sdd`, `standalone-minimal`, `maintenance`, `incident-recovery`.
- Phase 2 profiles may name the workflow subagents used by each profile. The user clarified these profile-specific subagents are intended to be replaced/reorganized later by the reusable role system planned for Phase 4; keep that migration explicit without prematurely deciding the exact mapping or claiming Phase 4 is implemented.
- Required contract tests: stage dependency ordering, mandatory gates, restart/resume, incompatible profiles, and fail-closed resolution.
- Canonical authority is `openspec/decisions/standalone-platform-roadmap.md`; `docs/architecture/standalone-product-roadmap.md` has distinct P0-P8 numbering and must not reorder or redefine Phase 2.
- Do not include execution permissions or claim that profile validation itself enforces runtime behavior. Preserve distinctions among unsupported, unavailable, failed, skipped, and unverified capabilities.
- Technical artifacts and comments in English.

## Approved profile values baseline

The user approved this five-profile table as the baseline for Phase 2 contract tests and implementation. Values that were not established by source are accepted as initial product proposals, not claims about existing runtime support. By user decision, profile semantics and compatibility mode are separate axes; this does not establish that every combination is supported. Role entries below are Phase 2 workflow subagent slots; Phase 4 is expected to replace/reorganize them and adjust the profiles to use the reusable role system. The exact mapping is intentionally deferred.

| Profile | `stages` | `roles` (proposed Phase 2 subagent slots) | `checks` | `memory_policy` | `review_policy` | `delivery_policy` |
|---|---|---|---|---|---|---|
| `odd` | Authorize → explore → resolve uncertainty → classify → track if substantial → implement task by task → close | Human, coordinator, explorer, implementer, verifier | TDD only when configured; otherwise applicable functional checks; coordinator spot-check | Durable task ledger and Engram mirror for substantial work; store evidence as well as status | Inherit user-owned RDD switch; candidate consent remains separate | One work unit per task; Conventional Commit only within authorization and repo policy; push/PR/merge remain separate decisions |
| `sdd` | Dispatcher-selected planning phases → tasks → apply → verify → archive | Human, orchestrator, phase subagents | Native dispatcher/dependencies; configured TDD and apply checks; execute `verify` and record its report before archive. Findings do not automatically block archive. | Use only the store declared/resolved for the change; do not infer or mix stores | Inherit RDD and per-candidate consent; no inferred approval | Strategy declared for the change; default proposal `ask-on-risk`; remote delivery follows ordinary policy |
| `standalone-minimal` | Authorize → bound scope → execute one bounded sequential path → check → report | Human and one sequential executor; no subagent fan-out required | Relevant available checks; preserve `unavailable`/`unverified`, never synthesize PASS | No persistence required; allow only a store explicitly configured by the caller | No Gentle/RDD dependency; state when native review is unavailable | Local by default; no implicit commit, push, PR, or release |
| `maintenance` | Inspect → isolate smallest change → repair → validate → report | Human, investigator, maintainer, verifier | Before/after evidence and focused checks; expand with impact, not ceremony | Record substantial work units; do not promote transient incidents to reusable memory | Inherit RDD; use applicable candidate review only when enabled | Bounded local change; delivery follows repository policy, with no automatic publication |
| `incident-recovery` | Preserve evidence → classify → identify supported recovery → obtain required authorization → recover → verify → report | Human incident owner, investigator, recovery operator, verifier | Capture initial state; use only supported audited recovery; confirm final state; stop when evidence is missing | Case-bounded evidence; exclude secrets/raw logs; preserve verifiable references | Does not bypass RDD or maintenance authorization; use only supported native recovery | Recover only authorized scope; no publication or opportunistic scope expansion |

### Approved design decisions

1. **Compatibility/profile relationship:** separate axes. A Workflow Profile describes work organization; compatibility mode describes integration/dependency posture. This does not establish support for every combination.
2. **SDD verify:** run before archive and record its report. Findings do not automatically block archive.
3. **Standalone-minimal persistence:** persistence is not required; a caller-configured store is allowed.
4. **Phase 4 role migration:** Phase 2 profile subagent slots will later be replaced/reorganized, and the Workflow Profiles themselves will be adjusted to use the reusable role system. Exact mapping and profile edits belong to Phase 4.
5. **Profile values:** user approved the complete table as the baseline for contract tests and implementation. Values proposed where source contracts are silent are product decisions for this initial baseline, not claims about existing runtime support.

Profile data remains declarative and grants no execution, review, memory-write, or delivery authority.

## Route and authorized surfaces
- Route: delegated direct; source mapping precedes one bounded writer because implementation spans a model, profile values, and contract tests.
- Trigger evidence: Phase 2 requires mapping multiple existing contracts and conventions; implementation is expected to touch multiple non-trivial files.
- Parent-owned tracking surface: this task document and its Engram mirror.
- Source edit surfaces: `engine/workflowprofile/**`; parent-owned tracker: `odd/tasks/workflow-profile-contract.md`.

## TDD and checks
- Effective mode: strict TDD enabled by `openspec/config.yaml` (`testing.strict_tdd: true`, `apply.rules.tdd: true`).
- Configured engine runner: `cd engine && go test ./...`.
- Focused RED/GREEN/refactor command: `cd engine && go test ./workflowprofile -count=1`; writer observed failing baseline/regressions, then passing runs.
- Run applicable focused contract tests, then the configured engine test suite. Record all unavailable, failed, skipped, and passing checks honestly.

## Acceptance criteria
- The public declarative contract contains all canonical fields and can represent each of the five named profiles.
- Concrete profile values are grounded in canonical repository contracts; unsupported assumptions fail closed or remain explicitly unresolved rather than being silently defaulted.
- Validation/resolution rejects invalid dependencies, missing mandatory gates, incompatible profile combinations, and invalid restart/resume state according to the derived contract.
- Tests exercise all canonical Phase 2 coverage areas and demonstrate fail-closed behavior.
- No execution, permission, runtime integration, memory persistence, installation, or release behavior is introduced.
- Strict TDD evidence records observed RED, GREEN, and refactor/integration results.

## Progress
**Current:** Phase 2 implementation is committed on dedicated branch `feat/workflow-profile-phase-2` at `1f654f1e78f1b25e2e1ed9d860f5b008ff71b5f0`. Focused tests, full engine suite (writer-reported), independent verification, and committed-range native review passed. The original dirty worktree and its branch remain unchanged. The review-only worktree is retained and still has the copied ledger untracked.

- [x] Confirm canonical Phase 2 identity and distinguish it from the supporting roadmap's P-numbering.
- [x] Confirm user wants concrete values for all five initial profiles.
- [x] Confirm dirty worktree/branch and preserve baseline.
- [x] Map authoritative values, current source conventions, and exact edit/test surfaces.
- [x] Search prior standalone planning records and session transcripts for explicit profile values; document any remaining gaps without mapping compatibility modes by analogy.
- [x] Resolve proposal decisions and obtain user approval of the complete five-profile baseline.
- [x] Add failing profile contract tests and observe RED for the baseline API.
- [x] Implement initial profile types/values and resolve the incident-recovery verifier finding with RED/GREEN evidence.
- [x] Run focused/configured checks for the incident-recovery correction.
- [x] Enforce all approved SDD mandatory checks with regression tests and observed RED/GREEN.
- [x] Rerun focused/configured tests after SDD correction; record evidence.
- [x] Complete fresh independent verification; focused and full test evidence recorded.
- [x] Create user-authorized dedicated worktree and branch; commit implementation plus tests locally.
- [x] Independently verify the committed tree and acknowledge committed-range native review.
- [x] Record commit identity and final verification; no remote delivery.

## Evidence and decisions
- User confirmed: Phase 2 should define concrete values for all five initial profiles.
- Canonical roadmap lists the five names and contract fields, but does not itself enumerate their internal stage/role/check/policy values; derive these from existing project contracts and surface genuine unresolved requirements instead of inventing them.
- Existing adjacent concepts: `engine/propagator/propagator.go`, `engine/runtime/runtime.go`, `engine/gate/gate.go`, and `engine/goal/goal.go`; no unified WorkflowProfile model was found. The SDD phase graph and phase responsibilities are documented in `skills/_shared/sdd-orchestrator-workflow.md` and `agents/sdd-apply.md` / `agents/sdd-verify.md`.
- Historical decision search completed across Engram and the seven local Pi session JSONL files available for this checkout (dated Sep 8, 13 twice, 20, 23 twice, and 24), including targeted terms for all five profile names and fields. No inspected record contains a human-approved assignment of `stages`, `roles`, `checks`, `memory_policy`, `review_policy`, or `delivery_policy` to all five profiles, nor a crosswalk from compatibility modes. Search scope is bounded to these records; it does not prove that no other transcript/archive exists.
- `/home/labdrian/labdrian-standalone/docs/architecture/standalone-contracts.md` is explicitly a draft v0.1. It defines the separate compatibility modes `standalone` and `gentle-compatible`, explicit-root ownership, durable progress, and ordered Run/Resume behavior, but does not define the five Workflow Profiles. Keep the crosswalk unresolved; do not equate taxonomies by analogy.
- The user confirmed the planned evolution: subagents assigned to Workflow Profiles in Phase 2 will later be replaced/reorganized by the reusable role system planned for Phase 4. The currently recalled role sequence is `prototyper -> shaper -> estimator -> builder -> sweeper -> polisher -> reviewers -> delivery`, with work ordered from general to deep and TDD. Treat this as a migration direction, not an approved profile-to-role mapping; define that mapping in its proper phase.
- Current-source mapping confirms SDD phase/role vocabulary and delivery/memory store contracts. The documented graph shows `apply -> verify -> archive`, while `agents/sdd-verify.md` describes verification as optional diagnostic and non-blocking. The user decided `verify` must run and be recorded before archive, but its findings do not automatically block archive. The user approved the standalone-minimal no-required-persistence policy, separate compatibility/profile axes, and the Phase 4 profile/subagent migration direction.
- Writer TDD evidence: baseline RED from undefined API, followed by GREEN/refactor. Independent verification found missing incident-recovery audited-recovery enforcement; writer fixed it with RED/GREEN regression and reran focused/full tests. A later verifier found the two SDD checks `native dispatcher/dependencies` and `configured TDD and apply checks` were not mandatory. Writer added table-driven removal regressions; targeted RED for both omissions, then GREEN; focused/full engine tests passed and gofmt had no diff. Fresh independent verification then PASSed all requested contracts, focused test, and gofmt.
- Initial pre-commit native review on isolated files approved with informational `R3-001` WARNING at `workflowprofile.go:145`; no correction route was offered. The user then authorized a dedicated branch/worktree and a local commit. Created branch `feat/workflow-profile-phase-2` from base `f983e7435494452d6bed40ef683ac5024e4e3cd8` in `/home/labdrian/labdrian-sdd-overlay-review-workflow-profile`. Commit `1f654f1e78f1b25e2e1ed9d860f5b008ff71b5f0` (`feat(workflowprofile): add validated workflow profiles`) includes only the two implementation/test files (471 insertions). The task ledger remains an untracked copy in that worktree and is not part of the commit.
- Post-commit native ASSESS returned unassessable because that copied ledger is untracked; treated as high risk. A fresh independent verifier then PASSed the committed files; focused test and gofmt check passed. Native INSPECT excluded the untracked ledger and froze only the two committed files against the base commit; committed-range native review approved with no findings and exact acknowledgement burned authority.
- Original dirty worktree/branch remain unchanged. No push, PR, issue, merge, or runtime mutation occurred.
- Engram discovery record: `discovery/workflow-profile-prior-decisions` (obs 3662). Tests and runtime code were not run or changed in the original worktree by the review; test evidence is recorded above.

## Delivery
- Local work-unit commit `1f654f1e78f1b25e2e1ed9d860f5b008ff71b5f0` exists on `feat/workflow-profile-phase-2` in the dedicated worktree. No push, issue, PR, merge, or runtime mutation occurred. The original dirty worktree/branch remain unchanged. The dedicated worktree and its untracked ledger copy are retained; cleanup or remote delivery requires separate authorization.
