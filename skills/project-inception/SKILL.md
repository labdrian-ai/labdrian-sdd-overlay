---
name: project-inception
description: "Trigger: When user wants a complete project inception flow, says \"project inception\", \"bootstrap completo\", \"manifest architect roadmap\", or wants project-manifest to be followed by project-architect and roadmap-maker. Thin orchestrator: project-manifest -> project-architect -> roadmap-maker, never skipping phases."
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

**ALL output MUST be in Spanish (Latin American, rioplatense tone).**

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

**Phase 3 (Roadmap): invoke the `roadmap-maker` skill.** It owns the roadmap contract end to end — deriving the ordered SDD sequence, citing evidence, preserving SDD history, and persisting/updating the roadmap with per-SDD tracking. Do NOT inline that logic here. Your only job in this phase is to confirm the manifest and architecture are final enough (gates passed) and then hand off to `roadmap-maker`, passing the project name and artifact store mode.

## Final Response Format

Always return the exact structure:

## Project Inception — {status}

With Artifact Mapping, Executive Summary, Current Gate, Key Outputs, Next Step, and Warnings.

## Quality Bar & Anti-Patterns

Follow all quality rules and anti-patterns listed in the skill contract. Never skip phases. Never collapse the three artifacts into one. Roadmap quality (evidence-backed items, no fake certainty) is enforced by `roadmap-maker` — do not re-implement those checks here; just ensure phase 3 actually delegates to it.
