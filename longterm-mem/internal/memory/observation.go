// Package memory is the domain model of the mid-term memory longterm-mem reads: the observation
// and the shapes a search and a relation ledger answer with, and the two rules that belong to the
// model rather than to any store, how a query is split into the words searched and how a body is
// cut into a snippet a person can read.
//
// It holds no store. Engram's SQLite database is one adapter that maps its rows to these types
// (internal/engram); the consumers that read memory (promote, query, staleness, skillstale and the
// MCP server) depend on these types and on the reader ports they each declare, never on the adapter.
// The package imports the standard library's text packages and nothing else: no database, no
// file system, no clock.
package memory

// Observation is one mid-term memory row.
//
// CreatedAt, UpdatedAt and DeletedAt are carried as the raw text the store holds (an empty DeletedAt
// means the observation is live) rather than parsed into time.Time: there is no cross-row or
// cross-format comparison this model needs to perform, and every row of one database is written with
// the same convention, so the text sorts within a column. CreatedAt drives the newer-by-created_at
// successor rule of propagation; DeletedAt lets propagation tell a soft-deleted observation from an
// active one. TopicKey is the curated-topic eligibility signal of promotion, so every observation a
// reader returns must carry it.
type Observation struct {
	ID            int64
	SyncID        string
	Type          string
	Title         string
	Content       string
	Project       string
	RevisionCount int
	Pinned        bool
	CreatedAt     string
	UpdatedAt     string
	DeletedAt     string
	TopicKey      string
}
