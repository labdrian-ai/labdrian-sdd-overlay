package engram

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// SourceName is the name Engram's observations carry in an identity.
const SourceName = "engram"

// LineageFunc answers, for the observations of one project, the memories other stores have of the piece of memory each
// one is: the memorycard memories imported from it (Kin.Imported), and the ones mirrored by hand with its content
// (Kin.Mirrored). It is asked once for a listing, keyed by observation id, with no entry for an observation that has
// none. The composition root supplies it, so this adapter names no other adapter.
type LineageFunc func(project string, observations []memory.Observation) (map[int64]memory.Kin, error)

// Source serves Engram's observations as memory.Memory, the port of promotion (promote.Source). It is as
// read-only as the Store, and it holds the rule that turns an observation's soft delete and its supersedes edges
// into a status and a successor, because Engram's rows have no status of their own (R-033 in
// openspec/specs/longterm-mem-promotion/spec.md).
type Source struct {
	store   *Store
	lineage LineageFunc
}

// NewSource returns the Source over store. lineage may be nil, and then no memory has a lineage.
func NewSource(store *Store, lineage LineageFunc) *Source {
	return &Source{store: store, lineage: lineage}
}

// siblings finds an observation of the same project by its sync id, which is how an edge names one.
type siblings func(syncID string) (memory.Observation, bool, error)

// Candidates returns the live observations of project (R-020: not soft-deleted), each mapped as ByID maps it. It lists
// the live rows alone, as the promotion pass always did, and reads what the status of each needs (the project's edges,
// and the rows the edges name) only as far as the mapping asks, so a store whose relations cannot be read still lists
// its candidates: the memories whose status could not be resolved say so (memory.StatusUnknown).
func (s *Source) Candidates(project string) ([]memory.Memory, error) {
	live, err := s.store.ListObservations(project)
	if err != nil {
		return nil, err
	}
	return s.mapAll(project, live, nil)
}

// Memories returns every observation of project, soft-deleted ones included, each mapped as ByID maps it.
func (s *Source) Memories(project string) ([]memory.Memory, error) {
	every, err := s.store.ObservationsIncludingDeleted(project)
	if err != nil {
		return nil, err
	}
	return s.mapAll(project, every, every)
}

// Probe asks the store for the relations of an observation id no row can carry, so that the query matches nothing and
// costs nothing: what is under test is whether the statement can be prepared and run at all (promote.Prober).
func (s *Source) Probe(string) error {
	_, err := s.store.RelatedEdges(0)
	return err
}

// ByID returns the observation whose id is id, a soft-deleted one included. ok is false, with a nil error, when
// there is no such observation; an id that is not a number names none.
func (s *Source) ByID(id string) (memory.Memory, bool, error) {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return memory.Memory{}, false, nil
	}
	o, ok, err := s.store.ObservationByID(n)
	if err != nil || !ok {
		return memory.Memory{}, false, err
	}
	// What became of the observation (its status) is not what a lookup by id is for: when its relations cannot be
	// read the memory comes back with an unknown status, and only what reads the status reports the cause.
	unresolved := map[int64]error{}
	edges, edgesErr := s.store.RelatedEdges(o.ID)
	if edgesErr != nil {
		unresolved[o.ID] = edgesErr
	}
	find := func(syncID string) (memory.Observation, bool, error) {
		return s.store.observationBySyncID(o.Project, syncID)
	}
	memories, err := s.build(o.Project, []memory.Observation{o}, map[int64][]memory.Edge{o.ID: edges}, unresolved, find)
	if err != nil {
		return memory.Memory{}, false, err
	}
	return memories[0], true, nil
}

// mapAll maps observations, the rows of project that a listing serves. The edges of the project are read once; when
// they cannot be read, each observation's own are asked for, so that what cannot be read is a failure of that
// observation's status (memory.StatusUnknown, memory.Memory.StatusCause) and not of the listing. The sync-id index of the supersedes rule is
// built from every, or from one read of every row of the project the first time an edge needs it when every is nil.
func (s *Source) mapAll(project string, observations, every []memory.Observation) ([]memory.Memory, error) {
	var bySyncID map[string]memory.Observation
	var everyErr error
	find := func(syncID string) (memory.Observation, bool, error) {
		if bySyncID == nil && everyErr == nil {
			rows := every
			if rows == nil {
				rows, everyErr = s.store.ObservationsIncludingDeleted(project)
			}
			bySyncID = make(map[string]memory.Observation, len(rows))
			for _, o := range rows {
				bySyncID[o.SyncID] = o
			}
		}
		if everyErr != nil {
			return memory.Observation{}, false, everyErr
		}
		other, ok := bySyncID[syncID]
		return other, ok, nil
	}
	unresolved := map[int64]error{}
	edges, err := s.store.RelatedEdgesOfProject(project)
	if err != nil {
		edges = map[int64][]memory.Edge{}
		for _, o := range observations {
			own, ownErr := s.store.RelatedEdges(o.ID)
			if ownErr != nil {
				unresolved[o.ID] = ownErr
				continue
			}
			edges[o.ID] = own
		}
	}
	return s.build(project, observations, edges, unresolved, find)
}

