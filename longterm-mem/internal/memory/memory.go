package memory

// Identity names one memory of one store: the store's name and the id that store gives it. The ids of two
// stores are unrelated numbers, so an id means nothing without its source. The id is text because a page of
// the vault records it as text and a store may key its memories by something other than an integer.
type Identity struct {
	Source string
	ID     string
}

// The statuses a Memory can have. A store that keeps no status of its own maps what it knows to these.
const (
	StatusActive     = "active"
	StatusSuperseded = "superseded"
	StatusArchived   = "archived"
)

// Memory is one memory of any store, as promotion reads it: what a page of the vault is built from and what its
// propagation compares. A store is an adapter that maps its own rows to it; nothing here names a store.
type Memory struct {
	Identity Identity
	Project  string
	Type     string
	Title    string
	Body     string
	// TopicKey is empty when the memory has none.
	TopicKey string
	Pinned   bool
	// Status is StatusActive, StatusSuperseded or StatusArchived.
	Status string
	// Successor is the identity of the memory that replaced this one, and SuccessorTitle its title, for the link
	// a page renders. Both are zero when nothing replaced it.
	Successor      Identity
	SuccessorTitle string
	// Revision is the number a page records to tell whether it is behind its memory.
	Revision int
	// Lineage is the identities the same piece of memory has in other stores: the Engram observation a
	// memorycard memory was imported from, the memorycard memory imported from an observation, or the
	// observations that hold the content of a memory that was mirrored by hand. A page that carries one of them
	// is the page of this memory.
	Lineage []Identity
}
