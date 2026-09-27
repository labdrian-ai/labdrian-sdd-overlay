# Resume the independent Labdrian product work

## Read first

Read [Standalone Platform Master Roadmap](../../openspec/decisions/standalone-platform-roadmap.md) first. It is the **sole active roadmap authority** for standalone product work: stable Phase 0–10 IDs, canonical status, source crosswalk, and next action. Then read this supporting [product rationale and source crosswalk](standalone-product-roadmap.md) for detailed package acceptance, non-goals, architecture rationale, and open decisions.

The P0–P8 labels in the supporting document are historical source aliases, not a parallel active sequence. `openspec/project/roadmap.md` is a generated historical SDD mirror (on disk v7); the supplied canonical Engram locator is #2067 v11 with eight appendices. Neither is a competing standalone-product roadmap. Use current evidence; old planning statuses are not current truth.

## Fresh-session prompt

Copy this into a session opened at the labdrian-sdd-overlay repository root:

> Read docs/architecture/standalone-product-start-here.md and openspec/decisions/standalone-platform-roadmap.md first; then consult docs/architecture/standalone-product-roadmap.md for supporting rationale and detailed package acceptance. Resume the next evidence-backed canonical work item from the master; reconcile actual repository state, active work, historical SDD sources, and dependencies before changing the roadmap. Labdrian must remain a platform above existing runtimes, not a new agent execution runtime. Gentle AI and gentle-pi are optional. Laya is the first optional specialized decision backend, shadow-first; do not substitute a generic classifier as the product direction. Preserve longterm-mem and existing memory workflows. Identify the first-runtime recommendation from evidence and report open decisions. Do not implement, install, download models, migrate configuration, commit, push, or initiate SDD execution without further explicit authorization.

## Recovery sequence

1. Confirm the Git root, branch and current status. Read local instructions before task work. Do not switch branches, stash or clean someone else's work.
2. Read `openspec/decisions/standalone-platform-roadmap.md` in full, including its work register, source crosswalk, open decisions, and evidence limits. The product rationale is supporting detail, not a second schedule.
3. Read docs/architecture/laya-system-one-overlay-plan.md only as earlier context. Do not use its historical feature-completion claims as current evidence.
4. Read the archived `openspec/changes/archive/2026-09-21-procedural-memory-lifecycle/` evidence and reconcile `openspec/changes/longterm-mem-knowledge-ingestion/` with its current artifacts. The archive report records verification as PARTIAL; preserve the exact limitations and all execution evidence. Earlier planning recorded a different dirty state, so never assume that snapshot is current. Compare historical Engram #2067 v11/eight appendices against the on-disk v7 mirror without editing the generated mirror.
5. Follow repository structural exploration policy, including CodeGraph where applicable. Delegate bounded mapping from this repository rather than trying to register it as a worktree of another Git clone. Preserve the owner's preference for no grep-based exploration.
6. Produce P0's dependency/license inventory and a first-runtime recommendation. Find actual Gentle/gentle-pi dependencies in install, skills, dispatcher, subagents, resume, review, archive and uninstall paths.
7. Resolve only decisions needed for the next authorized action. Do not ask again whether to build a runtime: the user already chose a platform over existing runtimes.
8. If implementation is later authorized, select the smallest applicable workflow, resolve test/TDD configuration, and record actual results per work package. SDD is not automatically selected by this roadmap.

## Settled choices (product direction; implementation evidence remains separate)

- Own product, contracts and distribution; Gentle compatibility optional.
- Reuse existing runtimes; do not build an agent loop or universal model gateway.
- Laya first, optional local Python adapter; proposed persistent JSONL transport still needs validation.
- Specialized decision models are the strategic goal. Future Jev integration is unverified, not committed.
- Models advise; actual human/runtime authority controls execution.
- longterm-mem remains a separate memory component. No inference or direct Laya writes inside it.
- Model weights require explicit setup. No implicit downloads during ordinary startup/apply/sync.
- No immediate broad rename or fork; extract dependencies incrementally and preserve licenses/provenance.

## Completion test for independence

In a clean supported environment with neither gentle-ai nor gentle-pi installed, a user can install Labdrian, connect a supported runtime, complete and resume its minimal workflow, use configured memory, and update/rollback/uninstall without losing user data. Laya can be disabled or unavailable without breaking that journey. A second runtime validates portability before broad support claims.

This does not mean zero third-party dependencies or guaranteed commercial success. Runtime capabilities, licenses, performance and usability require evidence.

## Roadmap authority, persistence, and limits

These Markdown files are the primary handoff and require no Engram access. They are local/uncommitted: a new session in this same checkout can read them; a different clone cannot until the owner transfers or commits them.

The planning conversation ran in test-decitions-model. Prior conversation memories may therefore be scoped there, not to this repository. Do not conclude the plan is missing from an empty target-project memory search, and never use memory to override newer repository evidence.

The previous assistant's broad architectural assessment was not an exhaustive audit. Delegated planning failed because of cross-clone worktree registration. P0 explicitly closes that evidence gap before implementation.
