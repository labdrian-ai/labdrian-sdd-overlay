# longterm-mem Sync Triggers Specification

## Purpose

Automatically promote newly-firm canonical knowledge to the long-term vault at
two natural boundaries — Claude Code session close and SDD change archive —
without ever failing, blocking, or delaying the triggering host command.

## Requirements

### Requirement: Non-Blocking Sync Runner Contract

The system MUST provide a shared `sync-trigger` runner invoked by both trigger
surfaces that locates the `longterm-mem` binary, runs `longterm-mem sync` with
its working directory set to the project directory — letting `longterm-mem`
resolve the project from that working directory itself, since no `--project`
flag is passed — detached from the caller under a bounded timeout (~60s),
classifies the outcome, appends one log line per firing to an
operator-discoverable location, and always exits 0 to its caller regardless
of the sync outcome.

#### Scenario: Missing binary is a silent skip

- GIVEN `longterm-mem` is not installed at the expected state-directory path
- WHEN the runner fires for either event
- THEN it logs `skip:no-binary`
- AND it exits 0 without surfacing an error to the caller

#### Scenario: Missing vault is a silent skip

- GIVEN the resolved project has no provisioned vault
- WHEN the runner fires
- THEN it logs `skip:no-vault`
- AND it exits 0

#### Scenario: Sync failure is logged, caller unaffected

- GIVEN `longterm-mem sync` exits non-zero
- WHEN the runner fires
- THEN it logs `failure` with the outcome
- AND the triggering host command's own result is unaffected

#### Scenario: Sync timeout is bounded and logged

- GIVEN `longterm-mem sync` exceeds the bounded timeout
- WHEN the runner fires
- THEN it terminates the child, logs `timeout`
- AND does not block the caller past the timeout budget

#### Scenario: Successful run is logged

- GIVEN `longterm-mem sync` completes successfully
- WHEN the runner fires
- THEN it logs a success summary at the documented log location

#### Scenario: Unwritable log location does not block the host

- GIVEN the log directory is unwritable or the log file cannot be opened
- WHEN the runner fires
- THEN it still exits 0 to the caller
- AND it reports the logging failure to stderr on a best-effort basis only

#### Scenario: Runner cannot re-exec itself

- GIVEN the runner's own executable is missing or is being rebuilt when it
  attempts to detach and re-exec itself
- WHEN the runner fires
- THEN it exits 0 without blocking the caller

#### Scenario: A longterm-mem usage error is logged as an error, not a skip

- GIVEN `longterm-mem sync` exits with a usage error (exit code 2) for a
  reason other than the working directory not being a recognized project
- WHEN the runner fires
- THEN it logs the outcome as an error, not as a silent skip

### Requirement: SessionEnd Sync Trigger

The system MUST install a `SessionEnd`-only hook entry, owned and identified
independently by `engine/settings` using a `sync-trigger` identity token, that
invokes the shared `sync-trigger` runner with `--event session-end` and its
working directory set to the session's resolved project directory, coexisting
idempotently with pre-existing `SessionEnd`/`Stop` entries owned by other
tools (`moshi-hook`, `gentle-ai`), and MUST NOT install any entry on `Stop`.

#### Scenario: SessionEnd fires the runner

- GIVEN a session with a resolvable project directory
- WHEN the `SessionEnd` hook fires
- THEN it invokes the `sync-trigger` runner with `--event session-end` and
  that project directory as the working directory

#### Scenario: Stop never fires the sync trigger

- GIVEN a `Stop` event fires mid-session
- WHEN that event is processed
- THEN no sync invocation occurs — one run per session, not per turn

#### Scenario: Install coexists with foreign entries

- GIVEN `~/.claude/settings.json` already contains `SessionEnd`/`Stop` entries
  owned by other tools
- WHEN `engine/settings` installs the new `SessionEnd` entry
- THEN pre-existing foreign entries remain intact
- AND the new entry is independently identifiable

#### Scenario: Install is idempotent

- GIVEN the `SessionEnd` entry is already installed
- WHEN install runs again
- THEN no duplicate entry is created

#### Scenario: Uninstall removes only the owned entry

- GIVEN the `SessionEnd` entry is installed alongside foreign entries
- WHEN `uninstall-hooks` runs
- THEN only the owned entry is removed
- AND foreign entries remain untouched

#### Scenario: status-hooks reports the SessionEnd family

- GIVEN the `SessionEnd` entry may or may not be installed
- WHEN `status-hooks` runs
- THEN it reports the `SessionEnd` family's installed/missing state honestly

### Requirement: Archive-Time Sync Trigger

The system MUST invoke the shared `sync-trigger` runner with `--event archive`
and its working directory set to the project root — so `longterm-mem`
resolves and syncs the whole project, not only the archived change's topics —
from an overlay-owned surface (inception-pipeline closure-feedback) after an
SDD change completes archival, without editing the managed
`sdd-archive/SKILL.md`.

#### Scenario: Archive completion fires a whole-project sync

- GIVEN an SDD change reaches archive completion
- WHEN closure-feedback runs
- THEN it invokes the `sync-trigger` runner with `--event archive` and the
  project root as the working directory
- AND the resulting sync covers the whole project, not only the archived
  change's topics

#### Scenario: Trigger lives outside the managed skill

- GIVEN the archive-time trigger is installed
- WHEN its call site is inspected
- THEN it is defined in `skills/inception-pipeline/SKILL.md`, not in
  `sdd-archive/SKILL.md`
