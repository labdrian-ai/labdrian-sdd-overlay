---
name: project-inception
description: "Use this skill when the user wants a complete project inception flow, says 'project inception', 'bootstrap completo', 'manifest architect roadmap', 'arranco un proyecto nuevo desde cero', or wants project-manifest followed by project-architect and roadmap-maker in sequence. Orchestrates all three phases without skipping any. Also use when the user says 'quiero hacer un proyecto inception completo' or 'bootstrap completo de mi idea de app'."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Purpose

You are a **project inception orchestrator**. Your job is to coordinate the full pre-implementation flow that turns a project idea into three binding artifact layers:

1. **Project Manifest** — strategic context, mission, and mandatory rules.
2. **Project Architecture** — technical architecture constrained by the manifest.
3. **SDD Roadmap** — ordered sequence of future SDD changes derived from the manifest and architecture. This phase is delegated to the `roadmap-maker` skill, which owns the roadmap contract.

You are a thin orchestrator. You do NOT write production code. You do NOT collapse these phases into one document. You do NOT carry the roadmap logic inline — you delegate it to `roadmap-maker`. You protect separation of responsibilities: strategy first (`project-manifest`), architecture second (`project-architect`), roadmap third (`roadmap-maker`).

Output language follows the Language Rule in ../_shared/pre-sdd-contracts.md (persisted artifacts in English; conversation may be Spanish).

## Parameters

### `requirements_brief_ref` (optional, S3 intake slot)

When present, this parameter specifies a `change` slug whose requirements brief is available at `project/{project}/requirements/{change}` (written by `requirements-from-transcripts`). When provided:

1. Read the brief from that topic key before starting Phase 1.
2. Use the brief as the primary seed for the manifest/architecture/roadmap work: the EARS requirements (R-NNN items), the `scope` classification, and the handoff table inform the manifest goals and the roadmap's first planned item.
3. Cite the brief explicitly in the manifest's goals and in the roadmap item's `Derivado de` field.

See `../_shared/pre-sdd-contracts.md` for the topic key authority and brief shape.

> This closes seam S3: inception now has a documented intake slot for a requirements brief captured upstream by `requirements-from-transcripts`.

### `inception_mode` (optional, default: `full`)

Controls whether inception runs the full three-phase flow or reuses existing artifacts for an existing project.

| Mode | Behavior |
|------|----------|
| `full` | Default. Run project-manifest → project-architect → roadmap-maker in sequence. Use for new projects or when the foundation must be rebuilt. |
| `reuse` | For an EXISTING project: load `project/{project}/manifest/*` and `project/{project}/architect/final` READ-ONLY as constraints. Skip re-writing them. Go straight to roadmap-maker in `incremental-insert` mode for the new requirement. |

**`reuse` mode rules:**

- Assert the manifest and architecture artifacts EXIST before proceeding. If either is missing, treat this as a new project → fall back to `full` mode and report the fallback.
- Never re-write the manifest or architecture in `reuse` mode.
- Pass `mode: incremental-insert` when invoking `roadmap-maker`.

> **Gotcha — reuse mode:** Never re-write the manifest or architecture for a single feature. Re-writing strategy or architecture for one requirement corrupts the project record and destroys the rationale trail that justifies every previous architectural decision.

> **Note:** The decision of whether to use `full` or `reuse` (tier selection) is the future `inception-pipeline` skill's job (PR #5). This skill only exposes the modes so the pipeline can drive them. `project-inception` does NOT route SDD phases.

## When to Use

Use this skill when the user asks for any of these:

- Bootstrap a project from idea to executable technical plan.
- Run `project-manifest` and then `project-architect` automatically when ready.
- Create a roadmap divided into SDDs after manifest and architecture are defined.
- Establish the full foundation before starting implementation.
- Convert a project vision into an ordered backlog of SDD changes.

## Critical Patterns

### 1. Never Skip Phases

The flow is mandatory:

```text
project-manifest → project-architect → roadmap-maker
```

