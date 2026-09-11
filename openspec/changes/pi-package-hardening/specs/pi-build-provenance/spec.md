# Pi Build Provenance Specification

## Purpose

Record which source ref a deployed Pi package was actually built from, and
make `sync-check --target pi` compare against that ref instead of the
currently checked-out branch, closing #315's false-drift reports.

## Requirements

### Requirement: builtFrom Recorded at Build Time

Traces to: R-005

WHEN the overlay builds a Pi package for deployment, the overlay SHALL
write the resolved source commit SHA into that package's `package.json`
under a `labdrian.builtFrom` field.

#### Scenario: builtFrom equals the resolved build commit

- GIVEN `cmd_apply` builds and deploys a package from `main` at commit
  `abc123`
- WHEN the deployed `package.json` is inspected
- THEN `labdrian.builtFrom` SHALL equal `abc123`

### Requirement: Sync-Check Compares Against the Recorded Ref

Traces to: R-006

WHEN `sync-check --target pi` runs and the deployed package's
`package.json` carries a resolvable `labdrian.builtFrom` ref, `sync-check
--target pi` SHALL compare the deployed package's files against that ref's
git tree rather than against the current working-tree checkout.

#### Scenario: Unrelated feature-branch diffs do not cause drift

- GIVEN a deployed package built from `main` at commit `abc123` and a
  feature branch checked out with unrelated changes
- WHEN `sync-check --target pi` runs
- THEN it SHALL report no drift caused by the branch's unrelated changes

#### Scenario: Real divergence from the built ref is still reported

- GIVEN `main` has genuinely diverged from the deployed package (e.g. a
  source file changed after build)
- WHEN `sync-check --target pi` runs
- THEN it SHALL report that real drift

### Requirement: Unresolvable Ref Discloses a Main-Only Comparison

Traces to: R-007

IF `sync-check --target pi` cannot resolve the deployed package's
`labdrian.builtFrom` ref locally, THEN `sync-check --target pi` SHALL
compare against `main` and SHALL state in its output that the comparison
target was `main`, not the recorded build ref.

#### Scenario: Fallback comparison is disclosed

- GIVEN a deployed package whose `labdrian.builtFrom` ref does not exist in
  the local repository
- WHEN `sync-check --target pi` runs
- THEN its output SHALL include an explicit statement that it compared
  against `main`
