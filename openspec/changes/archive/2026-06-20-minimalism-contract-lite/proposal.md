# Proposal: minimalism-contract-lite

## Why

The overlay has no unified minimalism layer. Anti-inflation guidance is scattered and
duplicated across skills that each restate it in their own words:

- `skills/project-architect/SKILL.md` (lines 164-166) declares ad-hoc "Do NOT inflate
  sections / module count / trade-offs" rules.
- `skills/sdd-tasks/SKILL.md` carries a 400-line changed-budget heuristic.
- `skills/sdd-verify/SKILL.md` encodes severities for over-engineering.
- `skills/kadia-content-guard/SKILL.md` guards content sprawl in its own domain.

None of these express a single, reusable **preference order** for resolving a requirement
with the least new code. An implementer has no canonical "try this before that" ladder, and
the duplicated prose drifts over time because there is no single source.

This change introduces ONE minimalism contract — a 6-rung preference ladder plus a mandatory
architectural tiebreaker — and points the existing project-architect rule at it instead of
restating it. The new genuine value is the **explicit preference ORDER** and the
**consolidation** of duplicated anti-inflation text into a single referenced source.

### Honest enforcement note (probabilistic, not deterministic)

This slice is **instruction-only (prose) enforcement injected into sub-agent prompts**. It is
therefore probabilistic, not guaranteed. The inspiration for this ladder (the ponytail
project's "lazy senior dev" ruleset) achieved its headline reductions (54% / 20% / 27%)
through **deterministic hooks**, which are explicitly NOT being ported here. Those numbers
MUST NOT be cited as expected outcomes for this change. We expect a directional, best-effort
nudge — measured manually against a baseline — not a hard gate.

## Tier and foundation override (auditable rationale)

This change runs at **Tier 3 (slim slice)** with deliberately minimal foundation.

The inception pipeline's default rule is "no manifest captured -> Tier 1 (treat as new
project)". That rule MISFIRES here. The repo (`labdrian-sdd-overlay`) is a MATURE governance
overlay; the absence of a captured SDD foundation in Engram is a recording gap, not evidence
of a greenfield project. Using "no manifest" as a proxy for "new project" is wrong in this
case. The change itself is **Markdown-only** (one new contract file plus a referential
refactor of one existing skill), with **no testable production code** (`strict_tdd: false`).

We therefore override to a slim slice on purpose and record the rationale here so the
deviation from the default tiering rule is auditable.

## What Changes

1. **Add** `skills/_shared/minimalism-contract.md` — the single source of truth containing:
   - The **6-rung ladder** as an explicit preference order (lowest viable rung first, climb
     only when the lower rung cannot satisfy the requirement):
     1. YAGNI — do not build it at all if not required now
     2. stdlib / language built-ins
     3. native platform feature
     4. existing dependency already in the project
     5. one-liner / minimal local code
     6. custom code / new abstraction (last resort)
   - A **mandatory architectural tiebreaker**: minimalism operates WITHIN design boundaries.
     A boundary mandated by the architecture is NEVER collapsed merely to save lines. Code
     economy never overrides a deliberate architectural seam.
   - The inline comment convention for when rung 6 (custom code) is chosen over a lower
     rung: use the host language's single-line comment syntax, e.g. `// minimal: <reason>`
     in C-style languages.

2. **Refactor** `skills/project-architect/SKILL.md` — replace the ad-hoc anti-inflation prose
   (lines 164-166) with a REFERENCE to `../_shared/minimalism-contract.md` as the single
   source. No content is duplicated; the architect cites the contract instead of restating it.

3. **Scope the contract to `sdd-tasks` and `sdd-apply` ONLY**, via DIRECT orchestrator
   injection of the contract file path into those two phases' sub-agent prompts under
   `## Skills to load before work`. It is NOT registered in `.atl/skill-registry.md`, because
   that registry's BEGIN/END blocks carry no per-phase metadata and propagate to ALL
   sub-agents — an existing anti-pattern we refuse to extend.

   > **MECHANISM REVERSAL NOTE (design phase decision — see design.md Decision 1 part B):**
   > The above scoping mechanism was reversed during the design phase. The final implemented
   > mechanism is a **scoped Trigger row in `.atl/skill-registry.md`** (option c+d from the
   > design adversarial review), NOT direct orchestrator injection. The registry row carries a
   > phase-scoped Trigger description: "Inject ONLY into sdd-tasks and sdd-apply sub-agent
   > prompts under '## Skills to load before work'. Do NOT inject into
   > propose/spec/design/verify/archive." This row is the load-bearing scoping binding the
   > orchestrator resolver consumes. The "no registry" decision in this proposal is superseded
   > by design Decision 1 part B. Original text preserved above for audit continuity.

### Requirements (EARS)

- **R-001**: WHEN the orchestrator injects skills for `sdd-tasks`/`sdd-apply`, the system
  SHALL include `minimalism-contract.md` in `## Skills to load before work`, and SHALL NOT
  include it for `propose`/`spec`/`design`/`verify`/`archive`.
- **R-002**: The contract SHALL define the 6-rung ladder as an explicit preference order —
  pick the lowest viable rung before climbing.
