# Tasks: Codex Runtime Lifecycle

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~620 |
| Review budget (lines) | 800 |
| 800-line budget risk | Low |
| 400-line heuristic risk | High |
| Chained PRs recommended | No |
| Suggested split | Single PR with work-unit commits |
| Delivery strategy | auto-forecast |
| Chain strategy | none |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: none
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | Likely PR | Notes |
|------|------|-----------|-------|
| 1 | Codex lifecycle foundation, wiring, and tests | PR 1 | `engine/runtime/codex.go`, `engine/runtime/runtime.go`, `engine/runtime/runtime_test.go`, `engine/runtime/codex_test.go`, `engine/cmd/main.go`, `engine/cmd/runtime_test.go` |


## Phase 1: Foundation

- [x] 1.1 Add Codex-owned manifest model in `engine/runtime/codex.go`: `codexManifestFile`, `type codexManifest`, `managed_by`/`installed_version`/`config_root` JSON fields, plus helper constructors and root validators.
- [x] 1.2 Add `DefaultCodexConfigRoot()` in `engine/runtime/codex.go` with precedence `$CODEX_HOME` then `~/.codex` fallback using `os.UserHomeDir()`, and explicit root sanitization for absolute paths only.
- [x] 1.3 Write **failing** unit tests in `engine/runtime/codex_test.go` for root precedence and root-resolution error cases (CODEX_HOME set/unset, invalid empty root, non-absolute root).

## Phase 2: Core Implementation

- [x] 2.1 Implement a filesystem-backed `CodexAdapter` in `engine/runtime/codex.go` (`target`, `root`, `NewCodexAdapter(root string)`) and remove placeholder unsupported-only result methods.
- [x] 2.2 Implement `Install`/`Update` to write only Labdrian-owned manifest file (`labdrian-runtime-lifecycle.json`) with deterministic contents; keep user-owned files untouched.
- [x] 2.3 Implement `Uninstall` to delete only the owned manifest and return `restart_required` on successful removal.
- [x] 2.4 Implement honest `Status` in `engine/runtime/codex.go` with required semantics (`partial` for missing/incomplete managed state and for current managed state without activation/reload proof, `unsupported` only when root is unresolvable or safe operation is impossible), and always include activation/reload uncertainty in reasons when not provable.

## Phase 3: Integration / Wiring

- [x] 3.1 Update `engine/runtime/runtime.go` so `NewFoundationAdapter(TargetCodex)` uses `NewCodexAdapter("")` and preserves default-root behavior through `DefaultCodexConfigRoot()`.
- [x] 3.2 Update `engine/runtime/runtime.go` constructor wiring so Codex follows the same injectable-root pattern used by Claude/OpenCode without breaking test stubs.
- [x] 3.3 Update `engine/cmd/main.go` runtime adapter routing to instantiate real Codex adapter for `--target codex` and remove advisory-only Codex bypass logic.
- [x] 3.4 Update `engine/cmd/main.go` target aggregation so `--target all` applies normal failure semantics to Codex, including treating Codex `partial` as warn-only only when Claude/OpenCode targets are healthy.

## Phase 4: Testing / Verification

- [x] 4.1 Add/replace `engine/runtime/runtime_test.go` tests to validate foundation adapter behavior and transition from Codex placeholder unsupported to concrete status classification.
- [x] 4.2 Extend `engine/runtime/codex_test.go` with behavior-first tests for install/update/uninstall happy paths, mutation-boundary preservation, failed mutation safety, and honest status (`partial`/`unsupported`, with uncertainty reasons).
- [x] 4.3 Update `engine/cmd/runtime_test.go` with `--target codex` root precedence/`--config-root` override, mutation and status outcomes, and `--target all` behavior that includes codex with standard failure semantics.
- [x] 4.4 Add explicit non-regression checks for legacy Claude/OpenCode behavior in runtime tests and keep existing `status/install/update/uninstall` expectations stable where unaffected.
- [x] 4.5 Re-run focused runtime validation commands from `engine` (`go test ./runtime` and `go test ./cmd`) after the final alignment changes.

## Phase 5: Closure Checklist

- [x] 5.1 Re-run relevant docs/contracts for alignment (`spec`, `design`, `exploration`) and remove any stale references to advisory Codex behavior.
- [x] 5.2 Confirm no partial regression for existing Codex-advisory tests or messaging beyond this scope.
