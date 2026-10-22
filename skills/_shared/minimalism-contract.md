---
applies_to_phases: [sdd-tasks, sdd-apply, shaper, builder]
excluded_phases: [sdd-propose, sdd-spec, sdd-design, sdd-verify, sdd-archive, reviewer, verifier]
injection_point: "## Skills to load before work"
---
# Minimalism Contract

> Scope: this contract is injected ONLY into `shaper` and `builder` prompts, and into the
> `sdd-tasks` and `sdd-apply` prompts until those agents are removed. The frontmatter above is
> load-bearing: `engine gate-task` (the PreToolUse `Agent` hook) parses it on every sub-agent
> launch to inject or strip the contract path, `engine propagate` renders the scoped Trigger
> row of `.atl/skill-registry.md` from it, and the Pi gate and the OpenCode prompt config read
> the same lists.

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
