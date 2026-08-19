# Exploration: sdd-cycle-timestamp-instrumentation

**Change**: `sdd-cycle-timestamp-instrumentation` | **Project**: labdrian-sdd-overlay | **Phase**: explore | **Date**: 2026-08-13
**Artifact store**: hybrid | **Base**: main @ `ef35927`

## Problem

The SDD actuals/calibration instrument produces no data. Effective sample size is stuck at **n=2**. The goal is to record timestamps in both Engram and OpenSpec so the real duration of a complete development cycle can be computed.

## Current state

`skills/_shared/actuals-record.schema.json` is closed (`additionalProperties: false`), 13 properties, 9 required. `engine/skills/actuals_instrumentation_contract_test.go:224-230` pins the exact 13-name property list, so **any new schema property fails CI**. An artifact outside the schema is legal; a new schema field is not, without a spec amendment.

Of the 9 required fields:

| Field | Unit | Durable source today |
|---|---|---|
| `implementation_hours` | agent compute | **none** |
| `review_gate_hours` | agent compute | **none** |
| `post_review_fix_hours` | agent compute | **none** |
| `total_wall_clock_hours` | elapsed calendar | **yes — unharvested** |
| the other five | narrative / derivable | yes |

The three compute-time fields were derived once, for `skills-validate-ondisk-gate` (obs #2789), from live orchestrator telemetry — per-subagent second counts that no file in the repo writes. They cease to exist when the session ends. R-001/R-002 forbid substituting elapsed time for them, so they are out of scope for a timestamp mechanism.

`closure-feedback` exists only as prose in `skills/inception-pipeline/SKILL.md:128-152`. The sole code reference, `engine/skills/actuals_instrumentation_contract_test.go:111`, asserts that section's *text*, not that it runs. Whether it fired is unobservable after the fact; it silently did not fire for 2 of 5 changes.

No stamping actor is specified anywhere. `openspec/specs/actuals-instrumentation/spec.md:19-33` (R-003/4/5) defines the capture boundary (tiering-go-ahead → archive, interruption gaps included) and forbids new properties (`spec.md:21`), but never names who stamps, nor mandates that any timestamp be recorded.

## Key finding: the boundary is already bracketed by two artifacts closure-feedback already reads

`skills/inception-pipeline/SKILL.md:142` names `sdd/{change}/pipeline-state` as the artifact recording the tiering-go-ahead checkpoint — the exact left edge of the R-003 boundary. `SKILL.md:132-138` shows closure-feedback's Plan step already reads both `sdd/{change}/pipeline-state` and `sdd/{change}/archive-report` — but only their *content*, never their `Created` metadata.

That Engram observation `Created` timestamps are usable is not theoretical: `openspec/changes/archive/2026-07-30-actuals-calendar-checkpoint-instrumentation/archive-report.md:19-24` already lists them per artifact (`Proposal: obs #2538 (2026-07-29 18:18:43)`).

So `total_wall_clock_hours = Created(archive-report) − Created(pipeline-state)` is computable today with **no new writer and no schema change**.

## Investigated and ruled out

**`gentle-ai sdd-attempt` ledger — dead end, confirmed by direct inspection.** Its records under `.git/gentle-ai/sdd-runtime/v1/<change>/records/*.json` carry exactly these keys: `begin, change, operation, previous_revision, request_digest, request_id, schema`. There is **no timestamp field**. Its "immutable history" is a content-addressed hash chain, not a timed log. The only `_at` field in the tree is `acquired_at` inside the transient `LOCK` (`gentle-ai.review-store-lock/v1`). Filesystem mtimes on the record files do carry signal (30 records spanning ~26h on `deterministic-verification-evidence`) but are not durable across clones and are not part of any schema.

**Harness-level subagent duration — not a reliable source.** The orchestrator session transcript has `timestamp` on every `assistant` line, but `isSidechain==true` matches zero lines, so subagent internals are absent. `duration_ms` appears exactly once in an entire session transcript, embedded in free text inside one agent's result block, not as a structured field. Hook events `SubagentStart`/`SubagentStop` exist and carry `agent_id`/`agent_type` but no duration, and `agent_id` correlation stability across concurrent subagents is undocumented.

**`sdd/{change}/state`** is explicitly off-limits: `skills/_shared/pre-sdd-contracts.md:47` says engine-owned keys "are NOT listed here — do not reuse them."

## Blocker discovered during this phase

`sdd-explore` returned `status: partial` because it could persist nowhere. Root cause, verified:

- All 13 agent definitions under `~/.claude/agents/` declare `mcp__plugin_engram_engram__mem_save` (and `mem_search`/`mem_get_observation`).
- This session's Engram MCP server is registered as `engram` (`command: engram`, `args: mcp --tools=agent`), so its tools are `mcp__engram__*`. `ToolSearch` for `mcp__plugin_engram_engram__mem_save` returns no match. No engram plugin is installed.
- The overlay itself **ships and deploys** the stale namespace: `agents/sdd-explore.md` carries it and `overlay.manifest:79` marks it `managed`.

Consequence: no SDD phase agent can write to Engram under this configuration. `sdd-explore` additionally has no `Write` tool by design, so it can persist to neither backend. **Hybrid mode is silently degraded**, which is the same class of failure this change exists to fix — an instrument that cannot record.

## Options

| # | Option | Spec status | Effort |
|---|---|---|---|
| 1 | closure-feedback diffs `Created(pipeline-state)` and `Created(archive-report)` to set `total_wall_clock_hours` in place | **sanctioned** — R-003 already requires in-place correction; no new property | Low |
| 2 | Harvest the `sdd-attempt` ledger | — | **ruled out**: no timestamps exist |
| 3 | New artifact outside the schema (`sdd/{change}/cycle-timestamps` + `openspec/changes/{change}/timings.md`) stamped per phase boundary | artifact mechanism sanctioned; the stamping actor underneath is undefined | Medium |
| 4 | Hook-based stamping (`SubagentStart`/`SubagentStop`) | needs spec change; Claude-Code-only, not portable to OpenCode/Codex | High |
| 5 | Relax `required` for the three compute fields | ambiguous; legitimizes absence without creating data | Low, solves nothing |

## Open questions for the proposal

1. **Which store is authoritative** when both hold a timestamp, and how do they stay consistent?
2. **Right-edge anchor**: `archive-report` creation is pollutable by archive lag — a real incident in this repo (`deterministic-verification-evidence` sat unarchived 5 days, `archive-report.md:1-13`). Is the intended measure tiering-go-ahead → **archive**, or → **merge** (needs a third anchor, e.g. merge-commit time)?
3. **Interrupted/resumed cycles**: R-003 says interruption gaps are included; confirm that survives a multi-session cycle.
4. **Per-phase granularity**: is whole-cycle wall clock enough, or is per-phase needed (which forces option 3 and a defined actor)?
5. **Is `Created` programmatically retrievable** by a skill via `mem_get_observation`/`mem_search`, or only as rendered display text? Must be confirmed before design locks option 1.
6. **Does the Engram namespace defect block this change**, or is it a separate prerequisite?

## Recommendation

Option 1 as the immediate, spec-sanctioned fix — it needs no schema change, no new actor, and reuses two artifacts closure-feedback already reads. Option 3 only if per-phase granularity is required, and only after a stamping actor is named. Options 2, 4 and 5 are not recommended.

Resolve the Engram namespace defect before or alongside this change: a measurement instrument built on a pipeline whose phases cannot record is not verifiable.
