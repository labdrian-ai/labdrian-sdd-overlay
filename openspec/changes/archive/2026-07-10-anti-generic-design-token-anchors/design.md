# Design: anti-generic-design-token-anchors

## Technical Approach

Content-only change: no engine/Go, TUI, or runtime-wiring. All edits are confined to three
existing files under `skills/anti-generic-design/` (plus the change's `apply-progress.md`).
The "design" here is **document structure + capture methodology**, not software architecture.
The strategy maps 1:1 to the proposal's Approach: (1) restructure `palette-typography.md` from
an aspect-indexed qualitative table into a **dimension-indexed, first-party dated capture**;
(2) append one anti-mimicry gate line to `critique-template.md` and `SKILL.md`; (3) validate
the gate against real outputs. Capture VALUES are produced in apply — design fixes only the
structure/methodology those values must follow.

## Content Structure Decisions

### Decision: Restructure `palette-typography.md` in place (not additive)

| Option | Tradeoff | Decision |
|--------|----------|----------|
| (a) Replace aspect table with the 5-row dimension table | Loses the 3 compositional rows unless relocated | **CHOSEN** (with a fenced note for the 3) |
| (b) Add dimension table ALONGSIDE the old table | Two structures that duplicate Base/Accent/Typography and can drift; violates the "no bolt-on / no new file category" fence | Rejected |
| (c) Replace and drop Sequencing/Layout/Voice entirely | `SKILL.md` "Steer toward" and the "editorial/asymmetric signal" checklist line depend on them → information loss | Rejected |

**Verified against the current file**: of the 6 existing rows, only **Base palette → canvas
polarity**, **Accent → accent scarcity**, and **Typography → type philosophy** map onto a
directly-observable rendered value. `corner-radius signature` and `depth mechanism` are NEW
observable dimensions. **Sequencing, Layout, Voice** are compositional — not a single rendered
hex/px/font value — so they CANNOT honestly join a "directly observed value" capture.
**Rationale**: restructure in place — replace the aspect table with the 5-row dimension table,
and relocate Sequencing/Layout/Voice into a compact fenced subsection
`## Compositional signals (qualitative — not per-anchor captured)`, explicitly outside the
dated capture. One coherent table, no drift, no info loss, honesty preserved (only
single-value-observable dimensions are anchored).

### Decision: Six sector verticals, chosen for maximal cross-dimension contrast

| Sector (current market leader — pick in apply) | Canvas | Radius | Depth | Type | Accent |
|---|---|---|---|---|---|
| Dev-tools / developer platform | dark, technical | tight | border / flat | mono / grotesque | restrained |
| Fintech / payments | light, trust | medium | soft / avoided | geometric sans | brand, moderate |
| Luxury automotive | cinematic black or stark white | near-zero (sharp) | full-bleed imagery | wide-track serif/grotesque | metallic, minimal |
| Aerospace / frontier-tech | stark black | zero | flat | high-contrast minimal | near-zero (mono) |
| Consumer electronics / entertainment | high-key or deep-black | large (friendly) | gradient / imagery | bold display | vivid |
| Editorial retail / e-commerce | warm paper | moderate | flat / hairline | editorial serif | single warm |

**Rationale**: these sectors were chosen because their brand semiotics pull in OPPOSITE
directions on each axis (dark-technical vs light-trust vs cinematic-luxury vs stark-frontier vs
playful-vivid vs warm-editorial). Contrast is **structurally guaranteed by sector opposition**,
not hand-curated. Each dimension row cites the **2–3 sectors that most disagree on that axis**
(not all 6 per row). Apply MAY start at 5 if a sector's current leader is contested; the table
is additive — a 6th vertical is one appended anchor, no restructure.

### Decision: Capture recording format — inline per-anchor, dual-level dating

Each anchor is recorded **inline in the table cell** (the cell IS the ledger — no separate
capture file, honoring "no new file category"):

`Site (sector): <observed value> — <method>, verified <YYYY-MM-DD>`

- **observed value**: the concrete rendered token — hex; computed `font-family`/weight/size;
  `border-radius` px; shadow spec or `flat`/`border`.
- **method** ∈ `computed-style` | `view-source` | `rendered`.
- **verified**: per-ANCHOR date (re-verification is per-site, incremental).
- Each dimension ROW also carries a derived `oldest-verified: <date>` so row-level staleness is
  visible at a glance — satisfies the proposal's "last verified per row" stamp WHILE keeping
  per-site precision.

### Decision: Anti-mimicry gate — wording and placement

Exact line (wording fixed by the proposal):

`[ ] token set is NOT >80% traceable to a single cited anchor → PASS`

Appended as the **final (7th) line** of BOTH the fenced checklist in `critique-template.md` and
the inline `## Self-critique checklist` in `SKILL.md`. Re-pad the block so the `→ PASS` column
stays aligned with the existing six lines. It sits **last** as the provenance/diversity
capstone: a distinct gate from the six pattern checks — it catches *distinctive-but-mimicked*
output that all six otherwise PASS.

## Data Flow (methodology)

    apply: pick 5-6 sector leaders ─→ open each live ─→ record ONLY rendered values
              │                                              │
              ▼                                              ▼
       5-row dimension table  ◀── 2-3 CONTRASTING anchors per dimension
              │
              ├─→ SKILL.md "Steer toward" + References point here
              └─→ critique-template.md + SKILL.md checklist ─→ gate line 7
                        │
                        ▼
       validation: run 7-line checklist vs 3 outputs ─→ record in apply-progress.md

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `skills/anti-generic-design/references/palette-typography.md` | Modify | Replace aspect table with 5-row dimension table (dated per-anchor capture); relocate Sequencing/Layout/Voice to a fenced qualitative "Compositional signals" note; rewrite the Attribution note — retire the "human recollection / no capture artifact" caveat, replace with the dated first-party capture framing + observational-CSS-commentary trademark mitigation |
| `skills/anti-generic-design/references/critique-template.md` | Modify | Append gate line 7; re-pad `→ PASS` alignment |
| `skills/anti-generic-design/SKILL.md` | Modify | Append gate line 7 to the inline checklist; update "Distilled from 4 real award-gallery sites" + the References bullet to the sector-leader dimension capture; reconcile "Steer toward" bullets with the 5 dimensions (values live in the table) |
| `skills/anti-generic-design/references/example-output.md` | Modify (recommended) | Re-point the "After" citations to new anchors; serves as validation case B |
| `openspec/changes/anti-generic-design-token-anchors/apply-progress.md` | Create/append | Validation subsection recording the gate outcome |

## Validation Methodology

Run the 7-line checklist against **3 cases**:

- **A — Mimicry case (purpose-built)**: a generated output that faithfully reproduces ONE
  anchor's full kit (one sector leader's canvas + radius + depth + font). Expected: **PASS the
  original 6** (it is distinctive), **FAIL line 7**. This is the decisive test.
- **B — `example-output.md` "After"**: draws from multiple contrasting anchors. Expected:
  PASS all 7.
- **C — Fresh generation attempt**: Expected PASS all 7, or surface a real gap.

**Pass/fail for the validation task**: validation PASSES iff line 7 flags ≥1 output (case A)
that all six pre-existing lines PASS — proving the gate catches single-anchor mimicry the old
checklist misses. If every line-7 failure coincides with an earlier-line failure, the gate is
redundant → validation FAILS, rework required. **Outcome recorded inline** in
`openspec/changes/anti-generic-design-token-anchors/apply-progress.md` (no new file).

## Migration / Rollout

No migration. Additive/reversible content; reverting the commit restores the prior three files
verbatim (see proposal Rollback Plan). No engine/registry/manifest/TUI state touched.

## Open Questions

- [ ] None blocking. Apply MAY reduce 6→5 verticals if a sector's current leader is contested —
      a capture-time call, not a design blocker.
