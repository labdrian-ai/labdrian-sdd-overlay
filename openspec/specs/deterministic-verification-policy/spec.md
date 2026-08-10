# Deterministic Verification Policy Specification

## Purpose

Determinism decides which check findings block strict-TDD verification and CI. Red deterministic binary checks (`gofmt`, `go vet`, `staticcheck`) become CRITICAL; non-deterministic or threshold-relative checks (coverage, `deadcode`) stay WARNING. This resolves the existing contradiction between `skills/sdd-verify/SKILL.md:79` (CRITICAL on non-zero test exit) and `strict-tdd-verify.md:121/:127/:266` (WARNING on lint/typecheck).

## Requirements

### Requirement: No Upstream Modification

The deterministic-verification-evidence change MUST leave `bin/overlay` and every `gentle-ai` schema byte-identical. Nothing in this change SHALL touch a vendored or upstream baseline path.

#### Scenario: Upstream binary untouched

- GIVEN the change branch
- WHEN `git diff main -- bin/overlay` runs
- THEN it returns empty output

#### Scenario: No vendored path touched

- GIVEN the change branch
- WHEN the full diff is listed
- THEN no path under a vendored/upstream baseline appears

### Requirement: Determinism Is the Membership Criterion for Blocking Checks

IF a check does not produce an identical exit code across repeated runs on identical inputs, THEN the strict-TDD verify module MUST exclude that check from the blocking (CRITICAL) severity set. A check MUST declare a determinism attribute before it is eligible for CRITICAL.

#### Scenario: Unproven determinism stays non-blocking

- GIVEN a check whose determinism is unproven
- WHEN sdd-verify classifies it
- THEN it is assigned WARNING or SUGGESTION, never CRITICAL

#### Scenario: New check requires a determinism attribute

- GIVEN the runner's registry of checks
- WHEN a check is added
- THEN it must declare a determinism attribute before it is eligible for CRITICAL

### Requirement: Red Deterministic Binary Checks Are CRITICAL

WHEN a deterministic binary check (`go vet`, `gofmt`, `staticcheck`) exits non-zero on changed files, the strict-TDD verify module MUST classify the finding as CRITICAL and the verdict MUST be FAIL.

#### Scenario: go vet failure blocks

- GIVEN `go vet` exits non-zero
- WHEN sdd-verify reports
- THEN the finding is CRITICAL and the verdict is FAIL

#### Scenario: staticcheck failure blocks

- GIVEN `staticcheck` exits non-zero
- WHEN sdd-verify reports
- THEN the finding is CRITICAL

#### Scenario: Clean deterministic run raises nothing

- GIVEN all deterministic checks exit zero
- WHEN sdd-verify reports
- THEN no CRITICAL finding originates from this source

### Requirement: Coverage and Dead-Code Remain WARNING

The strict-TDD verify module MUST classify coverage metrics and `deadcode` findings as WARNING, never CRITICAL, because coverage is threshold-relative and `deadcode` has structural false positives (reflection, generated code).

#### Scenario: Low coverage does not fail

- GIVEN coverage below any local expectation
- WHEN sdd-verify reports
- THEN the finding is WARNING and does not cause a FAIL verdict

#### Scenario: deadcode finding does not fail

- GIVEN deadcode reports unreachable symbols
- WHEN sdd-verify reports
- THEN the finding is WARNING

### Requirement: CI Runs staticcheck as a Blocking Step

WHEN CI runs the `engine` and `tui` jobs, the workflow MUST execute `staticcheck ./...` in each module and fail the job on non-zero exit.

#### Scenario: Violation fails the job

- GIVEN a staticcheck violation in `engine`
- WHEN CI runs
- THEN the `test-engine` job fails

#### Scenario: Clean tree passes

- GIVEN a clean tree
- WHEN CI runs
- THEN both `test-engine` and `test-tui` jobs pass

### Requirement: CI Reports deadcode Without Blocking

WHEN CI runs the `engine` and `tui` jobs, the workflow MUST report `go run golang.org/x/tools/cmd/deadcode ./...` output as a non-blocking step, consistent with the WARNING classification above.

#### Scenario: deadcode findings do not fail CI

- GIVEN deadcode reports unreachable symbols
- WHEN CI runs
- THEN the job still succeeds and the output is visible in the log

#### Scenario: deadcode failing to run does not fail CI

- GIVEN deadcode fails to run
- WHEN CI runs
- THEN the job still succeeds
