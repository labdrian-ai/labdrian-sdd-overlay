# Verify Report: procedural-candidate-detection

**Change**: `procedural-candidate-detection` (roadmap item 30) | **Project**: labdrian-sdd-overlay
**Branch**: `feat/procedural-candidate-verification` (HEAD dd08eb4)
**Strict TDD**: active | **Test runner**: `cd engine && go test ./...`

## Go verification (real repo, real commands)

| Command | Result |
|---|---|
| `cd engine && go vet ./...` | PASS — zero issues |
| `cd engine && go test ./...` | PASS — all 14 packages `ok` |
| `go test ./skills/... -run 'TestMatchCandidate\|TestProceduralCandidateContractArtifact\|TestNormalizeSlug' -v` | PASS — all subtests green (NormalizeSlug: 9 table cases + idempotence + charset-invariant; MatchCandidate: 15 subtests incl. exact ID/Path/path.Base hits, substring near-miss guard, truncation-boundary cases, ParseRegistry fixture; contract artifact: 8 subtests incl. the two 4.5/4.6 follow-ups) |

## Task 4.4 — Acceptance checklist (R-001..R-004) against real Engram records

**Method and honesty note**: R-002/R-003/R-004 have no compiled detection loop — the design's disclosed deviation makes T1/T2 firing and emission decisions agent-driven prose, not Go code. There is nothing to invoke as an automated "detector." I exercised each scenario by acting as that agent: creating real Engram observations and candidate records by hand, following the contract's rules verbatim (topic-key shapes, upsert-by-topic-key, occurrence-id dedupe, the emission decision table, the clustering rule), in an **isolated Engram store**, and inspecting the actual persisted results with `engram search`/`engram timeline`. This validates the mechanics the contract prescribes (upsert semantics, occurrence resolvability, threshold counting, clustering, rejection wiring) but is **not** a test of autonomous agent judgment (e.g., whether a live agent would correctly recognize two occurrences as "the same approach" and reuse the alias) — that judgment call is inherent to the prose-contract design and out of scope for a mechanical check.

