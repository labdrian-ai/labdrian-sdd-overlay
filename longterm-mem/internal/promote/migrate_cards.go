package promote

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// This file is the one-time migration of S6 (c) of the store-agnostic promotion. Pages written before the
// store-agnostic keys carry one of two families of identity keys: engram_id, engram_revision, engram_type (and
// engram_sync_id) for a page promoted from an Engram observation, memorycard_id, memorycard_revision and
// memorycard_type for a page promoted from a memorycard memory. The migration gives every such page of a project the
// store-agnostic keys of its memorycard memory (source: memorycard, source_id, source_type, source_revision) and
// takes the legacy ones away, so that from then on the page is the memory's own page: `promote --source memorycard`
// answers `already promoted`, and `sync` never promotes the memory a second time.
//
// A page keeps its address, its created date, its body and every key that is not an identity key; only the lines of
// the identity keys change, and so does its precedence entry's frontmatter hash (the body hash and the recorded
// revision stay). A page naming an Engram observation takes the identity of the memory memorycard imported from it
// (its import_map row, through CardBridges): it reads no Engram database. Its source_revision is that memory's
// revision at the migration, so the migration changes no body and a memory edited in memorycard after its import
// keeps the Engram-era body until its next revision. A page that already names a memorycard memory keeps that
// memory, its revision and its type.
//
// A page is refused, and left byte-identical, when the migration cannot show that the rewrite is safe (RefusalKind):
// it differs from its precedence entry (a hand edit, R-030) or has none, memorycard holds no memory imported from
// its observation (or none with the id it names), its memory already has a page of its own, its identity lines
// are not ones the rewrite can replace whole, or it names no project (a page with no project is kept in every run, as
// it is no other project's page). The run goes on to the other pages. A page that matches its memory by
// content is never paired: the owner decides what is left (R5). A page that carries a legacy key and names no memory
// is a legacy page too: it is counted and refused by name, so the count of legacy pages is the count of pages that
// still carry a legacy key.

// errNoMigrationBackup is what a migration that must write answers when it was handed no way to keep a copy.
var errNoMigrationBackup = errors.New("promote: the migration has no backup")

// errNoCardBridges is what a migration answers when it was handed no way to find the memory of a page.
var errNoCardBridges = errors.New("promote: the migration has no memorycard bridge")

// errNoPrecedenceStore is what a migration that must write answers when the writer holds no precedence store.
var errNoPrecedenceStore = errors.New("promote: the writer has no precedence store")

// CardBridges is the port the migration finds a page's memory through: the memory memorycard imported from an
// Engram observation (its import_map row), and a memory by its own id, for a page that already names one. ok is
// false, with a nil error, when there is none.
type CardBridges interface {
	MemoryByEngramID(engramID int64) (memory.CardMemory, bool, error)
	MemoryByID(memoryID int64) (memory.CardMemory, bool, error)
}

// MigrationBackup keeps a copy of what a migration is about to rewrite: the pages named by address, and the
// precedence sidecar. The vault the migration runs on is under no version control, so the copy is the only way
// back. BackUp is asked once, before the first write, and a failure of it stops the run with nothing written.
type MigrationBackup interface {
	BackUp(addresses []string) (location string, err error)
}

// RefusalKind says why a page of the migration was left as it is.
type RefusalKind int

const (
	// RefusedEdited: the page differs from the precedence entry of longterm-mem's last write (a hand edit, R-030).
	RefusedEdited RefusalKind = iota
	// RefusedUntracked: the page has no precedence entry, so its provenance is unknown.
	RefusedUntracked
	// RefusedNoImport: no import_map row names the Engram observation the page names, or the memory it names is gone.
	RefusedNoImport
	// RefusedNoMemory: the page names a memorycard memory by its id and memorycard holds no memory with it.
	RefusedNoMemory
	// RefusedHasOwnPage: the memory the page maps to already has a page of its own, or is the memory of another page
	// of the run: two pages for one memory.
	RefusedHasOwnPage
	// RefusedOther: the page's identity cannot be read as one memory (an id that is no number, an Engram and a
	// memorycard identity at once, a legacy key and no memory, identity lines the rewrite cannot replace whole), or
	// the memory is in another project.
	RefusedOther
	// RefusedHalfMigrated: the page's body is the one its entry records, but its frontmatter is neither the recorded
	// page nor a rewrite a stopped run could have written: a frontmatter line was edited, or a stopped run was followed
	// by an edit. The way back is the backup of the run that stopped.
	RefusedHalfMigrated
	// RefusedUnreadable: the page carries a legacy key but its frontmatter cannot be read (CRLF line endings, or a block with
	// no closing line), so the migration cannot rewrite it; it is counted and named, never skipped.
	RefusedUnreadable
)

