# requirements-from-transcripts — Full Output Template

## Contradiction/Ambiguity Decision Prompt

When blocking contradictions or ambiguities exist, return ONLY this and stop:

```markdown
## Contradiction / Ambiguity Decision Needed — <Issue ID>

**Source anchors:**
- "<exact stakeholder phrase>"

**Problem:** <why the transcript cannot safely become requirements yet>

**Why it matters:** <permission/data/workflow/user-visible impact>

**Options:**
- A. <resolution option and tradeoff>
- B. <resolution option and tradeoff>

**Question:** Which option should we use for <Issue ID>?
```

---

## Full Requirements Brief Template

After all blocking issues are resolved:

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
**Scope:** <new-capability | feature | fix>
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
