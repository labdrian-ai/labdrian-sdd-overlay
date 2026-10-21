# ADR: the memory wiring is accepted per runtime, from one table of claims, with live sessions as a separate opt-in step (C23, M11)

- **Status:** Accepted, 2026-10-21.
- **Decided by:** the owner, one scope decision and three questions asked one at a time on 2026-10-21.
- **Ledger:** `odd/tasks/consolidation-phase9.md`, task C23, unit M11 (gap G14, Engram #4133).

## Context

- **G14.** Nothing proves that the memory wiring of M1-M10 works from a session's point of view. The
  automated proof ends at the engine: `TestPhase7Acceptance` runs the built engine as separate processes
  with realistic Claude hook JSON, and the M6, M7 and M8 tests drive single hooks. No test chains
  SessionStart, the gate and the evidence, and none runs a real runtime. The capability claim for memory
  enforcement says so (`engine/capability/declarations.go`, partial).
- **Scope.** The ledger asked for Claude Code only. On 2026-10-21 the owner widened it to Claude Code,
  Codex and Pi.
- **What each runtime receives today.** Claude: the plan as instructions (SessionStart, the subagent
  rewrite), partial enforcement in the gate, read evidence, availability lines. Codex and Pi: only the
  `memory-plan` and `procedural-candidates` skills, which an agent has to invoke; no enforcement, evidence or
  probe (that is M12, G11). The three CLIs have a non-interactive mode (`claude -p`, `codex exec`,
  `pi -p`).
- **A defect found while mapping.** The Pi package copied only the two gate contracts into
  `skills/_shared/`, so the detector document that both memory skills cite as their normative reference was
  not in the package.

## Decision

1. **One table of claims, a process test, live runs apart (the owner, option A).** A table in Go says, for
   each runtime, which surfaces it claims (instructions, enforcement, evidence, availability) and the proof
   of each. A test builds the engine and runs it as separate processes, like `TestPhase7Acceptance`: for
   Claude the chain SessionStart, gate and evidence over one bound workflow; for Codex and Pi the skills
   deployed into a scratch home, the normative reference resolved from there, and
   `labdrian memory plan --instructions` run. It runs in CI. Sessions of the real CLIs are a separate,
   opt-in step (decision 3).
2. **The claim of Codex and Pi is what they receive (the owner, option A).** Delivery is proven;
   enforcement, evidence and availability are declared unsupported with the limit written, and the test
   fails if a runtime claims a surface it has no proof for. M12 flips those claims on the same table.
3. **Live sessions are an opt-in script, one run per runtime the owner authorizes (the owner, option A).**
   The step needs an environment variable and the owner's authorization of each runtime. It runs `claude -p`,
   `codex exec` and `pi -p` in a scratch repository with a bound workflow, state isolated in a scratch
   directory, the real home only for the credentials. A run passes when the agent states the sources of the
   plan; for Claude, also when a memory read leaves a line in the evidence sidecar. The result goes into the
   ledger. It never runs in CI and never runs by default.
   **Amended 2026-10-21 after the first live runs:** the verdict is one exact report line that carries a per-run
   random goal id, which exists only in the workflow state and the plan (profile `incident-recovery`, scope
   goal), never in the prompt, the repository or its name. The first verdict looked for `engram` and `proj-1`
   in the transcript, and Codex passed it by guessing both from the prompt and the directory name without ever
   having received the plan.
4. **The Pi package carries what its skills cite.** A skill copied into the package may cite
   `skills/_shared/<name>.md`; the build copies each cited document and refuses a citation the overlay cannot
   satisfy. The alternative of a fixed extra file was rejected: it would break every fixture, and an
   optional copy would hide this very defect.

## Consequences

- The Pi package now also carries the other shared documents the registered skills cite
  (`ingested-observation-contract.md`, `openspec-convention.md`, `persistence-contract.md`,
  `pre-sdd-contracts.md`), found by the differential check against `origin/main`. They were cited and absent
  before; the owner's `labdrian apply` rebuilds the package.
- No claim of the capability table changes in this unit, except where the table of decision 1 shows a claim
  without proof.
- A live run is evidence of one session on one day, not a guarantee; the ledger records it as such.

## Alternatives rejected

| Question | Rejected | Why |
|---|---|---|
| Harness | Shell scripts under `tools/` only | Does not run in CI, has no verifiable table of claims, duplicates the process tests. |
| Codex and Pi claim | The same four claims everywhere, red until M12 | Leaves CI red or the tests skipped for a unit that is not this one; the table already shows the gap. |
| Live runs | A manual checklist only | Nothing repeatable, nothing the engine can check. |
| Pi package | A fixed list of extra shared files | Breaks every fixture, and says nothing about what a skill actually cites. |
