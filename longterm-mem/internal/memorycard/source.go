package memorycard

import (
	"strconv"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// SourceName is the name memorycard's memories carry in an identity.
const SourceName = "memorycard"

// engramSource is the name of the Engram store in the import_map table, which records, per memory, what it was
// imported from. It is the source of the identities a memory's lineage names.
const engramSource = "engram"

// Source serves memorycard's memories as memory.Memory, the port of promotion (promote.Source). It maps through
// Store's one mapping (memorySelect and scanMemory), so a memory reads the same through either, and it is as
// read-only as the Store.
type Source struct {
	store *Store
}

// NewSource returns the Source over store. It reads memorycard alone: the lineage and the mirrors of a memory come
// from its import_map rows and the content hashes of memorycard's own memories, so it opens no other store.
func NewSource(store *Store) *Source { return &Source{store: store} }

// Candidates returns the memories of project that promotion reads, in id order: the active and the superseded
// ones (Store.ListMemories).
func (s *Source) Candidates(project string) ([]memory.Memory, error) {
	cards, err := s.store.ListMemories(project)
	if err != nil {
		return nil, err
	}
	return s.mapAll(cards)
}

// Memories returns every memory of project whatever its status, in id order (Store.ListAllMemories): what
// propagation reads.
func (s *Source) Memories(project string) ([]memory.Memory, error) {
	cards, err := s.store.ListAllMemories(project)
	if err != nil {
		return nil, err
	}
	return s.mapAll(cards)
}

// ByID returns the memory whose id is id, whatever its status. ok is false, with a nil error, when there is no
// such memory; an id that is not a number names none.
func (s *Source) ByID(id string) (memory.Memory, bool, error) {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return memory.Memory{}, false, nil
	}
	card, ok, err := s.store.MemoryByID(n)
	if err != nil || !ok {
		return memory.Memory{}, false, err
	}
	m, err := s.toMemory(card)
	return m, err == nil, err
}

func (s *Source) mapAll(cards []memory.CardMemory) ([]memory.Memory, error) {
	var memories []memory.Memory
	for _, card := range cards {
		m, err := s.toMemory(card)
		if err != nil {
			return nil, err
		}
		memories = append(memories, m)
	}
	return memories, nil
}

// toMemory is the one mapping of a card to a memory. The lineage is the Engram observation the import_map row
// names and, for a memory with no import_map row of any source, the twins the composition root finds.
func (s *Source) toMemory(card memory.CardMemory) (memory.Memory, error) {
	m := memory.Memory{
		Identity: memory.Identity{Source: SourceName, ID: strconv.FormatInt(card.ID, 10)},
		Project:  card.Project, Type: card.Type, Title: card.Title, Body: card.Body, TopicKey: card.TopicKey,
		Pinned: card.Pinned, Status: card.Status, SuccessorTitle: card.SupersededByTitle, Revision: card.Revision,
	}
	if card.SupersededBy != 0 {
		m.Successor = memory.Identity{Source: SourceName, ID: strconv.FormatInt(card.SupersededBy, 10)}
	}
	if card.EngramID != 0 {
		lineage, err := s.lineageOf(card)
		if err != nil {
			return memory.Memory{}, err
		}
		m.Lineage = lineage
	}
	if !card.Mapped {
		mirrors, err := s.mirrorsOf(card)
		if err != nil {
			return memory.Memory{}, err
		}
		m.Mirrored = mirrors
	}
	return m, nil
}

// lineageOf is the lineage of a memory imported from Engram observation card.EngramID, read from memorycard's
// import_map alone: the observation, then the other memories imported from it (its parts, in part order), so
// that a page keyed by one part's memory is still the page of the others.
func (s *Source) lineageOf(card memory.CardMemory) ([]memory.Identity, error) {
	lineage := []memory.Identity{{Source: engramSource, ID: strconv.FormatInt(card.EngramID, 10)}}
	parts, err := s.store.MemoryIDsByEngramIDs([]int64{card.EngramID})
	if err != nil {
		return nil, err
	}
	for _, id := range parts[card.EngramID] {
		if id != card.ID {
			lineage = append(lineage, memory.Identity{Source: SourceName, ID: strconv.FormatInt(id, 10)})
		}
	}
	return lineage, nil
}

// mirrorsOf is what a memory with no import_map row holds the content of: the memories of its project with the same
// content hash that have one, each followed by the observation its row names. Memorycard says it alone: it needs
// no other store to know which memories were imported.
func (s *Source) mirrorsOf(card memory.CardMemory) ([]memory.Identity, error) {
	twins, err := s.store.MappedTwins(card.Project, card.ContentHash)
	if err != nil {
		return nil, err
	}
	var mirrors []memory.Identity
	for _, twin := range twins {
		mirrors = append(mirrors, memory.Identity{Source: SourceName, ID: strconv.FormatInt(twin.MemoryID, 10)})
		if twin.EngramID != 0 {
			mirrors = append(mirrors, memory.Identity{Source: engramSource, ID: strconv.FormatInt(twin.EngramID, 10)})
		}
	}
	return mirrors, nil
}
