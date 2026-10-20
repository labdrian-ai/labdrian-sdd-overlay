# ADR: availability is probed at session time and reported as degradation lines (C23, M8)

- **Status:** Accepted, 2026-10-20.
- **Decided by:** the owner, four questions asked one at a time on 2026-10-20.
- **Ledger:** `odd/tasks/consolidation-phase9.md`, task C23, unit M8 (gap G10, Engram #4133).

## Context

Availability of the memory sources is probed in one place only: `Lifecycle.observationsFor`
(`engine/workflow/lifecycle.go`), once, when a workflow is created, through the
`workflow.DependencyProber` port. The real prober, `engine/capability/presence`, only stats files
(`~/.engram/engram.db`, `~/.labdrian-overlay/longterm-mem-registration.json`) and by contract never opens
a file or starts a process. Two defects follow:

- `memory:procedural-skills` is a hard-coded case that always reads unavailable ("no presence check
  exists for procedural skills"), and the profile goldens pin that wrong answer.
- The result is a record of the moment of creation. `InstructionsBlock`, which the `SessionStart` hook of
  M6 renders, takes no availability input, so a session never learns that a source in its plan cannot be
  read.

## Decision

1. **A real check at session time (the owner).** `SessionStart` probes each source in the plan by
   asking it, not by looking for a file: Engram's database is opened read-only, the longterm-mem binary
   is run. The owner chose this over a stat-only probe knowing it breaks the presence prober's
   "open nothing, start nothing" contract. The contract stays true for the create-time prober; the
   session-time probe is a new port with its own adapter. Every check has a short deadline, and a check
   that fails, times out or cannot start reports its source unavailable. `SessionStart` never waits
   longer than the sum of the deadlines and never fails because of a probe.
2. **procedural-skills (the owner).** Available when `.labdrian/procedural-skills.lock.json` is
   readable and valid, even if it lists no skill, through the existing `ProceduralSkillsReader`. No lock
   means none installed and reads unavailable with that reason, so the line tells the agent not to expect
   any; an unreadable or corrupt lock reads unavailable with the reason the reader gives.
6. **Where it runs (assumption of the implementer).** Only the `SessionStart` hook probes. The gate-task
   rewrite runs on every `Agent` call and `memory plan --instructions` is a command, so both keep the
   block without availability lines; a subagent gets the plan, not the session's probe. The engine has no
   SQLite driver (only longterm-mem does), so "opening Engram's database" is done by running the read-only
   `engram stats`, which opens it.
3. **longterm-mem (the owner).** Available when the binary answers and an MCP entry exists in any of the
   Claude, OpenCode or Codex configurations, through `runtime.LongtermMemAdapter`. The owner chose any
   runtime over Claude only, knowing the line can say available when Claude has no such tool loaded.
4. **Two probes, one rule (the owner).** The create-time record is kept, and the hard-coded
   procedural-skills case is replaced by the lock check of decision 2, so the record stops being wrong.
   `SessionStart` adds the fresh probe of decision 1.
5. **Degradation is information only (the owner).** For each source of the plan that the probe reports
   unavailable, the instructions block gets one line saying not to read it and why. The gate and the
   plan are unchanged: nothing is denied because of a probe, and a false negative of the probe costs a
   line of advice, not a blocked read.

## Consequences

- `projection.InstructionsBlock` takes the availability of the plan's sources; the `SessionStart` text
  changes for a plan with an unavailable source, and the pinned instructions golden and the cases of
  `projection/instructions_test.go` are updated together.
- A new port in `projection/app` and an adapter in `cmd`; the gate (`engine/projection/gate.go`) and the
  `UserPromptSubmit` plan line do not change.
- A session can still read a source the probe called unavailable, and the gate still allows it.
