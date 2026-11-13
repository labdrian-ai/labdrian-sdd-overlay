package promote

import (
	"context"
	"strconv"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

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

// asMemorycard is the memories as the one store the commands read serves them: the walks read the page style of the
// store a memory names, and the stand-in port serves the memories of one.
func asMemorycard(memories []memory.Memory) []memory.Memory {
	served := make([]memory.Memory, len(memories))
	for i, m := range memories {
		m.Identity.Source = memorycardSource
		served[i] = m
	}
	return served
}

// obsMemories maps a list of observations with obsMemory.
func obsMemories(observations []memory.Observation) []memory.Memory {
	var memories []memory.Memory
	for _, o := range observations {
		memories = append(memories, obsMemory(o))
	}
	return memories
}

// cardMem is a memorycard memory as the memorycard adapter maps it: the observation it was imported from is its
// lineage, and the observations that hold its content, when it has no import row, its mirrors.
func cardMem(c memory.CardMemory) memory.Memory {
	m := memory.Memory{
		Identity: memory.Identity{Source: memorycardSource, ID: strconv.FormatInt(c.ID, 10)},
		Project:  c.Project, Type: c.Type, Title: c.Title, Body: c.Body, TopicKey: c.TopicKey,
		Pinned: c.Pinned, Status: c.Status, SuccessorTitle: c.SupersededByTitle, Revision: c.Revision,
	}
	if c.SupersededBy != 0 {
		m.Successor = memory.Identity{Source: memorycardSource, ID: strconv.FormatInt(c.SupersededBy, 10)}
	}
	if c.EngramID != 0 {
		m.Lineage = append(m.Lineage, memory.Identity{Source: engramSource, ID: strconv.FormatInt(c.EngramID, 10)})
	}
	if !c.Mapped {
		for _, id := range c.SameContentEngramIDs {
			m.Mirrored = append(m.Mirrored, memory.Identity{Source: engramSource, ID: strconv.FormatInt(id, 10)})
		}
	}
	return m
}

// cardMems maps a list of memorycard memories with cardMem.
func cardMems(cards []memory.CardMemory) []memory.Memory {
	var memories []memory.Memory
	for _, c := range cards {
		memories = append(memories, cardMem(c))
	}
	return memories
}

// PromoteCard promotes a memorycard memory with the one promotion every store goes through: it asks nothing of the
// memory's eligibility, as the promotion of a memorycard memory never did.
func (w *Writer) PromoteCard(ctx context.Context, c memory.CardMemory) (Result, error) {
	return w.Promote(ctx, cardMem(c), true)
}

// EmitCardPage renders a memorycard memory as the page of the vault.
func EmitCardPage(c memory.CardMemory, address string, related []Link, at time.Time) (Page, error) {
	return EmitPage(cardMem(c), address, related, at)
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

// statusOfObservation is the rule of R-033 over the observations of one project, as the Engram adapter applies
// it: the older side of a supersedes edge is superseded by the newer, a soft-deleted observation is archived.
func statusOfObservation(o memory.Observation, edges []memory.Edge, all []memory.Observation) (string, *memory.Observation) {
	for _, edge := range edges {
		if edge.Relation != "supersedes" {
			continue
		}
		other := edge.SourceSyncID
		if o.SyncID != "" && other == o.SyncID {
			other = edge.TargetSyncID
		}
		for i := range all {
			if all[i].SyncID == other && all[i].CreatedAt > o.CreatedAt {
				return memory.StatusSuperseded, &all[i]
			}
		}
	}
	if o.DeletedAt != "" {
		return memory.StatusArchived, nil
	}
	return memory.StatusActive, nil
}

// ObservationLookup is a Source that serves one observation by id: what the tests of the explicit promote and of
// the reindexing one need of a store.
type ObservationLookup func(id int64) (memory.Observation, bool, error)

// ByID looks the observation up by its numeric id and maps it as the Engram adapter does.
func (f ObservationLookup) ByID(id string) (memory.Memory, bool, error) {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return memory.Memory{}, false, nil
	}
	o, ok, err := f(n)
	if err != nil || !ok {
		return memory.Memory{}, false, err
	}
	return obsMemory(o), true, nil
}

func (f ObservationLookup) Candidates(string) ([]memory.Memory, error) { return nil, nil }
func (f ObservationLookup) Memories(string) ([]memory.Memory, error)   { return nil, nil }

// CardLookup is the same for a memorycard memory.
type CardLookup func(id int64) (memory.CardMemory, bool, error)

// ByID looks the memory up by its numeric id and maps it as the memorycard adapter does.
func (f CardLookup) ByID(id string) (memory.Memory, bool, error) {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return memory.Memory{}, false, nil
	}
	c, ok, err := f(n)
	if err != nil || !ok {
		return memory.Memory{}, false, err
	}
	return cardMem(c), true, nil
}

func (f CardLookup) Candidates(string) ([]memory.Memory, error) { return nil, nil }
func (f CardLookup) Memories(string) ([]memory.Memory, error)   { return nil, nil }

// engramMemories maps the observations of a project the way the Engram adapter maps them, with the edges of each
// by observation id.
func engramMemories(all []memory.Observation, edges map[int64][]memory.Edge) []memory.Memory {
	var memories []memory.Memory
	for _, o := range all {
		m := obsMemory(o)
		status, successor := statusOfObservation(o, edges[o.ID], all)
		m.Status = status
		if successor != nil {
			m.Successor = memory.Identity{Source: engramSource, ID: strconv.FormatInt(successor.ID, 10)}
			m.SuccessorTitle = successor.Title
		}
		memories = append(memories, m)
	}
	return memories
}