// RefusalKinds lists the kinds in the order a report counts them.
var RefusalKinds = []RefusalKind{RefusedEdited, RefusedHalfMigrated, RefusedUnreadable, RefusedUntracked, RefusedNoImport, RefusedNoMemory, RefusedHasOwnPage, RefusedOther}

// String is how a count of the kind is named.
func (k RefusalKind) String() string {
	switch k {
	case RefusedEdited:
		return "edited since longterm-mem wrote it"
	case RefusedHalfMigrated:
		return "frontmatter differs from the entry and is no recorded rewrite"
	case RefusedUnreadable:
		return "frontmatter cannot be read (" + unreadableCauses + ")"
	case RefusedUntracked:
		return "not recorded as written by longterm-mem"
	case RefusedNoImport:
		return "no import_map row names its Engram observation"
	case RefusedNoMemory:
		return "memorycard holds no memory with the id it names"
	case RefusedHasOwnPage:
		return "its memory already has a page"
	}
	return "identity cannot be read as one memory"
}

// MigratedPage is a page the migration turned into a page of the store-agnostic keys, or with apply false would.
// EngramID is the observation the page named, or 0 when it already named a memorycard memory.
type MigratedPage struct {
	Address  string
	EngramID int64
	MemoryID int64
}

// MigrationRefusal is a page of the migration that was left as it is, and why.
type MigrationRefusal struct {
	Address string
	Kind    RefusalKind
	Reason  string
}

// CardMigrationReport is what MigrateCardPages did (or would): the pages of the project that carry legacy keys, the
// ones migrated and the ones refused, and where the backup of the run is (empty when nothing was backed up).
type CardMigrationReport struct {
	// Legacy is how many pages of the project carried legacy keys when the run began.
	Legacy   int
	Migrated []MigratedPage
	Refused  []MigrationRefusal
	Backup   string
	// NoProject are the pages that carry a legacy key and name no project: no project owns them, so they are in none
	// of the per-project numbers above and are reported once, apart, in every run. The migration leaves them alone.
	NoProject []MigrationRefusal
}

// RefusalCounts counts the refusals of each kind.
func (r CardMigrationReport) RefusalCounts() map[RefusalKind]int {
	counts := map[RefusalKind]int{}
	for _, refusal := range r.Refused {
		counts[refusal.Kind]++
	}
	return counts
}

// plannedMigration is one page the run will rewrite: the page as it is, and what it becomes.
type plannedMigration struct {
	MigratedPage
	body     string
	newBlock string
	// was is the precedence entry the page had, and entry the one it has once migrated.
	was, entry PrecedenceEntry
}

// legacyIdentityKeys are the keys that say what a page of an older release was promoted from; the migration takes
// them away. The store-agnostic keys are listed in identityKeys: a page that carries both has its identity rewritten once.
var legacyIdentityKeys = []string{
	engramIDField, engramRevisionField, engramTypeField, engramSyncIDField,
	memorycardIDField, memorycardRevisionField, memorycardTypeField,
}

var identityKeys = append([]string{sourceField, sourceIDField, sourceRevisionField, sourceTypeField}, legacyIdentityKeys...)

// carriesLegacyKeys reports whether the frontmatter block of a page has a top-level line of any legacy identity key,
// whatever its value (an empty one included): the parsed fields skip an empty value, and a page that still has the
// line still has the key. An indented line is text of some other value (a block scalar that mentions `engram_id:`),
// never a key.
func carriesLegacyKeys(block string) bool {
	for _, line := range strings.Split(block, "\n") {
		key, _, isField := strings.Cut(line, ":")
		if !isField {
			continue
		}
		for _, legacy := range legacyIdentityKeys {
			if key == legacy {
				return true
			}
		}
	}
	return false
}

