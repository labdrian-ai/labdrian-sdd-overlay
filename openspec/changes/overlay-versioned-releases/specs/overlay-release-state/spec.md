# Overlay Release State Specification

## Purpose

Persists, per deployment target (claude/opencode/codex), the deployed release
version and an aggregate content digest — replacing per-file diff reasoning
with one deterministic pass/fail comparison while preserving today's
per-target granularity. Traces to overlay-versioned-releases R-002, R-003.

## Requirements

### Requirement: Persisted per-target installed-version and digest state

ID: R-001
Traces to: overlay-versioned-releases R-002

WHEN a deploy (`apply`) to a target completes successfully, the system SHALL
persist that target's deployed release version and content digest to a local
state file. WHEN the state file does not yet exist, the system SHALL create
it with the correct schema on the first successful apply rather than failing;
a target with no recorded entry SHALL be reported as "never deployed."

#### Scenario: Successful apply records version and digest

- GIVEN a successful `apply --target claude` at release `v1.4.0`
- WHEN the command finishes
- THEN the persisted state file records the `claude` target's version as `v1.4.0` and its digest as the value computed by this capability's digest requirement (R-002 below)

#### Scenario: First-ever apply creates the state file

- GIVEN the state file does not yet exist (a pre-existing install with no prior versioned-release apply)
- WHEN the first successful `apply` runs
- THEN the state file is created with the correct schema instead of the command failing

#### Scenario: Never-deployed target reported honestly

- GIVEN a target has no recorded state entry
- WHEN that target's state is read
- THEN it is reported as "never deployed," never a fabricated or stale version

### Requirement: Aggregate per-target content digest, sorted for determinism

ID: R-002
Traces to: overlay-versioned-releases R-003

The system SHALL compute one aggregate content digest per deployment target,
derived from the sha256 hashes of every currently-deployed managed file
listed for that target in `overlay.manifest`, computed over sorted
`<path>:<filehash>` lines. Sorting by path is mandatory before concatenation:
`overlay.manifest`'s row order is not itself sorted, so an unsorted
concatenation would make the digest depend on manifest row order rather than
file content.

#### Scenario: Matching digest for a fully in-sync target

- GIVEN target `claude` has every managed file matching the `main` tree at `v1.4.0`
- WHEN the aggregate digest is computed for `claude`
- THEN it equals the digest recorded for `v1.4.0`'s `claude` target

#### Scenario: Digest changes on a single-file mutation

- GIVEN one managed file deployed to `claude` is modified out-of-band
- WHEN the aggregate digest is recomputed
- THEN it differs from the recorded `v1.4.0` digest for `claude`

#### Scenario: Digest is order-independent of manifest row order

- GIVEN `overlay.manifest`'s row order for a target's files changes (rows reordered) with no content change
- WHEN the aggregate digest is recomputed
- THEN it is identical to the digest computed before the reorder (sorting neutralizes row-order as a digest input)

## Open Questions for Design

- **State-file location:** repo-scoped (e.g. `$OVERLAY_DIR/.overlay-state.json`,
  needs a `.gitignore` addition) vs. home-scoped (e.g.
  `$HOME/.labdrian-overlay/state.json`, mirroring gentle-ai's own convention)
  is explicitly left open in both the requirements brief and proposal.md; not
  settled here.
