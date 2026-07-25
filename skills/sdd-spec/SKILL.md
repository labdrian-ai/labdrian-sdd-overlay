---
name: sdd-spec
description: "Write SDD delta specs with requirements and scenarios. Trigger: orchestrator launches spec work for a change."
disable-model-invocation: true
user-invocable: false
license: MIT
metadata:
  author: gentleman-programming
  version: "2.0"
  delegate_only: true
---

> **ORCHESTRATOR GATE**: If you loaded this skill via the `skill()` tool, you are
> the ORCHESTRATOR — STOP. Do NOT execute these instructions inline. Delegate to
> the dedicated `sdd-spec` sub-agent using your platform's delegation primitive
> (e.g., `task(...)`, sub-agent invocation, etc.). This skill is for EXECUTORS
> only.

## Executor Override

If you ARE the `sdd-spec` sub-agent (NOT the orchestrator), the gate above does NOT apply to you. Continue with the phase work below. Do NOT delegate. Do NOT call the Skill tool. You are the executor — execute.


## Language Domain Contract

Generated technical artifacts default to English. Do not inherit the user's conversational language or the active persona's regional voice for SDD artifacts unless the user explicitly requests that artifact language or the project convention requires it.

If technical artifacts are explicitly requested in another language, use a neutral/professional register unless the user explicitly requests a different tone or regional variant.

Public/contextual comments follow the target context language by default. Explicit user language or tone overrides win; otherwise use a neutral/professional register unless the target context clearly calls for another tone or regional variant.

## Purpose

You are a sub-agent responsible for writing SPECIFICATIONS. You take the proposal and produce delta specs — structured requirements and scenarios that describe what's being ADDED, MODIFIED, REMOVED, or RENAMED from the system's behavior.

## What You Receive

From the orchestrator:
- Change name
- Artifact store mode (`engram | openspec | hybrid | none`)

## Execution and Persistence Contract

> Follow **Section B** (retrieval) and **Section C** (persistence) from `skills/_shared/sdd-phase-common.md`.

- **engram**: Read `sdd/{change-name}/proposal` (required). If specs span multiple domains, concatenate into a single artifact with domain headers. Save as `sdd/{change-name}/spec`.
- **openspec**: Read and follow `skills/_shared/openspec-convention.md`.
- **hybrid**: Follow BOTH conventions — persist to Engram (single concatenated artifact) AND write domain files to filesystem.
- **none**: Return result only. Never create or modify project files.

## What to Do

### Step 1: Load Skills
Follow **Section A** from `skills/_shared/sdd-phase-common.md`.

### Step 2: Identify Affected Domains

Read the proposal's **Capabilities section** — this is your primary contract:

```
FOR EACH entry under "New Capabilities":
├── This becomes a NEW full spec: openspec/specs/<capability-name>/spec.md
└── Write a complete spec (not a delta) — no existing behavior to reference

FOR EACH entry under "Modified Capabilities":
├── This becomes a DELTA spec: openspec/changes/{change-name}/specs/<capability-name>/spec.md
└── Read existing openspec/specs/<capability-name>/spec.md first — your delta modifies it
```

If the proposal has no Capabilities section (older format), fall back to inferring from "Affected Areas". But always prefer the explicit Capabilities mapping when present.

### Step 3: Read Existing Specs

**IF mode is `openspec` or `hybrid`:** If `openspec/specs/{domain}/spec.md` exists, read it to understand CURRENT behavior. Your delta specs describe CHANGES to this behavior.

**IF mode is `engram`:** Existing specs were already retrieved from Engram in the Persistence Contract. Skip filesystem reads.

**IF mode is `none`:** Skip — no existing specs to read.

### Step 4: Write Delta Specs

**IF mode is `openspec` or `hybrid`:** Create specs inside the change folder:

```
openspec/changes/{change-name}/
├── proposal.md              ← (already exists)
└── specs/
    └── {domain}/
        └── spec.md          ← Delta spec
```

**IF mode is `engram` or `none`:** Do NOT create any `openspec/` directories or files. Compose the spec content in memory — you will persist it in Step 5.

#### MODIFIED Requirements Workflow (CRITICAL — read before writing deltas)

When writing a `## MODIFIED Requirements` section, follow this exact workflow:

```
1. Locate the requirement in openspec/specs/{domain}/spec.md
2. COPY the ENTIRE requirement block — from `### Requirement:` through ALL its scenarios
3. PASTE it under `## MODIFIED Requirements`
4. EDIT the copy to reflect the new behavior
5. Add "(Previously: {one-line summary of what changed})" under the requirement text

Why copy-full-then-edit?
→ The archive step REPLACES the requirement in main specs with your MODIFIED block
→ If your block is partial, the archive will lose scenarios you didn't copy
→ Common pitfall: only writing the changed scenario and losing the rest
→ If adding NEW behavior WITHOUT changing existing behavior, use ADDED instead
```

#### Requirement Sentence Format — EARS (CRITICAL)

Every requirement body MUST be written as ONE EARS sentence with exactly one
`SHALL`. EARS removes ambiguity and makes each requirement directly testable.

| Pattern | Template | Use when |
|---------|----------|----------|
| Ubiquitous | `The {system} SHALL {response}.` | Always-true invariant, no trigger. |
| Event-driven | `WHEN {trigger}, the {system} SHALL {response}.` | A discrete event occurs. |
| State-driven | `WHILE {state}, the {system} SHALL {response}.` | Behavior holds during a state. |
| Unwanted behavior | `IF {condition}, THEN the {system} SHALL {response}.` | Errors, validation, edge cases. |
| Optional feature | `WHERE {feature present}, the {system} SHALL {response}.` | Behavior gated by a flag/capability. |

Compose when needed: `WHILE {state}, WHEN {trigger}, the {system} SHALL {response}.`
One SHALL per requirement. If you need two SHALLs, write two requirements.

**Banned soft verbs — ONLY as the SHALL response verb** (they make the response
untestable): `might`, `could`, `should`, `would`, `as appropriate`, `if possible`,
`support`, `handle`, `manage`, `robust`, `user-friendly`. This ban applies ONLY to
the response slot. RFC 2119 `MAY`/`SHOULD` remain LEGAL to express requirement
strength — do NOT strip them. Keep: "The system SHOULD cache results" (strength).
Kill: "WHEN X, the system SHALL handle it" (vague response → name the action:
return / persist / display / reject / emit / set / block).

**Stable Requirement IDs** — every requirement carries a stable ID in the format
`R-001`, `R-002`, ... (zero-padded, dash), matching the upstream
requirements-from-transcripts contract:

- The ID lives OFF the heading line, as the first body line: `ID: R-001`.
- The heading line stays EXACTLY `### Requirement: {Name}` so archive's
  name-exact matching and the MODIFIED block-replace are never broken.
- Assign IDs once. NEVER renumber. MODIFIED and RENAMED KEEP their original ID.
- REMOVED IDs are retired and NEVER reused.
- If the upstream requirements brief already assigned `R-001` IDs, carry them
  forward VERBATIM — do NOT re-mint and do NOT reformat.

**Traceability contract**: every `R-NNN` MUST map to >=1 test, and every test MUST
cite its `R-NNN` (in the test name or a comment). Scenarios stay in
Given/When/Then and become the concrete test cases for that requirement.
Downstream, sdd-tasks cites `R-NNN` per task and sdd-verify keys its compliance
matrix on `R-NNN`. When Strict TDD is active, the RED test for each task MUST
cite the `R-NNN` it covers.

#### Delta Spec Format

```markdown
# Delta for {Domain}

## ADDED Requirements

### Requirement: {Requirement Name}

ID: R-NNN

WHEN {trigger}, the {system} SHALL {observable response}.
<!-- Use the matching EARS pattern; one SHALL per requirement. -->

#### Scenario: {Happy path scenario}

- GIVEN {precondition}
- WHEN {action}
- THEN {expected outcome}
- AND {additional outcome, if any}

#### Scenario: {Edge case scenario}

- GIVEN {precondition}
- WHEN {action}
- THEN {expected outcome}

## MODIFIED Requirements

### Requirement: {Existing Requirement Name}

ID: R-NNN

{Full updated requirement as a single EARS SHALL sentence — replaces ONLY the
previous requirement sentence; the existing ID is preserved.}

#### Scenario: {all existing scenarios preserved verbatim}
- GIVEN ...
- WHEN ...
- THEN ...
<!-- Copy the FULL existing block first, then edit only the EARS sentence.
     Do NOT drop scenarios. ID R-NNN preserved across this modification. -->

## REMOVED Requirements

### Requirement: {Requirement Being Removed}

(Reason: {why this requirement is being deprecated/removed})
(Migration: {what replaces it, or "None" if no migration is needed})

## RENAMED Requirements

### Requirement: {Old Requirement Name} → {New Requirement Name}

(Reason: {why the requirement is being renamed})
(Migration: {how references/tests/docs should update, or "None" if no migration is needed})
```

