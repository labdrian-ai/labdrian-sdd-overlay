# Tasks: anti-generic-ai-design-skill

**Change:** anti-generic-ai-design-skill
**Spec requirements covered:** Forbidden-pattern list beyond frontend-design | Reference palette/typography grounded in real sites | v1 validation heuristic (manual, rule-based) | Skill directory structure matches verifiable-skill precedent | Skill is indexed via skill-registry refresh, not hand-edited YAML
**Delivery strategy:** single-shot (content-only, no code) | **Chain strategy:** none — one PR
**TDD mode:** N/A — this change is pure authored content (SKILL.md + references/), no executable logic, no engine/TUI/Go code. No unit tests apply. Verification is manual structural/content review (see T-06) instead of a test suite.

---

## Dependency graph

```
T-01 (directory scaffold)
  └─> T-02 (SKILL.md)
        ├─> T-03 (references/palette-typography.md)
        ├─> T-04 (references/example-output.md)
        └─> T-05 (references/critique-template.md)
              all of T-02..T-05 ──> T-06 (manual content verification)
                                      └─> T-07 (skill-registry refresh --force)
                                            └─> T-08 (final verification)
```

---

## Tasks

### [x] T-01 — Scaffold skill directory
**Sequential:** first
**Spec:** Skill directory structure matches verifiable-skill precedent
**File(s):** `skills/anti-generic-design/` (NEW dir), `skills/anti-generic-design/references/` (NEW dir)

Create the directory structure only (empty placeholders acceptable at this step):
```
skills/anti-generic-design/
└── references/
```

No SKILL.md content yet — this task just establishes the structure matching the `anthropics/knowledge-work-plugins` `data/skills/data-context-extractor` precedent (SKILL.md + references/, no code).

Work-unit commit: `chore(skills): scaffold anti-generic-design directory structure`

---

### [x] T-02 — Author SKILL.md
**Sequential:** after T-01
**Spec:** Forbidden-pattern list beyond frontend-design; Skill directory structure matches verifiable-skill precedent
**File(s):** `skills/anti-generic-design/SKILL.md` (NEW)

Write SKILL.md per the design's File & Content Model, in order:
1. YAML frontmatter (`name: anti-generic-design`, `description`, `license: Apache-2.0`, `metadata.author`, `metadata.version`) — matches the repo skill convention (e.g. `genesis-design-system`).
2. **Activation Contract** — one paragraph stating this skill is complementary to, not a replacement for, `frontend-design`; explicitly names frontend-design's three existing bans (cream+serif+terracotta, near-black+acid-green, broadsheet-hairlines) so the non-overlap is visible.
3. **Forbidden patterns (the gap)** — hard list of all four: Inter/equivalent over-used sans, violet→blue gradient, generic `box-shadow` cards, flat 3-column grid.
4. **Steer toward** — 3–5 positive directions summarized from `references/palette-typography.md` (near-monochrome + warm accent, editorial/numbered sequencing, asymmetric layout, conversational copy), each pointing to `references/`.
5. **Self-critique checklist (v1, manual)** — the keyword/rule pass inlined, runnable by hand, no tooling.
6. **References** — links to the three `references/` files.

Constraints:
- Content-only; do not reference or edit any engine/TUI code.
- Do not hand-edit `skills.registry.yaml` in this task (registration happens in T-07).

Work-unit commit: `feat(skills): author anti-generic-design SKILL.md`

---

### [x] T-03 — Author references/palette-typography.md
**Parallel with T-04, T-05** (after T-02)
**Spec:** Reference palette/typography grounded in real sites
**File(s):** `skills/anti-generic-design/references/palette-typography.md` (NEW)

Write the traceability-first table per the design schema (Aspect | Distilled direction | Source site(s)):
- Base palette, Accent, Typography, Sequencing, Layout, Voice rows (or equivalent breakdown).
- Every row attributed to at least one of the 4 real sites: depoluxe.xyz, bymonolog.com, collection.industries, useportal.net.
- All 4 sites MUST appear somewhere in the file.
- Distill directions (ranges/patterns), do NOT copy verbatim hex codes, typefaces, or layout specifics lifted directly from the sites — re-derive honestly from the already-fetched captures per ADR-4.

Work-unit commit: `feat(skills): author anti-generic-design palette-typography reference`

---

### [x] T-04 — Author references/example-output.md
**Parallel with T-03, T-05** (after T-02)
**Spec:** Reference palette/typography grounded in real sites (supporting artifact); Skill directory structure matches verifiable-skill precedent
**File(s):** `skills/anti-generic-design/references/example-output.md` (NEW)

