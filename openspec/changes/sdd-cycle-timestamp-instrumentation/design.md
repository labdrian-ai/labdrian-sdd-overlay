# Design: SDD Cycle Timestamp Instrumentation

**Change**: `sdd-cycle-timestamp-instrumentation` | **Store**: hybrid | **Base**: main @ `ef35927`

## Gating Question: Is Engram `Created` programmatically retrievable?

**YES — with evidence.** `mem_get_observation` returns JSON `{project, project_path, project_source, result}` where `result` is text ending in a stable labeled footer: `Session: … / Project: … / Scope: … / Topic: … / Duplicates: N / Revisions: N / Created: YYYY-MM-DD HH:MM:SS`. Observed verbatim in this project's transcripts (session `3752673c`, 2026-08-05): topic `sdd/deterministic-verification-evidence/landed-on-main` footer reads `Created: 2026-08-05 13:12:21`. `Created` is **stable across topic-key upserts**: observations with `Revisions: 2` show `Created: 2026-07-29 18:24:17 / 18:28:12 / 19:37:24` — byte-identical to the first-write times independently recorded in `openspec/changes/archive/2026-07-30-actuals-calendar-checkpoint-instrumentation/archive-report.md:19-24`. Caveats: it is a labeled footer line inside a text envelope, not a typed JSON field (parse rule: the footer's `Created: ` line); the format carries **no timezone** (host-local). Fallback if a future engram version drops the label: treat t0 as missing (D5) — never estimate. **This footer is the gating evidence only** — proof that `Created` exists and is stable. D1 below evaluates two extraction methods against it and chooses the typed field, never footer parsing, for the shipped mechanism.

## Technical Approach

closure-feedback (existing single writer) harvests t0 from the **typed `created_at` field** of `sdd/{change}/pipeline-state` (D1 — never the rendered `Created` footer) and t1 from the versioned `landing_commit`/`approved_tree` anchor recorded at delivery (D2 revision 3 — never a folder scan), computes `total_wall_clock_hours = t1 − t0`, and writes both anchors where readers already look. No new actor, no new schema property, no hooks.

## Architecture Decisions

### D1 — t0 source (TYPED — supersedes footer parsing)
**Choice**: read the **typed `created_at` field** from `engram export`, never a display footer.

```
engram export <tmp>.json
jq -r '.observations[] | select(.topic_key=="sdd/{change}/pipeline-state") | .created_at' <tmp>.json
```

