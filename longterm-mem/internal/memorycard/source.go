package memorycard

import (
	"slices"
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
	memories, err := s.mapAll([]memory.CardMemory{card})
	if err != nil {
		return memory.Memory{}, false, err
	}
	return memories[0], true, nil
}

// kin is what a listing reads to say the lineage and the mirrors of all its memories: the memories imported from each
// observation the listing names or a mirror holds, and the observations each hand mirror holds the content of.
type kin struct {
	parts   map[int64][]int64
	holders map[int64][]int64
}

// kinOf reads what the lineage and the mirrors of cards need. For a memory with no import_map row it asks first, by
// one indexed lookup, which observations have an imported memory with the stored content hash of its text (what a
// hand mirror of one part, or of a record kept whole, finds), and only when that finds nothing reads the imported
// bodies of its project, once per listing, to compare them by ContentKey. Then it reads the parts of every observation
// it has named, in one lookup in chunks (MemoryIDsByEngramIDs).
func (s *Source) kinOf(cards []memory.CardMemory) (kin, error) {
	k := kin{holders: map[int64][]int64{}}
	wanted := map[int64]bool{}
	byKey := map[string]map[string][]int64{}
	for _, card := range cards {
		if card.EngramID != 0 {
			wanted[card.EngramID] = true
		}
		if card.Mapped {
			continue
		}
		holders, err := s.store.ObservationsWithStoredHash(card.Project, memory.CardContentHash(card.Title, card.Body))
		if err != nil {
			return kin{}, err
		}
		if len(holders) == 0 {
			if byKey[card.Project] == nil {
				if byKey[card.Project], err = s.store.ObservationsByKey(card.Project); err != nil {
					return kin{}, err
				}
			}
			holders = byKey[card.Project][ContentKey(card.Title, card.Body)]
		}
		k.holders[card.ID] = holders
		for _, id := range holders {
			wanted[id] = true
		}
	}
	observations := make([]int64, 0, len(wanted))
	for id := range wanted {
		observations = append(observations, id)
	}
	slices.Sort(observations)
	parts, err := s.store.MemoryIDsByEngramIDs(observations)
	if err != nil {
		return kin{}, err
	}
	k.parts = parts
	return k, nil
}

func (s *Source) mapAll(cards []memory.CardMemory) ([]memory.Memory, error) {
	k, err := s.kinOf(cards)
	if err != nil {
		return nil, err
	}
	var memories []memory.Memory
	for _, card := range cards {
		memories = append(memories, toMemory(card, k))
	}
	return memories, nil
}

// toMemory is the one mapping of a card to a memory. The lineage is the Engram observation the import_map row
// names and the other memories imported from it; a memory with no import_map row of any source has as mirrors the
// imported memories that hold its content (mirrorsOf).
func toMemory(card memory.CardMemory, k kin) memory.Memory {
	m := memory.Memory{
		Identity: memory.Identity{Source: SourceName, ID: strconv.FormatInt(card.ID, 10)},
		Project:  card.Project, Type: card.Type, Title: card.Title, Body: card.Body, TopicKey: card.TopicKey,
		Pinned: card.Pinned, Status: card.Status, SuccessorTitle: card.SupersededByTitle, Revision: card.Revision,
	}
	if card.SupersededBy != 0 {
		m.Successor = memory.Identity{Source: SourceName, ID: strconv.FormatInt(card.SupersededBy, 10)}
	}
	if card.EngramID != 0 {
		m.Lineage = lineageOf(card, k)
	}
	if !card.Mapped {
		m.Mirrored = mirrorsOf(card, k)
	}
	return m
}

// lineageOf is the lineage of a memory imported from Engram observation card.EngramID, read from memorycard's
// import_map alone: the observation, then the other memories imported from it (its parts, in part order), so
// that a page keyed by one part's memory is still the page of the others.
func lineageOf(card memory.CardMemory, k kin) []memory.Identity {
	lineage := []memory.Identity{{Source: engramSource, ID: strconv.FormatInt(card.EngramID, 10)}}
	for _, id := range k.parts[card.EngramID] {
		if id != card.ID {
			lineage = append(lineage, memory.Identity{Source: SourceName, ID: strconv.FormatInt(id, 10)})
		}
	}
	return lineage
}

// mirrorsOf is what a memory with no import_map row holds the content of: the imported observations that kinOf found
// to hold its text (the stored hash of an imported part, else the ContentKey of the whole observation or of a part),
// in observation id order, each as every memory of its parts, in part order, and the observation. Memorycard says it
// alone: it needs no other store to know which observations were imported.
func mirrorsOf(card memory.CardMemory, k kin) []memory.Identity {
	var mirrors []memory.Identity
	for _, observation := range k.holders[card.ID] {
		for _, id := range k.parts[observation] {
			mirrors = append(mirrors, memory.Identity{Source: SourceName, ID: strconv.FormatInt(id, 10)})
		}
		mirrors = append(mirrors, memory.Identity{Source: engramSource, ID: strconv.FormatInt(observation, 10)})
	}
	return mirrors
}
