# ADR: memory metrics and smoke tests are handed to Phases 10 and 11 as a contract, not built in Phase 9 (C23, M13)

- **Status:** Accepted, 2026-10-26.
- **Decided by:** the owner, to "does M13 build metrics now, or only leave the contract for Phases 10
  and 11? I recommend the hand-off: a short ADR and no code": "si deacuerdo".
- **Ledger:** `odd/tasks/consolidation-phase9.md`, task C23, unit M13 (Engram #4133).

## Context

- **What M13 was.** One line in the ledger, "Memory metrics and smoke tests for Phases 10 and 11", and
  one line in the gap analysis (Engram #4133), "M13: Phase 10/11 memory metrics and smoke". None of the
  gaps G1-G15 maps to it; G14, the closest, was closed by M11.
- **What Phases 10 and 11 are.** Two rows of `openspec/decisions/standalone-platform-roadmap.md`, both
  **unverified** and with no task ledger. Phase 10 (Observability and governance) is to measure, among
  other things, "latency/abstention/memory availability; exclude sensitive data and report missing
  memory/runtime honestly". Phase 11 (Release and distribution) is to prove "clean install, explicit
  config, runtime connection, bounded workflow, restart/resume, update, rollback, uninstall, data
  preservation, and second runtime".
- **Phase 9's purpose.** It pays down debt before Observability and Release are built
  (`odd/tasks/consolidation-phase9.md`, header). A metrics surface built now would be designed before the
  observability port of Phase 10 exists, and would likely be rebuilt around it.

## Decision

M13 is this document. It builds nothing. It states what the memory wiring of C23 (M1-M12b) already
records, what Phase 10 must measure from it, what has no source yet, and which memory checks Phase 11's
release proofs must include. Phase 10 and Phase 11 decide the shape (verb, module, port); this ADR only
fixes what they must cover and from where.

## What already exists to measure from

| Source | What it records | Where |
|---|---|---|
| Read evidence (M7, M12b) | One JSON line per memory read that ran: `tool`, `source` (`engram`, `longterm-mem`, `procedural-skills`), `project`, `goal`, `ids` of a read by id, `skill`, `session`, `response_sha256`, `response_bytes`. No query text, no content. | `<state home>/labdrian/workflows/<project>/<workflow>.memory.jsonl`, written by PostToolUse; Codex through `--source codex` (`adr-c23-m7-read-evidence.md`, `adr-c23-m12b-codex-memory-gate.md`) |
| Availability (M8) | Per source of the plan, at session start: readable or not, with the reason, as degradation lines. | The SessionStart hook (`adr-c23-m8-session-availability.md`) |
| Procedural candidates (M10) | Candidates and drafts with their status, and reconciliation anomalies. | `skills candidates --json` (`adr-c23-m10-skill-candidates.md`) |
| Per-runtime claims (M11) | Which memory surfaces each runtime has proof for (delivery, enforcement, evidence, availability), and which are declared unsupported. | `engine/cmd/memory_acceptance_test.go`, in CI (`adr-c23-m11-memory-acceptance.md`) |
| Cost baseline | Tokens and cost only; no memory fields. | `telemetry/`, CI job "Telemetry Cost Baseline" |

## What Phase 10 must measure for memory

1. **Reads by source, runtime and workflow**, counted from the read evidence; reads by id counted apart.
   Counts and hashes only: the evidence holds no content and a metric must not add any.
2. **Availability per source and runtime**, from the M8 probes: how often each source was unreadable,
   and why.
3. **Coverage of the plan**: for a bound workflow, which of the plan's sources were read at least once.
4. **Procedural candidates and drafts** by status, from `skills candidates`.
5. **Missing is reported as missing, never as zero.** A runtime or a source with no evidence surface is
   reported unsupported, with the limit, as the M11 claim table already does. Today: Claude has
   instructions, enforcement (partial), evidence and availability; Codex has a partial gate and evidence
   that fail open (M12b); Pi has the instructions only.

## What has no source yet

- **Gate denials.** PostToolUse sees only calls that ran (M4, M7), so a denied memory read leaves no
  record. Measuring denials needs a capture at the gate.
- **Latency and abstention** of memory reads: nothing records them.

Phase 10 decides whether to add these captures; until then a report must name them as unmeasured.

## What Phase 11's release proofs must include for memory

After install, update, rollback and uninstall in a scratch home:

1. The memory hooks (SessionStart, PreToolUse, PostToolUse) are present and current for each runtime
   that has them, and absent after uninstall.
2. The longterm-mem MCP entry is registered and its binary runs (`longterm-mem doctor`).
3. Data preservation: the read-evidence sidecars and the Engram store survive update and rollback, and
   uninstall leaves them unless the person asks otherwise.
4. The checks run as deterministic process tests in CI, built on the M11 claim table; live runtime
   sessions stay opt-in and authorized per runtime, as M11 decided.

## Consequences

- C23 closes with M13; no code, no new verb, no change to the evidence format.
- Phase 10 and Phase 11 inherit this list as acceptance input for their own ledgers.
- If Phase 10 adds a denial capture, the read-evidence format may grow a field; the version field
  (`version: 1`) is there for that.