// topLevelFields is parseFrontmatterFields of the top-level lines of block only: that parser reads indented lines too, so a
// block scalar that mentions `engram_id:` would otherwise look like an identity the page does not have.
func topLevelFields(block string) map[string]string {
	var top []string
	for _, line := range strings.Split(block, "\n") {
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		top = append(top, line)
	}
	return parseFrontmatterFields(strings.Join(top, "\n"))
}

// unreadableCauses says, in the name of the refusal and in its reason, every way a page that opens with --- can have
// frontmatter the migration cannot read.
const unreadableCauses = "CRLF line endings, no closing ---, a BOM, blank line or spaces before the opening ---, spaces after an opening or closing ---, or a line ending in --- inside the frontmatter"

// pageShape is what a page is to the migration: no frontmatter, frontmatter it can read, or frontmatter it cannot.
type pageShape int

const (
	shapeNone pageShape = iota
	shapeReadable
	shapeUnreadable
)

// shapeOf classifies a page and answers the text its legacy keys are looked for in.
//
// Readable: the page starts at byte 0 with a line of --- ending in a line feed and has a closing such line; the text is
// the bytes from 0 to the end of the closing line, so the body that follows it is exactly raw after the block. The shared
// reader frontmatterBlock, which MatchesPage and every other reader use, must split the page at the very same place: it
// takes the first --- line feed anywhere, so a frontmatter line that ends in --- (a word, or a line of four dashes) would
// make it end the block earlier, and a rewrite would then look like an edit to MatchesPage. Where the two splits
// disagree the page is not readable.
//
// Unreadable: the first line that is not blank, after a byte order mark, blank lines, spaces, tabs and carriage returns
// are taken off, is ---, but the page is not readable (see unreadableCauses). The text is the lines between that line and
// the next line of ---, trailing spaces and tabs ignored on both, or the rest of the page when there is none: what the
// page quotes after its frontmatter is never scanned. Such a page is never rewritten.
//
// None: any other page. Every reader of longterm-mem sees a page that does not open with --- as one with no frontmatter, so
// it is no promoted page and cannot be promoted twice, and its text is never scanned.
func shapeOf(raw string) (string, pageShape) {
	if strings.HasPrefix(raw, "---\n") {
		if i := strings.Index(raw[3:], "\n---\n"); i >= 0 {
			if shared, ok := frontmatterBlock(raw); ok && shared == raw[:3+i+5] {
				return shared, shapeReadable
			}
		}
	}
	lines := strings.Split(strings.TrimPrefix(raw, "\ufeff"), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSuffix(line, "\r")
	}
	first := 0
	for first < len(lines) && strings.Trim(lines[first], " \t") == "" {
		first++
	}
	if first == len(lines) || strings.Trim(lines[first], " \t") != "---" {
		return "", shapeNone
	}
	var inside []string
	for _, line := range lines[first+1:] {
		if strings.TrimRight(line, " \t") == "---" {
			break
		}
		inside = append(inside, line)
	}
	return strings.Join(inside, "\n"), shapeUnreadable
}

// legacyPage is a page of the project that carries a legacy key.
type legacyPage struct {
	address, raw, block string
	fields              map[string]string
	// unreadable: the page carries a legacy key but its frontmatter cannot be read, so block is empty and fields come
	// from the lines the page opens with.
	unreadable bool
}

// legacyPagesOf lists the pages of project that carry a legacy key, in the order the port lists them, through the
// same read as every page. A page that cannot be read fails the run: the caller has already stopped on the ones
// the index could not read, so this one changed since.
func legacyPagesOf(pages PageReader, project string) ([]legacyPage, error) {
	var found []legacyPage
	var failure error
	err := eachPage(pages, func(address, content string, err error) {
		if failure != nil {
			return
		}
		if err != nil {
			failure = fmt.Errorf("read %s: %w", pages.PageLocation(address), err)
			return
		}
		block, shape := shapeOf(content)
		if shape == shapeNone || !carriesLegacyKeys(block) {
			return
		}
		unreadable := shape == shapeUnreadable
		fields := topLevelFields(block)
		// A page that names no project is no other project's page: it is kept, and refused by name, in every run.
		if owner := fields[projectField]; owner != "" && owner != project {
			return
		}
		if unreadable {
			block = ""
		}
		found = append(found, legacyPage{address: address, raw: content, block: block, fields: fields, unreadable: unreadable})
	})
	if err != nil {
		return nil, err
	}
	return found, failure
}

