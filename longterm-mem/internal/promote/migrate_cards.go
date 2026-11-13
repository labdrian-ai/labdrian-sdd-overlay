package promote

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// This file is the one-time migration of S6 (c) of the store-agnostic promotion. Pages written before the
// store-agnostic keys carry one of two families of identity keys: engram_id, engram_revision, engram_type (and
// engram_sync_id) for a page promoted from an Engram observation, memorycard_id, memorycard_revision and
// memorycard_type for a page promoted from a memorycard memory. The migration gives every such page of a project the
// store-agnostic keys of its memorycard memory (source: memorycard, source_id, source_revision, source_type) and
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
// its observation, or its memory already has a page of its own. The run goes on to the other pages. A page that
// matches its memory by content is never paired: the owner decides what is left (R5).

// errNoMigrationBackup is what a migration that must write answers when it was handed no way to keep a copy.
var errNoMigrationBackup = errors.New("promote: the migration has no backup")

// CardBridges is the port the migration finds a page's memory through: the memory memorycard imported from an
// Engram observation (its import_map row). ok is false, with a nil error, when there is none.
type CardBridges interface {
	MemoryByEngramID(engramID int64) (memory.CardMemory, bool, error)
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
	// RefusedHasOwnPage: the memory the page maps to already has a page of its own, or is the memory of another page
	// of the run: two pages for one memory.
	RefusedHasOwnPage
	// RefusedOther: the page's identity cannot be read as one memory (an id that is no number, an Engram and a
	// memorycard identity at once), or the memory is in another project.
	RefusedOther
)

// RefusalKinds lists the kinds in the order a report counts them.
var RefusalKinds = []RefusalKind{RefusedEdited, RefusedUntracked, RefusedNoImport, RefusedHasOwnPage, RefusedOther}

