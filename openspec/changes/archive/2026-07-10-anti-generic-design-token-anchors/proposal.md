# Proposal: anti-generic-design-token-anchors

## Intent

`skills/anti-generic-design/references/palette-typography.md` openly admits a rigor gap: it is
"a SCHEMA + qualitative direction guide, informed by human recollection… NOT backed by any
persisted capture artifact," and deliberately ships qualitative language ("rust, amber, or
clay") instead of concrete values "because pinning exact values without a verifiable capture
would overclaim rigor this file doesn't have." That honesty is correct, but the gap is real:
a coding agent following the skill has no observed, re-checkable anchors to steer by.

This change closes that gap with OUR OWN small, human-verified capture — not by importing a
third-party corpus. A GADU dialectic (builder-advocate vs skeptic-operator, both grounded by
actually fetching and reading the source) evaluated vendoring VoltAgent/awesome-design-md (73
MIT DESIGN.md files) and **REJECTED** it: the corpus has confirmed, unfixed accuracy failures
(`Stripi`/`Supabaze` typos in shipped front-matter; issue #421 reports Notion's file describes
"a different company… looks like Ramp", never corrected), and its `CONTRIBUTING.md` blocks
community PRs from fixing DESIGN.md content — a closed, commercially-funneled generation
pipeline with zero human proofreading. Vendoring it would launder unverified (and
demonstrably wrong) AI output as "real brand data," trading an honestly-labeled rigor gap for
a false one.

What the dialectic **ADOPTED** is the advocate's *mechanism*, decoupled from the bad source: a
**dimension-indexed, contrast-forcing anchor table** plus an **anti-mimicry checklist gate**.
Instead of citing one brand's full token kit (which an agent would faithfully reproduce —
defeating the "distinctive, non-homogenized" goal), reference content is structured as ONE ROW
PER DESIGN DIMENSION, each row citing 2–3 CONTRASTING anchors that visibly disagree, so no
single "average" direction emerges. Success looks like: a real, dated, re-checkable capture
backing each dimension, and a validated gate that catches single-brand mimicry in practice —
raising confidence from the dialectic's MEDIUM-HIGH to HIGH before shipping.

## Scope

### In Scope
- **Sector-led capture methodology, built to scale.** Select a fixed table of distinct
  industry verticals (e.g. dev-tools/SaaS, fintech, e-commerce/retail, automotive,
  aerospace/frontier-tech, consumer electronics/entertainment — 5–6 to start), and for each
  vertical identify the CURRENT market leader: the site's present, live, up-to-date state, not
  a stale or superseded rebrand. Sector-forced diversity does the contrast work that
  obscure/recognizable mixing previously did — different verticals converge on genuinely
  different visual codes by default, so no artificial obscurity requirement is needed.
  A human/agent actually opens each site and records ONLY directly observed values (rendered
  hex, computed `font-family`/weight/size, actual `border-radius`, actual shadow/no-shadow
  treatment), each entry dated and framed as observational/factual CSS commentary (not a
  redistributed brand system) — this framing is the sole trademark-attribution mitigation now
  that obscure-site mixing is dropped. The table is additive: a new vertical is one new row,
  no restructuring; each row carries a "last verified: <date>" stamp so staleness is visible
  and re-verification is a scoped, incremental task — this is the sustainability answer to the
  exact failure mode (stale, unrefreshable, no-correction-path data) that disqualified
  awesome-design-md.
- **Dimension-indexed anchor table** (new section in, or restructuring of,
  `palette-typography.md`): one row per design dimension — canvas polarity, accent scarcity,
  corner-radius signature, depth mechanism, type philosophy — each row citing 2–3 *contrasting*
  anchors that visibly disagree with each other.
- **New anti-mimicry checklist line** added to `critique-template.md` (and mirrored in
  `SKILL.md`'s inline checklist):
  `[ ] token set is NOT >80% traceable to a single cited anchor → PASS`.
- **Validation before close**: run the new checklist against 2–3 real generated design outputs
  (the existing `example-output.md` before/after scenario plus 1–2 fresh generation attempts)
  to confirm the gate catches single-brand mimicry in practice, not just in theory.

### Out of Scope
- Importing / vendoring awesome-design-md or ANY third-party DESIGN.md corpus (explicitly
  rejected by the dialectic).
- Expanding into general design-system / token-library tooling — elevation systems,
  breakpoints, full type hierarchies. This stays a self-critique steering skill, not a brand
  or token library. No new category of skill is created.
- Automating the checklist. It remains v1 manual, per the skill's stated deferral of automation
  to a future v2.
- Any engine/Go, TUI, or runtime-wiring change. This is content-only under
  `skills/anti-generic-design/**`, distinct from the already-shipped runtime-wiring mechanism
  for this skill.

## Capabilities

### New Capabilities
None. No new skill or command surface is introduced.

### Modified Capabilities
- `anti-generic-design`: its reference content gains a first-party, dated, re-checkable
  dimension-indexed anchor table and an anti-mimicry checklist gate, closing the "no verifiable
  capture" gap the current `palette-typography.md` admits to.

## Approach

Extend the existing three reference files in place — do NOT add a new file category.

1. **Capture pass.** Fix a table of 5–6 distinct industry verticals; for each, identify and
   visit the CURRENT market leader's live site. Record only directly observed, rendered values
   via devtools computed-styles / view-source. Note the method and a "last verified" date
   inline per row so any reader can re-verify, and so re-verification later is a per-row,
   incremental task rather than a full recapture.
2. **Restructure `palette-typography.md`** around design DIMENSIONS rather than one direction
   per aspect. Each dimension row cites 2–3 contrasting anchors so the guidance forces contrast
   instead of converging on an average look. Frame every citation as *observational/factual
   commentary* ("this site's rendered CSS uses X"), never as a redistributed "brand design
   system." Update the attribution note to reflect the now-real capture (and retire the
   "human recollection / no capture artifact" caveat that no longer applies).
