package promote

import "github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"

// Lister is the port through which promotion reads a project's candidates: its live (not
// soft-deleted) observations (R-020).
type Lister interface {
	ListObservations(project string) ([]memory.Observation, error)
}

// HistoryReader is the port through which propagation reads what happened to what was promoted:
// every observation of the project, soft-deleted ones included (R-033), and the accepted relation
// edges touching one of them (D7).
type HistoryReader interface {
	ObservationsIncludingDeleted(project string) ([]memory.Observation, error)
	RelatedEdges(observationID int64) ([]memory.Edge, error)
}

// Memory is what Deps needs of the memory it promotes from: the candidates to promote and the history
// to propagate. The package owns these ports and the model they speak; the store that serves them
// (the Engram adapter in production, a fake in a unit test) is wired by the caller.
type Memory interface {
	Lister
	HistoryReader
}