- **R-003**: The contract SHALL include a mandatory architectural tiebreaker: minimalism
  operates WITHIN design boundaries; a boundary mandated by architecture is NEVER collapsed to
  save code.
- **R-004**: WHEN `project-architect` emits its anti-inflation rule, the system SHALL
  reference `minimalism-contract.md` as the canonical single source for deduplication
  purposes, and SHALL NOT load or apply the 6-rung ladder during the design phase. The
  ladder applies ONLY where R-001 injects it (tasks/apply). The reference is citational,
  not a load instruction.
- **R-005**: WHEN an apply sub-agent picks custom code (rung 6) over a lower option, the
  system SHALL emit an inline `// minimal: <reason>` comment.
  > **Spec delta (R-005):** expanded to three states during spec authoring. The spec defines:
  > State 1 (judgment — lower rung rejected, emit reason); State 2 (forced non-obvious —
  > no lower rung + non-obvious context, emit `// minimal: forced — <ref>`); State 3
  > (forced obvious — no lower rung + obvious context, no comment). Rationale: a blanket
  > "always comment" rule generates noise when the constraint is self-evident. The contract
  > file reflects the three-state spec.
- **R-006**: The contract content SHALL be injected by path (full file read), NEVER as a
  pre-digested summary.

## Impact

- **New file**: `skills/_shared/minimalism-contract.md`
- **Modified**: `skills/project-architect/SKILL.md` (lines 164-166 -> reference)
- **Modified behavior** (orchestrator runtime): tasks/apply prompt construction injects the
  contract path. This is a convention documented in the contract + propagated by the
  orchestrator; no code instrumentation.
- **Affected actors**: `sdd-tasks` and `sdd-apply` sub-agents (receive the contract);
  `project-architect` (now references instead of restates). All other phases are unchanged.

## Scope

### In scope (first slice)

- One contract file with the 6-rung ladder + architectural tiebreaker + `// minimal:`
  convention.
- Direct orchestrator injection scoping (tasks/apply only).
- Referential refactor of project-architect's anti-inflation rule.

### Out of scope (explicit non-goals)

- NO deterministic hooks, MCP servers, TUI, or adapters.
- NO gate or checklist added to `sdd-verify` in this slice (a verify gate may be PROPOSED for
  slice 2 only if AC-4 shows a measurable behavior change).
- NO block added to `.atl/skill-registry.md` (avoids global all-phase contamination).
  > **MECHANISM REVERSAL NOTE:** This non-goal is superseded by design Decision 1 part B.
  > A scoped **Trigger row** (NOT a BEGIN/END constraint block) WAS added to
  > `.atl/skill-registry.md`. The distinction is critical: constraint blocks propagate to ALL
  > phases (the anti-pattern rejected here); a scoped Trigger row carries per-phase
  > injection metadata that the resolver reads. The "no BEGIN/END block" constraint holds;
  > the "no registry entry at all" decision was reversed. See design.md Decision 1 part B.
- NO separate ledger and NO `// debt:` convention (debt aggregation stays in Engram only).
- NO global scope and NO design-phase clause (tasks/apply only).
- Do NOT touch `sdd-phase-common.md` (it lives in the gentle-ai runtime, not editable here).
- NO automated LOC instrumentation (no hooks) — the baseline is captured MANUALLY.

## Acceptance Criteria

Baseline (captured MANUALLY before/independent of implementation): over 3-5 archived changes,
record LOC added, new dependencies introduced, and single-caller abstractions created.

- **AC-1**: The contract reaches ONLY `sdd-tasks` and `sdd-apply` prompts — verified by
  inspecting injected `## Skills to load before work` blocks across all phases.
- **AC-2**: `project-architect` no longer duplicates anti-inflation text — it references the
  contract; the old lines 164-166 prose is gone.
- **AC-3**: At least 1 real change shows a justified `// minimal: <reason>` comment, with no
  false architectural-boundary collapse.
- **AC-4**: Over the next 3-5 changes, LOC added / new deps trend DOWN or stay EQUAL-with-
  justification versus baseline (directional, given probabilistic enforcement).
- **AC-5 (gate decision)**: A slice-2 `sdd-verify` minimalism gate is proposed ONLY if AC-4
  demonstrates a real behavior change worth enforcing harder.

## Risks and Open Questions

- **Probabilistic enforcement**: instruction-only prose may be ignored by sub-agents under
  load. No deterministic guarantee. Mitigation: full-path injection (R-006) preserves author
  intent better than summaries; measure via AC-4.
- **Measurement noise**: a 3-5 change manual baseline is small; LOC/deps vary by change type.
  Trend, not absolute numbers, is the signal.
- **Misuse of the tiebreaker**: implementers could over-apply minimalism and collapse real
  boundaries. R-003 + AC-3 ("no false boundary collapse") are the guardrails.
- **Scoping fragility**: scoping is bound by the scoped Trigger row in `.atl/skill-registry.md`
  (advisory/probabilistic; the resolver matches trigger prose, no deterministic hook). A
  registry edit or orchestrator change could accidentally drop or over-broaden it. AC-1 is the
  recurring check.
- **Open question**: should the `// minimal:` convention be language-agnostic (comment syntax
  varies)? Slice-1 assumes `//`-style; deferred refinement if non-`//` languages appear.
