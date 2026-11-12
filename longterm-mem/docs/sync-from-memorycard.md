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
4. Rebuilds the vault's index and records the sync's completion, as the Engram `sync` does (R-031).

A page the vault holds for a memory is found by `(project, memorycard_id)`. A newer revision of a page that was
edited by hand since it was written is not written, and the run counts it as left alone.

## Bridged memories

A memory that `promote --source memorycard` would refuse (it is bridged by `import_map`, or has the content of an
Engram observation that already has a page) is never a failure of `sync`. It is skipped, nothing is written for it,
its page stays byte-identical, and the run counts it.

## Output and exit codes

```
longterm-mem: sync --source memorycard promoted 3 memory(ies); skipped 1 bridged; left 0 locally edited page(s) alone
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
longterm-mem: sync --source memorycard --dry-run: would promote 3 memory(ies), skip 1, 1 bridged, into <vault>
  would promote: <title>
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

## Not covered here

Status propagation for pages that carry `memorycard_id` (a memory that became `superseded` or `archived`) is a later
step of the same follow-up; until then `sync --source memorycard` promotes and updates, and does not patch a page's
`status`.
