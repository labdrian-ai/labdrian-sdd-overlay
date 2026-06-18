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
- Never collapse multiple concerns into one generic requirement such as "improve UX". Split by observable behavior.
- ALWAYS write the `**Requirement:**` line as ONE EARS sentence with exactly one SHALL (see EARS Sentence Format); banned soft verbs as the response verb are blocking — replace them with a specific action verb.
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
- **CHANGE-NAME AUTHORITY (mandatory):** This skill derives `change-name` exactly once, as a kebab-case slug (lowercase, hyphen-separated, no spaces or special characters, stable once set). Emit `change-name` in the requirements brief and in the Project-Inception Handoff table. Downstream stages inherit it verbatim and NEVER re-derive it. See `../_shared/pre-sdd-contracts.md` for the full identifier contract.
- **PERSISTENCE (mandatory):** After producing the requirements brief, call `mem_save` with topic key `project/{project}/requirements/{change}` and `capture_prompt: false`. The saved record must include the full set of EARS + R-NNN requirements (each with its `scope` enum: `new-capability | feature | fix`), and the Project-Inception Handoff table. See `../_shared/pre-sdd-contracts.md` for topic-key authority.

## EARS Sentence Format (CRITICAL)

Every `**Requirement:**` line MUST be ONE EARS sentence with exactly one `SHALL`. Read `references/patterns-catalog.md` for the full EARS pattern table and banned verb list.

Core patterns:

| Pattern | Template |
|---------|----------|
| Ubiquitous | `The {system} SHALL {response}.` |
| Event-driven | `WHEN {trigger}, the {system} SHALL {response}.` |
| State-driven | `WHILE {state}, the {system} SHALL {response}.` |
| Unwanted behavior | `IF {condition}, THEN the {system} SHALL {response}.` |
| Optional feature | `WHERE {feature present}, the {system} SHALL {response}.` |

Banned soft verbs as the SHALL response: `handle`, `support`, `manage`, `improve`, `might`, `could`, `would`, `robust`, `user-friendly`, `if possible`, `as appropriate`. Use: return / display / persist / reject / emit / set / block.

**ID format**: `R-001`, `R-002`, ... (zero-padded, dash). IDs are stable — assigned once, never renumbered, never reused. These IDs carry forward verbatim into sdd-spec, sdd-tasks, and sdd-verify.

## Contradiction and Ambiguity Pre-Analysis

Before extracting final atomic requirements, inspect the transcript for contradictions and ambiguous requests. This pre-analysis is mandatory.

Issue types: Contradiction, Ambiguity, Missing rule, Assumption. Read `references/patterns-catalog.md` for the full classification table and worked examples.

When blocking issues are found:
- Preserve the exact stakeholder phrases.
- Explain why the issue matters in product/technical terms.
- Propose concrete resolution options (Option A / Option B).
- State the tradeoff of each option briefly.
- Do NOT silently pick one option.
- Ask for the user's decision on exactly ONE issue at a time. Stop and wait.
- After the user resolves that issue, continue pre-analysis for the next unresolved issue.
- Only draft the requirements brief after ALL blocking contradictions and ambiguities are resolved, or after the user explicitly approves continuing with named assumptions.

Read `references/output-template.md` for the exact Contradiction/Ambiguity Decision Prompt format to return when a blocking issue is found.

## Atomic Requirement Rule

One fix or one feature equals one requirement. If a stakeholder sentence contains multiple behaviors, split it.

Requirements must be: independently understandable, independently testable, independently traceable, small enough to become one SDD requirement or one SDD slice, explicit about type (fix, feature, policy, visual correction, or technical debt item).

If a requirement cannot be tested independently, it is still too large. Read `references/patterns-catalog.md` for splitting examples.

## Execution Steps

