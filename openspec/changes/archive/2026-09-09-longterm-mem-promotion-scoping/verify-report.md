```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:73bf75ff8119168f3d6cb727566d49022b2c84f5b4d8ea23d0446b833e4d0f8d
verdict: pass
blockers: 0
critical_findings: 0
requirements: 3/3
scenarios: 18/18
test_command: cd longterm-mem && go test ./internal/promote/... ./internal/engram/...
test_exit_code: 0
test_output_hash: sha256:d485afa09d34ff984e9c97f091744313e16ad7661c77b374096336c9b700f185
build_command: cd longterm-mem && go build -o /tmp/claude-1000/-home-labdrian-labdrian-sdd-overlay/54b233bf-75b3-4ffe-9efb-d4c8015a06cf/scratchpad/longterm-mem ./cmd/longterm-mem
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report

**Change**: longterm-mem-promotion-scoping
**Version**: N/A
**Mode**: Strict TDD

### Re-run Context

This is a re-run of verify after a scoped remediation batch. The prior verify
report (this same topic key, evidence_revision
`sha256:3f3513a914a3373668734dbaa0fa8d40da9135e1dc7091232fed085dac9407ff`)
passed 17/17 scenarios against the pre-remediation delta specs. Commits
`b51bf70` (code fix) and `8e1fede` (spec/tasks) added one new scenario to
`specs/longterm-mem-promotion/spec.md` (R-007's "A leading-slash topic_key
has an empty first segment and is excluded"), raising the authoritative
scenario count from 17 to 18. This report re-derives and re-verifies all 18
scenarios against the current spec and code state at HEAD (`380d9ff`).

### Entry Contract vs Realized Delivery

| Field | Planned | Realized |
|---|---|---|
| Review slices | 1 (`promotion-scoping`) | 1 PR/work unit (commits 08f3e81, 98a7689, 38684a3, b51bf70, 8e1fede, 380d9ff) |
| Size exception | N/A | `granted` — 440 total changed lines incl. SDD artifacts |

Planned and realized slice counts match (1 planned, 1 realized). Size exception was granted by the maintainer per `entry.json`.

### Completeness
| Metric | Value |
|--------|-------|
| Tasks total | 20 core + 5 remediation = 25 |
| Tasks complete | 25 |
| Tasks incomplete | 0 |

### Build & Tests Execution

**Build**: ✅ Passed
```text
$ cd longterm-mem && go build -o /tmp/claude-1000/-home-labdrian-labdrian-sdd-overlay/54b233bf-75b3-4ffe-9efb-d4c8015a06cf/scratchpad/longterm-mem ./cmd/longterm-mem
(no output, exit 0)
```

**Tests (focused)**: ✅ 144 passed / ❌ 0 failed / ⚠️ 0 skipped
```text
$ cd longterm-mem && go test -count=1 -v ./internal/promote/... ./internal/engram/...
ok  	.../longterm-mem/internal/promote	1.384s
ok  	.../longterm-mem/internal/engram	1.060s
```
`TestEligible` now has 18 subtests (11 original + 7 remediation cases: bare
`sdd`, bare non-excluded key, whitespace-only, leading-slash `/sdd/auth`,
leading-slash `/sdd/x`, bare `/`, uppercase `SDD/x`), all PASS.

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
longterm-mem: sync --dry-run: would promote 90 observation(s), patch 0 page(s), skip 538, into /home/labdrian/labdrian-brain
longterm-mem: sync --dry-run: nothing was written
```

Cross-checked directly against SQLite (read-only): counting rows for project `labdrian-sdd-overlay`, not soft-deleted, not pinned, and (topic_key NULL/empty OR first path segment in {sdd, review, delivery} OR topic_key starting with `/`) returns exactly **538** — matching the CLI's skip count exactly. Total non-deleted rows for the project is 628; 628 − 538 = 90, matching the would-promote count exactly. No row in the live DB currently carries a leading-slash `topic_key` (checked directly), so the new remediation scenario has no live-DB manifestation today; its coverage is proven at the unit level (`TestEligible`) and the `plan_test.go` integration assertion (R.3), not by live-DB acceptance evidence — consistent with R-007's scenario being a defensive/edge-case fix rather than an observed production pattern.

