package engram

import (
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// Standings returns the standing of each of ids that has one. Observations
// with nothing to report are absent from the map rather than present and
// empty, so a caller can range over findings alone.
//
// Only verdicts that qualify a memory are reported. "related",
// "compatible", "scoped" and "not_conflict" are verdicts that everything is
// FINE -- 120 of 169 relations on the real database -- and rendering those
// as warnings would mark almost every memory, which is the same as marking
// none.
func (s *Store) Standings(ids []int64) (map[int64]memory.Standing, error) {
	if len(ids) == 0 {
		return map[int64]memory.Standing{}, nil
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

	out := map[int64]memory.Standing{}
	add := func(id int64, mutate func(*memory.Standing)) {
		if !wanted[id] {
			return
		}
		st := out[id]
		mutate(&st)
		out[id] = st
	}

	for rows.Next() {
		var relation, status string
		var src, tgt memory.Neighbour
		if err := rows.Scan(&relation, &status, &src.ID, &src.Title, &tgt.ID, &tgt.Title); err != nil {
			return nil, fmt.Errorf("engram: scan relation standing row: %w", err)
		}

		switch {
		case status == "pending":
			add(src.ID, func(s *memory.Standing) { s.Unjudged = append(s.Unjudged, tgt) })
			add(tgt.ID, func(s *memory.Standing) { s.Unjudged = append(s.Unjudged, src) })
		case status != "judged":
			// orphaned, or anything a later Engram adds: not a verdict.
		case relation == "supersedes":
			add(tgt.ID, func(s *memory.Standing) { s.SupersededBy = append(s.SupersededBy, src) })
		case relation == "conflicts_with":
			add(src.ID, func(s *memory.Standing) { s.ConflictsWith = append(s.ConflictsWith, tgt) })
			add(tgt.ID, func(s *memory.Standing) { s.ConflictsWith = append(s.ConflictsWith, src) })
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("engram: iterate relation standing rows: %w", err)
	}
	return out, nil
}