Write a worked "before (generic) → after (distinctive)" pass:
- A short "before" description/snippet exhibiting the forbidden pattern cluster (Inter, gradient, shadow cards, flat 3-col grid).
- A short "after" description/snippet applying the distilled directions from `palette-typography.md`, citing which source site(s) informed the change.

Work-unit commit: `feat(skills): author anti-generic-design worked example`

---

### [x] T-05 — Author references/critique-template.md (the v1 heuristic)
**Parallel with T-03, T-04** (after T-02)
**Spec:** v1 validation heuristic (manual, rule-based)
**File(s):** `skills/anti-generic-design/references/critique-template.md` (NEW)

Write the blank, copy-paste self-critique checklist — pure text, no script, no tooling reference:
```
[ ] font-family does NOT resolve to Inter / Roboto / generic system sans → PASS
[ ] no violet→blue (or indigo→purple) gradient on background/hero/CTA      → PASS
[ ] cards are not styled by a generic soft box-shadow alone                 → PASS
[ ] layout is not a flat, even 3-column grid                                → PASS
[ ] palette is near-monochrome + ≤1 warm accent (see palette-typography.md) → PASS
[ ] at least one editorial/asymmetric signal present                        → PASS
```
Must be completable using only the checklist text — no script or tool invocation required (spec requirement, manual-only v1).

Work-unit commit: `feat(skills): author anti-generic-design v1 self-critique checklist`

---

### [x] T-06 — Manual content verification (no test suite — TDD N/A)
**Sequential:** after T-02, T-03, T-04, T-05
**Spec:** all four content requirements (cross-check)
**File(s):** none (review only)

This change has no executable code, so there is no unit-test task. Instead, manually verify against the spec's scenarios:
1. Open `SKILL.md` — confirm all four forbidden patterns are named explicitly, and the complementarity statement (vs. frontend-design's three bans) is present.
2. Open `references/palette-typography.md` — confirm every entry cites at least one of the 4 named sites, and all 4 sites appear somewhere in the file.
3. Walk through `references/critique-template.md` by hand against the worked example in `example-output.md` — confirm it is completable with text only, no tooling.
4. List `skills/anti-generic-design/` — confirm it contains exactly `SKILL.md` + `references/` (3 files inside), and no engine/TUI source code.

Work-unit: no commit (verification checkpoint before registry step).

---

### [x] T-07 — Register the skill via skill-registry refresh
**Sequential:** after T-06
**Spec:** Skill is indexed via skill-registry refresh, not hand-edited YAML
**File(s):** `.atl/skill-registry.md` (REGENERATED)

Run:
```
gentle-ai skill-registry refresh --force
```
(equivalently the `/skill-registry` skill/command).

Do NOT hand-edit `skills.registry.yaml` as part of this or any prior task — the refresh command is the sole indexing mechanism (ADR-5).

Work-unit commit: `chore(skills): index anti-generic-design via skill-registry refresh`

---

### [x] T-08 — Final verification: registry output + rollback sanity
**Sequential:** after T-07
**Spec:** Skill is indexed via skill-registry refresh, not hand-edited YAML (scenario check)
**File(s):** none (review only)

1. Confirm `.atl/skill-registry.md` now contains a row for `anti-generic-design` with its trigger/description and path.
2. Confirm no manual edits were made to `skills.registry.yaml` in this change (git diff should show it untouched, or only refresh-tool-driven if it's a generated artifact itself).
3. Confirm rollback sanity: deleting `skills/anti-generic-design/` and re-running the refresh command would cleanly remove it from the index (additive-only, no other files depend on it).

Work-unit: no commit (final checkpoint; change is done after T-07's commit).

---

## Execution order summary

| Order | Task | Depends on | Can parallel with |
|-------|------|-----------|-------------------|
| 1 | T-01 scaffold directory | — | — |
| 2 | T-02 SKILL.md | T-01 | — |
| 3 | T-03 palette-typography.md | T-02 | T-04, T-05 |
| 3 | T-04 example-output.md | T-02 | T-03, T-05 |
| 3 | T-05 critique-template.md | T-02 | T-03, T-04 |
| 4 | T-06 manual content verification | T-02–T-05 | — |
| 5 | T-07 skill-registry refresh --force | T-06 | — |
| 6 | T-08 final verification | T-07 | — |

**Parallelism pairs:** T-03 ∥ T-04 ∥ T-05 (after T-02).

---

## Review Workload Forecast

| Metric | Estimate |
|--------|----------|
| SKILL.md | ~90 lines |
| references/palette-typography.md | ~30 lines |
| references/example-output.md | ~40 lines |
| references/critique-template.md | ~15 lines |
| `.atl/skill-registry.md` (regenerated, not authored) | small diff |
| **Total estimated changed lines** | **~175 lines** |

**400-line budget risk: Low**
**Chained PRs recommended: No — single PR**
**Decision needed before apply: No**