// MigrateCardPages gives the pages of project that carry legacy keys the store-agnostic identity of their
// memorycard memory. With apply false it decides everything and writes nothing.
//
// A vault with a page that cannot be read stops the run before any write, because that page may be a memory's own.
// A failed lookup or backup does too. With apply true the pages to rewrite are backed up first, once, and then for
// each its precedence entry is persisted and its page written, in that order: a run that stops between the two
// leaves a page whose entry already records what the page becomes, and the next run recognizes that page and
// finishes it, where the other order would leave a migrated page that reads as hand-edited. A failure leaves the
// pages before it migrated and recorded, and the error names the page that failed. The catalog, the log and the
// address map are not touched: the pages keep their addresses. A second run finds no page carrying legacy keys
// among the migrated ones and changes nothing.
func MigrateCardPages(cards CardBridges, w *Writer, backup MigrationBackup, project string, apply bool) (CardMigrationReport, error) {
	var report CardMigrationReport
	if memory.IsMissing(cards) {
		return report, errNoCardBridges
	}
	if err := w.checkPatchPorts(); err != nil {
		return report, fmt.Errorf("promote: migrate card pages: %w", err)
	}
	if apply && w.Store == nil {
		return report, fmt.Errorf("promote: migrate card pages: %w", errNoPrecedenceStore)
	}
	index, err := readPages(w.Pages)
	if err != nil {
		return report, fmt.Errorf("promote: migrate card pages: %w", err)
	}
	if len(index.unreadable) > 0 {
		return report, fmt.Errorf("promote: migrate card pages: %s; nothing was written", describePages(index.unreadable))
	}
	pages, err := legacyPagesOf(w.Pages, project)
	if err != nil {
		return report, fmt.Errorf("promote: migrate card pages: %w", err)
	}

	var plan []plannedMigration
	g := &migrator{cards: cards, index: index, project: project, claimed: map[int64]string{}}
	for _, page := range pages {
		if page.fields[projectField] == "" {
			report.NoProject = append(report.NoProject, MigrationRefusal{Address: page.address, Kind: RefusedOther, Reason: "it carries legacy keys and has no project field: add a `project:` line to the page by hand, then run again"})
			continue
		}
		report.Legacy++
		refuse := func(kind RefusalKind, reason string) {
			report.Refused = append(report.Refused, MigrationRefusal{Address: page.address, Kind: kind, Reason: reason})
		}
		if page.unreadable {
			refuse(RefusedUnreadable, "unreadable frontmatter ("+unreadableCauses+"): longterm-mem reads frontmatter that starts at the first byte with a line of --- and closes with another, both ending in a line feed, so the page was left alone; fix it, then run again")
			continue
		}
		if !simpleIdentityLines(page.block) {
			refuse(RefusedOther, "an identity key's value runs over several lines, which the migration does not rewrite")
			continue
		}

		moved, kind, reason, err := g.mapPage(page.address+".md", page.fields)
		if err != nil {
			return report, err
		}
		if reason != "" {
			refuse(kind, reason)
			continue
		}
		body := page.raw[len(page.block):]
		entry, tracked := w.Store.Get(page.address)
		newBlock := rewrittenIdentity(page.block, moved.MemoryID, moved.revision, moved.kind)
		switch {
		case !tracked:
			refuse(RefusedUntracked, "the page was not recorded as written by longterm-mem, so its provenance is unknown")
			continue
		case entry.MatchesPage(page.raw):
		default:
			// A run that stopped after persisting the entry and before writing the page left an entry that records
			// the page the run meant to write. Only when the body is the recorded one is that worth looking for: the
			// entry's frontmatter hash must then be the rewrite of this very block under an identity the memory had at
			// the stop. What the memory has become since does not matter: the page is finished as recorded. Candidates
			// are built only here, never for a page that matches its entry.
			if entry.BodyHash != hashText(body) {
				refuse(RefusedEdited, "the page was edited locally since longterm-mem last wrote it")
				continue
			}
			stopped, ok := moved.recordedRewrite(entry, page.block)
			if !ok {
				refuse(RefusedHalfMigrated, "the page's body is the one its entry records, but no rewrite of its frontmatter reproduces the entry: a frontmatter line was edited, or a run that stopped was followed by an edit; nothing was changed, and the backup of the run that stopped (.vault-meta/card-page-migration-<UTC instant>/) restores the page and the sidecar when copied over the vault")
				continue
			}
			newBlock = stopped
		}
		// The identity to write comes from the memory (its type) and must not end the frontmatter early for the shared
		// reader, as a type that ends in --- would: the page would then fail MatchesPage after the rewrite. The page that
		// would be written must be readable, with exactly newBlock as its block, or it is not written.
		if written, shape := shapeOf(newBlock + body); shape != shapeReadable || written != newBlock {
			refuse(RefusedOther, "the identity to write (the memory's type or revision) would make the shared page reader read the rewritten page differently (it would end the frontmatter early), so the page would look hand-edited; nothing was changed")
			continue
		}

		g.claimed[moved.MemoryID] = page.address
		moved.Address = page.address
		updated := entry
		updated.FrontmatterHash = hashText(newBlock)
		plan = append(plan, plannedMigration{MigratedPage: moved.MigratedPage, body: body, newBlock: newBlock, was: entry, entry: updated})
	}

	if !apply {
		for _, p := range plan {
			report.Migrated = append(report.Migrated, p.MigratedPage)
		}
		return report, nil
	}
	return applyMigration(w, backup, plan, report)
}

