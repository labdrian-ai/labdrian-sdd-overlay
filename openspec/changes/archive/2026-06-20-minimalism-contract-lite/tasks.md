# Tasks: minimalism-contract-lite

Generated: 2026-06-19
Spec: `openspec/changes/minimalism-contract-lite/specs/minimalism-contract/spec.md`
Design: `openspec/changes/minimalism-contract-lite/design.md`
Delivery strategy: ask-on-risk
strict_tdd: false (Markdown authoring; no test-execution tasks)

---

## Apply Progress

| Task | Status | Completed |
|------|--------|-----------|
| T-01 Create `skills/_shared/minimalism-contract.md` | [x] done | 2026-06-19 |
| T-02 Refactor `skills/project-architect/SKILL.md` lines 164-166 | [x] done | 2026-06-19 |
| T-03 Register in `overlay.manifest` | [x] done | 2026-06-19 |
| T-04 Regenerate scoped registry Trigger row | [x] done | 2026-06-19 |
| T-05 Verify AC-1 by observation (WARNING) | [x] done | 2026-06-19 |
| T-06 Capture manual baseline | [x] done | 2026-06-19 |
| T-07 Deploy via `overlay apply` (all targets: claude + opencode + codex) | [x] done | 2026-06-19 |
| T-FIX-1 R-005 traceability: spec-evolution note in proposal + design Decision 3 | [x] done | 2026-06-19 |
| T-FIX-2 [overlay-integrity] Deploy minimalism-contract.md + project-architect to opencode/codex | [x] done | 2026-06-19 |
| T-FIX-3 [gentle-ai-integrity] Add MECHANISM REVERSAL NOTEs to proposal.md | [x] done | 2026-06-19 |
| T-FIX-4 [gentle-ai-integrity] Add .atl/skill-registry.md to git tracking | [x] done | 2026-06-19 |
| T-FIX-5 [gentle-ai-integrity] Reconcile T-07 status (tasks.md vs apply-progress.md) | [x] done | 2026-06-19 |

---

## Review Workload Forecast

| Metric | Estimate |
|--------|----------|
| New files | 1 (minimalism-contract.md, ~30-40 lines incl. advisory frontmatter) |
| Edited files | 3 (project-architect/SKILL.md ~3 lines; overlay.manifest +1 line; `.atl/skill-registry.md` regenerated minimalism-contract Trigger row) |
| Estimated changed lines | ~45-55 total |
| 400-line budget risk | **Low** — well under budget |
| Chained PRs recommended | **No** — single-PR scope |
| Decision needed before apply | **No** — proceed directly |
| `~/.claude/CLAUDE.md` touched | **No** — global file is explicitly out of scope |

---

## Task Sequence

Tasks are ordered by dependency. T-01 must complete before any other task. T-02, T-03, T-04
may run in parallel after T-01. T-05 (AC-1 observation) runs after T-01 + T-04 (the contract
file and the scoped registry row must exist before injection can be observed). T-06 is
independent and may run at any point. T-07 is a follow-up flag, not a blocking task for this
slice.

---

### T-01 — Create `skills/_shared/minimalism-contract.md`

**Satisfies:** R-002 (six-rung ladder), R-003 (architectural tiebreaker), R-005 (three-state
`// minimal:` comment convention), R-006 (full-path injection; file must be small enough
for cheap full read)

Write `skills/_shared/minimalism-contract.md` following the exact structure from design
Decision 3:

0. An **advisory YAML frontmatter header** (option (d) / part A — documentation, NOT the
   scoping mechanism):
   ```yaml
   ---
   applies_to_phases: [sdd-tasks, sdd-apply]
   excluded_phases: [sdd-propose, sdd-spec, sdd-design, sdd-verify, sdd-archive]
   injection_point: "## Skills to load before work"
   ---
   ```
   The orchestrator resolver does NOT parse this; it documents AC-1 intent inside the artifact.
   The real binding is the scoped registry Trigger row (T-04).
1. `# Minimalism Contract` title.
2. `## Preference ladder` — exactly six labeled rungs in order: YAGNI, stdlib/built-ins,
   native platform feature, existing dep, one-liner/minimal local code, custom code.
3. `## Architectural tiebreaker` — architecture boundaries NEVER collapsed to save lines.
4. `## Comment convention` — the three-state `// minimal:` rule:
   - **State 1 (judgment):** lower rung existed but was rejected — emit `// minimal: <reason>`
     naming the lower rung considered and why it was insufficient.
   - **State 2 (forced, non-obvious):** no lower rung applicable AND constraint is not obvious
     from context — emit `// minimal: forced — <design/constraint ref>`.
   - **State 3 (no comment):** no lower rung applicable AND constraint IS obvious from context
     — omit the comment entirely (noise-free, consistent with YAGNI).
   - Note: use the host language's single-line comment prefix (`//`, `#`, etc.).

