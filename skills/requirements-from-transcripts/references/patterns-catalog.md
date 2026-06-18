# requirements-from-transcripts — Patterns Catalog

## EARS Sentence Patterns

| Pattern | Template | Use when |
|---------|----------|----------|
| Ubiquitous | `The {system} SHALL {response}.` | Always-true invariant. |
| Event-driven | `WHEN {trigger}, the {system} SHALL {response}.` | A discrete event occurs. |
| State-driven | `WHILE {state}, the {system} SHALL {response}.` | Behavior holds during a state. |
| Unwanted behavior | `IF {condition}, THEN the {system} SHALL {response}.` | Errors, validation, edge cases. |
| Optional feature | `WHERE {feature present}, the {system} SHALL {response}.` | Gated behavior. |

One SHALL per requirement — if two behaviors are needed, write two requirements.

**Banned soft verbs as the SHALL response**: `handle`, `support`, `manage`, `improve`, `might`, `could`, `would`, `robust`, `user-friendly`, `if possible`, `as appropriate`. Name the actual action instead: return / display / persist / reject / emit / set / block.

---

## Contradiction and Ambiguity Classification

| Issue Type | Meaning | Required Action |
|---|---|---|
| Contradiction | Two or more transcript statements cannot all be true or implemented together as-is. | Quote the conflicting anchors, explain the conflict, propose resolution options, and leave the choice to the user. |
| Ambiguity | A term or request has multiple valid interpretations. | Explain the possible meanings and ask for/record the needed definition before implementation. |
| Missing rule | The transcript states desired behavior but omits a policy, boundary, state, role, or data rule needed to implement safely. | Mark as blocking when it affects permissions, persistence, workflow, money, compliance, or user-visible behavior. |
| Assumption | A likely interpretation is useful for planning but not explicitly stated. | Label it as an assumption and require confirmation before SDD apply. |

---

## Atomic Requirement Splitting Examples

| Raw Stakeholder Input | Correct Split |
|---|---|
| "La propuesta muestra Media y el responsable no puede editar prioridad." | R-001 Proposal must show selected creation priority. R-002 Current responsible must be able to edit priority. |
| "Arreglar scrollbars y tema oscuro." | R-001 Scrollbar thumb/track must use theme tokens. R-002 Scrollbars must be visually verified in light mode. R-003 Scrollbars must be visually verified in dark mode. |
| "Mejorar la creación de tickets." | BLOCKED until split into concrete observable failures. |

---

## Ambiguous Term Examples

| Term Example | Must Clarify |
|---|---|
| responsable | Current assignee, area owner, subarea responsible, admin, collaborator, or capability holder? |
| prioridad | UI label, request metadata, proposal draft value, create payload, persisted current state, or editable ticket state? |
| propuesta | Agent draft, review page display, POST payload, or final ticket preview? |
| congruente | Same value, same color token, same label, same behavior across themes, or all of these? |
| urgente | UI label for `critical` or a separate enum value? |

---

## Keyword Index Requirements

For every atomic requirement, create a keyword set:

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

---

## Anchor Status Mapping

| Anchor Status | Meaning |
|---|---|
| Requirement | Must be implemented and verified. |
| Business rule | Defines behavior or authorization. |
| UX rule | Defines visible behavior or copy/state. |
| Open question | Too ambiguous to implement safely. |
| Out of scope | Explicitly excluded with reason. |

---

## Contradiction Pre-Analysis Examples

| Transcript Signal | Pre-Analysis Output |
|---|---|
| "Supongo que no lo pueda hacer ni el solicitante ni un colaborador." | Ambiguity: comment policy is not confirmed. Ask only this decision first: Option A: block both requester and collaborators while unassigned. Option B: block collaborators only. Stop and wait. |
| "responsable" used for current assignee and destination owner | Ambiguity: define whether responsible means current assignee, area owner, destination assignee, or capability holder. |
| "limpiar el chat" | Missing rule: decide whether to clear only active UI session or delete persisted conversation history. |

---

## Verification Evidence Types

| Requirement | Evidence Required |
|---|---|
| UI display | Component test, visual check, or screenshot acceptance. |
| Submit payload | Test asserting the exact API payload. |
| Persistence | Backend unit/integration test or DB projection check. |
| Authorization | Capability projection plus endpoint guard test. |
| Theme congruence | Token/class check plus light/dark visual or DOM evidence. |
| Event/timeline behavior | Event reducer/rendering test. |

---

## Behavior Chain Pattern

For workflow requirements, define the full chain:

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

---

## Anti-Patterns

- "Improve UX" without observable behavior.
- Silently resolving a contradiction or ambiguity instead of exposing it and letting the user choose.
- "Fix priority flow" as one requirement when it contains proposal display, ticket persistence, and responsible edit behavior.
- Roadmap entries that group many fixes under one SDD without preserving separate requirement IDs and evidence.
- Missing keywords that make the requirement impossible to find later in Engram.
- Manifest input that omits the stakeholder problem and only lists implementation ideas.
- Architecture input that does not mention affected domains, permissions, persistence, or integration risk.
- SDD roadmap seeds that are broad themes instead of ordered small requirements.
- "Responsible user" without defining the exact role/capability.
- UI acceptance that does not check submitted payload.
- Backend-only verification for a user-visible issue.
- Tests for only `problem` when the story also covers `feature`.
- Roadmap marked complete from archive status alone instead of source-anchor evidence.

---

## Acceptance Scenario Variants to Cover

- ticket type: `problem`, `feature`, `question` if applicable;
- priority: `low`, `medium`, `high`, `critical`;
- role: requester, current assignee, collaborator, same-subarea viewer, admin;
- state: new, open/in progress, paused, resolved, cancelled;
- theme: light and dark when visual congruence is requested.

---

## Project-Inception Handoff Fields

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

---

## Downstream Handoff Layers

| Layer | Receives | Must Answer |
|---|---|---|
| Manifest | stakeholder intent, mission, non-negotiable rules, success criteria, scope/out-of-scope | What outcome do we need and what must never drift? |
| Architecture | domains, data flows, integrations, permissions, persistence, UI surfaces, risks | What systems/files/contracts are affected and how big is this? |
| SDD Roadmap | ordered atomic requirements, dependencies, evidence requirements, risk | In what sequence do we implement small accurate changes? |