The 90-vs-89/538-vs-536 drift from the prior verify run's 89/536 is expected ordinary DB growth between sessions (2 new rows), not a predicate regression — consistent with the same drift noted and accepted in the prior verify report.

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
| R-007 | A leading-slash topic_key has an empty first segment and is excluded | `eligible_test.go > TestEligible/leading-slash_topic_key_has_an_empty_first_segment_and_is_not_eligible` + `.../leading-slash_before_a_non-excluded_segment.../a_single_slash_...` | ✅ COMPLIANT |
| R-007 | High-revision, decision-typed, unpinned, untopiced observation is not eligible | `eligible_test.go > TestEligible/high-revision,_decision-typed,_unpinned,_untopiced_observation_is_not_eligible` | ✅ COMPLIANT |
| R-007 | Pinned observation overrides both the untopiced and prefix exclusions | `eligible_test.go > TestEligible/pinned_observation_overrides_both_the_untopiced_and_prefix_exclusions` | ✅ COMPLIANT |
| R-009 | Never-promoted eligible observation is promoted | `sync_test.go > TestSync/Never-promoted_eligible_observation_is_promoted` | ✅ COMPLIANT |
| R-009 | Revised eligible observation is re-promoted | `sync_test.go > TestSync/Revised_eligible_observation_is_re-promoted` | ✅ COMPLIANT |
| R-009 | Unchanged eligible observation is a no-op | `sync_test.go > TestSync/Unchanged_eligible_observation_is_a_no-op` | ✅ COMPLIANT |
| R-009 | sync --dry-run transcript is the acceptance evidence | manual runtime evidence, cross-checked against SQLite (see Acceptance Evidence above); reinforced by `plan_test.go > TestPlan_WritesNothingAndPredictsWhatSyncThenDoes` which asserts a `sdd/`-prefixed fixture row is skipped by both `Plan` and `Sync` and never appears in `Titles`/promoted page bodies | ✅ COMPLIANT |
| R-020 | A row with a topic_key populates the loaded struct | `store_test.go > TestListObservations_PopulatesTopicKey` | ✅ COMPLIANT |
| R-020 | A NULL topic_key column maps to an empty string | `store_test.go > TestListObservations_PopulatesTopicKey` | ✅ COMPLIANT |
| R-020 | Every observation-loading query path carries topic_key populated | static: all 4 query call sites (`ListObservations`, `ObservationsIncludingDeleted`, `ObservationByID` x1 row/x1 rows) route through the single `scanObservationRow` function, which now scans `topic_key`; source-inspected in `store.go` | ✅ COMPLIANT |

**Compliance summary**: 18/18 scenarios compliant

### Correctness (Static Evidence)

| Requirement | Status | Notes |
|---|---|---|
| R-007 rewritten predicate | ✅ Implemented | `Eligible(obs, explicit) bool { return explicit \|\| obs.Pinned \|\| curatedTopicKey(obs.TopicKey) }` — matches design's Interfaces/Contracts section exactly |
| R-007 leading-slash fix | ✅ Implemented | `curatedTopicKey` now returns `false` when `strings.Cut` yields an empty `head`, closing the bypass an empty first segment previously allowed through |
| R-003 retired criteria removed | ✅ Implemented | `eligibleTypes` and `minEligibleRevisionCount` deleted, not deprecated, per design decision |
| R-020 TopicKey exposure | ✅ Implemented | `Observation.TopicKey` field added; `topic_key` appended to `observationColumns`; scanned via `sql.NullString` alongside existing nullable columns |
| Exact-segment matching | ✅ Implemented | `strings.Cut(strings.TrimSpace(key), "/")` against `excludedTopicPrefixes` map, case-sensitive, empty-head guarded |

### Coherence (Design)

| Decision | Followed? | Notes |
|---|---|---|
| Excluded set as unexported package-level map in `promote` | ✅ Yes | `excludedTopicPrefixes` in `eligible.go`; doc comment refreshed in remediation (R.4) |
| Exact, case-sensitive first-segment match via `strings.Cut` | ✅ Yes | Implemented exactly as specified, with an empty-head guard added in remediation |
| `topic_key` scanned through `sql.NullString` | ✅ Yes | Follows existing `syncID`/`deletedAt` pattern in `scanObservationRow` |
| `eligibleTypes`/`minEligibleRevisionCount` deleted, not deprecated | ✅ Yes | Both identifiers removed from `eligible.go`; `Observation.Type`/`RevisionCount` retained for other consumers |
| Fixture sweep scoped by call path, not fixture type | ✅ Yes | `update_test.go`/`reconcile*_test.go` confirmed untouched (task 4.6); `writer_test.go`, `projectmove_test.go`, `projectmove_recovery_test.go` got direct `TopicKey` literals; `sync_test.go`/`plan_test.go`/`propagate_test.go` got `fixtureObs.topicKey` via shared helper |
| No signature change to `Eligible(obs, explicit)` or its two callers | ✅ Yes | `sync.go:119` and `writer.go:77` call sites untouched per diff |
| No migration; connection stays read-only | ✅ Yes | Diff touches no schema, no vault, no DSN construction |
| Stale-page retention documented, not auto-remediated | ✅ Yes | README's `sync` row states a previously-promoted page is neither retracted nor refreshed once its observation stops being eligible; a `doctor` check is explicitly deferred as a follow-up, not silently dropped |

