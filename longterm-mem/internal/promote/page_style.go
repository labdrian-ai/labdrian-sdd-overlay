package promote

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultlayout"
)

// pageStyle is how the pages of one store's memories are written and what the promotion of one of them says, as
// it has been since each store's pages were written with keys of their own (engram_*, memorycard_*). It is the one
// place these differences live: the promotion walk and its decision are the same for every store and read a
// style for what they cannot share. When pages carry store-agnostic keys only (T6 of the store-agnostic
// promotion), the styles come together.
type pageStyle struct {
	// noun names a memory of the store in what a promotion says of it ("observation", "memorycard memory").
	noun string
	// emit renders a memory as a vault page at an address.
	emit func(m memory.Memory, address string, related []Link, at time.Time) (Page, error)
	// linksSuccessor says that the page of a superseded memory is written with a link to its successor's page. An
	// Engram page is written without one, and propagation adds it.
	linksSuccessor bool
	// rewriteCurrent says that a promote of a memory whose own page is at its revision rewrites the page in place
	// anyway, the repair path an explicit promote of an Engram observation has always been, and not that it answers
	// that the memory is already promoted.
	rewriteCurrent bool
	// requireAddress says that a page of the store that names no address is corrupted promotion state.
	requireAddress bool
}

var pageStyles = map[string]pageStyle{
	engramSource:     {noun: "observation", emit: emitEngramPage, rewriteCurrent: true, requireAddress: true},
	memorycardSource: {noun: "memorycard memory", emit: emitCardPage, linksSuccessor: true},
}

// styleOf is the style of the pages of source, or an error naming the store when none is known.
func styleOf(source string) (pageStyle, error) {
	style, ok := pageStyles[source]
	if !ok {
		return pageStyle{}, fmt.Errorf("promote: no page format is known for source %q", source)
	}
	return style, nil
}

// numericID is the id of m as the integer the pages of the stores record it as.
func numericID(m memory.Memory) (int64, error) {
	id, err := strconv.ParseInt(m.Identity.ID, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("promote: %s %q has an id that is not a number: %w", m.Identity.Source, m.Identity.ID, err)
	}
	return id, nil
}

// words is how a memory of a store is named in what a promotion says: "Engram observation 3", "memorycard memory
// 7", and the store it is promoted from.
func words(identity memory.Identity) (named, from string) {
	switch identity.Source {
	case engramSource:
		return "Engram observation " + identity.ID, "Engram"
	case memorycardSource:
		return "memorycard memory " + identity.ID, "memorycard"
	}
	return identity.Source + " memory " + identity.ID, identity.Source
}

// BridgedError is the refusal of a memory that is the same piece of memory as one of another store which already
// has a page: a memorycard memory imported from an Engram observation, an Engram observation a memorycard memory
// was imported from, or a memory that only holds the content of one. Its history stays on that page, and
// promoting it would make a second page for one piece of memory (updating the first would write a page that
// exists), so a promote does neither.
type BridgedError struct {
	// Memory is the memory that was refused and Peer the one whose page it is bridged to.
	Memory memory.Identity
	Peer   memory.Identity
	// Address is the page the peer already has.
	Address string
	// SameContent reports that the memory has no import record tying it to the peer: it holds the same content
	// (the content hash memorycard computes, in the same project), which is how a memory mirrored by hand is found.
	SameContent bool
}

func (e *BridgedError) Error() string {
	memoryNamed, from := words(e.Memory)
	peerNamed, _ := words(e.Peer)
	if e.SameContent {
		return fmt.Sprintf("promote: %s has the same content as %s, which already has the page %s; it is not promoted from %s, and the page %s is left as it is",
			memoryNamed, peerNamed, e.Address, from, e.Address)
	}
	return fmt.Sprintf("promote: %s is bridged to %s, which already has the page %s; it is not promoted from %s, and the page %s is left as it is",
		memoryNamed, peerNamed, e.Address, from, e.Address)
}