3. **Add the anti-mimicry line** to `critique-template.md` and the inline checklist in
   `SKILL.md`, keeping wording and format consistent with the existing six checklist lines.
4. **Validate the gate** against 2–3 real outputs and record the outcome, confirming it flags
   single-anchor mimicry before this change is considered done.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `skills/anti-generic-design/references/palette-typography.md` | Modified | Add dimension-indexed contrast-forcing anchor table from first-party dated capture; update/retire the "no capture artifact" note |
| `skills/anti-generic-design/references/critique-template.md` | Modified | Add `[ ] token set is NOT >80% traceable to a single cited anchor → PASS` |
| `skills/anti-generic-design/SKILL.md` | Modified | Mirror the new checklist line; point "Steer toward"/References at the dimension table |
| `skills/anti-generic-design/references/example-output.md` | Modified (optional) | May serve as one of the 2–3 validation cases for the new gate |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Naming real/recognizable market-leader brands carries trademark-attribution weight (sector-leader selection means almost all citations are recognizable, no obscure-site buffer) | Med | Sole mitigation: every citation is framed strictly as observational/factual CSS commentary ("this site's rendered CSS uses X"), never as a redistributed brand design system |
| The anti-mimicry gate might not actually catch mimicry in practice | Med | Explicit in-scope validation task: run the gate against 2–3 real outputs before closing; the change is not "done" until this passes |
| Fresh capture costs real effort vs. cheap third-party data | Med (accepted) | Deliberate tradeoff: a small honest verifiable capture beats a large demonstrably-wrong corpus; capture is intentionally bounded to 5–6 sector-leader sites |
| An agent reproduces one anchor faithfully, re-homogenizing output | Med | Contrast-forcing table (2–3 disagreeing anchors per dimension, forced by sector diversity) + the >80%-single-anchor gate are jointly designed to prevent this |
| Scope creep into a token/design-system library | Low | Out-of-scope fence: extend existing 3 files only; no elevation/breakpoint/type-hierarchy tooling |
| Captured sites redesign over time; the capture goes stale with no correction path — the exact failure mode that disqualified awesome-design-md | Med | Each row carries a "last verified: <date>" stamp; re-verification is scoped per-row, not a full recapture; the table structure (one row per vertical) makes staleness locally visible and spot-checkable instead of buried in a 73-file opaque corpus |

## Rollback Plan

Revert the change commits/PR. All edits are additive content within
`skills/anti-generic-design/**`; the prior `palette-typography.md`, `critique-template.md`, and
`SKILL.md` remain valid on revert. No engine, registry, manifest, TUI, or runtime-wiring state
is touched, so removal leaves the overlay fully valid.

## Dependencies

- The existing shipped `anti-generic-design` skill (the three reference files this change
  extends). No dependency on awesome-design-md or any external corpus — that source is
  explicitly rejected.
- The completed GADU dialectic (verdict: reject the source, adopt the mechanism) as the
  grounding rationale.

## Success Criteria

- [ ] `palette-typography.md` carries a dimension-indexed table with one row per dimension
      (canvas polarity, accent scarcity, corner-radius signature, depth mechanism, type
      philosophy), each citing 2–3 *contrasting* anchors sourced from a fixed table of 5–6
      distinct industry verticals' CURRENT market leaders.
- [ ] Every anchor value is a directly observed, rendered value from a live site, with the
      observation method and a "last verified" date documented inline per row so it is
      independently re-checkable and incrementally re-verifiable without a full recapture.
- [ ] Citations are framed strictly as observational/factual CSS commentary, never as a
      redistributed brand design system — this is the sole trademark-attribution mitigation.
- [ ] `critique-template.md` and `SKILL.md` both include
      `[ ] token set is NOT >80% traceable to a single cited anchor → PASS`.
- [ ] The new gate is validated against 2–3 real generated outputs and demonstrably flags
      single-brand mimicry (outcome recorded), raising confidence to HIGH.
- [ ] The old "no verifiable capture / human recollection" caveat is updated or retired to
      match the now-real capture.
- [ ] No engine/Go, TUI, registry, manifest, or runtime-wiring file is modified; change stays
      within `skills/anti-generic-design/**` and is fully removable.
