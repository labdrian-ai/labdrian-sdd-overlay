# Tasks: OO Quality Contract

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 180-260 |
| Pre-commit staged diff | 883 insertions before the corrective auditability/test adjustment, mostly archived SDD documentation |
| 800-line session budget risk | Exceeded after archive documentation was included |
| 400-line budget risk | High at pre-commit review |
| Chained PRs recommended | No |
| Suggested split | Single first-slice PR with maintainer-approved size exception |
| Delivery strategy | auto-forecast |
| Chain strategy | size-exception approved 2026-07-08 |

Decision needed before apply: No — original apply forecast did not require a decision, but pre-commit review found the actual staged diff exceeded the review budgets.
Chained PRs recommended: No — maintainer/user explicitly approved a single-PR size exception on 2026-07-08.
Chain strategy: size-exception approved 2026-07-08
400-line budget risk: High at pre-commit review

### Budget Audit Update

The original 180-260 line forecast was not accurate once the change was archived and staged for commit. The pre-commit staged diff reached 883 insertions before this corrective adjustment, mostly because archived SDD documentation was included in the review surface. This artifact preserves that drift for auditability instead of rewriting the forecast as if it had been safe.

### Suggested Work Units

| Unit | Goal | Likely PR | Notes |
|------|------|-----------|------|
| 1 | Add artifact tests, contract, manifest row, and verification for R-001..R-008 | PR 1 | Keep tests with artifact changes; no runtime wiring. |

## Phase 1: RED Artifact Tests

- [x] 1.1 Create `engine/skills/oo_quality_contract_artifact_test.go` with failing artifact tests for R-001 path/content and non-vendoring assertions.
- [x] 1.2 Add failing tests in `engine/skills/oo_quality_contract_artifact_test.go` for R-002 manifest row and exactly-one `_shared/oo-quality-contract.md custom` assertion.
- [x] 1.3 Add failing tests in `engine/skills/oo_quality_contract_artifact_test.go` for R-003 frontmatter include/exclude phase tokens.
- [x] 1.4 Add failing tests in `engine/skills/oo_quality_contract_artifact_test.go` for R-004 precedence, R-005 context gate, R-006 advisory OO guidance, R-007 TDD non-mandate, and R-008 no runtime wiring boundary.

## Phase 2: GREEN Contract Artifact

- [x] 2.1 Create `skills/_shared/oo-quality-contract.md` with English-only frontmatter covering R-003: `sdd-design`, `sdd-tasks`, `sdd-apply`; exclude `sdd-propose`, `sdd-spec`, `sdd-archive`, `sdd-verify`.
- [x] 2.2 Add concise contract body satisfying R-001, R-004, R-005, R-006, and R-007: local/non-vendored, higher-precedence artifacts win, pass-through contexts, diagnostic SOLID vocabulary, justified abstractions only, no global TDD mandate.
- [x] 2.3 Add `_shared/oo-quality-contract.md custom` to `overlay.manifest` near the existing `_shared/*` entries for R-002.

## Phase 3: Boundary Protection

- [x] 3.1 Confirm no changes are made to `.atl/skill-registry.md`, external `solid-skills` content, or vendored dependency paths for R-001 and R-008.
- [x] 3.2 Confirm no engine propagation, gate, marker-block, embedded registration, CLI, or runtime prompt injection wiring is added for R-008.

## Phase 4: Verification

- [x] 4.1 Run `cd /home/labdrian/labdrian-sdd-overlay/engine && go test ./...` to verify artifact tests and existing engine tests.
- [x] 4.2 Run `cd /home/labdrian/labdrian-sdd-overlay/tui && go test ./...` to verify the untouched TUI module.
- [x] 4.3 Run `git diff --check` and inspect `git diff --stat`; later pre-commit review found the staged diff reached 883 insertions, so the maintainer/user approved a single-PR size exception on 2026-07-08.