// emitEngramPage renders an Engram observation as a vault page: the frontmatter of the contract with the Engram
// type mapped onto the vault's own type contract (concept, D7) plus flat engram_* extras, related links from the
// caller-resolved related slice, and a body carrying the content behind an H1 title (omitted when the content
// already opens with one) plus a footer naming the Engram source.
func emitEngramPage(m memory.Memory, address string, related []Link, at time.Time) (Page, error) {
	if address == "" {
		return Page{}, fmt.Errorf("promote: emit page for observation %s: address is required", m.Identity.ID)
	}
	id, err := numericID(m)
	if err != nil {
		return Page{}, err
	}

	today := at.UTC().Format("2006-01-02")
	fm := frontmatter{
		Title: m.Title, Address: address, Aliases: []string{m.Title},
		Created: today, Updated: today,
		Tags: pageTags(m), Status: statusFor(m),
		Related:        relatedWikilinks(related),
		EngramID:       id,
		EngramSyncID:   m.SyncID,
		EngramType:     m.Type,
		EngramRevision: m.Revision,
		Project:        m.Project,
	}
	return Page{Address: address, Path: vaultlayout.PageFile(address), Frontmatter: fm.Render(), Body: renderBody(m, id)}, nil
}

// emitCardPage renders a memory of memorycard as a vault page at address: the frontmatter of the Engram page with
// memorycard_* extras in place of the engram_* ones, related links from the caller-resolved related slice, and a
// body that carries the body behind an H1 title (omitted when the body already opens with one) and a footer naming
// the source.
func emitCardPage(m memory.Memory, address string, related []Link, at time.Time) (Page, error) {
	if address == "" {
		return Page{}, fmt.Errorf("promote: emit page for memorycard memory %s: address is required", m.Identity.ID)
	}
	id, err := numericID(m)
	if err != nil {
		return Page{}, err
	}
	today := at.UTC().Format("2006-01-02")
	tags := pageTags(m)

	var b strings.Builder
	b.WriteString("---\n")
	writeField(&b, "type", vaultType)
	writeField(&b, titleField, quoteYAML(m.Title))
	writeField(&b, "address", address)
	writeListField(&b, aliasesField, []string{m.Title})
	writeField(&b, "created", today)
	writeField(&b, "updated", today)
	writeListField(&b, tagsField, tags)
	writeField(&b, statusField, cardStatus(m))
	writeListField(&b, relatedField, relatedWikilinks(related))
	writeListField(&b, "sources", nil)
	writeField(&b, memorycardIDField, strconv.FormatInt(id, 10))
	writeField(&b, memorycardTypeField, m.Type)
	writeField(&b, memorycardRevisionField, strconv.Itoa(m.Revision))
	writeField(&b, projectField, m.Project)
	b.WriteString("---\n")

	return Page{Address: address, Path: vaultlayout.PageFile(address), Frontmatter: b.String(), Body: renderCardBody(m, id)}, nil
}

// The frontmatter keys a memorycard page carries and a scan reads back.
const (
	memorycardIDField       = "memorycard_id"
	memorycardTypeField     = "memorycard_type"
	memorycardRevisionField = "memorycard_revision"
)

// cardStatus is the status line of a memorycard memory's page: superseded and archived follow the memory, a pinned
// memory is mature, any other is developing. emitCardPage renders it and status propagation patches to it, so the
// two agree.
func cardStatus(m memory.Memory) string {
	switch {
	case m.Status == memory.StatusSuperseded:
		return "superseded"
	case m.Status == memory.StatusArchived:
		return "archived"
	case m.Pinned:
		return "mature"
	}
	return "developing"
}

// renderCardBody is renderBody for a memory of memorycard.
func renderCardBody(m memory.Memory, id int64) string {
	content := strings.TrimRight(m.Body, "\n")
	var b strings.Builder
	if !strings.HasPrefix(strings.TrimSpace(content), "# ") {
		b.WriteString("# ")
		b.WriteString(m.Title)
		b.WriteString("\n\n")
	}
	b.WriteString(content)
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "---\nPromoted from memorycard memory %d, revision %d.\n", id, m.Revision)
	return b.String()
}
