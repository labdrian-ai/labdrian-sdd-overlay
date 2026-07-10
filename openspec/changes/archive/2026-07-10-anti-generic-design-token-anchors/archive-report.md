# Archive Report: anti-generic-design-token-anchors

**Archived**: 2026-07-10
**Status**: CLOSED — clean verify, all spec deltas merged into main.

## What This Change Did

`skills/anti-generic-design/references/palette-typography.md` previously admitted an honest
rigor gap: its guidance was "informed by human recollection… NOT backed by any persisted
capture artifact," and deliberately used qualitative language ("rust, amber, or clay") rather
than concrete values, because pinning exact values without a verifiable capture would have
overclaimed rigor the file didn't have.

This change closed that gap with a first-party, human-verified capture instead of importing a
third-party corpus. A GADU dialectic evaluated vendoring `awesome-design-md` (73 MIT DESIGN.md
files) and rejected it: the corpus has confirmed, unfixed accuracy failures (typos, a
misattributed brand description) and a contribution model that blocks community fixes — a
closed pipeline with zero human proofreading. The dialectic instead adopted the *mechanism*
(a dimension-indexed, contrast-forcing anchor table) decoupled from that unreliable source.

Concretely, the change:

- Replaced the old aspect-indexed table (4 named award-gallery sites, human recollection, no
  capture artifact) with a **dimension-indexed anchor table** — one row per design dimension
  (canvas polarity, accent scarcity, corner-radius signature, depth mechanism, type philosophy)
  — each row citing 2–3 *contrasting* anchors drawn from 6 sector-leader sites (Vercel, Stripe,
  Ferrari, SpaceX, Nintendo, Aesop), each entry independently observed (view-source / rendered)
  and dated with a "last verified: <date>" stamp.
- Relocated the prior Sequencing/Layout/Voice rows — which are compositional, not
  single-rendered-value observations — into a fenced "Compositional signals (qualitative — not
  per-anchor captured)" subsection, preserving them without forcing them into the new capture
  discipline dishonestly.
- Framed every citation strictly as observational/factual CSS commentary ("this site's rendered
  CSS uses X"), never as a redistributed brand design system — the sole trademark-attribution
  mitigation, since sector-leader selection means most citations are recognizable brands.
- Added an **anti-mimicry checklist gate** — `[ ] token set is NOT >80% traceable to a single
  cited anchor → PASS` — as the 7th line of both `critique-template.md`'s fenced checklist and
  `SKILL.md`'s inline checklist, byte-identical in both files.
- **Validated the gate** against 3 real cases per `design.md`'s methodology: Case A (a
  purpose-built single-anchor mimicry reproduction of SpaceX's full kit) passed the original 6
  lines but failed line 7 — proving the gate is non-redundant, not just theoretically sound;
  Case B (`example-output.md`'s "After" section, mixing 4 sites) passed all 7; Case C (a fresh,
  independent generation attempt) surfaced a real, honestly-reported gap in the pre-existing
  (out-of-scope) checklist line 5, which was fixed rather than left as debt.

## Spec Merge

The delta spec at (formerly) `openspec/changes/anti-generic-design-token-anchors/specs/anti-generic-design/spec.md`
was merged into the main spec `openspec/specs/anti-generic-design/spec.md`:

- **R-002** (MODIFIED) — "Reference palette/typography grounded in real sites" — replaced in
  place with the dimension-indexed-table requirement, retiring the old 4-site aspect-table
  wording.
- **R-006** (ADDED) — "Sector-led capture sourcing method" — 5–6 fixed verticals, current
  market leaders, dated per-anchor observation.
- **R-007** (ADDED) — "Observational/factual-commentary framing as sole trademark mitigation."
- **R-008** (ADDED) — "Anti-mimicry checklist line" — identical line required in both
  `critique-template.md` and `SKILL.md`.
- **R-009** (ADDED) — "Anti-mimicry gate validated before close" — the gate must be run against
  2–3 real outputs with the outcome recorded before the change counts as complete.

All other requirements in the main spec (forbidden-pattern list, v1 manual heuristic, skill
directory structure, skill-registry indexing) were left untouched.

## Verification Verdict

Final `verify-report.md`: **CLEAN PASS (0 CRITICAL, 0 WARNING, 0 SUGGESTION)**. All 5
requirements (R-002, R-006–R-009) PASS against the shipped skill artifacts; both checklists
confirmed byte-identical via diff; task completeness 100% (25/25 checkboxes); scope confirmed
content-only — exactly 4 files under `skills/anti-generic-design/**` touched (164 changed
lines), no engine/Go, TUI, registry, or manifest file modified.

## Iterative Refinement History

This change went through multiple explicit refinement rounds before reaching a clean verdict,
per the user's explicit request to leave nothing pending before archive:

1. **Initial apply** (Phases 1–3): capture pass across 6 sector-leader sites, table
   restructure, checklist gate line added.
2. **Phase 4 validation** run separately (by explicit instruction to defer it), producing
   Cases A/B/C and surfacing a real gap in pre-existing checklist line 5 (stale "≤1 warm
   accent" wording falsified by Nintendo's legitimate 2-accent capture).
3. **Post-validation fix**: line 5 reworded from a fixed "≤1 warm accent" rule to
   "accent usage is scoped/deliberate, matching an observed range… (zero to a few, functional
   not decorative)," fixed rather than deferred, per explicit user instruction.
4. **First `sdd-verify` pass**: found a WARNING (sector tag only applied on each site's first
   mention in the table, not every mention) — fixed before re-verify.
5. **Second verify pass**: carried forward a non-blocking SUGGESTION (line 5's "zero to a few"
   had no hard upper bound) — closed by tightening to "zero to two… the max observed in the
   capture," grounded in the 3 anchors the Accent-scarcity dimension actually cites
   (SpaceX=0, Stripe=1, Nintendo=2).
6. **Final re-verify pass**: independently re-checked every claim against current file bytes
   (not accepted from prose), confirmed the prior WARNING resolved with no new issues,
   producing the clean 0/0/0 verdict recorded above.

Net effect: every warning and suggestion raised across the review cycle was resolved rather
than deferred, consistent with the user's explicit instruction to close this change out
completely before archiving.
