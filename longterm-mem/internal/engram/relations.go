package engram

import (
	"fmt"
	"strings"
)

// Edge is one accepted relation edge between two observations, identified
// by their Engram sync_ids: memory_relations keys source_id/target_id on
// sync_id, not the integer id (live schema #3129).
type Edge struct {
	Relation     string
	SourceSyncID string
	TargetSyncID string
}

// acceptedRelations are the relation kinds RelatedEdges surfaces (D7); a
// relation still "pending", or "not_conflict"/"orphaned"/"ignored", never
// reaches a promoted page's related field.
var acceptedRelations = []string{"related", "compatible", "scoped", "supersedes", "conflicts_with"}

// RelatedEdges returns every accepted relation edge touching the
// observation identified by observationID: judgment_status = 'judged',
// superseded_at IS NULL, and relation in the accepted set, on either side
// of the edge (D7, live schema #3129).
func (s *Store) RelatedEdges(observationID int64) ([]Edge, error) {
	// The placeholder list and the argument slice are both derived from
	// acceptedRelations, so the declaration above stays the single source
	// of truth for the accepted set's size and contents.
	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(acceptedRelations)), ", ")
	args := make([]any, 0, len(acceptedRelations)+1)
	args = append(args, observationID)
	for _, relation := range acceptedRelations {
		args = append(args, relation)
	}

	// A half-dangling row (an endpoint left NULL by an orphaned or
	// partially written record) cannot be rendered as a link; skip it in
	// SQL instead of aborting the whole edge set on a NULL scan.
	rows, err := s.db.Query(
		`SELECT r.relation, r.source_id, r.target_id
		 FROM memory_relations r, observations o
		 WHERE o.id = ?
		   AND (r.source_id = o.sync_id OR r.target_id = o.sync_id)
		   AND r.source_id IS NOT NULL
		   AND r.target_id IS NOT NULL
		   AND r.judgment_status = 'judged'
		   AND r.superseded_at IS NULL
		   AND r.relation IN (`+placeholders+`)`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("engram: related edges for observation %d: %w", observationID, err)
	}
	defer rows.Close()

	var edges []Edge
	for rows.Next() {
		var e Edge
		if err := rows.Scan(&e.Relation, &e.SourceSyncID, &e.TargetSyncID); err != nil {
			return nil, fmt.Errorf("engram: scan relation edge row: %w", err)
		}
		edges = append(edges, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("engram: iterate relation edge rows: %w", err)
	}
	return edges, nil
}
