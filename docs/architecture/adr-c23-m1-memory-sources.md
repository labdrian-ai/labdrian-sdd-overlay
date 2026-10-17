# ADR: how the memory plan's sources map to tools, and the goal filter (C23, M1)

- **Status:** Accepted, 2026-10-17.
- **Decided by:** the owner, one question at a time, on 2026-10-16 and 2026-10-17.
- **Ledger:** `odd/tasks/consolidation-phase9.md`, task C23, unit M1. The gap list (G1 to G15) is in Engram #4133.
- **Implemented by:** M2, M4, M5 and M6 of C23. This record changes no code.

## Context

A workflow's memory plan says which memory it may read, and the engine computes it. Nothing executes
it yet, and the one place that enforces a part of it enforces very little.

**The sources.** A plan names its sources from a closed set of three: `engram`, `longterm-mem` and
`procedural-skills` (`engine/workflowprofile/workflowprofile.go:33-39`, and the same set as
`memoryscope.Source` at `engine/memoryscope/memoryscope.go:43-47`). Each built-in profile declares
a ceiling (`engine/workflowprofile/workflowprofile.go:88-154`):

| Profile | Scope | Sources |
|---|---|---|
| `odd` | project | engram, longterm-mem, procedural-skills |
| `sdd` | project | engram |
| `standalone-minimal` | none | none |
| `maintenance` | project | engram, longterm-mem, procedural-skills |
| `incident-recovery` | goal | engram, procedural-skills |

A request may only narrow a profile's ceiling. The plan it resolves to carries `Filters.ProjectID`
and, for scope goal, `Filters.GoalID` (`engine/memoryscope/memoryscope.go:149-154` and `:230-238`).
A plan executes nothing and grants no write (`planAuthority`, `:159`).

**What the gate does today.** The PreToolUse gate (`engine/projection/gate.go`) has one memory rule.
`GateRelevant` (`:115`) is true only for the file-edit tools and for the tool matched by
`longtermQueryTool` (`:126`), the `query` tool of longterm-mem. For that tool, `memoryGate` (`:167`)
denies every call under scope none and denies a call whose `project` argument is missing or differs
from the plan's project. It never reads the plan's sources. A plan that excludes `longterm-mem`
still permits a query to it (gap G3). Every Engram tool, longterm-mem `get` and `promote`, and
every skill are outside the gate (gap G5; the file comment at `:19-23` says so). The gate is total:
anything it cannot understand is an allow (`:128-130`).

**Goals in longterm-mem.** The plan has a goal and longterm-mem does not. A promoted page carries
`project` and `tags` in its frontmatter (`longterm-mem/internal/promote/frontmatter.go:153,156,173`),
and the `query` tool takes `project`, `query`, `top`, `exclude_types` and `sources` (the tool schema
in `longterm-mem/cmd/longterm-mem/testdata/retrieval-golden/mcp-00-the-handshake-and-the-tools.golden`).
Nothing narrows a query to a goal (gap G4). The `get` tool takes only an `engram_id`, so it names no
project. The project's procedural skills are listed in `.labdrian/procedural-skills.lock.json`
(`engine/skills/project_lock.go:26`), and each is one `SKILL.md` written under every target
directory, `.claude/skills/<id>/` and `.agents/skills/<id>/` (`engine/skills/project_plan.go:18-21`,
`engine/skills/project_lock.go:504`).

## Decision

### The mapping

| Source | Tools the source covers | When the gate denies |
|---|---|---|
| `engram` | the Engram read tools: `mem_search`, `mem_context`, `mem_get_observation`, `mem_timeline` | the plan excludes `engram` |
| `longterm-mem` | the longterm-mem tools `query` and `get` | the plan excludes `longterm-mem` |
| `procedural-skills` | the `Skill` tool, when the requested skill is in the project's procedural lock; and the `Read` tool, when its path is the `SKILL.md` of a procedural skill in that lock | the plan excludes `procedural-skills` |

### The five decisions

1. **Engram: reads only.** The `engram` source covers the four read tools above. The gate denies
   them when the plan excludes `engram`. Writes are always allowed: `mem_save`,
   `mem_session_summary` and every other write. The plan grants no write and forbids none, so the
   session close works under every profile, `standalone-minimal` included.
2. **longterm-mem: `query` and `get`.** When the plan excludes the source, the gate denies both.
   When it includes the source, `query` keeps today's project check, and `get` passes, because it
   names no project. `promote` is always allowed, as a write.
