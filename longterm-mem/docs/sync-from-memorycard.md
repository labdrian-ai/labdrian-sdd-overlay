# Syncing from memorycard

`longterm-mem sync` takes an optional `--source`, `engram` (the default) or `memorycard`. With no `--source`, and
with `--source engram`, the command is the one that has always synced from Engram: its output and exit codes are
unchanged, which is what the hook-driven `engine/synctrigger` relies on. Only `--source memorycard` reads
memorycard, so no hook writes into the vault from memorycard until an operator runs it.

```
longterm-mem sync [--project P] [--vault DIR] [--dry-run] --source memorycard
```

The memorycard database is opened read-only, at `LONGTERM_MEM_MEMORYCARD_DB` or, when that is unset,
`~/.memorycard/memory.db` (`MEMORYCARD_DB` is memorycard's own variable and does not reach `longterm-mem`). Engram's
database is opened read-only too, to find the memories that duplicate one of its pages (below). A database that
cannot be opened is exit 4. `--source` with any other value is exit 2.

When either database could only be opened through its `immutable=1` fallback, the run works from the snapshot taken
when the connection was opened and says so on stderr, one line per store (`longterm-mem: sync: memorycard is being
read through the immutable=1 fallback, ...`), as `sync` and `promote` do for a frozen Engram. It is a declaration, not
a failure. A store that fails to close once the run is over is a `warning: close ...` line on stderr and changes no
exit code.

## What it does

1. Lists the project's memories whose `status` is `active` or `superseded` (an `archived` memory plays the part of
   Engram's soft-deleted observation and is not listed).
2. Keeps the memories that are eligible by the rule `promote` and the Engram `sync` use (R-007): pinned, or carrying
   a curated `topic_key` (not `sdd/`, `review/`, `delivery/`, `odd/` or `session/`). An `ingested/` key is curated.
3. Reads the vault's pages once, and for each eligible memory decides, with the same decision
   `promote --source memorycard --id N` makes:
   - no page: a page is created;
   - a page at an older `memorycard_revision`: the page is updated in place;
   - a page at the memory's revision: nothing is written, not even to the precedence store;
   - bridged (below): skipped and counted.
4. Propagates status (below): the page of a memory that memorycard has since superseded or archived is patched.
5. Rebuilds the vault's index and records the sync's completion, as the Engram `sync` does (R-031).

A page the vault holds for a memory is found by `(project, memorycard_id)`. A newer revision of a page that was
edited by hand since it was written is not written, and the run counts it as left alone.

## Status propagation

A promoted memory whose memorycard `status` becomes `superseded` or `archived` has its page's `status` and `related`
patched on the next `sync --source memorycard`, the counterpart of what the Engram `sync` does (R-033). memorycard's
supersede and archive change only the memory's `status` (and `superseded_by`) and add no version, so the page's
`memorycard_revision` stays current and the promotion pass has nothing to do for it: the two passes are independent.

- Lists every memory of the project whatever its status (the only listing that shows an archived one) and finds each
  one's own page by `(project, memorycard_id)`. A memory with no page is left alone. When a page of the vault could
  not be read and none belongs to a superseded or archived memory, it is left alone and named (exit 6), as for
  promotion.
- A page is patched only when it is behind its memory: its `status` or `related` differs from what the memory says.
  A page that already says it is not touched, not counted, and not predicted by the dry run.
- `status` becomes `superseded` or `archived`. A memory that memorycard has restored to active brings its page back
  from `superseded` or `archived` to what a render of the active memory gives (`mature` when pinned, otherwise
  `developing`). The status a human set by hand on the page of an active memory is left alone. `related` names the successor's page (`superseded_by`) in the form a
  page render uses, `[[<address>|<successor title>]]`, and is empty when the memory has no successor or the successor
  has no page. Because the promotion pass runs first, a page it created before its successor's page existed gets its
  link in the same run.
- Only those two lines of the frontmatter are written, never the body, even on a page edited by hand. The patch is
  recorded in the precedence store the way the Engram pass records it: only the frontmatter hash moves, so the
  body's divergence from the last write is not erased and a later newer revision of that page is still left alone.
- If memorycard cannot list the memories for this pass after promotion wrote pages, the index is still rebuilt and
  the sync recorded, and the run answers exit 4.
- The run and the dry run share one walk (`would patch: <address>` lines, a `patch N page(s)` count). The dry run
  reads the pages as they are before the run, so it does not predict a link the run adds to a page it creates.

## Bridged memories

A memory that `promote --source memorycard` would refuse (it is bridged by `import_map`, or has the content of an
Engram observation that already has a page) is never a failure of `sync`. It is skipped, nothing is written for it,
its page stays byte-identical, and the run counts it.

## Output and exit codes

```
longterm-mem: sync --source memorycard promoted 3 memory(ies); patched 1 page(s); skipped 1 bridged; left 0 locally edited page(s) alone
```

| Case | Exit |
|---|---|
| The run finished, whatever it skipped or counted as bridged or left alone | 0 |
| A page that could not be read may be a memory's own, so nothing was written for that memory | 6 (the memory is named on stderr) |
| memorycard or Engram cannot be opened or listed | 4 |
| The vault's index could not be rebuilt | 5 |
| Any other failure (a page that could not be written, the sync-state record) | 1 |
| `--source` is neither store | 2 |

What landed is reported whether or not the run failed. One memory that fails does not stop the others, and the index
rebuild and the sync-state record still run, as for the Engram `sync`.

## `--dry-run`

```
longterm-mem: sync --source memorycard --dry-run: would promote 3 memory(ies), patch 1 page(s), skip 1, 1 bridged, into <vault>
  would promote: <title>
  would patch: <address>
longterm-mem: sync --source memorycard --dry-run: nothing was written
```

The dry run walks the same memories through the same decision as the run and stops where the run would write: no
page, no precedence entry, no index rebuild and no sync-state record. It answers the exit code of the run for the
same failure: 4 when memorycard cannot be listed, 6 when memories are left alone for an unreadable page, 1 for any
other failure; the index rebuild (5) is a step it does not take. A page that was edited by hand is only found
when the run tries to write it, so the dry run counts such a memory among those it would promote and the run then
reports it as left alone.

Run it before the first real `sync --source memorycard` on a vault: memorycard also holds the imported Engram
history, so a first run can promote many eligible memories that never had a page.
