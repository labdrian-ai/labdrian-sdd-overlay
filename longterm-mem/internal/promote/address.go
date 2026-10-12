package promote

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultlayout"
)

// AddressAllocator is the port through which promotion obtains the address of
// a page it is about to create: the next free "c-NNNNNN" of the vault. The
// package owns it and never runs the vault's allocator itself; the
// composition root hands the Writer an adapter (internal/vault's
// AddressAllocator, which runs scripts/allocate-address.sh under its lock),
// and a test hands it a fake.
//
// The context is the caller's: cancelling it abandons the allocation. An
// allocator that cannot allocate returns an error and no address; promote
// writes nothing for that observation. An address handed out is spent
// whether or not the page is ever written, as the vault's counter is.
type AddressAllocator interface {
	NextAddress(ctx context.Context) (string, error)
}

// errNoAddressAllocator is what a promotion that needs a new address answers
// when its Writer was built without an AddressAllocator.
var errNoAddressAllocator = errors.New("promote: the writer has no address allocator")

// allocateAddress returns the vault address for the Engram observation
// identified by (project, engramID) among the pages of the vault (R-028).
// When a page already promoted carries this engram_id and project in its
// frontmatter, that page's own address is reused: the allocator is not asked
// (so a Writer that has none still re-promotes) and the address map is not
// written again. Otherwise a fresh address is taken from addresses and
// entered, through addressMap, in the vault's address map under the page's
// address-derived path, the manifest being dated at (UTC) when it has to be
// created.
func allocateAddress(ctx context.Context, pages PageReader, project string, engramID int, addresses AddressAllocator, addressMap AddressMapRecorder, at time.Time) (string, error) {
	if existing, ok, err := findPromotedPage(pages, project, engramID); err != nil {
		return "", err
	} else if ok {
		return existing.Address, nil
	}

	if memory.IsMissing(addresses) {
		return "", errNoAddressAllocator
	}
	if memory.IsMissing(addressMap) {
		return "", errNoAddressMapRecorder
	}
	address, err := addresses.NextAddress(ctx)
	if err != nil {
		return "", err
	}
	if address == "" {
		return "", errors.New("promote: the address allocator returned no address")
	}

	if err := addressMap.RecordAddress(vaultlayout.PageFile(address), address, at); err != nil {
		return "", err
	}
	return address, nil
}

// promotedPage is one already-promoted page's address and the Engram
// revision it was last promoted at (7a REFACTOR): allocateAddress's re-promotion
// address reuse (R-028) and Sync's unpromoted-or-revised gate (R-009)
// share this one wiki/memory/ scan instead of each re-implementing it.
type promotedPage struct {
	Address string
	// Revision is the engram_revision the page was last promoted at. It is
	// meaningful for the page findPromotedPage returns; findMovedPages
	// leaves it zero, since superseding a moved page patches status and
	// related only and never consults a revision.
	Revision int
	// Project is the project frontmatter value the page was promoted
	// under. It is carried rather than merely matched on because the
	// address lookup has three outcomes, not two: found under this
	// project, found under a DIFFERENT one (the observation moved between
	// Engram projects), and not promoted at all.
	Project string
}

// findPromotedPage reports the page whose frontmatter already carries this
// engram_id AND this project, with the engram_revision it was last promoted
// at (R-028 re-promotion reuse; R-009 Sync's unpromoted-or-revised gate).
// A page carrying this engram_id under a DIFFERENT project is deliberately
// NOT a match here -- see findMovedPages for that outcome and why the
// project stays part of this key.
//
// Corrupted promotion state fails closed, but only for the project that
// owns the page: a matched page with no usable address, or an unparseable
// engram_revision, errors here exactly as it always has. Widening that to
// every page sharing the engram_id was tried and reverted. This lookup is
// Sync's R-009 gate (sync.go) and Propagate's page lookup as well as
// allocateAddress's reuse, so one corrupted page under project A would have
// permanently failed project B's promotion of the one observation sharing
// that engram_id -- nothing writes an address into that page, so no run
// could ever clear it. Sync and Propagate record such an error per
// observation and continue, so the rest of project B's run is unaffected;
// the earlier wording here claimed the whole run wedged, which overstates
// it. One permanently unpromotable observation, on account of a page in a
// project it has nothing to do with, is reason enough. The page is a real
// defect; doctor is what names it, for the project it belongs to.
//
// A page that cannot be read is skipped, as the scan skips it, but it is not
// nothing: its frontmatter is unknown, so it may be this observation's page.
// A readable page that carries the observation settles the lookup, whatever
// else cannot be read (the unreadable page cannot also be its page, or the
// observation would have two). When no readable page does and some page could
// not be read, the lookup is neither "promoted" nor "not promoted": it answers
// an *unreadablePagesError, and the observation is left alone by its caller
// (Writer.Promote, Sync, Plan, Propagate), because the answer "not promoted"
// would have it written again at a new address beside a page that may be its
// own.
func findPromotedPage(pages PageReader, project string, engramID int) (promotedPage, bool, error) {
	scan, err := scanPromotedPages(pages, engramID)
	if err != nil {
		return promotedPage{}, false, err
	}
	for _, match := range scan.matches {
		if match.Project != project {
			continue
		}
		page, err := match.resolve()
		if err != nil {
			return promotedPage{}, false, err
		}
		return page, true, nil
	}
	if len(scan.unreadable) > 0 {
		return promotedPage{}, false, &unreadablePagesError{pages: scan.unreadable}
	}
	return promotedPage{}, false, nil
}

