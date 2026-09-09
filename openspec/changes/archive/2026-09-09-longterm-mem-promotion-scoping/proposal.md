# Proposal: Curated topic_key gates automatic vault promotion

## Intent

`Eligible()` promotes any observation of type `decision|architecture|pattern` or with `revision_count >= 3`. Evidence (obs #3283) over 282 currently auto-eligible rows: 182 `sdd/`, 58 untopiced, 4 `delivery/`, 1 `review/` — SDD phase artifacts, Judgment Day verdicts, PR splits and RDD consent records, not canonical knowledge. Confirmed decision (obs #3287, "Solo topics curados"): automatic eligibility becomes explicit promote OR pinned OR a non-empty `topic_key` whose first segment is not `sdd`, `review`, or `delivery`. The rule is unenforceable today because `store.go` never reads the existing `topic_key` column.

## Scope

### In Scope
- R-001: exclude observations with empty/absent `topic_key` from automatic eligibility.
- R-002: exclude first-segment prefixes `sdd`, `review`, `delivery`.
- R-003: retire `type` and `revision_count` as automatic criteria; read `topic_key` in the Engram store adapter so `Eligible` receives it.
- R-004/R-005: explicit promote and pinned override every exclusion.
- Strict TDD unit coverage plus a `sync --dry-run` transcript as acceptance evidence.

### Out of Scope
- R-006..R-008 sync triggers (archive, session close, failure isolation) — change `longterm-mem-sync-triggers`.
- Any `capture_prompt`-based rule: not a persisted column (obs #3283), permanently retired.
- Core sync/promote mechanics, retroactive re-tagging, untopiced-review reporting.

## Capabilities

### New Capabilities
- None

### Modified Capabilities
- `longterm-mem-promotion`: Requirement `Promotion Eligibility Predicate` (R-007) replaces type/revision criteria with the curated-topic_key rule and its explicit/pinned overrides.
- `longterm-mem-memory-access`: observations loaded from the read-only Engram connection must carry `topic_key`.

## Approach

Add `TopicKey` to `engram.Observation`, `observationColumns` and `scanObservationRow`. Rewrite `Eligible` as: `explicit || obs.Pinned || curatedTopicKey(obs.TopicKey)`, where `curatedTopicKey` rejects empty keys and the three excluded first segments (split on `/`). Delete `eligibleTypes` and `minEligibleRevisionCount`. Callers (`writer.go`, `explicit.go`, `reconcile.go`) keep their signature. Tests first per the entry contract.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `longterm-mem/internal/promote/eligible.go` | Modified | New predicate; type/revision criteria removed |
| `longterm-mem/internal/promote/eligible_test.go` | Modified | Exclusion, prefix, and override cases |
| `longterm-mem/internal/engram/store.go` | Modified | Read `topic_key` into `Observation` |
| `longterm-mem/README.md` | Modified | R-007 eligibility description |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Genuine untopiced decisions stop promoting | High (accepted) | Documented tradeoff; pin or explicit promote |
| Already-promoted noise pages remain in the vault | Med | Out of scope; report in dry-run diff, handle separately |
| Store column mapping drift breaks scans | Low | Store-level test asserting `topic_key` is populated |

## Rollback Plan

Single PR, one review slice. Revert the commit: `Eligible` and `store.go` return to type/revision behavior; no schema, vault, or persisted-state migration is involved.

## Dependencies

- Confirmed decision obs #3287; evidence obs #3283. None external.

## Success Criteria

- [ ] `Eligible` returns false for untopiced and `sdd`/`review`/`delivery`-prefixed observations, true for explicit and pinned.
- [ ] Loaded `Observation` structs carry `topic_key`.
- [ ] `cd longterm-mem && go vet ./... && go test ./...` passes.
- [ ] A post-change `sync --dry-run` transcript shows the promoted set free of the 182+58+4+1 evidence rows unless pinned or explicit.
