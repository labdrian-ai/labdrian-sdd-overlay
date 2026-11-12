package engram

import (
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// acceptedRelations are the relation kinds RelatedEdges surfaces (D7); a
// relation still "pending", or "not_conflict"/"orphaned"/"ignored", never
// reaches a promoted page's related field.
var acceptedRelations = []string{"related", "compatible", "scoped", "supersedes", "conflicts_with"}

// RelatedEdges returns every accepted relation edge touching the
// observation identified by observationID: judgment_status = 'judged',
// superseded_at IS NULL, and relation in the accepted set, on either side
// of the edge (D7, live schema #3129).
func (s *Store) RelatedEdges(observationID int64) ([]memory.Edge, error) {
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

	var edges []memory.Edge
	for rows.Next() {
		var e memory.Edge
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

// RelatedEdgesOfProject returns, in one query, the edges RelatedEdges returns for each observation of project,
// keyed by observation id: the same filter, on either side of the edge, in the order the relation rows were
// written. An observation with no accepted edge has no entry.
func (s *Store) RelatedEdgesOfProject(project string) (map[int64][]memory.Edge, error) {
	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(acceptedRelations)), ", ")
	args := []any{project}
	for _, relation := range acceptedRelations {
		args = append(args, relation)
	}
	rows, err := s.db.Query(
		`SELECT o.id, r.relation, r.source_id, r.target_id
		 FROM observations o JOIN memory_relations r ON (r.source_id = o.sync_id OR r.target_id = o.sync_id)
		 WHERE o.project = ?
		   AND r.source_id IS NOT NULL
		   AND r.target_id IS NOT NULL
		   AND r.judgment_status = 'judged'
		   AND r.superseded_at IS NULL
		   AND r.relation IN (`+placeholders+`)
		 ORDER BY o.id, r.rowid`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("engram: related edges for project %q: %w", project, err)
	}
	defer rows.Close()

	edges := map[int64][]memory.Edge{}
	for rows.Next() {
		var id int64
		var e memory.Edge
		if err := rows.Scan(&id, &e.Relation, &e.SourceSyncID, &e.TargetSyncID); err != nil {
			return nil, fmt.Errorf("engram: scan relation edge row: %w", err)
		}
		edges[id] = append(edges[id], e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("engram: iterate relation edge rows: %w", err)
	}
	return edges, nil
}
