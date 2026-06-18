---
name: requirements-from-transcripts
description: "Trigger: meeting transcripts, customer stories, client conversations, user interviews, requerimientos desde meets, historias de cliente. Convert raw stakeholder conversations into explanatory, traceable technical requirements for Genesis SDD work."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Use this skill when the user provides meeting transcripts, chat conversations, customer stories, stakeholder notes, support summaries, or asks to turn business conversation into Genesis technical requirements.

The output must be explanatory enough for engineers, product reviewers, and SDD agents to understand the intent without re-reading the full transcript. It must also be structured as upstream input for the Genesis inception chain:

```text
atomic requirements → project manifest → project architecture → SDD roadmap
```

The goal is not merely to document requests. The goal is to preserve stakeholder intent in a shape that lets `project-inception` write a faithful manifest, lets the architecture phase size and constrain the solution, and lets the SDD roadmap plan accurate small changes that solve the actual requirements.

## Hard Rules

- Do not invent requirements. Derive every requirement from transcript evidence or mark it as an assumption.
- Preserve stakeholder language before translating it into technical language.
- Separate facts, interpretations, assumptions, and open questions.
- Before writing requirements, run a contradiction and ambiguity pre-analysis over the transcript. If the transcript contains conflicting requests, uncertain policies, overloaded terms, or missing definitions that affect permissions, data, workflow, money, compliance, or user-visible behavior, stop immediately at the first unresolved issue, expose the problem clearly, propose one or more resolution options, ask the user to choose, and wait. Do not continue to the requirements brief until that issue is resolved. Resolve issues one by one.
- Treat ambiguous business words as blocking until defined when they affect permissions, data, workflow, money, compliance, or user-visible behavior.
- Never collapse multiple concerns into one generic requirement such as “improve UX”. Split by observable behavior.
- ALWAYS write the `**Requirement:**` line as ONE EARS sentence with exactly one SHALL (see Section 1c); banned soft verbs as the response verb are blocking — replace them with a specific action verb.
- ALWAYS use `R-001` format IDs (zero-padded, dash); IDs are stable and must carry forward verbatim into all downstream SDD artifacts.
- Every fix, feature, policy change, or visual correction must become its own small requirement. Do not bundle unrelated fixes into one requirement.
- Requirements must be ordered for later `project-inception` ingestion: dependencies first, user-visible fixes second, polish last, unless business priority says otherwise.
- Every requirement must include stable keywords in English and stakeholder language so future Engram/SDD searches can recover it.
- Every requirement must include acceptance evidence: UI state, API contract, persisted data, permission rule, event, test, or operational check.
- Every critical user phrase must appear in the traceability matrix or in an explicit out-of-scope section.
- The brief must produce explicit inputs for three downstream layers: Manifest Inputs, Architecture Inputs, and SDD Roadmap Inputs.
- Do not let roadmap planning start from broad themes. It must start from ordered atomic requirements and their business/technical constraints.
- If the requirement will feed SDD, produce change candidates that can become `sdd/<change-name>/{proposal,spec,design,tasks}` artifacts in Engram.
- Genesis SDD artifacts are Engram-first. Do not create `openspec/` artifacts unless the maintainer explicitly approved that exception.

## Critical Patterns

### 1. Conversation-to-Requirement Pipeline

Follow this sequence:

```text
Raw transcript/story
  → contradiction and ambiguity pre-analysis
  → stakeholder claims
  → user-visible problems
  → business rules
  → atomic technical requirements
  → acceptance scenarios
  → traceability matrix
  → ordered project-inception handoff
  → manifest inputs
  → architecture inputs
  → SDD change candidates / roadmap seeds
```

Do not jump directly from transcript to implementation tasks. That is how scope drift happens.

### 1a. Contradiction and Ambiguity Pre-Analysis

Before extracting final atomic requirements, inspect the transcript for contradictions and ambiguous requests. This pre-analysis is mandatory because stakeholder conversations often contain assumptions, corrections, partial memories, or words with multiple domain meanings.

Classify every issue as one of:

| Issue Type | Meaning | Required Action |
|---|---|---|
| Contradiction | Two or more transcript statements cannot all be true or implemented together as-is. | Quote the conflicting anchors, explain the conflict, propose resolution options, and leave the choice to the user. |
| Ambiguity | A term or request has multiple valid interpretations. | Explain the possible meanings and ask for/record the needed definition before implementation. |
| Missing rule | The transcript states desired behavior but omits a policy, boundary, state, role, or data rule needed to implement safely. | Mark as blocking when it affects permissions, persistence, workflow, money, compliance, or user-visible behavior. |
| Assumption | A likely interpretation is useful for planning but not explicitly stated. | Label it as an assumption and require confirmation before SDD apply. |

