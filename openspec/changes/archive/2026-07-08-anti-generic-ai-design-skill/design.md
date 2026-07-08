# Design: anti-generic-ai-design-skill

## Outcome

Author a self-contained, content-only overlay skill at `skills/anti-generic-design/`
(SKILL.md + `references/`) that closes the ONE gap `anthropics/skills/frontend-design`
leaves open: the "Claude/SaaS look" (Inter-or-equivalent sans, violet→blue gradient,
generic `box-shadow` cards, flat 3-column grid). The skill is **complementary** — it
names patterns frontend-design does not ban, grounds a positive palette/type direction
in 4 real fetched award sites, and adds a manual, script-free v1 self-critique checklist.
No engine/TUI/Go code is touched. Registration into the overlay's skill index is a
**post-implementation operational step** (`gentle-ai skill-registry refresh --force`),
not new code and not a hand-edit of `skills.registry.yaml`.

## Architecture Approach

Pure authored content over the existing skill-loading convention. The unit of work is a
skill *directory*, modeled 1:1 on the `anthropics/knowledge-work-plugins`
`data/skills/data-context-extractor` precedent (a `SKILL.md` orchestrator + a `references/`
folder of supporting docs, zero code). Layering:

- **SKILL.md** — the always-loaded contract: YAML frontmatter (name/description/trigger),
  a short activation contract stating complementarity with frontend-design, the explicit
  forbidden-pattern list, a pointer to `references/`, and the manual self-critique checklist.
- **`references/`** — heavier, lazily-consulted material the SKILL.md links to: the
  palette/typography reference distilled from the 4 sites, plus a worked example and a
  reusable output template (mirroring the precedent's `domain-template.md` /
  `example-output.md` / `skill-template.md` triad).

The skill introduces NO new load mechanism: it is discovered by the same
`*/SKILL.md` filesystem scan every other overlay skill uses. Its only runtime coupling is
conceptual — it runs *alongside* frontend-design, never in place of it.

## Component & Data-Flow Map

```
skills/anti-generic-design/
├── SKILL.md                         # contract: frontmatter + forbidden list + checklist + links
└── references/
    ├── palette-typography.md        # distilled palette/type, each entry cites 1..4 source sites
    ├── example-output.md            # a worked "before (generic) → after (distinctive)" pass
    └── critique-template.md         # blank, copy-paste self-critique checklist (the v1 heuristic)
```

Activation / data flow (conceptual, no code):

```
design/UI task ─▶ frontend-design (official, background)     ─┐
                    bans: cream+serif+terracotta,             │  two independent,
                          near-black+acid-green,              │  additive lenses over
                          broadsheet-hairlines                │  the same output
              ─▶ anti-generic-design (this skill)            ─┘
                    bans: Inter/equiv, violet→blue gradient,
                          generic shadow cards, flat 3-col grid
                    steers toward: references/palette-typography.md
                    self-critique: references/critique-template.md (manual)
```

Both skills are self-critique lenses applied to the SAME generated design. There is no
ordering dependency, no shared state, no merge: each flags its own disjoint pattern cluster.

## File & Content Model

### SKILL.md

Frontmatter (matches repo skill convention, e.g. `genesis-design-system`):

```yaml
---
name: anti-generic-design
description: "Flags the generic AI 'Claude/SaaS look' (Inter/equivalent sans, violet-blue
  gradient, generic shadow cards, flat 3-column grid) that frontend-design does not cover,
  and steers toward evidence-based distinctive alternatives. Trigger: when generating,
  reviewing, or refining web/UI visual design, landing pages, or component styling."
license: Apache-2.0
metadata:
  author: labdrian
  version: "1.0"
---
```

