# Design: minimalism-contract-lite

> Phase order note: this design runs BEFORE spec by deliberate manual override of the
> dispatcher's parallel recommendation. The CRUX of this change is a MECHANISM (how the
> contract is scoped to two phases), and the spec's requirement shapes depend on the
> mechanism decided here. Design first, then spec the observable requirements around it.

## Context

The overlay duplicates anti-inflation prose across several skills. This change consolidates
it into ONE referenced contract (`skills/_shared/minimalism-contract.md`) and injects that
contract ONLY into `sdd-tasks` and `sdd-apply` sub-agent prompts. Everything in this change
is Markdown — there is no production code, no hook, no instrumentation. Enforcement is
instruction-only and therefore probabilistic (see proposal "Honest enforcement note").

The hard problem is NOT writing the ladder. It is **scoping** the ladder to exactly two
phases without leaking it into the other five (`propose`, `spec`, `design`, `verify`,
`archive`), and without re-creating the global-contamination anti-pattern that the
`.atl/skill-registry.md` BEGIN/END blocks already suffer from.

### Two planes that must not collide

There are two independent planes by which a contract file can reach a sub-agent:

1. **Skill-content reference plane** — a SKILL.md file *mentions* a path in its prose (e.g.
   project-architect citing the contract). This is documentation. It does NOT cause the
   referenced file to be loaded into any sub-agent's working instruction set.
2. **Runtime injection plane** — the orchestrator resolves skill paths from the registry by
   code+task context and writes them into a sub-agent prompt under `## Skills to load before
   work`. This IS a load instruction: the sub-agent reads the full file before working.

The whole design hinges on keeping these planes separate. The contract lives on plane 2 for
tasks/apply, and on plane 1 (citation only) for project-architect. They must never cross.

## Goals / Non-Goals

**Goals**
- Per-phase scoping (best-effort / probabilistic): contract reaches `sdd-tasks` + `sdd-apply`
  prompts only, bound via the overlay-tracked registry Trigger row.
- A minimal, single-source contract file (the 6-rung ladder + tiebreaker + comment rule) with
  an advisory `applies_to_phases` frontmatter header documenting scope intent.
- project-architect references the contract as documentation, not as a loaded ladder.
- An observational check for AC-1 (inspect injected blocks across all phases; report WARNING).
- Track + deploy the new file via the overlay toolchain.

**Non-Goals** (inherited from proposal)
- No deterministic hooks, no LOC instrumentation, no verify gate in this slice.
- No `.atl/skill-registry.md` BEGIN/END constraint block for this contract (global
  contamination); scoping uses a phase-scoped Trigger row instead.
- No edit to `~/.claude/CLAUDE.md` (global orchestrator file, not overlay-tracked) — scoping
  must NOT be hardcoded there.
- No edit to `sdd-phase-common.md` (runtime-owned, not editable here).
- No deterministic frontmatter-parsing injection hook (that is the deferred option (b)).

## Decision 1 — Scoping mechanism (the CRUX)

### How scoping actually works in this repo today

The orchestrator's skill-resolver protocol (CLAUDE.md → "Sub-Agent Launch Pattern") matches
registry skills to a sub-agent launch by TWO axes:

- **code context** — file extensions/paths the sub-agent will touch.
- **task context** — what the sub-agent will DO (review, PR, test, author, etc.).

The resolver reads `.atl/skill-registry.md` (the overlay-tracked registry), matches skills by
those axes using each row's **Trigger** description, and copies the matched `SKILL.md` paths
into the launch prompt under `## Skills to load before work`. This is exactly how
`genesis-design-system` reaches only frontend-touching phases: it is matched by the Trigger
text in its registry row, NOT by a hardcoded rule baked into the orchestrator's instruction
file. **The registry is the authoritative binding surface the resolver actually consumes;**
the trigger description is where per-phase selectivity is expressed in prose.

