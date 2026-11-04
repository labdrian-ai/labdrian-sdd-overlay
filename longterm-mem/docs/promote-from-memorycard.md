# Promoting from memorycard

`longterm-mem promote` takes an optional `--source`, `engram` (the default) or `memorycard`. With no `--source` the
command is the one that has always promoted an Engram observation. The README's command table has not been
updated for this flag yet; this page is its description.

```
longterm-mem promote [--project P] [--vault DIR] --source memorycard --id N
```

`N` is a memorycard memory id. The database is opened read-only, at `LONGTERM_MEM_MEMORYCARD_DB` or, when that is
unset, `~/.memorycard/memory.db`. A database that cannot be opened is exit 4, as Engram's is.

## What it writes

A memorycard promote only ever creates a new page. It writes no page that exists.

The page has the frontmatter of an Engram page, with these keys in place of the `engram_*` ones:

| Key | Value |
|---|---|
| `memorycard_id` | the memory's id |
| `memorycard_type` | the memory's type |
| `memorycard_revision` | the memory's next version number: the highest `memory_versions.version`, plus one |
| `project` | the memory's project |

`related` holds the page of the memory in `superseded_by`, when that memory has a page in the same project; no
other relation of memorycard is read. `status` is `superseded` or `archived` when the memory is, `mature` when it
is pinned, `developing` otherwise.

A page is found again by `(project, memorycard_id)`.

## What it refuses or leaves alone

| Case | Result |
|---|---|
| The memory has no page in its project | a page is created (`promoted c-NNNNNN (created)`) |
| The memory already has a page in its project | nothing is written; exit 0, `memorycard memory N already promoted as c-NNNNNN; the page is left as it is`. A later revision of the memory does not reach that page |
| The memory is bridged: its `import_map` row (source `engram`, `source_key` = an Engram observation id) names an observation that already has a page | refused, exit 6 (`registration_conflict`); the message names that page, and the page stays byte-identical. No page is created |
| A page of the vault could not be read and none that could be read decides the memory | nothing is written, exit 6, as for an Engram observation |
| No such memory | exit 7 |
| `--source` is neither store | exit 2 |

A memory imported from an Engram observation that has no page is not bridged to a page, and is promoted.

The accepted tradeoff (feature document `odd/tasks/memorycard-engram-replacement.md`, L16 and L18): the 558 pages
that carry `engram_id` keep resolving against Engram's database and are never touched, so a bridged memory, and a
memory with the same content as one of those observations, cannot be promoted from memorycard. Its history stays on
the Engram page.

The MCP `promote` tool still promotes Engram observations only.
