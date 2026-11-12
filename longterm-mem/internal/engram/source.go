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

// LineageFunc answers the identities other stores give the piece of memory an observation is: the memorycard
// memory imported from it. The composition root supplies it, so this adapter names no other adapter.
type LineageFunc func(observationID int64) ([]memory.Identity, error)

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

// Candidates returns the live observations of project (R-020: not soft-deleted), each mapped as ByID maps it.
func (s *Source) Candidates(project string) ([]memory.Memory, error) {
	live, err := s.store.ListObservations(project)
	if err != nil {
		return nil, err
	}
	return s.mapAll(project, live)
}

// Memories returns every observation of project, soft-deleted ones included, each mapped as ByID maps it.
func (s *Source) Memories(project string) ([]memory.Memory, error) {
	every, err := s.store.ObservationsIncludingDeleted(project)
	if err != nil {
		return nil, err
	}
	return s.mapAll(project, every)
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
	m, err := s.toMemory(o, func(syncID string) (memory.Observation, bool, error) {
		return s.store.observationBySyncID(o.Project, syncID)
	})
	return m, err == nil, err
}

func (s *Source) mapAll(project string, observations []memory.Observation) ([]memory.Memory, error) {
	every, err := s.store.ObservationsIncludingDeleted(project)
	if err != nil {
		return nil, err
	}
	bySyncID := make(map[string]memory.Observation, len(every))
	for _, o := range every {
		bySyncID[o.SyncID] = o
	}
	var memories []memory.Memory
	for _, o := range observations {
		m, err := s.toMemory(o, func(syncID string) (memory.Observation, bool, error) {
			other, ok := bySyncID[syncID]
			return other, ok, nil
		})
		if err != nil {
			return nil, err
		}
		memories = append(memories, m)
	}
	return memories, nil
}

// toMemory is the one mapping of an observation to a memory.
func (s *Source) toMemory(o memory.Observation, find siblings) (memory.Memory, error) {
	status, successor, err := s.statusOf(o, find)
	if err != nil {
		return memory.Memory{}, err
	}
	m := memory.Memory{
		Identity: memory.Identity{Source: SourceName, ID: strconv.FormatInt(o.ID, 10)},
		Project:  o.Project, Type: o.Type, Title: o.Title, Body: o.Content, TopicKey: o.TopicKey,
		Pinned: o.Pinned, Status: status, Revision: o.RevisionCount,
	}
	if successor != nil {
		m.Successor = memory.Identity{Source: SourceName, ID: strconv.FormatInt(successor.ID, 10)}
		m.SuccessorTitle = successor.Title
	}
	if s.lineage != nil {
		if m.Lineage, err = s.lineage(o.ID); err != nil {
			return memory.Memory{}, fmt.Errorf("engram: find the lineage of observation %d: %w", o.ID, err)
		}
	}
	return m, nil
}

// statusOf is the rule of R-033 (scenario 3 and decision D11): the older side of an accepted supersedes edge is
// superseded by the other side, the newer by created_at, whether or not it was soft-deleted; otherwise a
// soft-deleted observation is archived and any other is active. The newer side of an edge is the survivor and is
// left alone, and an edge whose other side is not in the project is not recognized, so it decides nothing.
func (s *Source) statusOf(o memory.Observation, find siblings) (string, *memory.Observation, error) {
	edges, err := s.store.RelatedEdges(o.ID)
	if err != nil {
		return "", nil, err
	}
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
