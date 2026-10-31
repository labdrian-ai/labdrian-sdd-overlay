# Delta for sync-check-verdicts

## ADDED Requirements

### Requirement: ACTION line names the release version an apply would bring

ID: R-007
Traces to: overlay-versioned-releases R-004

WHEN an `ACTION:` line recommends running `apply` for a target because its
live digest does not match its recorded version's expected digest (per
overlay-release-state), the system SHALL append the release version that
`apply` would bring to that line, e.g. `ACTION:claude: run 'overlay apply
--target claude' (release v1.5.0 available)`. WHEN a target has no recorded
state yet ("never deployed"), the `ACTION:` line SHALL still recommend
`apply` without fabricating a version.

#### Scenario: Version named when apply is required

- GIVEN `self-update` has fast-forwarded `main` to a new release tag and `apply` has not yet run for target `claude`
- WHEN `overlay sync-check --target claude` runs
- THEN it reports `claude` as requiring `apply`, naming the new release version, not merely a raw commit-behind count

#### Scenario: Never-deployed target recommends apply without a fabricated version

- GIVEN target `claude` has no recorded state (never deployed)
- WHEN `overlay sync-check --target claude` runs
- THEN the `ACTION:` line still recommends `apply` for `claude`, without printing a fabricated or stale version string

### Requirement: VERDICT/status line names the matching version when in sync

ID: R-008
Traces to: overlay-versioned-releases R-004

WHEN a target's live aggregate digest matches its recorded version's expected
digest, `sync-check`/`status` SHALL report that target as in sync at that
named version rather than a bare "in sync" with no version identity.

#### Scenario: In-sync target names its version

- GIVEN `apply` has run and target `claude`'s live digest matches its recorded version's digest
- WHEN `overlay status --target claude` runs
- THEN it reports `claude` as in sync at that named version

## Open Questions for Design

- **Exact field name/wire format** for the new version-carrying data on the
  `VERDICT:`/`ACTION:` lines is not settled by proposal.md (it states only
  "VERDICT/ACTION lines gain release-version fields"). The scenarios above
  are testable independent of the exact field name; design should finalize
  the literal token(s), consistent with the established `KEY=value`
  convention already used for `UPSTREAM_CHANGED`, `OVERLAY_NOT_DEPLOYED`, and
  `REPO_BEHIND_ORIGIN`.