// build is the one mapping of observations to memories, given the edges of each (by observation id) and the
// observations whose edges could not be read: such a memory is kept, with an unknown status and the cause. Any other
// failure of the mapping is the call's own.
func (s *Source) build(project string, observations []memory.Observation, edges map[int64][]memory.Edge, unresolved map[int64]error, find siblings) ([]memory.Memory, error) {
	var lineage map[int64]memory.Kin
	if s.lineage != nil {
		var err error
		if lineage, err = s.lineage(project, observations); err != nil {
			return nil, fmt.Errorf("engram: find the lineage of the observations of project %q: %w", project, err)
		}
	}
	var memories []memory.Memory
	for _, o := range observations {
		var m memory.Memory
		if cause := unresolved[o.ID]; cause != nil {
			m = observationMemory(o)
			m.Status, m.StatusCause = memory.StatusUnknown, cause
		} else {
			var err error
			if m, err = toMemory(o, edges[o.ID], find); err != nil {
				return nil, err
			}
		}
		m.Lineage, m.Mirrored = lineage[o.ID].Imported, lineage[o.ID].Mirrored
		memories = append(memories, m)
	}
	return memories, nil
}

// observationMemory is what an observation says of itself, with no status: the fields both mappings of it share.
func observationMemory(o memory.Observation) memory.Memory {
	return memory.Memory{
		Identity: memory.Identity{Source: SourceName, ID: strconv.FormatInt(o.ID, 10)},
		Project:  o.Project, Type: o.Type, Title: o.Title, Body: o.Content, TopicKey: o.TopicKey,
		Pinned: o.Pinned, Revision: o.RevisionCount, SyncID: o.SyncID,
	}
}

// toMemory maps an observation to a memory, without its lineage.
func toMemory(o memory.Observation, edges []memory.Edge, find siblings) (memory.Memory, error) {
	status, successor, err := statusOf(o, edges, find)
	if err != nil {
		return memory.Memory{}, err
	}
	m := observationMemory(o)
	m.Status = status
	if successor != nil {
		m.Successor = memory.Identity{Source: SourceName, ID: strconv.FormatInt(successor.ID, 10)}
		m.SuccessorTitle = successor.Title
	}
	return m, nil
}

// statusOf is the rule of R-033 (scenario 3 and decision D11): the older side of an accepted supersedes edge is
// superseded by the other side, the newer by created_at, whether or not it was soft-deleted; otherwise a
// soft-deleted observation is archived and any other is active. The newer side of an edge is the survivor and is
// left alone, and an edge whose other side is not in the project is not recognized, so it decides nothing.
func statusOf(o memory.Observation, edges []memory.Edge, find siblings) (string, *memory.Observation, error) {
	for _, edge := range edges {
		if edge.Relation != "supersedes" {
			continue
		}
		otherSyncID := edge.SourceSyncID
		if o.SyncID != "" && otherSyncID == o.SyncID {
			otherSyncID = edge.TargetSyncID
		}
		other, ok, err := find(otherSyncID)
		if err != nil {
			return "", nil, err
		}
		if !ok || other.CreatedAt <= o.CreatedAt {
			continue
		}
		return memory.StatusSuperseded, &other, nil
	}
	if o.DeletedAt != "" {
		return memory.StatusArchived, nil, nil
	}
	return memory.StatusActive, nil, nil
}

// observationBySyncID returns the observation of project whose sync id is syncID, soft-deleted or not.
func (s *Store) observationBySyncID(project, syncID string) (memory.Observation, bool, error) {
	row := s.db.QueryRow(`SELECT `+observationColumns+` FROM observations WHERE project = ? AND sync_id = ?`, project, syncID)
	o, err := scanObservationRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return memory.Observation{}, false, nil
		}
		return memory.Observation{}, false, fmt.Errorf("engram: look up observation %q of project %q: %w", syncID, project, err)
	}
	return o, true, nil
}
