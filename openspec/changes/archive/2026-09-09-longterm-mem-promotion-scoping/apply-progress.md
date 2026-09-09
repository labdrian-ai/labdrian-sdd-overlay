# Apply Progress: Curated topic_key gates automatic vault promotion

**Change**: longterm-mem-promotion-scoping
**Mode**: Strict TDD
**Status**: 20/20 tasks complete — Ready for verify

## Completed Tasks

### Phase 1: Store adapter — topic_key column
- [x] 1.1 RED: store_test.go asserts TopicKey populated (NULL -> "")
- [x] 1.2 GREEN: store.go adds TopicKey field, appends topic_key to observationColumns, scans via sql.NullString
- [x] 1.3 Run go test ./internal/engram/...

### Phase 2: Eligibility predicate
- [x] 2.1 RED: eligible_test.go rewritten — one case per design scenario
- [x] 2.2 GREEN: eligible.go rewrites Eligible as explicit || obs.Pinned || curatedTopicKey(obs.TopicKey); deletes eligibleTypes/minEligibleRevisionCount
- [x] 2.3 Run go test ./internal/promote/... -run Eligible

### Phase 3: Fixture infrastructure for store-backed tests
- [x] 3.1 sync_test.go: fixtureObs gains topicKey field; newFixtureEngramStore INSERT gains topic_key
- [x] 3.2 Curated topicKey set on 7 fixtureObs rows in sync_test.go

### Phase 4: Fixture sweep — call-path scoped
- [x] 4.1 writer_test.go: TopicKey on 13 `Promote(obs, false)` obs literals (18 call sites via var reuse incl. promoteTwice helper); "Not Eligible" fixture untouched
- [x] 4.2 projectmove_test.go: TopicKey on 7 literals
- [x] 4.3 projectmove_recovery_test.go: TopicKey on 5 literals
- [x] 4.4 plan_test.go: topicKey on 6 fixtureObs rows
- [x] 4.5 propagate_test.go: topicKey on 11 fixtureObs rows
- [x] 4.6 Confirmed update_test.go and reconcile*_test.go untouched (`git diff --stat` empty for those paths)

### Phase 5: Verification and documentation
- [x] 5.1 Focused suite green
- [x] 5.2 Broad suite green (go vet + go test ./...) — including an unlisted but necessary fix to `cmd/longterm-mem/sync_dryrun_test.go`'s raw SQL fixture insert (added `topic_key`), since the eligibility rewrite broke that integration test
- [x] 5.3 README.md: sync row restates R-007 eligibility meaning
- [x] 5.4 Acceptance: sync --dry-run transcript attached below

## Files Changed

| File | Action | What Was Done |
|------|--------|---------------|
| `longterm-mem/internal/engram/store.go` | Modified | `TopicKey` field on `Observation`; `topic_key` appended to `observationColumns`; scanned via `sql.NullString` |
| `longterm-mem/internal/engram/store_test.go` | Modified | New `TestListObservations_PopulatesTopicKey`; `insertObservationWithTopicKey` helper |
| `longterm-mem/internal/promote/eligible.go` | Modified | `Eligible` rewritten to `explicit \|\| obs.Pinned \|\| curatedTopicKey(obs.TopicKey)`; new `curatedTopicKey`/`excludedTopicPrefixes`; deleted `eligibleTypes`, `minEligibleRevisionCount` |
| `longterm-mem/internal/promote/eligible_test.go` | Modified | Table rewritten to R-007's new scenario set (untopiced, curated, sdd/review/delivery exclusion, exact-segment match, high-revision-no-longer-eligible, pinned override) |
| `longterm-mem/internal/promote/sync_test.go` | Modified | `fixtureObs.topicKey` field + INSERT column; curated key on 7 rows |
| `longterm-mem/internal/promote/plan_test.go` | Modified | Curated `topicKey` on 6 rows |
| `longterm-mem/internal/promote/propagate_test.go` | Modified | Curated `topicKey` on 11 rows |
| `longterm-mem/internal/promote/writer_test.go` | Modified | Curated `TopicKey` on 13 obs literals; "Not Eligible" fixture unchanged |
| `longterm-mem/internal/promote/projectmove_test.go` | Modified | Curated `TopicKey` on 7 obs literals |
| `longterm-mem/internal/promote/projectmove_recovery_test.go` | Modified | Curated `TopicKey` on 5 obs literals |
| `longterm-mem/cmd/longterm-mem/sync_dryrun_test.go` | Modified | Added `topic_key` to the raw SQL fixture insert (broad-suite fallout, not in the original scope list, required to keep `go test ./...` green) |
| `longterm-mem/README.md` | Modified | `sync` row states the curated-topic_key eligibility rule |

## TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 1.1–1.3 | `internal/engram/store_test.go` | Unit | ✅ pre-change suite green | ✅ Written — `o.TopicKey undefined` compile error | ✅ Passed (`go test ./internal/engram/...` → ok) | ✅ 2 cases (curated + NULL row) | ➖ None needed |
| 2.1–2.3 | `internal/promote/eligible_test.go` | Unit | ✅ pre-change `TestEligible` green | ✅ Written — 8 new/changed cases, 6 failed against the old predicate | ✅ Passed (11/11 subtests) | ✅ 11 cases across R-001/R-002/R-004/R-005 | ✅ `curatedTopicKey` extracted as a pure function |
| 3.1–3.2, 4.1–4.6 | fixture files listed above | Integration (store-backed + direct `Promote`) | ✅ baseline captured before rewrite (all green) | N/A — fixture-only changes following the RED at 2.1 | ✅ `go test ./internal/promote/... ./internal/engram/...` → ok | ➖ Single (fixture sweep, not new behavior) | ➖ None needed |
| 5.2 fallout fix | `cmd/longterm-mem/sync_dryrun_test.go` | Integration (CLI) | ✅ baseline broad suite had 1 failure caused by this change | N/A — pre-existing test, fixture-only fix | ✅ `go test ./...` → ok | ➖ Single | ➖ None needed |

### Test Summary
- **Total tests written/changed**: 1 new (`TestListObservations_PopulatesTopicKey`), 1 rewritten (`TestEligible`, 11 cases), ~46 fixture literals/rows touched across 8 files
- **Total tests passing**: full `go test ./...` green (all 17 packages)
- **Layers used**: Unit (engram, eligible), Integration (promote store-backed + direct Promote, cmd/longterm-mem CLI)
- **Approval tests**: None — no refactoring-only tasks
- **Pure functions created**: 1 (`curatedTopicKey`)

## Work Unit Evidence

| Evidence | Value |
|---|---|
| Focused test command and exact result | `cd longterm-mem && go test ./internal/promote/... ./internal/engram/...` → `ok .../promote (cached)`, `ok .../engram (cached)` |
| Runtime harness command/scenario and exact result | `longterm-mem sync --project labdrian-sdd-overlay --dry-run` — real binary against the live read-only `~/.engram/engram.db`; transcript below, 89 promoted / 535 skipped, 0 patched, nothing written |
| Rollback boundary | `git revert 08f3e81` (the single implementation commit) — no schema or vault migration involved; the `topic_key` column already existed |

## Acceptance Evidence (R-003) — sync --dry-run transcript

Command: `longterm-mem sync --project labdrian-sdd-overlay --dry-run` (built from this commit, run read-only against `~/.engram/engram.db`)

```
longterm-mem: sync --dry-run: would promote 89 observation(s), patch 0 page(s), skip 535, into /home/labdrian/labdrian-brain
  would promote: sdd-init/labdrian-sdd-overlay
  would promote: Separated labdrian platform from gentle-ai via bin/labdrian-overlay router
  ... (89 titles total, truncated here — full transcript captured during apply)
longterm-mem: sync --dry-run: nothing was written
```

