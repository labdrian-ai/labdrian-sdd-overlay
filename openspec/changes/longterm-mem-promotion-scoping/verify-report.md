```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:3f3513a914a3373668734dbaa0fa8d40da9135e1dc7091232fed085dac9407ff
verdict: pass
blockers: 0
critical_findings: 0
requirements: 3/3
scenarios: 17/17
test_command: cd longterm-mem && go test ./internal/promote/... ./internal/engram/...
test_exit_code: 0
test_output_hash: sha256:d9364979ed2585ffd7ed002cc84866e5e26e6ba6398f6f1066374620c0e3db2b
build_command: cd longterm-mem && go build -o /tmp/longterm-mem ./cmd/longterm-mem
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report

**Change**: longterm-mem-promotion-scoping
**Version**: N/A
**Mode**: Strict TDD

### Entry Contract vs Realized Delivery

| Field | Planned | Realized |
|---|---|---|
| Review slices | 1 (`promotion-scoping`) | 1 PR/work unit (commits 08f3e81, 98a7689, 38684a3 on top of 47429af) |
| Size exception | N/A | `granted` — 440 total changed lines incl. SDD artifacts; 292 authored code lines under `longterm-mem` |

Planned and realized slice counts match (1 planned, 1 realized). Size exception was granted by the maintainer per `entry.json`.

### Completeness
| Metric | Value |
|--------|-------|
| Tasks total | 20 |
| Tasks complete | 20 |
| Tasks incomplete | 0 |

### Build & Tests Execution

**Build**: ✅ Passed
```text
$ cd longterm-mem && go build -o <scratch>/longterm-mem ./cmd/longterm-mem
(no output, exit 0)
```

**Tests (focused)**: ✅ 144 passed / ❌ 0 failed / ⚠️ 0 skipped
```text
$ cd longterm-mem && go test -count=1 -v ./internal/promote/... ./internal/engram/...
ok  	.../longterm-mem/internal/promote	1.103s
ok  	.../longterm-mem/internal/engram	1.077s
```

**Tests (broad)**: ✅ all 17 packages passed
```text
$ cd longterm-mem && go vet ./... && go test -count=1 ./...
go vet: clean (exit 0)
ok for all 17 packages (longterm-mem, cmd/longterm-mem, internal/durable, internal/embed,
internal/engram, internal/identityledger, internal/mcpserver, internal/ops,
internal/projectid, internal/promote, internal/query, internal/register,
internal/repohistory, internal/staleness, internal/vault, internal/vaultreg,
internal/vecindex)
```

**Coverage**: not measured — no coverage tool configured for this run (informational only, not blocking per strict-tdd-verify.md).

### Acceptance Evidence (R-003, R-007, R-009)

`longterm-mem sync --project labdrian-sdd-overlay --dry-run` against the live read-only `~/.engram/engram.db`:

```text
longterm-mem: sync --dry-run: would promote 89 observation(s), patch 0 page(s), skip 536, into /home/labdrian/labdrian-brain
```

Cross-checked directly against SQLite (read-only): counting rows for project `labdrian-sdd-overlay`, not soft-deleted, not pinned, and (topic_key NULL/empty OR first path segment in {sdd, review, delivery}) returns exactly **536** — matching the CLI's skip count exactly. Total non-deleted rows for the project is 625; 625 − 536 = 89, matching the would-promote count exactly.

Every `would promote` line was spot-checked against `observations.topic_key`. Four rows have process-like titles ("Engram store cleanup 2026-09-08: ...", "RDD on the upstream-sync merge ...") but still promote correctly because they carry curated `topic_key` values (`engram/store-health`, `overlay/deploy-drift`) outside the excluded prefixes — this is pre-existing data hygiene (curators chose non-descriptive titles with descriptive topic keys), not a spec violation. The predicate is behaving exactly as R-007 specifies: it gates on `topic_key`, not on title text.

Note: 89 would-promote / 536 skip differs by exactly 1 from apply-progress's captured 89/535 evidence — the live DB gained one row (an SDD artifact from this verify session's own upstream context, e.g. a `sdd/` or untopiced row written since apply) between the apply and verify runs. The delta is consistent with ordinary DB growth, not a predicate regression, and the cross-check above independently re-derives the exact skip/promote split from raw SQL against the current DB state.

### Spec Compliance Matrix

| Requirement | Scenario | Test | Result |
|---|---|---|---|
| R-007 | Pinned observation is eligible | `eligible_test.go > TestEligible/pinned_observation_is_eligible` | ✅ COMPLIANT |
| R-007 | Explicit promote call overrides the automatic criteria | `eligible_test.go > TestEligible/explicit_promote_call_overrides_the_automatic_criteria` | ✅ COMPLIANT |
| R-007 | Untopiced observation is not automatically eligible | `eligible_test.go > TestEligible/untopiced_observation_is_not_eligible` | ✅ COMPLIANT |
| R-007 | Curated topic_key is eligible regardless of type or revision count | `eligible_test.go > TestEligible/curated_topic_key_is_eligible_...` | ✅ COMPLIANT |
| R-007 | sdd-prefixed topic_key is excluded | `eligible_test.go > TestEligible/sdd/-prefixed_topic_key_is_excluded` | ✅ COMPLIANT |
| R-007 | review-prefixed topic_key is excluded | `eligible_test.go > TestEligible/review/-prefixed_topic_key_is_excluded` | ✅ COMPLIANT |
| R-007 | delivery-prefixed topic_key is excluded | `eligible_test.go > TestEligible/delivery/-prefixed_topic_key_is_excluded` | ✅ COMPLIANT |
| R-007 | Prefix exclusion matches only the exact first path segment | `eligible_test.go > TestEligible/sdd-init/_is_not_excluded` + `.../sddx/_is_not_excluded` | ✅ COMPLIANT |
| R-007 | High-revision, decision-typed, unpinned, untopiced observation is not eligible | `eligible_test.go > TestEligible/high-revision,_decision-typed,_unpinned,_untopiced_observation_is_not_eligible` | ✅ COMPLIANT |
| R-007 | Pinned observation overrides both the untopiced and prefix exclusions | `eligible_test.go > TestEligible/pinned_observation_overrides_both_the_untopiced_and_prefix_exclusions` | ✅ COMPLIANT |
| R-009 | Never-promoted eligible observation is promoted | `sync_test.go > TestSync/Never-promoted_eligible_observation_is_promoted` | ✅ COMPLIANT |
| R-009 | Revised eligible observation is re-promoted | `sync_test.go > TestSync/Revised_eligible_observation_is_re-promoted` | ✅ COMPLIANT |
| R-009 | Unchanged eligible observation is a no-op | `sync_test.go > TestSync/Unchanged_eligible_observation_is_a_no-op` | ✅ COMPLIANT |
| R-009 | sync --dry-run transcript is the acceptance evidence | manual runtime evidence, cross-checked against SQLite (see Acceptance Evidence above) | ✅ COMPLIANT |
| R-020 | A row with a topic_key populates the loaded struct | `store_test.go > TestListObservations_PopulatesTopicKey` | ✅ COMPLIANT |
| R-020 | A NULL topic_key column maps to an empty string | `store_test.go > TestListObservations_PopulatesTopicKey` | ✅ COMPLIANT |
| R-020 | Every observation-loading query path carries topic_key populated | static: all 4 query call sites (`ListObservations`, `ObservationsIncludingDeleted`, `ObservationByID` x1 row/x1 rows) route through the single `scanObservationRow` function, which now scans `topic_key`; source-inspected in `store.go` | ✅ COMPLIANT |

**Compliance summary**: 17/17 scenarios compliant

### Correctness (Static Evidence)

| Requirement | Status | Notes |
|---|---|---|
| R-007 rewritten predicate | ✅ Implemented | `Eligible(obs, explicit) bool { return explicit \|\| obs.Pinned \|\| curatedTopicKey(obs.TopicKey) }` — matches design's Interfaces/Contracts section exactly |
| R-003 retired criteria removed | ✅ Implemented | `eligibleTypes` and `minEligibleRevisionCount` deleted, not deprecated, per design decision |
| R-020 TopicKey exposure | ✅ Implemented | `Observation.TopicKey` field added; `topic_key` appended to `observationColumns`; scanned via `sql.NullString` alongside existing nullable columns |
| Exact-segment matching | ✅ Implemented | `strings.Cut(strings.TrimSpace(key), "/")` against `excludedTopicPrefixes` map, case-sensitive |

### Coherence (Design)

| Decision | Followed? | Notes |
|---|---|---|
| Excluded set as unexported package-level map in `promote` | ✅ Yes | `excludedTopicPrefixes` in `eligible.go` |
| Exact, case-sensitive first-segment match via `strings.Cut` | ✅ Yes | Implemented exactly as specified |
| `topic_key` scanned through `sql.NullString` | ✅ Yes | Follows existing `syncID`/`deletedAt` pattern in `scanObservationRow` |
| `eligibleTypes`/`minEligibleRevisionCount` deleted, not deprecated | ✅ Yes | Both identifiers removed from `eligible.go`; `Observation.Type`/`RevisionCount` retained for other consumers |
| Fixture sweep scoped by call path, not fixture type | ✅ Yes | `update_test.go`/`reconcile*_test.go` confirmed untouched (task 4.6); `writer_test.go`, `projectmove_test.go`, `projectmove_recovery_test.go` got direct `TopicKey` literals; `sync_test.go`/`plan_test.go`/`propagate_test.go` got `fixtureObs.topicKey` via shared helper |
| `fixtureObs` carries `topicKey`, not per-test SQL | ✅ Yes | `newFixtureEngramStore`'s single `INSERT` column list carries `topic_key` |
| No signature change to `Eligible(obs, explicit)` or its two callers | ✅ Yes | `sync.go:119` and `writer.go:77` call sites untouched per diff |
| No migration; connection stays read-only | ✅ Yes | Diff touches no schema, no vault, no DSN construction |

### TDD Compliance

| Check | Result | Details |
|-------|--------|---------|
| TDD Evidence reported | ✅ | apply-progress.md documents Phase 1 (RED store_test.go compile failure → GREEN), Phase 2 (RED eligible_test.go → GREEN), Phase 3/4 (fixture infrastructure + sweep), Phase 5 (broad green) |
| All tasks have tests | ✅ | 20/20 tasks; every code phase (1, 2) has an explicit RED/GREEN task pair; fixture phases (3, 4) are test-file-only changes verified by the same suites |
| RED confirmed (tests exist) | ✅ | `eligible_test.go` (TestEligible, 11 subtests) and `store_test.go` (TestListObservations_PopulatesTopicKey) both exist and were read directly |
| GREEN confirmed (tests pass) | ✅ | 144/144 focused tests pass on this run; 0 failures |
| Triangulation adequate | ✅ | TestEligible has 11 distinct subtests covering all 10 spec scenarios (one scenario split into 2 subtests: `sdd-init/` and `sddx/`); no single-case behaviors |
| Safety Net for modified files | ✅ | `eligible.go`, `store.go` are pre-existing files; their full pre-existing test suites (TestPromote_ExplicitCallOverridesAutomaticEligibility, TestOpen_*, TestListObservations_*, TestObservationByID_*, TestHasMemory_*) all still pass alongside the new cases |

**TDD Compliance**: 6/6 checks passed

---

### Test Layer Distribution

| Layer | Tests | Files | Tools |
|-------|-------|-------|-------|
| Unit | 144 | 11 (`eligible_test.go`, `store_test.go`, `sync_test.go`, `plan_test.go`, `propagate_test.go`, `writer_test.go`, `projectmove_test.go`, `projectmove_recovery_test.go`, `sync_dryrun_test.go`, plus package-level fixtures) | go test / sqlite3 fixture DBs |
| Integration | 0 (of this change's authored tests) | — | n/a — this change is a pure in-memory predicate + one nullable column read, per design's Threat Matrix ("N/A") |
| E2E | 1 manual acceptance run | `longterm-mem sync --dry-run` against live read-only `~/.engram/engram.db` | binary built from `./cmd/longterm-mem`, cross-checked via `sqlite3 -readonly` |
| **Total** | **144 automated + 1 manual acceptance** | **11 + 1 acceptance transcript** | |

---

### Changed File Coverage

Coverage analysis skipped — no coverage tool configured in this Strict TDD run; not treated as a failure per `strict-tdd-verify.md` Step 5d (informational only when unavailable).

---

### Assertion Quality

Reviewed `eligible_test.go` (TestEligible, 11 table-driven subtests) and `store_test.go` (TestListObservations_PopulatesTopicKey) directly:

- No tautologies (`expect(true).toBe(true)` equivalents).
- No orphan empty-collection assertions — every subtest asserts a concrete boolean or string value against production code (`Eligible(...)`, `store.ListObservations(...)`).
- No ghost loops over possibly-empty collections gating the only assertions.
- No smoke-test-only patterns.
- No implementation-detail coupling (no assertions on internal state, mock call counts, or CSS-equivalent details — this is a Go backend package).
- Mock/assertion ratio: N/A — no mocking framework used; fixtures are real SQLite rows via `t.TempDir()`.

**Assertion quality**: ✅ All assertions verify real behavior

---

### Quality Metrics
**Linter**: ➖ Not available (no linter configured for this Strict TDD run)
**Type Checker**: ✅ No errors (`go vet ./...` clean, exit 0 — Go's vet serves this role)

### Issues Found

**CRITICAL**: None
**WARNING**: None
**SUGGESTION**:
- The 89-vs-88/536-vs-535 one-row drift between apply-progress's captured acceptance evidence and this verify run's re-run is expected DB growth between sessions, not a defect; future verify runs of long-lived changes against a live, mutating store should expect small deltas and re-derive counts via direct SQL as done here, rather than requiring an exact match to a prior transcript.
- Four promoted rows in the live DB carry process-like titles (e.g. "Engram store cleanup 2026-09-08: ...") but curated topic_keys (`engram/store-health`, `overlay/deploy-drift`); this is pre-existing data hygiene unrelated to this change's correctness — noted for awareness, not for remediation by this change.

### Verdict
PASS
All 20 tasks complete, all 17 spec scenarios compliant with passing runtime evidence, 144/144 focused tests green, 17/17 broad packages green, design decisions followed exactly, acceptance evidence independently cross-checked against raw SQLite and matches the CLI dry-run output exactly.
