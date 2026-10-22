# Role agents — v3 slice 1 (Phase 9, PR 1 of `docs/architecture/role-design/per-role-design-v3.md` §6)

Objective: role sources and Claude Code agent files for the six writing/judging roles, per the approved v3 design. Branch `feat/phase9-role-agents-s1`, stacked on `docs/phase9-c12-role-design` (design not yet on `main`). Delivery: `ask-on-risk`, one PR. Runners: `go test ./...` in `engine/`, `labdrian skills validate`/lint where they apply, structural readback for prose.

## Specs

Every spec quotes the approved design `docs/architecture/role-design/per-role-design-v3.md` (v3); read it and the drafts it cites (v2 §2, P §6, S §3) before any edit.

- **S1** — v3 §6 row 1, verbatim: "Role sources + Claude agent files for builder, shaper, polisher, prototyper, sweeper, delivery; manifest `agent` rows; `skills/judgment-day` actors renamed; `minimalism-contract.md` `applies_to_phases: [shaper, builder]`, `excluded_phases` += reviewer/verifier, stale \"resolver does not parse it\" note fixed; `engine propagate` registry row". Risk named there: "item 3 (the contract is gate-consumed; `.atl/skill-registry.md` rows consumed by `skills validate`)".
- **S2** — Role sources, v3 §5: "Source per role: `roles/<role>/body.md` (core prefix + delta)". Core prefix, v3 §2: "v2 §2 lines 49-54 verbatim". Each role's tools, model and delta are the v3 §2 table row for that role, verbatim (builder, shaper, polisher, prototyper, sweeper, delivery only; reviewer and verifier are PR 2).
- **S3** — Claude agent files `agents/<role>.md`, v3 §2: "Each agent file ≈ 20-30 lines; no stamped gentle blocks (v2 §1: they are gentle's sync markers; the generator never emits them)." Names stay `labdrian`-era (v3 D13); no persona (D1).
- **S4** — Scope fence. Not in this slice (v3 §6 rows 2-6): reviewer/verifier/role-chain, estimator rewrite, the `engine/gadu` → `engine/agentgen` generator and Pi/Codex projection, deletion of `sdd-*`/`review-*`/`jd-*`/GADU files, CI ratchets. Existing agent files and their manifest rows stay untouched.
- **S5** — Every existing consumer of a changed contract keeps working: the gate's reading of `minimalism-contract.md` frontmatter, `skills validate` over `.atl/skill-registry.md`, the judgment-day skill's references. Tests prove the previous behaviour still holds where a runner exists (ODD test-first: RED before GREEN).

## Tasks

| ID | Specs | Route | Status | Commit / evidence |
|---|---|---|---|---|
| T1 | S1-S5 | delegated (owner: "sonnet max para implementaciones"), workflow map → implement → verify; L4 fix inline | done | this commit; `skills validate` (CI form) exit 0; engine suite green except 3 baseline-environmental failures |

## Log

- **L1** (2026-10-21, owner): "bueno cotinua en ese orden ve en ultracode con opus high para las verify y sonnet max para implementaciones" — item 3 of the order: "v3 slice 1 (role agents)".
- **L2** (2026-10-22): b64 merged as #663 (`7d1f665a`); C12 rebased clean onto it (`45461ee6`, still unpushed); this branch stacks on it. 32 leaked osproc test shells killed (ppid 1, > 1 h); none remain.
- **L3** (2026-10-22): workflow `wf_96941905-53c` (Sonnet max writer, two Opus high verifiers, one correction + recheck). S1-S4 pass; S5 fails with 3 open blockers, uncommitted: (a) the literal `applies_to_phases: [shaper, builder]` stops contract injection for the legacy `sdd-tasks`/`sdd-apply` agents on Claude, and the packaged Pi gate (`engine/pipkg/labdrian-gate.ts:30`, hardcoded `PHASE_NAMES`) then injects for no agent, breaking `openspec/specs/pi-runtime-target/spec.md:71-72`; v3 §4's "Claude PreToolUse gate only" premise is wrong (Pi gate and OpenCode derive also read the frontmatter); (b) `engine/runtime/opencode_test.go:100` pins the old phase list; (c) `skills/judgment-day/SKILL.md` changed bytes and needs a human `labdrian skills approve --id judgment-day`. Needs owner decision on (a).
- **L4** (2026-10-22, owner): chose "Transición (Recomendado)" — `applies_to_phases: [sdd-tasks, sdd-apply, shaper, builder]` until PR 5 removes the sdd-* agents; deviates from S1's literal list on purpose. Applied inline: contract frontmatter + scope note (names the Pi gate and the OpenCode prompt config as readers), gate/propagator/skills pins, `engine/runtime/opencode_test.go` pin (included + excluded lists). Pi gate untouched and working again (its hardcoded pair is in the list). `go test` of runtime/gate/propagator ok; vet/gofmt clean; remaining red: engine/skills approval pair, pending the owner's `skills approve` of judgment-day; goal/memoryscope/settingsfile failures are baseline-environmental.
- **L5** (2026-10-22, owner): reviewed `git diff skills/judgment-day/SKILL.md` and ran `labdrian skills approve --id judgment-day --approver labdrian-ai --source-root skills` (sha256 `dd875086…72a31dc`). CI-form `skills validate` exit 0 (7 approved, 32 grandfathered); `go test ./...` red only in goal/memoryscope/settingsfile (baseline). Open advisory: the fix actor moved from jd-fix-agent's opus/xhigh to a sonnet `builder` launch (v2 OD2 "per-launch model when needed", not stated in the skill).
