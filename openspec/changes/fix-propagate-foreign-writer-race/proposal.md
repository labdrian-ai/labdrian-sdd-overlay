# Proposal: Close the read-side foreign-writer race in propagate

## Intent

Fix #1889 (`runPropagateVerified`) added a bounded write-then-verify-then-retry loop
so `propagate` stops trusting a write that a foreign, uncoordinated writer
(`gentle-ai skill-registry refresh`, a separate Homebrew binary that regenerates
`.atl/skill-registry.md` wholesale and ignores our `<registry>.lock`) clobbered.
Both hooks run in PARALLEL on every `UserPromptSubmit`, and the wipe was reproduced
live: refresh erased all 3 scope blocks. But two `judgment-day` judges (both agreeing)
confirmed the fix only covers the WRITE side. The READ side still fails:

1. Torn/empty read → `runPropagateCore` `exit(1)` immediately (main.go ~705), even though
   its own comment says an empty registry IS the signature of a concurrent torn read.
   The wrapper treats any `coreExit != -1` as a non-retryable hard failure, so it never
   retries — a transient torn read becomes a hard failure.
2. Absent read without `--require-registry` → treated as "project does not use the overlay,
   no-op" and returns exit 0 (main.go ~684-694). Since nothing was written, the wrapper
   forwards it as SUCCESS — a false success from the read side, exactly the failure mode
   this fix claims to eliminate.

## Scope

### In Scope
- Extend the bounded retry loop so a torn/empty read (currently the `exit(1)` at
  main.go ~705) is treated as RETRYABLE inside `maxPropagateWriteAttempts`, not a
  terminal hard failure — then fail loud only after exhausting attempts.
- Treat a transient `os.IsNotExist` read as potentially a mid-refresh race window and
  retry within the same bounded loop before concluding the genuine "overlay not used"
  no-op. After attempts are exhausted with the file still absent, keep the existing
  exit-0 no-op (a persistently absent registry really is not-our-project).
- Distinguish retryable read-race exits from genuine hard failures (bad args, unreadable
  contract, broken frontmatter) so those still forward immediately with no retry.
- New TDD tests covering both read-side races (RED before GREEN), mirroring the 4
  existing write-side tests.

### Out of Scope
- Corrupt `BEGIN`-without-matching-`END` block in `propagator.go replaceBlock` (treated
  as "already correct" forever). Preexisting bug NOT introduced by this fix; flagged by
  the judges at lower severity. Declared explicit technical debt for a separate change
  unless the user opts to fold it in.
- Any rewrite of the already-landed write-side fix. Both judges rated it sound (no-op
  path never rewrites, propagate-vs-propagate lock preserved, tests non-tautological).
  This change only ADDS read-side coverage on top of it.

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- None at spec level (bugfix hardening an existing behavior). If the team wants the
  read-side retry guarantee captured as a requirement, `sdd-spec` may add a delta to the
  propagate/skill-registry capability; otherwise mark "None".

## Approach

Keep `runPropagateCore` byte-for-byte unchanged (its stateless map-backed mocks in ~20
existing tests forbid adding verification reads inside it — same constraint #1889 hit).
Extend `runPropagateVerified` only: classify the core's exit so the empty-registry
`exit(1)` and the transient-absent no-op become RETRYABLE signals inside the existing
`maxPropagateWriteAttempts=3` loop, distinct from genuine hard failures that still pass
through untouched. The exact classification mechanism (sentinel stderr match vs. a
richer core-result channel) is a design decision for `sdd-design` — the constraint is:
zero changes to `runPropagateCore`'s signature/behavior, zero collateral to existing
tests.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `engine/cmd/main.go` | Modified | Extend `runPropagateVerified` to retry read-side torn/empty and transient-absent races |
| `engine/cmd/main_test.go` | Modified | New TDD tests for both read-side race paths |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Retrying a GENUINE hard failure (bad args) as if it were a race → masks real errors | Med | Classify retryable vs. terminal exits precisely; hard failures forward immediately, unchanged tests prove it |
| Retrying a legitimately-absent registry adds latency to the common no-op | Low | Bounded to 3 attempts; persistent absence still resolves to the fast exit-0 no-op |
| Distinguishing torn-empty from truly-empty is impossible from bytes alone | Med | Both are non-propagatable; bounded retry then fail-loud is safe for either |

## Rollback

Additive and reversible: the change only extends `runPropagateVerified`. Revert the
diff in `engine/cmd/main.go` + `engine/cmd/main_test.go`, rebuild, redistribute. The
write-side fix (#1889) and `runPropagateCore` are untouched and keep working.

## Dependencies

- Go toolchain to rebuild `gentle-ai-overlay`; redistribute to `~/.claude/bin/`.
- Builds on landed fix #1889 (`runPropagateVerified`), currently uncommitted on branch
  `fix/skill-registry-scope-block-wipe`.

## Success Criteria

- [x] A torn/empty read during a concurrent `skill-registry refresh` is retried and
      succeeds within bounds, instead of an immediate `exit(1)`.
- [x] A transient absent-registry read is retried before concluding no-op; a
      persistently absent registry still returns the exit-0 no-op.
- [x] Genuine hard failures (bad args, unreadable contract, broken frontmatter) still
      forward immediately with no retry — proven by unchanged existing tests.
- [x] New RED-before-GREEN tests cover both read-side race paths.
- [x] Corrupt-block case explicitly documented as deferred technical debt.