Keep the file minimal — it must embody its own philosophy. Target ~25-35 lines.

**Parallel:** No — all other file tasks reference or depend on this file existing.

---

### T-02 — Refactor `skills/project-architect/SKILL.md` lines 164-166

**Satisfies:** R-004 (citational reference, no inline ladder prose, not a load instruction)

Replace the three lines (164-166) in the `## Rules` block that currently read:

```
- **Do NOT inflate sections** — ...
- **Do NOT inflate module count** — ...
- **Do NOT inflate trade-offs** — ...
```

with a single citational reference. The replacement text must:
- Reference `../_shared/minimalism-contract.md` explicitly as the canonical anti-inflation
  source.
- State that the reference is **citational, not a load instruction**: the 6-rung ladder
  applies only during `sdd-tasks`/`sdd-apply`, NOT during architecture/design.
- NOT reproduce the six-rung ladder in SKILL.md prose.
- NOT add `minimalism-contract.md` to project-architect's `## Skills to load before work`.

Example wording (adapt as needed):
> Anti-inflation guidance (module count, section inflation, trade-off inflation) is consolidated
> in `../_shared/minimalism-contract.md` (canonical source). This is a documentation reference
> for deduplication; the 6-rung ladder is applied only during `sdd-tasks`/`sdd-apply`, NOT
> during architecture/design.

**Parallel:** Yes — can run concurrently with T-03 and T-04 once T-01 is complete.

---

### T-03 — Register `_shared/minimalism-contract.md` in `overlay.manifest`

**Satisfies:** Design Decision 4 (overlay toolchain tracking)

Add one line to `overlay.manifest` after the existing `_shared/pre-sdd-contracts.md` line:

```
_shared/minimalism-contract.md custom
```

Classification is `custom` (overlay-original; no vendor counterpart).

**Parallel:** Yes — independent of T-02 and T-04.

---

### T-04 — Regenerate the scoped `minimalism-contract` Trigger row in `.atl/skill-registry.md`

**Satisfies:** R-001 (phase-scoped injection into `sdd-tasks`/`sdd-apply` only) — this row is
the **load-bearing scoping binding** the orchestrator resolver actually consumes (Design
Decision 1 part B); Design Decision 4 (discoverability)

Regenerate the `minimalism-contract` row in `.atl/skill-registry.md` via the **skill-registry
skill's marker format** (NOT a freehand hand-edit), so the marker format stays consistent and
idempotent. The row carries a **phase-scoped Trigger description**:

```
| minimalism-contract | `skills/_shared/minimalism-contract.md` | Inject ONLY into sdd-tasks and sdd-apply sub-agent prompts under '## Skills to load before work'. Do NOT inject into propose/spec/design/verify/archive. |
```

This is the same path `genesis-design-system` uses (scoping via its registry Trigger, NOT via
`~/.claude/CLAUDE.md`). Notes:
- This row is BOTH the discoverable index entry AND the phase-scope expression — it is the
  mechanism, not discovery-only.
- Do NOT create a `<!-- BEGIN: ... -->` constraint block for this file (constraint blocks
  propagate to ALL phases — global contamination).
- Do NOT hardcode any phase→contract rule in `~/.claude/CLAUDE.md` — that global file is out
  of scope (see T-05).
- Scoping remains probabilistic: the resolver matches this Trigger prose, it is not a
  deterministic filter. AC-1 is verified by observation (T-05).

**Parallel:** Yes — independent of T-02 and T-03.

---

### T-05 — Verify AC-1 by observation (positive + negative), report WARNING

**Satisfies:** R-001 (phase-scoped injection into `sdd-tasks`/`sdd-apply` only), R-006
(full-path reference under `## Skills to load before work`, not a summary)

> **REPLACES the rejected original T-05.** The original T-05 edited the global
> `~/.claude/CLAUDE.md` to hardcode a phase→contract rule. That is **REJECTED**:
> `~/.claude/CLAUDE.md` is the shared global orchestrator file, is NOT overlay-tracked, and the
> "genesis-design-system scopes via CLAUDE.md" justification is FALSE (genesis scopes via its
> registry Trigger). `~/.claude/CLAUDE.md` MUST NOT be touched. The scoping binding now lives
> in the overlay-tracked registry Trigger row (T-04) plus the advisory frontmatter (T-01).
> This T-05 is therefore the **observational verification** of AC-1.