1. Identify source type: transcript, story, support note, chat, meeting summary, screenshot feedback, or mixed input.
2. Run contradiction and ambiguity pre-analysis before deriving requirements.
3. If the pre-analysis finds a blocking contradiction or ambiguity, ask the user to resolve the first one only, then stop and wait.
4. After each user answer, incorporate the decision and continue pre-analysis until the next unresolved issue; repeat one by one.
5. Extract stakeholder anchors as exact quotes or close paraphrases only after blocking issues are resolved or explicitly approved as assumptions.
6. Cluster anchors into concerns: data continuity, permissions, UI state, workflows, visual/theme, notifications, reporting, integrations, operations.
7. For each concern, write the business intent in plain language.
8. Split every concern into atomic requirements: one fix, feature, permission rule, visual correction, or data continuity rule per requirement.
9. Assign each atomic requirement an ID, type, priority, dependency, and keyword set. Read `references/patterns-catalog.md` for the keyword index format.
10. Translate each atomic requirement using MUST/SHOULD/MUST NOT.
11. Add acceptance scenarios using Given/When/Then. Read `references/patterns-catalog.md` for the acceptance scenario variant checklist.
12. Build a traceability matrix from source anchor to requirement to acceptance evidence. Read `references/patterns-catalog.md` for evidence types by requirement category.
13. Order the requirements for `project-inception` and mark any item that is too large and must be split.
14. Derive Manifest Inputs from the atomic requirements and stakeholder anchors.
15. Derive Architecture Inputs from affected domains, data flows, permissions, integrations, and risks.
16. Derive SDD Roadmap Inputs with ordered small change candidates and required evidence.
17. Mark assumptions, blocking questions, contradictions, and out-of-scope items.
18. Propose SDD change candidates with stable change ids, dependency order, and risk.
19. Add a verification checklist that future SDD verify agents must use before PASS/archive.
20. **Derive `change-name`** as a kebab-case slug from the primary requirement theme. Emit it in the requirements brief header and in every row of the Project-Inception Handoff table. Downstream stages inherit this slug verbatim.
21. **Persist the brief** via `mem_save` to topic key `project/{project}/requirements/{change}` with `capture_prompt: false`.

## Downstream Input Layers

For every requirements brief, produce these explicitly.

**Manifest Inputs**: mission / user outcome, product rules that must not drift, non-goals and explicit exclusions, stakeholder vocabulary and definitions, success criteria in business/user terms, risk of doing nothing.

**Architecture Inputs**: affected domains and bounded contexts, likely frontend/backend/contracts/data surfaces, permission and role implications, data continuity or persistence chain, integration points and external dependencies, observability/audit/event/timeline implications, complexity/risk estimate per atomic requirement, unknowns that block safe sizing.

**SDD Roadmap Inputs**: ordered atomic requirement IDs, proposed SDD change name per small requirement or tightly coupled group, dependencies and sequencing rationale, minimum PASS evidence, keywords that must appear in proposal/spec/tasks/verify, split recommendations when a requirement is too large, reviewer-burden risk.

## Output Contract

Read `references/output-template.md` for:
- The full Contradiction/Ambiguity Decision Prompt format (return and stop when blocking issues exist).
- The complete Requirements Brief template with all sections.

## Lightweight Mode (Tier 3 — Small Fix)

Use when the entry tier is **3** (small fix): single, unambiguous fix with no stakeholder transcript and no manifest/architecture involvement required.

Emit a minimal structured requirement object:

```yaml
R-NNN: <zero-padded ID>
scope: fix
one_line_impact: <one sentence describing user-visible or system impact>
acceptance_evidence: <minimum observable proof>
change_name: <kebab-case slug derived here; inherited verbatim downstream>
```

Plus the EARS requirement sentence and the `change-name` slug.

Does NOT run full contradiction/ambiguity pre-analysis, does NOT produce Manifest/Architecture/SDD Roadmap Input layers, does NOT generate traceability matrix or verification checklist.

If any ambiguity surfaces during Lightweight Mode, immediately switch to the full pipeline.

## Quality Bar

- Requirements must be understandable without the transcript.
- Contradictions and ambiguities must be surfaced before the requirements list, with options that let the user decide instead of letting the agent guess.
- Requirements must be small by default; if not small, split them before SDD planning.
- Keywords must make Engram retrieval easy months later.
- The traceability matrix must make missing coverage obvious.
- The project-inception handoff must preserve atomic requirements instead of merging them into broad roadmap themes.
- Architecture inputs must make sizing possible before roadmap planning.
- PASS is forbidden when any critical requirement has no evidence.