Verified independently via a read-only SQLite query against `~/.engram/engram.db`: 535 rows for project `labdrian-sdd-overlay` match `(topic_key IS NULL OR topic_key='' OR topic_key LIKE 'sdd/%' OR topic_key LIKE 'review/%' OR topic_key LIKE 'delivery/%') AND pinned=0` — exactly the "skip 535" count the CLI reported, and none of their titles appear in the "would promote" list (spot-checked 5 `sdd/`-topiced titles by name). `sdd-init/labdrian-sdd-overlay` correctly appears in the promoted set: first segment `sdd-init` != `sdd` (exact-match rule, R-002 scenario).

## Deviations from Design

None in the core predicate/store change. One addition beyond the design's explicit file list: `cmd/longterm-mem/sync_dryrun_test.go`'s raw SQL fixture needed `topic_key` added to its INSERT — this integration test builds its own observation row outside `fixtureObs`/`newFixtureEngramStore` and started failing (`would promote 0 observation(s)... skip 1`) once the eligibility predicate stopped granting automatic eligibility to an untopiced `decision`-typed row. Fixed with a 2-line change (4 lines in diff stat) to keep the broad suite green, consistent with the "fixture sweep ONLY on paths that reach Eligible" principle — this fixture does reach `Eligible` via `Sync` → `decidePromotion`.

writer_test.go: design said "TopicKey on 18 Promote(obs, false) literals" — the actual count is 13 distinct `engram.Observation{...}` literal declarations (18 is the count of `.Promote(obs, false)` call sites, since several tests reuse the same `obs` variable for a second call, including via the `promoteTwice` helper which mutates `RevisionCount`/`Content` but preserves `TopicKey`). All 18 call sites are covered by the 13 tagged literals.

## Issues Found

None.

## Workload / PR Boundary

