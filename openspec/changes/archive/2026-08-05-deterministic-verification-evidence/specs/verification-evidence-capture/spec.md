# Verification Evidence Capture Specification

## Purpose

`sdd-verify` reaches the deterministic check runner through a stable CLI seam on `bin/labdrian-overlay` and pipes its stdout into the existing `gentle-ai review capture-evidence --input -` rail. Outcome selection follows a strict precedence: `procedural_tooling_failed` outranks `verification_failed`, so a missing tool never burns the single correction attempt or strands a run in the dead-end terminal state `review/escalate-correction-verification` (obs #2668).

## Requirements

### Requirement: sdd-verify Declares the normalize-before-START Ordering

The `sdd-verify` skill MUST declare `normalize` as a pre-`review start` step and `check` as the only permitted post-freeze step.

#### Scenario: Ordering is documented

- GIVEN `skills/sdd-verify/`
- WHEN its text is read
- THEN it names `normalize` as pre-START and `check` as post-freeze

#### Scenario: No mutating step scheduled after freeze

- GIVEN the declaration
- WHEN an agent follows it
- THEN no mutating mode runs after candidate freeze

### Requirement: Runner Reachable Through the Overlay CLI

The `bin/labdrian-overlay` dispatch table MUST route a deterministic-checks subcommand to the runner, following the `validate-entry-contract` pattern, and MUST fail loud (exit 3, explicit stderr message) when the runner source or the `go` toolchain is missing.

#### Scenario: Dispatch propagates exit code

- GIVEN `bin/labdrian-overlay`
- WHEN the subcommand is invoked
- THEN the runner executes and its exit code propagates

#### Scenario: Missing go toolchain fails loud

- GIVEN a missing `go` toolchain
- WHEN the subcommand is invoked
- THEN it exits 3 with an explicit error, matching `cmd_validate_entry_contract`

#### Scenario: Subcommand is discoverable

- GIVEN the CLI help output
- WHEN it is displayed
- THEN the new subcommand is listed

### Requirement: sdd-verify Pipes Runner Output into capture-evidence

WHEN the deterministic check run completes, `sdd-verify` MUST pass the runner's stdout to `gentle-ai review capture-evidence --input -`.

#### Scenario: Captured bytes match runner output

- GIVEN a completed run
- WHEN evidence is captured
- THEN the captured bytes are exactly the runner's emitted rows

#### Scenario: Empty run still satisfies minLength

- GIVEN an empty run with zero configured checks
- WHEN evidence is captured
- THEN a non-empty payload satisfying minLength 1 is still sent

#### Scenario: Oversized payload truncates, never rejects

- GIVEN a run exceeding 4194304 bytes
- WHEN evidence is captured
- THEN the payload is truncated to paths and counts rather than rejected

### Requirement: Clean Run Maps to Outcome passed

IF every executed deterministic check exited zero, THEN `sdd-verify` MUST set `--outcome passed` on the capture-evidence call.

#### Scenario: All-green run

- GIVEN all checks exit zero
- WHEN evidence is captured
- THEN the outcome flag is `passed`

### Requirement: Failing Deterministic Check Maps to Outcome verification_failed

IF a blocking deterministic check exited non-zero AND no tool was unavailable AND the runner itself did not error, THEN `sdd-verify` MUST set `--outcome verification_failed` on the capture-evidence call. This outcome MUST NOT be set when Requirement "Unavailable Tooling Maps to Outcome procedural_tooling_failed" applies to the same run (see precedence there).

#### Scenario: One blocking check fails, tooling was available

- GIVEN one blocking check exits non-zero and every configured tool was available
- WHEN evidence is captured
- THEN the outcome flag is `verification_failed`

#### Scenario: Non-blocking check exiting non-zero does not trigger this outcome

- GIVEN a check excluded from the blocking set by the determinism policy exits non-zero
- WHEN evidence is captured
- THEN the outcome flag is NOT `verification_failed`

### Requirement: Unavailable Tooling Outcome Is Severity-Proportional, and BLOCKING-Set Unavailability Outranks verification_failed

(Previously: any unavailable tool, regardless of severity class, forced `procedural_tooling_failed` for the whole run. Amended because a machine missing only the WARNING-only tool `deadcode` could never reach `passed`.)

IF a BLOCKING-set tool (`gofmt`, `go vet`, `staticcheck`) is unavailable, OR the runner itself errored, OR `bin/labdrian-overlay` is absent from the runtime, THEN `sdd-verify` MUST set `--outcome procedural_tooling_failed` on the capture-evidence call, regardless of any other check's result. IF a WARNING-only tool (`deadcode`) is unavailable, THEN the runner MUST render a WARNING row for that tool, MUST continue evaluating the remaining checks normally, and that tool's absence alone MUST NOT force `procedural_tooling_failed` and MUST NOT prevent an otherwise-clean run from reporting `passed`.

Outcome precedence (exact order):
1. A BLOCKING-set tool is unavailable, or the runner itself errored → `procedural_tooling_failed`
2. Otherwise, a deterministic blocking check failed → `verification_failed`
3. Otherwise → `passed`

`procedural_tooling_failed` still outranks `verification_failed` whenever both a BLOCKING-set tooling-unavailability condition and a failing-check condition are true for the same run — this precedence is load-bearing: miscounting a missing tool as a verification failure burns the single correction attempt and can strand the run in `review/escalate-correction-verification`, which `retry-final-verification` rejects (obs #2668). A missing WARNING-only tool never triggers this precedence rule, because it can never produce a blocking outcome in the first place.

#### Scenario: Missing BLOCKING-set tool alone

- GIVEN `staticcheck` (a BLOCKING-set tool) is not installed
- WHEN the run completes
- THEN the outcome flag is `procedural_tooling_failed`, not `verification_failed`

#### Scenario: Precedence — missing tool wins over a failing check

- GIVEN a BLOCKING-set tool is unavailable AND another check exits non-zero in the same run
- WHEN the outcome is selected
- THEN `procedural_tooling_failed` takes precedence over `verification_failed`

#### Scenario: Overlay CLI absence routes here, never to a silent pass

- GIVEN `bin/labdrian-overlay` is absent from the runtime that received only the deployed skill files
- WHEN `sdd-verify` attempts to invoke it
- THEN the outcome is `procedural_tooling_failed` — never a silent pass and never `verification_failed`

#### Scenario: Missing WARNING-only tool does not block a clean run

- GIVEN `deadcode` is not installed and every other configured check exits zero
- WHEN the run completes
- THEN the runner emits a WARNING row for `deadcode` and the outcome flag is `passed`

#### Scenario: Missing WARNING-only tool does not mask or escalate a genuine failure

- GIVEN `deadcode` is not installed AND a BLOCKING-set check (for example `go vet`) exits non-zero in the same run
- WHEN the outcome is selected
- THEN the outcome flag is `verification_failed`, not `procedural_tooling_failed`

### Requirement: New skills/ Files Are Registered in overlay.manifest

WHERE the change adds a new file under `skills/`, the change MUST add a matching row to `overlay.manifest`.

#### Scenario: New skill file gets a manifest row

- GIVEN a new file under `skills/`, registered with a matching `overlay.manifest` row
- WHEN `skills validate --registry ../skills.registry.yaml --manifest ../overlay.manifest --source-root ../skills` runs
- THEN it exits zero

#### Scenario: No new skill file, no regression

- GIVEN no new file under `skills/`
- WHEN validation runs
- THEN it exits zero unchanged
