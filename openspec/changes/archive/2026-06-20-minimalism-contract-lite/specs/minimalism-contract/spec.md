# Minimalism Contract Specification

## Purpose

Formal requirements for the `minimalism-contract-lite` change: a single-source minimalism
contract injected into `sdd-tasks` and `sdd-apply` only, with a citational reference in
`project-architect`, verified by artifact inspection (no test runner).

Verification method for all requirements: artifact inspection of generated files and
orchestrator-emitted prompt blocks. No test runner is applicable (Markdown-only change,
`strict_tdd: false`).

---

## Requirements

### Requirement: Phase-Scoped Contract Injection

ID: R-001

WHEN the orchestrator constructs the launch prompt for `sdd-tasks` or `sdd-apply`, the
orchestrator SHALL include `skills/_shared/minimalism-contract.md` under
`## Skills to load before work`, and SHALL NOT include it in the prompt for any of
`sdd-propose`, `sdd-spec`, `sdd-design`, `sdd-verify`, or `sdd-archive`.

#### Scenario: Contract present in tasks prompt

- GIVEN the orchestrator is constructing the `sdd-tasks` sub-agent launch prompt
- WHEN the orchestrator applies its skill-resolution rules
- THEN the `## Skills to load before work` block in the emitted prompt contains the path
  `skills/_shared/minimalism-contract.md`

#### Scenario: Contract present in apply prompt

- GIVEN the orchestrator is constructing the `sdd-apply` sub-agent launch prompt
- WHEN the orchestrator applies its skill-resolution rules
- THEN the `## Skills to load before work` block in the emitted prompt contains the path
  `skills/_shared/minimalism-contract.md`

#### Scenario: Contract absent from non-tasks/apply prompts (AC-1 guard)

- GIVEN the orchestrator is constructing a launch prompt for any phase in
  `{sdd-propose, sdd-spec, sdd-design, sdd-verify, sdd-archive}`
- WHEN the orchestrator applies its skill-resolution rules
- THEN the `## Skills to load before work` block does NOT contain
  `skills/_shared/minimalism-contract.md`

---

### Requirement: Six-Rung Preference Ladder

ID: R-002

The `skills/_shared/minimalism-contract.md` file SHALL define a six-rung preference ladder
in which rungs are ordered from lowest to highest implementation cost, and each rung is
labeled such that implementers can identify the lowest viable rung before climbing.

#### Scenario: Ladder content present and ordered

- GIVEN `skills/_shared/minimalism-contract.md` exists on the filesystem
- WHEN an inspector reads the file
- THEN it contains exactly six labeled rungs in the following order:
  1. YAGNI (do not build if not required now)
  2. stdlib / language built-ins
  3. native platform feature
  4. existing dependency already in the project
  5. one-liner / minimal local code
  6. custom code / new abstraction (last resort)

#### Scenario: Ladder is self-contained

- GIVEN the contract file is read in isolation (no other file loaded)
- WHEN an implementer reads the ladder section
- THEN the preference order is unambiguous and no external document is required to
  interpret any rung

---

### Requirement: Architectural Tiebreaker

ID: R-003

The `skills/_shared/minimalism-contract.md` file SHALL include a mandatory architectural
tiebreaker stating that minimalism operates within design boundaries and that a boundary
mandated by the architecture is never collapsed to save lines of code.

#### Scenario: Tiebreaker present in contract

- GIVEN `skills/_shared/minimalism-contract.md` exists on the filesystem
- WHEN an inspector reads the file
- THEN it contains a section explicitly stating that architectural boundaries take
  precedence over code economy, and that such boundaries MUST NOT be collapsed to achieve
  a lower rung on the ladder

#### Scenario: Tiebreaker does not override ladder for non-boundary decisions

- GIVEN a requirement that has no architectural boundary constraint
- WHEN an implementer applies the tiebreaker rule
- THEN the tiebreaker is inapplicable and the implementer MUST select the lowest viable
  ladder rung without invoking the tiebreaker

---

### Requirement: Citational Reference in project-architect

ID: R-004

WHEN `project-architect` emits its anti-inflation guidance, the system SHALL reference
`../_shared/minimalism-contract.md` as the canonical single source for deduplication, and
SHALL NOT include an entry for `minimalism-contract.md` in `project-architect`'s own
`## Skills to load before work` set, and SHALL NOT reproduce the six-rung ladder prose
inline in `skills/project-architect/SKILL.md`.

#### Scenario: Citation present, ladder prose absent

- GIVEN `skills/project-architect/SKILL.md` has been refactored
- WHEN an inspector reads the file at the location of the former lines 164-166
  anti-inflation prose
- THEN it contains a reference to `../_shared/minimalism-contract.md` as the canonical
  source
- AND it does NOT contain a restated copy of the six-rung ladder

#### Scenario: Contract not loaded during design phase

- GIVEN the orchestrator is constructing the `sdd-design` sub-agent launch prompt
  (which includes `project-architect` skills)
- WHEN an inspector reads the emitted `## Skills to load before work` block
- THEN `skills/_shared/minimalism-contract.md` is NOT listed as a file to load
- AND the citational reference in `project-architect/SKILL.md` is documentation only,
  not a load instruction

#### Scenario: Citation text distinguishes path from load instruction

