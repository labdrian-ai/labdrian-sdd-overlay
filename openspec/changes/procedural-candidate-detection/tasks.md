# Tasks: Procedural Promotion-Candidate Detection and Storage

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 630-940 total across 3 slices (design forecast: slice 1 ≈250, slice 2 ≈180, slice 3 ≈200; task-level estimate below refines these) |
| 400-line budget risk | Low per slice — each slice is well under 400; no slice in this breakdown is expected to approach the budget |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 candidate-store → PR 2 repeat-and-recovery-detection → PR 3 duplicate-rejection |
| Delivery strategy | auto-chain / ask-on-risk |
| Chain strategy | stacked-to-main |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: Low (flag only if slice 2's contract-doc additions plus acceptance checklist prose grow materially beyond the design's ≈180-line estimate)

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 candidate-store | `NormalizeSlug` + record/topic-key contract sections 1-3 | PR 1 | `cd engine && go test ./skills/... ./cmd/...` | Go table tests, no fixtures | delete `engine/skills/match.go`, `match_test.go`; delete `skills/_shared/procedural-candidate-detection.md` |
| 2 repeat-and-recovery-detection | Emission decision table + T1/T2 trigger contract, sections 4-5 | PR 2 | `cd engine && go test ./skills/... ./cmd/...` (contract-artifact test only; no new Go logic) | Contract-artifact assertion test; acceptance checklist is executed manually during `sdd-verify`, not by `go test` | revert sections 4-5 of the contract doc |
| 3 duplicate-rejection | `MatchCandidate` + rejection record fields, section 6 | PR 3 | `cd engine && go test ./skills/... ./cmd/...` | Go table tests over `ParseRegistry`-built and literal `Registry` fixtures | revert `match.go`/`match_test.go` hunks; revert section 6 of the contract doc |

**Note on TDD applicability**: R-001 and R-004 are pure Go and follow strict RED→GREEN→REFACTOR below. R-002 and R-003 are agent-driven prose (`skills/_shared/procedural-candidate-detection.md`), not Go code — per the design's disclosed deviation from the proposal's literal "unit tests over the threshold boundary" wording, their equivalent gate is the fixture-driven acceptance checklist executed during `sdd-verify`, not a `go test` row. Contract-artifact content (verbatim table/field-block presence) is still Go-testable and follows RED→GREEN like any other file.

## Phase 1: candidate-store (PR 1, R-001)

- [x] 1.1 RED `engine/skills/match_test.go`: `NormalizeSlug` table — mixed case, spaces, underscores, punctuation runs, leading/trailing separators, unicode input, empty string, all-punctuation input, 48-byte truncation at a `-` boundary, idempotence (`NormalizeSlug(NormalizeSlug(x)) == NormalizeSlug(x)`)
- [x] 1.2 GREEN `engine/skills/match.go`: `NormalizeSlug(s string) string` — lowercase ASCII, collapse non-`[a-z0-9]` runs to `-`, trim leading/trailing `-`, truncate to 48 bytes at the last `-` boundary at or before 48
- [x] 1.3 REFACTOR: confirm `NormalizeSlug` has no filesystem/network access and is exported as the single normalization definition cited by the contract document
- [x] 1.4 RED `engine/skills/procedural_candidate_contract_test.go` (new, following the `oo_quality_contract_artifact_test.go` precedent): assert `skills/_shared/procedural-candidate-detection.md` exists and contains, verbatim, both topic-key shapes (`procedural/candidates/repeated-success/{approach-slug}` and `procedural/candidates/failure-recovery/{failure-slug}/{recovery-slug}`) and the full record field block (`Kind`, `Candidate`, `Aliases`, `Status`, `RejectionReason`, `MatchedSkillPath`, `Threshold`, `OccurrenceCount`, `FirstObserved`, `LastObserved`, `Occurrences`, `Summary`)
- [x] 1.5 GREEN `skills/_shared/procedural-candidate-detection.md` — sections 1-3 (R-001): record schema and field block, both topic-key shapes with `{kind}` as identity, slug grammar table (`{approach-slug}`, `{failure-slug}`, `{recovery-slug}` with examples), the `Aliases` drift-prevention rule (namespace search before minting a new slug), the `Threshold` field note (declared per-record, default 3), and the occurrence-identity/dedupe rules (occurrence id = Engram observation id, appending an already-listed id is a no-op, `OccurrenceCount` is always derived from distinct ids in `Occurrences`)
- [x] 1.6 Acceptance checklist addition (R-001, non-Go, executed during `sdd-verify`): add the cross-session cold-start scenario to the contract doc — candidate created in session A with count 1 and one evidence reference, same candidate observed in session B reads count 2 with both `engram:<id>` references resolvable via `mem_get_observation`, topic-key shape asserted identical across both sessions
- [x] 1.7 Verify: `cd engine && go test ./skills/... ./cmd/...`

## Phase 2: repeat-and-recovery-detection (PR 2, R-002, R-003)

