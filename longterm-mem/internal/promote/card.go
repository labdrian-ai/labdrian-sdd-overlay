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
	// SameContent reports that the memory has no bridge: it holds the same content as that observation (the
	// content hash memorycard computes, in the same project), which is how a memory mirrored by hand is found.
	SameContent bool
}

func (e *BridgedError) Error() string {
	if e.SameContent {
		return fmt.Sprintf("promote: memorycard memory %d has the same content as Engram observation %d, which already has the page %s; it is not promoted from memorycard, and the page %s is left as it is",
			e.MemoryID, e.EngramID, e.Address, e.Address)
	}
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
	decision, err := decideCard(m, scan)
	if err != nil {
		return Result{}, err
	}
	return w.applyCard(ctx, m, scan, decision)
}

// cardDecision is what a promotion does about one memory of memorycard, given what the pages say about it.
type cardDecision int

const (
	// cardCreate: no page belongs to the memory and every page could be read: a new page is written.
	cardCreate cardDecision = iota
	// cardUpdate: the memory's own page is at an older revision: it is brought up to date in place.
	cardUpdate
	// cardCurrent: the memory's own page is at its revision, or a newer one: nothing is written.
	cardCurrent
	// cardBridged: the memory is bridged to an Engram observation that has a page (BridgedError): nothing is
	// written.
	cardBridged
	// cardWithheld: no readable page belongs to the memory and a page could not be read, so it cannot be told
	// whether the memory is promoted: nothing is written.
	cardWithheld
)

// decideCard is the one decision a promotion of m makes over a scan, shared by PromoteCard (and so by
// ExplicitPromoteCard) and by SyncCards, so that the two cannot drift apart. It reads nothing and writes
// nothing. It fails only on a page of the memory that carries a revision it cannot read.
func decideCard(m memory.CardMemory, scan cardScan) (cardDecision, error) {
	switch {
	case scan.bridged != nil:
		return cardBridged, nil
	case scan.own != nil:
		onDisk := 0
		if raw := scan.own.RawRevision; raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil {
				return cardCurrent, fmt.Errorf("promote: promoted page %s carries an unparseable %s %q: %w", scan.own.File, scan.own.RevisionKey, raw, err)
			}
			onDisk = parsed
		}
		if m.Revision <= onDisk {
			return cardCurrent, nil
		}
		return cardUpdate, nil
	case len(scan.unreadable) > 0:
		return cardWithheld, nil
	}
	return cardCreate, nil
}

// applyCard writes what decision says about m: the one write path of a memory of memorycard.
func (w *Writer) applyCard(ctx context.Context, m memory.CardMemory, scan cardScan, decision cardDecision) (Result, error) {
	switch decision {
	case cardBridged:
		engramID, _ := strconv.ParseInt(scan.bridged.EngramID, 10, 64)
		return Result{}, &BridgedError{MemoryID: m.ID, EngramID: engramID, Address: scan.bridged.address(), SameContent: engramID != m.EngramID}
	case cardCurrent:
		address := scan.own.address()
		return Result{Page: Page{Address: address, Path: vaultlayout.PageFile(address)}, Action: Action{Kind: ActionAlreadyPromoted}}, nil
	case cardUpdate:
		return w.updateCardPage(m, scan)
	case cardWithheld:
		return Result{Action: Action{Kind: ActionSkippedUnreadablePages, Diagnostic: &Diagnostic{
			Rule: "unreadable-pages",
			Detail: fmt.Sprintf("memorycard memory %d was not promoted: no readable page belongs to it, but a page that could not be read may: %s; nothing was written for it",
				m.ID, describePages(scan.unreadable)),
		}}}, nil
	}
	return w.createCardPage(ctx, m, scan)
}

