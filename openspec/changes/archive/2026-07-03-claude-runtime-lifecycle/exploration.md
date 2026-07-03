## Exploration: claude-runtime-lifecycle

### Current State
`engine/runtime` already has a three-target lifecycle interface (`claude`, `opencode`, `codex`) and a CLI subcommand `engine runtime <action> --target <target>`.

However, `claude.go` and `codex.go` are still placeholder adapters: all lifecycle actions currently return `CapabilityUnsupported` with generic “scheduled for later PR” messages. OpenCode is the only runtime currently implementing real lifecycle behavior in `opencode.go`, including install/update/status/uninstall of `labdrian-runtime-parity.js`, deterministic JSON config (`labdrian-runtime-parity.json`), and restart signaling via an active marker.

The command path already handles per-target dispatch (`runRuntime` + `runtimeAdapterForTarget`), `--target all` expansion, and error/exit semantics that already mark unsupported/partial/restart-required states as failure for install/status/update/uninstall flows.

### Affected Areas
- `engine/runtime/claude.go` — currently placeholder lifecycle implementation for Claude; this is the primary file to replace.
- `engine/runtime/runtime.go` — shared adapter factory and lifecycle interfaces; may need extension if Claude requires additional root/config handling.
- `engine/runtime/runtime_test.go` — adapter-contract tests currently assert Claude is unsupported; will need updates once implemented.
- `engine/runtime/opencode_test.go` and `engine/runtime/runtime_test.go` — existing lifecycle test patterns that can be mirrored for Claude parity.
- `engine/cmd/main.go` — runtime CLI dispatch already routes `status|install|update|uninstall` through adapter APIs; likely only expected output/expectation updates required in tests.
- `engine/cmd/runtime_test.go` — tests confirm non-OpenCode targets are unsupported; these will need targeted updates after Claude support lands.
- `engine/settings/settings.go` — existing safe merge/uninstall hook logic is reusable for Claude runtime settings mutation if/when a runtime hook strategy is chosen.

### Approaches
1. **Clone OpenCode parity logic into ClaudeAdapter with dedicated Claude config + marker files**
   - Pros: predictable implementation path, aligned with existing tested lifecycle semantics (install/write/install state/status checks/uninstall + restart requirement).
   - Cons: must define equivalent Claude on-disk state schema and active-status semantics; OpenCode tests cannot be copied 1:1.
   - Effort: Medium

2. **Introduce a shared runtime-lifecycle helper to unify OpenCode/Claude behavior and reduce duplication**
   - Pros: lowers repeated code and future-proofs Codex/other runtimes when they land; centralizes status/marker handling patterns.
   - Cons: higher immediate scope/risk, likely needs refactor plus test re-alignment before this slice is complete.
   - Effort: High

### Recommendation
Implement Claude lifecycle directly in `engine/runtime/claude.go` first, keeping changes in this slice narrowly scoped to runtime parity for install/status/update/uninstall and reusing existing `settings` merge/uninstall utilities where appropriate.

Suggested immediate design:
- Define a Claude config root (explicit, validated, absolute), install artifacts/marker under it (or under the same logical directory pattern used by current runtime state),
- Implement deterministic JSON config and restart-required status states analogous to OpenCode, and
- Make status semantics explicit (unsupported vs restart required vs supported) so `engine runtime status --target claude` and `--target all` return meaningful, testable behavior.

### Risks
- The exact Claude runtime bridge format and install path may need product alignment before final behavior is locked.
- This change could uncover divergence in restart semantics between CLI-managed config and Claude runtime hook/activation model, requiring one additional compatibility step.
- A broad `--target all` rollout can expose inconsistent behavior differences for mixed-supported/runtime targets during initial verification.

### Ready for Proposal
Yes — sufficient understanding exists to move to `sdd-propose` with a concrete requirement set and explicit rollback/compatibility boundary (Codex stays untouched/unsupported in this slice).