When contradictions or ambiguities are found:

- preserve the exact stakeholder phrases that caused the issue;
- explain why the issue matters in product/technical terms;
- propose concrete resolution options, for example Option A / Option B;
- state the tradeoff of each option briefly;
- do **not** silently pick one option;
- ask for the user's decision on exactly one issue at a time;
- stop and wait after asking;
- after the user resolves that issue, continue the pre-analysis and repeat the same process for the next unresolved issue;
- only draft the requirements brief after all blocking contradictions and ambiguities have been resolved, or after the user explicitly approves continuing with named assumptions.

Do not output a complete requirements brief in the same response where a blocking contradiction or ambiguity is first found. That loses the decision thread. The correct behavior is: detect issue → ask one decision → wait → incorporate answer → continue.

Examples:

| Transcript Signal | Pre-Analysis Output |
|---|---|
| “Supongo que no lo pueda hacer ni el solicitante ni un colaborador.” | Ambiguity: comment policy is not confirmed. Ask only this decision first: Option A: block both requester and collaborators while unassigned. Option B: block collaborators only. Stop and wait. |
| “responsable” used for current assignee and destination owner | Ambiguity: define whether responsible means current assignee, area owner, destination assignee, or capability holder. |
| “limpiar el chat” | Missing rule: decide whether to clear only active UI session or delete persisted conversation history. |

### 1b. Atomic Requirement Rule

One fix or one feature equals one requirement. If a stakeholder sentence contains multiple behaviors, split it.

| Raw Stakeholder Input | Correct Split |
|---|---|
| “La propuesta muestra Media y el responsable no puede editar prioridad.” | R-001 Proposal must show selected creation priority. R-002 Current responsible must be able to edit priority. |
| “Arreglar scrollbars y tema oscuro.” | R-001 Scrollbar thumb/track must use theme tokens. R-002 Scrollbars must be visually verified in light mode. R-003 Scrollbars must be visually verified in dark mode. |
| “Mejorar la creación de tickets.” | BLOCKED until split into concrete observable failures. |

Atomic requirements must be:

- independently understandable;
- independently testable;
- independently traceable to a source anchor;
- small enough to become one SDD requirement or one SDD slice;
- explicit about whether it is a fix, feature, policy, visual correction, or technical debt item.

If a requirement cannot be tested independently, it is still too large.

### 1c. EARS Sentence Format (CRITICAL)

Every `**Requirement:**` line MUST be ONE EARS sentence with exactly one `SHALL`.
EARS sentences are unambiguous and directly testable — they make the downstream
sdd-spec, sdd-tasks, and sdd-verify chain work correctly.

| Pattern | Template | Use when |
|---------|----------|----------|
| Ubiquitous | `The {system} SHALL {response}.` | Always-true invariant. |
| Event-driven | `WHEN {trigger}, the {system} SHALL {response}.` | A discrete event occurs. |
| State-driven | `WHILE {state}, the {system} SHALL {response}.` | Behavior holds during a state. |
| Unwanted behavior | `IF {condition}, THEN the {system} SHALL {response}.` | Errors, validation, edge cases. |
| Optional feature | `WHERE {feature present}, the {system} SHALL {response}.` | Gated behavior. |

One SHALL per requirement — if two behaviors are needed, write two requirements.

**Banned soft verbs as the SHALL response**: `handle`, `support`, `manage`, `improve`,
`might`, `could`, `would`, `robust`, `user-friendly`, `if possible`, `as appropriate`.
Name the actual action instead: return / display / persist / reject / emit / set / block.
RFC 2119 `MUST`/`SHOULD`/`MAY` remain legal for requirement strength and MUST NOT be
stripped; only the vague response verbs are banned.

**ID format**: `R-001`, `R-002`, ... (zero-padded, dash). IDs are stable — assigned once,
never renumbered, never reused. These IDs carry forward verbatim into sdd-spec, sdd-tasks,
and sdd-verify to close the traceability chain.

### 2. Use User Words as Anchors

Extract exact phrases that carry requirement meaning, for example:

- “la propuesta aparece con prioridad media”
- “el responsable tiene que poder editar prioridad”
- “las barras de scroll no son congruentes con tema oscuro/claro”

Each anchor must map to one of:

| Anchor Status | Meaning |
|---|---|
| Requirement | Must be implemented and verified. |
| Business rule | Defines behavior or authorization. |
| UX rule | Defines visible behavior or copy/state. |
| Open question | Too ambiguous to implement safely. |
| Out of scope | Explicitly excluded with reason. |

### 2b. Keyword Index Is Mandatory

For every atomic requirement, create a keyword set that includes:

- stakeholder language keywords, including Spanish terms from the transcript;
- canonical technical terms used in code or contracts;
- likely file/domain terms;
- SDD/search terms for Engram retrieval.

Example:

```text
R-001 keywords:
- stakeholder: prioridad, propuesta, media, alta, urgente, creación ticket
- technical: priority, urgencyLevel, proposal draft, nuevaSession.metadata, CreateTicketInput
- domains: service-desk, ticket creation, proposal review
```

If a future SDD cannot find a requirement by those keywords, the requirement brief failed.

### 3. Define Ambiguous Terms Before Implementation

When a stakeholder term has multiple technical meanings, stop and define it.

| Term Example | Must Clarify |
|---|---|
| responsable | Current assignee, area owner, subarea responsible, admin, collaborator, or capability holder? |
| prioridad | UI label, request metadata, proposal draft value, create payload, persisted current state, or editable ticket state? |
| propuesta | Agent draft, review page display, POST payload, or final ticket preview? |
| congruente | Same value, same color token, same label, same behavior across themes, or all of these? |
| urgente | UI label for `critical` or a separate enum value? |

If ambiguity changes security, authorization, persistence, or user trust, mark it as **Blocking Question**.

### 4. Require Behavior Chains

For workflow requirements, define the full chain. Do not test only a middle function.

Example:

```text
Selected priority in creation
  → stored in session/request metadata
  → shown in proposal review
  → same effective value is submitted on confirm
  → backend persists same current priority
  → list/detail show same label and color
  → authorized responsible can edit it when allowed
```

Each arrow is a potential failure point and needs acceptance evidence.

### 5. Detect Display-vs-Payload Drift

Any review page must follow this rule:

> The UI must render from the same effective object that will be submitted.

If display uses fallback A and submit uses object B, create an explicit drift risk and require a test that proves displayed value equals submitted payload value.

### 6. Acceptance Scenarios Must Cover Variants

Do not write one happy-path scenario when variants exist.

Cover variants such as:

- ticket type: `problem`, `feature`, `question` if applicable;
- priority: `low`, `medium`, `high`, `critical`;
- role: requester, current assignee, collaborator, same-subarea viewer, admin;
- state: new, open/in progress, paused, resolved, cancelled;
- theme: light and dark when visual congruence is requested.

### 7. Verification Must Reuse the Traceability Matrix

Before SDD verify/archive, every requirement row must have evidence.

| Requirement | Evidence Required |
|---|---|
| UI display | Component test, visual check, or screenshot acceptance. |
| Submit payload | Test asserting the exact API payload. |
| Persistence | Backend unit/integration test or DB projection check. |
| Authorization | Capability projection plus endpoint guard test. |
| Theme congruence | Token/class check plus light/dark visual or DOM evidence. |
| Event/timeline behavior | Event reducer/rendering test. |

No evidence means **not complete**, even if tasks are checked off.

### 8. Project-Inception Handoff

The requirements brief must hand off ordered atomic requirements to `project-inception`. `project-inception` then creates the manifest, architecture, and SDD roadmap from those small requirements instead of from broad themes.

The handoff has three layers:

| Layer | Receives | Must Answer |
|---|---|---|
| Manifest | stakeholder intent, mission, non-negotiable rules, success criteria, scope/out-of-scope | What outcome do we need and what must never drift? |
| Architecture | domains, data flows, integrations, permissions, persistence, UI surfaces, risks | What systems/files/contracts are affected and how big is this? |
| SDD Roadmap | ordered atomic requirements, dependencies, evidence requirements, risk | In what sequence do we implement small accurate changes? |

Each handoff row must include:

| Field | Purpose |
|---|---|
| Order | Implementation/review sequence. |
| Requirement ID | Stable reference, e.g. `R-001`. |
| Requirement size | Small / Medium; Large must be split before roadmap. |
| Change type | Fix, feature, visual correction, permission rule, data continuity, technical debt. |
| Keywords | Search terms that must appear in future SDD artifacts. |
| Suggested SDD change | Candidate `sdd/<change-name>`. |
| Dependency | Prior requirement or external decision needed first. |
| Acceptance evidence | Minimum proof needed before PASS. |

