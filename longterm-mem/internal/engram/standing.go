package engram

import (
	"fmt"
	"strings"
)

// Neighbour is the observation on the far side of a relation.
type Neighbour struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

// Standing is what Engram's relation ledger says about one observation, in
// the only terms a reader cares about: may I treat this as current?
//
// It exists because the ledger is otherwise unread. Engram records every
// verdict in memory_relations and its own search never joins that table --
// verified on a copy of a real database: inserting "B supersedes A" left
// the search results for A byte-identical, still ranked first, unmarked. So
// a memory that was explicitly replaced comes back looking exactly like one
// that was not, and the reader reintroduces what was abandoned. longterm-mem
// cannot fix Engram's search (its connection is read-only, R-002), but it
// can refuse to repeat the omission in its OWN answers.
type Standing struct {
	// SupersededBy names the observations that replaced this one. Direction
	// is the whole of it: being the TARGET of a supersedes means something
	// replaced you, being the SOURCE means you are the replacement.
	SupersededBy []Neighbour `json:"superseded_by,omitempty"`
	// ConflictsWith names observations judged to contradict this one.
	ConflictsWith []Neighbour `json:"conflicts_with,omitempty"`
	// Unjudged names observations Engram flagged against this one and
	// nobody ever decided about. On the real database this was the largest
	// class by far, and every one of them was invisible to every reader: an
	// undecided conflict is not the same as no conflict.
	Unjudged []Neighbour `json:"unjudged,omitempty"`
}

// Empty reports whether there is nothing worth telling a reader.
func (s Standing) Empty() bool {
	return len(s.SupersededBy) == 0 && len(s.ConflictsWith) == 0 && len(s.Unjudged) == 0
}

// Standings returns the standing of each of ids that has one. Observations
// with nothing to report are absent from the map rather than present and
// empty, so a caller can range over findings alone.
//
// Only verdicts that qualify a memory are reported. "related",
// "compatible", "scoped" and "not_conflict" are verdicts that everything is
// FINE -- 120 of 169 relations on the real database -- and rendering those
// as warnings would mark almost every memory, which is the same as marking
// none.
func (s *Store) Standings(ids []int64) (map[int64]Standing, error) {
	if len(ids) == 0 {
		return map[int64]Standing{}, nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(ids)), ", ")
	args := make([]any, 0, len(ids)*2)
	for range 2 {
		for _, id := range ids {
			args = append(args, id)
		}
	}

	// Both endpoints are joined so each side can be named by title, and a
	// half-dangling row (an endpoint left NULL by an orphaned record) drops
	// out of the join rather than aborting the scan.
	//
	// superseded_at IS NULL: a relation that was itself re-judged carries
	// it, and reporting such a verdict would state a conclusion its own
	// author has already withdrawn.
	rows, err := s.db.Query(
		`SELECT r.relation, r.judgment_status, src.id, src.title, tgt.id, tgt.title
		 FROM memory_relations r
		 JOIN observations src ON src.sync_id = r.source_id
		 JOIN observations tgt ON tgt.sync_id = r.target_id
		 WHERE r.superseded_at IS NULL
		   AND src.deleted_at IS NULL
		   AND tgt.deleted_at IS NULL
		   AND (src.id IN (`+placeholders+`) OR tgt.id IN (`+placeholders+`))`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("engram: reading relation standings: %w", err)
	}
	defer rows.Close()

	wanted := make(map[int64]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}

	out := map[int64]Standing{}
	add := func(id int64, mutate func(*Standing)) {
		if !wanted[id] {
			return
		}
		st := out[id]
		mutate(&st)
		out[id] = st
	}

	for rows.Next() {
		var relation, status string
		var src, tgt Neighbour
		if err := rows.Scan(&relation, &status, &src.ID, &src.Title, &tgt.ID, &tgt.Title); err != nil {
			return nil, fmt.Errorf("engram: scan relation standing row: %w", err)
		}

		switch {
		case status == "pending":
			add(src.ID, func(s *Standing) { s.Unjudged = append(s.Unjudged, tgt) })
			add(tgt.ID, func(s *Standing) { s.Unjudged = append(s.Unjudged, src) })
		case status != "judged":
			// orphaned, or anything a later Engram adds: not a verdict.
		case relation == "supersedes":
			add(tgt.ID, func(s *Standing) { s.SupersededBy = append(s.SupersededBy, src) })
		case relation == "conflicts_with":
			add(src.ID, func(s *Standing) { s.ConflictsWith = append(s.ConflictsWith, tgt) })
			add(tgt.ID, func(s *Standing) { s.ConflictsWith = append(s.ConflictsWith, src) })
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("engram: iterate relation standing rows: %w", err)
	}
	return out, nil
}