The export envelope is `{exported_at, observations, prompts, sessions, version}` and each observation carries typed `id`, `topic_key`, `created_at`, `updated_at`, `last_seen_at`, `sync_id`, `type`, `scope`, `project`. Verified against the real record: `sdd/skills-validate-ondisk-gate/pipeline-state` → `created_at: "2026-08-05 14:12:55"`, byte-identical to the value the footer route produced (obs #2772) and to `openspec/changes/archive/2026-07-30-actuals-calendar-checkpoint-instrumentation/archive-report.md:19-24`. Same values, but as a field rather than parsed prose.

**Rejected**: parsing the `Created:` footer of `mem_get_observation` — it is display text inside a text envelope, so a future engram release that renames or reflows the label silently breaks t0. A typed field cannot be broken by presentation changes. Also rejected: the `mem_search` hit's unlabeled timestamp line (semantics unproven); hooks (not portable).

**Requires `topic_key` to be set.** `mem_save` does NOT accept `topic_key` nested inside `metadata` — passing it there is silently dropped and the observation exports with a null topic. It must be supplied as a first-class argument, or corrected afterwards with `mem_update(id, topic_key=…)`. Four artifacts of this very change were written without their topic keys for exactly this reason and had to be repaired. Tasks MUST include a guard for this, since a `pipeline-state` with no `topic_key` is invisible to the D1 selector and degrades t0 to "missing".

### D1b — t0 fallback when the change never ran inception-pipeline
`sdd/{change}/pipeline-state` only exists for changes routed through inception-pipeline. A change entered directly at SDD — like this one — has none, so the strict rule would degrade every such cycle to "no t0" and keep the sample stalled.

**Choice**: WHEN no `pipeline-state` observation exists, t0 falls back to the **earliest `created_at` among the change's own `sdd/{change}/*` observations** (exploration, else proposal). That is the first durable evidence the cycle had begun, it is typed and already written by the normal flow, and it is a lower bound that never invents time the cycle did not take. The fallback MUST be disclosed by name in `variance_vs_plan` so a reader never mistakes it for a tiering-go-ahead anchor.
**Rejected**: fabricating a `pipeline-state` observation for changes that skipped inception-pipeline — that would forge an artifact of a phase which never ran, and its `created_at` would be the write time rather than the decision time. Also rejected: omitting t0 entirely (the degraded path becomes the common path, defeating the change's purpose).

### D2 (REVISION 3 — supersedes revision 2 below, which verify proved unexecutable)

Verify blocked revision 2 with two findings, both reproduced independently:

- **The receipt store is not portable or discoverable.** Receipts live under `.git/gentle-ai/review-transactions/v2/<lineage>/`, and `.git/` is never versioned — a fresh clone has zero receipts. The store holds 39 lineages keyed by opaque ids with **no binding to a change name**, so "find this change's receipt" has no rule. The mandated primary path could not run for anyone but the machine that performed the review.
- **The folder-scan fallback selects the wrong commit.** For `deterministic-verification-evidence` it returns `79995ea` — the merge of PR #130, a *different* change that incidentally touched the same folder — while the true landing is `be2c3ca` (PR #129), recorded in that change's own archive-report as "merged as `be2c3ca`". t1 drifts +7h40m, exactly the drift R-017 forbids. The retro test passed only because it was pinned to two constants the algorithm happened to fit; swept over all 19 archived changes it fails, and returns nil for 9 of them.

**What survives**: the content binding itself is sound and was verified across the receipt store — for every receipt sampled, `final_candidate_tree` equals the tree of a real commit on `main` (8/8 in the sweep, e.g. `aaae3588` → `41bf6f82`, the merge of PR #138). Git trees ship with the repository even though receipt files do not. Only the *transport* was wrong.

**Revision 3 choice — record the anchor in a versioned artifact at delivery, not in `.git/`.**

At delivery, the change's own archive-report records two values, both already available from the approved receipt:

| Field | Value | Purpose |
|---|---|---|
| `landing_commit` | the merge/landing commit SHA on the default branch | the anchor — always recorded |
| `approved_tree` | that receipt's `final_candidate_tree` | makes the SHA **verifiable**, not merely asserted — recorded ONLY when a review receipt exists for the candidate |

**Correction (Phase 13): `approved_tree` MUST NOT be synthesized from `landing_commit`'s own tree when no review ran.** The original revision 3 text populated `approved_tree` from the receipt's `final_candidate_tree` when review ran, and derived it from the landing commit's own tree by default when it did not — that default-derivation branch made the mandated check compare a value against itself, so it could never fail. A check that is structurally incapable of failing is not verification, and reporting its pass as "verified" is a fabricated assurance — the same defect class this instrument exists to eliminate from the numbers it measures. `approved_tree` is now recorded only when an independent receipt exists.

t1 = the committer timestamp of `landing_commit`, and it resolves whether or not `approved_tree` is recorded. WHEN `approved_tree` IS recorded, a reader verifies the record rather than trusting it: `git show -s --format=%T <landing_commit>` MUST equal `approved_tree`; a match makes the outcome **verified**, a mismatch makes it **rejected** and t1 omitted. WHEN `approved_tree` is absent because no review ran, the outcome is **self-asserted**: t1 still resolves from `landing_commit` alone — the measurement is not withheld — but `variance_vs_plan` MUST record the outcome as self-asserted, not verified, so a reader never mistakes a self-asserted anchor for an independently checked one. This is portable (both values are plain text in a versioned file), exact (content-bound), and self-checking where an independent receipt exists — a mis-recorded SHA is then detectable, which a bare SHA never is.

Precedent already exists: `openspec/changes/archive/2026-08-05-deterministic-verification-evidence/archive-report.md` records "merged as `be2c3ca`" in versioned prose today. This formalizes what the repository already does by habit and adds the verifying hash.

**Disambiguation rule.** Resolving by tree alone is ambiguous: `main` has 434 commits sharing only 84 distinct duplicated tree values, so **205 of 434 commits (~47%) sit in a colliding tree** — because merges that change no content reproduce their parent's tree. A tree hash MUST be used to **verify** a commit, never to **discover** one: WHEN a tree hash is all that is available and more than one commit on the default branch carries it, the anchor is ambiguous — t1 is omitted and the ambiguity disclosed, never resolved by position. "Earliest carrier" is a guess wearing the costume of a rule: this repository contains a real three-way collision (tree `2ad8e42e…`, carried by `be2c3ca` — the true landing of `deterministic-verification-evidence` — `aa81361`, and `459b48d`), where earliest-by-time selects `459b48d`, a commit belonging to a different change. The recorded `landing_commit` avoids this search entirely; the rule exists only for verification and for reconstructing historical records whose archive-report already names a landing SHA in versioned prose.

**No heuristic fallback.** Revision 2's path scan is removed outright rather than demoted. It selects a wrong commit on a real case and returns nothing for 47% of archived changes, and a confidently wrong duration is worse for calibration than an absent one. WHEN no `landing_commit` is recorded, t1 is omitted and `variance_vs_plan` states that the change predates the anchor convention.

**Historical records** are resolved from the merge commits their own archive-reports already name in versioned prose (e.g. `be2c3ca`), verified against the tree rule above — documented evidence, never a heuristic re-derivation.

---

### D2 (revision 2 — SUPERSEDED, retained for provenance)

**Choice**: anchor t1 to the delivery Gentle AI itself authorized, not to a guess about which commit "looks like" the landing.

Every reviewed delivery mints a receipt under `.git/gentle-ai/review-transactions/v2/<lineage>/review-receipt.json` carrying `final_candidate_tree` — and that value **is a real git tree object** (`git cat-file -t <final_candidate_tree>` → `tree`, verified on lineage `review-cd5a0e7bf37aed72` → `7f2e634f…`). So the approved content is content-addressed into git history, and the landing commit is the one whose tree **contains that exact approved tree**:

```
# receipts approved for this change (recorded at delivery time in apply-progress)
T=$(jq -r .final_candidate_tree <receipt>)
git log --first-parent --format='%H %T %cd' --date=format-local:'%Y-%m-%d %H:%M:%S' origin/main
# select the commit whose tree equals, or whose diff introduced, T
t1 = committer timestamp of that commit
```

Why this is stronger: the previous rule scanned `openspec/changes/{change}/` and skipped commits by path heuristics, so **any** later commit touching that folder could shift t1 — the exact risk raised. A receipt binds to *content*, so a subsequent unrelated commit cannot move it. When a change delivers as chained PRs, each slice has its own approved receipt; t1 is the committer timestamp of the **last** one to land, matching the locked decision.

**Fallback, in order:**
1. Receipt-bound resolution above (authoritative).
2. If receipt-driven development was disabled for that candidate, or no receipt is discoverable, fall back to the first-parent scan of `openspec/changes/{change}/`, skipping commits that touch `openspec/changes/archive/` or that touch only `archive-report.md`.
3. If neither resolves, omit `total_wall_clock_hours` (R-021) and disclose which resolution was attempted.

The resolution path actually used MUST be named in `variance_vs_plan` — `receipt-bound` or `path-scan` — so no reader has to guess how a number was produced.

**Rejected**: commit trailers (new convention, absent retroactively); branch naming (inconsistent); `gh pr list` (network + `gh` dependency, breaks portability); `sdd-attempt` ledger (no timestamp fields — confirmed by direct inspection: record keys are exactly `begin, change, operation, previous_revision, request_digest, request_id, schema`).

### D3 — Where anchors are written
| Store | Location | Content |
|---|---|---|
| Engram | `sdd/{change}/actuals` → `variance_vs_plan` terminal sentence "Cycle timestamps: …" | t0 (Created + obs id + topic), t1 (SHA + timestamp + rule name), computed hours |
| OpenSpec | `## Cycle Timestamps` section **appended** to archived `archive-report.md` | 2-row table: t0 (topic, obs id, Created); t1 (SHA, committer time, identification rule spelled out) |

Append is a named, proposal-sanctioned carve-out to closure-feedback's "do NOT modify engine outputs" rule: append-only, delimited section, engine bytes untouched, Engram archive-report topic untouched.

### D4 — Boundary co-move (archive → merge)
Edits: schema descriptions of `total_wall_clock_hours` + `checkpoint_count`; `inception-pipeline/SKILL.md:142/144`; `sdd-time-estimation/SKILL.md:28` parenthetical; contract-test pins (`"from the tiering go-ahead checkpoint to archive"` → `…to merge`). Post-merge archive-authorization replies are bookkeeping, outside both boundaries. **Flag**: proposal listed the schema "Unchanged", but leaving descriptions at "archive" contradicts the amended spec AND the pinned test; the spec's own binding permits description-only edits. 13-property list and `additionalProperties: false` untouched.

### D5 — t0 missing → omit, never estimate (locked)
**Forced consequence the proposal did not sanction**: `total_wall_clock_hours` is in the schema's `required` list and closure-feedback STOPs on validation failure — "omit the field" is inexecutable unless the delta spec removes `total_wall_clock_hours` from `required` (shape change beyond "descriptions and values"). **Choice**: sanction the `required` drop in the delta; record still written; omission cause disclosed in `variance_vs_plan`.
**Rejected**: STOP-with-no-record (loses all other actuals; makes the locked decision a dead letter). Surfaced in Open Questions for user visibility.

**Second, independent drop (orchestrator finding, post-round-6, Phase 12 remediation): `implementation_hours`, `review_gate_hours`, and `post_review_fix_hours` also removed from `required`, for a different reason.** R-019 says the three compute-time fields "MUST stay unpopulated until a durable source exists" and its own scenario opens "GIVEN a closed record with the three compute-time fields empty" — but until Phase 12 all three remained in the schema's `required` list, so an R-019-conforming record was REJECTED by the very schema this change ships, and closure-feedback's own "report and STOP" clause fired on the one thing exercising the instrument end to end (task 6.3). **Choice**: sanction this second, independent `required` drop alongside the R-021 one above — same non-destructive treatment (`required` is a floor, not a prohibition; `properties` and `additionalProperties: false` untouched). Two distinct reasons for two distinct groups of fields, never collapsed into one claim of exclusivity: `total_wall_clock_hours` under R-021 (its anchors may not resolve), the three compute-time fields under R-019 (no durable source exists yet).
**Rejected**: leaving the three compute-time fields in `required` (makes R-019's own scenario inexecutable — the same defect class this decision's first drop exists to avoid).

Two subtests bind this decision to the shipped schema — `R021_required_drops_wall_clock` (negative pin on `total_wall_clock_hours` plus the write-anyway clause) and `R019_required_drops_compute_time_fields` (negative pins on the three compute-time fields plus a positive proof that a conforming record validates against the shipped schema) — and `D2_D5_closed_schema_invariant` additionally pins the FULL remaining `required` list by exact membership (mirroring the same exact-list assertion already used for `properties`), so any further name entering or leaving the list fails independently of whether a negative pin happens to name it. This corrects this decision's earlier parenthetical ("the contract test does not pin the required list — verified"), which was true when written and is stale now that two subtests plus the exact-list assertion pin it.

### D6 — Clock skew / timezones
Both anchors host-local (Engram `Created` has no offset; git uses `--date=format-local`). If t1 ≤ t0 or duration implausible (rewritten history), omit and disclose. Same-machine anchors; residual DST/skew disclosed in `variance_vs_plan`.

### D7 — Historical n=2 records (mechanics)
- **#2096 `sdd/sync-check-repo-behind-origin/actuals`** — annotate only: append boundary-provenance sentence to `variance_vs_plan` in the committed fixture `engine/skills/testdata/corrected-actuals-sync-check-repo-behind-origin.json` (preserving every R-014 pinned substring), then upsert the live obs with exact fixture bytes (byte-compare precedent D4-2026-07).
- **#2789 `sdd/skills-validate-ondisk-gate/actuals`** — re-base per the historical-reconstruction rule (D2 revision 3, "Historical records" above): t0 = `Created` of `sdd/skills-validate-ondisk-gate/pipeline-state` (exists — saved 2026-08-05, sync `obs-f360786155f8018a`, transcript-verified); t1 = the landing SHA this change's own archive-report already names in versioned prose (`openspec/changes/archive/2026-08-05-skills-validate-ondisk-gate/archive-report.md`: "Merged to main: YES — commit `79995ea`" and "Candidate tree: `b654d2d0…`"), cross-checked via `git show -s --format=%T 79995ea` = `b654d2d0…` (match). No folder scan, no receipt-store lookup — the archive-report is the versioned artifact both values come from. Upsert recomputed hours + provenance sentence naming the archive-report as the resolution source. No fixture exists for this record.

### D8 — Runtime portability
Engram MCP text envelope (same `engram` binary under Claude Code/OpenCode/Codex) + read-only git + file append. No hooks, no `gh`, no runtime-specific APIs.

## Data Flow

    pipeline-state (Engram typed created_at) ──t0──┐
    archive-report landing_commit (+ approved_tree when a receipt exists) ─t1──┼─→ closure-feedback ─→ actuals.variance_vs_plan (Engram)
    (verified via git show -s --format=%T when approved_tree is recorded, else self-asserted) └─→ archive-report.md += "## Cycle Timestamps" (OpenSpec)

## File Changes

| File | Action | Description |
|---|---|---|
| `openspec/changes/sdd-cycle-timestamp-instrumentation/specs/actuals-instrumentation/spec.md` | Create | Delta: R-003/4/5 merge boundary, named anchors + actor, dual-store legibility, compute-time deferral text, `required` drop sanction |
| `skills/_shared/actuals-record.schema.json` | Modify | 2 description boundary edits; drop `total_wall_clock_hours` (R-021) and, in Phase 12 remediation, `implementation_hours`/`review_gate_hours`/`post_review_fix_hours` (R-019) from `required` (D5, flagged) |
| `engine/skills/actuals_instrumentation_contract_test.go` | Modify | Update boundary pins; add pins for anchor prose ("merge-commit committer timestamp", "Cycle Timestamps"); 13-property list untouched |
| `skills/inception-pipeline/SKILL.md` | Modify | See closure-feedback edit list below |
| `skills/sdd-time-estimation/SKILL.md` | Modify | Line 28: "(tiering-go-ahead to archive, …)" → merge; R-009 pins unaffected |
| `engine/skills/testdata/corrected-actuals-sync-check-repo-behind-origin.json` | Modify | Append provenance sentence (D7) |
| Engram #2096 / #2789 | Upsert | Annotate / re-base (D7) |

**closure-feedback edit, precisely** (`skills/inception-pipeline/SKILL.md:128-157`): (1) Plan list: new bullet — harvest t0 via `engram export`'s typed `created_at` field of `sdd/{change}/pipeline-state`, never the rendered `Created:` footer (D1); (2) line 142: boundary "through archive" → "through merge — committer timestamp of the last landing commit on main (merge commit of the last PR; the landing commit itself when no PR)"; keep "MUST include any interruption gaps"; add bookkeeping-exclusion sentence; (3) new paragraph: D2 revision 3 algorithm — anchor verified via `git show -s --format=%T <landing_commit>` against the recorded `approved_tree`, and omit/disclose failure rules, no folder scan anywhere; (4) line 144: checkpoint boundary co-move, preserving pinned markers `one unit per distinct human round-trip reply` and `explicitly zero`; (5) Execute: item 3 = archive-report append; partial-write STOP rule extended to all three writes; (6) Gotchas: append-only carve-out named.

## Interfaces / Contracts

Anchor sentence (actuals): `Cycle timestamps: t0 = 2026-08-13 10:12:01 (Engram obs #NNNN, sdd/{change}/pipeline-state, typed created_at field); t1 = 2026-08-15 17:40:22 (landing_commit <sha>, verified: git show -s --format=%T <sha> == approved_tree <tree>); total_wall_clock_hours = 55.47.` WHEN no review ran for the candidate, `approved_tree` is never recorded and the sentence instead states `t1 = ... (landing_commit <sha>, self-asserted: no review receipt exists for this candidate); total_wall_clock_hours = 55.47.` — t1 STILL resolves; only the outcome label changes. WHEN a recorded `approved_tree` does not verify, the sentence instead states the anchor as `rejected` and omits `total_wall_clock_hours` (R-021); WHEN no `landing_commit` was ever recorded, the anchor is `absent` and `total_wall_clock_hours` is omitted — never a folder scan, never a receipt-store lookup.

## Testing Strategy

| Layer | What | Approach |
|---|---|---|
| Unit | Boundary pins, new anchor markers, schema shape | Updated Go contract test: RED before edits, GREEN after |
| Integration | D2 determinism **without waiting a cycle** | `TestD2AnchorIsVerifiableAndUnambiguous` (real repo git, no synthetic fixtures): a matching-tree anchor resolves; a mismatched-tree anchor is rejected, not trusted; a tree carried by more than one real commit on `main` (the genuine 3-way collision cited in D2's Disambiguation rule) is ambiguous and never resolved by position; a `nil` anchor yields no t1 and cannot invoke a folder scan by construction (the function's signature accepts no path/slug argument); a structural subtest reads shipped `SKILL.md` directly and asserts it matches the same proven rule |
| Integration | Upserts | Read back #2096/#2789 via `mem_get_observation`; #2096 byte-equals fixture |
| E2E | First live run | This change's own closure (n=3) — evidence, not a gate |

## Threat Matrix

| Boundary | Applicability | Design response |
|---|---|---|
| Documentation-like paths | N/A — markdown append only, no executable classification | — |
| Git repository selection | Applicable — the only shipped git command is `git show -s --format=%T <landing_commit>` (read-only, no `-C` flag — closure-feedback runs it from the repository root) | Failure (missing ref/shallow clone) → omit t1 + disclose; RED coverage = `R016_R017_R018_closure_feedback_anchor_prose` pins the exact command string in SKILL prose, plus `TestD2AnchorIsVerifiableAndUnambiguous` proving the verify/reject logic against real commits |
| Commit state / Push state / PR commands | N/A — closure-feedback performs no staging, commit, push, or PR automation (`gh` explicitly rejected in D2) | — |

## Migration / Rollout

D7 upserts are the only migration; reversible by topic-key upsert. Revert = delta spec + 5 file edits (`skills/_shared/actuals-record.schema.json`, `engine/skills/actuals_instrumentation_contract_test.go`, `skills/inception-pipeline/SKILL.md`, `skills/sdd-time-estimation/SKILL.md`, `engine/skills/testdata/corrected-actuals-sync-check-repo-behind-origin.json`) + 2 file deletions (`engine/skills/actuals_instrumentation_d2_anchor_test.go`, `engine/skills/testdata/corrected-actuals-skills-validate-ondisk-gate.json`) — 7 files total. The anchor test (`actuals_instrumentation_d2_anchor_test.go`) reads shipped `SKILL.md` prose at runtime (see D2 revision 3's Testing Strategy row), so deleting it is mandatory, not optional: leaving it in place after reverting the other 5 edits pins reverted prose against a test that still expects the shipped wording, leaving the suite RED.

## Open Questions

- [ ] **D5 `required` drop** exceeds the proposal's "schema Unchanged" claim — needs user confirmation before apply (locked "omit the field" is otherwise inexecutable).
- [ ] Contract-test pin edits are compatible with the success criterion's letter ("13-property list untouched") but the criterion's spirit ("test still green") is met by editing pins — flagged for transparency.
- [ ] A post-merge, pre-archive commit touching the change folder on main (beyond skipped bookkeeping) would shift t1 late; mitigated by cross-check + disclosure, not structurally prevented.
- [ ] This change's own `pipeline-state` may be absent (Engram registration repaired mid-cycle); its own closure may exercise the D5 path.
