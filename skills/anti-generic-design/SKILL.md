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

Distilled from 4 real award-gallery sites (see `references/palette-typography.md` for full
attribution):

- **Near-monochrome base + a single warm accent**, used sparingly, instead of a gradient.
- **Editorial typography pairing** — an oversized display face + restrained body text, never
  Inter.
- **Numbered / indexed sequencing** (01, 02…) for sections instead of a plain grid.
- **Asymmetric / broken-grid layout** instead of a flat, even 3-column grid.
- **Conversational, first-person copy** instead of generic marketing-speak.

See `references/palette-typography.md` for the attributed table and
`references/example-output.md` for a worked before/after pass.

## Self-critique checklist (v1, manual)

Run this by hand against any generated design output — no script or tool required:

```
[ ] font-family does NOT resolve to Inter / Roboto / generic system sans → PASS
[ ] no violet→blue (or indigo→purple) gradient on background/hero/CTA      → PASS
[ ] cards are not styled by a generic soft box-shadow alone                 → PASS
[ ] layout is not a flat, even 3-column grid                                → PASS
[ ] palette is near-monochrome + ≤1 warm accent (see palette-typography.md) → PASS
[ ] at least one editorial/asymmetric signal present                        → PASS
```

Any unchecked line is a flag to revise the design before shipping it. Automated tooling for
this check is explicitly deferred to a future v2 — this checklist is v1 and manual-only.

## References

- `references/palette-typography.md` — attributed palette/typography directions, distilled
  (not copied) from depoluxe.xyz, bymonolog.com, collection.industries, useportal.net.
- `references/example-output.md` — a worked "before (generic) → after (distinctive)" pass.
- `references/critique-template.md` — the blank, copy-paste v1 self-critique checklist.
</content>
