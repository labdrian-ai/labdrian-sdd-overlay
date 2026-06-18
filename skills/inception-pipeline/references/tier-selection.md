# Tier Selection — Worked Examples

Conditional-load companion to the tier-selection checklist in `../SKILL.md`. Read this **only when the tier is ambiguous** (checklist rule 4 fired). It illustrates the firing rule for each canonical case. The checklist in the SKILL.md is authoritative; these examples disambiguate edge cases.

The checklist, repeated for reference (evaluate in order, stop at the first match):

1. No `project/{project}/manifest/context` in engram → **Tier 1**.
2. manifest + architect exist AND self-contained, single surface, no new architectural surface, no schema/integration/prod risk → **Tier 3**.
3. manifest + architect exist AND adds a feature/capability needing roadmap sequencing or cross-cutting work → **Tier 2**.
4. Ambiguous → **default UP one tier**, record rationale in `sdd/{change}/pipeline-state`, ask ONE question only if genuinely ambiguous.

---

## Example A — New project (Tier 1)

**Signal:** User says "let's build a new invoicing service." No `project/{project}/manifest/context` exists in engram.

**Rule fired:** #1 — no manifest.

**Tier:** 1. Run the full flow: requirements-from-transcripts (if a transcript exists) → project-inception `inception_mode: full` → sdd-time-estimation. Foundation is built from scratch.

**Why not lower:** Without a manifest, architecture would invent context and the roadmap would invent dependencies. Stated intent ("just a small service") does NOT override the missing-manifest evidence.

---

## Example B — New feature on an existing project (Tier 2)

**Signal:** manifest + architect exist. User wants "add multi-currency support to invoicing." It introduces a new capability, touches several modules, and needs to be sequenced relative to existing roadmap items.

**Rule fired:** #3 — feature/capability needing roadmap sequencing.

**Tier:** 2. requirements-from-transcripts (per feature) → project-inception `inception_mode: reuse` (drives roadmap-maker `incremental-insert`) → sdd-time-estimation. Manifest/architect are read-only constraints — never rewritten for one feature.

**Why not Tier 3:** It adds a new architectural surface (currency handling, FX integration) and cross-cuts modules; it is not a self-contained single-surface change.

---

## Example C — Chore on an existing project (Tier 2)

**Signal:** manifest + architect exist. User wants "migrate the logging library across all services and standardize log format." Cross-cutting, touches many surfaces, needs sequencing so it doesn't collide with in-flight changes.

**Rule fired:** #3 — cross-cutting work needing roadmap sequencing.

**Tier:** 2. Even though it's a "chore" not a "feature", the cross-cutting blast radius makes it Tier 2. The deciding factor is sequencing/blast-radius, not the feature-vs-chore label.

---

## Example D — Small fix (Tier 3)

**Signal:** manifest + architect exist. User wants "fix the off-by-one in the invoice line-item total." Single function, single file, no schema change, no integration touched, no production-data risk.

**Rule fired:** #2 — self-contained, single surface, no new architectural surface, no schema/integration/prod risk.

**Tier:** 3. Lightweight path: requirements-from-transcripts (lightweight) → roadmap-maker `incremental-insert` `tier: 3` → sdd-time-estimation → engine runs a slim slice. No full inception, no architecture rework.

---

## Example E — Ambiguous: single-file change WITH a schema migration (default UP)

**Signal:** manifest + architect exist. User wants "rename a column and update the one query that reads it." It LOOKS like a single-surface fix (one file feel), but it carries a **schema migration** — which rule #2 explicitly excludes ("no schema risk").

**Rule fired:** #4 — contradictory signals (single-file feel vs schema/migration risk). Rule #2 cannot fire because schema risk is present.

**Tier:** default UP from the Tier-3 instinct → **Tier 2**. Record in `sdd/{change}/pipeline-state`: "single-file edit but schema migration present; rule #2 excludes schema risk → defaulted up to Tier 2 for sequencing and migration safety." Ask ONE clarifying question only if the migration's reversibility / data impact is genuinely unknown.

**Why default up, not down:** A schema migration can break production and downstream consumers. The cost of treating it as a casual Tier-3 fix (no sequencing, no roadmap insert, slim slice) far outweighs the cost of one extra inception step. When in doubt, choose the safer, higher tier.

---

## Quick reference

| Case | manifest? | new arch surface | schema/integration/prod risk | sequencing needed | Tier | Rule |
|---|---|---|---|---|---|---|
| New project | no | — | — | — | 1 | #1 |
| New feature | yes | yes | maybe | yes | 2 | #3 |
| Cross-cutting chore | yes | maybe | maybe | yes | 2 | #3 |
| True small fix | yes | no | no | no | 3 | #2 |
| Single-file + schema migration | yes | no | **yes (schema)** | maybe | 2 (up) | #4 |
