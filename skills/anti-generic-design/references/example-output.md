# Worked Example: Generic → Distinctive

## Before (generic "Claude/SaaS look")

A landing page hero with:
- Headline set in Inter, bold, centered.
- Background: a violet-to-blue gradient (`linear-gradient(135deg, #7c3aed, #2563eb)`).
- Three feature cards below the hero, each in a white rounded box with a soft
  `box-shadow: 0 4px 12px rgba(0,0,0,0.08)`, arranged in a flat, even 3-column grid.
- Copy: "Our platform empowers teams to work smarter and faster."

This trips every line of the forbidden-pattern list: Inter, violet-blue gradient, generic
shadow cards, flat 3-column grid.

## After (distinctive, evidence-based)

Applying the DIMENSION table from `palette-typography.md` — deliberately mixing anchors
ACROSS dimensions rather than reproducing one site's full kit:
- Canvas: warm cream/paper ground (`#fffef2`-family, NOT pure white) — canvas-polarity
  direction sourced from Aesop.
- Accent: a single warm accent used ONLY on the primary CTA, never as a background wash or
  gradient — accent-scarcity direction sourced from Stripe's "one functional accent applied
  consistently to CTAs, nowhere else" pattern (reinterpreted in a warm hue, not Stripe's blue).
- Corner-radius: sharp/zero-radius cards — sourced from Aesop — paired with a pill-shaped
  primary CTA — sourced from Vercel — a deliberate dual convention rather than one uniform
  radius.
- Depth: flat, hairline-bordered cards with zero box-shadow — depth-mechanism direction
  sourced from SpaceX's "translucent border instead of shadow" treatment.
- Typography: an oversized editorial display face for the headline + a restrained,
  self-hosted body face (never Inter/Roboto/system sans) — type-philosophy direction informed
  by the capture's self-hosted-font pattern (Ferrari, Nintendo, SpaceX all ship custom
  `@font-face` families rather than a system/Google-default sans).
- Features presented as a numbered, asymmetric sequence (`01`, `02`, `03`) with varied block
  widths instead of a flat 3-column grid — compositional-signal direction sourced from
  collection.industries and useportal.net (see the Compositional signals subsection).
- Copy rewritten in first person: "We built this because our own team was tired of juggling
  five tools." — compositional-signal direction sourced from bymonolog.com and useportal.net's
  conversational tone.

## Why this passes the checklist

- No Inter/Roboto/system sans → editorial display + restrained self-hosted body face instead.
- No violet→blue gradient → warm cream ground + single warm accent used only on the CTA.
- No generic shadow cards → flat, hairline-bordered cards; radius mixed (sharp cards / pill
  CTA), not one uniform value.
- No flat 3-column grid → numbered, asymmetric sequencing.
- Token set is NOT >80% traceable to a single cited anchor → canvas (Aesop), accent pattern
  (Stripe), corner-radius (Aesop + Vercel), depth (SpaceX), and type-philosophy pattern
  (Ferrari/Nintendo/SpaceX) are drawn from 4 different sites across 5 dimensions, plus 2
  additional compositional-signal sites (collection.industries, useportal.net, bymonolog.com)
  — no single site supplies more than one dimension's direction.
</content>
