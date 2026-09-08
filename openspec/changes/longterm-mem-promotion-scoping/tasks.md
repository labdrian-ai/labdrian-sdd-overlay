# Tasks: Curated topic_key gates automatic vault promotion

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 220–320 |
| 400-line budget risk | Medium |
| Chained PRs recommended | No |
| Suggested split | single PR |
| Delivery strategy | single-pr |
| Chain strategy | none |

Decision needed before apply: Yes
Chained PRs recommended: No
Chain strategy: none
400-line budget risk: Medium

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | Store carries `TopicKey`; predicate rewritten; fixture sweep scoped by call path; README updated | single PR | `cd longterm-mem && go test ./internal/promote/... ./internal/engram/...` | `longterm-mem sync --project labdrian-sdd-overlay --dry-run` transcript attached to verify report | revert the single commit — no schema/vault migration involved |

## Phase 1: Store adapter — `topic_key` column

- [x] 1.1 RED: `longterm-mem/internal/engram/store_test.go` — assert `ListObservations`/`ObservationByID` populate `Observation.TopicKey`, including a NULL row mapping to `""`. Confirm it fails to compile (no field yet).
- [x] 1.2 GREEN: `longterm-mem/internal/engram/store.go` — add `TopicKey string` to `Observation`; append `topic_key` to `observationColumns`; scan it through `sql.NullString` in `scanObservationRow` alongside `syncID`/`deletedAt`.
- [x] 1.3 Run `cd longterm-mem && go test ./internal/engram/...` and confirm GREEN.

## Phase 2: Eligibility predicate

- [x] 2.1 RED: `longterm-mem/internal/promote/eligible_test.go` — add one case per design scenario: untopiced `decision` excluded; curated `discovery`, revision 0, `longterm-mem/...` key eligible regardless of type/revision; `sdd/...`, `review/...`, `delivery/...` excluded; exact-first-segment only (`sdd-init/...`, `sddx/...` eligible); high-revision decision-typed unpinned untopiced observation not eligible; explicit override eligible; pinned override eligible.
- [x] 2.2 GREEN: `longterm-mem/internal/promote/eligible.go` — rewrite `Eligible(obs, explicit) bool` as `explicit || obs.Pinned || curatedTopicKey(obs.TopicKey)`; add `curatedTopicKey` using `head, _, _ := strings.Cut(strings.TrimSpace(key), "/")` against `excludedTopicPrefixes = map[string]bool{"sdd": true, "review": true, "delivery": true}`; delete `eligibleTypes` and `minEligibleRevisionCount`.
- [x] 2.3 Run `cd longterm-mem && go test ./internal/promote/... -run Eligible` and confirm GREEN.

## Phase 3: Fixture infrastructure for store-backed tests

- [x] 3.1 GREEN: `longterm-mem/internal/promote/sync_test.go` — add `topicKey` field to `fixtureObs` (line ~25-32) and `topic_key` to the `INSERT INTO observations` column list in `newFixtureEngramStore` (line ~47-77).
- [x] 3.2 Set curated `topicKey` on the 7 `fixtureObs` rows in `longterm-mem/internal/promote/sync_test.go` that reach `Eligible` via `decidePromotion`.

## Phase 4: Fixture sweep — call-path scoped

- [x] 4.1 `longterm-mem/internal/promote/writer_test.go` — set curated `TopicKey` on the 18 `Promote(obs, false)` `engram.Observation` literals; leave the "Not Eligible" fixture (line ~634) untouched.
- [x] 4.2 `longterm-mem/internal/promote/projectmove_test.go` — set curated `TopicKey` on the 7 `Promote(obs, false)` literals.
- [x] 4.3 `longterm-mem/internal/promote/projectmove_recovery_test.go` — set curated `TopicKey` on the 5 `Promote(obs, false)` literals.
- [x] 4.4 `longterm-mem/internal/promote/plan_test.go` — set curated `topicKey` on the 6 `fixtureObs` rows that reach `Eligible` via `Plan` → `decidePromotion`.
- [x] 4.5 `longterm-mem/internal/promote/propagate_test.go` — set curated `topicKey` on the 11 `fixtureObs` rows.
- [x] 4.6 Confirm no edits touch `longterm-mem/internal/promote/update_test.go` or `reconcile_test.go`/`reconcile_guard_test.go`/`reconcile_repair_test.go` — they call `UpdateInPlace()`/`Reconcile()` and never reach `Eligible`.

## Phase 5: Verification and documentation

- [x] 5.1 Run focused suite: `cd longterm-mem && go test ./internal/promote/... ./internal/engram/...` — all GREEN.
- [x] 5.2 Run broad suite: `cd longterm-mem && go vet ./... && go test ./...` — all GREEN.
- [x] 5.3 Update `longterm-mem/README.md` — restate what "eligible" means for `sync` under R-007 (curated `topic_key` gate, no more type/revision criteria).
- [x] 5.4 Capture acceptance evidence: run `longterm-mem sync --project labdrian-sdd-overlay --dry-run`; confirm none of the 182 `sdd/`, 4 `delivery/`, 1 `review/`, 58 untopiced evidence rows appear unless pinned or explicit; attach the transcript to the verify report.
