# Promoting from memorycard

`longterm-mem promote` reads one store, memorycard, and names it: `--source memorycard` is required. Without a source
the command is refused, exit 2: `longterm-mem: promote: --source is required: use --source memorycard`. Any other
name, `engram` included, is refused the same way: `longterm-mem: promote: --source "engram" is not a store: use
memorycard`. (Previously: `--source` was `engram`, the default, or `memorycard`, and `promote --id N` promoted an
Engram observation.) The stores a command can read are registered in one place, `cmd/longterm-mem/sources.go`, so a
new store is one adapter and one entry there.

The MCP `promote` tool follows the command: its `source` is required and is `memorycard`, and its id is `memory_id`
or `engram_id`, its former name. One of them given is the id, both given with the same value is that id, both
given with different values is an error naming both, and neither is a missing id. (Previously: `source` was optional
and `engram` by default, and the id was `engram_id` alone.)

```
longterm-mem promote [--project P] [--vault DIR] --source memorycard --id N
```

`N` is a memorycard memory id. The memorycard database is opened read-only, at `LONGTERM_MEM_MEMORYCARD_DB` or, when
that is unset, `~/.memorycard/memory.db`. Nothing else is opened: the bridge to the pages of earlier releases (below)
is read from memorycard's own tables: its `import_map`, and, for a memory with no `import_map` row, the content hash it stores for each memory. A database that cannot be opened is exit 4. (Previously: Engram's database
was opened read-only too, to find the memories that duplicate one of its pages.)

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

A page is found again by `(project, memorycard_id)`. A page that carries `engram_id` (written by a release that
promoted from Engram) is never written by a promote. A page that carries `memorycard_id` is updated in place when the
memory's revision is newer than the page's; an equal revision writes nothing, and a page edited by hand since it was
written is left alone (exit 6).

## What it refuses or leaves alone

| Case | Result |
|---|---|
| The memory has no page in its project | a page is created (`promoted c-NNNNNN (created)`) |
| The memory has a page in its project at the same revision | nothing is written; exit 0, `memorycard memory N already promoted as c-NNNNNN; the page is left as it is` |
| The memory has a page in its project and a newer revision | the page is updated in place (`promoted c-NNNNNN (updated)`) |
| The memory has a page of its own and its lineage observation also has one | the page of its own decides first: it is updated or left as it is, and the memory is never refused as bridged |
| The memory's project changed since it was promoted | a page is created in the new project and the page left in the old one is marked superseded by it |
| The memory is bridged: its `import_map` row (source `engram`, `source_key` = an Engram observation id) names an observation that already has a page | refused, exit 6 (`registration_conflict`); the message names that page, and the page stays byte-identical. No page is created |
| The memory has no `import_map` row and its `content_hash` is the hash memorycard would compute for an Engram observation of the same project that already has a page | refused exactly like a bridged memory, the message saying the content is the same |
| A page of the vault could not be read and none that could be read decides the memory | nothing is written, exit 6 |
| No such memory | exit 7 |
| `--source` is missing, or is not a store | exit 2 |

The hash is memorycard's: sha256, in hex, of `title + "\n" + body` lowercased with every run of white space
collapsed to one space. A copy that was edited after it was mirrored has another hash and is not caught.

A memory imported from an Engram observation that has no page is not bridged to a page, and is promoted.

The accepted tradeoff (feature document `odd/tasks/memorycard-engram-replacement.md`, L16, L18 and L19): the pages
that carry `engram_id` are never touched, so a bridged memory, and a memory with the same content as one of those
observations, cannot be promoted from memorycard. Its history stays on the Engram page.
