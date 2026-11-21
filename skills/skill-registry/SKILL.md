---
name: skill-registry
description: "Trigger: update skills, skill registry, actualizar skills, after skill changes. Index available skills by trigger and path."
license: MIT
metadata:
  author: labdrian
  version: "2.0"
---

## Activation Contract

Use this skill after installing, removing, creating, moving, or renaming skills, or when a delegator needs a fresh skill index.

## Hard Rules

- The registry is an index, not a compiler or summary. `SKILL.md` remains the source of truth.
- Do not generate or inject compact rules by default; preserve author intent by passing exact skill paths to subagents.
- Always write `.atl/skill-registry.md`. The file is the only copy of the index: do not save it anywhere else, because a copy would be stale by the next skill change.
- Generate the registry with `labdrian skill-registry refresh --cwd <project>`: it owns the scan order, deduplication, and the skills it never indexes (`_shared`, `skill-registry` and the `sdd-*` executor skills, whose rows the same command puts back as `executor` rows). Scan by hand only when that command is unavailable.
- When scanning by hand, skip `_shared`, `skill-registry` and `sdd-*`; deduplicate by skill name, preferring project-level skills over user-level skills.
- The command never edits `.gitignore`; add `.atl/` there yourself only when the project should not track the registry.

## Decision Gates

| Situation | Action |
| --- | --- |
| Same skill exists globally and in project | Keep the project-level skill |
| Same skill exists in multiple global locations | Keep the first source in scan order |
| No skills found | Write an empty registry so agents stop searching blindly |
| Agent will delegate work | Select matching registry rows and pass their `SKILL.md` paths |

## Execution Steps

1. Run `labdrian skill-registry refresh --cwd <project>` (add `--force` after skill changes); it writes `.atl/skill-registry.md`. If it reports the refresh was skipped (not a project: no `.git` or `.atl`), stop: there is no registry to write. If it succeeds, go to step 4. If it fails, report the error instead of scanning by hand.
2. Only if the command is unavailable: scan all known user and project skill directories for `*/SKILL.md`, reading frontmatter only as needed to extract `name` and `description` trigger text.
3. Render `.atl/skill-registry.md` with scanned sources, registry contract, skill name, trigger/description, scope, and exact path.
4. Return the registry path, skill count, and cache status.

## Output Contract

Return:
- Project name and `.atl/skill-registry.md` path.
- Number of indexed skills.
- Whether the registry was already up to date or was regenerated.
- Any skipped or duplicate skills when relevant.
