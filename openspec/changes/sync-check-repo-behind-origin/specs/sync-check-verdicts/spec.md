# sync-check-verdicts Specification

## Purpose

The sync-check verdict pipeline (`bin/labdrian-overlay cmd_sync_check`, consumed by `tui/run.go`'s `ParseSyncCheck`) detects drift between deployed overlay files, the local `main` branch, and the vendored `upstream` branch, reporting it via a VERDICT/ACTION line contract. This capability extends that pipeline with a fourth, additive signal — `REPO_BEHIND_ORIGIN` — so a local clone that has fallen behind the repository's own `origin` remote is never reported as fully healthy.

## Requirements

### Requirement: Default cached-ref comparison against origin/main

ID: R-001

WHEN sync-check runs without `--check-origin` or `--fetch`, the system SHALL compute `git rev-list HEAD..origin/main --count` against the locally cached `refs/remotes/origin/main` ref without invoking `git fetch`.

#### Scenario: Cached-ref count computed with no network call

- GIVEN a repo with a cached `origin/main` ref and no override flag
- WHEN sync-check runs
- THEN it reports the rev-list count from the cached ref only
- AND no `git fetch` is invoked

#### Scenario: Offline default path completes without error

- GIVEN no network connectivity
- WHEN sync-check runs in default mode
- THEN it completes without a network-related error

### Requirement: Emit REPO_BEHIND_ORIGIN verdict field

ID: R-002

The system SHALL always append a `REPO_BEHIND_ORIGIN=` field to the target's VERDICT line alongside the existing `UPSTREAM_CHANGED`/`OVERLAY_NOT_DEPLOYED` fields — the field MUST NEVER be omitted. WHEN the origin comparison count (cached or fetched) is available, the value SHALL be `REPO_BEHIND_ORIGIN=<count>`. WHEN the origin comparison is unavailable (per R-004) or a fetch fails (per R-003), the value SHALL be the literal sentinel token `REPO_BEHIND_ORIGIN=NA`, and this exact token is the only accepted unavailable-value encoding.

#### Scenario: Non-zero count reported

- GIVEN HEAD is 3 commits behind origin/main
- WHEN sync-check completes for that target
- THEN the VERDICT line includes `REPO_BEHIND_ORIGIN=3`

#### Scenario: Zero count reported

- GIVEN HEAD is even with origin/main
- WHEN sync-check completes for that target
- THEN the VERDICT line includes `REPO_BEHIND_ORIGIN=0`

#### Scenario: Unavailable count reported as literal NA token, never omitted

- GIVEN the origin comparison is unavailable for a target
- WHEN sync-check completes for that target
- THEN the VERDICT line includes the literal `REPO_BEHIND_ORIGIN=NA`
- AND the `REPO_BEHIND_ORIGIN` field is present on the VERDICT line (not omitted)

### Requirement: Opt-in live fetch before comparison

ID: R-003

WHERE the user supplies `--check-origin` or `--fetch`, the system SHALL run `git fetch origin` before computing the origin comparison used for `REPO_BEHIND_ORIGIN`.

#### Scenario: Explicit flag triggers fetch

- GIVEN the user passes `--check-origin`
- WHEN sync-check runs
- THEN `git fetch origin` is invoked before the rev-list comparison
- AND the reported count reflects the freshly fetched state

#### Scenario: Fetch failure degrades to a scoped warning

- GIVEN the user passes `--check-origin` and the fetch fails
- WHEN sync-check runs
- THEN the origin check surfaces a scoped warning for that check
- AND the existing offline checks (`UPSTREAM_CHANGED`, `OVERLAY_NOT_DEPLOYED`) still complete

### Requirement: Graceful degrade with no remote or no cached ref

ID: R-004

IF no `origin` remote is configured or no cached `refs/remotes/origin/main` ref exists, THEN the system SHALL report the origin comparison as unavailable by emitting the literal `REPO_BEHIND_ORIGIN=NA` (per R-002 — never omitted) and SHALL still complete the existing `OVERLAY_NOT_DEPLOYED`/`UPSTREAM_CHANGED` checks.

#### Scenario: No origin remote configured

- GIVEN no origin remote is configured
- WHEN sync-check runs, with or without `--check-origin`
- THEN the VERDICT line includes the literal `REPO_BEHIND_ORIGIN=NA`
- AND existing checks still complete successfully

#### Scenario: Remote configured but ref never fetched

- GIVEN an origin remote is configured but `refs/remotes/origin/main` was never fetched
- WHEN sync-check runs in default (cached-ref) mode
- THEN the VERDICT line includes the literal `REPO_BEHIND_ORIGIN=NA` rather than erroring

### Requirement: TUI dashboard surfaces REPO_BEHIND_ORIGIN

ID: R-005

WHEN sync-check stdout for a target includes `REPO_BEHIND_ORIGIN` greater than zero, the TUI dashboard SHALL render a distinct indicator for that target alongside its existing verdict indicators.

#### Scenario: Distinct indicator rendered

- GIVEN sync-check stdout contains `REPO_BEHIND_ORIGIN=2` for a target
- WHEN the TUI parses and renders that target
- THEN a distinct origin-behind indicator is shown alongside existing verdict indicators

#### Scenario: Zero count renders no origin indicator

- GIVEN sync-check stdout contains `REPO_BEHIND_ORIGIN=0` for a target
- WHEN the TUI parses and renders that target
- THEN no origin-behind indicator is shown for that target

### Requirement: No silent healthy status while behind origin

ID: R-006

IF a target's `REPO_BEHIND_ORIGIN` count is greater than zero, THEN the system SHALL NOT present that target as fully healthy/IN_SYNC without also displaying the `REPO_BEHIND_ORIGIN` verdict, and this presentation change SHALL NOT alter the target's sync-check exit code.

#### Scenario: Non-healthy presentation despite other verdicts being zero

- GIVEN a target with `UPSTREAM_CHANGED=0`, `OVERLAY_NOT_DEPLOYED=0`, and `REPO_BEHIND_ORIGIN=2`
- WHEN overall status is reported (CLI or TUI)
- THEN the target is not presented as fully healthy/IN_SYNC
- AND `REPO_BEHIND_ORIGIN=2` is visible in that report

#### Scenario: Exit code unaffected by origin drift

- GIVEN a target with `REPO_BEHIND_ORIGIN=2` and all other verdicts at zero
- WHEN sync-check's CLI exit code is evaluated
- THEN the exit code is identical to the exit code produced when `REPO_BEHIND_ORIGIN=0`

#### Scenario: Additive precedence alongside existing verdicts

- GIVEN a target with `UPSTREAM_CHANGED=1` and `REPO_BEHIND_ORIGIN=2` simultaneously
- WHEN overall status is reported (CLI or TUI)
- THEN both verdicts are shown alongside each other
- AND neither verdict overrides or hides the other