The critical consequence: scoping must be expressed in a place the resolver READS, and the
resolver reads `.atl/skill-registry.md` — NOT `~/.claude/CLAUDE.md` per-contract rules, and
NOT a frontmatter field on the contract file (the resolver does not parse contract
frontmatter). `~/.claude/CLAUDE.md` is the global orchestrator instruction file and is
**out of bounds for this change** (see "Decision boundary" below).

### Options evaluated

**(a) Hardcoded phase→contract association in `~/.claude/CLAUDE.md`.**
Add an explicit per-contract rule to the global orchestrator instructions: "when launching
`sdd-tasks` or `sdd-apply`, inject `skills/_shared/minimalism-contract.md`; do NOT inject for
the other five phases."
- Pros: deterministic at the injection point in prose.
- Cons: it edits the **global** `~/.claude/CLAUDE.md`, which is shared across every project
  and not tracked by this overlay. A per-contract hardcode there is unscoped global
  contamination of the user's machine, does not travel with the overlay, and the
  "genesis-design-system scopes via CLAUDE.md" justification is FALSE — genesis is scoped via
  its **registry trigger**, not via CLAUDE.md. **Rejected.** `~/.claude/CLAUDE.md` MUST NOT be
  touched by this change.

**(b) Per-contract metadata flag a custom resolver parses.** Add a machine-readable field
(e.g. `phases: [tasks, apply]`) and teach the resolver to read it deterministically.
- Pros: declarative, self-documenting, would make scoping deterministic.
- Cons: the current resolver does NOT parse such a field; this needs a net-new pre-launch
  injection hook that reads `applies_to_phases` and filters per phase. Inventing that hook is
  net-new mechanism for a Markdown-only, Tier-3 slim slice — it over-builds (violates rung 1,
  YAGNI, of the very ladder we ship). **Deferred** to a future slice. (Note: this option is
  what a *deterministic* version of scoping would require; see "Honesty" below.)

**(c) Registry trigger description scoped to two phases (the path the resolver consumes).**
Regenerate the `minimalism-contract` row in the overlay-tracked `.atl/skill-registry.md` via
the **skill-registry skill's marker format** (NOT hand-edited freehand), giving it a Trigger
description that scopes injection: "Inject ONLY into sdd-tasks and sdd-apply sub-agent prompts
under '## Skills to load before work'. Do NOT inject into propose/spec/design/verify/archive."
- Pros: git-tracked in the overlay repo (durable via repo history, not deployed by `overlay apply`);
  uses the same proven path as genesis-design-system; no global contamination; no net-new hook.
  Regenerated through the skill-registry skill so the marker format stays consistent and idempotent.
- Cons: the trigger is **prose** the resolver matches against, so scoping is **probabilistic /
  advisory**, not deterministic — the resolver can in principle over-match. AC-1 must be a
  recurring observational guard, not a tooling guarantee.

**(d) Advisory frontmatter header on the contract file ("applies only in tasks/apply").**
Put a frontmatter block at the top of the contract self-declaring its scope.
- Pros: documents intent in the artifact that travels via the overlay; visible to any human or
  agent that opens the file.
- Cons: the resolver does NOT parse this frontmatter, so on its own it produces NO real
  scoping — it is the **fragile self-declared-scope anti-pattern**. MUST NOT be the sole
  mechanism. Useful only as documentation paired with (c).

### Decision — hybrid (c) + (d)

Scoping is achieved by a **hybrid of two complementary parts**, neither of which is
`~/.claude/CLAUDE.md`:

