# Design: OO Quality Contract Runtime Wiring

## Technical Approach

Introduce a small contract-evaluation model shared by the Claude gate and OpenCode plugin config. Existing phase-only contracts continue to evaluate from `applies_to_phases`, `excluded_phases`, and `injection_point`. Contracts with `language_context` or `activation_context` require explicit trusted work-context metadata before injection; missing, malformed, unsupported, or insufficient metadata is pass-through for that contract.

## Architecture Decisions

| Decision | Choice | Alternatives considered | Rationale |
|---|---|---|---|
| Runtime model | Add a reusable `ContractDecision`/`WorkContext` concept in `engine/gate/gate.go` and mirror the serialized shape in OpenCode config. | Keep one `ContractPath` in `gate.Config`; duplicate special cases per contract. | Independent contract decisions are required, and single-contract state would make OO behavior brittle. |
| OO activation | Require explicit trusted metadata fields, not prompt text. | Infer from prompt words like TypeScript, NestJS, SOLID, or domain. | The spec forbids prompt heuristics; metadata makes pass-through deterministic and testable. |
| Backward compatibility | Treat contracts without context requirements as phase-only. | Force all contracts to declare context. | Preserves current minimalism and skill-discovery-safety behavior. |
| Failure mode | Invalid contract metadata or unsupported context operators skip only that contract. | Fail the whole hook or inject all valid phase matches. | Existing runtime is fail-safe; one bad contract must not block other valid contracts. |

## Data Flow

    Runtime adapter ── loads contract specs ──→ prompt_config/contracts[]
          │                                      │
          └─ trusted work_context ───────────────┘
                                                 ↓
       Claude gate / OpenCode plugin ── evaluate each contract ── mutate prompt or pass through

Evaluation order per contract:

1. Parse metadata.
2. If phase is excluded, strip exact contract path.
3. If phase is not included, pass through.
4. If context requirements exist, require trusted `WorkContext` match.
5. Inject exact contract path once, otherwise pass through.

## File Changes

| File | Action | Description |
|---|---|---|
| `engine/gate/gate.go` | Modify | Extend `Config` to accept multiple contracts and optional trusted `WorkContext`; preserve current `ContractPath` path for compatibility or adapt through a single-contract wrapper. |
| `engine/cmd/main.go` | Modify | Parse optional explicit work-context JSON/file flags for `gate-task`; absent or invalid context remains pass-through for context-aware contracts. |
| `engine/runtime/opencode.go` | Modify | Replace `openCodePromptConfig` single-contract fields with `contracts[]` entries containing path, phase scopes, injection point, and context requirements. Bump plugin version/hash behavior naturally through source/config changes. |
| `engine/runtime/labdrian-runtime-parity-plugin.mjs` | Modify | Evaluate `prompt_config.contracts[]` independently and require trusted context before OO injection. Preserve current `mutatePrompt(prompt, subagentType, config)` behavior when config has only phase-only contracts. |
| `engine/settings/settings.go` | Modify | Add distinct OO hook identity only if Claude wiring is enabled; do not collapse ownership with minimalism or safety identities. |
| `skills/_shared/oo-quality-contract.md` | Reference | Use existing frontmatter as the source for context requirements; no content change required. |

## Interfaces / Contracts

```go
type WorkContext struct {
    Trusted bool
    Languages []string
    Activations []string
    WorkKinds []string // e.g. application-code, docs, config, generated-artifact
}

type ContractConfig struct {
    Path string
    Content string
    ContextRequired bool
}
```

OpenCode config should serialize `prompt_config.contracts[]` with snake_case equivalents plus `language_context` and `activation_context` arrays.

## Testing Strategy

| Layer | What to Test | Approach |
|---|---|---|
| Unit | Gate decisions for phase-only, OO matched context, missing context, malformed metadata, and unsupported context. | Table-driven Go tests in `engine/gate/gate_test.go`; assert exact pass-through `{}` or one injected path. |
| Integration | CLI and settings preserve lifecycle behavior and distinct ownership identities. | Extend `engine/cmd/main_test.go`, `engine/settings/settings_test.go`, and runtime adapter tests using `t.TempDir()`. |
| E2E | OpenCode plugin mutates prompts from multi-contract config without prompt heuristics. | Extend Node-backed test in `engine/runtime/opencode_test.go`; skip when Node is unavailable. |

## Migration / Rollout

No data migration required. Runtime install/update should rewrite managed plugin/config artifacts and report restart-required as it does today. If OO Claude hook identity is not enabled in this slice, keep it absent and document pass-through.

## Trusted Context Boundary

- OpenCode runtime injection trusts only per-invocation `work_context` carried on the current hook payload.
- Static or persisted top-level `work_context` in `labdrian-runtime-parity.json` is ignored for contract decisions.
- When the hook payload does not include trusted per-invocation context, context-aware contracts pass through unchanged.
