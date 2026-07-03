# Tasks: Claude Runtime Lifecycle

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~520–640 |
| Review budget (lines) | 800 |
| 400-line budget risk | Medium |
| Chained PRs recommended | No |
| Suggested split | Single PR |
| Delivery strategy | auto-forecast |
| Chain strategy | pending |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: pending
400-line budget risk: Medium

### Suggested Work Units

| Unit | Goal | Likely PR | Notes |
|------|------|-----------|-------|
| 1 | Implement Claude runtime lifecycle end-to-end (adapter, wiring, tests, and regression coverage) | PR 1 | One PR slice is acceptable under the 800-line budget |

## Phase 1: Foundation / Infrastructure

- [x] 1.1 In `engine/settings/settings.go`, add read-only helpers to detect Labdrian-owned Claude hook entries in `settings.json` and validate root/path preconditions.
- [x] 1.2 Add or confirm behavior coverage for these helper(s) against mixed third-party hooks and malformed/absent settings.
- [x] 1.3 In `engine/runtime/runtime.go`, add `DefaultClaudeConfigRoot()` and constructor support so Claude uses default root unless `--config-root` is explicit.
- [x] 1.4 In `engine/runtime/claude.go`, add adapter state for resolved root, `settings.json` path, and hook identity path.

## Phase 2: Core Implementation

- [x] 2.1 In `engine/runtime/claude_test.go`, write RED tests for `status`, `install`, `update`, `uninstall`, default-root behavior, and unproven-health failures.
- [x] 2.2 In `engine/runtime/claude.go`, implement `NewClaudeAdapter(root string)` and lifecycle methods via `settings.Merger` with explicit-root behavior.
- [x] 2.3 In `engine/runtime/claude.go`, implement honest `Status()` that returns `supported` only when required Labdrian-owned hooks are proven present.
- [x] 2.4 In `engine/runtime/claude.go`, implement backup-safe install/update/uninstall flows with no partial writes on malformed settings.

## Phase 3: Integration / Wiring

- [x] 3.1 In `engine/cmd/main.go`, resolve Claude root via `engine/runtime.DefaultClaudeConfigRoot()` and pass explicit `--config-root` when provided.
- [x] 3.2 In `engine/cmd/main.go`, adjust `--target all` aggregation so unsupported Codex is downgraded to advisory when Claude/OpenCode succeed.
- [x] 3.3 In `engine/runtime/runtime.go`, align adapter factory wiring for Claude while preserving existing OpenCode/Codex control flow.

## Phase 4: Testing / Verification

- [x] 4.1 In `engine/runtime/runtime_test.go`, replace Claude placeholder contract checks with real status/install/update/uninstall assertions, including default-vs-explicit root behavior.
- [x] 4.2 In `engine/cmd/runtime_test.go`, add/adjust tests for Claude root selection, lifecycle transitions, and `--target all` advisory success.
- [x] 4.3 Verify read-only status helper stability and non-regression of unrelated settings entries through runtime/settings behavior tests.
- [x] 4.4 Run strict module checks: `cd engine && go test ./...` and `cd tui && go test ./...`.
