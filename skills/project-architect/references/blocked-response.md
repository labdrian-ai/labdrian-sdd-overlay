# project-architect — Blocked Response (Option C)

When `blocked-no-manifest` or `blocked-incomplete-manifest`, do not generate or persist architecture. Return the concrete blocker and the following choice to the parent for the owner; never select it automatically. New envelopes are English except verbatim user quotes.

## Project Architect — Blocked

**Status**: {blocked-no-manifest | blocked-incomplete-manifest}
**Project**: {project name}
**Problem**: {Identify the absent manifest or list the specific critical pending sections.}

**Option 1 — Run `/project-manifest` first (recommended)**: Produce the context, mission and rules documents before designing architecture.

**Option 2 — Provide context inline**: For a small project, the owner may explicitly choose this route and provide the project's nature and exclusions, agreed design constraints, macro architectural preference, 3–5 mandatory non-negotiable rules, and technical stack if `sdd-init` was not detected. Mark inferred material `[SUPUESTO SIN MANIFEST]`; identify the supplied context as its source and do not claim compliance with a persisted manifest that is absent. Apply the compliance table to the supplied rules.

**Option 3 — Cancel**: Stop without producing architecture.

The owner answers `1`, `2`, or `3`; for `2`, include the context in the same message. Return `status: blocked` until that choice and required context are supplied. Ask only one blocking question per parent interaction.

## Input Sufficiency Assessment

- `blocked-no-manifest`: manifest context or required rules are absent; offer the choices above.
- `blocked-incomplete-manifest`: critical sections are `[PENDIENTE]`; name them and offer the same choices.
- `proceed`: sufficient manifest context and rules are present, or the owner explicitly chose option `2` and supplied the required inline context.

Critical manifest sections: project nature, agreed design constraints, macro architectural preference, and identified conceptual modules. Missing permission or other required information is a concrete blocker too.
