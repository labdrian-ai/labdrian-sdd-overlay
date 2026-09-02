package engram

import (
	"fmt"
	"strings"
)

// Row is one FTS5 search match, returned in Engram's own bm25 rank order
// (best match first) rather than insertion order (R-006).
type Row struct {
	ID      int64
	Title   string
	Content string
	Project string
}

// Search runs an FTS5 search scoped to project, excluding soft-deleted rows
// (R-020), in Engram's own bm25 order, limited to limit rows.
func (s *Store) Search(project, query string, limit int) ([]Row, error) {
	match := ftsMatchQuery(query)
	if match == "" {
		return nil, nil
	}

	rows, err := s.db.Query(
		`SELECT o.id, o.title, o.content, o.project
		 FROM observations_fts
		 JOIN observations o ON o.id = observations_fts.rowid
		 WHERE observations_fts MATCH ? AND o.project = ? AND o.deleted_at IS NULL
		 ORDER BY observations_fts.rank
		 LIMIT ?`,
		match, project, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("engram: search observations for project %q: %w", project, err)
	}
	defer rows.Close()

	var results []Row
	for rows.Next() {
		var r Row
		if err := rows.Scan(&r.ID, &r.Title, &r.Content, &r.Project); err != nil {
			return nil, fmt.Errorf("engram: scan search row: %w", err)
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("engram: iterate search rows: %w", err)
	}
	return results, nil
}

// ftsMatchQuery double-quotes each whitespace-separated token (doubling any
// internal quote) and AND-joins them, so a token starting with "-" (FTS5's
// NOT operator) is always literal text, not query syntax.
func ftsMatchQuery(query string) string {
	tokens := strings.Fields(query)
	if len(tokens) == 0 {
		return ""
	}
	quoted := make([]string, len(tokens))
	for i, t := range tokens {
		quoted[i] = `"` + strings.ReplaceAll(t, `"`, `""`) + `"`
	}
	return strings.Join(quoted, " AND ")
}