`project-inception` must not merge multiple atomic requirements into one roadmap item unless it records the reason and preserves each requirement as a separate scenario with separate evidence.

### 9. Manifest, Architecture, and Roadmap Inputs

For every requirements brief, produce these downstream inputs explicitly.

#### Manifest Inputs

Use this to shape the project manifest:

- mission / user outcome;
- product rules that must not drift;
- non-goals and explicit exclusions;
- stakeholder vocabulary and definitions;
- success criteria in business/user terms;
- risk of doing nothing.

#### Architecture Inputs

Use this to help project architecture dimension the work:

- affected domains and bounded contexts;
- likely frontend/backend/contracts/data surfaces;
- permission and role implications;
- data continuity or persistence chain;
- integration points and external dependencies;
- observability, audit, event, or timeline implications;
- complexity/risk estimate per atomic requirement;
- unknowns that block safe sizing.

#### SDD Roadmap Inputs

Use this to seed the roadmap:

- ordered atomic requirement IDs;
- proposed SDD change name per small requirement or tightly coupled group;
- dependencies and sequencing rationale;
- minimum PASS evidence;
- keywords that must appear in proposal/spec/tasks/verify;
- split recommendations when a requirement is too large;
- reviewer-burden risk.

## Execution Steps

1. Identify source type: transcript, story, support note, chat, meeting summary, screenshot feedback, or mixed input.
2. Run contradiction and ambiguity pre-analysis before deriving requirements.
3. If the pre-analysis finds a blocking contradiction or ambiguity, ask the user to resolve the first one only, then stop and wait.
4. After each user answer, incorporate the decision and continue pre-analysis until the next unresolved issue; repeat one by one.
5. Extract stakeholder anchors as exact quotes or close paraphrases only after blocking issues are resolved or explicitly approved as assumptions.
6. Cluster anchors into concerns: data continuity, permissions, UI state, workflows, visual/theme, notifications, reporting, integrations, operations.
7. For each concern, write the business intent in plain language.
8. Split every concern into atomic requirements: one fix, feature, permission rule, visual correction, or data continuity rule per requirement.
9. Assign each atomic requirement an ID, type, priority, dependency, and keyword set.
10. Translate each atomic requirement using MUST/SHOULD/MUST NOT.
11. Add acceptance scenarios using Given/When/Then.
12. Build a traceability matrix from source anchor to requirement to acceptance evidence.
13. Order the requirements for `project-inception` and mark any item that is too large and must be split.
14. Derive Manifest Inputs from the atomic requirements and stakeholder anchors.
15. Derive Architecture Inputs from affected domains, data flows, permissions, integrations, and risks.
16. Derive SDD Roadmap Inputs with ordered small change candidates and required evidence.
17. Mark assumptions, blocking questions, contradictions, and out-of-scope items.
18. Propose SDD change candidates with stable change ids, dependency order, and risk.
19. Add a verification checklist that future SDD verify agents must use before PASS/archive.

## Output Contract

Return this structure:

If blocking contradictions or ambiguities exist, return only this decision prompt and then stop:

```markdown
## Contradiction / Ambiguity Decision Needed — <Issue ID>

**Source anchors:**
- “<exact stakeholder phrase>”

**Problem:** <why the transcript cannot safely become requirements yet>

**Why it matters:** <permission/data/workflow/user-visible impact>

**Options:**
- A. <resolution option and tradeoff>
- B. <resolution option and tradeoff>

**Question:** Which option should we use for <Issue ID>?
```

After all blocking issues are resolved, return this structure:

```markdown
# Requirements Brief — <topic>

## Executive Summary
<What stakeholders need and why it matters.>

## Source Inputs
| Source | Description | Confidence |
|---|---|---|

## Stakeholder Anchors
| Anchor | Speaker/Source | Interpretation | Status |
|---|---|---|---|

## Contradictions and Ambiguities Pre-Analysis
| Issue ID | Type | Conflicting / Ambiguous Anchors | Why It Matters | Resolution Options | Decision Needed |
|---|---|---|---|---|---|

<!-- Promotion note: when promoting this brief to a delta spec, reformat each `### R-NNN — <name>` heading to `### Requirement: {Name}` with `ID: R-NNN` as the first body line — never embed the ID in the heading (archive matches by exact heading). -->

