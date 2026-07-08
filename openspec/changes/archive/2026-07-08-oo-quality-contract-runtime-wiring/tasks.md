# Tasks: OO Quality Contract Runtime Wiring

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 500-750 |
| 800-line budget risk | Medium |
| Chained PRs recommended | No |
| Suggested split | Single PR with work-unit commits |
| Delivery strategy | auto-forecast |

Estimated changed lines: 500-750
800-line budget risk: Medium
Chained PRs recommended: No
Decision needed before apply: No

### Suggested Work Units

| Unit | Goal | Likely PR | Notes |
|------|------|-----------|-------|
| 1 | Gate model and CLI context input | PR 1 | Keep Go tests with model/CLI behavior. |
| 2 | OpenCode config/plugin parity | PR 1 | Keep Node-backed runtime tests with adapter changes. |
| 3 | Lifecycle non-regression | PR 1 | Confirm settings and install/update/status behavior. |

## Phase 1: RED Tests

- [x] 1.1 Add table-driven cases in `engine/gate/gate_test.go` for independent contracts, phase-only stability, OO matched context, missing context, malformed metadata, unsupported context, and prompt-text non-proof.
- [x] 1.2 Add CLI tests in `engine/cmd/main_test.go` for optional `gate-task` work-context input and absent/invalid context pass-through.
- [x] 1.3 Add runtime tests in `engine/runtime/opencode_test.go` for `prompt_config.contracts[]`, phase-only compatibility, OO exact-once injection, non-domain pass-through, and no prompt heuristics.
- [x] 1.4 Add `engine/settings/settings_test.go` coverage for existing lifecycle compatibility and distinct OO hook ownership only when enabled.

## Phase 2: Gate Model Implementation

- [x] 2.1 Extend `engine/gate/gate.go` with `WorkContext`, multi-contract config, and a compatibility wrapper for current `ContractPath` behavior.
- [x] 2.2 Implement per-contract evaluation in `engine/gate/gate.go`: phase exclusion, phase inclusion, trusted context match, exact-once injection, and per-contract pass-through failures.
- [x] 2.3 Update `engine/cmd/main.go` to parse explicit work-context JSON/file flags for `gate-task`; treat absent, malformed, unsupported, or insufficient context as untrusted.

## Phase 3: Runtime Wiring

- [x] 3.1 Update `engine/runtime/opencode.go` to emit `prompt_config.contracts[]` with path, phase scopes, injection point, and context requirement fields from contract metadata.
- [x] 3.2 Update `engine/runtime/labdrian-runtime-parity-plugin.mjs` to evaluate each contract independently and require trusted context before OO injection.
- [x] 3.3 Update `engine/settings/settings.go` only as needed for distinct OO hook identity; do not merge it with minimalism or skill-discovery-safety ownership.

## Phase 4: Verification and Cleanup

- [x] 4.1 Run targeted tests: `cd engine && go test ./gate ./cmd ./runtime ./settings`.
- [x] 4.2 Run full project tests: `cd engine && go test ./... && cd ../tui && go test ./...`.
- [x] 4.3 Run formatting and static checks for changed Go/JS files; keep `skills/_shared/oo-quality-contract.md` unchanged unless tests prove metadata needs correction.

## Final Delivery Note

- Actual delivered review size exceeded the original 500-750 line forecast and crossed the 800-line review budget before counting all archive artifacts.
- Any PR or equivalent review submission for this archived slice requires explicit reviewer-burden handling or a documented size exception instead of treating the original forecast as accurate.