#### For NEW Specs (No Existing Spec)

If this is a completely new domain, create a FULL spec (not a delta):

```markdown
# {Domain} Specification

## Purpose

{High-level description of this spec's domain.}

## Requirements

### Requirement: {Name}

ID: R-001

WHEN {trigger}, the {system} SHALL {observable response}.
<!-- Use the matching EARS pattern; one SHALL per requirement. -->

#### Scenario: {Name}

- GIVEN {precondition}
- WHEN {action}
- THEN {outcome}
```

### Step 5: Persist Artifact

**This step is MANDATORY — do NOT skip it.**

Follow **Section C** from `skills/_shared/sdd-phase-common.md`.
- artifact: `spec`
- topic_key: `sdd/{change-name}/spec`
- type: `architecture`

### Step 6: Return Summary

Return to the orchestrator:

```markdown
## Specs Created

**Change**: {change-name}

### Specs Written
| Domain | Type | Requirement IDs | Scenarios |
|--------|------|-----------------|-----------|
| {domain} | Delta/New | R-001, R-002, R-003 (3 added, 0 mod) | {total scenarios} |

### Coverage
- Happy paths: {covered/missing}
- Edge cases: {covered/missing}
- Error states: {covered/missing}

### Next Step
Ready for design (sdd-design). If design already exists, ready for tasks (sdd-tasks).
```

## Rules

- ALWAYS use Given/When/Then format for scenarios
- ALWAYS use RFC 2119 keywords (MUST, SHALL, SHOULD, MAY) for requirement strength
- ALWAYS write each requirement body as ONE EARS sentence with exactly one SHALL (see EARS section)
- ALWAYS assign a stable `R-NNN` ID as the first body line; keep the heading line EXACTLY `### Requirement: {Name}` (archive matches by exact heading)
- When promoting an upstream requirements brief, reformat each `### R-NNN — <name>` brief heading to `### Requirement: {Name}` with `ID: R-NNN` as the first body line — never embed the ID in the heading.
- ALWAYS preserve IDs across MODIFIED/RENAMED, retire on REMOVED, never reuse; carry upstream `R-NNN` forward verbatim
- NEVER use soft verbs (might/could/should/handle/support/manage) as the SHALL response verb; RFC 2119 MAY/SHOULD stay legal for strength
- For MODIFIED, copy the FULL existing block (all scenarios) first, then rewrite only the EARS sentence — never drop scenarios
- Every `R-NNN` MUST have >=1 Given/When/Then scenario a covering test can cite back (traceability contract)
- Read the proposal's **Capabilities section** first — it tells you exactly which spec files to create
- If existing specs exist, write DELTA specs (ADDED/MODIFIED/REMOVED sections)
- If NO existing specs exist for the domain, write a FULL spec
- Every requirement MUST have at least ONE scenario
- Include both happy path AND edge case scenarios
- Keep scenarios TESTABLE — someone should be able to write an automated test from each one
- DO NOT include implementation details in specs — specs describe WHAT, not HOW
- **MODIFIED requirements MUST be the FULL block** — copy entire requirement + all scenarios from main spec, then edit. Partial MODIFIED blocks lose content at archive time.
- If adding new behavior without changing existing behavior → use ADDED, not MODIFIED
- REMOVED requirements MUST include Reason and SHOULD include Migration when consumers, persisted behavior, docs, or tests are affected
- RENAMED requirements MUST state both old and new names explicitly and SHOULD include Migration guidance for references/tests/docs
- Apply any `rules.specs` from `openspec/config.yaml`
- **Size budget**: Spec artifact MUST be under 650 words. Prefer requirement tables over narrative descriptions. Each scenario: 3-5 lines max.
- Return envelope per **Section D** from `skills/_shared/sdd-phase-common.md`.

## RFC 2119 Keywords Quick Reference

| Keyword | Meaning |
|---------|---------|
| **MUST / SHALL** | Absolute requirement |
| **MUST NOT / SHALL NOT** | Absolute prohibition |
| **SHOULD** | Recommended, but exceptions may exist with justification |
| **SHOULD NOT** | Not recommended, but may be acceptable with justification |
| **MAY** | Optional |
