# longterm-mem

Packaged long-term memory layer for the runtimes covered by `labdrian-sdd-overlay`
(Claude Code, opencode, codex).

- **Engram** — mid-term memory: current-project decisions, bugs, conventions.
- **claude-obsidian-based vault** — long-term memory: a per-project "neural net"
  of core + emerging knowledge, meta-cognition of how that project evolves. The
  vault is resolved per project from configuration (`labdrian-brain` is the
  default for `labdrian-sdd-overlay`); cross-project querying is out of scope
  for the first wave.

Goal: one mutable source of truth per project that the agent team can query
before a fix/feature/core change, so it doesn't reprocess or re-derive things,
or break canonical process decisions that already exist.

Status: scaffold only — design pending.
