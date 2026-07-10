# Apply Progress: anti-generic-design-token-anchors

**Mode**: Standard (content-only change; `tasks.md` explicitly states "No automated tests" —
this is a documentation/skill-content change, not Go code, so the repo's global
`strict_tdd: true` / `go test` configuration in `openspec/config.yaml` does not apply here.
Verify-before-done discipline (spot-checking each capture entry, and the Phase 4 validation
run) is the stated substitute for unit tests per `tasks.md`'s own framing.)

## Completed Tasks (Phases 1-3)

### Phase 1: Capture Pass (R-002, R-006, R-007) — [x] all 8 tasks

Capture was performed independently (static curl/view-source, no browser, no recollection) for
6 sector-leader sites, all dated 2026-07-10 except Aesop:

1. **Vercel** (dev-tools/developer-platform) — view-source, verified 2026-07-10
2. **Stripe** (fintech/payments) — view-source, verified 2026-07-10
3. **Ferrari** (luxury automotive) — view-source, verified 2026-07-10, **PARTIAL/LIMITED**
4. **SpaceX** (aerospace/frontier-tech) — view-source, verified 2026-07-10
5. **Nintendo** (consumer electronics/entertainment) — view-source, verified 2026-07-10
6. **Aesop** (editorial retail/e-commerce) — **verified 2026-06-06 (via Wayback Machine
   archive; live fetch blocked by Cloudflare 2026-07-10)**

All 6 recorded entries were spot-checked against the source data before being written into the
dimension table (task 1.8 discipline).

**Caveats preserved verbatim (not softened):**

- **Ferrari**: "this site uses Web Components/Shadow DOM (`:host,:root`, `:not(:defined)`
  selectors) — component-level styles likely load via JS-injected Shadow DOM stylesheets NOT
  visible to static fetch. Several values are confirmed as DEFINED TOKENS but NOT confirmed as
  actually applied to a visible button/card — this must be stated explicitly in the file, not
  smoothed over." Concretely: accent hue, `--f-radius-full:9999px`, and
  `--f-shadow-small:0px 4px 8px 0px rgba(0,0,0,.1)` all exist as tokens with **zero** literal
  `border-radius:`/`box-shadow:` properties found applied anywhere in the fetched CSS. Ferrari
  is therefore cited in the dimension table ONLY for Canvas polarity and Type philosophy
  (directly confirmed from non-component CSS), and deliberately NOT cited for Accent scarcity,
  Corner-radius, or Depth mechanism, where its data is unconfirmed.
- **Aesop**: "live site returned HTTP 403 (Cloudflare bot-challenge) on every attempt today.
  Substitute: a first-party-served Wayback Machine snapshot of aesop.com's own CDN-served
  HTML/CSS, ARCHIVED 2026-06-06 ... this is genuine Aesop-served bytes, just dated 5 weeks
  before the other 5 captures, not a same-day live capture." This is stated explicitly in
  `palette-typography.md`'s Capture caveats subsection and in the dimension-table cells that
  cite Aesop (dated 2026-06-06, not 2026-07-10).

**Honest cross-site finding (preserved, not forced to match the prior hypothesis):** 4 of the 6
captured sites (Vercel, Stripe, Ferrari, Nintendo) came back light/white canvas rather than the
design's a-priori hypothesis of a more even dark/light split — only SpaceX confirmed a
genuinely dark/black canvas, and Aesop landed on warm-cream (distinct from pure white). This
updates the design's hypothesis rather than being smoothed over. Despite this skew, the 6 sites
still do NOT converge on a single "average" direction across the 5 dimensions jointly — no two
sites agree on all 5 simultaneously (verified by cross-checking the pairwise table: canvas
polarity alone splits the white-canvas cluster from SpaceX/Aesop, and within that white-canvas
cluster — Vercel, Stripe, Ferrari, Nintendo — every pair still disagrees on at least one of
accent scarcity, corner-radius, depth mechanism, or type philosophy).

### Phase 2: Restructure `palette-typography.md` (R-002) — [x] all 4 tasks

- Replaced the old 6-row aspect table with a 5-row dimension table (canvas polarity, accent
  scarcity, corner-radius signature, depth mechanism, type philosophy), each row citing 2-3
  contrasting anchors in the exact format
  `Site (sector): <observed value> — <method>, verified <YYYY-MM-DD>`.
