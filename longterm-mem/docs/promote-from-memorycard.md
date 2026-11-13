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

The page has the frontmatter of an Engram page, with these store-agnostic keys, in this order, in place of the
`engram_*` ones:

| Key | Value |
|---|---|
| `source` | `memorycard`, the store the memory comes from |
| `source_id` | the memory's id |
| `source_type` | the memory's type |
| `source_revision` | the memory's next version number: the highest `memory_versions.version`, plus one |
| `project` | the memory's project |

A page the writer creates or fully re-renders (a new page, an update in place) carries these keys and no `engram_*` or
`memorycard_*` key. A status patch changes `status` and `related` only: it leaves the identity lines of a legacy page as they
are, because only the migration below rewrites the identity of an existing page. (Previously: `memorycard_id`, `memorycard_type` and `memorycard_revision`, one family of keys per
store.) Pages written by earlier releases keep their legacy keys, which are read as the identity they name, until the
one-time migration below rewrites them.

`related` holds the page of the memory in `superseded_by`, when that memory has a page in the same project; no
other relation of memorycard is read. `status` is `superseded` or `archived` when the memory is, `mature` when it
is pinned, `developing` otherwise.

A page is found again by `(project, source_id)` beside `source: memorycard`, or by the legacy `memorycard_id` of a page
written before. A page that carries `engram_id` (written by a release that promoted from Engram) is never written by
a promote until the migration gives it its memory's identity. A page of the memory is updated in place when the
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
observations, cannot be promoted from memorycard. Its history stays on the Engram page. The migration below ends the
tradeoff for every page it can map: the page becomes the memory's own.

## The one-time migration of the pages of older releases

Pages written before the store-agnostic keys carry `engram_id`, `engram_revision`, `engram_type` (and
`engram_sync_id`), or `memorycard_id`, `memorycard_revision` and `memorycard_type`. `migrate-card-pages` gives every
such page of a project the keys above, once:

```
longterm-mem migrate-card-pages [--project P] [--vault DIR] [--apply]
```

- A page that names Engram observation N takes the identity of the memory memorycard imported from N, found in
  memorycard's `import_map` (the memory of its first part): `source: memorycard`, that memory's id and type, and its
  revision at the migration as `source_revision` (so no body changes; a memory edited in memorycard after its import
  keeps the Engram-era body until its next revision). No Engram database is opened.
- A page that already names a memorycard memory keeps that memory, its revision and its type.
- The page keeps its address, `created`, `updated`, body and every key that is not an identity key; the identity keys
  (and `engram_sync_id`, which nothing reads) are replaced by the four `source*` lines where the first of them was. The
  precedence entry of the page changes only in its frontmatter hash. The catalog, the log and the address map are not
  touched: every address, so every wikilink, holds.
- Without `--apply` it prints what it would migrate and writes nothing. With `--apply` it first copies every page it
  will rewrite, and the precedence sidecar `.raw/.longterm-mem-manifest.json`, to
  `.vault-meta/card-page-migration-<UTC instant>/` (the same paths under it, so putting them back is a copy of that
  directory over the vault; the pages and the sidecar of a vault are not under version control, so this copy is the only
  way back), says where, and then rewrites the pages. If the copy cannot be made, nothing is written.
- Both runs print how many pages of the project carry legacy keys and the count of each refusal. The pages it
  would move or moved are named on stdout, each refusal on stderr with its reason.

A page is refused, left byte-identical and named, and the run goes on to the others; any refusal makes the exit code 6:

| Case | Counted as |
|---|---|
| the page's body differs from its precedence entry (a hand edit, R-030) | `edited since longterm-mem wrote it` |
| the page's body is the recorded one but its frontmatter matches neither the entry nor any rewrite a stopped run could have written (a frontmatter line was edited) | `frontmatter differs from the entry and is no recorded rewrite` (shown when it is not zero); the message names the backup directory and says copying it over the vault restores the page and the sidecar, and lists the backup directories the vault has (`backups present: ...`, or `none`) |
| the page carries a legacy key but its frontmatter cannot be read: CRLF line endings, or a block with no closing `---` | `frontmatter cannot be read (CRLF line endings or no closing ---)` (shown when it is not zero); the reason is `unreadable frontmatter (CRLF line endings or no closing ---)` |
| the page has no precedence entry | `not recorded as written by longterm-mem` |
| no `import_map` row names the page's observation, or the memory is gone | `no import_map row names its Engram observation` |
| the page names a memorycard memory by `memorycard_id` and memorycard holds no memory with that id | `memorycard holds no memory with the id it names` (shown when it is not zero) |
| the memory already has a page of its own, or is the memory of another page of the run (two pages for one memory; two `memorycard_id` pages of one memory are both refused) | `its memory already has a page` |
| the page's id is no number, it names an Engram observation and a memorycard memory at once, it carries a legacy key and names no memory, an identity key's value runs over several lines, or the memory is in another project | `identity cannot be read as one memory` (shown when it is not zero) |