Because scoping is probabilistic (a registry Trigger the resolver matches against, not a
deterministic hook), verify AC-1 by **OBSERVATION** and report it as a **WARNING**, never as a
guarantee:

1. **Positive observation:** inspect the launch prompts the orchestrator constructs for
   `sdd-tasks` and `sdd-apply`; confirm `skills/_shared/minimalism-contract.md` appears in
   their `## Skills to load before work` blocks (R-006: full path, not a summary/paraphrase).
2. **Negative observation:** inspect the launch prompts for the other five phases
   (`sdd-propose`, `sdd-spec`, `sdd-design`, `sdd-verify`, `sdd-archive`); confirm the path
   does NOT appear in any of them.
3. **Report** the result as a WARNING-level finding (observed PASS/FAIL), not a hard gate.
   Record any over-match (path leaking into an excluded phase) as a deviation for the deferred
   option (b) parsing hook.

**Guard:** `~/.claude/CLAUDE.md` MUST NOT be edited. `sdd-phase-common.md` is runtime-owned
(gentle-ai) and MUST NOT be edited. All scoping is achieved via the overlay-tracked registry
Trigger row (T-04) + advisory frontmatter (T-01) only.

**Parallel:** No — this is an observation step; it runs after T-01 + T-04 exist (the contract
file and the scoped registry row must be in place before injection can be observed).

---

### T-06 — Capture manual baseline in a file

**Satisfies:** Proposal AC-4 (directional behavioral evidence); Design Risks section
(accepted residual fragility of probabilistic enforcement)

Create `openspec/changes/minimalism-contract-lite/baseline.md`. For 3-5 archived changes
in this overlay, record:

- Change name
- LOC added (approximate, from `git diff --stat` or inspection)
- Net new dependencies introduced (if any)
- Single-caller abstractions added (functions/classes used in only one call site)

Note: Engram persistence is currently blocked by project ambiguity (multiple git repos in
cwd). The baseline lives as a file rather than an engram observation so it is recoverable
without Engram. Target 3-5 entries; no format ceremony required — a simple Markdown table
is sufficient.

**Parallel:** Yes — fully independent; can run at any point.

---

### T-07 — Deploy via `overlay apply` (follow-up flag)

**Satisfies:** Design Decision 4 (deployment)

`overlay apply` deploys tracked files from the overlay to their target locations. Once T-01
and T-03 are complete (file created + registered in `overlay.manifest`), running
`overlay apply` propagates `skills/_shared/minimalism-contract.md` to the deployment
targets.

**In-scope determination:** This is a deploy step, NOT an authoring task. It is flagged here
for awareness but is considered a **follow-up** to the authoring slice — run it after all
authoring tasks (T-01 through T-06) are verified. It does not require a separate PR; it is
the final execution step after apply is complete.

**Parallel:** N/A — blocked on T-01 + T-03 + sdd-verify passing.

---

## Dependency Summary

```
T-01 (create contract file + advisory frontmatter)
  ├── T-02 (refactor project-architect)        [parallel after T-01]
  ├── T-03 (overlay.manifest line)              [parallel after T-01]
  └── T-04 (scoped registry Trigger row)        [parallel after T-01]
        └── T-05 (AC-1 observation, WARNING)    [after T-01 + T-04]

T-06 (baseline file)                            [independent, any order]
T-07 (overlay apply — follow-up)                [after T-01 + T-03 + verify]
```

Sequential gates: T-01 before T-02/T-03/T-04. T-05 after T-01 + T-04. `~/.claude/CLAUDE.md` is
NOT edited by any task.

---

## Requirements Coverage

| Req | Satisfied by |
|-----|-------------|
| R-001 | T-04 (scoped registry Trigger row = binding) + T-01 (advisory frontmatter); T-05 verifies by observation |
| R-002 | T-01 (six-rung ladder in contract file) |
| R-003 | T-01 (architectural tiebreaker section) |
| R-004 | T-02 (citational reference in project-architect) |
| R-005 | T-01 (three-state `// minimal:` comment convention) |
| R-006 | T-04 (full-path registry row, no summary); T-05 observes full-path injection |
| AC-1 (scoping) | T-05 (positive + negative observation, reported WARNING) |
| AC-4 baseline | T-06 (manual baseline file) |
| Deploy | T-03 (manifest), T-07 (deploy — follow-up) |