// mappedPage is the memory a page maps to, with the revision and type its new keys carry. A page naming an Engram
// observation takes both from the memory (fromMemory), which may have moved on since a run that stopped recorded
// them: memoryRevision is the memory's revision now and ownKind the type the page itself names.
type mappedPage struct {
	MigratedPage
	revision, kind string
	fromMemory     bool
	memoryRevision int
	ownKind        string
}

// recordedRewrite is the block a stopped run meant to write for block, when the entry records it: the rewrite of
// block under an identity the page could have been given has the entry's frontmatter hash. A page naming a memorycard
// memory has one candidate, its own revision and type. A page naming an Engram observation takes the memory's, and
// revisions only grow, so the candidates are the memory's current revision first and then every revision below it
// down to 0 and then none, each with the memory's current type, the type the page names and none. They are built here, one at a time,
// and only for a page whose body is the recorded one and whose frontmatter is not.
//
// An edit confined to the identity lines of such a page is adopted: the migration rewrites those lines anyway, and
// the backup of the run that stopped holds them as they were.
func (m mappedPage) recordedRewrite(entry PrecedenceEntry, block string) (string, bool) {
	try := func(revision, kind string) (string, bool) {
		candidate := rewrittenIdentity(block, m.MemoryID, revision, kind)
		return candidate, hashText(candidate) == entry.FrontmatterHash
	}
	if !m.fromMemory {
		return try(m.revision, m.kind)
	}
	// The empty type and the empty revision are candidates too: a memory with no type, and a rewrite that wrote no
	// revision line, are rewrites a run could have recorded.
	kinds := []string{m.kind}
	for _, kind := range []string{m.ownKind, ""} {
		if !slices.Contains(kinds, kind) {
			kinds = append(kinds, kind)
		}
	}
	for revision := m.memoryRevision; revision >= -1; revision-- {
		text := ""
		if revision >= 0 {
			text = strconv.Itoa(revision)
		}
		for _, kind := range kinds {
			if candidate, ok := try(text, kind); ok {
				return candidate, true
			}
		}
	}
	return "", false
}

// migrator decides, page by page of one run, which memory a page is the page of.
type migrator struct {
	cards   CardBridges
	index   *pageIndex
	project string
	// claimed maps a memory to the page of this run that takes it.
	claimed map[int64]string
}

