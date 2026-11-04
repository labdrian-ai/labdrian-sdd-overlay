package promote

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultlayout"
)

// This file promotes a memory of memorycard to a page of the vault. Engram's pages are keyed by engram_id and
// stay so: the 558 pages that exist keep resolving against Engram's database, and a promote from memorycard
// never writes one of them, nor any other page that exists. A memorycard page carries memorycard_id,
// memorycard_revision and memorycard_type instead, and is found again by (project, memorycard_id).

// The frontmatter keys a memorycard page carries and a scan reads back.
const (
	memorycardIDField       = "memorycard_id"
	memorycardTypeField     = "memorycard_type"
	memorycardRevisionField = "memorycard_revision"
)

// CardLookup resolves one memory of memorycard by id: the function seam ExplicitPromoteCard reads through, as
// ObservationLookup is for Engram. ok is false, with a nil error, when memorycard holds no such memory.
type CardLookup func(id int64) (memory.CardMemory, bool, error)

// BridgedError is the refusal of a memory that is bridged to an Engram observation which already has a page:
// the memory was imported from that observation (memorycard's import_map), and its history stays on that page.
// Promoting it would make a second page for one piece of memory, and updating the first would write a page
// that exists, so a promote does neither.
type BridgedError struct {
	// MemoryID is the memorycard memory that was refused.
	MemoryID int64
	// EngramID is the observation it was imported from.
	EngramID int64
	// Address is the page that observation already has.
	Address string
}

func (e *BridgedError) Error() string {
	return fmt.Sprintf("promote: memorycard memory %d is bridged to Engram observation %d, which already has the page %s; it is not promoted from memorycard, and the page %s is left as it is",
		e.MemoryID, e.EngramID, e.Address, e.Address)
}

// ExplicitPromoteCard promotes the memorycard memory named by id (PromoteCard). Like ExplicitPromote it is
// explicit: nothing about the memory has to make it eligible.
func ExplicitPromoteCard(ctx context.Context, w *Writer, lookup CardLookup, id int64) (Result, error) {
	m, ok, err := lookup(id)
	if err != nil {
		return Result{}, fmt.Errorf("promote: look up memorycard memory %d: %w", id, err)
	}
	if !ok {
		return Result{}, fmt.Errorf("%w: memorycard memory %d", ErrObservationNotFound, id)
	}
	return w.PromoteCard(ctx, m)
}

// PromoteCard writes m as a new page of the vault, and only as a new page.
//
// It refuses (BridgedError) a memory whose Engram observation already has a page, whatever project that page
// is in. It answers ActionAlreadyPromoted, with the page's address and nothing written, for a memory that
// already has a page of the same project: a later revision of the memory does not reach that page, because no
// existing page is written. It leaves the memory alone (ActionSkippedUnreadablePages) when a page could not be
// read and none that could be read decides the memory, since the page it could not read may be the memory's.
// Otherwise it takes an address and publishes the page the way Promote publishes a new one: fingerprint first,
// page second, then the catalog and the log.
func (w *Writer) PromoteCard(ctx context.Context, m memory.CardMemory) (Result, error) {
	if err := w.checkPorts(); err != nil {
		return Result{}, err
	}
	scan, err := scanCardPages(w.Pages, m)
	if err != nil {
		return Result{}, err
	}
	if scan.bridged != nil {
		return Result{}, &BridgedError{MemoryID: m.ID, EngramID: m.EngramID, Address: scan.bridged.address()}
	}
	if scan.own != nil {
		address := scan.own.address()
		return Result{Page: Page{Address: address, Path: vaultlayout.PageFile(address)}, Action: Action{Kind: ActionAlreadyPromoted}}, nil
	}
	if len(scan.unreadable) > 0 {
		return Result{Action: Action{Kind: ActionSkippedUnreadablePages, Diagnostic: &Diagnostic{
			Rule: "unreadable-pages",
			Detail: fmt.Sprintf("memorycard memory %d was not promoted: no readable page belongs to it, but a page that could not be read may: %s; nothing was written for it",
				m.ID, describePages(scan.unreadable)),
		}}}, nil
	}

	address, err := takeAddress(ctx, w.Addresses, w.AddressMap, utc(w.Clock))
	if err != nil {
		return Result{}, err
	}
	var related []Link
	if m.SupersededBy > 0 {
		if successor, ok := scan.byCardID[m.SupersededBy]; ok {
			related = append(related, Link{Address: successor, Title: m.SupersededByTitle})
		}
	}
	page, err := EmitCardPage(m, address, related, utc(w.Clock))
	if err != nil {
		return Result{}, err
	}
	if err := w.publishNew(address, page, w.Pages.PageLocation(address)); err != nil {
		return Result{}, err
	}
	if err := w.register(address, m.Title); err != nil {
		return Result{}, err
	}
	return Result{Page: page, Action: Action{Kind: ActionCreated}}, nil
}