- GIVEN the refactored `project-architect/SKILL.md` text
- WHEN an inspector reads the anti-inflation section
- THEN the text explicitly states the reference is citational (e.g., "documentation
  reference", "not a load instruction", or equivalent phrasing) and that the ladder
  applies only during sdd-tasks / sdd-apply

---

### Requirement: Inline Justification Comment for Rung 6

ID: R-005

The `// minimal:` convention defines THREE comment states for rung-6 code, using the host
language's single-line comment syntax. The state applied depends on whether a lower rung
was available and whether the forcing constraint is obvious from immediate context.

**State 1 — judgment comment** (lower rung existed but was not taken):
WHEN an `sdd-apply` sub-agent selects rung 6 AND a lower viable rung (1-5) existed but
was rejected, the system SHALL emit `// minimal: <reason>` immediately adjacent to the
custom code, where `<reason>` names the lower rung that was considered and explains why it
was insufficient. This state documents an explicit judgment call.

**State 2 — forced comment** (no lower rung was applicable AND constraint is non-obvious):
WHEN an `sdd-apply` sub-agent selects rung 6 AND no lower rung was applicable AND the
forcing constraint is NOT obvious from the immediate context (e.g., not apparent from the
type signature, the linked design artifact, or the call site), the system SHALL emit
`// minimal: forced — <design/constraint ref>` immediately adjacent to the custom code,
citing the design artifact or constraint name that mandates custom code.

**State 3 — no comment** (no lower rung was applicable AND constraint is obvious):
WHEN an `sdd-apply` sub-agent selects rung 6 AND no lower rung was applicable AND the
forcing constraint IS obvious from immediate context (e.g., a unique interface requirement
stated in the type signature or the directly linked design artifact), the system SHALL NOT
emit a `// minimal:` comment. Emitting a comment in this state is DISCOURAGED as noise,
consistent with the contract's own minimalism: do not add comments that carry no information.

> **Enforcement scope — CONVENTION ONLY (slice 1):** In this slice the `// minimal:`
> convention is enforced by prose injection into the `sdd-apply` sub-agent prompt only
> (probabilistic, not deterministic). An automated verification gate (linter or grep check
> in `sdd-verify`) is DELIBERATELY DEFERRED to slice 2, and will be proposed ONLY if AC-4
> demonstrates a measurable behavior change worth enforcing harder (per AC-5). Adding a
> `sdd-verify` gate now would violate the slice scope and contradict the contract's own YAGNI
> rung. This note is informational; it is NOT a testable requirement for this slice.

#### Scenario: Judgment comment — lower rung existed but was not taken

- GIVEN an `sdd-apply` sub-agent selects rung 6
- AND at least one lower rung (1-5) was evaluated and found applicable in principle but
  rejected for a specific reason
- WHEN the sub-agent writes the custom code
- THEN it emits a comment in the form `// minimal: <reason>` (using the host language's
  single-line comment syntax) immediately adjacent to the rung-6 code
- AND `<reason>` names the lower rung that was considered and states why it was insufficient
- AND the comment is distinguishable from a forced comment by the absence of the
  `forced —` prefix

#### Scenario: Forced comment — no lower rung applicable and constraint is non-obvious

- GIVEN an `sdd-apply` sub-agent selects rung 6
- AND no lower rung (1-5) was applicable
- AND the forcing constraint is NOT immediately apparent from the type signature, call
  site, or directly linked design artifact
- WHEN the sub-agent writes the custom code
- THEN it emits a comment in the form `// minimal: forced — <design/constraint ref>`
  immediately adjacent to the rung-6 code
- AND `<design/constraint ref>` cites a specific design artifact name, requirement ID,
  or constraint that explains why custom code was the only option
- AND the comment is distinguishable from a judgment comment by the presence of the
  `forced —` prefix

#### Scenario: No comment — no lower rung applicable and constraint is obvious

- GIVEN an `sdd-apply` sub-agent selects rung 6
- AND no lower rung (1-5) was applicable
- AND the forcing constraint IS obvious from immediate context (e.g., stated in the type
  signature or directly visible from the linked design artifact at the call site)
- WHEN the sub-agent writes the custom code
- THEN it does NOT emit a `// minimal:` comment of any form
- AND a reviewer inspecting the output can confirm that the absence of a comment is
  intentional, not an omission, because the constraint is self-evident from the context

#### Scenario: Comment syntax adapts to host language

- GIVEN the target file uses a non-C-style single-line comment syntax (e.g., Python `#`,
  shell `#`, YAML `#`)
- WHEN any of the above comment-required states apply
- THEN the sub-agent uses the correct single-line comment prefix for that language
  (e.g., `# minimal: <reason>` or `# minimal: forced — <ref>`) rather than `//`

---

### Requirement: Full-Path Injection, No Summaries

ID: R-006

The orchestrator SHALL inject `minimalism-contract.md` into sub-agent prompts by
including its full filesystem path under `## Skills to load before work`, and SHALL NOT
substitute a pre-digested summary or inline paraphrase of the contract content in place
of the path reference.

#### Scenario: Path reference present in injected block

- GIVEN the orchestrator emits a `sdd-tasks` or `sdd-apply` launch prompt
- WHEN an inspector reads the `## Skills to load before work` section
- THEN it contains a line with the path `skills/_shared/minimalism-contract.md` (or
  the fully-qualified equivalent), not a bullet list of ladder rungs or paraphrased prose

#### Scenario: Sub-agent reads the file, not a summary

- GIVEN the sub-agent receives the injected path
- WHEN the sub-agent executes its skill-loading step
- THEN it calls the file-read tool on the path and receives the full file content,
  not a cached summary from a prior session or an orchestrator-generated paraphrase
