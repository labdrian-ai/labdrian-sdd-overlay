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
	// StatusUnknown is the status of a memory whose store could not say what became of it (its relations could not
	// be read): it is neither active nor retired, no reader may take it for either, and StatusCause says why.
	StatusUnknown = "unknown"
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
	// Status is StatusActive, StatusSuperseded, StatusArchived or StatusUnknown. A reader that compares it handles
	// StatusUnknown by name: it is never an active memory, and nothing is promoted over it as retired or patched
	// from it.
	Status string
	// Successor is the identity of the memory that replaced this one, and SuccessorTitle its title, for the link
	// a page renders. Both are zero when nothing replaced it.
	Successor      Identity
	SuccessorTitle string
	// Revision is the number a page records to tell whether it is behind its memory.
	Revision int
	// Lineage is the identities the same piece of memory has in other stores, as an import record names them: the
	// Engram observation a memorycard memory was imported from, the memorycard memory imported from an observation.
	// A page that carries one of them is the page of this memory.
	Lineage []Identity
	// Mirrored is the identities that hold the content of this memory in other stores with no import record naming
	// them: a memory mirrored by hand. A page that carries one of them is the page of this memory too, and is told
	// from a lineage one because a refusal names which it was.
	Mirrored []Identity
	// StatusCause is why the status is StatusUnknown, and is nil for any other status.
	StatusCause error
	// SyncID is transitional: the sync id of an Engram observation, which the pages Engram observations produce
	// still record (engram_sync_id). It is dropped with that key, when pages stop carrying store-specific keys
	// (T6 of the store-agnostic promotion), and no other store sets it.
	SyncID string
}

// Kin is what the stores other than a memory's own say of it: the memories that are the same piece of memory.
// Imported are the ones an import record names, Mirrored the ones that only hold the same content.
type Kin struct {
	Imported []Identity
	Mirrored []Identity
}

// LineageError is the failure to read what another store says of a memory (its Kin). A source that lists memories
// wraps it into the error of the listing, so that whoever reports the failure can tell a store that could not be
// asked for the lineage from the store of the memories that could not be listed.
type LineageError struct {
	// Store names the store that could not be read.
	Store string
	Err   error
}

func (e *LineageError) Error() string {
	return e.Store + ": read the lineage of the memories: " + e.Err.Error()
}

func (e *LineageError) Unwrap() error { return e.Err }