// findMovedPages returns every already-promoted page carrying this
// engram_id under a project OTHER than project: the pages an observation
// left behind when it moved between Engram projects (an ordinary Engram
// operation -- projects can be merged).
//
// This is the third outcome findPromotedPage deliberately does not have.
// The alternative -- dropping the project comparison from the lookup
// entirely and rewriting project: in place on the old page -- was rejected:
// the move is real history worth keeping, and the address would stop
// implying a stable project while Sync's R-009 gate shares this same
// lookup. So the page is recognised by engram_id, the observation is
// promoted to a fresh address under its new project, and the page it left
// is marked superseded pointing at that new address (Writer.supersedeMoved).
//
// The key is engram_id alone; project only decides which side of the split
// a matched page falls on. Two different observations that merely share a
// project never see each other here.
//
// A matched page under another project with no usable address is skipped
// rather than reported: there is no file for the successor's pointer to
// name, and failing here would let a corrupted page under project A wedge
// every promotion under project B -- the same widening findPromotedPage
// refuses. Revision is not read for a moved page (supersession patches
// status and related only), so an unparseable engram_revision on one is
// not consulted either; that corruption still fails closed for the project
// that owns the page. A page that cannot be read has no known engram_id or
// address, so it is not among the moved pages: it cannot be superseded
// because it cannot be patched, and the doctor names it.
func findMovedPages(pages PageReader, project string, engramID int) ([]promotedPage, error) {
	scan, err := scanPromotedPages(pages, engramID)
	if err != nil {
		return nil, err
	}
	var moved []promotedPage
	for _, match := range scan.matches {
		if match.Project == project || match.Address == "" {
			continue
		}
		moved = append(moved, promotedPage{Address: match.Address, Project: match.Project})
	}
	return moved, nil
}

// pageScan is what one pass over the pages found: the pages that carry the engram_id asked for, and the pages
// that could not be read at all.
type pageScan struct {
	matches    []promotedPageMatch
	unreadable []UnreadablePage
}

// UnreadablePages names every page of the vault that could not be read, in the order the port lists them. It is
// what a sync run reports as skipped, surveyed once before the run looks at its observations: it reads each page
// once and looks at none of their frontmatter.
//
// A vault whose pages cannot be listed at all, and a reader that was not wired, have no page to name, and that
// is deliberate and not a swallowed error: the same failure is the answer of every observation's own lookup
// (scanPromotedPages), which is where Sync and Plan report it, once per observation, as they always have. The
// survey adding a second report of it would change the output of the commands.
func UnreadablePages(pages PageReader) []UnreadablePage {
	var unreadable []UnreadablePage
	if err := eachPage(pages, func(address, _ string, err error) {
		if err != nil {
			unreadable = append(unreadable, UnreadablePage{File: address + ".md", Err: err})
		}
	}); err != nil {
		return nil
	}
	return unreadable
}

// eachPage reads every page of the vault once, through pages, in the order the port lists them, and hands each
// to visit with the address, the whole content, and the error if the page could not be read (the content is
// then empty). A page that cannot be read does not stop the pass. Only a pages directory that cannot be listed,
// or a reader that was not wired, is an error: there is then no page to hand over.
func eachPage(pages PageReader, visit func(address, content string, err error)) error {
	if memory.IsMissing(pages) {
		return errNoPageReader
	}
	addresses, err := pages.ListPages()
	if err != nil {
		return fmt.Errorf("promote: list %s: %w", pages.PagesLocation(), err)
	}
	for _, address := range addresses {
		content, err := pages.ReadPage(address)
		visit(address, content, err)
	}
	return nil
}

