package memory

// CardMemory is one memory of memorycard, the store that replaces Engram, as promotion reads it. It is the
// counterpart of Observation for the second source: promotion keeps the two apart because a page of the vault
// names its source (engram_id or memorycard_id), and the ids of the two stores are unrelated numbers.
type CardMemory struct {
	ID      int64
	Project string
	Type    string
	Title   string
	Body    string
	// TopicKey is empty when the memory has none.
	TopicKey string
	Pinned   bool
	// Status is memorycard's own: active, superseded or archived.
	Status string
	// SupersededBy is the id of the memory that replaced this one, or 0. It is the only relation promotion
	// reads from memorycard, and SupersededByTitle is that memory's title, for the link a page renders.
	SupersededBy      int64
	SupersededByTitle string
	// Revision is the memory's next version number: the highest version memorycard kept for it, plus one.
	Revision int
	// EngramID is the id of the Engram observation this memory was imported from (the import_map row of source
	// engram), or 0 when it was not imported from one: a memory that is bridged to an observation.
	EngramID int64
}
