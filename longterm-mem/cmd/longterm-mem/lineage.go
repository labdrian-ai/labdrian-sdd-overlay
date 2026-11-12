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

// newEngramSource is the Engram adapter whose observations name the memorycard memories that are the same piece of
// memory: the memory of every part its import_map rows name, and the memories mirrored by hand that hold its
// content (no import_map row, the same project and content hash), the twin rule of newMemorycardSource in
// reverse. Both are read once for a listing.
// cards may be nil: with no memorycard store there is no lineage to compose, and no observation has one.
func newEngramSource(engramStore *engram.Store, cards *memorycard.Store) *engram.Source {
	if cards == nil {
		return engram.NewSource(engramStore, nil)
	}
	return engram.NewSource(engramStore, func(project string, observations []memory.Observation) (map[int64]memory.Kin, error) {
		ids := make([]int64, 0, len(observations))
		hashes := make([]string, 0, len(observations))
		for _, o := range observations {
			ids = append(ids, o.ID)
			hashes = append(hashes, memory.CardContentHash(o.Title, o.Content))
		}
		// A failure to read memorycard is memorycard's, not Engram's: the listing of Engram's observations says so.
		imported, err := cards.MemoryIDsByEngramIDs(ids)
		if err != nil {
			return nil, &memory.LineageError{Store: memorycard.SourceName, Err: err}
		}
		mirrored, err := cards.UnmappedMemoryIDsByHash(project, hashes)
		if err != nil {
			return nil, &memory.LineageError{Store: memorycard.SourceName, Err: err}
		}
		lineage := map[int64]memory.Kin{}
		for i, o := range observations {
			var kin memory.Kin
			for _, id := range imported[o.ID] {
				kin.Imported = append(kin.Imported, memory.Identity{Source: memorycard.SourceName, ID: strconv.FormatInt(id, 10)})
			}
			for _, id := range mirrored[hashes[i]] {
				kin.Mirrored = append(kin.Mirrored, memory.Identity{Source: memorycard.SourceName, ID: strconv.FormatInt(id, 10)})
			}
			lineage[o.ID] = kin
		}
		return lineage, nil
	})
}

// newMemorycardSource is the memorycard adapter whose memories, besides the observation their import_map row
// names, name the observations that hold their content when they have no row (the memories mirrored by hand) as
// their mirrors.
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