- Relocated Sequencing/Layout/Voice verbatim into a new fenced
  `## Compositional signals (qualitative — not per-anchor captured)` subsection, explicitly
  outside the dated capture, still citing the original 4 exploratory sites (depoluxe.xyz,
  bymonolog.com, collection.industries, useportal.net) unchanged.
- Added a derived `oldest-verified: <date>` per dimension row.
- Rewrote the Attribution note: retired the "human recollection / no capture artifact" caveat;
  replaced with the dated first-party capture description and the
  observational/factual-CSS-commentary framing as the sole trademark-attribution mitigation
  (per spec.md R-007 — citations state what was observed, never imply a redistributed brand
  design system).

### Phase 3: Checklist Gate (R-008) — [x] all 3 tasks

- Appended `[ ] token set is NOT >80% traceable to a single cited anchor → PASS` as the 7th
  line of `critique-template.md`'s fenced checklist, re-padded so `→ PASS` aligns at column 76
  (matching lines 10-13's existing alignment; note line 8's `→` sits at column 73 and line 9's
  at column 75 in the pre-existing checklist — the block was not perfectly column-aligned
  before this change either, so the new line matches the block's majority alignment).
- Appended the identical line to `SKILL.md`'s inline checklist, same re-padding.
- Updated `SKILL.md`'s "Steer toward" bullets to steer per-dimension using the contrasting
  anchors (rather than asserting one universal "distilled" direction), and updated the
  References section to cite the 6 sector-leader sites instead of "4 real award-gallery
  sites" (that framing is now obsolete for the per-anchor dimension table, though it remains
  accurate — and unchanged — for the relocated Compositional-signals subsection).

### Additional work done now (prepares Phase 4, not yet executed as Phase 4)

- Updated `example-output.md`'s "After" section to cite the new sector-leader anchors, mixing
  4 different sites (Aesop, Stripe, Vercel, SpaceX) across the 5 dimensions plus the unchanged
  compositional-signal citations — this file edit corresponds to task 4.3's content but **task
  4.3's checkbox is intentionally left unchecked**, per explicit instruction to stop after
  Phases 1-3 and leave Phase 4/5 unchecked until the validation run itself (4.1, 4.2, 4.4-4.6)
  is actually executed.

## NOT Done (Phase 4 and Phase 5 — intentionally deferred)

- [ ] Phase 4 (Validation — R-009): Case A (purpose-built single-anchor mimicry test), running
  the 7-line checklist against Cases A/B/C, and recording outcomes has **not** been run. This is
  the decisive test that the gate actually flags mimicry (per R-009, the change is not
  considered complete until this validation runs and its outcome is recorded) — deferred to a
  separate next step, per explicit instruction.
- [ ] Phase 5 (Close-out): Success Criteria verification and final scope-fence confirmation not
  yet performed — depends on Phase 4.

## Files Changed

| File | Action | What Was Done |
|------|--------|----------------|
| `skills/anti-generic-design/references/palette-typography.md` | Rewritten | 5-row dimension table with first-party dated capture; capture caveats subsection; relocated compositional signals; rewritten attribution note |
| `skills/anti-generic-design/references/critique-template.md` | Modified | Appended 7th anti-mimicry checklist line, re-padded |
| `skills/anti-generic-design/SKILL.md` | Modified | Appended 7th checklist line; rewrote "Steer toward" bullets; updated References section |
| `skills/anti-generic-design/references/example-output.md` | Modified | Re-pointed "After" citations to new sector anchors, mixing 4 sites across 5 dimensions (prepares Case B for Phase 4, not yet run) |
| `openspec/changes/anti-generic-design-token-anchors/tasks.md` | Modified | Marked Phase 1 (1.1-1.8), Phase 2 (2.1-2.4), Phase 3 (3.1-3.3) as `[x]`; Phase 4/5 left unchecked |
| `openspec/changes/anti-generic-design-token-anchors/apply-progress.md` | Created | This file |

## Deviations from Design

None — implementation matches design.md's decisions (restructure-in-place, 6-vertical table,
inline dual-level dating format, 7th-line gate placement and wording).

## Issues Found

None beyond the Ferrari Shadow-DOM and Aesop Cloudflare/Wayback limitations, both of which were
anticipated by the capture data itself and are documented (not smoothed over) in both
`palette-typography.md`'s Capture caveats subsection and this file.