A page that carries a legacy key and has no `project` field belongs to no project, so it is in no project's run: every run prints it
once, as a note on stdout only (`note: N page(s) carry legacy keys and name no project, so no project's run counts them: <pages>; add a
`project:` line to each by hand, then run again`), leaves it alone, and it makes no exit code of its own: exit 6 comes only from that
project's own refusals. The rehearsal script's vault-wide count includes these pages, and that count is what the retirement waits on
(it must be zero), so no page is hidden.

A refused page is left for the owner: it is never matched to a memory by its content. A page that cannot be read stops
the run before any write, since it may be a memory's own page. A second run changes nothing: the pages it migrated no
longer carry legacy keys, so the count it prints is the number of pages left. `promote reconcile` takes no part.

After the migration `sync` predicts the same promotions as before it: a memory whose page was migrated is now the
owner of that page (counted as skipped) and not a bridged memory, and `promote --source memorycard --id N` of it
answers `already promoted as c-NNNNNN`. `doctor` joins the pages by the same identity and reports the same findings.
A page that names an Engram observation takes the memory's current revision as its `source_revision` while its body stays
the one written in the Engram era: the page says "revision N" for the memory and holds the text of its import until the
memory's next revision rewrites it. Every page of the project that carries a legacy key as a top-level line is counted, and each one that is not migrated is
refused under one of the names of the table above (a page that names no memory is among them), so the
count of a project plus the pages with no project is the number of pages that still carry a legacy key. A line that only looks like a key inside a longer value is
no key. The command reads the pages that are `.md` files directly in `wiki/memory`; a page in a subdirectory is not seen, as for
every other command. It reads frontmatter delimited by lines ending in a line feed and closed by a `---` line: a page that
carries a top-level legacy key in a frontmatter it cannot read (CRLF line endings, or no closing `---`) is counted as a legacy
page and refused by name as above, never skipped, and the rehearsal script counts it the same way. Fix its line endings or close
the block, then run again.

The command works on one project. Run the dry run and then `--apply` for every project that has pages in the vault (the
rehearsal script reads the list from the vault's pages and prints a vault-wide count of the pages that still carry a legacy
key at the end; that count is the sum of the per-project counts and the pages with no project, and is zero when the vault is ready
for the retirement of the legacy keys).

If a run stops partway (a full disk, a killed process), the pages already migrated stay migrated and recorded. Each
page's precedence entry is saved before the page is written, so a page that was being written reads as the page the
rerun would write, and running the command again finishes it. The error names the backup directory
`.vault-meta/card-page-migration-<UTC instant>[-N]/` (a retry in the same second takes a numbered name; a copy that fails
halfway removes its directory, so one that is there is whole, and every file is synced and keeps its mode). A page a
stopped run recorded is finished by the rerun even if its memory has been revised or retyped since: for a page that names an
Engram observation the rerun searches the revisions from the memory's current one down to 0 (the memory's type and the page's own)
for the rewrite the entry records (also with no type and with no revision), only for a page whose body is the recorded one, and finishes the page with that identity. An edit
confined to the identity lines of such a page is adopted, since those lines are rewritten anyway and the backup holds them as they
were; an edit to any other frontmatter line is refused (above). The migration creates and
renames no page, so to go back to the state before it, copy the files of that directory over the vault
(`cp -a <backup>/. <vault>/`): every page it rewrote and the sidecar return to their prior bytes.

There is no lock between the migration and a `sync`, and the hook-driven `sync` (`engine/synctrigger`, fired when an agent
session ends or a change is archived) has no switch to turn it off. Before `--apply`, close every agent session that uses
the vault and check that no sync is running: `pgrep -af '[l]ongterm-mem.* sync'` prints nothing (the bracket keeps the check from matching itself; any
output is a sync that is running). Then no sync can write a page the migration is rewriting.

Run it first on copies of the vault and of memorycard's database (`scripts/rehearse-card-page-migration.sh` does, and
changes nothing live), read the counts, and only then run `--apply` on the vault, project by project.
