# Design: OO Quality Contract

## Technical Approach

Implement the smallest coherent first slice as a manifest-tracked shared contract plus artifact-level validation. The contract will live under `skills/_shared/`, not as a standalone `SKILL.md`, and will be advisory only. This satisfies R-001 through R-008 without adding propagation, gate, or runtime injection behavior in this change.

## Architecture Decisions

| Option | Tradeoff | Decision |
| --- | --- | --- |
| Shared Markdown contract at `skills/_shared/oo-quality-contract.md` | Matches existing shared-contract pattern and avoids over-broad skill matching; requires explicit future wiring for runtime injection. | Use this path for the first slice. |
| Standalone `skills/oo-quality-contract/SKILL.md` | Easier discovery through normal skill registry, but risks global OO/SOLID activation. | Reject for this slice. |
| Go artifact tests in `engine/skills` | Reuses existing Go test workflow and gives stable content assertions; tests repository artifacts rather than engine behavior. | Add `engine/skills/oo_quality_contract_artifact_test.go`. |
| Shell validation script | Simple, but introduces a new script pattern where none currently exists. | Reject unless Go tests prove insufficient. |

## Data Flow

```text
SDD apply task
  ├─ creates skills/_shared/oo-quality-contract.md
  ├─ adds _shared/oo-quality-contract.md custom to overlay.manifest
  └─ adds Go artifact tests
          ├─ read contract file
          ├─ parse/assert frontmatter tokens
          ├─ assert precedence/context/advisory language
          ├─ assert non-vendoring markers
          └─ assert manifest row
```

Runtime prompts are not changed in this slice.

## File Changes

| File | Action | Description |
| --- | --- | --- |
| `skills/_shared/oo-quality-contract.md` | Create | Local English-only advisory OO/domain quality contract. |
| `overlay.manifest` | Modify | Add `_shared/oo-quality-contract.md custom` in the existing `_shared/*` block, next to `minimalism-contract.md`. |
| `engine/skills/oo_quality_contract_artifact_test.go` | Create | Artifact-level tests for path, frontmatter scope, precedence, context gate, advisory posture, non-vendoring, and manifest row. |
| `.atl/skill-registry.md` | No change | Generated index; do not hand-edit for this first slice. |
| `engine/gate/*`, `engine/propagator/*`, `engine/cmd/*` | No change | Runtime injection/propagation is explicitly deferred. |

## Interfaces / Contracts

Contract frontmatter:

```yaml
---
applies_to_phases: [sdd-design, sdd-tasks, sdd-apply]
excluded_phases: [sdd-propose, sdd-spec, sdd-archive, sdd-verify]
injection_point: "## Skills to load before work"
language_context: [typescript, nestjs]
activation_context: [oo-domain-design, domain-heavy-application-code, review]
---
```

Content outline:
- Title: `# OO Quality Contract`
- Advisory scope: applies only to OO/domain-heavy TypeScript or NestJS application work.
- Precedence: specs, design, project conventions, minimalism contract, and review budget win over this contract.
- Context gate: pass through for Go, shell, docs, config, generated artifacts, non-domain work, and non-OO changes.
- Guidance: use SOLID as diagnostic vocabulary only; justify abstractions, value objects, entities, and patterns by invariants, active variation, or tested seams.
- Testing: do not impose TDD unless project or task config already requires it.
- Review: report advisory warnings tied to concrete risk, not style violations.

## Testing Strategy

| Layer | What to Test | Approach |
| --- | --- | --- |
| Unit/artifact | Contract file exists and frontmatter scopes design/tasks/apply while excluding propose/spec/archive/verify. | Go test reads `../../skills/_shared/oo-quality-contract.md` and asserts stable tokens. |
| Unit/artifact | Precedence, context gate, advisory behavior, TDD non-mandate, and non-vendoring. | Behavior-oriented string assertions for required rules and absence of external-vendor wording/paths. |
| Integration | Manifest tracks the contract. | Go test reads `../../overlay.manifest` and asserts exactly one `_shared/oo-quality-contract.md custom` row. |
| E2E | Runtime injection. | Out of scope for this slice. |

Verification commands:

```bash
cd /home/labdrian/labdrian-sdd-overlay/engine && go test ./...
cd /home/labdrian/labdrian-sdd-overlay/tui && go test ./...
git diff --check
```

## Migration / Rollout

No migration required. Rollback is to delete `skills/_shared/oo-quality-contract.md`, remove `_shared/oo-quality-contract.md custom` from `overlay.manifest`, and delete `engine/skills/oo_quality_contract_artifact_test.go`. No runtime state or generated registry file is changed.

## Open Questions

None.

## Explicit Out of Scope

- No engine propagation, gate wiring, embedded contract registration, marker-block changes, or runtime prompt injection.
- No vendoring, importing, or copying external `solid-skills`.
- No hand edits to generated `.atl/skill-registry.md`.
