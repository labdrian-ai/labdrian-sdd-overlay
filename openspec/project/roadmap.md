<!-- GENERATED MIRROR — do not hand-edit. -->

> **This file is the on-disk mirror of the canonical Engram roadmap** (`artifact_store_mode: hybrid`).
> Canonical source: Engram topic `project/labdrian-sdd-overlay/roadmap` (obs #2067, v7), read together with
> `project/labdrian-sdd-overlay/roadmap/appendix` (obs #2771, Parts A-G) and
> `project/labdrian-sdd-overlay/roadmap/appendix-2` (obs #3116, Parts H-N).
> Regenerated 2026-08-28 by the v7 `roadmap-maker` incremental-insert pass that spliced item 25 `longterm-mem`.
> The previous on-disk copy was dated 2026-07-21 and carried only 10 items; every splice from v3 to v6 updated
> Engram only. Any future splice MUST regenerate this file as well.

# Roadmap SDD — labdrian-sdd-overlay

**Date**: 2026-08-28
**Version**: 7 (incremental-insert splice on top of v6 2026-08-27; adds `longterm-mem` as item 25 — all 24 pre-existing items keep their order, dependencies, and tracking/history data with zero renumbering, since the new item depends only on items 18, 19 and 24, all three already delivered. Two authorized non-mechanical edits outside the new item: item 24's Status/Tracking advance to `awaiting-archive (delivered)` on verified merge evidence, and one additive coordination note on item 21. The only other change compresses the settled-history detail of items 14, 18, 23 and 24 plus four rationale sections into stubs pointing at a NEW second appendix volume, purely to stay under Engram's 50000-character cap; no status, dependency, or tracking value moves with that prose.)
**Based on**:
- Manifest: `project/labdrian-sdd-overlay/manifest/{context,rules}` (obs #2063, #2065)
- Architecture: `project/labdrian-sdd-overlay/architect/final` (obs #2066)
- SDD history: 15 archived changes under `openspec/changes/archive/`, 8 active changes under `openspec/changes/` (verified 2026-08-05, carried unchanged — this splice's only authorized write is the new item)
- Requirements brief (v2 insert): `project/labdrian-sdd-overlay/requirements/sync-check-repo-behind-origin`
- Requirements brief (v3 insert): `project/labdrian-sdd-overlay/requirements/actuals-calendar-checkpoint-instrumentation` (obs #2526); pipeline state `sdd/actuals-calendar-checkpoint-instrumentation/pipeline-state` (obs #2527, Tier 2 rationale)
- Requirements brief (v4 insert): `project/labdrian-sdd-overlay/requirements/deterministic-verification-evidence` (obs #2685, R-001..R-019); source evaluation obs #2684; upstream defect obs #2668
- Requirements brief (v5 insert): `project/labdrian-sdd-overlay/requirements/skills-validate-ondisk-gate` (obs #2769, R-001..R-011); mirrored on disk at `openspec/changes/skills-validate-ondisk-gate/requirements.md`; pipeline state `sdd/skills-validate-ondisk-enforcement/pipeline-state` (obs #2768 — provisional change name; authoritative name is `skills-validate-ondisk-gate`)
- Requirements brief (v6 insert): `project/labdrian-sdd-overlay/requirements/overlay-versioned-releases` (obs #3062, R-001..R-011) — captured 2026-08-27 from a real pairing-session incident (self-update fast-forwarded `main` correctly, but the deployed skill file stayed stale until a manual `apply`, with no way to name "which version" was installed); mode: project-inception `reuse` — manifest #2063/#2064/#2065 and architect #2066 loaded READ-ONLY, not rewritten
- Requirements brief (v7 insert): `project/labdrian-sdd-overlay/requirements/longterm-mem` (obs #3103, Part 1/3 — R-001..R-015), `.../longterm-mem-requirements-2` (obs #3109, Part 2/3 — R-016..R-035), `.../longterm-mem-appendix` (obs #3110, Part 3/3 — traceability, Atomic Requirement Order, SDD Change Candidates, Project-Inception Handoff); decisions obs #3102 and #3106; exploration obs #3098 and #3099; pipeline state obs #3105 (Tier 2, rule #3) — mode: project-inception `reuse`, manifest #2063/#2064/#2065 and architect #2066 loaded READ-ONLY, not rewritten

> **Artifact structure (extended at v7 from two observations to three)**: the roadmap outgrows Engram's 50000-character per-observation cap. `project/labdrian-sdd-overlay/roadmap` (this observation) is canonical and self-sufficient for sequencing: complete 25-row Sequence Summary, every item's status/dependencies/Tracking value including all 13 foundational items, full detail for item 25, and its sequencing rationale. `.../roadmap/appendix` (obs #2771, **unchanged at v7**) holds Parts A-G: A foundational items 1-13, B unscheduled `visual-determinism-runner`, C/D item 18 v4 detail and rationale, E rationale for items 23/14, F/G item 19 detail and rationale. `.../roadmap/appendix-2` (obs #3116, **new at v7**) holds Parts H-N: H item 14 detail, I item 18 v5/v6 status-evidence and residual detail, J item 23 detail, K item 24 planning detail, L the v6 pointer rationale sections, M item 24 rationale, N item 19 residual detail. Nothing deleted, summarized away, or downgraded. Any future roadmap-maker pass MUST load all three.

## Sequence Summary

| Order | SDD-id | Status | Depends on |
|---|---|---|---|
| 1 | minimalism-contract-lite | completed | none |
| 2 | tui-professional-polish | completed | none |
| 3 | prespec-malandra | completed | none |
| 4 | opencode-runtime-plugin-lifecycle | completed | none |
| 5 | claude-runtime-lifecycle | completed | opencode-runtime-plugin-lifecycle |
| 6 | codex-runtime-lifecycle | completed | claude-runtime-lifecycle |
| 7 | overlay-coherence-mini-fix | completed | none |
| 8 | anti-generic-ai-design-skill | completed | none |
| 9 | oo-quality-contract | completed | none |
| 10 | oo-quality-contract-runtime-wiring | completed | oo-quality-contract |
| 11 | anti-generic-design-token-anchors | completed | anti-generic-ai-design-skill |
| 12 | fix-propagate-foreign-writer-race | completed | none |
| 13 | skill-external-provenance | completed | none |
| 14 | actuals-calendar-checkpoint-instrumentation | planned *(drift: archived 2026-07-30 — see Verified Status Drift)* | none — corrective/instrumentation change to already-shipped shared tooling |
| 15 | skill-package-manager | awaiting-archive | none |
| 16 | skill-lifecycle | awaiting-archive | skill-package-manager |
| 17 | anti-generic-design-runtime-wiring | awaiting-archive | anti-generic-design-token-anchors |
| 18 | deterministic-verification-evidence | awaiting-archive (delivered) *(v4 recorded `planned`; advanced on verified merge evidence — see item detail)* | none among open items — gates items 19-22 |
| 19 | skills-validate-ondisk-gate | planned | deterministic-verification-evidence (item 18) — already satisfied |
| 20 | skill-project-scope | planned | skill-lifecycle |
| 21 | skill-manifest-gen | planned | skill-project-scope |
| 22 | gadu-portable-operator | in-progress | skill-project-scope |
| 23 | sync-check-repo-behind-origin | planned *(drift: archived 2026-07-11 — see Verified Status Drift)* | none — orthogonal to the skill-registry chain |
| 24 | overlay-versioned-releases | awaiting-archive (delivered) *(v6 recorded `planned`; advanced on verified merge evidence — see item detail)* | deterministic-verification-evidence (item 18); sync-check-repo-behind-origin (item 23) — both satisfied |
| 25 | **longterm-mem** | **planned (new)** | **deterministic-verification-evidence (item 18); skills-validate-ondisk-gate (item 19); overlay-versioned-releases (item 24) — all three already delivered** |

## Detail per SDD

## Foundational Completed Items 1-13 — status and tracking preserved in full

> Full detail blocks (objective, derived-from, dependencies, acceptance evidence, risk, command) for these 13 items are held verbatim in the companion observation at topic key `project/labdrian-sdd-overlay/roadmap/appendix`, Part A. They were moved there at v5 solely because this observation reached Engram's 50000-character cap. Every status and every Tracking/actuals value is reproduced here unchanged; nothing was summarized away.

### minimalism-contract-lite
- **Status**: completed
- **Tracking**: estimate [PENDIENTE] · impl [PENDIENTE] · verify [PENDIENTE] · review [PENDIENTE] · fixes [PENDIENTE] · closure archived 2026-06-20 · deviation none recorded · impact established the reusable minimalism contract other later changes build on

### tui-professional-polish
- **Status**: completed
- **Tracking**: estimate [PENDIENTE] · impl [PENDIENTE] · verify [PENDIENTE] · review [PENDIENTE] · fixes [PENDIENTE] · closure archived 2026-06-22 · deviation none recorded · impact hardened the TUI base that sync-check rendering (R-005) builds on

### prespec-malandra
- **Status**: completed
- **Tracking**: estimate [PENDIENTE] · impl [PENDIENTE] · verify [PENDIENTE] · review [PENDIENTE] · fixes [PENDIENTE] · closure archived 2026-06-25 · deviation none recorded · impact none blocking

### opencode-runtime-plugin-lifecycle
- **Status**: completed
- **Tracking**: estimate [PENDIENTE] · impl completed · verify completed · review completed · fixes completed · closure PR #66 merged · deviation salvage from obsolete PR #13 sliced into a clean OpenCode foundation · impact enabled Claude/Codex native lifecycle slices

### claude-runtime-lifecycle
- **Status**: completed
- **Tracking**: estimate ~520-640 changed lines · impl completed · verify PASS · review Judgment Day approved after ownership fix · fixes post-review removed broad uninstall matching · closure PR #68/#69 merged · deviation split code/OpenSpec artifacts to stay under 800-line review budget · impact Codex was the remaining lifecycle target

### codex-runtime-lifecycle
- **Status**: completed
- **Tracking**: estimate ~620 lines · impl completed · verify PASS · review completed · fixes completed · closure PASS · deviation none recorded · impact closed the three-runtime parity loop

### overlay-coherence-mini-fix
- **Status**: completed
- **Tracking**: estimate [PENDIENTE] · impl [PENDIENTE] · verify [PENDIENTE] · review [PENDIENTE] · fixes [PENDIENTE] · closure archived 2026-07-07 · deviation none recorded · impact none blocking

### anti-generic-ai-design-skill
- **Status**: completed
- **Tracking**: estimate [PENDIENTE] · impl [PENDIENTE] · verify [PENDIENTE] · review [PENDIENTE] · fixes [PENDIENTE] · closure archived 2026-07-08 · deviation none recorded · impact foundation for the token-anchors follow-up

### oo-quality-contract
- **Status**: completed
- **Tracking**: estimate 180-260 changed lines originally, exceeded due to archived docs · impl completed · verify PASS · review completed · fixes completed · closure PR #78 merged · deviation proceeded under maintainer-approved size exception · impact created the source contract for deterministic runtime loading

### oo-quality-contract-runtime-wiring
- **Status**: completed
- **Tracking**: estimate 500-750 changed lines originally · impl completed · verify PASS with blocker fixes applied · closure archived 2026-07-08 · deviation actual review size exceeded the 800-line forecast · impact converted the advisory artifact into scoped runtime behavior

### anti-generic-design-token-anchors
- **Status**: completed
- **Tracking**: estimate [PENDIENTE] · impl [PENDIENTE] · verify [PENDIENTE] · review GADU adversarial dialectic · fixes [PENDIENTE] · closure archived 2026-07-10 · deviation rejected the original vendoring approach for a human-verified anchor table instead · impact none blocking

### fix-propagate-foreign-writer-race
- **Status**: completed
- **Tracking**: estimate [PENDIENTE] · impl [PENDIENTE] · verify [PENDIENTE] · review [PENDIENTE] · fixes [PENDIENTE] · closure archived 2026-07-10 · deviation none recorded · impact none blocking

### skill-external-provenance
- **Status**: completed
- **Tracking**: estimate [PENDIENTE] · impl [PENDIENTE] · verify [PENDIENTE] · review [PENDIENTE] · fixes [PENDIENTE] · closure archived 2026-07-10 · deviation scope-narrowed from auto-fetch to metadata-only per platform-owner rejection · impact none blocking

### actuals-calendar-checkpoint-instrumentation (added in v3 — that splice's triggering requirement)
- **Status**: planned
- **Full detail** (objective, derived-from citations, dependencies, acceptance evidence, risk-if-early, and suggested entry command) is held verbatim in appendix-2 (`project/labdrian-sdd-overlay/roadmap/appendix-2`), Part H — moved at v7 for the 50000-character cap only, and because on-disk evidence shows this change archived (see Verified Status Drift). Status and Tracking are unchanged.
- **Tracking**: estimate [PENDIENTE — next producer: `sdd-time-estimation`] · impl [PENDIENTE] · verify [PENDIENTE] · review [PENDIENTE] · fixes [PENDIENTE] · closure [PENDIENTE] · deviation none yet · impact corrects the calibration baseline every subsequent estimate on this project (including its own) will read from

### skill-package-manager
- **Status**: awaiting-archive **(corrected from "planned" in the stale 2026-07-08 on-disk roadmap — real evidence shows work is substantially done)**
- **Objective**: strengthen package manifest install lifecycle for local skills and provenance (Issue #29 slice 1, read-only registry semantic layer).
- **Derived from**: `openspec/changes/skill-package-manager/` (active, unarchived); manifest rule "preserve reproducible merge semantics"; architecture `engine/skills` module
- **Dependencies**: none
- **Acceptance evidence**: `verify-report.md` in the change folder already records a resolved NO-GO (spec/design contradiction fixed per commit `d69a387` amending ADR-5 to reconcile R-044) — evidence points to ready-to-archive, not "not started".
- **Risk if done early**: n/a — mostly done; risk now is letting it sit unarchived and drifting from `main`.
- **Command**: `/sdd-status skill-package-manager` to confirm, then `/sdd-archive skill-package-manager`
- **Tracking**: estimate [PENDIENTE] · impl [PENDIENTE] · verify NO-GO then resolved per commit d69a387 · review [PENDIENTE] · fixes ADR-5 amendment · closure [PENDIENTE — recommend confirming via sdd-status before archive] · deviation initial verify NO-GO required a design amendment · impact blocks skill-lifecycle from formally closing
- **Coordination note (added v5, no status/ordering change)**: this change's unpromoted delta at `openspec/changes/skill-package-manager/specs/spec.md:225-226` carries R-034 ("`engine skills validate` SHALL be a STANDALONE command this slice; it MUST NOT be invoked or wired inside `cmd_apply`, `cmd_capture`, or any other existing command"). Item 19 `skills-validate-ondisk-gate` inherits and obeys that prohibition; see "Sequencing Rationale (why item 19 lands where it does)" §3 for why it does NOT create an ordering edge between the two items.

### skill-lifecycle
- **Status**: awaiting-archive **(corrected from "planned" — commit "mark all skill-lifecycle tasks done" indicates completion)**
- **Objective**: formalize lifecycle checks/deterministic validation for skill entries; mutable `skills add`/`remove` (Issue #29 slice 3).
- **Derived from**: `openspec/changes/skill-lifecycle/` (active, unarchived); manifest rules "explicit errors"/"manifest tracking"; architecture `engine/skills`
- **Dependencies**: skill-package-manager
- **Acceptance evidence**: tasks.md fully checked per commit history; needs `sdd-verify` confirmation before archive.
- **Risk if done early**: n/a — mostly done.
- **Command**: `/sdd-status skill-lifecycle` to confirm, then `/sdd-verify skill-lifecycle` / `/sdd-archive skill-lifecycle`
- **Tracking**: estimate [PENDIENTE] · impl completed per commit history · verify [PENDIENTE] · review [PENDIENTE] · fixes [PENDIENTE] · closure [PENDIENTE] · deviation none recorded · impact unblocks skill-project-scope

### anti-generic-design-runtime-wiring
- **Status**: awaiting-archive **(new entry — was entirely missing from the on-disk 2026-07-08 roadmap)**
- **Objective**: auto-inject anti-generic-design guidance into UI-generating phases at runtime.
- **Derived from**: `openspec/changes/anti-generic-design-runtime-wiring/` (active, unarchived); anti-generic-design-token-anchors as its foundation
- **Dependencies**: anti-generic-design-token-anchors
- **Acceptance evidence**: tasks.md shows 28 checked / 0 unchecked — evidence points to complete, pending formal verify/archive.
- **Risk if done early**: n/a — mostly done.
- **Command**: `/sdd-status anti-generic-design-runtime-wiring` to confirm, then `/sdd-verify` / `/sdd-archive`
- **Tracking**: estimate [PENDIENTE] · impl completed per tasks.md · verify [PENDIENTE] · review [PENDIENTE] · fixes [PENDIENTE] · closure [PENDIENTE] · deviation none recorded · impact none blocking

### deterministic-verification-evidence (added in v4 — that splice's triggering requirement)
- **Status**: awaiting-archive (delivered) — **carried unchanged from v6**; the v5/v6 merge-evidence prose (PR #129, `be2c3ca`, 82/82 tasks) is verbatim in appendix-2 Part I. **Newly verified 2026-08-28**: the folder is now at `openspec/changes/archive/2026-08-05-deterministic-verification-evidence/`, so the correct status is `completed` — recorded as drift, not corrected here, since this splice's only authorized status write is item 24's.
- **Full detail** (the v5/v6 status-evidence prose, objective, dependencies, and suggested entry command) is held verbatim in appendix-2 Part I; the v4 citations, R-001..R-019 evidence, atomic order, hard constraints, and the item-18 rationale remain in obs #2771 Parts C and D. Moved at v7 for the cap only.
- **Tracking**: estimate [PENDIENTE — pre-start estimate recorded at obs #2687] · impl completed (82/82 tasks checked; merged `be2c3ca`, PR #129, 2026-08-05) · verify [PENDIENTE] · review [PENDIENTE] · fixes [PENDIENTE] · closure [PENDIENTE — merged but not archived; change folder still under `openspec/changes/`] · deviation none recorded · impact hardens the verification gate and the CI static-analysis gate that every subsequent SDD change on this project runs through — items 19-22 will be written against the stricter contract rather than retrofitted to it. **No `sdd/deterministic-verification-evidence/actuals` record exists in Engram (verified 2026-08-05), so every actuals field stays `[PENDIENTE]`; per `roadmap-maker` rule, actuals are read from `sdd/{change}/actuals` and never authored here.**

### skills-validate-ondisk-gate
- **Status**: planned
- **Full detail** (objective, dependencies, and suggested entry command) is held verbatim in appendix-2 Part N; the per-requirement acceptance evidence, risk analysis, atomic order, five hard constraints, and the item-19 rationale remain in obs #2771 Parts F and G. Moved at v7 for the cap only, and because on-disk evidence shows this change archived at `openspec/changes/archive/2026-08-05-skills-validate-ondisk-gate/` with its gate live in `RenderValidateCore` (see Verified Status Drift row 5). Status and Tracking are unchanged.
- **Tracking**: estimate [PENDIENTE — next producer: `sdd-time-estimation`] · impl [PENDIENTE] · verify [PENDIENTE] · review [PENDIENTE] · fixes [PENDIENTE] · closure [PENDIENTE] · deviation none yet · impact makes manifest rule 8 machine-enforced for every subsequent change that adds a file under `skills/` — items 20 and 21 in particular land inside the gate instead of being audited after the fact

### skill-project-scope
- **Status**: planned
- **Objective**: scoped registry and per-project skill install integration (Issue #29 slice 2); tighten scope resolution for project-level registries.
- **Derived from**: `openspec/changes/skill-project-scope/` (active, unarchived); architecture boundaries + reproducible-sync requirement
- **Dependencies**: skill-lifecycle
- **Acceptance evidence**: deterministic project-scope behavior in registry operations, documented user-facing behavior.
- **Risk if done early**: partial scope handling can route manifests incorrectly, corrupt per-project rules.
- **Command**: `/sdd-new skill-project-scope` (if re-opened) or `/sdd-continue skill-project-scope`
- **Tracking**: estimate [PENDIENTE] · impl [PENDIENTE] · verify [PENDIENTE] · review [PENDIENTE] · fixes [PENDIENTE] · closure [PENDIENTE] · deviation [PENDIENTE] · impact [PENDIENTE]

### skill-manifest-gen
- **Status**: planned **(new entry — was missing from the on-disk 2026-07-08 roadmap)**
- **Objective**: `skills sync-manifest` command to regenerate `overlay.manifest` from the registry (Issue #29 slice 4).
- **Derived from**: `openspec/changes/skill-manifest-gen/` (active, unarchived); architecture `engine/skills` module
- **Dependencies**: skill-project-scope
- **Acceptance evidence**: `skills sync-manifest` regenerates a manifest matching the registry with no drift.
- **Risk if done early**: could regenerate an inconsistent manifest if project-scope semantics aren't settled first.
- **Command**: `/sdd-continue skill-manifest-gen`
- **Tracking**: estimate [PENDIENTE] · impl [PENDIENTE] · verify [PENDIENTE] · review [PENDIENTE] · fixes [PENDIENTE] · closure [PENDIENTE] · deviation [PENDIENTE] · impact [PENDIENTE]
- **Coordination note**: naming is close to but distinct from `sync-check` (this item's `sync-manifest` regenerates `overlay.manifest`; `sync-check-repo-behind-origin` extends the separate `cmd_sync_check` verdict pipeline) — no functional overlap, flagged only to avoid naming confusion during parallel work.
- **Coordination note (added v5, no status/ordering change)**: this change's `proposal.md:15,67` claims `sync-manifest` makes a drifted manifest pass validate. Verified false in the general case — `engine/skills/sync.go:19-44` `isSkillRow` regenerates only `*/SKILL.md` rows and excludes infra dirs, so it cannot clear an `UNREGISTERED_ON_DISK` for a `references/` file or a `_shared/*` file. Item 19 R-009 corrects the remediation claim; this item should reconcile its own proposal text when it starts.

### gadu-portable-operator
- **Status**: in-progress
- **Objective**: complete/stabilize dual output (agent + skill) generation contract for GADU with strict generation checks; make the persona portable/durable across gentle-ai upgrades.
- **Derived from**: `openspec/changes/gadu-portable-operator/` (active); manifest mission; existing `gadu-operator` outputs
- **Dependencies**: skill-project-scope
- **Acceptance evidence**: generated artifacts match canonical persona source and pass check mode.
- **Risk if done early**: mismatch between generated agent and portable persona causing inconsistent orchestration.
- **Command**: `/sdd-continue gadu-portable-operator`
- **Tracking**: estimate [PENDIENTE] · impl [PENDIENTE] · verify [PENDIENTE] · review [PENDIENTE] · fixes [PENDIENTE] · closure [PENDIENTE] · deviation [PENDIENTE] · impact [PENDIENTE]

### sync-check-repo-behind-origin (added in v2 — that backfill's triggering requirement)
- **Status**: planned
- **Full detail** (objective, derived-from citations, dependencies, per-requirement acceptance evidence, risk-if-early, suggested entry command, and atomic implementation order) is held verbatim in appendix-2 Part J; the item-23 rationale remains in obs #2771 Part E. Moved at v7 for the cap only, and because on-disk evidence shows this change archived (see Verified Status Drift). Status and Tracking are unchanged.
- **Tracking**: estimate [PENDIENTE — recommend running `sdd-time-estimation` before `sdd-propose`] · impl [PENDIENTE] · verify [PENDIENTE] · review [PENDIENTE] · fixes [PENDIENTE] · closure [PENDIENTE] · deviation none yet · impact none yet — first item to exercise the sync-check pipeline since its last touch in overlay-coherence-mini-fix

### overlay-versioned-releases (added in v6 — that splice's triggering requirement)
- **Status**: awaiting-archive (delivered) — **advanced at v7 from the v6 value `planned`, on verified merge evidence, with nothing overwritten**: `openspec/changes/overlay-versioned-releases/tasks.md` reads 17 checked / 0 unchecked and a `verify-report.md` is present; the four tracker PRs are merged on `main` — #165 `feat(overlay): release identity, per-target digest, and CI tag-cutting` (`8a64937`), #168 `feat(tui): surface release version, digest status, and restore action` (`c47faca`), #170 `feat(overlay): self-update converges to release tag, REPO_BEHIND_RELEASE, update check` (`fe1c100`), and docs #174 (`49ebe35`, `e73b7d4`). The change folder is still under `openspec/changes/` (NOT under `archive/`), so the correct status is `awaiting-archive (delivered)`, not `completed` — verified 2026-08-28 on the `feat/longterm-mem` worktree at `e73b7d4`.
- **Objective**: Adopt a gentle-ai-style versioned-release model for `labdrian-overlay`'s own update mechanism — semantic-version release tags, persisted per-target installed-version/digest state, an aggregate per-target content digest, an apply-required (not silently healthy) `status`/`sync-check`, a tag-converging `self-update`, a read-only `update` check, pre-apply backups with `restore`, an extended `doctor`, a `version` command, and TUI surfacing of all of it. The full v6 objective sentence is held verbatim in appendix-2 Part K.
- **Dependencies**: item 18 `deterministic-verification-evidence` and item 23 `sync-check-repo-behind-origin` — **both already satisfied** when this item started, and both now archived on disk. No dependency on items 15-22. The full three-bullet dependency analysis, the four derived-from citations, and both risk paragraphs are held verbatim in appendix-2 Part K.
- **Pre-implementation planning detail** (the R-001..R-011 acceptance evidence, the atomic implementation order, and the six hard constraints) and **the complete "why item 24 lands where it does" sequencing rationale** are held verbatim in appendix-2 Parts K and M. Moved at v7 for the cap only, and because this item is now delivered so its planning detail is settled history — mirroring how item 18 was compressed at v5.
- **Suggested SDD entry command**: `/sdd-new overlay-versioned-releases` — the brief's own suggested first phase is `sdd-explore` (confirm working assumptions A-01..A-04 against the live codebase before `sdd-propose`), and `sdd-new` runs exploration before the proposal.
- **Tracking**: estimate [PENDIENTE — pre-start estimate not recorded; entry contract revision 3 at obs #3071] · impl completed (17/17 tasks checked; PRs #165/#168/#170 merged, docs #174) · verify [PENDIENTE — `verify-report.md` present in the change folder, outcome not read into Engram] · review [PENDIENTE] · fixes [PENDIENTE] · closure [PENDIENTE — merged but not archived; change folder still under `openspec/changes/`] · deviation the human split the original Slice 3 into 3a/3b rather than accept a size exception, taking `review_slices` from 3 to 4 (obs #3071) · impact gives every subsequent SDD change a nameable release identity and a per-target installed-version/digest state file that `apply`/`status`/`sync-check`/`doctor` now read — item 25 `longterm-mem` must land its new non-file deployment artifacts additive on top of that model. **No `sdd/overlay-versioned-releases/actuals` record exists in Engram (verified 2026-08-28), so every actuals field stays `[PENDIENTE]`; per `roadmap-maker` rule, actuals are read from `sdd/{change}/actuals` and never authored here.**

### longterm-mem (NEW — this splice's triggering requirement)
- **Status**: planned
- **Objective**: Package `longterm-mem` as a standalone Go module exposing an MCP stdio server plus CLI subcommands that unify Engram (mid-term, project-scoped, read-only SQLite) and the per-project claude-obsidian vault (long-term, BM25+rerank) into one queryable, mutable, retraction-aware source of truth, deployed to all three runtimes through a new `mcp` route in `overlay.manifest`.
- **Derived from**:
  1. `project/labdrian-sdd-overlay/requirements/longterm-mem` Rev 2 — obs #3103 (Part 1/3, R-001..R-015), #3109 (Part 2/3, R-016..R-035), #3110 (Part 3/3, appendix): 35 atomic EARS requirements, `artifact_store_mode: hybrid`, `strict_tdd: true`. All three parts are required to reconstruct the brief.
  2. Stakeholder decisions obs #3102 (AMB-01 → per-project vault resolved from configuration, `~/labdrian-brain` the pre-seeded default here; cross-project querying out of scope) and obs #3106 (agent-workflow integration of `query` is an explicit Non-Goal this wave; eligibility includes `revision_count >= 3`); exploration obs #3098 (feasibility: standalone Go module, MCP stdio + CLI, no daemon) and obs #3099 (bridge probe: `engram obsidian-export` rejected as the promotion path — a disconnected dump, not an ingest; longterm-mem owns its own contract-conformant writer).
  3. Manifest rules obs #2065: rule 8 (track every skill/agent change through `overlay.manifest`) — the rule R-013's new route must extend without breaking; rule 9 (fail loud, no silent healthy) — binds R-023/R-025/R-035; rule 10 (minimal surface change) — binds R-014's build/register split. Architecture obs #2066 §3 (`bin/labdrian-overlay` dispatch table — the seam R-014 extends), §4 (`engine/skills` manifest reconciliation — the parser R-013 extends; `engine/runtime` `Adapter` — where R-014's registration call lands), §8 (no remote fetch/execution inside `engine` — the invariant R-001 keeps intact by living outside it). Pipeline state obs #3105 (Tier 2, rule #3).
- **Dependencies**:
  - item 18 `deterministic-verification-evidence` — **already satisfied** (archived at `openspec/changes/archive/2026-08-05-deterministic-verification-evidence/`): its R-017 makes `staticcheck ./...` blocking on the `engine` module, and R-013/R-035 land new Go code in `engine/skills` and the installer route handling.
  - item 19 `skills-validate-ondisk-gate` — **already satisfied** (archived at `openspec/changes/archive/2026-08-05-skills-validate-ondisk-gate/`), and the edge is concrete and file-verified: the gate is now live inside `RenderValidateCore` (`engine/skills/skills.go:129`, `:141` call `DeployableManifestPaths`/`DiffOnDisk`) and `engine/skills/ondisk.go:31-34` recognizes exactly TWO non-skill routes (`agent`, `opencode-agent`) — every other third-column value falls through to the default `skills/` route. A `longterm-mem/… mcp` row therefore counts as a deployable skills-tree path today and would fail the live CI gate with `MISSING_ON_DISK`. **R-013 must register `mcp` in THREE parsers, not the two the brief names**: `route_resolve` in `bin/labdrian-overlay`, the Go installer route handling, and `nonSkillRoutes` in `engine/skills/ondisk.go` (verified 2026-08-28).
  - item 24 `overlay-versioned-releases` — **already satisfied as delivered** (17/17 tasks; PRs #165/#168/#170 merged): it introduced the persisted per-target installed-version/digest state that `apply`/`status`/`sync-check`/`doctor` now read. R-014/R-015 add a NEW deployment artifact class — a persistent compiled binary plus three runtime MCP config entries, none of them a manifest-routed file copy — so they must land additive on that model, exactly as item 24's R-004 had to be additive on item 23's VERDICT field.
  - No dependency on items 15-17 or 20-22 — verified against the brief's Architecture Inputs table: no requirement touches `engine/gadu`, `skills.registry.yaml`, or GADU generation. The one contact point with item 21 is a content obligation, not an ordering edge (see its coordination note).
- **Evidence of acceptance** (headline proof per SDD change candidate; the complete per-requirement evidence lists live in the brief, obs #3103/#3109, and its Project-Inception Handoff table, obs #3110):
  - `longterm-mem-scaffold` (R-001, R-002, R-020, R-021, R-003, R-022, R-023, R-004, R-024): `go build ./longterm-mem/...` succeeds while `engine/go.mod` stays require-free and `go test ./engine/skills -run TestZeroFetchImportAllowlist` passes unmodified; plus read-only-connection, `deleted_at`+project-filter, no-`engram`-CLI-subprocess, vault override/default/reject, `retrieve.py --top` parse and exit-10 → `not_provisioned` tests.
  - `longterm-mem-query` (R-005, R-025, R-006, R-026): index rebuild plus first-provision test; failing-script non-zero-exit test; library-level merge test asserting source grouping, `engram_id`↔`page_address` dedupe, required-`project` rejection, and the Engram-only degrade path.
  - `longterm-mem-promotion` (R-007, R-027..R-029, R-008, R-030, R-009, R-031, R-033, R-032): eligibility predicate over all five criteria; wiki-lint pass on an emitted page carrying a contract-enum `type`, `engram_type`/`engram_id`/`project` extras, resolvable `related` wikilinks and an id-first filename; address allocate/reuse plus `.raw/.manifest.json` and `index.md`/`log.md` registration; double-promotion, retitle, local-edit-skip, three-scenario `sync`, sync-state timestamp, supersession/soft-delete and explicit-promote tests.
  - `longterm-mem-ops` (R-010, R-011, R-012, R-034): `status` under healthy/not-provisioned/never-synced; `doctor` per named check; MCP handshake listing `query` and `promote` plus a `query` round trip over stdio; process-lifecycle assertions after every subcommand.
  - `longterm-mem-overlay-integration` (R-013, R-035, R-014, R-015): an `mcp`-route fixture row parsed correctly by every parser named above, and an unrouted `longterm-mem/**` row rejected rather than resolved to `skills/longterm-mem/**`; install test proving the binary is really built, copied, then registered via `engine runtime`; persistence test at the fixed path.
  - `longterm-mem-mcp-registration` (R-016..R-019): three-scenario config-merge tests per runtime (additive, idempotent replace, untagged-conflict refusal), a byte-level TOML round-trip for codex, and per-runtime uninstall tests covering selective removal, untagged preservation and the partial-vs-full binary-removal policy.
- **Risk if done too early**: none identified — all three dependencies are delivered, so waiting buys nothing. The real risk is blast radius, not timing: per the brief's Architecture Inputs table R-027/R-008/R-030 are High (they own the brief's highest-risk write path — mutating tracked vault content, where a wrong frontmatter/type mapping reproduces the "disconnected island" defect the bridge probe rejected and a missing precedence check clobbers a human edit) and R-016..R-019 are Medium-High (they mutate three real user config files whose unrelated MCP entries must stay byte-identical). Correctness risks for `sdd-design`/`sdd-apply`, not sequencing risks.
- **Risk if done too late**: bounded and non-compounding — nothing in the roadmap degrades while this waits, and the gap it closes is a capability absence, not an active defect. The one time-sensitive edge is item 21: `sync-manifest` regenerates only `*/SKILL.md` rows, so if it lands first it is written blind to a route it must preserve.
- **Suggested SDD entry command**: `/sdd-new longterm-mem` — the brief marks `sdd-propose` as the suggested first phase per candidate, but five design decisions are still open (below) and `sdd-new` runs exploration before the proposal.
- **Chaining forecast (flagged by the brief, not resized here)**: four of six candidates are at or over the 400-changed-line review budget — `longterm-mem-promotion` (~900-1300+, strongest `chained-pr` candidate), `longterm-mem-mcp-registration` (~700-1000), `longterm-mem-scaffold` (~600-900, largely `go.sum`), `longterm-mem-overlay-integration` (~350-500, borderline). Delivery strategy is `inception-pipeline` entry-assembly's call.
- **Atomic implementation order** (per the brief's 35-row Atomic Requirement Order): R-001 → R-002 → R-020 → R-021 → R-003 → R-022 → R-023 → R-004 → R-024 → R-005 → R-025 → R-006 → R-026 → R-007 → R-027 → R-028 → R-029 → R-008 → R-030 → R-009 → R-031 → R-033 → R-032 → R-010 → R-011 → R-012 → R-034 → R-013 → R-035 → R-014 → R-015 → R-016 → R-017 → R-018 → R-019. Rows 28-31 (R-013/R-035/R-014/R-015) are parallelizable with rows 1-27.
- **Hard constraints that must survive into design and apply**:
  1. `engine/`'s zero-dependency invariant (ADR-15, `zero_fetch_test.go`) is never violated — `longterm-mem` lives outside `engine/` precisely so it may take third-party dependencies (R-001).
  2. `engine/` bans `os/exec`, so `engine runtime` can only RECORD a registration; `bin/labdrian-overlay` is what builds and copies the binary (R-014). Routing the whole install through `engine runtime` is not implementable.
  3. Never invoke the `engram search`/`save` CLI — it has no `--json`; read the SQLite database read-only instead (R-002, R-021). `engram obsidian-export` is never the promotion source of truth (#3099).
  4. No background daemon, ever — the MCP server is spawned per session and must outlive nothing (R-034).
  5. `query` takes `project` as a required argument; server-cwd inference is documented-wrong and must be rejected, not defaulted (R-006).
  6. A locally edited vault page is never silently overwritten (R-030), and a superseded or soft-deleted observation never stays canonical (R-033).
- **Open decisions carried into `sdd-explore`/`project-architect`** (the brief's Open Questions, deliberately unresolved here): where the three MCP registration writers live (`longterm-mem` module vs. `engine/runtime` — this swings R-018 between a TOML library and a hand-rolled section-preserving editor); the fixed persistent-binary install path (R-015); the vault-registry config format/location (R-003/R-022/R-023); the local-edit-precedence hash-storage mechanism (R-030); and whether R-033's status update also respects R-030's precedence rule.
- **Tracking**: estimate [PENDIENTE — next producer: `sdd-time-estimation`] · impl [PENDIENTE] · verify [PENDIENTE] · review [PENDIENTE] · fixes [PENDIENTE] · closure [PENDIENTE] · deviation none yet · impact adds a second, non-skill deployment artifact class (a persistent binary plus per-runtime MCP registrations) to the overlay's install surface, which every later item touching `overlay.manifest` routing, `bin/labdrian-overlay` dispatch, or item 24's per-target digest/state model must account for

## Sequencing Rationale for items 14, 18, 19 and 23 (pointers)

The four v6 pointer sections that summarized these rationales were moved verbatim into appendix-2 Part L at v7, for the 50000-character cap only. The rationales themselves are unchanged and remain in obs #2771: Part D (why item 18 lands where it does — five evidence-backed points), Part E (why items 23 and 14 land where they do, plus the item-23 data-integrity note), Part G (why item 19 lands where it does — six evidence-backed points). Every position they argue for is unchanged, and item 18's and item 19's arguments both held: both are now archived on disk. Nothing was dropped or downgraded.

## Sequencing Rationale (why item 24 lands where it does)

Held verbatim in appendix-2 Part M — four evidence-backed points (it depends on items 18 and 23, both already satisfied; it is orthogonal to the skill-registry/GADU chain, verified per-requirement against its own Architecture Inputs table; placing it after item 23 keeps the two `cmd_sync_check`-touching items in the order they actually landed; appending at the end invents no ordering constraint and renumbers nothing). Moved at v7 for the 50000-character cap only. The position it argues for is unchanged, and the argument held — item 24 is now delivered, ahead of every item it was placed after.

## Sequencing Rationale (why item 25 lands where it does)

`longterm-mem` is placed **last in the sequence, immediately after item 24 (`overlay-versioned-releases`) and outside the skill-registry/GADU chain (items 15-22)** — derived from real, file-verified dependencies, not arrival order:

1. **All three dependencies are delivered, so the position imposes no wait.** Item 18 (archived) supplies the blocking `staticcheck ./...` gate on `engine` that R-013/R-035's Go code is written against; item 19 (archived) supplies the live manifest/on-disk cross-check; item 24 (17/17 tasks, PRs #165/#168/#170 merged) supplies the per-target installed-version/digest state model. Every edge is satisfied on `main` at `e73b7d4`.

2. **It cannot precede item 19 — item 19's gate would reject its very first manifest row.** The strongest edge, and concrete rather than thematic. `RenderValidateCore` now calls `DeployableManifestPaths` and `DiffOnDisk` (`engine/skills/skills.go:129`, `:141`), and `nonSkillRoutes` (`engine/skills/ondisk.go:31-34`) contains exactly `agent` and `opencode-agent`; the surrounding comment states any other third-column value "falls through to the default skill route". A `longterm-mem/… mcp` row would therefore be treated as a deployable `skills/` path and reported `MISSING_ON_DISK` by a gate that runs in CI. Had item 19 not landed first, R-013 would have been specified against two parsers instead of three and the defect would have surfaced only in CI.

3. **It must build additive on item 24's state model, which exists only because item 24 delivered.** R-015 puts a persistent compiled binary at a fixed path and R-016/R-017/R-018 write entries into three runtime config files — neither is a manifest-routed file copy, so neither is covered by item 24's per-target content digest as written. Sequencing after item 24 keeps the causal order visible and forces the design to extend that model rather than fork a parallel one, exactly as item 24's own R-004 had to extend item 23's `REPO_BEHIND_ORIGIN` field.

4. **It is orthogonal to items 15-17 and 20-22 — verified, not assumed.** All 35 requirements route through the new `longterm-mem/` module, `bin/labdrian-overlay`'s dispatch table, `overlay.manifest` routing, `engine/skills`'s route parser, or the three runtime config files; none touches `engine/gadu`, `skills.registry.yaml`, or GADU generation. The single contact point is item 21 `skill-manifest-gen`, whose `sync-manifest` regenerates only `*/SKILL.md` rows and would drop `mcp` rows — a content obligation on whichever item starts second (recorded as a coordination note on item 21), not an ordering edge, since neither needs the other's output. Same distinction item 19's rationale drew for R-034.

5. **Appending at position 25 invents no ordering constraint and renumbers nothing.** Nothing earlier supplies the `staticcheck` gate, the on-disk route gate, or the per-target digest model this item builds on, and nothing after item 24 changes that. Position 25 is the only position with zero invented dependencies and zero disturbance to the 24 pre-existing items.

## Unscheduled — Pending Decision (NOT a sequenced item, NOT committed)

`visual-determinism-runner` — browser-mode visual runner (computed styles, axe-core, geometry, per-breakpoint screenshots). Status `[PENDIENTE DE DECISIÓN]`: unscheduled, no target repository decided, no Order number, no dependency edge, no SDD entry command. MUST NOT be treated as a planned roadmap item until the ownership question is resolved by the user and, if it lands here, an architecture amendment is authored by `project-architect`. **The complete v4 entry — why it is not sequenced, the verified consumer-disjointness finding, the three preserved findings, and the blocking open question — is held verbatim in the companion observation at topic key `project/labdrian-sdd-overlay/roadmap/appendix`, Part B.** It was moved there at v5 solely because this observation reached Engram's 50000-character cap; nothing was dropped or downgraded.

## Verified Status Drift (surfaced 2026-07-31; re-verified 2026-08-05 and 2026-08-28, NOT corrected in this splice)

Seven discrepancies between this roadmap and on-disk archive evidence are now verified. Rows 1-3 were found in the v4 splice and re-verified in v5, v6 and v7; rows 4-7 are NEW at v7. All remain uncorrected, because each run's only authorized status write is its own new item (plus, at v7, item 24's advance on verified merge evidence). Each is recorded here with its evidence so the next roadmap-maker pass can correct all seven in one authorized update.

| # | Item | Roadmap says | On-disk evidence | Correct status |
|---|---|---|---|---|
| 1 | Item 14 `actuals-calendar-checkpoint-instrumentation` | `planned` | `openspec/changes/archive/2026-07-30-actuals-calendar-checkpoint-instrumentation/` (confirmed present 2026-08-05); closure record obs #2609 (`complete: true`, spec promoted to `openspec/specs/actuals-instrumentation/spec.md`, three approved review receipts with 0 blocking findings) | completed |
| 2 | Item 23 `sync-check-repo-behind-origin` | `planned` | `openspec/changes/archive/2026-07-11-sync-check-repo-behind-origin/` (confirmed present 2026-08-05); actuals record obs #2096 | completed |
| 3 | `restore-skill-registry-scoped-blocks` | absent from the roadmap entirely | `openspec/changes/archive/2026-07-21-restore-skill-registry-scoped-blocks/` (confirmed present 2026-08-05) | completed — needs a foundational backfill entry |
| 4 | Item 18 `deterministic-verification-evidence` | `awaiting-archive (delivered)` | `openspec/changes/archive/2026-08-05-deterministic-verification-evidence/` (confirmed present 2026-08-28) | completed |
| 5 | Item 19 `skills-validate-ondisk-gate` | `planned` | `openspec/changes/archive/2026-08-05-skills-validate-ondisk-gate/` (2026-08-28); gate live at `engine/skills/skills.go:129,141` | completed |
| 6 | `sdd-cycle-timestamp-instrumentation` | absent from the roadmap | `openspec/changes/archive/2026-08-19-sdd-cycle-timestamp-instrumentation/` (2026-08-28) | completed — needs a backfill entry |
| 7 | `tui-self-update-offer` | absent from the roadmap | `openspec/changes/archive/2026-08-24-tui-self-update-offer/` (2026-08-28); merged PR #150 | completed — needs a backfill entry |

**Recommended follow-up** (one authorized roadmap-maker pass, not an SDD change): correct drifts 1, 2, 4 and 5 to `completed`, backfill drifts 3, 6 and 7 as foundational completed items, and populate every corrected tracking line from its `sdd/{change}/actuals` record per `roadmap-maker` step 8. Drift 2 was flagged in the v3 splice (2026-07-29) and has persisted through four versions. That pass should also refresh item 24's now-spent `Suggested SDD entry command` (`/sdd-new overlay-versioned-releases`), which v7 left untouched because its only authorized writes on that item were Status and Tracking. Note: neither `sdd/actuals-calendar-checkpoint-instrumentation/actuals` (obs #2686) nor `sdd/overlay-versioned-releases/actuals` (verified 2026-08-28) exists in Engram, so those lines must stay `[PENDIENTE]`, not be invented.

**On-disk mirror (updated at v7)**: the v5 note recorded `openspec/project/roadmap.md` as dated 2026-07-21 and far behind. It was **regenerated from this v7 content by the v7 pass**. Engram `project/labdrian-sdd-overlay/roadmap` plus its two appendix volumes remains canonical; the on-disk file is a mirror every future splice must regenerate.

## Known Roadmap Inconsistencies (pre-existing, carried unchanged)

> **Truncation discovered at v6**: this section's v5 copy was found cut off mid-word ("...item 20 `skill-pr") when this splice loaded it, consistent with hitting the same 50000-character save cap that already forced the v5 appendix split. The bullet below is freshly re-derived from the live Sequence Summary's own dependency data (item 22 depends on item 20, still `planned`) — a re-derivation from current evidence, not a recovery of the lost bytes. Any additional original content beyond this is presumed lost and is not invented here.

- Item 22 `gadu-portable-operator` is `in-progress` while its declared dependency, item 20 `skill-project-scope`, is still `planned` (not yet started) — a pre-existing ordering inconsistency carried unchanged from v4/v5, not introduced or corrected by this splice.