// cardScan is what one pass over the pages found out about a memory of memorycard.
type cardScan struct {
	// bridged is the page of the Engram observation the memory was imported from, if it has one.
	bridged *promotedPageMatch
	// own is the page already promoted from this memory in its project, if there is one.
	own *promotedPageMatch
	// byCardID maps the memorycard_id of each page of the memory's project to its address: the pages a
	// successor link can name.
	byCardID   map[int64]string
	unreadable []UnreadablePage
}

// address is the address of the page a match names, or its file when the page carries none.
func (m promotedPageMatch) address() string {
	if m.Address != "" {
		return m.Address
	}
	return m.File
}

// scanCardPages reads every page once and answers cardScan for m. A page that cannot be read is kept apart,
// as the Engram scan keeps it.
func scanCardPages(pages PageReader, m memory.CardMemory) (cardScan, error) {
	scan := cardScan{byCardID: map[int64]string{}}
	wantEngram := ""
	if m.EngramID > 0 {
		wantEngram = strconv.FormatInt(m.EngramID, 10)
	}
	err := eachPage(pages, func(address, content string, err error) {
		file := address + ".md"
		if err != nil {
			scan.unreadable = append(scan.unreadable, UnreadablePage{File: file, Err: err})
			return
		}
		block, ok := frontmatterBlock(content)
		if !ok {
			return
		}
		fields := parseFrontmatterFields(block)
		match := promotedPageMatch{File: file, Project: fields[projectField], Address: strings.TrimSpace(fields["address"])}
		if wantEngram != "" && fields[engramIDField] == wantEngram && scan.bridged == nil {
			match.EngramID = wantEngram
			scan.bridged = &match
		}
		if id, err := strconv.ParseInt(strings.TrimSpace(fields[memorycardIDField]), 10, 64); err == nil && match.Project == m.Project {
			if id == m.ID && scan.own == nil {
				scan.own = &match
			}
			if match.Address != "" {
				scan.byCardID[id] = match.Address
			}
		}
	})
	return scan, err
}

// EmitCardPage renders m as a vault page at address: the frontmatter of EmitPage with memorycard_* extras in
// place of the engram_* ones, related links from the caller-resolved related slice, and a body that carries
// m.Body behind an H1 title (omitted when the body already opens with one) and a footer naming the source.
func EmitCardPage(m memory.CardMemory, address string, related []Link, at time.Time) (Page, error) {
	if address == "" {
		return Page{}, fmt.Errorf("promote: emit page for memorycard memory %d: address is required", m.ID)
	}
	today := at.UTC().Format("2006-01-02")
	status := "developing"
	switch {
	case m.Status == "superseded":
		status = "superseded"
	case m.Status == "archived":
		status = "archived"
	case m.Pinned:
		status = "mature"
	}
	tags := []string{"memory", m.Type}
	if goal, ok := memory.GoalOfTopic(m.TopicKey); ok {
		tags = append(tags, memory.GoalTag(goal))
	}

	var b strings.Builder
	b.WriteString("---\n")
	writeField(&b, "type", vaultType)
	writeField(&b, titleField, quoteYAML(m.Title))
	writeField(&b, "address", address)
	writeListField(&b, aliasesField, []string{m.Title})
	writeField(&b, "created", today)
	writeField(&b, "updated", today)
	writeListField(&b, tagsField, tags)
	writeField(&b, statusField, status)
	writeListField(&b, relatedField, relatedWikilinks(related))
	writeListField(&b, "sources", nil)
	writeField(&b, memorycardIDField, strconv.FormatInt(m.ID, 10))
	writeField(&b, memorycardTypeField, m.Type)
	writeField(&b, memorycardRevisionField, strconv.Itoa(m.Revision))
	writeField(&b, projectField, m.Project)
	b.WriteString("---\n")

	return Page{Address: address, Path: vaultlayout.PageFile(address), Frontmatter: b.String(), Body: renderCardBody(m)}, nil
}

// renderCardBody is renderBody for a memory of memorycard.
func renderCardBody(m memory.CardMemory) string {
	content := strings.TrimRight(m.Body, "\n")
	var b strings.Builder
	if !strings.HasPrefix(strings.TrimSpace(content), "# ") {
		b.WriteString("# ")
		b.WriteString(m.Title)
		b.WriteString("\n\n")
	}
	b.WriteString(content)
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "---\nPromoted from memorycard memory %d, revision %d.\n", m.ID, m.Revision)
	return b.String()
}
