# ADR: the goal filter of `longterm-mem query` (C23, M5)

- **Status:** Accepted, 2026-10-19.
- **Decided by:** the owner, four questions asked one at a time on 2026-10-19. The goal filter itself
  (decision 5 of `adr-c23-m1-memory-sources.md`) was decided in M1.
- **Ledger:** `odd/tasks/consolidation-phase9.md`, task C23, unit M5 (gap G4, Engram #4133).

## Context

M1 decided that a record created under a goal carries the tag `goal:<id>`, in the frontmatter `tags`
of a longterm-mem page and in the Engram topic, and that under scope goal `longterm-mem query`
returns that goal's records and the project's records with no goal tag. M1 left open how the topic
spells the goal and which unit writes the page tag. M3 stores the goal id in the workflow log, and
the plan carries it as `Filters.GoalID` (`engine/memoryscope/memoryscope.go:177-180`).

Nothing in longterm-mem knows about goals today. `query.Run` (`longterm-mem/internal/query/query.go`)
asks each source on its own and caps each at `top`:

- `engram-fts` filters in SQL (`internal/engram/search.go`), by project and type;
- `engram-embed` ranks the index entries and looks each one up in the live observations, which carry
  `TopicKey` (`internal/query/embedarm.go`, `internal/memory/observation.go:34`);
- `vault` asks the vault's own script, whose candidates carry no tags (`internal/query/vault.go`).

The rows that come out carry no topic and no tags, so the filter cannot run after the merge. The
embedding index stores only `{engram_id, fingerprint}` per entry (`internal/vecindex/index.go`), so
it needs no change. A promoted page's tags are `{"memory", <type>}` (`internal/promote/page.go:56`).
The gate checks a query's `project` only (`engine/projection/gate.go`, `longtermProjectGate`).

## Decision

1. **The topic prefix (the owner, 2026-10-19).** An Engram record created under a goal has a
   `topic_key` that begins with `goal:<id>/`, for example `goal:g-42/architecture/auth`. A topic
   without that prefix belongs to no goal. The same topic under two goals is two keys, so an upsert
   under one goal never overwrites the other's record. A goal id is one safe name, so the first `/`
   after `goal:` ends it.
2. **M5 writes the page tag, in promote (the owner, 2026-10-19).** When the observation's topic
   carries the prefix, the promoted page's `tags` gain `goal:<id>` after `memory` and the type.
   Pages promoted before M5 do not change: no observation of theirs carries the prefix.
3. **`query` takes a `goal`, and the gate checks it (the owner, 2026-10-19).** The MCP `query` tool
   gains an optional `goal` argument and the command a `--goal` flag. With a goal, each source
   returns that goal's records and the records with no goal, and leaves out the records of other
   goals; without one, nothing changes. Under scope goal the PreToolUse gate denies a query whose
   `goal` is missing or differs from the plan's `Filters.GoalID`, as it denies a missing or different
   `project`. Under any other scope the argument passes unchecked.
4. **The vault is asked for more (the owner, 2026-10-19).** With a goal, `query` asks the vault for
   three times `top` candidates, up to a fixed bound, filters them by the `tags` of each page's
   frontmatter, and keeps the first `top`. Without a goal the vault is asked exactly as today.

### Where each source filters

| Source | Where the goal is applied |
|---|---|
| `engram-fts` | in the SQL, before the limit, so the bm25 order and the limit hold |
| `engram-embed` | on the live observation's `TopicKey`, before the cut at `top` |
| `vault` | on the page's frontmatter `tags`, after an enlarged retrieval (decision 4) |

### Alternatives considered

| Decision | Rejected | What choosing it would have meant |
|---|---|---|
| 1 | `goal:<id>` as any segment of the topic | A topic with `goal:` in the middle by accident would count as a goal's, and SQL could not anchor the match. |
| 1 | The whole topic is `goal:<id>` | Every record of a goal would share one key, and an upsert would overwrite the last. |
| 2 | M6 writes the page tag | Until M6 the vault would filter nothing under a goal, and M5's vault side could not be tested. |
| 3 | Deny only a different goal | A query without `goal` would read every goal's records under scope goal. |
| 3 | No gate check | Scope goal would rest on instructions alone. |
| 4 | Accept fewer vault rows | A goal among many pages of other goals could get almost nothing from the vault. |

## Consequences

- **No format change.** The embedding index, the vault's index and the result row keep their
  shapes; a query without `goal` prints what it prints today, so the existing goldens hold.
- **The tool schema changes.** The `query` tool lists one more optional property, so the MCP
  handshake golden is re-recorded; the gate's `QueryArguments`, its decode in `hookwire` and its
  wiring gain the goal.
- **The gate stays total.** A plan that cannot be computed, or a call whose arguments cannot be read,
  is an allow, as before. A workflow without a goal id under scope goal cannot exist (`Resolve`
  requires one).
- **The prefix says only the goal.** Every other rule that reads a topic reads what follows the
  prefix (`memory.TopicWithoutGoal`): promotion's curated topics, so `goal:g-42/sdd/x` is excluded
  as `sdd/x` is, and the skill candidates, so a candidate record made under a goal is still that
  candidate's.
- **M6** tells the agent to create records under a goal with the topic prefix and to pass `goal` to
  `query`.
- **Accepted cost.** A record created under a goal without the prefix is visible to every goal, as
  M1 accepted for records from before M5.
