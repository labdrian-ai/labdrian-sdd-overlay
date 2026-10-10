package query

import "github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"

// Searcher is the port through which the lexical source reads memory: a search scoped to project that
// excludes soft-deleted rows (R-020), in the store's own rank order, limited to limit rows, leaving out
// the observation types named in excludeTypes. It answers how it had to ask as well as what it found
// (memory.SearchResult): a widened search and the words it never searched change how far the rows can
// be trusted.
type Searcher interface {
	Search(project, query string, limit int, excludeTypes ...string) (memory.SearchResult, error)
}

// StandingReader is the port through which a result is annotated with what the relation ledger says
// about it: the standing of each of ids that has one. An observation with nothing to report may be
// absent from the map or present and empty; the caller reports neither.
type StandingReader interface {
	Standings(ids []int64) (map[int64]memory.Standing, error)
}

// CoverageReader is the port through which the embedding arm counts what exists: from one consistent
// read, how many live observations project has and which of indexedIDs resolve to live rows in it.
// Taking the two numbers apart is what let them disagree, so they are one question here.
type CoverageReader interface {
	CoverageSnapshot(project string, indexedIDs []int64) (live int, liveByID map[int64]memory.Observation, err error)
}

// DegradationReporter is the port through which a query learns that the store is being read from a
// point-in-time snapshot instead of live, and why. It matters most where the connection outlives the
// call (the MCP server opens it once for a whole session): a frozen corpus whose results look complete.
type DegradationReporter interface {
	Degraded() (degraded bool, cause string)
}

// Memory is what Deps needs of the memory it queries. The package owns these ports and the model they
// speak; the store that serves them (the Engram adapter in production, a fake in a unit test) is wired
// by the caller.
type Memory interface {
	Searcher
	StandingReader
	CoverageReader
	DegradationReporter
}
