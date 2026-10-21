# ADR: Codex receives the memory plan through a SessionStart hook; Pi stays skill-invoked (C23, M12)

- **Status:** Accepted, 2026-10-21.
- **Decided by:** the owner, three questions asked one at a time on 2026-10-21.
- **Ledger:** `odd/tasks/consolidation-phase9.md`, task C23, unit M12 (gap G11, Engram #4133).

## Context

- **The live acceptance of M11 failed on Codex.** `tools/memory-live-acceptance/run.sh codex` gave FAIL: the
  agent could not run `labdrian memory plan --instructions` ("command not found") nor open the `memory-plan`
  skill, and answered `unavailable`. Claude and Pi passed.
- **Cause, diagnosed with read-only probes of `codex exec` (Engram #4281).** The agent's shell cannot see
  most of `$HOME`: `~/.codex/skills`, `~/.local/bin/labdrian` (a symlink into `~/labdrian-sdd-overlay`),
  `~/.claude/skills` and the overlay are hidden. `~/.codex/config.toml` selects
  `default_permissions = "gentle-dev"`, a profile written by gentle-ai 4.2.0 whose filesystem section is an
  allow-list (`:minimal`, linuxbrew, the Codex package, the workspace). So a skill under `~/.codex/skills` and
  a binary in `~/.local/bin` cannot be the channel for Codex on this machine, and the same would hold for
  anyone using a restrictive profile.
- **What Codex 0.148 offers (version-pinned sources, `rust-v0.148.0`).** Lifecycle hooks configured in
  `hooks.json` or `config.toml` (`SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse` and others).
  A hook's stdout, plain or `{"hookSpecificOutput":{"hookEventName":...,"additionalContext":...}}`, becomes
  model context. Hooks run outside the agent shell's sandbox (`gentle-ai skill-registry refresh` already runs
  from `SessionStart` on this machine). A new hook is trust-gated by a per-hook hash
  (`[hooks.state."<file>:<event>:<i>:<j>"] trusted_hash`); `features.hooks` alone does not trust it.
  `hooks.json` is shared with other tools (gentle-ai telemetry, moshi).

## Decision

1. **The plan reaches Codex through a `SessionStart` hook (the owner, option A).** `install-hooks` writes an
   entry in `~/.codex/hooks.json` (under `CODEX_HOME` when set) that runs the same `projection hook --event
   SessionStart` the Claude family runs, so one renderer writes the block for both. The entry is merged by
   ownership like the Claude settings (our entry is recognized and replaced; every foreign value, key and number spelling is
   kept byte for byte, and only the whitespace layout of the one `SessionStart` array we edit is rewritten, which
   Codex does not see because it trusts a hook by the hash of its normalized configuration), and `uninstall-hooks` removes only ours (the foreign values come back identical; the array layout is not restored). The alternative of widening the `gentle-dev`
   permission profile was rejected: it edits another tool's configuration, loosens the owner's sandbox, and
   keeps the dependence on a skill file and a binary outside the workspace.
2. **Instructions only in this unit (the owner, option A).** Codex has `PreToolUse` and `PostToolUse` hooks, so
   the gate and the read evidence can follow, but that needs the mapping of Codex's tool names and its
   input and denial formats and touches a guard. It is a separate unit (M12b). After this unit the table of
   M11 says: Codex instructions supported through the hook; enforcement, evidence and availability still
   unsupported with the limit written.
3. **Pi stays as it is (the owner, option A).** Pi obtains the plan when the agent runs the command and passed
   the live session. The table says so: instructions supported, invoked by the agent, nothing enforced.

## Assumptions of the implementer

- **The hook needs the human's trust once.** Merging the entry does not run it: Codex marks a new or
  modified hook as needing review and runs it only after the owner trusts it (`t` in the hooks view of the
  TUI, which stores the hash). `install-hooks` says so; `status-hooks` reports the entry as present, not as
  trusted. The acceptance script is not changed to bypass trust: a live Codex run before the owner trusts the
  hook is expected to fail, and that is the honest result.
- **Input and output formats come from the pinned schema** (`codex-rs/hooks/schema/generated/` of
  `rust-v0.148.0`), not from memory; the hook reads the working directory from the hook input as the Claude
  path does. A hook that cannot be used (unreadable input, no bound workflow) prints nothing and exits 0.
- **Not done here.** The gate, the evidence and the availability line for Codex; an `AGENTS.md` block
  (static, and the plan changes per workflow); any change to Pi.

## Consequences

- `install-hooks` now also touches a file of Codex. It is a merge with ownership and a backup, and a second
  run changes nothing.
- The `memory-plan` skill keeps working for Pi and for a Codex that can run the command; its text says that
  Codex gets the plan from the hook.
- The M11 live script is the proof: Codex passes only after the owner trusts the hook.

## Alternatives rejected

| Question | Rejected | Why |
|---|---|---|
| Channel | Widen the `gentle-dev` permission profile | Edits another tool's config, weakens the sandbox, keeps the skill and binary dependence. |
| Channel | A block in `AGENTS.md` | Static; the plan changes with the bound workflow. |
| Scope | Instructions, gate and evidence together | Large, high risk (a guard), needs the Codex tool and denial formats first. |
| Pi | Inject the plan from `labdrian-gate.ts` | More TypeScript and a Node test for a runtime that already passes. |
