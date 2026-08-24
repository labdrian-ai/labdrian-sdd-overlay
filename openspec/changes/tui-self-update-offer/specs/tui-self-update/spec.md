# tui-self-update Specification

## Purpose

Launch-time offer plus a safe action that converges a stale clone's `main`
with `origin/main`. This is a **new capability**, not a `sync-check-verdicts`
delta — per the approved proposal ("Modified Capabilities: None"),
`REPO_BEHIND_ORIGIN` detection, VERDICT fields, and exit codes stay owned by
`sync-check-verdicts` (its R-001–R-006). This capability only reads that
published value as a launch-time consumer; it MUST NOT redefine or duplicate
`sync-check-verdicts`' detection logic.

## Requirements

### Requirement: Launch-time cached-only origin probe

ID: R-001

WHEN the TUI starts, `Init()` SHALL return a `tea.Cmd` that reads
`REPO_BEHIND_ORIGIN` from the cached `refs/remotes/origin/main` ref only,
MUST NOT invoke `git fetch`, and SHALL deliver the result asynchronously
without blocking the first render.

#### Scenario: Probe is cached-only and async

- GIVEN the TUI starts
- WHEN `Init()`'s `tea.Cmd` runs and completes
- THEN no `git fetch` is invoked, and `Update()` receives the count/`NA` via a message after the first render

### Requirement: Dismissible behind-origin banner, no new screen

ID: R-002

WHEN the probe reports `REPO_BEHIND_ORIGIN` greater than zero, the system
SHALL render a dismissible banner via the existing `repoLine()`/`View()`
slot. The system SHALL NOT introduce a new screen enum value.

#### Scenario: Banner shown for nonzero, hidden for zero/NA

- GIVEN the probe reports `REPO_BEHIND_ORIGIN=2`
- WHEN `View()` renders
- THEN a behind-origin banner line is present; it is absent when the reported value is `0` or `NA`

#### Scenario: Dismissal suppresses the banner for the session

- GIVEN the banner is visible
- WHEN the user dismisses it
- THEN subsequent `View()` renders in the session omit the banner, and no value was added to the screen enum

### Requirement: Update-repository action entry

ID: R-003

The system SHALL add an "Actualizar repositorio" entry to `Actions()` with
`TargetAgnostic: true` and `Mutating: true`, routed through the existing
confirm→run→result screens; its `ConfirmMessage` SHALL state that only
`main` is updated.

#### Scenario: Target-agnostic, mutating, and scoped confirm copy

- GIVEN `Actions()` is enumerated and the entry is selected
- WHEN `screenConfirm` renders
- THEN `TargetAgnostic`/`Mutating` are both true, `screenTargets` is skipped, and the confirm text names `main` as the only branch updated

### Requirement: self-update subcommand fast-forwards main only

ID: R-004

The system SHALL provide a `self-update` subcommand that checks out `main`,
fast-forwards it to `origin/main` via an explicit fast-forward-only merge,
and restores the original branch via the existing trap-based EXIT pattern.

#### Scenario: Clean tree converges main, original branch restored

- GIVEN a clean tracked tree on branch "feature-x" with `main` behind `origin/main`
- WHEN `self-update` runs
- THEN `main` is fast-forwarded to `origin/main`, "feature-x" is checked out again, and exit code is 0

#### Scenario: Only main moves

- GIVEN the same preconditions
- WHEN `self-update` completes
- THEN "feature-x"'s HEAD is unchanged and `main`'s HEAD equals `origin/main`'s HEAD

### Requirement: Hard refusal on a dirty tracked tree

ID: R-005

IF `git status --porcelain --untracked-files=no` reports tracked changes,
THEN `self-update` SHALL exit 1 with an `ERROR: `-prefixed stderr message
before checking out `main`, and SHALL NOT modify any branch.

#### Scenario: Dirty tracked tree blocks the update

- GIVEN a tracked file has uncommitted changes
- WHEN `self-update` runs
- THEN it exits 1, stderr is prefixed `ERROR: `, and the checked-out branch is unchanged

#### Scenario: Untracked-only changes do not block

- GIVEN only untracked files are present, no tracked changes
- WHEN `self-update` runs
- THEN the dirty-tree refusal does not trigger

### Requirement: Hard refusal on local-ahead divergence

ID: R-006

IF `main` contains commits not present in `origin/main`, THEN `self-update`
SHALL refuse the fast-forward, exit 1 with an `ERROR: `-prefixed stderr
message, leave `main` unchanged, and restore the original branch.

#### Scenario: Local-ahead main blocks the update

- GIVEN `main` has one commit not in `origin/main`
- WHEN `self-update` runs
- THEN it exits 1, stderr is prefixed `ERROR: `, `main`'s HEAD is unchanged, and the original branch is restored

### Requirement: Successful update is observable via REPO_BEHIND_ORIGIN

ID: R-007

WHEN `self-update` completes successfully, a subsequent sync-check run SHALL
report `REPO_BEHIND_ORIGIN=0`; this capability MUST NOT alter
`sync-check-verdicts`' detection logic, VERDICT fields, or exit codes.

#### Scenario: Post-update convergence

- GIVEN `self-update` completed successfully
- WHEN sync-check runs afterward
- THEN the VERDICT line for the repo includes `REPO_BEHIND_ORIGIN=0`
