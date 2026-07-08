---
applies_to_phases: [sdd-tasks, sdd-apply]
excluded_phases: [sdd-propose, sdd-spec, sdd-design, sdd-verify, sdd-archive]
injection_point: "## Skills to load before work"
---
# Anti-Generic Design Contract

> Advisory scope: this contract is injected into the phases that generate or refine UI/design
> output (`sdd-tasks`, `sdd-apply`). The frontmatter above is documentation only — the resolver
> does not parse it. See `.atl/skill-registry.md` for the load-bearing binding.

This contract is **complementary to, not a replacement for**, `anthropics/skills/frontend-design`.
As of this writing, `anthropics/skills/frontend-design` (not vendored in this repo) is documented
to ban three OTHER clustered looks via a two-pass token + self-critique process — verify against
the current upstream skill if precision matters here, since it is not pinned or vendored:

- cream + serif + terracotta
- near-black + acid-green
- broadsheet-hairlines

None of those overlap with what this contract forbids. Apply this contract alongside
frontend-design, not instead of it — both are independent self-critique lenses applied to the
same generated design.

## Forbidden patterns (the gap)

The following "Claude/SaaS look" patterns are what AI-generated web designs converge on by
default, and frontend-design does not flag them. They are hard-forbidden here:

1. **Inter, or an equivalent over-used sans-serif** (Roboto, generic system sans) as the primary
   typeface.
2. **Violet-to-blue gradient** backgrounds, hero sections, or CTA accents (also indigo→purple
   variants).
3. **Generic `box-shadow` card styling** — the soft, uniform drop-shadow card as the sole
   differentiator for content blocks.
4. **Flat 3-column grid** layouts as the default structure for feature/content sections.

## Steer toward

- **Near-monochrome base + a single warm accent**, used sparingly, instead of a gradient.
- **Editorial typography pairing** — an oversized display face + restrained body text, never
  Inter.
- **Numbered / indexed sequencing** (01, 02…) for sections instead of a plain grid.
- **Asymmetric / broken-grid layout** instead of a flat, even 3-column grid.
- **Conversational, first-person copy** instead of generic marketing-speak.

## Self-critique checklist (v1, manual)

Run this by hand against any generated design output — no script or tool required:

```
[ ] font-family does NOT resolve to Inter / Roboto / generic system sans → PASS
[ ] no violet→blue (or indigo→purple) gradient on background/hero/CTA      → PASS
[ ] cards are not styled by a generic soft box-shadow alone                 → PASS
[ ] layout is not a flat, even 3-column grid                                → PASS
[ ] palette is near-monochrome + ≤1 warm accent                             → PASS
[ ] at least one editorial/asymmetric signal present                        → PASS
```

Any unchecked line is a flag to revise the design before shipping it. Automated tooling for this
check is explicitly deferred to a future v2 — this checklist is v1 and manual-only.
