package query

import (
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// Neighbour is the observation on the far side of a relation, as a result row names it.
type Neighbour struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

// Standing is what the memory's relation ledger says about the observation behind a row, in the shape the query
// command and the MCP query tool carry it: its JSON names are the wire format a client reads. The domain's own
// memory.Standing carries no wire format; this package owns the one the surfaces send, and standingOf writes
// the first in the second.
type Standing struct {
	// SupersededBy names the observations that replaced this one.
	SupersededBy []Neighbour `json:"superseded_by,omitempty"`
	// ConflictsWith names observations judged to contradict this one.
	ConflictsWith []Neighbour `json:"conflicts_with,omitempty"`
	// Unjudged names observations flagged against this one and never decided about.
	Unjudged []Neighbour `json:"unjudged,omitempty"`
}

// standingOf is the wire standing for what the memory said. The types differ, so it shares nothing with the domain's value, and a list
// the memory left unsaid stays unsaid (nil, so the wire omits it).
func standingOf(in memory.Standing) Standing {
	return Standing{
		SupersededBy:  neighboursOf(in.SupersededBy),
		ConflictsWith: neighboursOf(in.ConflictsWith),
		Unjudged:      neighboursOf(in.Unjudged),
	}
}

func neighboursOf(in []memory.Neighbour) []Neighbour {
	if in == nil {
		return nil
	}
	out := make([]Neighbour, len(in))
	for i, n := range in {
		out[i] = Neighbour{ID: n.ID, Title: n.Title}
	}
	return out
}

// attachStandings annotates each row that has an observation behind it.
//
// A relation ledger that cannot be read degrades to a diagnostic rather
// than failing the query, exactly as a failing vault does: the results are
// still the results, and losing the annotation is a smaller harm than
// losing the answer. It is reported rather than swallowed, because silence
// here is indistinguishable from "nothing is superseded" -- the reading
// that lets an abandoned decision pass as current.
func attachStandings(store StandingReader, rows []ResultRow) []Diagnostic {
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		if r.EngramID != 0 {
			ids = append(ids, r.EngramID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	standings, err := store.Standings(ids)
	if err != nil {
		return []Diagnostic{{
			Code:   DiagnosticRelationsUnreadable,
			Detail: fmt.Sprintf("engram's relation ledger could not be read, so no result is marked as superseded or contradicted even if it is: %v", err),
		}}
	}

	for i := range rows {
		if st, ok := standings[rows[i].EngramID]; ok && !st.Empty() {
			standing := standingOf(st)
			rows[i].Standing = &standing
		}
	}
	return nil
}
