package promote

import "github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"

// Source is the port through which promotion reads one store's memories, whichever store it is: each store is an
// adapter that maps its own rows to memory.Memory (today the memorycard adapter in internal/memorycard), wired by the
// composition root. It is the one port of the
// store-agnostic model.
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