- **(B) The REAL binding — option (c):** the overlay-tracked `.atl/skill-registry.md`
  `minimalism-contract` row, **regenerated via the skill-registry skill's marker format**,
  carries a Trigger description scoping injection to `sdd-tasks` / `sdd-apply` ONLY ("Inject
  ONLY into sdd-tasks and sdd-apply sub-agent prompts under '## Skills to load before work'.
  Do NOT inject into propose/spec/design/verify/archive."). This is the path the orchestrator
  resolver actually consumes — the same path `genesis-design-system` uses. This is the
  load-bearing mechanism.

- **(A) Advisory documentation — option (d):** a frontmatter header in
  `skills/_shared/minimalism-contract.md` self-declaring its scope:

  ```yaml
  ---
  applies_to_phases: [sdd-tasks, sdd-apply]
  excluded_phases: [sdd-propose, sdd-spec, sdd-design, sdd-verify, sdd-archive]
  injection_point: "## Skills to load before work"
  ---
  ```

  This is **ADVISORY ONLY**: the orchestrator skill-resolver does NOT parse it. It documents
  AC-1 intent inside the same artifact that travels via the overlay, so the intent is legible
  even when the file is read in isolation. It is documentation, not the mechanism.

### Decision boundary — `~/.claude/CLAUDE.md` is NOT touched

This change MUST NOT edit the global `~/.claude/CLAUDE.md`. The earlier draft of this design
proposed a hardcoded phase→contract rule in CLAUDE.md / orchestrator skill-resolution; that is
**WRONG and is replaced**. The "genesis-design-system scopes via CLAUDE.md" justification is
FALSE — genesis scopes via its registry trigger. All scoping for this contract lives in the
overlay-tracked registry row (B) plus the advisory frontmatter (A). No global file is mutated.

### Honesty — scoping is probabilistic, not deterministic

Scoping here stays **PROBABILISTIC** (advisory prose in the registry trigger that the resolver
matches against), NOT deterministic. A deterministic version would require a pre-launch
injection hook that parses `applies_to_phases` and filters per phase — that is option (b), and
it is **OUT OF SCOPE** for this lite slice. Consequently AC-1 is verified by **OBSERVATION
(positive + negative)** and reported as a **WARNING**, never as a guarantee (see "Verifying
AC-1" below).

**Propagation caveat:** `.atl/skill-registry.md` is per-project, not deployed by `overlay apply`
(only `skills/**` is). Consumer projects (genesis, kadia) regenerate their own registry and will
receive this contract WITHOUT the scoped Trigger row unless their regeneration carries it — the
`applies_to_phases` frontmatter is advisory and no generator parses it. Cross-project scoped
propagation is OUT OF SCOPE for this lite slice (deferred with option (b)). AC-1 is asserted
for the overlay repo's own SDD runs only.

**Option (b) is explicitly deferred.** If AC-1 drifts repeatedly in practice, a future slice
may introduce the parsing hook, but building it now violates rung 1 (YAGNI) of the very ladder
we ship.

### Why not the registry BEGIN/END constraint block

The proposal already rules this out: registry constraint blocks
(`project-architect-constraints`, `project-manifest-rules`) carry no per-phase scoping and
propagate to ALL sub-agents. The contract is therefore NOT registered as a constraint
BEGIN/END block. Its binding is instead a **scoped Trigger row** in the registry's skill index
(part B above) — discoverable AND phase-scoped by the trigger description — not an
all-phases constraint block.

## Decision 2 — Citation vs load boundary for project-architect

project-architect's lines 164-166 (the ad-hoc "Do NOT inflate" prose) are replaced by a
single reference to `../_shared/minimalism-contract.md`. This reference lives on the
**skill-content reference plane** (plane 1): it is a documentation pointer in the SKILL.md
prose. It does the following and ONLY the following:

- Tells a human reader / auditor where the canonical anti-inflation source lives.
- Satisfies deduplication (the prose is no longer restated in project-architect).

It does NOT, and must not, do any of these:

- It is NOT an entry in project-architect's `## Skills to load before work` set.
- It does NOT cause the 6-rung ladder to be loaded or applied during the design phase.
- It does NOT change which phases the orchestrator injects the contract into.

This is the Part A.1 hardening (R-004) expressed at the design level: the reference is
**citational, not a load instruction**. The architect cites the contract the way a footnote
cites a source — the reader is not obligated to load and execute the source during design.

### Confirming the two planes do not collide

- project-architect runs (when invoked) as its own skill; the orchestrator does NOT inject
  the minimalism contract into it (Decision 1 excludes all non-tasks/apply phases).
- The design phase (`sdd-design`) is likewise excluded by Decision 1.
- So even though project-architect's TEXT names the contract path, neither project-architect
  nor sdd-design ever receives the contract under `## Skills to load before work`. Plane 1
  (citation in architect prose) and plane 2 (runtime injection into tasks/apply) reference
  the same file but never both fire for the same phase. No collision.

The refactored project-architect text should make this explicit, e.g.: "Anti-inflation
guidance is consolidated in `../_shared/minimalism-contract.md` (canonical source). This is a
documentation reference for deduplication; the 6-rung ladder is applied only during
sdd-tasks/sdd-apply, NOT during architecture/design."

## Decision 3 — Contract file shape

`skills/_shared/minimalism-contract.md` must itself be minimal (it is a minimalism contract;
inflating it would be self-refuting). Target structure, kept tight:

```
---
applies_to_phases: [sdd-tasks, sdd-apply]
excluded_phases: [sdd-propose, sdd-spec, sdd-design, sdd-verify, sdd-archive]
injection_point: "## Skills to load before work"
---
# Minimalism Contract

## Preference ladder (climb only when the lower rung cannot satisfy the requirement)
1. YAGNI — do not build it if not required now.
2. stdlib / language built-ins.
3. native platform feature.
4. existing dependency already in the project.
5. one-liner / minimal local code.
6. custom code / new abstraction (last resort).

## Architectural tiebreaker (mandatory)
Minimalism operates WITHIN design boundaries. A boundary mandated by the architecture is
NEVER collapsed merely to save lines. Code economy never overrides a deliberate seam.

## Comment convention (three states — R-005 expanded from proposal's single-state during spec authoring)
When rung 6 (custom code) is chosen, three cases apply based on whether a lower rung was
rejected and whether the constraint is obvious:

- **State 1 (judgment):** lower rung existed but was rejected — emit `// minimal: <reason>`
  naming the lower rung considered and why it was insufficient.
- **State 2 (forced, non-obvious):** no lower rung applicable AND constraint is not obvious
  from context — emit `// minimal: forced — <design/constraint ref>`.
- **State 3 (no comment):** no lower rung applicable AND constraint IS obvious from context
  — omit the comment entirely (YAGNI applied to the comment itself; noise-free).

Use the host language's single-line comment prefix (`//`, `#`, etc.).
```

> **R-005 spec-evolution note:** The proposal's R-005 stated a single-state rule
> (`// minimal: <reason>` always). During spec authoring the rule was expanded to three
> states. Rationale: a blanket "always comment" rule generates noise when the constraint is
> self-evident (State 3 → no comment) or when no lower rung was available (States 2 vs 3
> distinguish forced-non-obvious from forced-obvious). The contract file reflects the
> three-state spec; this note closes the traceability gap between the proposal and the spec.

Notes:
- The frontmatter header is the **advisory** scope annotation from Decision 1 (option (d) /
  part A) — it documents AC-1 intent in the artifact but is NOT parsed by the resolver and is
  NOT the scoping mechanism. The load-bearing binding is the scoped registry Trigger row
  (Decision 1, part B).
- Content is full-text on purpose — R-006 requires injection by path / full read, never as a
  pre-digested summary. The file must be small enough that full injection is cheap.
- No `// debt:` convention, no ledger, no extra rungs — out of scope per proposal.

## Decision 4 — overlay.manifest registration and deployment

`overlay.manifest` is a flat list of `path  classification` pairs (e.g.
`_shared/pre-sdd-contracts.md custom`). The new contract is overlay-original (no vendor
counterpart), so it is classified **custom**:

```
_shared/minimalism-contract.md custom
```

- Add this line to `overlay.manifest` so the file is tracked by the overlay toolchain.
- `overlay apply` deploys tracked files; once registered as `custom`, the contract is carried
  along with the other `_shared/*` custom artifacts (alongside `pre-sdd-contracts.md`).
- Regenerate the `minimalism-contract` row in `.atl/skill-registry.md` via the **skill-registry
  skill's marker format** (NOT hand-edited freehand) with the **phase-scoped Trigger
  description** from Decision 1 part B ("Inject ONLY into sdd-tasks and sdd-apply ... Do NOT
  inject into propose/spec/design/verify/archive."). This registry row is the load-bearing
  scoping binding the resolver consumes — it is BOTH the discoverable index entry AND the
  phase-scope expression. It is NOT a constraint BEGIN/END block (those propagate to all
  phases; see Decision 1 "Why not the registry BEGIN/END constraint block").

## Verifying AC-1 (the scoping guarantee)

AC-1 = "the contract reaches ONLY `sdd-tasks` and `sdd-apply` prompts." Because scoping is
probabilistic (a registry Trigger the resolver matches against, not a deterministic hook),
AC-1 is verified by **OBSERVATION — positive AND negative — and reported as a WARNING, never
as a guarantee**:

1. **Positive observation:** inspect the launch prompts the orchestrator constructs for
   `sdd-tasks` and `sdd-apply` and confirm `skills/_shared/minimalism-contract.md` appears in
   their `## Skills to load before work` blocks.
2. **Negative observation:** inspect the launch prompts for the other five phases
   (`sdd-propose`, `sdd-spec`, `sdd-design`, `sdd-verify`, `sdd-archive`) and confirm the path
   does NOT appear in any of them.
3. Report the result as a **WARNING-level** finding (PASS/FAIL of an observation), not a hard
   gate. Because the binding is the registry Trigger prose (Decision 1 part B) and the resolver
   matches it probabilistically, AC-1 is a **recurring observational guard**, re-checked
   whenever the registry trigger or the resolver's matching behavior changes. A deterministic
   guarantee would require the deferred option (b) parsing hook. This residual fragility is
   accepted for a Tier-3 slim slice and recorded below.

## Risks / Constraints

- **Probabilistic enforcement.** Instruction-only prose; a loaded sub-agent may ignore the
  ladder. Mitigation: full-path injection (R-006) preserves author intent better than a
  summary; measure directionally via AC-4. No hard guarantee is claimed.
- **Scoping fragility (binding is a probabilistic registry Trigger).** The phase→contract
  association is a prose Trigger description the resolver matches against (Decision 1 part B),
  not a tool-enforced deterministic filter; the resolver could over-match, or a registry
  regeneration could drop/over-broaden the trigger. Mitigation: AC-1 is a recurring
  observational guard (positive + negative, reported WARNING); the deferred option (b)
  parsing hook is the escape hatch if drift recurs. Accepted for a Tier-3 slim slice.
- **Advisory frontmatter is NOT enforced.** The `applies_to_phases` frontmatter (part A) is
  documentation only — the resolver does not parse it. It must never be mistaken for the
  scoping mechanism; the binding is the registry Trigger row.
- **Plane collision risk.** If someone mistakenly adds the contract to project-architect's or
  sdd-design's `## Skills to load before work`, citation would become a load and design-phase
  scoping would break. Mitigation: R-004 hardening + the explicit "citational, not a load"
  wording in the refactored architect text.
- **Self-inflation risk.** A minimalism contract that grows defeats itself. Constraint: keep
  the file to the four blocks above; new rungs/conventions require a separate change.
- **Do NOT touch `~/.claude/CLAUDE.md` or `sdd-phase-common.md`.** `~/.claude/CLAUDE.md` is
  the global orchestrator instruction file (shared across all projects, not overlay-tracked);
  `sdd-phase-common.md` is runtime-owned (gentle-ai). Neither is editable in this change. All
  scoping is achieved via the overlay-tracked registry Trigger row + the advisory frontmatter,
  never by editing global or shared runtime docs.

## Migration / rollout

Single slice, no data migration. Sequence: (1) add the contract file WITH the advisory
`applies_to_phases` frontmatter header (part A); (2) register it in `overlay.manifest`
(custom); (3) regenerate the `minimalism-contract` registry Trigger row via the skill-registry
skill with the phase-scoped description (part B — the load-bearing binding); (4) refactor
project-architect lines 164-166 to a citational reference. `~/.claude/CLAUDE.md` is NOT
touched. Rollback = remove the file, the manifest line, and the registry row, and restore the
architect prose; no state to unwind.