// createCardPage writes m as a new page: it takes an address and publishes the page the way Promote publishes a
// new one.
func (w *Writer) createCardPage(ctx context.Context, m memory.CardMemory, scan cardScan) (Result, error) {
	address, err := takeAddress(ctx, w.Addresses, w.AddressMap, utc(w.Clock))
	if err != nil {
		return Result{}, err
	}
	related := cardRelated(m, scan.byCardID)
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

// updateCardPage brings the page already promoted from m up to m's revision, in place and the way the Engram path
// does (UpdateInPlace: a page edited by hand since is skipped, not overwritten). A page at m's revision, or at a
// newer one, is left alone (cardCurrent) and reported as ActionAlreadyPromoted. Only a page that carries
// memorycard_id is ever updated here: the scan never selects another.
func (w *Writer) updateCardPage(m memory.CardMemory, scan cardScan) (Result, error) {
	address := scan.own.address()
	related := cardRelated(m, scan.byCardID)
	page, err := EmitCardPage(m, address, related, utc(w.Clock))
	if err != nil {
		return Result{}, err
	}
	action, err := UpdateInPlace(w.Pages, w.Store, page)
	if err != nil {
		return Result{}, err
	}
	if action.Kind != ActionSkippedLocalEdit {
		if err := w.Precedence.SavePrecedence(w.Store); err != nil {
			return Result{}, err
		}
		if err := w.register(address, m.Title); err != nil {
			return Result{}, err
		}
	}
	return Result{Page: page, Action: action}, nil
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
	index, err := readCardPages(pages)
	if err != nil {
		return cardScan{}, err
	}
	return index.scanFor(m), nil
}

// cardPages is one read of the vault's pages, kept so that a run decides every memory it lists against a single
// read (readCardPages) instead of reading the vault once per memory.
type cardPages struct {
	// engramPages are the pages that carry an engram_id, in the order the port lists them: the pages a memory
	// can be bridged to.
	engramPages []promotedPageMatch
	// own maps a project and a memorycard_id to the first page that carries them.
	own map[cardKey]*promotedPageMatch
	// blocks maps the same key to the frontmatter block of that first page: what status propagation compares a
	// memory against to see whether the page needs a patch.
	blocks map[cardKey]string
	// byCardID maps, per project, the memorycard_id of each page that has an address to that address: the pages
	// a successor link can name. The last page of an id wins.
	byCardID   map[string]map[int64]string
	unreadable []UnreadablePage
}

// cardKey names a page of memorycard: its project and its memorycard_id.
type cardKey struct {
	project string
	id      int64
}

// readCardPages reads every page once. A page that cannot be read is kept apart, as the Engram scan keeps it.
func readCardPages(pages PageReader) (*cardPages, error) {
	index := &cardPages{own: map[cardKey]*promotedPageMatch{}, blocks: map[cardKey]string{}, byCardID: map[string]map[int64]string{}}
	err := eachPage(pages, func(address, content string, err error) {
		file := address + ".md"
		if err != nil {
			index.unreadable = append(index.unreadable, UnreadablePage{File: file, Err: err})
			return
		}
		block, ok := frontmatterBlock(content)
		if !ok {
			return
		}
		fields := parseFrontmatterFields(block)
		match := promotedPageMatch{File: file, Project: fields[projectField], Address: strings.TrimSpace(fields["address"])}
		card, cardNamed := identityOf(fields, memorycardSource)
		match.RawRevision, match.RevisionKey = revisionOf(fields, memorycardSource)
		if engram, named := identityOf(fields, engramSource); named {
			engramMatch := match
			engramMatch.EngramID = engram.ID
			engramMatch.RawRevision, engramMatch.RevisionKey = "", ""
			index.engramPages = append(index.engramPages, engramMatch)
		}
		if id, err := strconv.ParseInt(strings.TrimSpace(card.ID), 10, 64); cardNamed && err == nil {
			key := cardKey{project: match.Project, id: id}
			if index.own[key] == nil {
				index.own[key] = &match
				index.blocks[key] = block
			}
			index.noteAddress(match.Project, id, match.Address)
		}
	})
	if err != nil {
		return nil, err
	}
	return index, nil
}

// noteAddress records the address of the page of memorycard memory id in project, when it has one.
func (p *cardPages) noteAddress(project string, id int64, address string) {
	if address == "" {
		return
	}
	if p.byCardID[project] == nil {
		p.byCardID[project] = map[int64]string{}
	}
	p.byCardID[project][id] = address
}

// scanFor answers cardScan for m from the pages read.
func (p *cardPages) scanFor(m memory.CardMemory) cardScan {
	scan := cardScan{byCardID: p.byCardID[m.Project], unreadable: p.unreadable, own: p.own[cardKey{project: m.Project, id: m.ID}]}
	// The observations whose pages bridge the memory: the one it was imported from, then the ones that hold its
	// content. A page of the first wins when both have one.
	wanted := map[string]int{}
	if m.EngramID > 0 {
		wanted[strconv.FormatInt(m.EngramID, 10)] = 0
	}
	for _, id := range m.SameContentEngramIDs {
		if _, seen := wanted[strconv.FormatInt(id, 10)]; !seen {
			wanted[strconv.FormatInt(id, 10)] = 1
		}
	}
	bridgedRank := 2
	for i := range p.engramPages {
		if rank, ok := wanted[p.engramPages[i].EngramID]; ok && rank < bridgedRank {
			scan.bridged = &p.engramPages[i]
			bridgedRank = rank
		}
	}
	return scan
}

// EmitCardPage renders m as a vault page at address: the frontmatter of EmitPage with memorycard_* extras in
// place of the engram_* ones, related links from the caller-resolved related slice, and a body that carries
// m.Body behind an H1 title (omitted when the body already opens with one) and a footer naming the source.
func EmitCardPage(m memory.CardMemory, address string, related []Link, at time.Time) (Page, error) {
	if address == "" {
		return Page{}, fmt.Errorf("promote: emit page for memorycard memory %d: address is required", m.ID)
	}
	today := at.UTC().Format("2006-01-02")
	status := cardStatus(m)
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

// cardStatus is the status line of m's page: superseded and archived follow the memory, a pinned memory is mature,
// any other is developing. EmitCardPage renders it and status propagation patches to it, so the two agree.
func cardStatus(m memory.CardMemory) string {
	switch {
	case m.Status == "superseded":
		return "superseded"
	case m.Status == "archived":
		return "archived"
	case m.Pinned:
		return "mature"
	}
	return "developing"
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