3. **Procedural skills: the `Skill` tool and the skill file.** The source covers the `Skill` tool
   when the requested skill is in the project's procedural lock, and a `Read` whose path is the
   `SKILL.md` of a procedural skill in that lock. Global and installed skills are untouched. `Bash`
   stays ungated, as the gate has always left it (`engine/projection/gate.go:15-17`).
4. **Engram project check, after M2.** When the plan includes `engram`, the gate denies an Engram
   read that names another project or none. This turns on only after M2 unifies the project identity
   (gap G7). Until then the gate checks the source and not the project.
5. **Goal filter.** Every record created under a goal carries the tag `goal:<id>`, in the
   frontmatter `tags` of a longterm-mem page and in the Engram topic. Under scope goal,
   `longterm-mem query` returns the records with that goal's tag and the project's records with no
   goal tag, and excludes the records of other goals. Memory created before M5 carries no goal tag
   and stays visible.

### Alternatives considered

The owner rejected each of these. The last column states what the option would have meant given the
code as it stands; it is not a quotation of the owner's reasons.

| Decision | Rejected | What choosing it would have meant |
|---|---|---|
| 1 | Gate the Engram writes too | The session close (`mem_session_summary`) would be denied wherever the plan excludes `engram`, and `standalone-minimal` excludes every source. |
| 1 | Leave Engram ungated | The `engram` source would constrain nothing, so G3 would stay open for Engram. |
| 2 | Gate `promote` as well | `promote` would be refused where the plan says nothing about writes, because a plan grants none and forbids none. |
| 2 | Cover `query` only | `get` would still read a whole observation after the plan excluded `longterm-mem`. |
| 3 | Cover the `Skill` tool alone | A procedural skill's `SKILL.md` could still be read directly with `Read`. |
| 3 | Cover it by instruction only | Nothing would stop a call, which is the situation today. |
| 4 | Turn the project check on in M4 | Before M2 the stores need not spell a project the same way, so a correct read could be denied. |
| 4 | Never check the Engram project | An Engram read of another project would stay open, although a longterm-mem query is already checked. |
| 5 | A strict goal tag (only tagged records) | Every record made before M5 carries no tag and would drop out of a goal-scoped query. |
| 5 | No goal filter | Scope goal would narrow nothing in `longterm-mem`. |

## Consequences

- **M2** gives the workflow, Engram and longterm-mem one project identity (G7). Decision 4 depends
  on it, and the Engram project check is part of M4 but is enabled only once M2 has landed.
- **M4** widens `GateRelevant` and `Gate`. It matches the Engram read tools, longterm-mem `get`, the
  `Skill` tool and the `Read` tool by name, with the same strictness `longtermQueryTool` has today
  (a plugin prefix is allowed, a lookalike server name is not). `ToolCall` must carry what the new
  rules read from the arguments: the requested skill and the file path. The gate is pure and reads
  no file (`engine/projection/gate.go:3-9`), so the caller reads the project lock, as it already
  reads the binding and the workflow, and hands it in. The gate denies a tool only when its source
  is excluded by a plan it could compute.
- **The gate stays total.** Anything it does not understand remains an allow: a plan that cannot be
  computed, arguments that cannot be read, a tool it does not recognize, a lock that is missing or
  unreadable. It warns in one line only where it had something to check and could not. Every new
  rule here is a denial for a clear violation and nothing else.
- **M5** adds the goal filter to `longterm-mem query`: it reads the `goal:<id>` tag and applies
  decision 5. It needs the goal id from the plan's `Filters.GoalID`, which M3 stores in the workflow
  log (G6).
- **M6** puts the plan in front of the agent as instructions (G1, G2). The instructions must state
  the sources the plan allows and, under a goal, that records are created with the tag `goal:<id>`.
- **Left open.** This record fixes that a record created under a goal carries the tag, in the
  frontmatter `tags` of a page and in the Engram topic. It does not fix which unit writes the tag
  into a promoted page's frontmatter. M5 and M6 are the candidates, and the unit that designs it
  records the choice in its own spec.
- **What does not change.** The plan still grants no write. Writes stay allowed under every
  profile. `Bash` stays ungated. Skills outside the project's procedural lock are not the memory
  plan's concern.
- **Accepted costs.** A skill file can still be read through `Bash`, because a shell command cannot
  be classified reliably. A record created under a goal without the tag is visible to every goal, as
  records from before M5 are.