- [x] 2.1 RED `engine/skills/procedural_candidate_contract_test.go`: extend to assert the emission decision table (all 7 rows: no-record→observing; observing+listed-id→observing; observing+new-id+count<N→observing; observing+new-id+count=N+no-match→emitted; observing+new-id+count=N+match→rejected; emitted+count>N→emitted; rejected+count>N→rejected) and `Threshold: 3` appear verbatim in `skills/_shared/procedural-candidate-detection.md`
- [x] 2.2 GREEN `skills/_shared/procedural-candidate-detection.md` — sections 4-5 (R-002, R-003): the emission decision table verbatim; the `Status` latch rule (`observing → emitted` fires once, on the single update where `OccurrenceCount` first reaches `Threshold`; later occurrences still append but never re-emit); the failure-recovery two-segment clustering explanation (same failure + same recovery → one key; same failure + divergent recovery → sibling keys, each count 1); the T1 trigger contract (fires immediately after any proactive `mem_save` of type `bugfix`/`pattern`/`discovery`/`decision`; that save is the occurrence anchor) and T2 trigger contract (sweep during the mandatory session-close protocol, beside `mem_session_summary`)
- [x] 2.3 Non-Go note (explicit, for `sdd-verify`): record in the contract doc, and confirm in this task list, that Go-sense TDD does not apply to R-002/R-003 — this is agent-driven prose, not compiled logic; the acceptance checklist below is the equivalent gate, per the design's disclosed deviation from the proposal's literal unit-test wording
- [x] 2.4 Acceptance checklist addition (R-002, non-Go, executed during `sdd-verify`): numbered scenarios for N-1 (no emission), N (exactly one emission referencing all N occurrence ids), N+1 (no second emission, existing record may be updated, no new candidate identity), and a single non-repeated success (nothing emitted)
- [x] 2.5 Acceptance checklist addition (R-003, non-Go, executed during `sdd-verify`): numbered scenarios for same-failure/same-recovery repeated N times (one candidate, `failure-recovery` label, references all N occurrences), same-failure/divergent-recovery N times (nothing emitted), and the mixed-cluster case (more than N occurrences, exactly N share a recovery — candidate emitted referencing only the matching subset; divergent occurrences excluded from the count)
- [x] 2.6 Verify: `cd engine && go test ./skills/... ./cmd/...` (contract-artifact assertions only — no new Go logic in this slice)

## Phase 3: duplicate-rejection (PR 3, R-004)

- [x] 3.1 RED `engine/skills/match_test.go`: `MatchCandidate` table — exact `Entry.ID` hit; exact `Entry.Path` hit; `path.Base(Entry.Path)` hit; case/underscore/space variants of a real id normalizing to a hit; no match on an unrelated candidate; **no match on a substring near-miss (`sdd-spec-review` must NOT match `sdd-spec`)**; empty candidate string; all-punctuation candidate string; empty registry; deterministic first-match-wins when two entries in registry order both match — built via both `ParseRegistry` over a fixture YAML and a literal `Registry` value
- [x] 3.2 GREEN `engine/skills/match.go`: `MatchCandidate(reg Registry, candidate string) (matched bool, skillPath string)` — compares `NormalizeSlug(candidate)` against `NormalizeSlug(entry.ID)`, `NormalizeSlug(entry.Path)`, `NormalizeSlug(path.Base(entry.Path))` for each entry in registry order, first match wins, no substring/prefix/fuzzy matching, empty/all-punctuation candidate returns `(false, "")`
- [x] 3.3 REFACTOR: confirm `MatchCandidate` performs no filesystem or Engram access, reuses `ParseRegistry` rather than re-parsing YAML, and document (as a code comment, not implemented) the `MatchCandidateBy(reg, candidate, fields ...MatchField)` extension point for a future trigger/keyword schema field
- [x] 3.4 RED `engine/skills/procedural_candidate_contract_test.go`: extend to assert section 6 (rejection record fields: `RejectionReason`, `MatchedSkillPath`, present-only-when-rejected notes) and the `MatchCandidate` call-site description appear verbatim in the contract doc
- [x] 3.5 GREEN `skills/_shared/procedural-candidate-detection.md` — section 6 (R-004): rejection record field rules, the `MatchCandidate(registry, slug)` call site at the emission boundary only (never per-occurrence), and the audit note that a rejected record is kept rather than discarded
- [x] 3.6 Acceptance checklist addition (R-004, non-Go, executed during `sdd-verify`): a candidate slug equal to a registered skill id/path yields `Status: rejected`, `RejectionReason: duplicate`, `MatchedSkillPath` set, no emission; an uncovered candidate emits normally with no rejection fields set
- [x] 3.7 Verify: `cd engine && go vet ./... && go test ./...`

## Phase 4: Full Verification

- [ ] 4.1 `cd engine && go vet ./... && go test ./...`
- [ ] 4.2 Confirm no file under `skills/` other than `skills/_shared/procedural-candidate-detection.md` was written, and no `skills.registry.yaml`/manifest mutation occurred (`git diff --stat` review)
- [ ] 4.3 Confirm no Go source path in this change opens Engram's database in a writable mode (grep for new `sql.Open`/DSN construction touching Engram; none expected — this change adds no Go-Engram code path at all)
- [ ] 4.4 Run the full R-001/R-002/R-003/R-004 acceptance checklist (sections added in 1.6, 2.4, 2.5, 3.6) against real Engram records during `sdd-verify`; record results honestly in the verify report, including any scenario that could not be exercised
