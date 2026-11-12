package promote

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// This file is the seam between the ports promotion read its stores through before it read one model (an Engram
// observation, a memorycard memory) and the Source port it reads now: the walk, the decision and the write are over
// memory.Memory for every store, and what still serves an observation or a card memory is viewed as a Source here.
// It goes away when the callers hand promotion a Source.

// CardMemories is the port SyncCards lists a project's memories of memorycard through.
type CardMemories interface {
	ListMemories(project string) ([]memory.CardMemory, error)
	// ListAllMemories lists every memory of the project whatever its status (active, superseded, archived): the
	// ones PropagateCards reads.
	ListAllMemories(project string) ([]memory.CardMemory, error)
}

// CardLookup resolves one memorycard memory by id.
type CardLookup func(id int64) (memory.CardMemory, bool, error)

// errNoLookup is what the view of a port with no lookup by id answers to ByID.
var errNoLookup = errors.New("promote: this view of a store looks no memory up by id")

// memoryOfObservation is an Engram observation as the Engram adapter maps it, without the status and the lineage the
// adapter finds in its database: an active memory with no kin.
func memoryOfObservation(o memory.Observation) memory.Memory {
	return memory.Memory{
		Identity: memory.Identity{Source: engramSource, ID: strconv.FormatInt(o.ID, 10)},
		Project:  o.Project, Type: o.Type, Title: o.Title, Body: o.Content, TopicKey: o.TopicKey,
		Pinned: o.Pinned, Status: memory.StatusActive, Revision: o.RevisionCount, SyncID: o.SyncID,
	}
}

// memoryOfCard is a memorycard memory as the memorycard adapter maps it: the observation it was imported from is its
// lineage, and the observations that hold its content, when it has no import row, its mirrors.
func memoryOfCard(c memory.CardMemory) memory.Memory {
	m := memory.Memory{
		Identity: memory.Identity{Source: memorycardSource, ID: strconv.FormatInt(c.ID, 10)},
		Project:  c.Project, Type: c.Type, Title: c.Title, Body: c.Body, TopicKey: c.TopicKey,
		Pinned: c.Pinned, Status: c.Status, SuccessorTitle: c.SupersededByTitle, Revision: c.Revision,
	}
	if c.SupersededBy != 0 {
		m.Successor = memory.Identity{Source: memorycardSource, ID: strconv.FormatInt(c.SupersededBy, 10)}
	}
	if c.EngramID != 0 {
		m.Lineage = append(m.Lineage, memory.Identity{Source: engramSource, ID: strconv.FormatInt(c.EngramID, 10)})
	}
	if !c.Mapped {
		for _, id := range c.SameContentEngramIDs {
			m.Mirrored = append(m.Mirrored, memory.Identity{Source: engramSource, ID: strconv.FormatInt(id, 10)})
		}
	}
	return m
}

// engramView serves the observations of a Memory as the memories of a Source: the live ones are the candidates, and
// the status of every observation is what R-033 makes of its soft delete and its supersedes edges.
type engramView struct{ m Memory }

func (v engramView) ByID(string) (memory.Memory, bool, error) {
	return memory.Memory{}, false, errNoLookup
}

func (v engramView) Candidates(project string) ([]memory.Memory, error) {
	observations, err := v.m.ListObservations(project)
	if err != nil {
		return nil, err
	}
	var memories []memory.Memory
	for _, o := range observations {
		memories = append(memories, memoryOfObservation(o))
	}
	return memories, nil
}

func (v engramView) Memories(project string) ([]memory.Memory, error) {
	observations, err := v.m.ObservationsIncludingDeleted(project)
	if err != nil {
		return nil, err
	}
	var memories []memory.Memory
	for _, o := range observations {
		m := memoryOfObservation(o)
		edges, err := v.m.RelatedEdges(o.ID)
		if err != nil {
			m.Status, m.StatusCause = memory.StatusUnknown, err
			memories = append(memories, m)
			continue
		}
		status, successor := statusOfObservation(o, edges, observations)
		m.Status = status
		if successor != nil {
			m.Successor = memory.Identity{Source: engramSource, ID: strconv.FormatInt(successor.ID, 10)}
			m.SuccessorTitle = successor.Title
		}
		memories = append(memories, m)
	}
	return memories, nil
}

// statusOfObservation is the rule of R-033 over the observations of one project: the older side of a supersedes
// edge is superseded by the newer, a soft-deleted observation is archived.
func statusOfObservation(o memory.Observation, edges []memory.Edge, all []memory.Observation) (string, *memory.Observation) {
	for _, edge := range edges {
		if edge.Relation != "supersedes" {
			continue
		}
		other := edge.SourceSyncID
		if o.SyncID != "" && other == o.SyncID {
			other = edge.TargetSyncID
		}
		for i := range all {
			if all[i].SyncID == other && all[i].CreatedAt > o.CreatedAt {
				return memory.StatusSuperseded, &all[i]
			}
		}
	}
	if o.DeletedAt != "" {
		return memory.StatusArchived, nil
	}
	return memory.StatusActive, nil
}

// cardsView serves the memories of a CardMemories as the memories of a Source.
type cardsView struct{ c CardMemories }

func (v cardsView) ByID(string) (memory.Memory, bool, error) {
	return memory.Memory{}, false, errNoLookup
}

func (v cardsView) Candidates(project string) ([]memory.Memory, error) {
	return v.list(v.c.ListMemories, project)
}

func (v cardsView) Memories(project string) ([]memory.Memory, error) {
	return v.list(v.c.ListAllMemories, project)
}

func (v cardsView) list(read func(string) ([]memory.CardMemory, error), project string) ([]memory.Memory, error) {
	cards, err := read(project)
	if err != nil {
		return nil, err
	}
	var memories []memory.Memory
	for _, c := range cards {
		memories = append(memories, memoryOfCard(c))
	}
	return memories, nil
}

// ExplicitPromoteCard promotes the memorycard memory named by id (Writer.Promote). Like ExplicitPromote it is
// explicit: nothing about the memory has to make it eligible. It differs in what it says: a memory, not an
// observation, is looked up, and the promotion of a memory whose page is already at its revision answers
// ActionAlreadyPromoted and writes nothing (pageStyle.rewriteCurrent).
func ExplicitPromoteCard(ctx context.Context, w *Writer, lookup CardLookup, id int64) (Result, error) {
	c, ok, err := lookup(id)
	if err != nil {
		return Result{}, fmt.Errorf("promote: look up memorycard memory %d: %w", id, err)
	}
	if !ok {
		return Result{}, fmt.Errorf("%w: memorycard memory %d", ErrObservationNotFound, id)
	}
	return w.Promote(ctx, memoryOfCard(c), true)
}
