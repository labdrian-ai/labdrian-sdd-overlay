# Skills for building this repository

These are the skills and tools the agent uses to build this repository, not the skills the product ships (those are in `skills/` and `skills.registry.yaml`).

| Job | Tool |
|---|---|
| Structure, call flow, impact | CodeGraph (`codegraph_explore`), only where the Claude instance has it; the playbook instance (`~/.claude-instances/playbook`) does not (audit-2026-11-25-7) |
| Memory across sessions | memorycard (MCP) for sessions and learnings; decisions stay in the wiki and `odd/tasks/` (audit-2026-11-25-6); Engram is frozen read-only |
| Define, close, audit, release | playbook skills `define`, `close`, `audit`, `release` |
| Feature work in progress | the project's own feature documents in `odd/tasks/` |

Open: how jobs split between the playbook and the project's own method (`odd/`, `openspec/`); the tools part of the audit raises it.
