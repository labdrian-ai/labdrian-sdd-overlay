package main

import (
	"strconv"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memorycard"
)

// The lineage of a memory, the identities the same piece of memory has in the other store, is composed here: this
// is the one place that knows both stores, so neither adapter imports the other and promotion imports neither.
// memorycard's import_map is read in both directions, and Engram answers which observations hold a memory's
// content.

// newEngramSource is the Engram adapter whose observations name the memorycard memory imported from them.
func newEngramSource(engramStore *engram.Store, cards *memorycard.Store) *engram.Source {
	return engram.NewSource(engramStore, func(observationID int64) ([]memory.Identity, error) {
		imported, ok, err := cards.MemoryByEngramID(observationID)
		if err != nil || !ok {
			return nil, err
		}
		return []memory.Identity{{Source: memorycard.SourceName, ID: strconv.FormatInt(imported.ID, 10)}}, nil
	})
}

// newMemorycardSource is the memorycard adapter whose memories, besides the observation their import_map row
// names, name the observations that hold their content when they have no row: the memories mirrored by hand.
func newMemorycardSource(cards *memorycard.Store, engramStore *engram.Store) *memorycard.Source {
	return memorycard.NewSource(cards, func(project, contentHash string) ([]memory.Identity, error) {
		ids, err := engramStore.ObservationIDsWithCardHash(project, contentHash)
		if err != nil {
			return nil, err
		}
		var twins []memory.Identity
		for _, id := range ids {
			twins = append(twins, memory.Identity{Source: engram.SourceName, ID: strconv.FormatInt(id, 10)})
		}
		return twins, nil
	})
}
