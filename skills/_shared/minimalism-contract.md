---
applies_to_phases: [sdd-tasks, sdd-apply]
excluded_phases: [sdd-propose, sdd-spec, sdd-design, sdd-verify, sdd-archive]
injection_point: "## Skills to load before work"
---
# Minimalism Contract

> Advisory scope: this contract is injected ONLY into `sdd-tasks` and `sdd-apply` prompts
> (via the scoped registry Trigger row). The frontmatter above is documentation only — the
> resolver does not parse it. See `.atl/skill-registry.md` for the load-bearing binding.

## Preference ladder

Pick the lowest viable rung. Climb only when the lower rung cannot satisfy the requirement.

1. **YAGNI** — do not build it if not required now.
2. **stdlib / language built-ins** — use what the language already provides.
3. **Native platform feature** — use what the runtime/framework already provides.
4. **Existing dependency** — use a library already in the project.
5. **One-liner / minimal local code** — write the shortest possible local solution.
6. **Custom code / new abstraction** — last resort; requires justification.

## Architectural tiebreaker (mandatory)

Minimalism operates WITHIN design boundaries. A boundary mandated by the architecture is
NEVER collapsed merely to save lines. Code economy never overrides a deliberate seam.

## Comment convention

When rung 6 is chosen, mark it inline using the host language's single-line comment prefix
(`//`, `#`, etc.) according to the state below:

- **State 1 — judgment** (a lower rung existed but was rejected):
  `// minimal: <reason>` — name the rung considered and why it was insufficient.

- **State 2 — forced, non-obvious** (no lower rung applicable AND constraint is not obvious):
  `// minimal: forced — <design/constraint ref>` — cite the artifact or constraint name.

- **State 3 — no comment** (no lower rung applicable AND constraint IS obvious from context):
  omit the comment entirely. Adding one here is noise, which the contract forbids.
