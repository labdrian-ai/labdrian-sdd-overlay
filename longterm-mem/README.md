# longterm-mem

Packaged mid/long-term memory layer for the runtimes covered by
`labdrian-sdd-overlay` (Claude Code, opencode, codex).

- **Engram** — mid-term memory: current-project decisions, bugs, conventions.
  `longterm-mem` reads it through a read-only SQLite connection; it never
  shells out to Engram's own `search`/`save` CLI.
- **claude-obsidian-based vault** — long-term memory: a per-project "neural net"
  of core + emerging knowledge, meta-cognition of how that project evolves. The
  vault is resolved per project from configuration (`labdrian-brain` is the
  default for `labdrian-sdd-overlay`); cross-project querying is out of scope
  for the first wave.

Goal: one mutable source of truth per project that the agent team can query
before a fix/feature/core change, so it doesn't reprocess or re-derive things,
or break canonical process decisions that already exist.

## Module boundary

`longterm-mem/` is a standalone Go module (its own `go.mod`, `go 1.26.1`),
outside `engine/`'s zero-dependency boundary — it is free to declare
third-party dependencies (`modernc.org/sqlite`, the MCP `go-sdk`, `go-toml`)
without affecting `engine/`'s own dependency manifest.

The component builds a single binary installed at a fixed path,
`~/.labdrian-overlay/bin/longterm-mem`. That binary is both the MCP stdio
server (`longterm-mem mcp`) and the CLI (`query`, `index`, `sync`, `promote`,
`status`, `doctor`, `register`, `unregister`, `vaults`).

Status: scaffold in progress — Engram read access and query scoping land
first; the vault, promotion, and MCP/CLI surfaces land in later slices. A
full CLI/MCP usage reference lands here once `install`/`status`/`uninstall`
exist end-to-end.