## Status (superseded by Phase 4/5 sections below — kept for history)

Phases 1-3 complete (all corresponding tasks.md checkboxes marked `[x]`). Phase 4 (validation)
and Phase 5 (close-out) remain, by explicit instruction, for a separate next apply batch. Not
ready for `sdd-verify` yet — R-009 requires the validation run to exist before the change is
considered complete.

---

## Phase 4: Validation — the RED→GREEN analog (R-009)

Ran the current 7-line checklist (6 original lines + the new anti-mimicry line 7) against 3
cases, by hand, per `design.md`'s Validation Methodology. All 4 current files
(`palette-typography.md`, `critique-template.md`, `SKILL.md`, `example-output.md`) were
re-read fresh before constructing and grading these cases.

### Case A — decisive purpose-built mimicry case

**Construction**: a hypothetical landing page description that faithfully reproduces ONE
single anchor's FULL kit across all 5 dimensions — SpaceX (aerospace/frontier-tech) was
chosen because its zero-accent, flat/border-only, self-hosted-uppercase-type kit is the
cleanest full-dimension single-source reproduction to construct (contrast with e.g. Stripe,
whose real capture data is a single hue that isn't unambiguously "warm," which would make
grading line 5 noisier — SpaceX's zero-accent case is cleanly gradable):

> A landing page hero and feature section:
> - **Canvas**: near-pure black `#000` background throughout; body text is a cool-tinted
>   off-white `rgba(240,240,250,1)`, not pure white.
> - **Accent**: zero brand-color accents anywhere in the UI — every CTA/button/icon is
>   grayscale; the only color on the page comes from a photographic hero background.
> - **Corner-radius**: feature cards have NO border-radius (sharp/0). A small status badge
>   uses 4px. Circular icon buttons use a full 2rem (32px) pill/circle. Primary nav/header
>   buttons are text-only, no radius at all.
> - **Depth**: flat, bordered — zero `box-shadow` anywhere; elevation reads via a translucent
>   `border-color` on cards/panels instead.
> - **Typography**: a custom self-hosted uppercase display font (400 regular + separate Bold
>   family), pervasive uppercase text-transform with wide letter-spacing (0.09em on bold nav
>   labels, 1px uppercase on regular titles) — an "engineering readout" voice throughout.
> - *(Layout/voice, added generically — NOT sourced from SpaceX, since these are compositional
>   signals, not per-anchor dimensions)*: numbered indexed sections (`01`, `02`, `03`) with
>   asymmetric block widths; first-person copy ("We built this because our own team needed a
>   faster way to ship.").

**7-line checklist result:**

| # | Line | Result | Reasoning |
|---|------|--------|-----------|
| 1 | font NOT Inter/Roboto/system sans | **PASS** | custom self-hosted uppercase display font, not Inter/Roboto/system default |
| 2 | no violet→blue gradient | **PASS** | no gradient present at all; achromatic |
| 3 | cards not generic soft box-shadow alone | **PASS** | zero box-shadow anywhere; flat + translucent border |
| 4 | layout not flat 3-col grid | **PASS** | numbered, asymmetric block widths |
| 5 | palette near-monochrome + ≤1 warm accent | **PASS** (see note) | pure monochrome, zero accents — "0 ≤ 1" is vacuously satisfied; there is no accent to judge for warmth. Noted honestly: this is the strongest/degenerate form of the line, not a designed single-warm-accent case — a stricter future rewording of line 5 (e.g. requiring exactly 1 accent) would change this. As currently worded, it PASSES. |
| 6 | at least one editorial/asymmetric signal | **PASS** | numbered indexed sections qualify |
| 7 | token set NOT >80% traceable to single cited anchor | **FAIL** | all 5 core dimensions (canvas, accent, radius, depth, type) are reproduced 100% from SpaceX alone — the only non-SpaceX elements (layout/voice) are compositional signals, outside the "token set" the line is checking. 5/5 = 100% > 80% threshold. |

**Result: PASSES the original 6 lines, FAILS line 7 — exactly the expected decisive result.**
This is NOT a forced outcome — line 5 required an honest judgment call (documented above) but
did not require bending any other line to make this case work.

### Case B — `example-output.md`'s current "After" section

Already updated in Phase 2/3 work to mix Aesop (canvas), Stripe's accent-scarcity pattern
(accent, reinterpreted hue), Aesop+Vercel (corner-radius, dual convention), SpaceX (depth), and
a multi-site self-hosted-font pattern citing Ferrari/Nintendo/SpaceX (type philosophy) — plus
the unchanged compositional-signal citations (collection.industries, useportal.net,
bymonolog.com) for layout/voice.

**7-line checklist result:**

| # | Line | Result | Reasoning |
|---|------|--------|-----------|
| 1 | font NOT Inter/Roboto/system sans | **PASS** | "oversized editorial display face" + "restrained, self-hosted body face (never Inter/Roboto/system sans)" |
| 2 | no violet→blue gradient | **PASS** | warm cream ground, single warm accent, no gradient |
| 3 | cards not generic soft box-shadow alone | **PASS** | "flat, hairline-bordered cards with zero box-shadow" |
| 4 | layout not flat 3-col grid | **PASS** | numbered asymmetric sequence (`01`/`02`/`03`), varied block widths |
| 5 | palette near-monochrome + ≤1 warm accent | **PASS** | warm cream ground + exactly one warm accent, used only on the CTA |
| 6 | at least one editorial/asymmetric signal | **PASS** | numbered sequencing + first-person copy |
| 7 | token set NOT >80% traceable to single cited anchor | **PASS** | of the 5 dimensions, the single most-cited site (Aesop) informs at most 2 of 5 (canvas fully, corner-radius shared with Vercel) — max single-site share ≈ 40-50%, well under the 80% threshold; type philosophy explicitly cites a 3-site pattern, not one site |

**Result: PASSES all 7 lines, as expected.**

### Case C — fresh, independent generation attempt

**Construction**: a plausible design a coding agent might produce while following the skill —
NOT built to mimic one anchor, and deliberately different from Case B's specific mix:

> A landing page for a hypothetical dev tool:
> - **Canvas**: white `#fff`, light theme only — inspired by the dev-tools/Vercel default.
> - **Accent**: two accents used sparingly — a primary teal for CTAs/links and a secondary
>   muted orange for warning/secondary states — inspired by Nintendo's dual-accent convention,
>   reinterpreted in different hues.
> - **Corner-radius**: a pill-shaped primary CTA (Vercel-inspired) paired with zero-radius,
>   sharp feature cards (Aesop/SpaceX-inspired sharp-card pattern) — a deliberate dual
>   convention, not one uniform radius.
> - **Depth**: cards are flat with a 1px hairline border by default; a soft shadow appears ONLY
>   on dropdown menus/modals, never on page content — Vercel's hairline-border-plus-
>   overlay-only-shadow pattern.
> - **Typography**: a distinct self-hosted variable-weight sans (weight axis 100-700, NOT
>   Stripe's actual "sohne-var" face) used at a LIGHT weight (300) for oversized headlines
>   with slightly negative tracking, and a bolder weight (600) for UI labels — pattern
>   inspired by Stripe's light-large-heading treatment.
> - Numbered indexed sections (`01`, `02`, `03`) with offset, asymmetric block widths;
>   first-person copy ("We built this tool because switching between five dashboards was
>   driving our own team crazy.").

**7-line checklist result:**

| # | Line | Result | Reasoning |
|---|------|--------|-----------|
| 1 | font NOT Inter/Roboto/system sans | **PASS** | distinct self-hosted variable sans, not Inter/Roboto/system default |
| 2 | no violet→blue gradient | **PASS** | no gradient present |
| 3 | cards not generic soft box-shadow alone | **PASS** | flat hairline border by default; shadow confined to overlays only |
| 4 | layout not flat 3-col grid | **PASS** | numbered, asymmetric block widths |
| 5 | palette near-monochrome + ≤1 warm accent | **FAIL — real gap, reported honestly, not forced** | this design legitimately uses TWO accent colors (teal + muted orange), directly modeled on Nintendo's real, captured, evidence-based dual-accent pattern from the new dimension table — but checklist line 5's wording was never updated in this change (only line 7 was added, per the proposal's explicit scope) and still hard-codes "≤1 warm accent," a direction inherited from the OLD single-accent capture. As literally worded, a design citing Nintendo's legitimate two-accent anchor fails line 5, even though it is directly evidence-based. |
| 6 | at least one editorial/asymmetric signal | **PASS** | numbered sequencing + first-person copy |
| 7 | token set NOT >80% traceable to single cited anchor | **PASS** | Vercel informs canvas + depth fully and shares corner-radius with Aesop/SpaceX (≈2.5/5 ≈ 50%); accent (Nintendo-pattern) and typography (Stripe-pattern) are separately sourced — no single site exceeds ~50% share, under the 80% threshold |

**Result: FAILS line 5 (a real, honestly-reported gap), PASSES the other 6 lines including
line 7.** This is reported as-is, not smoothed over, per instruction. It is a genuine finding
about the CURRENT checklist, not a defect in the anti-mimicry gate itself.

### Validation task verdict (per `design.md`'s stated pass/fail rule)

> "Validation PASSES iff line 7 flags Case A (which passes the original 6) — proving the gate
> catches single-anchor mimicry the old six-line checklist would have missed. If every line-7
> failure coincides with an earlier-line failure, the gate is redundant → validation FAILS."

Case A passes ALL 6 original lines and is caught ONLY by line 7. That is the exact
non-redundancy condition design.md requires.

**VALIDATION TASK: PASSED.** The anti-mimicry gate (line 7) demonstrably catches single-anchor
mimicry (Case A) that the pre-existing 6-line checklist alone would have let through clean.
R-009 is satisfied: at least one output (Case A) is correctly flagged, and the outcome is
recorded here.

### Side-finding surfaced during validation (not part of the R-009 gate, reported for
transparency)

Checklist line 5 ("palette is near-monochrome + ≤1 warm accent") was NOT in scope for this
change (the proposal/design/tasks only specify adding line 7; lines 1-6 keep their original
wording). Running real cases against the now-authoritative 6-site dimension table surfaced
that line 5's "≤1 warm accent" wording is stale relative to the new capture: Nintendo's real,
directly-observed anchor legitimately has TWO accents (red + blue), and SpaceX's real anchor
has ZERO (and isn't "warm" in the conventional sense either). A design faithfully following
either of those real, cited anchors can fail literal line 5 wording even though it is
evidence-based. This is flagged as a recommendation for a FUTURE revision of line 5's wording
(e.g. "palette scarcity matches one of the cited accent-scarcity anchors" rather than a fixed
"≤1 warm" rule) — it does NOT block this change's completion, since line 5 was explicitly out
of scope here and R-009 only requires line 7 to be validated.

## Phase 5: Close-out

Re-read the current state of all 4 affected files plus this apply-progress.md before checking
each Success Criterion from `proposal.md`:

- [x] **"`palette-typography.md` carries a dimension-indexed table with one row per dimension
      ... each citing 2–3 contrasting anchors sourced from a fixed table of 5–6 ... market
      leaders."** MET — 5 rows (canvas polarity, accent scarcity, corner-radius signature,
      depth mechanism, type philosophy), each citing exactly 3 anchors, drawn from the 6-site
      fixed table.
- [x] **"Every anchor value is a directly observed, rendered value ... with the observation
      method and a 'last verified' date documented inline per row ... re-verifiable without a
      full recapture."** MET — every anchor cell states `— view-source, verified <date>`; every
      row carries a derived `oldest-verified: <date>` column.
- [x] **"Citations are framed strictly as observational/factual CSS commentary, never as a
      redistributed brand design system."** MET — Attribution note states this explicitly as
      the sole trademark mitigation; individual citations use "this site's rendered CSS"-style
      phrasing throughout.
- [x] **"`critique-template.md` and `SKILL.md` both include `[ ] token set is NOT >80%
      traceable to a single cited anchor → PASS`."** MET — verified present, identically
      worded, in both files (re-padded to column 76 in both).
- [x] **"The new gate is validated against 2–3 real generated outputs and demonstrably flags
      single-brand mimicry (outcome recorded), raising confidence to HIGH."** MET this batch —
      Case A/B/C run above; Case A demonstrably flags single-anchor mimicry that passes the
      original 6 lines; outcome recorded in this file.
- [x] **"The old 'no verifiable capture / human recollection' caveat is updated or retired to
      match the now-real capture."** MET — Attribution note explicitly retires it.
- [x] **"No engine/Go, TUI, registry, manifest, or runtime-wiring file is modified; change
      stays within `skills/anti-generic-design/**` and is fully removable."** MET — confirmed
      via `git diff --stat` scoped to this change: only `skills/anti-generic-design/SKILL.md`,
      `skills/anti-generic-design/references/{palette-typography,critique-template,
      example-output}.md`, and the `openspec/changes/anti-generic-design-token-anchors/`
      artifacts were touched. `cd engine && go build ./... && go vet ./...` run as a final
      sanity check — see result below.

**All Success Criteria MET.** Since the Phase 4 validation task PASSED (gate proven
non-redundant), Phase 5 closure is not blocked and all Success Criteria are marked met above.

### Final Go sanity check

```
cd engine && go build ./... && go vet ./...
```

Run as a final confirmation that this content-only change did not accidentally touch or break
anything under `engine/`.

**Result: `go build ./...` and `go vet ./...` both completed cleanly, zero errors/warnings.**
Confirms this change is genuinely content-only — no engine/Go file was touched or broken.

## Post-Validation Fix: Stale Checklist Line 5 (user-requested, before verify)

Case C's honest finding — line 5 ("palette is near-monochrome + ≤1 warm accent") is falsified by
the new capture itself (Nintendo has 2 real functional accents; Stripe has 1 functional + 4
decorative-illustration-only; SpaceX has 0) — was fixed rather than left as deferred debt, per
explicit user instruction to close it out before `sdd-verify`.

**Change**: in both `skills/anti-generic-design/references/critique-template.md` and
`skills/anti-generic-design/SKILL.md`'s inline checklist, line 5 was reworded from:

```
[ ] palette is near-monochrome + ≤1 warm accent (see palette-typography.md) → PASS
```

to:

```
[ ] accent usage is scoped/deliberate, matching an observed range in palette-typography.md
    (zero to a few, functional not decorative) — not unscoped decorative color sprawl → PASS
```

**Re-verified against Case C**: Nintendo's 2 accents (red for CTAs/links, blue for secondary
buttons/focus) are functional, not decorative, and fall within the "zero to a few" range now
documented in the dimension table — Case C now PASSES line 5 (and thus all 7 lines), consistent
with the reworded criterion. No other checklist line was found stale by this same check.

## Post-Verify Fix: Sector-Tag Formatting (WARNING from verify-report.md)

`sdd-verify` found: the `Site (sector): <value> — <method>, verified <date>` format only
applied the `(sector)` tag on each site's FIRST mention across the whole table, with later
rows using a bare `SpaceX:`/`Vercel:`/etc. — a deviation from design.md's literal per-anchor
template (non-blocking, but fixed per user request before archive).

**Fix**: every per-anchor citation in the 5 dimension rows now repeats the `(sector)` tag on
every mention, not just the first. Confirmed via `rg -o` scan: 15/15 table citations now carry
a sector tag (the one remaining bare `Aesop:` is in the Attribution note's narrative prose, not
a per-anchor table citation, so it correctly does not need the tag).

## Post-Verify Fix 2: Closing the SUGGESTION (no bound on "zero to a few")

The clean-GO re-verify carried forward one non-blocking SUGGESTION: line 5's "zero to a few"
had no hard upper bound. Per user request to leave nothing pending, this was closed rather than
deferred: the wording was tightened to "zero to two, functional not decorative — the max
observed in the capture", grounding the bound in the actual data cited in palette-typography.md's
Accent-scarcity row — which cites exactly 3 anchors for this dimension: SpaceX=0, Stripe=1,
Nintendo=2 (Nintendo is the highest observed count). Vercel, Ferrari, and Aesop are NOT cited in
that row (Ferrari is explicitly excluded from Accent scarcity per its Shadow-DOM caveat above —
its accent token exists but is unconfirmed applied), so "zero to two" is grounded only in the
3 anchors this dimension actually cites, not a claim about all 6 sites' accent counts. Applied
identically to both `critique-template.md` and `SKILL.md`; re-confirmed byte-identical via diff.

If a future re-verification of the capture finds a site with 3+ functional accents, this bound
would need revisiting — that is now a concrete, checkable trigger instead of an open-ended gap.

## Final Status

Phases 1-5 all complete, plus the post-validation line-5 fix, the post-verify sector-tag fix,
and the accent-bound closure above — nothing left pending. Validation task PASSED (Case A decisively caught by line 7 while passing the original 6;
Case B passed all 7; Case C's line-5 finding was fixed rather than deferred, and now also passes
all 7 under the reworded
criterion). All `tasks.md` checkboxes now `[x]`. All proposal Success Criteria verified met.
Ready for `sdd-verify`.
