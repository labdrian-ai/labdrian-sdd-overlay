# ODD Task — Phase 5: Procedural memory integration

> **Roadmap authority:** [Standalone Platform Master Roadmap](../../openspec/decisions/standalone-platform-roadmap.md), canonical **Phase 5 — Procedural memory integration**: integrate Goal memory scope, workflow retrieval policy, Engram evidence, longterm-mem queries/promotions, candidate thresholds, recovery detection, deduplication, and per-candidate approval without merging memory layers or relaxing authority.

## Decisions (user, 2026-09-27)

1. **Close `procedural-memory-lifecycle` first.** Done: PR #434 (`409c2ab`) archived it on `main` with PARTIAL verification preserved and an explicit absent landing anchor.
2. **Per-candidate approval is global-tier only.** Promotion into `skills/<id>/SKILL.md` stays human-gated behind the lint gate; the project tier stays autonomous and auditable (prior two-tier decision, Engram #3426). The roadmap will state this; no code change.
3. **Typed memory directive (option a).** A small `MemoryDirective` v1 record — `scope`: `none | goal | project`; `sources`: a closed, duplicate-free set of `engram`, `longterm-mem`, `procedural-skills`; `write`: `none` (reading memory never grants writing it). A new `engine/memoryscope` package resolves directives into a **query plan** and executes nothing: the actual query belongs to the runtime adapter (Phase 7). The workflow profile supplies the default; Goal and handoff directives may only **narrow** it (smaller scope, subset of sources), never widen it — a widening attempt is refused. The existing free-text `memory_scope` and `memory_policy` stay as human-readable descriptions.

## Engineering defaults (parent; user may overturn)

- Goal and handoff directives are supplied as separate optional directive files, so Goal v2 and handoff v3 schemas stay unchanged; a future schema version may embed them.
- Profile defaults live in `engine/memoryscope` keyed by the five profile names, derived from each profile's existing `memory_policy` prose and documented next to it. `standalone-minimal` defaults to `none` with no sources.
- **Defaults are ceilings (accepted fix, 2026-09-27).** Because narrowers can only narrow, a source absent from every default is unreachable. The first defaults granted only `engram`, so `longterm-mem` and `procedural-skills` could never be planned. Defaults now grant the widest read each `memory_policy` allows: `odd`, `maintenance` → `project` with all three sources; `sdd` → `project` with `engram` only (its policy forbids mixing stores; corrected in review, `65ccf58`); `incident-recovery` → `goal` with `engram` + `procedural-skills` (case-bounded, no broad long-term memory); `standalone-minimal` → `none`. A reachability test guards the closed set of sources and scopes.
- The query plan carries the effective scope, sources, filters (`project_id` always for `goal` and `project`; `goal_id` only for `goal`), `write: none`, and a no-authority statement. No longterm-mem import is added to `engine`, keeping the module boundary.

## Scope and constraints

- Branch `feat/memory-directive` from `main` (`409c2ab`), worktree `~/labdrian-sdd-overlay-shaper`.
- In scope: new `engine/memoryscope`; `labdrian memory plan` CLI and dispatch; README; roadmap Phase 5 row (approval scope, retrieval policy, historical acceptance comparison).
- Out of scope: executing queries, writing memory, changing Goal/profile/handoff schemas, runtime adapters (Phase 7), relaxing any existing authority gate.
- TDD strict; checks `gofmt -l`, `go build ./...`, `go vet ./...`, `go test -count=1 ./...`, `staticcheck ./...`. RDD on. Delivery `ask-on-risk`, `stacked-to-main`.

## Tasks

- [x] M1 — `engine/memoryscope`: `MemoryDirective` v1 strict parse and validation, profile defaults, narrowing resolver that refuses widening, and the query plan. *Route: delegated writer.*
- [x] M2 — `labdrian memory plan --profile <name> [--goal <goal.json>] [--goal-directive <file>] [--handoff-directive <file>]` (read-only JSON), dispatcher entry, shell harness, README. *Route: delegated writer.*
- [x] M3 — Roadmap Phase 5 row: global-tier approval scope, typed retrieval policy, and a clause-by-clause comparison against the historical acceptance (procedural-memory-lifecycle, longterm-mem item 25), keeping unmatched items as linked distinct IDs; native reviews, issues, pull requests, merge on approval.

## Acceptance criteria

- A narrower Goal or handoff directive narrows the plan; any widening (larger scope or an extra source) is refused with a named reason.
- `write` is always `none`; no command queries or writes memory.
- Engine never imports longterm-mem internals; memory layers stay separate.

## Progress and evidence

- **M1** — `7b0f7eb` `feat(memoryscope): add typed memory directive and query plan` (541 lines). Delegated writer; RED observed before GREEN.
- **M2** — `b43b42f` `feat(cmd): add labdrian memory plan` (620 lines): CLI, dispatcher `cmd_memory`, shell harness, README. Accepted writer deviation: `Resolve` takes `projectID`/`goalID`; unused IDs are omitted, and plan sources serialize as `[]`, never `null`.
- **Reachability fix** — `daa205d` `fix(memoryscope): make every memory source reachable from profile defaults` (110 lines). Inline; RED observed (`TestEverySourceAndScopeIsReachableFromSomeDefault` named `longterm-mem` and `procedural-skills` as unreachable), then GREEN. The two CLI tests that relied on a `goal` ceiling moved to `incident-recovery`; a new end-to-end test shows `odd` reaching `procedural-skills` through a goal narrower. A latent index panic in `TestDefaultForKnownProfiles` on a length mismatch was hardened to `Fatalf`.
- **Slice 1 native review** — lineage `review-5a4bb6e9f60afdc2`, medium, granted. One reliability lens; CRITICAL `R3-goal-scope-empty-goalid` (scope goal accepted a blank `goal_id`, so a plan advertised single-goal reads while its filters covered the whole project). One bounded correction `5167781` `fix(memoryscope): require goal_id for goal-scoped plans` (RED observed, then GREEN); targeted validator approved; acknowledged. Informational follow-ups: `R3-parse-accept-fields-unasserted` (WARNING), `R3-defaultfor-fallback-unreached`, `R3-plan-json-shape-untested` (SUGGESTION). Reviewed boundary → `5167781`.
- Slice 2 rebased onto `5167781`: M2 `a693193`, reachability fix `3972a01` (commit IDs after rebase; previously `b43b42f`, `daa205d`).
- **Slice 2 native review** — lineage `review-ee72c5f077bb7eee`, high, granted, four lenses. Two CRITICAL blockers: `R2-sdd-default-mixes-stores` (the sdd ceiling listed three stores against its own "do not mix stores" policy) and `R4-stdout-write-ignored` (a failed plan write still exited 0). One bounded correction `65ccf58` `fix(memory): keep sdd on one store and report failed plan writes` (41 lines; RED observed for both, then GREEN); targeted validator approved; acknowledged. Informational follow-ups (WARNING): `R1-memory-default-widening`, `R2-odd-default-interpretive-leap`, `R3-1` (goal.Parse failure untested), `R3-2` (narrower read/parse failures untested), `R4-silent-identifier-drop` (scope none now drops IDs silently); plus SUGGESTIONs `R2-maintenance-default-write-vs-read-reasoning`, `R3-3`..`R3-6`.
- Checks at `65ccf58`: `gofmt -l` empty; build, vet, staticcheck clean; `go test -count=1 ./...` 22 packages ok, 0 FAIL.
- Checks at `3972a01`: `gofmt -l` empty; `go build ./...`, `go vet ./...`, `staticcheck ./...` clean; `go test -count=1 ./...` 22 packages ok, 0 FAIL.
- Delivery: both slices exceed the ~400-line budget, mostly tests. Slice 1 = M1 + correction (`7b0f7eb`, `5167781`); slice 2 = M2 + reachability fix + review correction (`a693193`, `3972a01`, `65ccf58`), stacked-to-main. `size:exception` is needed per slice, with maintainer approval.
- **M3** — this commit (`docs(roadmap): record Phase 5 procedural memory integration evidence`; hash is necessarily this file's own final commit hash, which cannot be embedded in itself without changing it — read it via `git -C /home/labdrian/labdrian-sdd-overlay-shaper log -1 --format=%H -- odd/tasks/procedural-memory-integration.md`): rewrote the Phase 5 row in `openspec/decisions/standalone-platform-roadmap.md` with a clause-by-clause classification (Goal memory scope, workflow retrieval policy, Engram evidence, longterm-mem queries/promotions, candidate thresholds, recovery detection, deduplication, per-candidate approval), status `implemented; pending user confirmation`, and updated the two comparison rows (`longterm-mem-knowledge-ingestion` → distinct/not merged; historical `longterm-mem` item 25 → partially compared, MCP module/query/promote confirmed, deployment routing left to Phase 7). Route: delegated writer (mapping trigger: 4+ sources — `engine/memoryscope`, `engine/skills`, `longterm-mem/internal/mcpserver`, `openspec/specs/procedural-candidate-detection`, the archived `procedural-memory-lifecycle` change, and the `longterm-mem-knowledge-ingestion` ledger).

## Next step

Both slices reviewed and acknowledged, pushed, and opened: slice 1 issue #435 → PR #437 (base `main`); slice 2 issue #436 → PR #438 (base `feat/memory-directive`, retargets to `main`). Issues carry `status:approved`; PRs carry `type:feature` + `size:exception` (user-approved 2026-09-27). CI green on both. Merged on user approval 2026-09-27: #437 → `017ec81`, #438 (retargeted to `main`) → `c727ea3`; issues #435 and #436 closed; post-merge `main` CI green (all nine jobs). M3 done (roadmap Phase 5 row, this commit). Next: native review, issue, PR, and merge on approval for this commit; then redeploy the engine from `main` on user approval.
