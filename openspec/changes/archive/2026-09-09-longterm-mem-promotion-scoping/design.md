# Design: Curated topic_key gates automatic vault promotion

## Technical Approach

Two seams, one direction of dependency. The `engram` store adapter starts carrying a field it already stores but never reads (`topic_key`); the `promote` package replaces its eligibility predicate with a rule over that field. No new type, no new package, no config surface, no signature change: `Eligible(obs, explicit) bool` and its two callers (`sync.go:119`, `writer.go:77`) are untouched. Implements R-001..R-005 from the proposal.

## Architecture Decisions

### Decision: the excluded set is an unexported package-level map in `promote`

**Choice**: `var excludedTopicPrefixes = map[string]bool{"sdd": true, "review": true, "delivery": true}` in `eligible.go`, mirroring the shape of the `eligibleTypes` map it replaces.
**Alternatives considered**: exported slice/API for reuse; a CLI flag or config file.
**Rationale**: no second consumer exists (YAGNI). A config surface would turn a confirmed product policy (obs #3287) into a runtime setting nobody asked for, and every value would need validation and documentation. Reverting the commit restores the old rule; that is the rollback.

### Decision: first-segment match is exact, case-sensitive, on `strings.Cut`

**Choice**: `head, _, _ := strings.Cut(strings.TrimSpace(key), "/")`, then `!excludedTopicPrefixes[head]`. A key with no `/` is its own first segment (`"longterm-mem"` → eligible).
**Alternatives considered**: `strings.HasPrefix(key, "sdd/")` — rejected, it misses a bare `"sdd"` key and matches `"sddx/..."` only by luck of the slash; `strings.Split` — allocates a slice to read one element; `EqualFold` — rejected below.
**Rationale**: `strings.Cut` is the stdlib one-liner for exactly this (rung 2). Case folding is deliberately NOT applied: every topic key in the evidence sample (obs #3283) is lowercase, and folding would silently widen a policy the owner stated in lowercase. Residual risk recorded below.

### Decision: `topic_key` is scanned through `sql.NullString`

**Choice**: append `topic_key` to the end of `observationColumns`, add `TopicKey string` to `Observation`, and scan it through a `sql.NullString` alongside the existing `syncID`/`deletedAt` locals in `scanObservationRow`.
**Alternatives considered**: `COALESCE(topic_key, '')` in the SELECT list; a second scan function.
**Rationale**: the column is nullable and a substantial minority of rows are NULL (58 of 282). `scanObservationRow` already documents the nullable-through-`sql.NullString` pattern and the production incident (one NULL `sync_id` errored an entire list) that motivated it; following it keeps one convention instead of two. Appending to the column list keeps the SELECT order and scan order aligned with the smallest diff, and all four query sites share the one constant.

### Decision: `eligibleTypes` and `minEligibleRevisionCount` are deleted, not deprecated

**Choice**: delete both identifiers. `Observation.Type` and `Observation.RevisionCount` stay — `page.go`/`frontmatter.go` still emit `engram_type` and `engram_revision`.
**Alternatives considered**: keep them behind a flag or comment.
**Rationale**: R-003 retires them as criteria; dead eligibility constants next to a live predicate are exactly the residue a future reader mistakes for policy.

### Decision: only fixtures that actually reach `Eligible` get a curated topic key

**Choice**: two disjoint groups, identified by call path, not by fixture type.

- **Direct `Promote(obs, false)`** — 30 sites: `writer_test.go` (18), `projectmove_test.go` (7), `projectmove_recovery_test.go` (5). Their `engram.Observation` literals gain `TopicKey`.
- **Store-backed `Plan`/`Sync`/`Propagate`** — `sync_test.go`, `plan_test.go`, `propagate_test.go` build rows through the shared `fixtureObs` struct (`sync_test.go:25-32`) inserted by raw SQL in `newFixtureEngramStore` (`sync_test.go:47-77`). `Plan` (`sync.go:167,175`) reaches `Eligible` via `decidePromotion` (`sync.go:118-119`), so these need the infrastructure change below, not a literal edit.

Out of scope: `update_test.go` calls `UpdateInPlace()` directly, and `reconcile_test.go`/`reconcile_guard_test.go`/`reconcile_repair_test.go` call `Reconcile(vaultRoot, project, address)`. Neither path reaches `Eligible`, so their `Type: "decision"` fixtures are untouched. `writer_test.go:634`'s "Not Eligible" fixture keeps no topic key and keeps proving the skip.

**Alternatives considered**: flipping the 30 call sites to `explicit=true` — rejected, it would stop exercising the automatic path those tests exist to cover; blanket-tagging every `Type: "decision"` fixture in the package — rejected, it would edit ~40 fixtures on paths that never consult eligibility.
**Rationale**: eligibility is a precondition of the mechanics under test, not their subject, and only on the paths that evaluate it. Scoping by call path is what keeps this diff at 220–320 lines instead of 400+.

### Decision: `fixtureObs` carries `topicKey` rather than each test inserting its own SQL

**Choice**: add a `topicKey` field to `fixtureObs` and `topic_key` to the `INSERT INTO observations` column list in `newFixtureEngramStore`, then set it on the ~24 fixture rows across `sync_test.go` (7), `propagate_test.go` (11), and `plan_test.go` (6).
**Alternatives considered**: a default curated key applied inside the helper for every row.
**Rationale**: `newFixtureEngramStore` is the single seam through which all three files reach the real `engram.Open` path, so the column belongs there once. A silent default is rejected: it would make an untopiced row unrepresentable in exactly the tests that must be able to express one.

## Data Flow

    SQLite observations ──scanObservationRow──→ engram.Observation{TopicKey}
                                                        │
                            sync.decidePromotion ───────┤
                            Writer.Promote      ───────→ Eligible(obs, explicit)
                                                        │
                            explicit ─ pinned ─ curatedTopicKey(TopicKey)
                                                        │
                                                 promote / skip

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `longterm-mem/internal/engram/store.go` | Modify | `TopicKey` field; `topic_key` appended to `observationColumns`; `sql.NullString` scan |
| `longterm-mem/internal/engram/store_test.go` | Modify | Assert `TopicKey` populated, including a NULL row |
| `longterm-mem/internal/promote/eligible.go` | Modify | New predicate + `curatedTopicKey`; delete `eligibleTypes`, `minEligibleRevisionCount` |
| `longterm-mem/internal/promote/eligible_test.go` | Modify | R-001/R-002/R-004/R-005 table; retired-criteria cases |
| `longterm-mem/internal/promote/sync_test.go` | Modify | `topicKey` field on `fixtureObs`; `topic_key` in `newFixtureEngramStore`'s INSERT; 7 rows |
| `longterm-mem/internal/promote/plan_test.go` | Modify | Curated `topicKey` on 6 fixture rows (`Plan` → `decidePromotion` → `Eligible`) |
| `longterm-mem/internal/promote/propagate_test.go` | Modify | Curated `topicKey` on 11 fixture rows |
| `longterm-mem/internal/promote/writer_test.go` | Modify | `TopicKey` on 18 `Promote(obs, false)` fixtures; "Not Eligible" fixture unchanged |
| `longterm-mem/internal/promote/projectmove_test.go` | Modify | `TopicKey` on 7 `Promote(obs, false)` fixtures |
| `longterm-mem/internal/promote/projectmove_recovery_test.go` | Modify | `TopicKey` on 5 `Promote(obs, false)` fixtures |
| `longterm-mem/README.md` | Modify | `sync` row states what "eligible" now means |

## Interfaces / Contracts

```go
func Eligible(obs engram.Observation, explicit bool) bool {
	return explicit || obs.Pinned || curatedTopicKey(obs.TopicKey)
}
```

## Testing Strategy

Strict TDD. Focused: `cd longterm-mem && go test ./internal/promote/... ./internal/engram/...`. Broad: `go vet ./... && go test ./...`.

| Step | State | What |
|---|---|---|
| 1 | RED | `store_test.go`: `ListObservations`/`ObservationByID` carry `topic_key`, NULL → `""`. Fails to compile (no field) — that is the RED. |
| 2 | GREEN | Add field, column, `sql.NullString` scan. |
| 3 | RED | `eligible_test.go`, one case per spec scenario: untopiced `decision` excluded; "Curated topic_key is eligible regardless of type or revision count" (`discovery`, revision 0, `longterm-mem/...`); `sdd`/`review`/`delivery` excluded; exact-first-segment only (`sdd-init/...`, `sddx/...` eligible); "High-revision, decision-typed, unpinned, untopiced observation is not eligible" (type `decision`, revision 5, empty key); explicit and pinned override both exclusions. |
| 4 | GREEN | Rewrite `Eligible`, add `curatedTopicKey`, delete the two retired identifiers. |
| 5 | GREEN | Infrastructure: `topicKey` on `fixtureObs` + `topic_key` in `newFixtureEngramStore`'s INSERT, so store-backed tests can express a curated and an untopiced row. |
| 6 | RED→GREEN | Run broad suite; set the curated key on the 30 `Promote(obs, false)` literals and the ~24 `fixtureObs` rows. No edit to `update_test.go` or the `reconcile*` tests — they never reach `Eligible`. |
| 7 | Acceptance | `longterm-mem sync --project labdrian-sdd-overlay --dry-run` transcript: none of the 182 `sdd/`, 4 `delivery/`, 1 `review/`, 58 untopiced evidence rows appear unless pinned or explicit. Attach to the verify report. |

## Threat Matrix

N/A — no routing, shell, subprocess, VCS/PR automation, executable-file classification, or process-integration boundary is changed. The promote path's existing `allocate-address.sh` subprocess and the read-only SQLite DSN are untouched; this change narrows a pure in-memory classification and adds one nullable column read.

## Migration / Rollout

No migration. The column exists; the connection stays read-only; no vault or sidecar state changes shape. Already-promoted noise pages remain (out of scope, per proposal).

## Open Questions

- [ ] None blocking. Residual: a topic key like `SDD/...` would not be excluded (case-sensitive by decision); no such key exists in the evidence sample.
- [ ] Budget: 220–320 authored lines (core predicate and store ~60; 30 fixture literals; ~24 `fixtureObs` rows plus the helper). Single PR remains credible under the 400-line budget. The estimate holds only while the fixture sweep stays scoped to the paths that reach `Eligible`; if apply finds itself editing `update_test.go` or the `reconcile*` tests, the scope is wrong, not the budget.
