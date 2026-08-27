# Overlay Release Identity Specification

## Purpose

Gives `labdrian-overlay` a nameable release identity: each published release is
an annotated semver git tag on `main`, cut automatically by CI, and resolvable
by any consumer (self-update, update, version) through one shared,
explicit-fetch tag-resolution operation. Traces to overlay-versioned-releases
R-001.

## Requirements

### Requirement: Semantic version release tag

ID: R-001
Traces to: overlay-versioned-releases R-001

The overlay repository SHALL identify each published release by a semantic
version number recorded as an annotated git tag matching `vMAJOR.MINOR.PATCH`
on the `main` branch.

#### Scenario: Tag exists on the release commit

- GIVEN a maintainer/CI finalizes a set of commits on `main` for release
- WHEN the release-tagging step runs
- THEN an annotated git tag matching `vMAJOR.MINOR.PATCH` exists on that exact commit

#### Scenario: Monotonic ordering across releases

- GIVEN two consecutive release tags
- WHEN their version numbers are compared
- THEN the newer tag's semantic version is strictly greater than the older one's

### Requirement: CI cuts release tags automatically on merge to main

ID: R-002
Traces to: overlay-versioned-releases R-001 (settled by proposal.md "In Scope": CI auto-cuts releases)

WHEN a push lands on `main`, a new `.github/workflows/release.yml` (distinct
from the existing `ci.yml`, which has no release job) SHALL compute the next
semantic version from conventional-commit messages since the last tag,
defaulting to a patch bump WHEN no distinguishing commit type is found, and
SHALL create and push an annotated `vX.Y.Z` tag for that version. IF `HEAD` is
already tagged, THEN the workflow SHALL skip tag creation rather than
double-tagging. The workflow SHALL run with `contents: write` permission and
`fetch-depth: 0` (full history) so the bump can be computed from tag history.

#### Scenario: New commits trigger a release tag

- GIVEN a push lands on `main` with commits since the last tag and HEAD is not yet tagged
- WHEN `release.yml` runs
- THEN it computes the next version from conventional-commit history (default: patch bump) and pushes a new annotated tag on HEAD

#### Scenario: Already-tagged HEAD is skipped

- GIVEN HEAD already carries an annotated release tag
- WHEN `release.yml` runs again for that same commit (e.g., a re-run)
- THEN it creates no additional tag

### Requirement: Explicit-fetch latest-tag resolution

ID: R-003
Traces to: overlay-versioned-releases R-001 (shared infrastructure consumed by tui-self-update R-005, overlay-update-check R-006, overlay-release-surfacing R-010)

WHEN any consumer needs the latest published release, the system SHALL first
explicitly fetch tags from `origin` (closing the gap where a plain branch
fetch does not guarantee tag visibility), THEN resolve the highest
semver-ordered annotated tag reachable from the given ref using semver-aware
comparison, never plain lexical sort.

#### Scenario: Tags fetched explicitly before resolution

- GIVEN a clone whose local tag set is stale relative to `origin`
- WHEN a consumer resolves the latest release tag
- THEN an explicit tag fetch runs first, and the resolved tag reflects `origin`'s current tag set

#### Scenario: Semver-aware ordering, not lexical

- GIVEN tags `v1.9.0` and `v1.10.0` both exist
- WHEN the latest tag is resolved
- THEN `v1.10.0` is selected, not `v1.9.0` (which a plain lexical sort would incorrectly prefer)

## Open Questions for Design

- **Pre-first-tag bootstrap (flagged High risk in proposal.md, unresolved):**
  proposal.md settles that CI cuts releases automatically but does not state
  what happens when `origin` carries zero tags — the very first release.
  Unresolved: (a) what version number the first tag receives (e.g. `v0.1.0`
  vs `v1.0.0`), and (b) how the tag-resolution operation (R-003) and every
  consumer that depends on "the latest tag" (self-update, update, version)
  behave when no tag exists at all yet, without crashing or fabricating a
  version. This is explicitly deferred to `sdd-design`, not resolved here.
- **Conventional-commit bump granularity:** proposal.md states
  "conventional-commit bump, default patch" but does not specify the exact
  commit-type-to-bump-level mapping (e.g. `feat:`→minor, `fix:`→patch,
  `BREAKING CHANGE:`→major). Left to design.