- Mode: single PR (per tasks.md's single suggested work unit)
- Current work unit: Unit 1 — "Store carries TopicKey; predicate rewritten; fixture sweep scoped by call path; README updated"
- Boundary: this apply batch is the complete work unit, start to finish
- Authored line count: 292 (201 insertions + 91 deletions) via `git diff --stat main...HEAD -- longterm-mem`, under the 400-line budget
- Commits: `08f3e81` (implementation) and `98a7689` (tasks.md checkboxes, SDD process artifact)

## Status

20/20 tasks complete. Ready for verify.

## Remediation batch

**Status**: 5/5 remediation items complete — Ready for verify

Independent verification of the original apply found one live defect and
four documentation/coverage gaps in the delta. This batch (a fresh SDD
attempt, ordinal 3, same objective lineage) fixes all five.

### Completed Tasks

- [x] R.1 RED/GREEN: `internal/promote/eligible.go` — `curatedTopicKey`
  treated `strings.Cut("/sdd/auth", "/")`'s empty head (`""`) as absent
  from `excludedTopicPrefixes`, so a leading-slash `topic_key` (`/sdd/auth`,
  `/`, `//sdd`) bypassed the exclusion and was reported eligible. Added
  `if head == "" { return false }`; case-sensitivity and `TrimSpace` kept
  as-is.
- [x] R.2 `internal/promote/eligible_test.go` — added table cases: bare
  `sdd` (excluded), bare `foo` (eligible), whitespace-only (excluded),
  `/sdd/auth`, `/sdd/x`, and `/` (all excluded post-fix), `SDD/x`
  (eligible — documents case-sensitivity).
- [x] R.3 `internal/promote/plan_test.go` —
  `TestPlan_WritesNothingAndPredictsWhatSyncThenDoes` gained a fourth
  `fixtureObs` row (`topicKey: "sdd/x"`), plus explicit assertions that it
  contributes to `plan.Skipped` (now 2), never appears in `plan.Titles`,
  and no promoted page's body contains its title — integration proof the
  R-007 gate runs inside both `Plan` and `Sync`, not just the unit
  predicate.
- [x] R.4 Refreshed three stale comments left over from the retired
  type/revision-count predicate: `eligible.go`'s `excludedTopicPrefixes`
  doc (no longer references a deleted `eligibleTypes` map),
  `eligible_test.go`'s `TestPromote_ExplicitCallOverridesAutomaticEligibility`
  doc (now cites the untopiced-observation case instead of a
  type/revision threshold), `writer_test.go`'s
  `TestWriter_Promote_IneligibleDoesNotRegister` fixture (comment makes
  explicit that the zero-value `TopicKey` is what makes it ineligible).
- [x] R.5 `longterm-mem/README.md` — `sync` row states that a page
  promoted under a previous, looser eligibility rule is neither retracted
  nor refreshed by `sync` once its observation stops being eligible; use
  an explicit `promote` or a pin to keep it current. A `doctor` check for
  this is out of scope, noted as a follow-up.

Delta spec (`specs/longterm-mem-promotion/spec.md`) updated: R-007's
requirement text now states the first segment must be non-empty AND not
one of the excluded prefixes, plus a new "leading-slash topic_key" scenario
covering `/sdd/auth` and `/`.

### TDD Cycle Evidence

| Task | Test File | RED | GREEN | REFACTOR |
|------|-----------|-----|-------|----------|
| R.1/R.2 | `eligible_test.go` | ✅ 3 new cases failed against pre-fix `curatedTopicKey` (`/sdd/auth`, `/sdd/x`, `/` all reported eligible=true, want false) | ✅ all 19 `TestEligible` subtests pass after the `head == ""` guard | ➖ none needed, single-line guard |
| R.3 | `plan_test.go` | N/A — integration assertion added alongside the fix, not a standalone RED (the underlying bug is already covered by R.1/R.2's RED) | ✅ `TestPlan_WritesNothingAndPredictsWhatSyncThenDoes` passes: `plan.Skipped == 2`, `sdd/x` absent from `Titles` and `Promoted` | ➖ none |

### Work Unit Evidence

| Evidence | Value |
|---|---|
| Focused test command and exact result | `cd longterm-mem && go test ./internal/promote/... ./internal/engram/...` → `ok .../promote`, `ok .../engram` |
| Runtime harness command/scenario and exact result | `cd longterm-mem && go vet ./... && go test -count=1 -race ./...` → all 17 packages `ok`, race detector clean |
| Rollback boundary | Two commits, each independently revertible: `b51bf70` (code fix + tests + README) touches only `longterm-mem/`; `8e1fede` (spec + tasks) touches only `openspec/changes/longterm-mem-promotion-scoping/` |

### Files Changed (remediation batch)

| File | Action | What Was Done |
|------|--------|---------------|
| `longterm-mem/internal/promote/eligible.go` | Modified | `curatedTopicKey` rejects an empty first path segment; refreshed `excludedTopicPrefixes` doc comment |
| `longterm-mem/internal/promote/eligible_test.go` | Modified | 8 new table cases (bare keys, whitespace, leading-slash, case-sensitivity); refreshed stale doc comment |
| `longterm-mem/internal/promote/plan_test.go` | Modified | Fourth `sdd/x` fixture row + integration assertions on `Skipped`/`Titles`/`Promoted` |
| `longterm-mem/internal/promote/writer_test.go` | Modified | Comment clarifying the "Not Eligible" fixture's untopiced zero-value |
| `longterm-mem/README.md` | Modified | `sync` row documents stale-page retention behavior |
| `openspec/changes/longterm-mem-promotion-scoping/specs/longterm-mem-promotion/spec.md` | Modified | R-007 requirement text + new leading-slash scenario |
| `openspec/changes/longterm-mem-promotion-scoping/tasks.md` | Modified | Added "Remediation" checklist, 5/5 items marked done |

### Verification (this batch)

- `cd longterm-mem && go test ./internal/promote/... ./internal/engram/...` → GREEN
- `cd longterm-mem && go vet ./... && go test -count=1 -race ./...` → GREEN, all 17 packages
- `cd longterm-mem && gofmt -l .` → empty (clean)
- `git diff --stat` for the uncommitted remediation changes (before commit): 7 files, 94 insertions(+), 16 deletions(-)

### Commits

- `b51bf70` — `fix(longterm-mem): reject empty first segment in curated topic keys`
- `8e1fede` — `docs(sdd): record longterm-mem-promotion-scoping remediation`

### Status

5/5 remediation items complete. Ready for verify.
