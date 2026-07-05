## Exploration: codex-runtime-lifecycle

### Current State
The runtime subsystem is already wired for a tri-runtime command surface (`claude`, `opencode`, `codex`) through `engine/runtime` adapters plus the `engine runtime` CLI, including `--target all` expansion and CLI error handling.

Codex lifecycle behavior is now concrete: `engine/runtime/codex.go` ships a conservative manifest-backed `CodexAdapter` with full lifecycle support. OpenCode and Claude are also concrete lifecycle adapters with established implementation behavior.

`runtime` CLI behavior now treats Codex as a first-class target in `--target all`; advisory handling was removed so Codex contributes real supported/partial/unsupported status.

### Affected Areas
- `engine/runtime/codex.go` — Implemented full install/status/update/uninstall support with a conservative manifest model.
- `engine/runtime/runtime.go` — shared lifecycle types, status enums, target parsing/expansion, and adapter factory; confirms codex is first-class in all targets and already participates in `all` target expansion.
- `engine/cmd/main.go` — runtime dispatch path routes actions through adapters and now reports Codex as a first-class target in `--target all` without advisory masking.
- `engine/runtime/runtime_test.go` — validates target parsing/expansion and now includes Codex foundation status expectations while preserving fallback coverage for unsupported targets.
- `engine/cmd/runtime_test.go` — covers mixed-target success/failure paths with Codex concrete states and continues to validate legacy Claude/OpenCode behavior.
- `engine/runtime/claude_test.go` and `engine/runtime/opencode_test.go` — concrete lifecycle test patterns that can be mirrored for Codex state validation and mutation/error semantics.
- `openspec/changes/archive/2026-07-03-claude-runtime-lifecycle/exploration.md` — prior slice explicitly deferred Codex and documents the remaining transition boundary.
- `openspec/specs/runtime-lifecycle/spec.md` + `openspec/project/roadmap.md` — define Codex as the next runtime lifecycle slice and the non-regression boundary relative to existing Claude/OpenCode behavior.

### Approaches
1. **Implement full native Codex lifecycle adapter now (install/status/update/uninstall) with explicit root/config model**
   - Pros: closes the roadmap slice cleanly; enables `runtime --target codex` to be real instead of advisory; aligns user expectations with the command surface.
   - Cons: highest ambiguity because Codex runtime storage/schema/activation model is not yet formalized in code, so product alignment is required before final file/marker format is stable.
   - Effort: High

2. **Stage Codex support by introducing status-first behavior first, then install/uninstall/update in follow-up slices**
   - Pros: lowers risk by proving discovery of Codex state model early; can deliver incremental, reviewable progress.
   - Cons: leaves `--target codex` mixed-target flows partially complete for longer and may keep users with mixed advisory/functional behavior.
   - Effort: Medium

### Recommendation
Approach 1 (native Codex implementation) was selected and implemented with a conservative root and manifest model; activation/reload proof is intentionally surfaced as an explicit uncertainty in status reasons until runtime integration proves it.

### Risks
- The primary risk is incorrect assumption of Codex configuration schema or startup/activation semantics, which can produce non-honest `status` results or brittle uninstall behavior.
- Divergence risk with existing three-runtime patterns: Codex may not map cleanly to OpenCode-like plugin/model artifacts, so implementation can fail if copied blindly.
- `--target all` behavior is now full support with explicit Codex status while preserving backward behavior for existing non-Codex targets.

### Ready for Proposal
Yes — implementation and status semantics are in place with advisory behavior removed; this exploration now accurately reflects the current scope.
