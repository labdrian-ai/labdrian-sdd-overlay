# ADR: the memory gate enforces the plan's sources (C23, M4)

- **Status:** Accepted, 2026-10-19.
- **Decided by:** the owner. The mapping, the writes and the Engram project check were decided in M1
  (`adr-c23-m1-memory-sources.md`) and M2 (`adr-c23-m2-project-identity.md`). The one decision
  they left open, the Engram read by id, was decided on 2026-10-19.
- **Ledger:** `odd/tasks/consolidation-phase9.md`, task C23, unit M4 (gaps G3 and G5, Engram #4133).

## Context

The PreToolUse gate (`engine/projection/gate.go`) checks one memory tool, the longterm-mem `query`,
and only its project. It never reads the plan's sources, so a plan that excludes `longterm-mem`
still permits a query (G3). Engram, longterm-mem `get` and the project's procedural skills are not
gated at all (G5). Since M3 the plan the gate computes is the profile's ceiling narrowed by the
Goal, the shaper handoff and the latest role directive (`workflow.PlanOf`).

Claude Code runs the gate only for the tools a settings matcher routes to it
(`engine/settings/projection.go`): today the file-edit tools and the longterm-mem `query`.

## Decision

1. **The sources are enforced (M1, the mapping).** A gated read whose source the plan excludes is
   denied:

   | Source | Gated reads |
   |---|---|
   | `engram` | `mem_search`, `mem_context`, `mem_get_observation`, `mem_timeline` |
   | `longterm-mem` | `query`, `get` |
   | `procedural-skills` | the `Skill` tool for a skill in the project's procedural lock; a `Read` of the `SKILL.md` of such a skill |

   A plan with scope none has no sources, so every gated read is denied, as every query is today.
2. **Writes are never gated (M1 decisions 1-2).** Every Engram write, longterm-mem `promote`, and
   every tool not listed above pass untouched. The plan grants no write and forbids none.
3. **The Engram project check (M1 decision 4, M2).** When the plan includes `engram`, `mem_search`
   and `mem_context` must name, in their `project` argument, `identity.EngramName` of the identity
   the workflow recorded. A read that names another project, names none, or asks for every project
   (`all_projects` true) is denied. A workflow that recorded no identity (created before M2) is an
   allow with one warning, as for longterm-mem.
4. **A read by id passes (the owner, 2026-10-19).** `mem_get_observation` and `mem_timeline` look an
   observation up by its id and name no project, like longterm-mem `get`. When the plan includes
   `engram` they pass; when it excludes it they are denied. The gate is pure and does not ask
   Engram which project an id belongs to.
5. **Procedural skills come from the lock, read by the caller.** The gate reads no file, so the hook
   service reads the project's procedural lock (`.labdrian/procedural-skills.lock.json`) through a
   port and hands the gate the procedural skill ids and their `SKILL.md` paths. Only the lock's
   procedural skills count; installed registry skills and global skills are untouched (M1
   decision 3). `Bash` stays ungated.
6. **The hook is routed to the new tools.** The projection family gains the PreToolUse matchers
   for the Engram reads, longterm-mem `get`, `Skill` and `Read`; `labdrian apply` installs them,
   and the tool-name patterns stay as strict as the `query` pattern (a plugin prefix is allowed, a
   lookalike server name is not).

## Alternatives considered

| Decision | Rejected | What choosing it would have meant |
|---|---|---|
| 4 | Deny a read by id | No observation could be read in full while a workflow is followed, even under a plan that includes `engram`. |
| 4 | Require a `project` argument on a read by id | Engram looks an id up without filtering by project, so the check would be one of form only. |

The alternatives of decisions 1-3 and 5 are recorded in the M1 and M2 ADRs.

## Consequences

- **The gate stays total.** A plan it cannot compute, arguments it cannot read, a lock that is
  missing or unreadable, a workflow with no identity: each is an allow, with one warning only where
  the gate had something to check and could not.
- **Accepted cost.** Every `Read` now runs the projection hook, because a matcher sees only the
  tool's name. For a `Read` the hook decides from the path alone, before it reads the binding or the
  lock, unless the path names a `SKILL.md`.
- **Accepted cost (M1).** A skill file can still be read through `Bash`.
- **Not in M4.** The goal filter of `longterm-mem query` (M5), the plan as instructions (M6), and the
  evidence of memory reads (M7).