// promotedPageMatch is one wiki/memory/ page whose frontmatter carries the
// scanned engram_id, with its fields still unvalidated. Validation is
// deliberately NOT part of the scan: a page's corruption may only fail the
// lookup that actually selects it, and the two lookups select on project
// and need different fields (see findPromotedPage and findMovedPages).
type promotedPageMatch struct {
	// File is the page's base filename, named in every error so a human
	// knows which page to repair.
	File string
	// EngramID is the engram_id both the page and the scan carry, kept so
	// an error names what the page matched on.
	EngramID string
	// Project is the page's project frontmatter value, verbatim.
	Project string
	// Address is its address frontmatter value, trimmed; empty when the
	// page carries none.
	Address string
	// RawRevision is its engram_revision frontmatter value, trimmed and
	// unparsed; empty when the page carries none.
	RawRevision string
}

// resolve validates a match into a usable promotedPage. A matched page
// without an address is corrupted promotion state and errors -- the same
// discipline as the fresh path's "produced no address" guard -- never an
// empty-string reuse. A page whose engram_revision cannot be parsed as an
// integer errors the same way, rather than silently reading as revision 0
// and letting Sync skip content that is not actually current. A page with
// no engram_revision field at all is revision 0, which is "never recorded",
// not corruption.
func (m promotedPageMatch) resolve() (promotedPage, error) {
	if m.Address == "" {
		return promotedPage{}, fmt.Errorf("promote: promoted page %s matches engram_id %s but carries no address", m.File, m.EngramID)
	}
	revision := 0
	if m.RawRevision != "" {
		parsed, err := strconv.Atoi(m.RawRevision)
		if err != nil {
			return promotedPage{}, fmt.Errorf("promote: promoted page %s carries an unparseable engram_revision %q: %w", m.File, m.RawRevision, err)
		}
		revision = parsed
	}
	return promotedPage{Address: m.Address, Revision: revision, Project: m.Project}, nil
}

// scanPromotedPages reads every page of the vault once, through pages, and
// returns each one whose frontmatter carries engramID, in the order pages
// lists them, whatever project each was promoted under. A vault with no pages
// directory means nothing has been promoted yet. A matched page's own
// corruption is the caller's to judge, because it may only fail the lookup
// that selects that page.
//
// A page that cannot be read is skipped and kept apart, in pageScan.unreadable,
// as the doctor skips and names it: one link to nothing among the pages must
// not stop the promotion of every other observation. Only a pages directory
// that cannot be listed (or a reader that was not wired) errors here, because
// there is then no per-page result to salvage. What an unreadable page means
// for an observation is findPromotedPage's to say.
func scanPromotedPages(pages PageReader, engramID int) (pageScan, error) {
	wantID := strconv.Itoa(engramID)
	var found []promotedPageMatch
	var unreadable []UnreadablePage
	if err := eachPage(pages, func(address, content string, err error) {
		file := address + ".md"
		if err != nil {
			unreadable = append(unreadable, UnreadablePage{File: file, Err: err})
			return
		}
		block, ok := frontmatterBlock(content)
		if !ok {
			return
		}
		fields := parseFrontmatterFields(block)
		if fields[engramIDField] != wantID {
			return
		}
		found = append(found, promotedPageMatch{
			File:        file,
			EngramID:    wantID,
			Project:     fields[projectField],
			Address:     strings.TrimSpace(fields["address"]),
			RawRevision: strings.TrimSpace(fields[engramRevisionField]),
		})
	}); err != nil {
		return pageScan{}, err
	}
	return pageScan{matches: found, unreadable: unreadable}, nil
}

// frontmatterBlock isolates the leading `---\n...\n---\n` frontmatter block
// from a full page's raw content, so parseFrontmatterFields (lint.go)
// never sees body prose that could collide with a field name.
func frontmatterBlock(raw string) (string, bool) {
	const delim = "---\n"
	parts := strings.SplitN(raw, delim, 3)
	if len(parts) < 3 {
		return "", false
	}
	return delim + parts[1] + delim, true
}