### TDD Compliance

| Check | Result | Details |
|-------|--------|---------|
| TDD Evidence reported | ✅ | apply-progress.md documents Phase 1–5 plus a "Remediation batch" section with its own RED/GREEN/REFACTOR table for R.1–R.3 |
| All tasks have tests | ✅ | 25/25 tasks (20 core + 5 remediation); every code-changing task has an explicit RED/GREEN pair |
| RED confirmed (tests exist) | ✅ | `eligible_test.go` (TestEligible, 18 subtests) and `store_test.go` (TestListObservations_PopulatesTopicKey) both exist and were read directly |
| GREEN confirmed (tests pass) | ✅ | 144/144 focused tests pass on this run; 0 failures |
| Triangulation adequate | ✅ | TestEligible has 18 distinct subtests covering all 11 R-007 spec scenarios (three of them split into 2 subtests each for symmetric no-slash/leading-slash or `sdd-init`/`sddx` pairs); no single-case behaviors |
| Safety Net for modified files | ✅ | `eligible.go`, `store.go`, `plan_test.go` are pre-existing files; their full pre-existing test suites all still pass alongside the new cases |

**TDD Compliance**: 6/6 checks passed

---

### Test Layer Distribution

| Layer | Tests | Files | Tools |
|-------|-------|-------|-------|
| Unit | 144 | 11 (`eligible_test.go`, `store_test.go`, `sync_test.go`, `plan_test.go`, `propagate_test.go`, `writer_test.go`, `projectmove_test.go`, `projectmove_recovery_test.go`, `sync_dryrun_test.go`, plus package-level fixtures) | go test / sqlite3 fixture DBs |
| Integration | 1 (`plan_test.go`'s R.3 assertion, counted within the 144 unit total above since it runs via `go test`) | 1 | in-memory fixture store, real `Plan`/`Sync` code paths |
| E2E | 1 manual acceptance run | `longterm-mem sync --dry-run` against live read-only `~/.engram/engram.db` | binary built from `./cmd/longterm-mem`, cross-checked via `sqlite3 -readonly` |
| **Total** | **144 automated + 1 manual acceptance** | **11 + 1 acceptance transcript** | |

---

### Changed File Coverage

Coverage analysis skipped — no coverage tool configured in this Strict TDD run; not treated as a failure per `strict-tdd-verify.md` Step 5d (informational only when unavailable).

---

### Assertion Quality

Reviewed `eligible_test.go` (TestEligible, 18 table-driven subtests), `store_test.go` (TestListObservations_PopulatesTopicKey), and `plan_test.go`'s R.3 additions directly:

- No tautologies (`expect(true).toBe(true)` equivalents).
- No orphan empty-collection assertions — every subtest asserts a concrete boolean or string value against production code (`Eligible(...)`, `store.ListObservations(...)`, `Plan(...)`/`Sync(...)`).
- No ghost loops over possibly-empty collections gating the only assertions — `plan_test.go`'s title/body loops assert a negative condition (`SDD Bookkeeping` must never appear) over a fixture guaranteed non-empty by the test's own setup.
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
- The 90-vs-89/538-vs-536 one/two-row drift between the prior verify run's captured acceptance evidence and this re-run is expected ordinary DB growth between sessions, not a defect; this is the same class of drift noted (and accepted) in the prior verify report.
- No live row in `~/.engram/engram.db` currently carries a leading-slash `topic_key`, so the remediation's headline fix (R.1) has no live acceptance-evidence manifestation today. Its coverage rests entirely on the unit-level `TestEligible` cases and the `plan_test.go` integration assertion, which is sufficient proof of correctness but means the fix's practical impact cannot be independently observed against production data at this time.

### Verdict
PASS
All 25 tasks (20 core + 5 remediation) complete, all 18 spec scenarios (11 R-007 + 4 R-009 + 3 R-020) compliant with passing runtime evidence, 144/144 focused tests green, 17/17 broad packages green, design decisions followed exactly including the remediation's empty-head guard and stale-page documentation, acceptance evidence independently cross-checked against raw SQLite and matches the CLI dry-run output exactly.
