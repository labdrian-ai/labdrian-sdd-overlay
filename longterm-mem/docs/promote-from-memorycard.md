# Promoting from memorycard

`longterm-mem promote` takes an optional `--source`, `engram` (the default) or `memorycard`. With no `--source` the
command is the one that has always promoted an Engram observation. The MCP `promote` tool takes the same optional
`source`; with `source: memorycard` its `engram_id` argument is a memorycard memory id.

```
longterm-mem promote [--project P] [--vault DIR] --source memorycard --id N
```

`N` is a memorycard memory id. The memorycard database is opened read-only, at `LONGTERM_MEM_MEMORYCARD_DB` or, when
that is unset, `~/.memorycard/memory.db`. Engram's database is opened read-only too, to find the memories that
duplicate one of its pages (below). A database that cannot be opened is exit 4.

## What it writes

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

A page is found again by `(project, memorycard_id)`. A page that carries `engram_id` is never written by a
memorycard promote. A page that carries `memorycard_id` is updated in place, the way the Engram path updates its
own, when the memory's revision is newer than the page's; an equal revision writes nothing, and a page edited by
hand since it was written is left alone (exit 6).

## What it refuses or leaves alone

| Case | Result |
|---|---|
| The memory has no page in its project | a page is created (`promoted c-NNNNNN (created)`) |
| The memory has a page in its project at the same revision | nothing is written; exit 0, `memorycard memory N already promoted as c-NNNNNN; the page is left as it is` |
| The memory has a page in its project and a newer revision | the page is updated in place (`promoted c-NNNNNN (updated)`) |
| The memory is bridged: its `import_map` row (source `engram`, `source_key` = an Engram observation id) names an observation that already has a page | refused, exit 6 (`registration_conflict`); the message names that page, and the page stays byte-identical. No page is created |
| The memory has no `import_map` row and its `content_hash` is the hash memorycard would compute for an Engram observation of the same project that already has a page | refused exactly like a bridged memory, the message saying the content is the same |
| A page of the vault could not be read and none that could be read decides the memory | nothing is written, exit 6, as for an Engram observation |
| No such memory | exit 7 |
| `--source` is neither store | exit 2 |

The hash is memorycard's: sha256, in hex, of `title + "\n" + body` lowercased with every run of white space
collapsed to one space. A copy that was edited after it was mirrored has another hash and is not caught.

A memory imported from an Engram observation that has no page is not bridged to a page, and is promoted.

The bridge works the other way too: an Engram observation whose memorycard memory (the one imported from it, or one
holding its content) already has a page is refused by `promote --id N` as a bridged memory is: exit 6, the message
naming that page, and nothing written. Previously `promote --id N` wrote a second page for it. Memorycard's database is
read for this when there is one; when there is none, no observation has a lineage and `promote` is what it was.

The accepted tradeoff (feature document `odd/tasks/memorycard-engram-replacement.md`, L16, L18 and L19): the pages
that carry `engram_id` keep resolving against Engram's database and are never touched, so a bridged memory, and a
memory with the same content as one of those observations, cannot be promoted from memorycard. Its history stays on
the Engram page.
