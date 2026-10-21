# ADR: skill candidates have a typed lifecycle, a read port and a list command, and the detector is a loaded skill (C23, M10)

- **Status:** Accepted, 2026-10-20.
- **Decided by:** the owner, four questions asked one at a time on 2026-10-20 (all four: option A, the hexagonal one).
- **Ledger:** `odd/tasks/consolidation-phase9.md`, task C23, unit M10 (gaps G8 and G9, Engram #4133).

## Context

- **G8.** The candidate detector is `skills/_shared/procedural-candidate-detection.md` (970 lines of
  contract). It is deployed (`overlay.manifest`), but nothing puts it in a session: the only references are
  one pointer line in `skills/memory-plan/SKILL.md` and in the instructions block of the plan
  (`engine/projection/render.go`), both saying "see section 5". Only tests read the file. Its section 5
  defines two triggers, T1 (right after a proactive `mem_save` of type bugfix, pattern, discovery or
  decision) and T2 (at session close, beside `mem_session_summary`), and says no hook is added (OQ-2).
- **G9.** A candidate is an Engram observation under `procedural/candidates/{repeated-success|failure-recovery}/...`
  with a `Status` of `observing|emitted|rejected|drafted|registered|promoted|retired`, an append-only
  `History` and a transition table that names the actor (agent or human) and the tier (section 9). A draft
  is another observation under `procedural/drafts/{kind}/{slug}` with its own `Status`
  (`open|registered|abandoned`) and the `ForRevision` latch (section 8). Only the agent writes them, with
  `mem_save`, following prose. No Go code reads either: the only typed part is the shape of the candidate
  key (`engine/skills/project_lock.go`, `candidateKeyPrefix`), checked by `project-register` and
  `project-revise`. No command lists what exists, so the alias search the contract requires (section 1) and a
  human's review both rely on the agent's recall.
- The engine is its own Go module and has no SQLite driver and no Engram reader; longterm-mem has one
  (`longterm-mem/internal/engram`, behind `skillstale.ObservationLister`), and its packages are `internal`.
  `memory.TopicWithoutGoal` (longterm-mem, M5) already says that "the skill candidates" read the topic
  without its goal prefix, and nothing uses it for that yet.

## Decision

1. **Lifecycle (the owner, option A).** The domain types both vocabularies as the contract has them:
   `CandidateStatus` (seven values) and `DraftStatus` (three), and the transition table of section 9 is a pure
   function over `(from, to, actor)`, the actor being part of the rule. The engine only reads and
   validates; "no engine verb ever writes `Status`" stays true. A record with a status outside the
   vocabulary, a `History` line that is not a legal transition, or a draft whose candidate is missing is
   reported as an anomaly, never repaired.
2. **Storage (the owner, option A).** Engram stays the one durability store and the agent stays the one
   writer. The typed read goes through a port the domain owns, `CandidateSource`, with an adapter of its own
   composed by `cmd`. No second store, no cache, no Go write path into Engram. The domain parses the record
   block of sections 2 and 8 (`Kind`, `Candidate`, `Aliases`, `Status`, `Threshold`, `OccurrenceCount`,
   `Disposition`, `Draft`, `ForRevision`, `History`) and reads a topic under a goal as `goal:<id>/` plus the
   key, normalized with the rule `TopicWithoutGoal` states.
3. **Loading the detector (the owner, option A).** A short operational skill, `procedural-candidates`,
   carries the T1/T2 procedure, the record shape, the alias rule, the do-not-capture list and one explicit step:
   run `skills candidates` before creating or updating a candidate. The long contract stays the normative
   reference the skill cites by section. The instructions block and the `memory-plan` skill tell the agent to
   load it, and it reaches Claude, Codex and Pi through the projection `memory-plan` uses. A contract test
   keeps the skill aligned with section 5 and the table of section 9 and checks that what it cites exists. No
   hook is added and the contract is not changed.
4. **The command (the owner, option A).** `skills candidates [--project-root <abs>] [--status <s>]...
   [--kind <k>] [--drafts] [--json]`, read-only. One line per candidate, sorted by key: status, key,
   `count/threshold`, `Disposition` and the draft when there is one; `--drafts` adds the drafts with their
   `Status` and `ForRevision`; `--json` prints the same in a stable shape. Anomalies are a section of their
   own. Exit 0 when the source was read (an empty list included), 1 for usage, and a code of its own when the
   source cannot be read: an unavailable Engram says why and is never printed as "no candidates". The use
   case lives in `engine/skills/app` over `CandidateSource`, `cmd` composes, usage and docs are updated.

## Assumptions of the implementer

- **Adapter.** The engine cannot import longterm-mem (another module, internal packages) and has no SQLite
  driver, so the adapter runs the read-only `engram export <file> --project <name>` into a scratch file and
  filters by topic-key prefix, as the session-time probe of M8 runs `engram stats`. `engram search` is not
  used because it truncates the content. The project name is the Engram name of the workflow's identity
  (M2). A failed or missing `engram` is the "cannot be read" outcome.
- **Not done here.** A `show <key>` or `check` verb, a hook for T1, and any write verb are left out; each can
  be added without undoing this.

## Alternatives rejected

| Question | Rejected | Why |
|---|---|---|
| Lifecycle | Type only the candidate; one unified lifecycle | The first leaves `ForRevision` and `abandoned` without a model; the second breaks the contract and two specs. |
| Storage | A store of files in the project; Engram plus a file cache | Two sources that diverge, and the contract says Engram is the sole store; the cache adds invalidation for a list. |
| Loading | A PostToolUse hook on `mem_save`; the detector inlined in the block; skill plus hook | The hook contradicts OQ-2 and exists only in Claude Code; inlining swells every prompt and M6 rejected mixing it with the plan. |
| Command | A `list|show|check` family; a verb that writes transitions | More surface than the value; writing contradicts the contract and decision 2. |