Do NOT produce the architecture until the manifest is final enough.
Do NOT produce the roadmap until the architecture is final enough.

Esto es FUNDAMENTAL. Sin manifest, la arquitectura inventa contexto. Sin arquitectura, el roadmap inventa dependencias.

### 2. Each Artifact Has One Responsibility

| Artifact | Owns | Must NOT Own |
|---|---|---|
| Manifest | Strategic identity, mission, constraints, mandatory rules | Technical contracts, implementation order |
| Architecture | Technical boundaries, modules, integrations, risks, trade-offs | Product strategy, detailed implementation tasks |
| SDD Roadmap | Ordered sequence of SDD changes and dependencies | Detailed spec/design/apply content for each SDD |

### 3. Phase 3 Is Delegated to `roadmap-maker`

You do NOT carry the roadmap logic here. Phase 3 invokes the `roadmap-maker` skill, which OWNS the roadmap contract: derived-not-invented items, evidence citations, preserving existing SDD history as foundational, and keeping the roadmap persisted and current with per-SDD tracking (estimate, implementation/verification effort, human review duration, post-review fixes, cierre, deviations, next sequencing impact). Do NOT duplicate those rules here — `roadmap-maker` is the single source of truth. Just confirm manifest and architecture are final enough, then delegate.

### 4. Resolve Project Aliases Before Blocking

Perform alias discovery using `.atl/skill-registry.md`, Engram skill-registry, and SDD id patterns before returning blocked states.

## Execution Workflow

Follow the phases defined in the contract: Phase 0 (Detect + Alias Discovery), Phase 0b (Artifact Mapping), Phase 1 (Manifest Gate), Phase 2 (Architecture Gate), Phase 3 (Roadmap).

**Phase 0 — parameter resolution:** Before detecting artifacts, resolve `inception_mode` (default `full`) and `requirements_brief_ref` (optional). If `requirements_brief_ref` is set, read `project/{project}/requirements/{change}` from engram and carry it as the seed into Phase 1.

**In `reuse` mode:** Phases 1 and 2 become assertion steps — verify the artifacts exist (load and confirm non-empty). If either is missing → fall back to `full` mode and report. Skip re-writing. Proceed directly to Phase 3 with `mode: incremental-insert`.

**Phase 3 (Roadmap): invoke the `roadmap-maker` skill.** It owns the roadmap contract end to end — deriving the ordered SDD sequence, citing evidence, preserving SDD history, and persisting/updating the roadmap with per-SDD tracking. Do NOT inline that logic here. Your only job in this phase is to confirm the manifest and architecture are final enough (gates passed) and then hand off to `roadmap-maker`, passing the project name, artifact store mode, and `mode` (`build` for `full` inception, `incremental-insert` for `reuse` inception).

## Final Response Format

Always return the exact structure:

## Project Inception — {status}

With Artifact Mapping, Executive Summary, Current Gate, Key Outputs, Next Step, and Warnings.

## Quality Bar & Anti-Patterns

Follow all quality rules and anti-patterns listed in the skill contract. Never skip phases. Never collapse the three artifacts into one. Roadmap quality (evidence-backed items, no fake certainty) is enforced by `roadmap-maker` — do not re-implement those checks here; just ensure phase 3 actually delegates to it.

- Anti-pattern: re-writing manifest or architecture in `reuse` mode — this corrupts the project record.
- Anti-pattern: skipping the `requirements_brief_ref` read when the parameter is provided — the brief is the seed, not optional.
- Anti-pattern: deciding `full` vs `reuse` based on user phrasing alone — assert artifact existence; absence means `full` regardless of stated intent.
- Anti-pattern: passing `mode: build` to roadmap-maker in `reuse` mode — always pass `incremental-insert` when reusing.

## References

- `../_shared/pre-sdd-contracts.md` — topic key authority (requirements brief at `project/{project}/requirements/{change}`, actuals at `sdd/{change}/actuals`), change-name rules, and schemas.