## Requirements
### R-001 — <requirement name>
**Type:** <fix | feature | visual correction | permission rule | data continuity | technical debt>
**Size:** <small | medium | must split>
**Order:** <number>
**Keywords:** <comma-separated stakeholder + technical search terms>
**Source anchor:** <quote/paraphrase>
**Intent:** <business reason>
**Requirement:** {EARS sentence — e.g. "WHEN {trigger}, the system SHALL {observable response}." One SHALL only.}
**Acceptance scenarios:**
- GIVEN ... WHEN ... THEN ...
**Evidence required:** <test/visual/API/persistence/auth evidence>
**Ambiguities:** <none or question>

## Traceability Matrix
| Source Anchor | Requirement | Keywords | Scenario | Evidence | Status |
|---|---|---|---|---|---|

## Atomic Requirement Order
| Order | Requirement | Type | Size | Depends On | Why This Order |
|---:|---|---|---|---|---|

## Manifest Inputs
### Mission / Outcome
<What the manifest must protect.>

### Product Rules
- <Rule that must not drift.>

### Non-Goals
- <Explicit exclusion.>

### Success Criteria
- <Business/user-visible success measure.>

## Architecture Inputs
| Requirement | Affected Domains | Likely Surfaces | Data/Permission Impact | Risk | Sizing Notes |
|---|---|---|---|---|---|

## Open Questions
| Question | Why it blocks | Suggested owner |
|---|---|---|

## Out of Scope
| Item | Reason |
|---|---|

## SDD Change Candidates
| Change ID | Requirement IDs | Goal | Keywords | Depends On | Risk | Suggested first phase |
|---|---|---|---|---|---|---|

## Project-Inception Handoff
| Roadmap Order | Requirement ID | Manifest Input | Architecture Input | Suggested SDD Change | Required Keywords In SDD | Minimum PASS Evidence |
|---:|---|---|---|---|---|---|

## Verification Gate
- [ ] Every stakeholder anchor is mapped or explicitly out of scope.
- [ ] Contradictions and ambiguities were analyzed before requirements were finalized.
- [ ] Every blocking contradiction or ambiguity was resolved one by one before the requirements brief was drafted, unless the user explicitly approved continuing with named assumptions.
- [ ] Every requirement has at least one acceptance scenario.
- [ ] Every requirement is atomic; large requirements are split before project-inception.
- [ ] Every requirement has stakeholder-language and technical keywords.
- [ ] Atomic requirements are ordered for project-inception.
- [ ] Manifest Inputs capture mission, product rules, non-goals, and success criteria.
- [ ] Architecture Inputs capture affected surfaces, data/permission impact, risks, and sizing notes.
- [ ] SDD Roadmap Inputs preserve atomic requirement IDs instead of broad themes.
- [ ] Every scenario has evidence type and target surface.
- [ ] Ambiguous business terms are defined before implementation.
- [ ] Review-page display values are proven to match submitted payloads.
- [ ] Permission requirements have both capability projection and endpoint guard evidence.
```

## Quality Bar

- Requirements must be understandable without the transcript.
- Contradictions and ambiguities must be surfaced before the requirements list, with options that let the user decide instead of letting the agent guess.
- Requirements must be small by default; if not small, split them before SDD planning.
- Keywords must make Engram retrieval easy months later.
- The traceability matrix must make missing coverage obvious.
- The project-inception handoff must preserve atomic requirements instead of merging them into broad roadmap themes.
- The manifest must reflect stakeholder needs, not implementation guesses.
- Architecture inputs must make sizing possible before roadmap planning.
- The SDD roadmap must be accurate because it is derived from small explicit requirements, not from summarized intent.
- If implementation agents later narrow scope, they must mark the row as partial and request confirmation.
- PASS is forbidden when any critical requirement has no evidence.

## Anti-Patterns

- “Improve UX” without observable behavior.
- Silently resolving a contradiction or ambiguity instead of exposing it and letting the user choose.
- “Fix priority flow” as one requirement when it contains proposal display, ticket persistence, and responsible edit behavior.
- Roadmap entries that group many fixes under one SDD without preserving separate requirement IDs and evidence.
- Missing keywords that make the requirement impossible to find later in Engram.
- Manifest input that omits the stakeholder problem and only lists implementation ideas.
- Architecture input that does not mention affected domains, permissions, persistence, or integration risk.
- SDD roadmap seeds that are broad themes instead of ordered small requirements.
- “Responsible user” without defining the exact role/capability.
- UI acceptance that does not check submitted payload.
- Backend-only verification for a user-visible issue.
- Tests for only `problem` when the story also covers `feature`.
- Roadmap marked complete from archive status alone instead of source-anchor evidence.