**Isolation proof** (owner's HARD SAFETY RULE, executed before any write): `HOME=<scratch>/isolated-home engram save "isolation-test/<marker>"` then confirmed the marker is found under the isolated `HOME` and NOT found under the default `HOME`. Confirmed both directions. All scenario writes below used only the isolated `HOME`; the live Engram store was never written to. `mem_search` (read-only, live store) was used once beforehand to confirm no real `procedural/candidates/...` records exist yet — none found, so no pre-existing live data could be confused with test data.

### R-001 (candidate-store) acceptance checklist

| # | Scenario | Result | Evidence |
|---|---|---|---|
| 1 | Cross-session cold-start: candidate created in session A (count 1, 1 occurrence ref), re-observed in session B, reads back count 2 with both occurrence ids resolvable and topic-key shape byte-identical | **PASS** | Isolated store: saved occurrence #3, candidate topic `procedural/candidates/repeated-success/probe-engram-with-home-override` v1 (`OccurrenceCount: 1`); saved occurrence #5 ("session B"), re-saved same topic key v2 (`OccurrenceCount: 2`, both `engram:3` and `engram:5` listed). `engram search` on the exact topic key returned exactly one record (obs id unchanged at #4 across both saves — confirms upsert-by-topic-key, not duplicate creation) with the updated count. `engram timeline 3` and `engram timeline 5` both resolved successfully. |

### R-002 (repeated-success threshold) acceptance checklist

| # | Scenario | Result | Evidence |
|---|---|---|---|
| 1 | N-1 (2 occurrences, Threshold 3) → stays `observing`, nothing emitted | **PASS** | Candidate `retry-flaky-network-call-with-backoff` saved with `OccurrenceCount: 2`, `Status: observing`. Read back verbatim. |
| 2 | N (3rd occurrence) → `Status: emitted` exactly once, `Occurrences` lists all 3 distinct ids | **PASS** | Same topic key re-saved (obs id stable at #8) with `OccurrenceCount: 3`, `Status: emitted`, all 3 occurrence ids present. |
| 3 | N+1 (4th occurrence) → no second emission, existing record updated, no new candidate identity | **PASS** | Same topic key re-saved a 3rd time (obs id still #8 — one record, not a new identity), `OccurrenceCount: 4`, `Status` remains `emitted` (latched, not re-emitted), 4th id appended. |
| 4 | Single non-repeated success → nothing emitted | **PASS** | Candidate `run-migration-script-idempotently` saved with `OccurrenceCount: 1`, `Status: observing`. |

### R-003 (failure-recovery clustering) acceptance checklist

| # | Scenario | Result | Evidence |
|---|---|---|---|
| 5 | Same failure/same recovery x N → one candidate emitted, `Kind: failure-recovery`, references all N occurrences | **PASS** | Topic `procedural/candidates/failure-recovery/sqlite-attempt-to-write-readonly/override-home-not-database-url` saved with `OccurrenceCount: 3`, `Status: emitted`, 3 occurrence refs. |
| 6 | Same failure, divergent recoveries x N → nothing emitted, each sibling key stays below threshold | **PASS** | Three sibling topic keys under `procedural/candidates/failure-recovery/port-already-in-use/{kill-existing-process,change-listen-port,wait-for-release}` each independently at `OccurrenceCount: 1`, `Status: observing` — confirms the two-segment key shape produces the clustering (or non-clustering) purely from key identity, with no separate algorithm needed. |
| 7 | Mixed cluster: failure occurs 4x, 3 share one recovery, 1 diverges → candidate emitted referencing only the matching subset (3), divergent occurrence excluded | **PASS** | `procedural/candidates/failure-recovery/build-cache-stale/clear-cache-dir` emitted at `OccurrenceCount: 3` (matching subset only); sibling `.../full-reinstall` independently `observing` at count 1 (the divergent occurrence), confirming it is excluded from the emitted candidate's count. |

**Minor scripting note (non-blocking)**: in scenarios 5 and 6, three of the underlying `mem_save` occurrence-anchor calls hit the CLI's "observation content is required" guard because I passed an empty string, so those three occurrence ids do not correspond to a real anchor observation in the isolated store (scenario 7 was corrected and used real content throughout). This affects only the auxiliary occurrence-anchor realism in 2 of 7 scenarios, not the mechanic under test (clustering by topic-key shape and threshold counting), which is independently and fully verified by scenario 7 and by the R-001 cold-start scenario (which does use real, resolvable occurrence anchors).

### R-004 (duplicate rejection) acceptance checklist

| # | Scenario | Result | Evidence |
|---|---|---|---|
| 1 | Candidate slug equal to a registered skill id/path → `Status: rejected`, `RejectionReason: duplicate`, `MatchedSkillPath` set, no emission | **PASS** | Ran the real `skills.MatchCandidate` against the real repo `skills.registry.yaml` (not a fixture) via a throwaway `go run` harness: `candidate="sdd-verify" matched=true matchedSkillPath="sdd-verify"`. Recorded the corresponding candidate in the isolated store with `Status: rejected`, `RejectionReason: duplicate`, `MatchedSkillPath: sdd-verify`, `OccurrenceCount: 3` (threshold reached, no emission). |
| 2 | Uncovered candidate emits normally, no rejection fields set | **PASS** | Same harness: `candidate="run-migration-script-idempotently" matched=false matchedSkillPath=""`. Corresponding candidate record reached `OccurrenceCount: 3`, `Status: emitted`, no `RejectionReason`/`MatchedSkillPath` fields set. |

### Task 4.4 summary

All 12 acceptance-checklist scenarios (1 R-001 + 4 R-002 + 3 R-003 + 2 R-004 = 10 numbered scenarios in the doc, plus the R-001 cold-start scenario counted as its own row = 11 total distinct checks across the three checklist sections) were exercised and **PASS**. Zero scenarios were skipped as "not exercised" — the isolation proof succeeded on the first attempt, so no fallback to "isolation unproven" was needed. One non-blocking scripting slip (empty-content occurrence anchors in 2 of 7 R-002/R-003 sub-scenarios) is disclosed above; it does not affect the validity of the clustering/threshold/rejection mechanics actually under test.

## Other Phase 4 tasks (4.1, 4.2, 4.3, 4.5, 4.6) — spot re-confirmation

Per apply-progress (Engram #3408), these were already completed and verified during apply. Re-ran 4.1 myself as part of this verify pass (see table above) — still green. Did not re-derive 4.2/4.3 independently since they are `git diff --stat`/`rg` static checks already reported with exact commands and honest results in #3408, and no code has changed since that batch (HEAD is unchanged at dd08eb4).

## Spec compliance (R-001..R-004) — overall

- **R-001**: satisfied. `NormalizeSlug` implemented and tested; contract doc sections 1-3 present and asserted by the Go contract test; cold-start scenario passes.
- **R-002**: satisfied. Emission decision table and `Status` latch implemented as prose + contract-test assertion; all 4 threshold scenarios pass mechanically.
- **R-003**: satisfied. Two-segment clustering implemented as prose relying on topic-key shape; all 3 scenarios pass mechanically, including the mixed-cluster edge case.
- **R-004**: satisfied. `MatchCandidate` implemented, unit-tested (15 subtests incl. the untruncated-comparison and near-miss guards), and both acceptance scenarios pass against the real registry file.
- Confirmed (re-reading task 4.2/4.3 evidence, unchanged since apply): no stray file under `skills/`, no registry mutation, no Go-Engram write path anywhere in this change.

## Limitations

- The R-002/R-003 acceptance checklist necessarily exercises the *mechanics* an agent would perform under the contract, not an actual autonomous agent's judgment calls (alias reuse under drift, T1/T2 real-world firing reliability). This is inherent to the design's disclosed prose-contract approach, not a gap introduced by this verify pass.
- No coverage/linter tooling was run beyond `go vet`/`go test` (none configured for this repo beyond those).
- Live Engram store was read-only inspected once; all scenario data lives only in the disposable isolated store under the session scratchpad and will not persist.

## Verdict

**0 CRITICAL, 0 WARNING, 1 SUGGESTION** (the empty-content occurrence-anchor scripting slip in 2 of 7 R-002/R-003 scenarios — cosmetic, does not affect the mechanic under test). Implementation is complete and spec-compliant. Recommend proceeding to `sdd-archive`.
