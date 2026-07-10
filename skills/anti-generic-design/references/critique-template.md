# Self-Critique Checklist (v1, manual)

Copy this checklist and run it by hand against a design output (CSS, tokens, or a written
design description). No script or tool is required — every line is checkable by reading
the output.

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

Any unchecked (failing) line means the design should be revised before it ships. This
checklist is v1 and intentionally manual-only — automated tooling is deferred to a future
v2 and is out of scope for this skill.
</content>
