# Proposal: anti-generic-ai-design-skill

## Intent

AI-generated web designs converge on a recognizable "Claude/SaaS look": Inter (or equivalent over-used sans), a violet-blue gradient, generic box-shadow cards, and a flat 3-column grid. The official `anthropics/skills/frontend-design` skill runs in background and bans 3 *other* clustered looks (cream+serif+terracotta, near-black+acid-green, broadsheet-hairlines) via a two-pass token+self-critique process — but it does NOT cover this specific gradient/Inter/shadow-card/flat-grid pattern. That is the real gap. This change adds a lightweight companion skill that closes it, distilling positive patterns from real award-gallery sites, without duplicating or replacing frontend-design.

## Scope

### In Scope
- New overlay skill `anti-generic-design/` with `SKILL.md`: explicit forbidden-pattern list frontend-design misses (Inter + equivalents, violet-blue gradient, generic shadow cards, flat 3-col grid).
- A `references/` file with a palette/typography set built from 4 real fetched sites (depoluxe.xyz, bymonolog.com, collection.industries, useportal.net): near-monochrome + single warm accent, editorial/numbered sequencing, asymmetric layouts, conversational copy.
- Simple v1 validation heuristic (rules/keywords) the skill applies during self-critique — no automated tooling required.
- Structure modeled on `knowledge-work-plugins` `data/skills/*` (SKILL.md + references/), the verifiable-skill precedent.

### Out of Scope
- Automated lint/script validator (possible v2).
- Building the `labdrian-brain` design vault (external infra, resolved separately; a future reference source only).
- Duplicating, merging, or replacing frontend-design, design-critique, or design-system.

## Capabilities

### New Capabilities
- `anti-generic-design`: authored skill (SKILL.md + references) that flags the Inter/gradient/shadow-card/flat-grid default and steers toward distinctive, evidence-based alternatives.

### Modified Capabilities
None.

## Approach

Author a self-contained skill directory (content, not Go code). SKILL.md states the gap vs. frontend-design, lists forbidden patterns, and adds a keyword/rule self-critique pass. `references/` carries a from-scratch palette/type file distilled (not copied) from the 4 visited sites. No engine/TUI code touched.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `skills/anti-generic-design/SKILL.md` | New | Forbidden patterns + heuristic |
| `skills/anti-generic-design/references/` | New | Palette/typography reference |
| `.gitignore` | Modified | Ignores `.atl/`, since `.atl/skill-registry.md` is a regenerated artifact, not hand-maintained source |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Overlaps/contradicts frontend-design | Med | Scope explicitly to the uncovered pattern; state complementarity |
| Palette feels arbitrary | Med | Ground every entry in a named fetched reference site |
| Over-scoping into a validator | Low | v1 = heuristic only; automation deferred to v2 |

## Rollback Plan

Delete the `skills/anti-generic-design/` directory. Additive-only; no existing skill, registry entry, engine, or TUI code changes, so removal leaves the overlay fully valid.

## Dependencies

- frontend-design (reference baseline, not modified).
- 4 fetched award-site references (already captured in exploration).

## Success Criteria

- [ ] SKILL.md lists the Inter/gradient/shadow-card/flat-3-col forbidden set absent from frontend-design.
- [ ] `references/` palette/type file cites all 4 real sites.
- [ ] A keyword/rule self-critique pass is defined and runnable by hand.
- [ ] No engine/TUI/registry files modified; skill is self-contained and removable.
