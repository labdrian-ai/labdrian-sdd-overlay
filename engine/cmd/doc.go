// Command engine provides subcommands for the deterministic-scoping engine:
//
//	engine propagate --registry <path> --contract-file <path> [--contract-path <str>]
//	engine gate-task --contract-file <path> [--contract-path <str>]
//	engine merge-settings --settings <path> --hook-command <binary-path>
//	engine uninstall-hooks --settings <path> --hook-command <binary-path>
//	engine status
//	engine skills <verb>  (verbs: list, status, validate, install, adopt, add, remove, sync-manifest, lint, approve, project-register, project-revise, project-status, project-retire)
//
// propagate: ensures the scoped minimalism-contract BEGIN/END marker block is
// present in a target .atl/skill-registry.md. Fails LOUD on bad input.
// Concurrency-safe: serializes via a bounded exclusive flock on <registry>.lock,
// writes the registry atomically (temp file + rename), and refuses (exit 1)
// to propagate over a registry that exists but is empty/whitespace-only.
//
// gate-task: reads a Claude Code PreToolUse 'Agent' tool_input JSON from STDIN,
// inspects subagent_type, and emits the hook response that deterministically
// injects or strips the minimalism-contract path. Fails SAFE on any error.
//
// merge-settings: safely merges two hook entries (UserPromptSubmit + PreToolUse)
// into a Claude Code settings.json. Preserves all existing keys, is idempotent,
// atomic (write+rename), creates a .bak backup, and refuses to write if the
// existing file contains invalid JSON.
//
// uninstall-hooks: removes exactly our two hook entries from settings.json,
// leaving all other keys and hooks intact. Idempotent; no-op if file absent.
//
// status: checks and reports the health of the overlay installation (binary,
// hooks wired in settings.json, contract readable, registry state). Exit codes:
// 0 = all OK, 1 = a hard check FAILED, 2 = no hard failure but a check is
// DEGRADED (e.g. registry present but its scoped block is missing). The registry
// is fail-loud: an empty or unreadable registry is a FAIL, never a silent OK.
// Intended for manual diagnostics — never called by hooks.
//
// propagate/gate-task accept --embedded-contract <name> to source an
// engine-owned managed contract (e.g. anti-generic-design) from the binary
// instead of an external file; propagate then writes that contract's DISTINCT
// marker block. propagate also accepts --require-registry to turn an absent
// registry into a fail-loud error instead of a silent no-op.
//
// skills: registry management commands for skills.registry.yaml and overlay.manifest.
// list: print sorted registry entries. status: print count summary.
// validate: cross-check registry vs overlay.manifest, and skills/ on disk vs
// overlay.manifest via the required --source-root flag, and that every global
// skill has a valid approval record (grandfathered baseline aside); exit 1 on
// divergence.
// install: copy project-scoped skills into <cwd>/.claude/skills.
// add: register a skill (custom or vendored); refused unless a valid approval
// record covers the exact SKILL.md bytes. remove: unregister from registry + manifest.
// sync-manifest: regenerate */SKILL.md rows from skills.registry.yaml.
// lint: lint a SKILL.md file against the authoritative rule table, or print
// that table with --rules; exit 1 on any hard error.
// approve: record a human approval of skills/<id>/SKILL.md, bound to the digest
// of its exact bytes, next to the skill (--id, --approver, --source-root). The
// engine cannot prove a human ran it; the record only proves the bytes match.
// guard-hook: the internal Claude Code PreToolUse hook that denies the agent
// running approve or writing the approval record (see cmd/skills_guard.go); a
// speed bump, not a security boundary, installed by install-hooks.
// project-register/revise/status: manage project-tier procedural skills and
// report ownership from the project lock.
package main