Body sections, in order:
1. **Activation Contract** — one paragraph: "complementary to, not a replacement for,
   `frontend-design`; it bans a DIFFERENT pattern cluster." Names frontend-design's three
   documented bans (attributed as an external, unpinned/unvendored reference — see the
   Activation Contract's own hedge) so a reader sees the non-overlap.
2. **Forbidden patterns (the gap)** — the four named patterns as a hard list.
3. **Steer toward** — 3–5 positive directions summarized from `references/palette-typography.md`
   (near-monochrome + single warm accent, editorial/numbered sequencing, asymmetric layout,
   conversational copy), each with a "see references/" pointer.
4. **Self-critique checklist (v1, manual)** — the keyword/rule pass, inline and runnable by hand.
5. **References** — links to the three `references/` files.

### references/palette-typography.md

A traceability-first table. Each row = one distilled token/direction + the source site(s)
that informed it. All 4 sites MUST appear somewhere. Schema:

| Aspect | Distilled direction | Source site(s) |
|--------|--------------------|----------------|
| Base palette | near-monochrome (off-white/ink) ground | depoluxe.xyz, bymonolog.com |
| Accent | single warm accent (rust/amber), used sparingly | collection.industries |
| Typography | editorial pairing; oversized display + restrained body; NOT Inter | bymonolog.com, useportal.net |
| Sequencing | numbered / indexed section markers (01, 02…) | collection.industries, depoluxe.xyz |
| Layout | asymmetric / broken-grid over flat 3-col | useportal.net, depoluxe.xyz |
| Voice | conversational, first-person copy over marketing-speak | bymonolog.com, useportal.net |

This file is a SCHEMA + qualitative direction guide informed by human recollection of the
4 sites, not by any persisted capture artifact (no screenshot, fetched HTML/CSS snapshot, or
research note is vendored in this repo). v1 ships qualitative/schema-level guidance; pinning
exact hex/typeface values to verifiable captures is deferred to a future revision that
vendors actual site captures — the design fixes the SCHEMA and the attribution rule, not
literal color codes, so the reference stays honest about its actual evidentiary status.

### references/critique-template.md — the v1 heuristic

A copy-paste checklist, pure text, NO script. Each line is a keyword/rule the reviewer
scans the output (CSS, tokens, or a design description) for:

```
[ ] font-family does NOT resolve to Inter / Roboto / generic system sans → PASS
[ ] no violet→blue (or indigo→purple) gradient on background/hero/CTA      → PASS
[ ] cards are not styled by a generic soft box-shadow alone                 → PASS
[ ] layout is not a flat, even 3-column grid                                → PASS
[ ] palette is near-monochrome + ≤1 warm accent (see palette-typography.md) → PASS
[ ] at least one editorial/asymmetric signal present                        → PASS
```

Any unchecked line = a flag to revise. Runnable with only the text, no tooling — satisfying
the spec's "manual-only, automation deferred to v2" requirement.

## Decisions (ADR-style)

### ADR-1 — Directory-with-references, not a single flat SKILL.md

**Decision.** Ship `SKILL.md` + a `references/` subdirectory (3 files), not one monolithic
SKILL.md.

**Rationale.** The spec REQUIRES the `references/` structure and pins the
`data-context-extractor` precedent (SKILL.md + references, no code). Splitting keeps the
always-loaded SKILL.md lean (contract + forbidden list + checklist) while the heavier,
evidence-grounded palette/type material and the worked example load only when consulted —
the same progressive-disclosure shape the precedent uses.

**Rejected — single SKILL.md** (like `data-visualization`, obs #1981). Simpler, but violates
the spec's explicit `references/` requirement and buries the 4-site traceability inside the
hot-path contract. The precedent chosen by the spec is the multi-file one.

### ADR-2 — Complementarity is asserted in prose + non-overlapping ban sets, not enforced in code

**Decision.** SKILL.md states in its Activation Contract that it is complementary to
frontend-design and names frontend-design's three documented bans, so the disjointness is
visible. There is NO code guard, no cross-skill import.

**Rationale.** These are content skills loaded as context; "don't duplicate frontend-design"
is a documentation contract, not a runtime invariant. The two ban sets are provably disjoint
(gradient/Inter/shadow/flat-grid vs cream-serif-terracotta / near-black-acid-green /
broadsheet-hairlines), so naming both sets is sufficient to prevent a reader conflating them.
Note: frontend-design is not vendored in this repo, so its three bans are cited as an
external, unpinned reference — verify against upstream if precision matters.

**Rejected — merging into / editing frontend-design.** Out of scope per proposal and spec;
frontend-design is upstream (anthropics) and must stay unmodified so it can be re-synced.

### ADR-3 — v1 heuristic is a manual keyword checklist, no script

**Decision.** The self-critique pass is a plain-text checklist in `critique-template.md`
(and inlined in SKILL.md), scanned by hand.

**Rationale.** Spec requirement: v1 MUST be human-runnable with no linter/tool; automation
is explicitly deferred to v2. A keyword list ("Inter", "gradient", "shadow", "3-column")
is the lowest-friction form that still catches the four target patterns. Keeping it
tool-free also keeps the skill additive and removable with zero engine surface.

**Rejected — a Go/JS validator or CSS lint rule.** That is the v2 out-of-scope item; adding
it now would touch the engine and break the "self-contained, deletable directory" property.

### ADR-4 — Every reference entry is attributed; values re-derived at apply, schema fixed at design

**Decision.** The design fixes the `palette-typography.md` SCHEMA (aspect → distilled
direction → source-site attribution) and the rule "all 4 sites cited, every row attributed."
v1 ships qualitative/schema-level direction (e.g. "rust, amber, or clay") rather than literal
hex codes / typeface names, since no persisted capture artifact is vendored in this repo to
verify exact values against; pinning concrete values is deferred to a future revision that
vendors actual site captures.

**Rationale.** Grounds the "palette feels arbitrary" risk in named evidence (spec requirement:
each entry cites ≥1 of the 4 sites, all 4 appear). Fixing literal values in the design doc
now, without a persisted capture to verify them against, would overclaim rigor the file
doesn't have; fixing the attribution schema and being explicit about the qualitative-only
scope keeps the artifact honest about its actual evidentiary status.

**Rejected — copy palettes verbatim from the sites.** Spec says "distilled, not copied";
verbatim lifting risks trademark/aesthetic cloning and produces a fifth clustered look rather
than a reusable direction.

### ADR-5 — Registration is a post-implementation operational step, not a hand-edit

**Decision.** After the directory exists, registration/indexing is performed by running
`gentle-ai skill-registry refresh --force` (equivalently the `/skill-registry` skill), which
scans `*/SKILL.md` on the filesystem and regenerates `.atl/skill-registry.md`. The SDD does
NOT hand-edit `skills.registry.yaml`.

**Rationale.** User correction (Engram #1985 + this session): the overlay's skill index is
command-generated from a filesystem scan, not manually curated. Treating registration as an
operational tail step keeps this change's *code/content* surface to exactly the new directory
and preserves the additive/rollback property (delete the directory, re-run refresh, gone).
This design agrees with spec.md's Requirement "Skill is indexed via skill-registry refresh,
not hand-edited YAML," which already specifies the refresh-command mechanism and prohibits
hand-editing `skills.registry.yaml`. (An earlier draft of this change used a hand-edit
approach; it was corrected to the refresh-command mechanism before this design was written,
and spec.md and design.md now agree — there is no remaining discrepancy to supersede.)

**Rejected — hand-editing `skills.registry.yaml` + manual `sync-manifest`.** Explicitly
overridden by the user; drift-prone and duplicates what the refresh command derives
automatically from the SKILL.md on disk.

## Relationship to frontend-design (complementary, not merged)

| | frontend-design (official, unmodified) | anti-generic-design (this skill) |
|---|---|---|
| Owner | anthropics upstream | labdrian overlay |
| Banned cluster | cream+serif+terracotta, near-black+acid-green, broadsheet-hairlines (external, unpinned/unvendored reference — verify upstream) | Inter/equiv, violet→blue gradient, generic shadow cards, flat 3-col grid |
| Method | two-pass token + self-critique | forbidden list + manual keyword self-critique |
| Positive source | its own token system | `references/palette-typography.md` (4 real sites) |
| Coupling | none — runs in background | none — runs alongside; names but never edits frontend-design |

## Risks & Alternatives

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Reader thinks it overlaps/replaces frontend-design | Med | ADR-2: Activation Contract names both disjoint ban sets; explicit complementarity statement |
| Palette reads as arbitrary / invented | Med | ADR-4: every row attributed to a named site; all 4 cited; values re-derived from real captures at apply |
| Scope creep into a validator | Low | ADR-3: v1 checklist is text-only; automation fenced to v2 in Non-Goals |
| Registration done the wrong (manual) way | Low | ADR-5: refresh-command step documented; no `skills.registry.yaml` hand-edit |
| Becomes a 5th clustered "look" | Low | Directions are ranges (near-mono + warm accent, asymmetric), not one fixed template; distilled not copied |

## Non-Goals (explicit)

- NO automated linter/script/CSS-rule validator (v2, out of scope).
- NO edit, merge, or replacement of `frontend-design`, `design-critique`, or `design-system`.
- NO engine/TUI/Go code, no new load mechanism.
- NO hand-edit of `skills.registry.yaml`; NO building the `labdrian-brain` design vault.
- NO verbatim copying of the 4 sites' assets — distilled directions only.

## Affected Files (edit points)

| File | Change |
|------|--------|
| `skills/anti-generic-design/SKILL.md` | NEW — frontmatter + activation contract + forbidden list + steer + inline self-critique checklist + references links |
| `skills/anti-generic-design/references/palette-typography.md` | NEW — attributed palette/type table, all 4 sites cited |
| `skills/anti-generic-design/references/example-output.md` | NEW — worked generic→distinctive pass |
| `skills/anti-generic-design/references/critique-template.md` | NEW — blank v1 self-critique checklist (manual) |
| `.atl/skill-registry.md` | REGENERATED (not hand-edited) via `gentle-ai skill-registry refresh --force`, post-implementation |
| `.gitignore` | MODIFIED — adds `.atl/`, since `.atl/skill-registry.md` is a regenerated artifact, not hand-maintained source |
