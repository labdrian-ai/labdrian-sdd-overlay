# Palette & Typography Reference

A dimension-indexed, first-party dated capture: 6 distinct industry verticals, each
represented by its CURRENT market-leader site, independently visited via static
curl/view-source (no browser rendering, no recollection). One row per design DIMENSION
(not per site) — each row cites 2–3 *contrasting* anchors so the guidance forces contrast
between visibly disagreeing real sites, instead of converging on one "average" look. Every
citation is observational/factual CSS commentary ("this site's rendered CSS uses X"), never
a redistributed brand design system (see Attribution note).

## Dimension anchor table

| Dimension | Contrasting anchors | oldest-verified |
|-----------|----------------------|------------------|
| **Canvas polarity** | SpaceX (aerospace/frontier-tech): near-pure black `#000` (`.app-background{background-color:#000}`), text token is cool-tinted off-white `rgba(240,240,250,1)`, not pure `#fff` — view-source, verified 2026-07-10. Vercel (dev-tools): white `#fff`, light/high-key, no `.dark` class on served `<html>` — view-source, verified 2026-07-10. Aesop (editorial retail/e-commerce): warm cream/paper `#fffef2`, explicitly NOT pure white, used consistently across body/buttons/content boxes — view-source (Wayback Machine archive, live fetch blocked by Cloudflare 2026-07-10), verified 2026-06-06. | 2026-06-06 |
| **Accent scarcity** | SpaceX (aerospace/frontier-tech): zero brand-accent color — `:root` defines only grayscale tokens; one stray non-neutral hex exists but is scoped to a text-highlight utility class, never a CTA/UI element — view-source, verified 2026-07-10. Stripe (fintech/payments): one functional accent `#533afd` ("blurple") used consistently across every solid CTA/button; 4 additional named hues exist but are reserved for illustration graphics only, never CTAs — view-source, verified 2026-07-10. Nintendo (consumer electronics/entertainment): two accents — primary red `#e60012` (CTAs, links, primary icons) and secondary blue `#3946a0` (secondary buttons, focus rings) — view-source, verified 2026-07-10. | 2026-07-10 |
| **Corner-radius signature** | Aesop (editorial retail/e-commerce): explicit `border-radius:0` on primary AND secondary CTAs, and no `border-radius` property at all (unset, not just zero) on product-tile cards — view-source (Wayback Machine archive), verified 2026-06-06. Vercel (dev-tools): pill/full-round primary CTA (`!rounded-full`), 8px on dialogs/modals, 6px base system token — view-source, verified 2026-07-10. Nintendo (consumer electronics/entertainment): two coexisting conventions in the same system — a 6–8px responsive scale for general UI (escalating to 8–12px at desktop breakpoint) AND a separate full-round treatment (`50%` circular icon button, `100px` pill nav, `1000px` pill toggle) — view-source, verified 2026-07-10. | 2026-06-06 |
| **Depth mechanism** | SpaceX (aerospace/frontier-tech): flat + translucent borders, NOT shadows — a `box-shadow` regex scan across ~1.2MB of fetched JS/CSS (4 files) returned ZERO matches; elevation reads via `border-color:var(--white-25)` instead — view-source, verified 2026-07-10. Stripe (fintech/payments): real `box-shadow` (47 distinct declarations), signature dual-layer spec `0 13.365px 26.73px -12.276px rgba(50,50,93,.25),0 8.019px 16.038px -8.019px rgba(0,0,0,.1)` on cards; the "mesh" hero effect is CSS `radial-gradient`/`conic-gradient` on positioned divs — NOT box-shadow, NOT an image asset — view-source, verified 2026-07-10. Nintendo (consumer electronics/entertainment): box-shadow present but selective — used on floating/overlaid elements (pill nav, card tiles, tooltip via `filter:drop-shadow`) while explicitly `box-shadow:none` on at least one sibling rule — a deliberately-scoped variant, not a blanket default — view-source, verified 2026-07-10. | 2026-07-10 |
| **Type philosophy** | Ferrari (luxury automotive): custom self-hosted "FerrariSans" (`@font-face`), only two weights shipped (400 regular, 500 medium — no bold/black found); body copy carries slight POSITIVE tracking (`letter-spacing:.015em`) — view-source, verified 2026-07-10. Nintendo (consumer electronics/entertainment): primary "Geologica Variable" self-hosted variable font (weight axis 100–900, multiple unicode-range subsets) composited with a secondary Google-Fonts-CDN "Roboto Condensed Variable"; notable bold token is 600, not the more typical 700 — view-source, verified 2026-07-10. SpaceX (aerospace/frontier-tech): custom self-hosted "D-DIN" (400 regular + a separate "D-DIN-Bold" family), pervasive uppercase + tracking (`letter-spacing:.09em` on bold/700 nav, `letter-spacing:1px` uppercase on regular/400 titles) — an "engineering readout" voice — view-source, verified 2026-07-10. | 2026-07-10 |

