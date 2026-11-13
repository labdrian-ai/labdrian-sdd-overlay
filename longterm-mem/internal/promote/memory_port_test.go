package promote

import (
	"strconv"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// fakeMemory is the Memory port promote owns, implemented without a database: promotion and
// propagation depend on the memory model and on that port, not on the store that serves them.
type fakeMemory struct {
	live       []memory.Observation
	all        []memory.Observation
	edges      map[int64][]memory.Edge
	listErr    error
	historyErr error
	edgesErr   error

	listed  []string
	history []string
}

// Candidates serves the live observations, as the Engram adapter does.
func (f *fakeMemory) Candidates(project string) ([]memory.Memory, error) {
	f.listed = append(f.listed, project)
	if f.listErr != nil {
		return nil, f.listErr
	}
	return asMemorycard(obsMemories(f.live)), nil
}

// Memories serves every observation, soft-deleted ones included, with the status the edges and the deletions give
// them. Edges that cannot be read are the failure of each memory's status, as the adapter reports it.
func (f *fakeMemory) Memories(project string) ([]memory.Memory, error) {
	f.history = append(f.history, project)
	if f.historyErr != nil {
		return nil, f.historyErr
	}
	memories := engramMemories(f.all, f.edges)
	if f.edgesErr != nil {
		for i := range memories {
			memories[i].Status, memories[i].StatusCause = memory.StatusUnknown, f.edgesErr
		}
	}
	return asMemorycard(memories), nil
}

// ByID serves the observation with the id, from every observation or else from the live ones.
func (f *fakeMemory) ByID(id string) (memory.Memory, bool, error) {
	for _, o := range append(append([]memory.Observation{}, f.all...), f.live...) {
		if strconv.FormatInt(o.ID, 10) == id {
			return obsMemory(o), true, nil
		}
	}
	return memory.Memory{}, false, nil
}