// mapPage decides which memory of the project the page with fields (file is its name, never counted as another page
// of the memory) is the page of. It answers the page migrated (its Address is set by the caller), or a refusal kind
// and reason, or an error when memorycard could not be read. Whatever the page names, the memory must exist, be of
// the project, have no other page and not be the memory of another page of the run: the checks are the same for both
// families of keys, and only finding the memory differs.
func (g *migrator) mapPage(file string, fields map[string]string) (mappedPage, RefusalKind, string, error) {
	engram, hasEngram := identityOf(fields, engramSource)
	card, hasCard := identityOf(fields, memorycardSource)
	var m memory.CardMemory
	var moved mappedPage
	var kind RefusalKind
	var reason string
	var err error
	switch {
	case hasEngram && hasCard:
		return mappedPage{}, RefusedOther, fmt.Sprintf("it names Engram observation %s and memorycard memory %s at once", engram.ID, card.ID), nil
	case hasCard:
		m, moved, kind, reason, err = g.viaMemorycard(card)
	case hasEngram:
		m, moved, kind, reason, err = g.viaEngram(engram, fields)
	default:
		return mappedPage{}, RefusedOther, "it carries legacy keys but names no memory", nil
	}
	if err != nil || reason != "" {
		return mappedPage{}, kind, reason, err
	}
	if kind, reason := g.ownership(m, file); reason != "" {
		return mappedPage{}, kind, reason, nil
	}
	return moved, 0, "", nil
}

// viaMemorycard finds the memory of a page that names a memorycard memory by its id; the page keeps its revision and
// type, so what a stopped run recorded is the one rewrite.
func (g *migrator) viaMemorycard(card PageIdentity) (memory.CardMemory, mappedPage, RefusalKind, string, error) {
	memoryID, err := strconv.ParseInt(strings.TrimSpace(card.ID), 10, 64)
	if err != nil || memoryID <= 0 {
		// An id that is no number is a page to refuse by name, not a failure of the run: the error is the reason.
		return memory.CardMemory{}, mappedPage{}, RefusedOther, fmt.Sprintf("memorycard id %q is not a memory id", card.ID), nil
	}
	m, found, err := g.cards.MemoryByID(memoryID)
	if err != nil {
		return memory.CardMemory{}, mappedPage{}, 0, "", fmt.Errorf("promote: migrate card pages: look up memory %d: %w", memoryID, err)
	}
	if !found {
		return memory.CardMemory{}, mappedPage{}, RefusedNoMemory, fmt.Sprintf("memorycard holds no memory %d", memoryID), nil
	}
	moved := mappedPage{MigratedPage: MigratedPage{MemoryID: memoryID}, revision: card.Revision, kind: card.Type}
	return m, moved, 0, "", nil
}

// viaEngram finds the memory memorycard imported from the observation a page names; the page takes that memory's
// revision and type, which a stopped run may have recorded as they were then.
func (g *migrator) viaEngram(engram PageIdentity, fields map[string]string) (memory.CardMemory, mappedPage, RefusalKind, string, error) {
	engramID, err := strconv.ParseInt(strings.TrimSpace(engram.ID), 10, 64)
	if err != nil || engramID <= 0 {
		return memory.CardMemory{}, mappedPage{}, RefusedOther, fmt.Sprintf("engram_id %q is not an observation id", engram.ID), nil
	}
	m, found, err := g.cards.MemoryByEngramID(engramID)
	if err != nil {
		return memory.CardMemory{}, mappedPage{}, 0, "", fmt.Errorf("promote: migrate card pages: look up the memory imported from Engram observation %d: %w", engramID, err)
	}
	if !found {
		return memory.CardMemory{}, mappedPage{}, RefusedNoImport, fmt.Sprintf("memorycard holds no memory imported from Engram observation %d (import_map)", engramID), nil
	}
	moved := mappedPage{MigratedPage: MigratedPage{EngramID: engramID, MemoryID: m.ID}, revision: strconv.Itoa(m.Revision), kind: m.Type,
		fromMemory: true, memoryRevision: m.Revision, ownKind: strings.TrimSpace(fields[engramTypeField])}
	return m, moved, 0, "", nil
}

