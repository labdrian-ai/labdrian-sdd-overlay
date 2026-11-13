package promote

import (
	"errors"
	"fmt"
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
// its observation (or none with the id it names), its memory already has a page of its own, or its identity lines
// are not ones the rewrite can replace whole. The run goes on to the other pages. A page that matches its memory by
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
)

// RefusalKinds lists the kinds in the order a report counts them.
var RefusalKinds = []RefusalKind{RefusedEdited, RefusedUntracked, RefusedNoImport, RefusedNoMemory, RefusedHasOwnPage, RefusedOther}

// String is how a count of the kind is named.
func (k RefusalKind) String() string {
	switch k {
	case RefusedEdited:
		return "edited since longterm-mem wrote it"
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

// carriesLegacyKeys reports whether the frontmatter block of a page has a line of any legacy identity key, whatever
// its value (an empty one included) and however it is indented: the parsed fields skip an empty value, and a page
// that still has the line still has the key.
func carriesLegacyKeys(block string) bool {
	for _, line := range strings.Split(block, "\n") {
		key, _, isField := strings.Cut(strings.TrimLeft(line, " \t"), ":")
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

// legacyPage is a page of the project that carries a legacy key.
type legacyPage struct {
	address, raw, block string
	fields              map[string]string
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
		block, ok := frontmatterBlock(content)
		if !ok || !carriesLegacyKeys(block) {
			return
		}
		fields := parseFrontmatterFields(block)
		if fields[projectField] != project {
			return
		}
		found = append(found, legacyPage{address: address, raw: content, block: block, fields: fields})
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
	claimed := map[int64]string{}
	for _, page := range pages {
		report.Legacy++
		refuse := func(kind RefusalKind, reason string) {
			report.Refused = append(report.Refused, MigrationRefusal{Address: page.address, Kind: kind, Reason: reason})
		}
		if !simpleIdentityLines(page.block) {
			refuse(RefusedOther, "an identity key is indented or its value runs over several lines, which the migration does not rewrite")
			continue
		}

		moved, kind, reason, err := mapPage(cards, index, claimed, project, page.address+".md", page.fields)
		if err != nil {
			return report, err
		}
		if reason != "" {
			refuse(kind, reason)
			continue
		}
		newBlock := rewrittenIdentity(page.block, moved.MemoryID, moved.revision, moved.kind)
		body := page.raw[len(page.block):]
		entry, tracked := w.Store.Get(page.address)
		switch {
		case !tracked:
			refuse(RefusedUntracked, "the page was not recorded as written by longterm-mem, so its provenance is unknown")
			continue
		case entry.MatchesPage(page.raw):
		case entry.FrontmatterHash == hashText(newBlock) && entry.BodyHash == hashText(body):
			// A run that stopped after persisting this entry and before writing the page: the entry records exactly
			// the page this run would write, and the page's body is the recorded one, so nothing a person wrote is lost.
		default:
			refuse(RefusedEdited, "the page was edited locally since longterm-mem last wrote it")
			continue
		}

		claimed[moved.MemoryID] = page.address
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

// mappedPage is the memory a page maps to, with the revision and type its new keys carry.
type mappedPage struct {
	MigratedPage
	revision, kind string
}

// mapPage decides which memory of project the page with fields (the file is its name, never counted as another page
// of the memory) is the page of. It answers the page migrated (its Address is set by the caller), or a refusal kind
// and reason, or an error when memorycard could not be read. Whatever the page names, the memory must exist, be of
// project, have no other page and not be the memory of another page of the run.
func mapPage(cards CardBridges, index *pageIndex, claimed map[int64]string, project, file string, fields map[string]string) (mappedPage, RefusalKind, string, error) {
	engram, hasEngram := identityOf(fields, engramSource)
	card, hasCard := identityOf(fields, memorycardSource)
	switch {
	case hasEngram && hasCard:
		return mappedPage{}, RefusedOther, fmt.Sprintf("it names Engram observation %s and memorycard memory %s at once", engram.ID, card.ID), nil
	case !hasEngram && !hasCard:
		return mappedPage{}, RefusedOther, "it carries legacy keys but names no memory", nil
	}

	var m memory.CardMemory
	var moved mappedPage
	if hasCard {
		memoryID, err := strconv.ParseInt(strings.TrimSpace(card.ID), 10, 64)
		if err != nil || memoryID <= 0 {
			// An id that is no number is a page to refuse by name, not a failure of the run: the error is the reason.
			return mappedPage{}, RefusedOther, fmt.Sprintf("memorycard id %q is not a memory id", card.ID), nil
		}
		found := false
		m, found, err = cards.MemoryByID(memoryID)
		if err != nil {
			return mappedPage{}, 0, "", fmt.Errorf("promote: migrate card pages: look up memory %d: %w", memoryID, err)
		}
		if !found {
			return mappedPage{}, RefusedNoMemory, fmt.Sprintf("memorycard holds no memory %d", memoryID), nil
		}
		moved = mappedPage{MigratedPage: MigratedPage{MemoryID: memoryID}, revision: card.Revision, kind: card.Type}
	} else {
		engramID, err := strconv.ParseInt(strings.TrimSpace(engram.ID), 10, 64)
		if err != nil || engramID <= 0 {
			return mappedPage{}, RefusedOther, fmt.Sprintf("engram_id %q is not an observation id", engram.ID), nil
		}
		found := false
		m, found, err = cards.MemoryByEngramID(engramID)
		if err != nil {
			return mappedPage{}, 0, "", fmt.Errorf("promote: migrate card pages: look up the memory imported from Engram observation %d: %w", engramID, err)
		}
		if !found {
			return mappedPage{}, RefusedNoImport, fmt.Sprintf("memorycard holds no memory imported from Engram observation %d (import_map)", engramID), nil
		}
		moved = mappedPage{MigratedPage: MigratedPage{EngramID: engramID, MemoryID: m.ID}, revision: strconv.Itoa(m.Revision), kind: m.Type}
	}

	if m.Project != project {
		return mappedPage{}, RefusedOther, fmt.Sprintf("memory %d belongs to project %q, not %q", m.ID, m.Project, project), nil
	}
	for _, other := range index.carriers[memory.Identity{Source: memorycardSource, ID: strconv.FormatInt(m.ID, 10)}] {
		if other.Project == project && other.File != file {
			return mappedPage{}, RefusedHasOwnPage, fmt.Sprintf("memory %d already has the page %s", m.ID, other.address()), nil
		}
	}
	if other, taken := claimed[m.ID]; taken {
		return mappedPage{}, RefusedHasOwnPage, fmt.Sprintf("memory %d is already the memory of %s in this run", m.ID, other), nil
	}
	return moved, 0, "", nil
}

// withStoreAgnosticIdentity is block with the identity keys of a page that names one memorycard memory by legacy
// memorycard_* keys replaced by the store-agnostic ones (rewrittenIdentity), keeping the memory, revision and type the
// page names. Every other block is returned as it is: a page of the store-agnostic keys, a page naming an Engram
// observation (only the migration can say which memory that is), and a page whose identity lines the line rewriter
// cannot rewrite whole (simpleIdentityLines).
func withStoreAgnosticIdentity(block string) string {
	if !carriesLegacyKeys(block) || !simpleIdentityLines(block) {
		return block
	}
	fields := parseFrontmatterFields(block)
	if _, engram := identityOf(fields, engramSource); engram {
		return block
	}
	card, named := identityOf(fields, memorycardSource)
	if !named {
		return block
	}
	memoryID, err := strconv.ParseInt(strings.TrimSpace(card.ID), 10, 64)
	if err != nil || memoryID <= 0 {
		return block
	}
	return rewrittenIdentity(block, memoryID, card.Revision, card.Type)
}

// simpleIdentityLines reports whether every identity key of block is a one-line `key: value` at the start of its line,
// which is what rewrittenIdentity replaces. An indented key, or a value that runs onto indented lines, is not: the
// rewrite would leave part of the old identity behind, so the page is refused by name instead.
func simpleIdentityLines(block string) bool {
	lines := strings.Split(block, "\n")
	for i, line := range lines {
		key, _, isField := strings.Cut(strings.TrimLeft(line, " \t"), ":")
		if !isField || !isIdentityKey(key) {
			continue
		}
		if line != strings.TrimLeft(line, " \t") {
			return false
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
			return report, fmt.Errorf("promote: migrate card pages: persist the precedence entry of %s: %w; the page was not written, and a rerun migrates it (the pages migrated before it stay migrated; the copy of the pages as they were is in %s)", p.Address, err, location)
		}
		if err := w.Pages.WritePage(p.Address, p.newBlock+p.body); err != nil {
			return report, fmt.Errorf("promote: migrate card pages: write %s: %w; its precedence entry is already recorded, so a rerun finishes it (the pages migrated before it stay migrated; the copy of the pages as they were is in %s)", p.Address, err, location)
		}
		report.Migrated = append(report.Migrated, p.MigratedPage)
	}
	return report, nil
}
