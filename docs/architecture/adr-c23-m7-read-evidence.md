# ADR: a PostToolUse hook records memory-read evidence (C23, M7)

- **Status:** Accepted, 2026-10-19.
- **Decided by:** the owner, four questions asked one at a time on 2026-10-19. Which reads are memory
  reads was decided in M1 (`adr-c23-m1-memory-sources.md`) and M4 (`adr-c23-m4-memory-gate.md`).
- **Ledger:** `odd/tasks/consolidation-phase9.md`, task C23, unit M7 (gap G5, Engram #4133).

## Context

Since M4 the projection family's `PreToolUse` gate (`engine/projection/gate.go`) denies a memory read
the plan excludes. What a read the plan allowed actually returned is recorded nowhere. G5: "there is
no PostToolUse hook and no memory evidence in the workflow log or handoffs."

- `projection hook` knows three events, `PreToolUse`, `UserPromptSubmit` and `SessionStart`
  (`engine/hookwire/hookwire.go`). Every path through `HookService` (`engine/projection/app/hook.go`)
  is read-only.
- No decoder parses `tool_response`, and `hookwire` refuses a whole hook input over 1 MiB
  (`MaxEnvelopeBytes`). A memory read can return more than that.
- The workflow log (`engine/workflow/filelog/store.go`) is a hash-chained JSONL file. `Store.Append`
  takes a flock, re-reads the log and rewrites the whole file with the new line, so each append costs
  the size of the log.
- A role handoff already carries `evidence[]` items `{kind, ref, sha256}`, described as "a pointer to
  content, not a copy" (`engine/roles/handoff.go`).

## Decision

1. **A sidecar per workflow (the owner, 2026-10-19).** The evidence goes to an append-only file next
   to the workflow log, `<state home>/labdrian/workflows/<project_id>/<workflow_id>.memory.jsonl`
   (mode 0600), behind its own port. The chained log is unchanged and its `EventVersion` stays 3.
   A handoff that cites the reads points to the sidecar with an `evidence[]` item
   `{kind: "memory_reads", ref: <sidecar path>, sha256: <digest of the file>}`.
2. **Reads only (the owner, 2026-10-19).** The hook records exactly the calls the M4 gate classifies
   as memory reads: the Engram reads `mem_search`, `mem_context`, `mem_get_observation` and
   `mem_timeline`; `longterm-mem` `query` and `get`; and a procedural skill read (`Skill`, or `Read`
   of a `SKILL.md` the lock names). It reuses the gate's classifiers; writes (`mem_save`,
   `mem_update`, `promote`) are not recorded and get no classifier.
3. **A digest and a size, never content (the owner, 2026-10-19).** From `tool_response` the record
   keeps only its SHA-256 and its length in bytes. Project, goal and ids come from `tool_input`. The
   hook reads its input hashing `tool_response` as it streams, so a response over 1 MiB is still
   recorded; the rest of the envelope keeps today's bound.
4. **A failure warns and goes on (the owner, 2026-10-19).** The hook exits 0, as every projection hook
   does. When the record cannot be written (the lock, the disk, permissions), it returns one
   `additionalContext` line telling the agent that the evidence of that read was not recorded, and
   why. The tool already ran; PostToolUse cannot undo it.

### The record

One JSON object per line, `version` 1:

```
{"version":1,"at":"2026-10-19T22:40:00Z","tool":"mcp__plugin_engram_engram__mem_search","source":"engram","project":"labdrian-sdd-overlay","goal":"","ids":[],"skill":"","tool_use_id":"toolu_...","session_id":"...","response_sha256":"<64 hex>","response_bytes":5120}
```

- `source` is `engram`, `longterm-mem` or `procedural-skills`, the plan's source names.
- `ids` holds the id a read by id names (`mem_get_observation`, `mem_timeline`, `longterm-mem get`);
  it is empty otherwise. `skill` names a procedural skill read; it is empty otherwise.
- Every string from the input is sanitized to one line and bounded, as the projection already does.
- The query text is not kept.

### Alternatives considered

| Decision | Rejected | What choosing it would have meant |
|---|---|---|
| 1 | A `memory_read` kind in the workflow log | One hash chain for everything, but `EventVersion` 4, new admission rules, and a whole-log rewrite under the lock on every memory read. |
| 2 | Reads and writes | A full trace of memory use, but new write classifiers and a sidecar that is no longer read evidence. |
| 3 | A digest and a bounded excerpt | What the agent saw is auditable without the source, but memory content is copied to the workflow's disk. |
| 4 | Silence | No noise, and evidence lost without anyone knowing, the gap G5 names. |

## Consequences

- **A new hook event.** `install-hooks` adds `PostToolUse` entries to the projection family, with the
  matchers the `PreToolUse` gate's memory entries use. `projection hook` accepts the event, and the
  status label names it. Removing the family removes them.
- **The first projection path that writes.** Only this hook writes, and only the sidecar. Appends take
  the sidecar's own lock, so concurrent reads of parallel subagents serialize on one small file, never
  on the workflow log.
- **Only reads that ran.** A read the gate denied never runs, so it has no record.
- **Nothing without a binding (assumption, following `readBound`).** No bound workflow, a binding the
  workflow does not own, or a closed workflow: the hook records nothing and says nothing.
- **Handoffs are not rewritten.** Nothing copies the sidecar into a handoff; a role that cites its
  reads adds the `memory_reads` evidence item itself. Shaper handoff v4 has no evidence slot and
  gets none here.
- **Codex and Pi** get no record; their hooks are M12.
