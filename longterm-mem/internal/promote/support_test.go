package promote

import (
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// This file is the one place the tests build what promotion reads from the rows of a store: an Engram observation
// (obsMemory), a memorycard memory (cardMem) and the Engram store itself (engramSourceOf). The tests build the
// rows of each store as the store does, and promotion reads them through these, so a change in what promotion
// reads is a change here and not in every test that builds a row.

// obsMemory is an Engram observation as promotion reads it.
func obsMemory(o memory.Observation) memory.Observation { return o }

// cardMem is a memorycard memory as promotion reads it.
func cardMem(c memory.CardMemory) memory.CardMemory { return c }

// engramSourceOf is the Engram store as promotion reads its memories.
func engramSourceOf(store *engram.Store) *engram.Store { return store }
