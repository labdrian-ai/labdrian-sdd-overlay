# ODD Task — Unify roadmap tasks and remove narrative duplication

## Objective
Create one authoritative project roadmap view that consolidates active work from the standalone phase roadmap, the product planning roadmap, and relevant historical SDD roadmap items; deduplicate identical outcomes while preserving distinct scopes, provenance, status evidence, and unresolved mappings.

## Why
The repository currently presents multiple numbered sequences (standalone Phases 0–10, product packages P0–P8, and historical SDD item numbers) plus execution ledgers. Similar outcomes appear in different narratives, making it unclear whether they are duplicate work, broader parent work, prerequisites, or historical evidence.

## Authorized scope
- Reconcile roadmap/task documents and relevant active OpenSpec task ledgers only; no implementation source, runtime, remote, branch, commit, PR, or delivery changes.
- Establish one active roadmap authority and turn competing planning narratives into references/crosswalks rather than maintaining duplicate active task lists.
- Preserve all task IDs/source citations and historical SDD records; do not delete completion evidence or fabricate status/dependencies.
- Preserve the dirty worktree and current branch exactly, including the existing procedural-memory lifecycle changes and all unrelated untracked paths.

## Inputs to reconcile
- `openspec/decisions/standalone-platform-roadmap.md` — canonical phases 0–10.
- `docs/architecture/standalone-product-roadmap.md` — product packages P0–P8.
- `docs/architecture/standalone-product-start-here.md` — recovery guide and active-work pointers.
- `openspec/project/roadmap.md` — generated mirror for historical SDD sequencing; on disk v7, while Engram observation #2067 records v11 and eight appendix volumes.
- `odd/tasks/standalone-platform-roadmap.md` — roadmap-artifact creation ledger.
- `odd/tasks/standalone-goal-contract.md` — Phase 1 Goal execution ledger, recorded complete.
- `odd/tasks/workflow-profile-contract.md` — Phase 2 execution ledger, still active.
- Relevant active OpenSpec tasks discovered by mapping: `runtime-roster`, `skill-project-scope`, `skill-manifest-gen`, `skill-lifecycle`, `skill-package-manager`, `longterm-mem-knowledge-ingestion`, `overlay-versioned-releases`; add others only with concrete scope overlap.
- Relevant planned historical SDD work from Engram #2067: procedural candidate/drafting/promotion/observability/runtime-projection/retirement items; validate current status without changing their historic evidence.

## Route and edit surfaces
- Route: delegated direct. Mapping required 4+ source documents, and consolidation changes multiple non-trivial planning artifacts; one bounded writer owns roadmap updates after this ledger is established.
- Parent-owned task ledger: this file and its complete Engram mirror at `odd/unified-roadmap-reconciliation/tasks`.
- Proposed canonical active roadmap: `openspec/decisions/standalone-platform-roadmap.md` (preserve the stable phase IDs and add an explicit crosswalk/integrated work-item registry; do not claim phase/package equivalence without evidence).
- Supporting plan and start-here guide are reduced to an unambiguous master pointer plus unique rationale/acceptance. Historical SDD mirror stays a history/provenance source, not a parallel standalone plan; its generated file was intentionally not edited.
- Execution task ledgers remain detailed progress records linked to one canonical roadmap item; they do not become additional roadmap authorities.

## Consolidation rules
- One canonical task identity for the same observable deliverable; keep source aliases and provenance as cross-references, not separate active work items.
- A broader package and a narrower phase are not duplicates when the broader acceptance includes additional outcomes; represent the shared subtask once and retain remaining acceptance separately.
- Similar domain or vocabulary is not sufficient for deduplication. Keep distinct outcomes (e.g., overlay release management versus standalone product release acceptance) distinct and connect them with a dependency only when supported.
- Preserve IDs/numbers as legacy aliases; do not renumber source histories.
- Never merge compatibility-mode names (`standalone`, `Gentle-compatible`) into workflow profiles (`odd`, `sdd`, `standalone-minimal`, `maintenance`, `incident-recovery`) without evidence. Record this as an unresolved crosswalk.
- Do not equate P0–P8 with Phases 0–10 by ordinal position. Use explicit mapping only where requirements and acceptance prove it.
- Keep historical SDD completed items as foundations/evidence. Import only relevant still-open work into the active master, without asserting archival status from stale copies.
- Keep task-level status, checks, acceptance criteria, and evidence in execution ledgers; the master records canonical objective, scope, dependencies, status summary, acceptance, source IDs, and next action.

## Acceptance criteria
- A source inventory covers every roadmap-bearing document and each current execution ledger with concrete overlap.
- The master contains one canonical active task per identical deliverable, with all duplicate narrative labels/source locations listed as aliases.
- Broader/partial overlaps are split into shared deliverable plus distinct remaining acceptance, with no lost requirement.
- Authority, status, numbering, compatibility vocabulary, and history boundaries are explicit.
- Every imported task status/dependency is backed by current on-disk or persisted evidence; uncertain values remain explicitly unresolved.
- Non-canonical planning documents and start-here text point to the master and do not present competing task sequences.
- Existing task ledgers are preserved and cross-linked; no unrelated files are modified.
- Read back all changed documents, check links/IDs and `git diff --check`; report any status claims not independently verifiable.

## Progress
- [x] Confirmed current branch/worktree and preserved pre-existing dirty state.
- [x] Mapped initial authority/numbering conflict and retrieved historical SDD roadmap evidence (#2067 and appendices).
- [x] Delegated a read-only overlap map; it identified clear duplicate (Goal ledger ↔ Phase 1) and same-outcome tracking duplication (Workflow Phase 2 ledger ↔ Phase 2 definition), plus partial overlaps and unresolved crosswalks.
- [x] Created this ODD tracker and persisted its Engram mirror before roadmap edits.
- [x] Complete the source-backed overlap inventory across the standalone phase roadmap, P0–P8 source plan, start-here guide, ODD ledgers, relevant active task ledgers, local procedural-memory archive, and historical SDD evidence. Engram v11 appendices were not independently available; those IDs/statuses remain explicitly unmapped.
- [x] Produce the canonical master in `openspec/decisions/standalone-platform-roadmap.md`, deduplicating Goal/Phase 1 and Workflow Profile/Phase 2 identities while preserving partial overlaps as linked distinct work.
- [x] Recast `docs/architecture/standalone-product-roadmap.md` as rationale/source acceptance, update the start-here guide, and link the Goal/Profile execution ledgers to canonical items. The generated historical SDD mirror remains untouched and is labeled history/reference in the master.
- [x] Read back the changed documents; checked relative links, whitespace, numbering/aliases, and preserved dirty status. Since `git diff --check` omits untracked docs, a direct per-file whitespace check was used.

## Known questions to preserve, not silently decide
- Relationship of the two compatibility modes to the five workflow profiles.
- Exact crosswalk between product packages P0–P8 and standalone phases 0–10.
- Whether each planned historical SDD procedural-memory item is already satisfied by newer standalone/procedural work or remains an independent requirement.
- Historical roadmap Engram v11 versus on-disk generated mirror v7 status discrepancies.

## Verification and delivery
- No implementation, runtime, remote operation, branch change, commit, PR, or merge was performed.
- Parent read back all five source artifacts. A direct check over the five changed documents passed for trailing whitespace, resolvable relative Markdown links, and whitespace errors; `git diff --check` alone would omit these untracked files.
- No code tests apply to this documentation-only reconciliation. Existing unrelated dirty status was preserved.