// ownership is what a memory must be, whichever page names it: of the project, without another page, and not the
// memory of another page of the run. It answers a refusal, or none.
func (g *migrator) ownership(m memory.CardMemory, file string) (RefusalKind, string) {
	if m.Project != g.project {
		return RefusedOther, fmt.Sprintf("memory %d belongs to project %q, not %q", m.ID, m.Project, g.project)
	}
	for _, other := range g.index.carriers[memory.Identity{Source: memorycardSource, ID: strconv.FormatInt(m.ID, 10)}] {
		if other.Project == g.project && other.File != file {
			return RefusedHasOwnPage, fmt.Sprintf("memory %d already has the page %s", m.ID, other.address())
		}
	}
	if other, taken := g.claimed[m.ID]; taken {
		return RefusedHasOwnPage, fmt.Sprintf("memory %d is already the memory of %s in this run", m.ID, other)
	}
	return 0, ""
}

// simpleIdentityLines reports whether every top-level identity key of block is a one-line `key: value`, which is what
// rewrittenIdentity replaces. A value that runs onto indented lines is not: the rewrite would leave part of the old
// identity behind, so the page is refused by name instead. An indented line that looks like a key is no key.
func simpleIdentityLines(block string) bool {
	lines := strings.Split(block, "\n")
	for i, line := range lines {
		key, _, isField := strings.Cut(line, ":")
		if !isField || !isIdentityKey(key) {
			continue
		}
		if i+1 < len(lines) && strings.TrimSpace(lines[i+1]) != "" && lines[i+1] != strings.TrimLeft(lines[i+1], " \t") {
			return false
		}
	}
	return true
}

// rewrittenIdentity is block with its identity keys replaced by the four store-agnostic ones, written where the
// first identity key was, and every other line untouched. An empty revision or type writes no line for it.
func rewrittenIdentity(block string, memoryID int64, revision, kind string) string {
	var identity strings.Builder
	writeField(&identity, sourceField, memorycardSource)
	writeField(&identity, sourceIDField, strconv.FormatInt(memoryID, 10))
	if kind != "" {
		writeField(&identity, sourceTypeField, kind)
	}
	if revision != "" {
		writeField(&identity, sourceRevisionField, revision)
	}
	written := false
	var lines []string
	for _, line := range strings.Split(block, "\n") {
		key, _, isField := strings.Cut(line, ":")
		if !isField || !isIdentityKey(key) {
			lines = append(lines, line)
			continue
		}
		if !written {
			lines = append(lines, strings.Split(strings.TrimRight(identity.String(), "\n"), "\n")...)
			written = true
		}
	}
	return strings.Join(lines, "\n")
}

func isIdentityKey(key string) bool {
	for _, k := range identityKeys {
		if key == k {
			return true
		}
	}
	return false
}

// applyMigration backs the planned pages up, then for each persists its precedence entry and writes its page.
func applyMigration(w *Writer, backup MigrationBackup, plan []plannedMigration, report CardMigrationReport) (CardMigrationReport, error) {
	if len(plan) == 0 {
		return report, nil
	}
	if memory.IsMissing(backup) {
		return report, errNoMigrationBackup
	}
	addresses := make([]string, len(plan))
	for i, p := range plan {
		addresses[i] = p.Address
	}
	location, err := backup.BackUp(addresses)
	if err != nil {
		return report, fmt.Errorf("promote: migrate card pages: back up %d page(s) first: %w; nothing was written", len(plan), err)
	}
	report.Backup = location
	for _, p := range plan {
		w.Store.Set(p.Address, p.entry)
		if err := w.Precedence.SavePrecedence(w.Store); err != nil {
			w.Store.Set(p.Address, p.was)
			return report, fmt.Errorf("promote: migrate card pages: persist the precedence entry of %s: %w; the page was not written, and the sidecar on disk may or may not hold its new entry (the pages migrated before it stay migrated and recorded), so a rerun, which finishes or starts it either way, is the way on; the copy of the pages as they were is in %s", p.Address, err, location)
		}
		if err := w.Pages.WritePage(p.Address, p.newBlock+p.body); err != nil {
			return report, fmt.Errorf("promote: migrate card pages: write %s: %w; its precedence entry is already recorded, so a rerun finishes it (the pages migrated before it stay migrated; the copy of the pages as they were is in %s)", p.Address, err, location)
		}
		report.Migrated = append(report.Migrated, p.MigratedPage)
	}
	return report, nil
}
