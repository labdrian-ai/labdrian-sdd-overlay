package promote

import (
	"context"
	"strconv"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// engramSourceOf is the Engram store as promotion reads its memories.
func engramSourceOf(store *engram.Store) *engram.Store { return store }

// This file is the support the tests of the package share for the model promotion reads. The tests were written
// over the two models of the two stores (an Engram observation, a memorycard memory); promotion now reads one
// (memory.Memory), so what they build is mapped here, the way each store's adapter maps its own rows, and the few
// lookups they made through helpers that no longer exist are answered over the one reading of the pages.

// obsMemory is an Engram observation as the Engram adapter maps it, without the status and the lineage the
// adapter finds in its database: an active memory with no kin.
func obsMemory(o memory.Observation) memory.Memory {
	return memory.Memory{
		Identity: memory.Identity{Source: engramSource, ID: strconv.FormatInt(o.ID, 10)},
		Project:  o.Project, Type: o.Type, Title: o.Title, Body: o.Content, TopicKey: o.TopicKey,
		Pinned: o.Pinned, Status: memory.StatusActive, Revision: o.RevisionCount, SyncID: o.SyncID,
	}
}

// PromoteCard promotes a memorycard memory with the one promotion every store goes through: it asks nothing of the
// memory's eligibility, as the promotion of a memorycard memory never did.
func (w *Writer) PromoteCard(ctx context.Context, c memory.CardMemory) (Result, error) {
	return w.Promote(ctx, memoryOfCard(c), true)
}

// EmitCardPage renders a memorycard memory as the page of the vault.
func EmitCardPage(c memory.CardMemory, address string, related []Link, at time.Time) (Page, error) {
	return EmitPage(memoryOfCard(c), address, related, at)
}

// legacyScan is what the scan of the pages for one Engram observation answered before the pages were read once
// for every store: every page that names the observation, in the order the port lists them, and the pages that
// could not be read.
type legacyScan struct {
	matches    []promotedPageMatch
	unreadable []UnreadablePage
}

func engramIdentity(id int) memory.Identity {
	return memory.Identity{Source: engramSource, ID: strconv.Itoa(id)}
}

// scanPromotedPages reads the pages and answers the pages that name the Engram observation engramID.
func scanPromotedPages(pages PageReader, engramID int) (legacyScan, error) {
	index, err := readPages(pages)
	if err != nil {
		return legacyScan{}, err
	}
	scan := legacyScan{unreadable: index.unreadable}
	for _, match := range index.carriers[engramIdentity(engramID)] {
		scan.matches = append(scan.matches, *match)
	}
	return scan, nil
}

// findPromotedPage is the page of the Engram observation engramID that was promoted under project: found, or not
// promoted at all, or not decidable because a page that could not be read may be its own.
func findPromotedPage(pages PageReader, project string, engramID int) (promotedPage, bool, error) {
	index, err := readPages(pages)
	if err != nil {
		return promotedPage{}, false, err
	}
	scan := index.scanFor(memory.Memory{Identity: engramIdentity(engramID), Project: project})
	if scan.own != nil {
		page, err := scan.own.resolve(true)
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

// findMovedPages are the pages of the Engram observation engramID left behind in a project other than project.
func findMovedPages(pages PageReader, project string, engramID int) ([]promotedPage, error) {
	index, err := readPages(pages)
	if err != nil {
		return nil, err
	}
	return index.scanFor(memory.Memory{Identity: engramIdentity(engramID), Project: project}).moved, nil
}

// allocateAddress is the address of the page of the Engram observation engramID in project when it has one, and
// a new one otherwise.
func allocateAddress(ctx context.Context, pages PageReader, project string, engramID int, addresses AddressAllocator, addressMap AddressMapRecorder, at time.Time) (string, error) {
	if existing, ok, err := findPromotedPage(pages, project, engramID); err != nil {
		return "", err
	} else if ok {
		return existing.Address, nil
	}
	return takeAddress(ctx, addresses, addressMap, at)
}

// UnreadablePages names every page of the vault that cannot be read; none when the pages cannot be listed at all.
func UnreadablePages(pages PageReader) []UnreadablePage {
	index, err := readPages(pages)
	if err != nil {
		return nil
	}
	return index.unreadable
}

// unreadablePagesError is what the lookup of an observation's page answers when no readable page carries the
// observation and some page could not be read.
type unreadablePagesError struct{ pages []UnreadablePage }

func (e *unreadablePagesError) Error() string {
	return "promote: no readable page belongs to this observation, but a page that could not be read may: " + describePages(e.pages)
}
