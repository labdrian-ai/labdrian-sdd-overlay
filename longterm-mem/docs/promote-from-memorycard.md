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
that is unset, `~/.memorycard/memory.db`. Nothing else is opened: the bridge between the parts of one imported observation (the bridged cases of "What it
refuses or leaves alone") is read from memorycard's own tables: its `import_map`, and, for a memory with no `import_map` row, the content hash it stores for each memory. A database that cannot be opened is exit 4. (Previously: Engram's database
was opened read-only too, to find the memories that duplicate one of its pages.)

## What it writes

The page carries these store-agnostic keys, in this order:

| Key | Value |
|---|---|
| `source` | `memorycard`, the store the memory comes from |
| `source_id` | the memory's id |
| `source_type` | the memory's type |
| `source_revision` | the memory's next version number: the highest `memory_versions.version`, plus one |
| `project` | the memory's project |

A page the writer creates or fully re-renders (a new page, an update in place) carries these keys and no
other identity key. A status patch changes `status` and `related` only. (Previously: `memorycard_id`, `memorycard_type`
and `memorycard_revision`, one family of keys per store, were read as the identity of a page too.)

A vault page that still carries only the retired keys (`engram_id`, `engram_revision`, `engram_type`, `memorycard_id`,
`memorycard_revision`, `memorycard_type`) is no memory's page: no reader takes those keys for an identity. `sync` and
`promote` therefore give the memory that page was written for a new page and leave the old page untouched, so the
vault then holds two pages for that memory. The owner's vault had none after the migration of 2026-11-15 (575 of 575
pages migrated, then 0 left); a vault that still has one needs the old page removed, or given the keys above by hand.

`related` holds the page of the memory in `superseded_by`, when that memory has a page in the same project; no
other relation of memorycard is read. `status` is `superseded` or `archived` when the memory is, `mature` when it
is pinned, `developing` otherwise.

A page is found again by `(project, source_id)` beside `source: memorycard`. A page of the memory is updated in place when the
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
| The memory is bridged: it is one part of an imported observation (`import_map` rows of source `engram` with the same `source_key`) and another part has a page | refused, exit 6 (`registration_conflict`); the message names that page, and the page stays byte-identical. No page is created |
| The memory has no `import_map` row and its `content_hash` is the hash memorycard stores for an imported part, of the same project, that already has a page | refused exactly like a bridged memory, the message saying the content is the same |
| A page of the vault could not be read and none that could be read decides the memory | nothing is written, exit 6 |
| No such memory | exit 7 |
| `--source` is missing, or is not a store | exit 2 |

The hash is memorycard's: sha256, in hex, of `title + "\n" + body` lowercased with every run of white space
collapsed to one space. A copy that was edited after it was mirrored has another hash and is not caught.

A memory imported from an observation none of whose other parts has a page is not bridged to a page, and is promoted. A
page that carries only the retired keys (`engram_id`, `memorycard_id` and their revision and type) names no memory, so
it bridges none: its memory is promoted as if the page were not there.
