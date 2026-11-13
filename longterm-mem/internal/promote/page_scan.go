package promote

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// This file is the one reading of the vault's pages that a promotion decides from, whichever store a memory comes
// from: the pages are read once into a pageIndex, and a memory is looked up in it by its identity. What a page
// says of its memory is read through identityOf, so a page carrying either spelling of its identity is found.

// pageSources are the stores a page can name its memory in, in the order a page is read for them. Each is a name
// identityOf reads the page's keys for, so a store is data here and never a type.
var pageSources = []string{engramSource, memorycardSource}

// promotedPage is one already-promoted page's address and the revision it was last promoted at: what the
// decision about a memory reads of the page that is its own.
type promotedPage struct {
	Address string
	// Revision is the revision the page was last promoted at. It is meaningful for the page a match resolves to;
	// the pages of a moved memory leave it zero, since superseding a moved page patches status and related only
	// and never consults a revision.
	Revision int
	// Project is the project frontmatter value the page was promoted under. It is carried rather than merely
	// matched on because the lookup has three outcomes, not two: found under this project, found under a
	// DIFFERENT one (the memory moved between projects), and not promoted at all.
	Project string
}

// promotedPageMatch is one wiki/memory/ page whose frontmatter names a memory, with its fields still unvalidated.
// Validation is deliberately NOT part of the scan: a page's corruption may only fail the lookup that actually
// selects it, and the lookups select on project and need different fields.
type promotedPageMatch struct {
	// File is the page's base filename, named in every error so a human knows which page to repair.
	File string
	// Source and ID are the memory the page names, and IDKey the key the id was read from, kept so an error names
	// what the page matched on.
	Source string
	ID     string
	IDKey  string
	// Project is the page's project frontmatter value, verbatim.
	Project string
	// Address is its address frontmatter value, trimmed; empty when the page carries none.
	Address string
	// RawRevision is its revision frontmatter value, trimmed and unparsed; empty when the page carries none.
	// RevisionKey is the key it was read from.
	RawRevision string
	RevisionKey string
	// Block is the page's frontmatter block, what status propagation compares a memory against to see whether the
	// page needs a patch.
	Block string
	// order is the position of the page among the pages the port lists.
	order int
}

// address is the address of the page a match names, or its file when the page carries none.
func (m promotedPageMatch) address() string {
	if m.Address != "" {
		return m.Address
	}
	return m.File
}

// resolve validates a match into a usable promotedPage. A page that names no address is corrupted promotion state
// for a store whose pages always carry one (requireAddress), and errors -- never an empty-string reuse. A page
// whose revision cannot be parsed as an integer errors the same way, rather than silently reading as revision 0
// and letting a sync skip content that is not actually current. A page with no revision field at all is revision
// 0, which is "never recorded", not corruption.
func (m promotedPageMatch) resolve(requireAddress bool) (promotedPage, error) {
	if m.Address == "" && requireAddress {
		return promotedPage{}, fmt.Errorf("promote: promoted page %s matches %s %s but carries no address", m.File, m.IDKey, m.ID)
	}
	revision := 0
	if m.RawRevision != "" {
		parsed, err := strconv.Atoi(m.RawRevision)
		if err != nil {
			return promotedPage{}, fmt.Errorf("promote: promoted page %s carries an unparseable %s %q: %w", m.File, m.RevisionKey, m.RawRevision, err)
		}
		revision = parsed
	}
	return promotedPage{Address: m.address(), Revision: revision, Project: m.Project}, nil
}

// addressKey names the page of a memory in a project: the pages a successor link can name.
type addressKey struct {
	project  string
	identity memory.Identity
}

// pageIndex is one read of the vault's pages, kept so that a run decides every memory against a single read
// instead of reading the vault once per memory.
type pageIndex struct {
	// carriers maps a memory's identity to the pages that name it, whatever project each was promoted under, in the
	// order the port lists them.
	carriers map[memory.Identity][]*promotedPageMatch
	// addresses maps a project and an identity to the address of the page that carries them. The last page wins.
	addresses  map[addressKey]string
	unreadable []UnreadablePage
}

// readPages reads every page once. A page that cannot be read is kept apart (pageIndex.unreadable), as the doctor
// skips and names it: one link to nothing among the pages must not stop the promotion of every other memory. Only
// a pages directory that cannot be listed, or a reader that was not wired, errors here, because there is then no
// per-page result to salvage. What an unreadable page means for a memory is decide's to say.
func readPages(pages PageReader) (*pageIndex, error) {
	index := &pageIndex{carriers: map[memory.Identity][]*promotedPageMatch{}, addresses: map[addressKey]string{}}
	order := 0
	err := eachPage(pages, func(address, content string, err error) {
		file := address + ".md"
		order++
		if err != nil {
			index.unreadable = append(index.unreadable, UnreadablePage{File: file, Err: err})
			return
		}
		block, ok := frontmatterBlock(content)
		if !ok {
			return
		}
		fields := parseFrontmatterFields(block)
		for _, source := range pageSources {
			identity, named := identityOf(fields, source)
			if !named {
				continue
			}
			_, idKey := fieldOf(fields, source, sourceIDField, legacyKeys[source].id)
			match := &promotedPageMatch{
				File: file, Source: source, ID: identity.ID, IDKey: idKey,
				Project: fields[projectField], Address: strings.TrimSpace(fields["address"]),
				Block: block, order: order,
			}
			match.RawRevision, match.RevisionKey = revisionOf(fields, source)
			who := memory.Identity{Source: source, ID: identity.ID}
			index.carriers[who] = append(index.carriers[who], match)
			index.noteAddress(match.Project, who, match.address())
		}
	})
	if err != nil {
		return nil, err
	}
	return index, nil
}

