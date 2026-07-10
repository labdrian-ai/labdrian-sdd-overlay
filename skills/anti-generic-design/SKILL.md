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

## Activation Contract

This skill is **complementary to, not a replacement for**, `anthropics/skills/frontend-design`.
As of this writing, `anthropics/skills/frontend-design` (not vendored in this repo) is
documented to ban three OTHER clustered looks via a two-pass token + self-critique process —
verify against the current upstream skill if precision matters here, since it is not pinned
or vendored:

- cream + serif + terracotta
- near-black + acid-green
- broadsheet-hairlines

None of those overlap with what this skill forbids. Run this skill alongside
frontend-design, not instead of it — both are independent self-critique lenses applied to
the same generated design.

## Forbidden patterns (the gap)

The following "Claude/SaaS look" patterns are what AI-generated web designs converge on by
default, and frontend-design does not flag them. They are hard-forbidden here:

1. **Inter, or an equivalent over-used sans-serif** (Roboto, generic system sans) as the
   primary typeface.
2. **Violet-to-blue gradient** backgrounds, hero sections, or CTA accents (also indigo→purple
   variants).
3. **Generic `box-shadow` card styling** — the soft, uniform drop-shadow card as the sole
   differentiator for content blocks.
4. **Flat 3-column grid** layouts as the default structure for feature/content sections.

## Steer toward

Steer per DIMENSION using the contrasting anchors in the dated, first-party capture in
`references/palette-typography.md` — combine choices ACROSS dimensions rather than copying one
site's full kit (that is exactly what the anti-mimicry checklist line below catches):

- **Canvas polarity, accent scarcity, corner-radius signature, depth mechanism, and type
  philosophy** — each dimension has 2–3 real, visibly-disagreeing sector-leader anchors; pick a
  combination, never one site's whole kit.
- **Numbered / indexed sequencing** (01, 02…) for sections instead of a plain grid.
- **Asymmetric / broken-grid layout** instead of a flat, even 3-column grid.
- **Conversational, first-person copy** instead of generic marketing-speak.

See `references/palette-typography.md` for the dimension table and the compositional-signals
subsection, and `references/example-output.md` for a worked before/after pass.

## Self-critique checklist (v1, manual)

Run this by hand against any generated design output — no script or tool required:

```
[ ] font-family does NOT resolve to Inter / Roboto / generic system sans → PASS
[ ] no violet→blue (or indigo→purple) gradient on background/hero/CTA      → PASS
[ ] cards are not styled by a generic soft box-shadow alone                 → PASS
[ ] layout is not a flat, even 3-column grid                                → PASS
[ ] accent usage is scoped/deliberate, matching an observed range in palette-typography.md
    (zero to two, functional not decorative — the max observed in the capture) — not
    unscoped decorative color sprawl → PASS
[ ] at least one editorial/asymmetric signal present                        → PASS
[ ] token set is NOT >80% traceable to a single cited anchor                → PASS
```

Any unchecked line is a flag to revise the design before shipping it. Automated tooling for
this check is explicitly deferred to a future v2 — this checklist is v1 and manual-only.

## References

- `references/palette-typography.md` — dimension-indexed, dated capture from 6 sector-leader
  sites (Vercel, Stripe, Ferrari, SpaceX, Nintendo, Aesop); the compositional-signals
  subsection (sequencing/layout/voice) remains attributed to the original 4 sites (depoluxe.xyz,
  bymonolog.com, collection.industries, useportal.net).
- `references/example-output.md` — a worked "before (generic) → after (distinctive)" pass.
- `references/critique-template.md` — the blank, copy-paste v1 self-critique checklist.
</content>
