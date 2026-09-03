// Package register implements ownership-tagged, byte-preserving edits to
// the MCP configuration files longterm-mem itself does not own: Claude
// Code's and opencode's JSON configs (mcpServers/mcp), and codex's TOML
// config (mcp_servers) (D9; R-016/R-017/R-018). Every writer here edits
// only the exact byte span of the member/section it owns, rather than
// decoding and re-encoding the whole document: a decode-re-encode round
// trip would silently reorder keys, reformat whitespace, and drop
// anything the parser doesn't model (e.g. comments), rewriting a file a
// human maintains out from under them.
//
// "Does not own" binds the file's identity as well as its bytes. Every
// write here goes through internal/durable.WriteFile, so a config keeps its
// permission bits and a config reached through a symlink (a dotfiles
// layout) has its real target edited rather than its link replaced. See
// that package's doc comment for the one identity property — hardlinks —
// deliberately traded away, and why.
//
// All three runtimes share one ownership model, decided by Decide from
// exactly four booleans:
//
//   - entryPresent: does the runtime's own config already have a
//     same-named entry/section?
//
//   - recordPresent: does install-state.json have an ownership record for
//     this target?
//
//   - entryOwned: are the bytes currently on disk byte-identical to the
//     entry this call is about to write? Only longterm-mem produces that
//     exact entry, so this re-derives ownership when the record is gone.
//
//   - fingerprintMatches: does that record's fingerprint match the entry
//     this call is about to write (not necessarily the bytes on disk —
//     see Decide's own doc comment)?
//
//     entryPresent  recordPresent  entryOwned  fingerprintMatches  →  Action   meaning
//     false         false          —           —                      insert   not installed yet
//     false         true           —           —                      replace  entry missing (record stale)
//     true          false          false       —                      refuse   conflict: an entry we did not write
//     true          false          true        —                      adopt    our own entry, record lost
//     true          true           —           false                  replace  stale (hand-edited or old binary path)
//     true          true           —           true                   noop     installed, up to date
//
// The adopt row is what keeps a lost install-state.json recoverable: see
// Decide's own doc comment for why re-deriving ownership from the entry's
// bytes is safe, and writer.go's uninstallCannotDeriveOwnership for the
// one direction that deliberately does not do it.
//
// This is the single source of truth for what "installed"/"stale"/
// "conflict" mean everywhere those words are used across this package's
// three writers (claude.go, opencode.go, codex.go) AND across
// engine/runtime's independent, read-only `doctor`/`status` reporting
// (LongtermMemAdapter, a separate Go module per D4 — it does not import
// this package, but re-derives the same three-boolean state from its own
// scan of each runtime's config and must mean the same thing by it).
package register
