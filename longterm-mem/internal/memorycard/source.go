package memorycard

import (
	"fmt"
	"strconv"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// SourceName is the name memorycard's memories carry in an identity.
const SourceName = "memorycard"

// engramSource is the name of the Engram store in the import_map table, which records, per memory, what it was
// imported from. It is the source of the identities a memory's lineage names.
const engramSource = "engram"

// TwinsFunc answers the identities of the memories of other stores that hold a memory's content: the same
// project and the same content hash. The composition root supplies it (the stores it knows are its own to
// join), so this adapter names no other adapter.
type TwinsFunc func(project, contentHash string) ([]memory.Identity, error)

// Source serves memorycard's memories as memory.Memory, the port of promotion (promote.Source). It maps through
// Store's one mapping (memorySelect and scanMemory), so a memory reads the same through either, and it is as
// read-only as the Store.
type Source struct {
	store *Store
	twins TwinsFunc
}

// NewSource returns the Source over store. twins may be nil, and then a memory's lineage holds the Engram
// observation its import_map row names and nothing else.
func NewSource(store *Store, twins TwinsFunc) *Source { return &Source{store: store, twins: twins} }

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
		m.Lineage = append(m.Lineage, memory.Identity{Source: engramSource, ID: strconv.FormatInt(card.EngramID, 10)})
	}
	if !card.Mapped && s.twins != nil {
		twins, err := s.twins(card.Project, card.ContentHash)
		if err != nil {
			return memory.Memory{}, fmt.Errorf("memorycard: find the twins of memory %d: %w", card.ID, err)
		}
		m.Mirrored = twins
	}
	return m, nil
}
