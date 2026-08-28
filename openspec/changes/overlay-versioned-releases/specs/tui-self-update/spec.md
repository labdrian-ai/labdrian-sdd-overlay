# Delta for tui-self-update

## MODIFIED Requirements

### Requirement: self-update subcommand fast-forwards main to the latest release tag

ID: R-004
Traces to: overlay-versioned-releases R-005
(Previously: fast-forwarded `main` directly to raw `origin/main` HEAD, with no tag concept)

The system SHALL provide a `self-update` subcommand that explicitly fetches
tags from `origin` and resolves the highest semver-ordered annotated release
tag reachable from `origin/main` (via overlay-release-identity's
tag-resolution), checks out `main`, fast-forwards it to that resolved tag's
commit via an explicit fast-forward-only merge, and restores the original
branch via the existing trap-based EXIT pattern. WHEN local `main` already
points at the latest known release tag, `self-update` SHALL exit 0 without
moving `main` and SHALL report "already up to date with vX.Y.Z".

#### Scenario: Clean tree converges main to the latest release tag, original branch restored

- GIVEN a clean tracked tree on branch "feature-x", with origin tag `v1.5.0` newer than local main's `v1.4.0`
- WHEN `self-update` runs
- THEN `main` is fast-forwarded to `v1.5.0`'s commit, "feature-x" is checked out again, exit code is 0, and the command reports `v1.5.0` by name

#### Scenario: Only main moves, and it converges to the tag, not raw origin/main HEAD

- GIVEN the same preconditions, and `origin/main`'s raw HEAD carries commits beyond `v1.5.0` that are not yet tagged
- WHEN `self-update` completes
- THEN "feature-x"'s HEAD is unchanged, and `main`'s HEAD equals the resolved release tag's commit — which MAY differ from `origin/main`'s raw HEAD when untagged commits exist beyond the latest tag

#### Scenario: Already at latest release tag — no-op

- GIVEN local `main` already points at the latest known release tag
- WHEN `self-update` runs
- THEN it exits 0 without moving `main` and reports "already up to date with vX.Y.Z"

### Requirement: Successful update is observable via release convergence

ID: R-007
Traces to: overlay-versioned-releases R-005 (resolved by design decision D2)
(Previously: success implied a subsequent sync-check reports
`REPO_BEHIND_ORIGIN=0` — that held only while `self-update` converged `main`
to `origin/main`'s raw HEAD)

WHEN `self-update` completes successfully in tag mode (at least one release
tag exists), a subsequent sync-check run SHALL report
`REPO_BEHIND_RELEASE=0` — the count of commits local `main` is behind the
newest locally-known release tag, published by `sync-check-verdicts` — as the
primary up-to-date signal. `REPO_BEHIND_ORIGIN` keeps its existing raw
origin/main comparison semantics unchanged and MAY legitimately be non-zero
after a successful `self-update` WHEN `origin/main` carries untagged commits
beyond the latest release tag; that value alone SHALL NOT drive the TUI's
actionable update banner (it demotes to an informational line). WHEN no
release tag exists anywhere (pre-first-tag bootstrap), `self-update`'s legacy
origin/main convergence applies and a subsequent sync-check SHALL still
report `REPO_BEHIND_ORIGIN=0`. This capability still MUST NOT alter
`sync-check-verdicts`' detection logic, existing VERDICT fields, or exit
codes.

#### Scenario: Post-update release convergence with untagged origin commits

- GIVEN `self-update` completed successfully, converging `main` to tag `v1.5.0`, and `origin/main` carries 2 untagged commits beyond `v1.5.0`
- WHEN sync-check runs afterward
- THEN the VERDICT line reports `REPO_BEHIND_RELEASE=0` and `REPO_BEHIND_ORIGIN=2`, and the TUI's actionable banner is not shown for the origin drift alone

#### Scenario: Pre-first-tag legacy convergence keeps the old claim

- GIVEN no `v*` release tag exists on origin or locally
- WHEN `self-update` completes successfully via the legacy origin/main fallback
- THEN a subsequent sync-check reports `REPO_BEHIND_ORIGIN=0`

## Open Questions for Design

None. The R-007 conflict flagged by sdd-spec is resolved by sdd-design
decision D2 (see `design.md`): post-self-update "up to date" is redefined as
release-based via the new `REPO_BEHIND_RELEASE` VERDICT field, raw
`REPO_BEHIND_ORIGIN` drift is demoted to informational, and the MODIFIED
R-007 above restates the standing spec's observability claim accordingly.
