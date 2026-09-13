# Pi Build Provenance Specification

## Purpose

Record which source ref a deployed Pi package was actually built from, and
make `sync-check --target pi` always compare against the DEPLOY ref (what
`apply` actually deploys from), disclosing the recorded build ref as
provenance rather than as the comparison target. This closes both #315's
false-drift reports on a feature branch with unrelated changes, and the
converse gap where comparing against the recorded build ref let committed
source changes made after the last build go undetected as drift.

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

### Requirement: Sync-Check Compares Against the Deploy Ref

Traces to: R-006

WHEN `sync-check --target pi` runs, `sync-check --target pi` SHALL compare
the deployed package's files against the DEPLOY ref's git tree (the ref
`apply` actually deploys from) rather than against the current
working-tree checkout and rather than the deployed package's recorded
`labdrian.builtFrom` ref, so that a deployed package left behind by
committed source changes on the deploy ref is still detected as drift even
when it byte-matches what was built at its own (now stale) recorded ref.

#### Scenario: Unrelated feature-branch diffs do not cause drift

- GIVEN a deployed package built from `main` at commit `abc123` and a
  feature branch checked out with unrelated changes
- WHEN `sync-check --target pi` runs
- THEN it SHALL report no drift caused by the branch's unrelated changes

#### Scenario: Real divergence from the deploy ref is still reported

- GIVEN the deploy ref has genuinely diverged from the deployed package
  (e.g. a source file changed and committed on the deploy ref after the
  package was last built), regardless of whether that divergence has also
  advanced the deployed package's own recorded `labdrian.builtFrom` ref
- WHEN `sync-check --target pi` runs
- THEN it SHALL report that real drift, naming the deployed package as
  stale relative to the deploy ref

### Requirement: Unresolvable Ref Discloses a Main-Only Comparison

Traces to: R-007

`sync-check --target pi` SHALL always compare against the deploy ref
(`main`, or its `origin/main`/`HEAD` fallback), independent of whether the
deployed package's `labdrian.builtFrom` ref is locally resolvable. IF
`labdrian.builtFrom` is absent or not locally resolvable, THEN
`sync-check --target pi` SHALL state in its output that the comparison
target was the deploy ref and that the recorded build ref could not be
used as provenance.

#### Scenario: Fallback comparison is disclosed

- GIVEN a deployed package whose `labdrian.builtFrom` ref does not exist in
  the local repository
- WHEN `sync-check --target pi` runs
- THEN its output SHALL include an explicit statement that it compared
  against the deploy ref, with the unresolvable recorded build ref
  disclosed only as provenance
