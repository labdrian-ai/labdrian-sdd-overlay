# Deterministic Check Runner Specification

## Purpose

An overlay-owned Go command under `tools/` executes the hardcoded v1 check set (`gofmt`, `go vet`, `staticcheck`, `deadcode`) and emits bounded, machine-parseable evidence rows. A `normalize`/`check` mode split guarantees the runner never mutates tracked files after the candidate freeze. The check set is hardcoded, not configurable (product decision, obs #2694).

## Requirements

### Requirement: Overlay-Owned Deterministic Check Runner

The overlay MUST provide a Go command under `tools/` that executes the hardcoded v1 check set and returns each check's exit code. The module SHALL mirror the shape of `tools/entry-contract-validator`.

#### Scenario: Module shape matches precedent

- GIVEN the repository
- WHEN the tool directory is inspected
- THEN it contains `go.mod`, `main.go`, `main_test.go`, and `testdata/`

#### Scenario: CI runs the module's tests

- GIVEN CI
- WHEN the workflow runs
- THEN a dedicated job executes `go test ./... -cover` for the new tool module

### Requirement: Machine-Readable Row Output

WHEN the runner finishes a run, it MUST emit exactly one `tool | exit_code | summary` row per executed check to stdout, in stable declared order, for exactly the four configured checks.

#### Scenario: Four configured checks, four rows

- GIVEN the hardcoded v1 check set
- WHEN the runner completes
- THEN stdout contains exactly 4 rows in stable order

#### Scenario: Row fields are typed

- GIVEN a row
- WHEN it is parsed
- THEN `exit_code` is an integer and `tool` is the check's stable identifier

### Requirement: Token-Bounded Summaries

The runner MUST render each summary as a finding count, at most N top findings (default N = 5), and a filesystem path to the full captured output. The runner MUST NOT emit a raw tool dump.

#### Scenario: Many findings truncate

- GIVEN a check producing 200 findings
- WHEN the row is emitted
- THEN it contains the count `200`, at most 5 excerpts, and one path

#### Scenario: Few findings show in full

- GIVEN a check producing 2 findings
- WHEN the row is emitted
- THEN it contains both findings and the path

#### Scenario: top-N is configurable at runtime

- GIVEN `--top-n=1`
- WHEN the runner emits
- THEN at most one finding excerpt appears per row

### Requirement: Zero Findings Render as a Count, Never a Status Word

IF a check produces zero findings, the runner MUST emit the numeric count `0` in the summary field. The runner MUST NOT emit `PASS`, `PASSED`, `SUCCESS`, `N/A`, `NA`, `NONE`, `TODO`, `TBD`, or `PLACEHOLDER` as a standalone summary value, because upstream's evidence schema regex rejects those literals.

#### Scenario: Clean run emits 0

- GIVEN a clean `go vet` run
- WHEN the row is emitted
- THEN the summary contains `0` and none of the banned literals

#### Scenario: No banned literal anywhere

- GIVEN any runner output
- WHEN it is scanned
- THEN no banned literal appears as a standalone summary value

### Requirement: Separate normalize and check Modes

The runner MUST expose `normalize` and `check` as two distinct subcommands. Invoking the runner with no subcommand MUST be an error naming both subcommands.

#### Scenario: No subcommand errors

- GIVEN the runner
- WHEN invoked with no subcommand
- THEN it exits non-zero with usage naming both subcommands

#### Scenario: check never mutates

- GIVEN `check`
- WHEN invoked
- THEN no formatter or fixer executes

### Requirement: check Mode Is Byte-Neutral

WHILE running in `check` mode, the runner MUST leave every tracked file's bytes and mode unchanged, so a post-freeze invocation never invalidates a review receipt.

#### Scenario: Dirty tree stays dirty, identically

- GIVEN a dirty working tree with unformatted files
- WHEN `check` runs
- THEN `git status --porcelain` output is identical before and after

#### Scenario: File mode preserved

- GIVEN a file with mode 0755
- WHEN `check` runs
- THEN the mode is unchanged