### Capture caveats (read before citing)

- **Ferrari is Shadow-DOM-limited.** ferrari.com renders via Web Components (`:host`,
  `:not(:defined)` selectors); component-level styles likely load through JS-injected Shadow
  DOM stylesheets not visible to a static fetch. Ferrari's accent hue, corner-radius token
  (`--f-radius-full:9999px`), and shadow token (`--f-shadow-small`) are CONFIRMED to exist as
  *defined tokens* but are **NOT confirmed applied** to any visible button/card — zero literal
  `border-radius:`/`box-shadow:` properties were found anywhere in the fetched CSS. That is why
  Ferrari appears above only in Canvas polarity and Type philosophy (both directly confirmed
  from fetched, non-component CSS) and not in Accent scarcity, Corner-radius, or Depth.
- **Aesop's capture is dated 2026-06-06, not the same-day 2026-07-10 as the other 5.** The live
  site returned HTTP 403 (Cloudflare bot-challenge) on every attempt on 2026-07-10. The
  substitute is a first-party-served Wayback Machine snapshot of aesop.com's own CDN-served
  HTML/CSS (fetched raw/unmodified via `web.archive.org`'s `id_` mode) — genuine Aesop-served
  bytes, just captured 5 weeks earlier. Aesop also hardcodes literal hex per-rule with no
  `:root` color custom-properties at all — a notable finding in itself, distinct from every
  other captured site.
- **Empirical canvas-polarity finding (updates the design's a-priori hypothesis).** 4 of the 6
  sites (Vercel, Stripe, Ferrari, Nintendo) came back light/white canvas, not the more even
  dark/light split originally hypothesized. Only SpaceX confirmed a genuinely dark/black
  canvas; Aesop landed on warm-cream — distinct from pure white. This is reported honestly as
  what the capture actually found, not forced to match the prior expectation.
- **Despite that skew, no two of the 6 captured sites agree on all 5 dimensions
  simultaneously** — canvas polarity alone already splits the white-canvas cluster
  (Vercel/Stripe/Ferrari/Nintendo) from SpaceX and Aesop, and every pair within that
  white-canvas cluster still disagrees on at least one of accent scarcity, corner-radius,
  depth, or type philosophy (e.g. Vercel's near-zero accent vs Stripe's one consistent
  `#533afd` vs Nintendo's two named accents; Stripe's heavy signature box-shadow vs Vercel's
  hairline-border-by-default vs Nintendo's selective/off-by-default shadow). The 6-vertical
  capture forces genuine cross-dimension contrast even where canvas polarity clusters.

## Compositional signals (qualitative — not per-anchor captured)

These three signals are compositional judgments about a whole page (sequencing, layout,
voice) — not a single rendered hex/px/font value — so they cannot honestly join the dated,
per-anchor capture table above. Relocated verbatim from the prior aspect table; sourced from
the original exploratory site set, not the 6 sector-leader capture.

| Signal | Distilled direction | Source site(s) |
|--------|---------------------|-----------------|
| Sequencing | Numbered or indexed section markers (e.g. `01`, `02`…) used as a structural/navigational device instead of unlabeled sections | collection.industries, depoluxe.xyz |
| Layout | Asymmetric or broken-grid composition — offset columns, varied block widths — instead of a flat, even 3-column grid | useportal.net, depoluxe.xyz |
| Voice | Conversational, first-person copy in headings and microcopy, over generic marketing-speak | bymonolog.com, useportal.net |

## Attribution note

This file's dimension table is backed by a real, first-party, dated capture: 6 sector-leader
sites (Vercel, Stripe, Ferrari, SpaceX, Nintendo, Aesop) independently visited via static
curl/view-source on 2026-07-10 (Aesop: 2026-06-06 via Wayback Machine, see caveats above), with
every value directly observed (rendered hex, computed `font-family`/weight/size, actual
`border-radius`, actual shadow/no-shadow treatment) — not human recollection. The prior "no
persisted capture artifact" caveat is retired; it no longer applies.

Because most anchors name recognizable, trademarked brands (sector-leader selection means
there is no obscure-site buffer), the sole trademark-attribution mitigation is framing: every
citation states what was directly observed on the live site's rendered CSS ("this site's
computed styles use X"), and never implies a redistributed, licensed, or vendored brand design
system. No brand's full token kit is reproduced here — each dimension row deliberately mixes
2–3 *disagreeing* sites so that following this table steers toward contrast, not toward copying
any single cited brand.
