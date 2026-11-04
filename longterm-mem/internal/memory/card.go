package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

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
	// ContentHash is the hash memorycard stored for the memory (CardContentHash of its title and body).
	ContentHash string
	// Mapped reports that the memory has an import_map row of any source: it came from an import, and the
	// duplicate rule below does not apply to it.
	Mapped bool
	// SameContentEngramIDs are the Engram observations of the memory's project whose CardContentHash equals
	// ContentHash. The caller fills it, for a memory with no import_map row only: a memory that was mirrored
	// to memorycard by hand has no bridge, and this is how its twin is found. Promotion refuses the memory when
	// one of them already has a page.
	SameContentEngramIDs []int64
}

// CardContentHash is the content hash memorycard computes for a memory (internal/memory/save.go of memorycard):
// the lowercased text of the title, a newline and the body, with every run of white space collapsed to one
// space, as sha256 in hex. The same function over an Engram observation's title and content says whether
// memorycard would hold that observation as the same memory.
func CardContentHash(title, body string) string {
	norm := strings.ToLower(strings.Join(strings.Fields(title+"\n"+body), " "))
	sum := sha256.Sum256([]byte(norm))
	return hex.EncodeToString(sum[:])
}