// String is how a count of the kind is named.
func (k RefusalKind) String() string {
	switch k {
	case RefusedEdited:
		return "edited since longterm-mem wrote it"
	case RefusedUntracked:
		return "not recorded as written by longterm-mem"
	case RefusedNoImport:
		return "no import_map row names its Engram observation"
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
	entry    PrecedenceEntry
}

// legacyIdentityKeys are the keys that say what a page of an older release was promoted from; the migration takes
// them away. The store-agnostic keys are listed too: a page that carries both has its identity rewritten once.
var legacyIdentityKeys = []string{
	engramIDField, engramRevisionField, engramTypeField, engramSyncIDField,
	memorycardIDField, memorycardRevisionField, memorycardTypeField,
}

var identityKeys = append([]string{sourceField, sourceIDField, sourceRevisionField, sourceTypeField}, legacyIdentityKeys...)

// carriesLegacyKeys reports whether a page's fields hold any legacy identity key.
func carriesLegacyKeys(fields map[string]string) bool {
	for _, key := range legacyIdentityKeys {
		if _, ok := fields[key]; ok {
			return true
		}
	}
	return false
}

// MigrateCardPages gives the pages of project that carry legacy keys the store-agnostic identity of their
// memorycard memory. With apply false it decides everything and writes nothing.
//
// A vault with a page that cannot be read stops the run before any write, because that page may be a memory's own.
// A failed lookup or backup does too. With apply true the pages to rewrite are backed up first, once, and then each
// is written and its precedence entry persisted in turn: a failure leaves the pages before it migrated and
// recorded, and the error names the page that failed. The catalog, the log and the address map are not touched: the
// pages keep their addresses. A second run finds no page carrying legacy keys among the migrated ones and changes
// nothing.
func MigrateCardPages(cards CardBridges, w *Writer, backup MigrationBackup, project string, apply bool) (CardMigrationReport, error) {
	var report CardMigrationReport
	if err := w.checkPatchPorts(); err != nil {
		return report, fmt.Errorf("promote: migrate card pages: %w", err)
	}
	index, err := readPages(w.Pages)
	if err != nil {
		return report, fmt.Errorf("promote: migrate card pages: %w", err)
	}
	if len(index.unreadable) > 0 {
		return report, fmt.Errorf("promote: migrate card pages: %s; nothing was written", describePages(index.unreadable))
	}

	var plan []plannedMigration
	claimed := map[int64]string{}
	for _, match := range candidatePages(index, project) {
		address := strings.TrimSuffix(match.File, ".md")
		raw, err := w.Pages.ReadPage(address)
		if err != nil {
			return report, fmt.Errorf("promote: migrate card pages: read %s: %w", w.Pages.PageLocation(address), err)
		}
		block, ok := frontmatterBlock(raw)
		if !ok {
			continue
		}
		fields := parseFrontmatterFields(block)
		if !carriesLegacyKeys(fields) {
			continue
		}
		report.Legacy++
		refuse := func(kind RefusalKind, reason string) {
			report.Refused = append(report.Refused, MigrationRefusal{Address: address, Kind: kind, Reason: reason})
		}

		moved, kind, reason, err := mapPage(cards, index, claimed, project, fields)
		if err != nil {
			return report, err
		}
		if reason != "" {
			refuse(kind, reason)
			continue
		}
		entry, tracked := w.Store.Get(address)
		switch {
		case !tracked:
			refuse(RefusedUntracked, "the page was not recorded as written by longterm-mem, so its provenance is unknown")
			continue
		case !entry.MatchesPage(raw):
			refuse(RefusedEdited, "the page was edited locally since longterm-mem last wrote it")
			continue
		}

		claimed[moved.MemoryID] = address
		moved.Address = address
		newBlock := rewrittenIdentity(block, moved.MemoryID, moved.revision, moved.kind)
		entry.FrontmatterHash = hashText(newBlock)
		plan = append(plan, plannedMigration{MigratedPage: moved.MigratedPage, body: raw[len(block):], newBlock: newBlock, entry: entry})
	}

	if !apply {
		for _, p := range plan {
			report.Migrated = append(report.Migrated, p.MigratedPage)
		}
		return report, nil
	}
	return applyMigration(w, backup, plan, report)
}

// candidatePages are the pages of project that name a memory, once each, in the order the port lists them.
func candidatePages(index *pageIndex, project string) []*promotedPageMatch {
	seen := map[string]bool{}
	var pages []*promotedPageMatch
	for _, matches := range index.carriers {
		for _, match := range matches {
			if match.Project != project || seen[match.File] {
				continue
			}
			seen[match.File] = true
			pages = append(pages, match)
		}
	}
	sort.Slice(pages, func(i, j int) bool { return pages[i].order < pages[j].order })
	return pages
}

// mappedPage is the memory a page maps to, with the revision and type its new keys carry.
type mappedPage struct {
	MigratedPage
	revision, kind string
}

// mapPage decides which memory of project the page with fields is the page of. It answers the page migrated (its
// Address is set by the caller), or a refusal kind and reason, or an error when memorycard could not be read.
func mapPage(cards CardBridges, index *pageIndex, claimed map[int64]string, project string, fields map[string]string) (mappedPage, RefusalKind, string, error) {
	engram, hasEngram := identityOf(fields, engramSource)
	card, hasCard := identityOf(fields, memorycardSource)
	switch {
	case hasEngram && hasCard:
		return mappedPage{}, RefusedOther, fmt.Sprintf("it names Engram observation %s and memorycard memory %s at once", engram.ID, card.ID), nil
	case hasCard:
		memoryID, err := strconv.ParseInt(strings.TrimSpace(card.ID), 10, 64)
		if err != nil || memoryID <= 0 {
			return mappedPage{}, RefusedOther, fmt.Sprintf("memorycard id %q is not a memory id", card.ID), nil
		}
		return mappedPage{MigratedPage: MigratedPage{MemoryID: memoryID}, revision: card.Revision, kind: card.Type}, 0, "", nil
	case !hasEngram:
		return mappedPage{}, RefusedOther, "it carries legacy keys but names no memory", nil
	}

	engramID, err := strconv.ParseInt(strings.TrimSpace(engram.ID), 10, 64)
	if err != nil || engramID <= 0 {
		return mappedPage{}, RefusedOther, fmt.Sprintf("engram_id %q is not an observation id", engram.ID), nil
	}
	m, found, err := cards.MemoryByEngramID(engramID)
	if err != nil {
		return mappedPage{}, 0, "", fmt.Errorf("promote: migrate card pages: look up the memory imported from Engram observation %d: %w", engramID, err)
	}
	memoryID := strconv.FormatInt(m.ID, 10)
	switch {
	case !found:
		return mappedPage{}, RefusedNoImport, fmt.Sprintf("memorycard holds no memory imported from Engram observation %d (import_map)", engramID), nil
	case m.Project != project:
		return mappedPage{}, RefusedOther, fmt.Sprintf("memory %d belongs to project %q, not %q", m.ID, m.Project, project), nil
	}
	if own := index.ownPage(project, memory.Identity{Source: memorycardSource, ID: memoryID}); own != nil {
		return mappedPage{}, RefusedHasOwnPage, fmt.Sprintf("memory %d already has the page %s", m.ID, own.address()), nil
	}
	if other, taken := claimed[m.ID]; taken {
		return mappedPage{}, RefusedHasOwnPage, fmt.Sprintf("memory %d is already the memory of %s in this run", m.ID, other), nil
	}
	return mappedPage{MigratedPage: MigratedPage{EngramID: engramID, MemoryID: m.ID}, revision: strconv.Itoa(m.Revision), kind: m.Type}, 0, "", nil
}

// withStoreAgnosticIdentity is block with the identity keys of a page that names one memorycard memory by legacy
// memorycard_* keys replaced by the store-agnostic ones (rewrittenIdentity), keeping the memory, revision and type the
// page names. Every other block is returned as it is: a page of the store-agnostic keys, a page naming an Engram
// observation (only the migration can say which memory that is), and a page whose identity lines the line rewriter
// cannot rewrite whole (simpleIdentityLines).
func withStoreAgnosticIdentity(block string) string {
	fields := parseFrontmatterFields(block)
	if !carriesLegacyKeys(fields) || !simpleIdentityLines(block) {
		return block
	}
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

// applyMigration backs the planned pages up, then writes each and persists its precedence entry.
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
		if err := w.Pages.WritePage(p.Address, p.newBlock+p.body); err != nil {
			return report, fmt.Errorf("promote: migrate card pages: write %s: %w; the pages migrated before it stay migrated", p.Address, err)
		}
		w.Store.Set(p.Address, p.entry)
		if err := w.Precedence.SavePrecedence(w.Store); err != nil {
			return report, fmt.Errorf("promote: migrate card pages: persist the precedence entry of %s, which is already written: %w", p.Address, err)
		}
		report.Migrated = append(report.Migrated, p.MigratedPage)
	}
	return report, nil
}