// noteAddress records the address of the page of identity in project, as the page resolves to it (its address, or its
// file when it carries none): the memories after it see the page it has, so a successor link names it.
func (p *pageIndex) noteAddress(project string, identity memory.Identity, address string) {
	if address != "" {
		p.addresses[addressKey{project: project, identity: identity}] = address
	}
}

// ownPage is the first page that names identity under project, or nil.
func (p *pageIndex) ownPage(project string, identity memory.Identity) *promotedPageMatch {
	for _, match := range p.carriers[identity] {
		if match.Project == project {
			return match
		}
	}
	return nil
}

// pageScan is what the pages say about one memory.
type pageScan struct {
	// own is the page already promoted from this memory in its project, if there is one.
	own *promotedPageMatch
	// bridged is the page of another store's memory that is the same piece of memory, if it has one, and
	// bridgedAs how the two are tied: the store's import record (the memory's lineage), or only the content they
	// share (its mirrors).
	bridged   *promotedPageMatch
	bridgedAs memory.Identity
	mirrored  bool
	// moved are the pages the memory left behind in another project; a page that carries no address is not among them.
	moved []promotedPage
	// successor is the address of the page of the memory that replaced this one, in its project, or empty.
	successor  string
	unreadable []UnreadablePage
}

// scanFor answers pageScan for m from the pages read. The page that bridges a memory is the one of its lineage
// first, then of its mirrors, whatever project that page was promoted under (C22-FU bridges across projects on
// purpose); among equals, the first page the port lists wins.
func (p *pageIndex) scanFor(m memory.Memory) pageScan {
	scan := pageScan{own: p.ownPage(m.Project, m.Identity), unreadable: p.unreadable}
	for _, match := range p.carriers[m.Identity] {
		if match.Project != m.Project && match.Address != "" {
			scan.moved = append(scan.moved, promotedPage{Address: match.Address, Project: match.Project})
		}
	}
	if m.Successor.ID != "" {
		scan.successor = p.addresses[addressKey{project: m.Project, identity: m.Successor}]
	}
	best, bestRank := (*promotedPageMatch)(nil), 2
	consider := func(kin []memory.Identity, rank int) {
		for _, identity := range kin {
			for _, match := range p.carriers[identity] {
				if rank < bestRank || (rank == bestRank && match.order < best.order) {
					best, bestRank = match, rank
					scan.bridgedAs, scan.mirrored = identity, rank == 1
				}
			}
		}
	}
	consider(m.Lineage, 0)
	consider(m.Mirrored, 1)
	scan.bridged = best
	return scan
}

// successorLinks is the related list of m's page: the successor's page when m has a successor that has one, named
// as a page renders it, and nothing otherwise.
func (s pageScan) successorLinks(m memory.Memory) []Link {
	if s.successor == "" {
		return nil
	}
	return []Link{{Address: s.successor, Title: m.SuccessorTitle}}
}

// decision is what a promotion does about one memory, given what the pages say about it.
type decision int

const (
	// decideCreate: no page belongs to the memory and every page could be read: a new page is written.
	decideCreate decision = iota
	// decideUpdate: the memory's own page is at an older revision: it is brought up to date in place.
	decideUpdate
	// decideCurrent: the memory's own page is at its revision, or a newer one: nothing is written.
	decideCurrent
	// decideBridged: the memory is the same piece of memory as one of another store that has a page
	// (BridgedError): nothing is written, and it is no failure.
	decideBridged
	// decideWithheld: no readable page belongs to the memory and a page could not be read, so it cannot be told
	// whether the memory is promoted: nothing is written, and it is no failure.
	decideWithheld
)

// decide is the one decision a promotion makes about m over a scan, shared by SyncCards, PlanCards and the explicit promote,
// so that they cannot drift apart. It reads nothing and writes nothing. The memory's own page decides first: at
// its revision or newer, nothing is written (rewriteCurrent says a promote that rewrites a page at its revision
// anyway, as the explicit promote of an Engram observation always has), at an older one, it is updated. A memory
// with no page of its own but a page of the same piece of memory in another store is bridged. It fails only on a
// page of the memory that cannot be read as one.
func decide(m memory.Memory, scan pageScan, style pageStyle, rewriteCurrent bool) (decision, error) {
	switch {
	case scan.own != nil:
		own, err := scan.own.resolve(style.requireAddress)
		if err != nil {
			return decideCurrent, err
		}
		if m.Revision <= own.Revision && !rewriteCurrent {
			return decideCurrent, nil
		}
		return decideUpdate, nil
	case scan.bridged != nil:
		return decideBridged, nil
	case len(scan.unreadable) > 0:
		return decideWithheld, nil
	}
	return decideCreate, nil
}
