# Proposal: SDD Cycle Timestamp Instrumentation

**Change**: `sdd-cycle-timestamp-instrumentation` | **Project**: labdrian-sdd-overlay | **Base**: main @ `ef35927` | **Store**: hybrid

## Intent

The actuals instrument records no durable cycle duration; calibration is stuck at n=2. `total_wall_clock_hours` is computable from artifacts closure-feedback already reads, but nothing mandates harvesting them — and the spec's right edge (archive) measures bookkeeping, not work: `deterministic-verification-evidence` merged 2026-08-05 and sat unarchived 5 days, so an archive-anchored measure would report ~5 phantom days. Anchor the cycle at merge and make both anchors legible in Engram and OpenSpec.

## Scope

### In Scope
- **Spec amendment** (delta to `actuals-instrumentation`): R-003/R-004/R-005 right edge changes **archive → merge**; anchors and harvesting actor named; both boundary-scenario texts updated.
- Anchors: **t0** = `Created` of `sdd/{change}/pipeline-state` (tiering go-ahead, `inception-pipeline/SKILL.md:142`); **t1** = merge-commit committer timestamp of the last PR that landed the change (landing commit when no PR). Git-derived — portable across Claude Code, OpenCode, Codex; no hooks.
- **closure-feedback** (existing single writer) computes `total_wall_clock_hours = t1 − t0` (interruption gaps included, per R-003), records both anchors in `variance_vs_plan` free text, and adds a "Cycle timestamps" archive-report section (t0 obs id + Created; t1 SHA + timestamp). Precedent: archive-report already lists `Created` per artifact (2026-07-30 archive, lines 19-24).
- **n=2 sample**: both records get boundary-provenance annotations in `variance_vs_plan`. `sync-check-repo-behind-origin` (~36h reconstructed, R-014) is annotated only — re-basing a reconstruction is false precision. `skills-validate-ondisk-gate` is re-based to merge-t1 if both anchors resolve, else annotated. Records are cross-comparable only via these annotations.
- **Documented deferral**: `implementation_hours`, `review_gate_hours`, `post_review_fix_hours` stay unpopulated — no durable source exists (telemetry is session-transient; sdd-attempt ledger has no timestamp fields; transcripts lack structured subagent durations). R-001/R-002 forbid filling them with elapsed time. Stated in spec text, not left silently broken.

### Out of Scope
- Per-phase duration breakdown (whole-cycle only).
- Any schema property change — `additionalProperties: false` and the CI-pinned 13-property list (`actuals_instrumentation_contract_test.go:224-230`) unchanged.
- Hook-based stamping; `sdd/{change}/state` reuse (forbidden, `pre-sdd-contracts.md:47`).
- Engram MCP registration repair (already resolved at machine-config level — see Dependencies).

## Capabilities

### New Capabilities
None.

### Modified Capabilities
- `actuals-instrumentation`: R-003/R-004/R-005 capture boundary becomes tiering-go-ahead → **merge**; anchor sources, harvesting actor, and dual-store legibility become requirements; compute-time deferral documented. Shared-boundary invariant preserved: `checkpoint_count` moves to merge too (post-merge archive-authorization replies are bookkeeping) — **pending user confirmation**.

## Approach

Option 1 from exploration: no new writer, no new actor, no schema change. closure-feedback reads t0 from Engram metadata it already touches and t1 from git, then writes the anchors where readers already look (actuals record, archive-report).

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `openspec/specs/actuals-instrumentation/spec.md` | Modified (delta) | Boundary, anchors, legibility, deferral |
| `skills/inception-pipeline/SKILL.md` | Modified | closure-feedback harvests/records t0, t1 |
| `sdd/{change}/actuals` + archive-report | Modified | Anchors legible in both stores |
| n=2 historical records | Modified | Provenance annotation / re-base |
| `skills/_shared/actuals-record.schema.json` | Modified (description only) | The `total_wall_clock_hours` description literally reads "from the tiering go-ahead checkpoint to archive", so the boundary move must edit it. **No property is added or removed** — the closed schema and the CI-pinned 13-property list are preserved. Whether `total_wall_clock_hours` also leaves `required` is a separate open decision (see Risks). |
| `engine/skills/actuals_instrumentation_contract_test.go` | Modified | Line 45 pins the exact phrase `"from the tiering go-ahead checkpoint to archive"`. The boundary move turns CI red unless this pin is co-edited. |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| No `pipeline-state` observation exists for a cycle, so t0 is absent | Med | Registration repaired 2026-08-13, effective next session; when t0 is missing, omit the field — never estimate (`SKILL.md:146`) |
| `Created` not programmatically retrievable via `mem_get_observation` | Med | Design phase confirms before locking mechanism |
| `checkpoint_count` boundary co-move changes its semantics | Med | Flagged for user decision |
| Engram clock vs git committer-time skew | Low | Same-machine anchors; disclose in `variance_vs_plan` |

## Rollback Plan

Revert the delta spec and `SKILL.md` edit. Historical-record annotations are additive free text, reversible by topic-key upsert. No schema or engine change to revert.

## Dependencies

- **Engram MCP registration — RESOLVED 2026-08-13, no code change.** Engram was registered twice under the same server name: once by the `engram@engram` plugin and once as a direct `mcpServers.engram` entry in the user config. The direct entry occupied the name, so the plugin's server never registered and the plugin-namespaced tools the agents declare resolved to nothing. `claude mcp remove engram -s user` restored `plugin:engram:engram`. The namespace the agents declare was always correct; there was no repository or upstream defect. Agent tool bindings are read at session start, so the repair lands in the next session — which is why this phase still could not write to Engram.
- Confirmed programmatic retrieval of observation `Created`.

## Success Criteria

- [ ] Delta spec amends R-003/R-004/R-005 with merge right edge and named anchors; OpenSpec validation passes.
- [ ] `actuals_instrumentation_contract_test.go` still green — 13-property list untouched.
- [ ] Next archived change's archive-report shows t0 (obs id + Created) and t1 (merge SHA + timestamp); `total_wall_clock_hours` equals their difference.
- [ ] Both n=2 records carry boundary-provenance annotations.
- [ ] Compute-time deferral documented with its technical reason in spec and skill text.
