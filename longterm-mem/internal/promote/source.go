package promote

import "github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"

// Source is the port through which promotion reads one store's memories, whichever store it is: each store is an
// adapter that maps its own rows to memory.Memory (the Engram adapter in internal/engram, the memorycard adapter
// in internal/memorycard), wired by the composition root. It is the one port of the store-agnostic model; the
// ports above it (Memory, CardMemories, CardLookup) are the two-model ones it replaces.
type Source interface {
	// Candidates returns the memories of project that promotion may write a page for: the live ones, in the
	// store's order.
	Candidates(project string) ([]memory.Memory, error)
	// Memories returns every memory of project whatever its status, retired ones included: what propagation
	// reads, since a page can be behind its memory in either direction.
	Memories(project string) ([]memory.Memory, error)
	// ByID returns the memory whose id, in the store's own terms, is id, whatever its status. ok is false, with
	// a nil error, when there is no such memory.
	ByID(id string) (memory.Memory, bool, error)
}

// Prober is what a Source says when it can tell whether its store still answers what became of its memories without
// listing any: the question a sync asks after a failure, to tell a store that went away from a step that failed. A
// project with no memory has none to carry the failure, so only a probe can tell.
type Prober interface {
	// Probe returns nil when the store answers, and why it does not otherwise. It costs nothing: it matches no row.
	Probe(project string) error
}
