# project-architect — Architecture Document Template

This is an outline: render new artifacts and envelopes in English, except verbatim user quotes. Preserve existing Spanish records. Every section is mandatory except where noted. For explicitly selected inline context, cite its provenance and supplied rules; do not claim compliance with an absent persisted manifest. Mark inferred material `[SUPUESTO SIN MANIFEST]`.

```markdown
# Technical Architecture — {Project Name}

**Date**: {current date}
**Based on**: {For inline context, cite its provenance and supplied rules instead of absent manifest records.}
- Manifest context: `project/{project}/manifest/context`
- Manifest rules: `project/{project}/manifest/rules`
- Manifest mission (if applicable): `project/{project}/manifest/mission`
- Technical stack: `sdd-init/{project}`
- Previous version (if applicable): `project/{project}/architect/final`

---

## 1. Architecture Style

**Choice**: {Modular monolith | Hexagonal | Clean Architecture | Layered | Event-Driven | etc.}

**Justification**:
{Why this style, tied explicitly to manifest constraints and preferences. Reference specific manifest sections. Do NOT default to "industry standard" — every style choice must be defended against THIS manifest.}

**Respects the manifest preference**: {Yes/No + explanation}. If the manifest preferred another style, explain with technical evidence why this one is better.

---

## 2. Modules and Technical Contracts

{For each conceptual module in the manifest, provide a technical contract:}

### 2.1 Module: {Name}

**Purpose**: {One sentence}
**Single responsibility**: {What it owns, what it does NOT own}
**Inputs**: {Data/events that enter this module — types and sources}
**Outputs**: {Data/events that leave this module — types and destinations}
**Invariants**: {Conditions this module guarantees at all times}
**Authorized decisions**: {What this module is allowed to decide}
**Forbidden decisions**: {What this module is NOT allowed to decide}
**Allowed dependencies**: {Modules or libraries it may depend on}
**Forbidden dependencies**: {Modules or libraries it must NOT depend on — and why}
**Persistence**: {Where this module stores state, if any}
**Technical risks**: {Specific risks tied to this module's design}

{Repeat for every conceptual module identified in the manifest. If a conceptual module is too vague to produce a technical contract, mark it as `[PENDIENTE DE FORMALIZACIÓN]` and explain what additional info is needed.}

---

## 3. Key Components (technical detail)

{For modules that contain multiple components, break them down:}

### 3.1 Component: {Name}
**Parent module**: {Module it belongs to}
**Responsibility**: {What it does}
**Public interface**: {Methods/endpoints exposed}
**Dependencies**: {What it depends on}
**Boundaries**: {What it does NOT do}

{Only include components that are non-trivial or that have specific boundaries worth documenting. Do not inflate this section with obvious components.}

---

## 4. Data Model (high level)

**Approach**: {Relational SQL | Document NoSQL | Event Sourcing | CQRS | hybrid}
**Justification**: {Tied to manifest and stack}

### 4.1 Main entities

{For each main entity:}

**{Entity Name}**
- Key attributes: {list}
- Relationships: {with which other entities and how}
- Invariants: {constraints that must always hold}
- Lifecycle: {when created, when archived/deleted, who can mutate}

### 4.2 Persistence strategy

{How data is actually stored: single DB, multiple DBs, caching layers, read models, etc.}

### 4.3 Consistency

{Strong/eventual, transactional boundaries, conflict resolution}

---

## 5. External Integrations

{For each external system the project must integrate with:}

### 5.1 Integration: {Name}
**What it is**: {External system}
**Why it is needed**: {Reason tied to manifest}
**Integration method**: {REST / gRPC / WebSocket / Webhook / SDK / Database / Message Queue}
**Frequency**: {Real-time / batch / on-demand}
**Error handling**: {Retry, circuit breaker, fallback, etc.}
**Coupling**: {How we isolate the rest of the system from this integration's instability}

{If the project has no external integrations in this phase, state "There are no external integrations in this phase" and explain why.}

---

## 6. Authentication and Authorization

**Authentication mechanism**: {JWT / OAuth / Session / API Keys / None}
**Justification**: {Why this mechanism for this project}

**Authorization model**: {RBAC / ABAC / ACL / None}
**Roles/scopes**: {If applicable, list them}

**Authentication flows**:
{Describe the main flows: login, logout, token refresh, permission check}

**Boundaries**:
{What this auth model does NOT cover and when we'd need to change it}

{If the project has no auth needs in this phase, state so explicitly.}

---

## 7. Security

**Threats considered**: {List the main threats relevant to this project}
**Mitigations implemented**: {For each threat, what defensive measure}
**Sensitive data**: {What's sensitive, how it's protected, where it lives}
**Auditability**: {What gets logged for security auditing}
**Compliance if applicable**: {PCI, HIPAA, GDPR, etc. — only if the manifest or domain requires it}

---

## 8. Scalability and Performance

**Expected load (initial)**: {Numbers: requests/sec, concurrent users, data volume}
**Expected load (12 months)**: {Projection}
**Scaling strategy**: {Vertical / horizontal / hybrid + when each kicks in}
**Known bottlenecks**: {What will bottleneck first and how we handle it}
**Target SLOs**: {Latency, availability, error rate}

{Be honest — if the project is small and scale doesn't matter yet, say so. Don't inflate this section with premature optimization.}

---

## 9. Observability

**Logging**: {Structure, levels, destinations}
**Metrics**: {What we measure, where they're exposed}
**Tracing**: {If applicable, how distributed tracing works}
**Health checks**: {Liveness, readiness, custom health}
**Alerts**: {What triggers alerts, who receives them}
**Auditability (linked to manifest)**: {Specifically address manifest Rule 5 — "Mandatory traceability". How is every decision reconstructible?}

---

## 10. Deployment and Operations

**Environments**: {dev / staging / prod — or simpler if the manifest constrains it}
**CI/CD**: {Pipeline, testing gates, deployment strategy}
**Configuration**: {How config is managed across environments — env vars, secret stores, etc.}
**Rollback**: {How we roll back a bad deployment}
**Backup and recovery**: {If applicable}

---

## 11. Technical Risks

{NOT strategic risks — those live in manifest Doc 1. Here we document technical risks specific to this architecture:}

| Risk | Probability | Impact | Mitigation |
|--------|--------------|---------|------------|
| {Risk 1} | {High/Medium/Low} | {High/Medium/Low} | {How we handle it} |
| {Risk 2} | ... | ... | ... |

---

## 12. Explicit Trade-offs

{For each major architectural decision, document what was given up:}

### 12.1 Trade-off: {Decision}
**We chose**: {What}
**We gave up**: {What we gave up}
**Why it was acceptable**: {Justification tied to manifest constraints}

{Document real trade-offs supported by the decisions and evidence; do not invent choices to fill a quota.}

---

## 13. Rejected Alternatives

{For major decisions (style, persistence, auth, integrations), 1-2 alternatives considered and rejected:}

### 13.1 Alternative: {Name}
**What it was**: {Description}
**Why it was considered**: {Reason}
**Why it was rejected**: {Concrete reason tied to manifest or stack, NOT "personal preference"}

---

## 14. Future Evolution

**Next 3 months**: {What stays the same, what might change}
**6–12 months**: {Projected evolution}
**Turning points**: {When we'd need to revisit this architecture from scratch}
**What this architecture does NOT support**: {Be explicit about what future needs would break this design}

---

## 15. Manifest Rules Compliance

{This section is MANDATORY. For each mandatory rule from `manifest_rules`, document explicitly how the architecture respects it:}

| Manifest rule | How the architecture respects it |
|--------------------|--------------------------------|
| {Rule 1} | {Specific architectural decision or pattern that enforces this rule} |
| {Rule 2} | ... |

{If any rule is NOT respected by the architecture, you have two options:
(a) Redesign the architecture to respect it (preferred)
(b) Document the tension explicitly as a trade-off and propose mitigation
You may NOT silently skip a rule.}

---

## Status
**Version**: {1 if first, n+1 if refinement}
**Ready for**: Implementation phase (SDD pipeline or manual work)
**Recommended next steps**:
- Hand off to the parent; see repository-relative skill identities `skills/project-inception/SKILL.md` and `skills/role-chain/SKILL.md` for the current route.
- For manual work, this architecture is binding guidance for the team
- Re-run `/project-architect` if the manifest changes significantly
```

---

## Structured Response Envelope

Return EXACTLY this format to the orchestrator:

```markdown
## Project Architect — {status}

**Project**: {project name}
**Status**: {blocked | proceed | updated}
**Artifact store mode**: {engram | openspec | hybrid | none}
**Artifacts**: {actual memorycard topic keys and/or file paths persisted, or "none" if blocked}

### Executive Summary
{Summarize what was produced in English. If blocked, summarize why.}

### Key Decisions
{If proceed: 3-5 bullet points with the most important architectural decisions. If blocked: "None — blocked by missing inputs."}

### Trade-offs Summary
{If proceed: list the real trade-offs. If blocked: omit.}

### Next Step
{What happens next:
- blocked: "The owner must choose Option 1, 2, or 3 (see `blocked-response.md`)."
- proceed: hand the proposed architecture to the parent, using the current skill paths above.
- updated: hand the updated architecture and actual storage locators to the parent.}

### Risks
- {Top technical risk 1}
- {Top technical risk 2}

### Warnings
{If sdd-init context was not found, warn here.}
{If manifest was partial, warn about which sections were missing.}
{If architecture tensions exist with manifest rules, warn here.}
```
