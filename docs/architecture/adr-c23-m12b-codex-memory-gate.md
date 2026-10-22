# ADR: Codex gets the memory gate and the read evidence through PreToolUse and PostToolUse hooks, with its tool names adapted at the boundary (C23, M12b)

- **Status:** Accepted, 2026-10-21.
- **Decided by:** the owner, one question on 2026-10-21 (option A, the hexagonal one); the rest are facts from the pinned Codex sources and assumptions of the implementer.
- **Ledger:** `odd/tasks/consolidation-phase9.md`, task C23, unit M12b (follows M12, gap G11).

## Context

- M12 (PR #659) gave Codex the memory plan as instructions through a `SessionStart` hook. The gate (M4) and the read evidence (M7) still exist only for Claude Code, so a Codex session is told the plan and enforced nothing.
- **Codex 0.148 facts** (pinned `rust-v0.148.0`, explorer report of 2026-10-21):
  - `PreToolUse` and `PostToolUse` hooks read the same envelope as Claude's (`hook_event_name`, `cwd`, `tool_name`, `tool_input`, plus `tool_response` for Post) and the existing decoders already read it; extra fields (`turn_id`, `model`, `agent_id`) are ignored.
  - A denial is exit 0 with `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"..."}}`, which is what the engine already emits. Every failure of a hook (crash, timeout, invalid JSON, other exit code) **fails open**.
  - `matcher` is exact-name alternatives when it holds only letters, digits, `_` and `|`, and an unanchored Rust regex otherwise, applied to the tool's canonical hook name.
  - MCP tools are named `mcp__<server>__<tool>` with the server name sanitized to letters, digits and `_`: Engram reads arrive as `mcp__engram__mem_search` (the gate already recognizes them) but `longterm-mem` arrives as **`mcp__longterm_mem__query` and `__get`**, which the gate does not recognize, so both would be ungated and unrecorded.
  - PostToolUse sees the raw MCP result of a **successful** call, not of a failed one. Shell calls arrive as `Bash` and `apply_patch` keeps that name; the Claude `Skill` and `Read` of a skill file have no equivalent.
  - A new handler needs the human's trust once (per handler key, hash of event, matcher and handler).

## Decision

1. **The names are adapted at the boundary (the owner, option A).** The Codex entries run the same `projection hook --event PreToolUse|PostToolUse` with a source flag, `--source codex`, and an adapter in the hook layer translates the tool names Codex sends to the ones the policy already knows (`mcp__longterm_mem__query` to `mcp__longterm-mem__query`, the same for `get`; every other name is passed through unchanged) before the gate or the evidence sees the call. The gate, its classifiers and every Claude test stay as they are; without the flag the command behaves exactly as today. A new runtime adds an adapter, not a rule. Rejected: teaching the shared classifier both spellings, which puts Codex knowledge into the policy and touches the guard Claude uses.
2. **Two more entries in `hooks.json`, owned and merged like the `SessionStart` one.** `PreToolUse` and `PostToolUse`, each a group whose matcher names only the memory reads (the four Engram read tools, with or without a plugin prefix, and longterm-mem `query` and `get`, in the spellings Codex sends). `merge-codex-hooks` installs the three entries, replaces ours without touching foreign values, and a second merge changes nothing; `uninstall-codex-hooks` removes only ours; `install-hooks` over an install that has only `SessionStart` adds the other two. Each new handler needs the owner's trust once, so `status-hooks` reports three entries as present, never trusted.
3. **What the claims become.** Codex memory enforcement: partial, with the limits written (the Engram reads and longterm-mem `query` and `get` against the plan's sources, project and goal, as the Claude gate does; fails open because Codex does; no check of the procedural skills, `Bash` or `apply_patch`). Codex evidence: supported for successful MCP reads, recorded in the same sidecar and format as Claude's. Availability stays partial. The M11 table and the capability declaration change together, each with a proof by an engine hook process.

## Assumptions of the implementer

- The matcher is the narrowest one that the gate would act on after adaptation; it is a regex, written once in a table beside the adapter and pinned by a test that every name the gate recognizes in Codex's spelling matches and nothing else does.
- The adapter is a pure function in the adapter ring (`hookwire`), with a test of its table; an unknown `--source` is a usage error, and the default source is Claude.
- A hook that cannot use its input prints nothing and exits 0, as the Claude hooks do; Codex fails open anyway.
- Subagents are not told apart (`agent_id` is ignored), and a failed call is not recorded.
- **Not done here.** A gate for procedural skills, `Bash` or `apply_patch` in Codex; any change to Pi; any change to the policy or to Claude's behavior.

## Consequences

- The owner trusts two more handlers in the Codex TUI (`/hooks`, key `t`); the existing `SessionStart` trust is unaffected, because the new groups are new events and its handler key does not move.
- The live script can now exercise Codex: one allowed Engram read leaves a line in the sidecar, and a longterm-mem read the plan excludes is denied.
- `--source` is a new flag of an existing internal command; the Claude entries do not pass it, so what is installed for Claude does not change.

## Alternatives rejected

| Question | Rejected | Why |
|---|---|---|
| Names | The gate accepts both spellings | Codex knowledge inside the policy; touches the Claude guard. |
| Scope | Gate Bash and apply_patch in Codex | Not in the memory plan's sources; the Claude gate leaves `Bash` alone on purpose (M4). |
| Failure | Fail closed through a wrapper | Codex fails open on any hook failure and a wrapper cannot change that. |
