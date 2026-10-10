package engram

import (
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// Search runs an FTS5 search scoped to project, excluding soft-deleted rows
// (R-020), in Engram's own bm25 order, limited to limit rows.
//
// It asks twice at most. The first attempt requires every token; only if
// that finds nothing does it retry requiring any one of them. Ordering the
// two this way keeps the precision that AND genuinely earns where it
// works -- on the live corpus "canonical identity" returns 13 focused rows
// AND-joined against 114 OR-joined -- while removing the case where AND
// returns nothing and the caller is told, in effect, that the memory does
// not exist. That case is the expensive one precisely because it looks
// free: an empty result costs no tokens, so no audit of what a query
// spends can ever find it.
//
// The words searched, and the stopwords dropped, are the model's own
// (memory.SearchTokens); this method only turns them into a MATCH expression.
func (s *Store) Search(project, query string, limit int, excludeTypes ...string) (memory.SearchResult, error) {
	tokens, dropped := memory.SearchTokens(query)
	if len(tokens) == 0 {
		return memory.SearchResult{MatchMode: memory.MatchAll}, nil
	}
	result := memory.SearchResult{MatchMode: memory.MatchAll, DroppedTokens: dropped}

	rows, err := s.searchMatching(project, joinTokens(tokens, " AND "), limit, excludeTypes)
	if err != nil {
		return memory.SearchResult{}, err
	}
	if len(rows) > 0 || len(tokens) == 1 {
		// One token: "any" and "all" are the same query, so retrying
		// would repeat the work and misreport an unwidened search.
		result.Rows = rows
		return result, nil
	}

	rows, err = s.searchMatching(project, joinTokens(tokens, " OR "), limit, excludeTypes)
	if err != nil {
		return memory.SearchResult{}, err
	}
	result.Rows = rows
	result.MatchMode = memory.MatchAny
	return result, nil
}

// searchMatching runs one FTS5 MATCH expression.
//
// It selects highlight() beside content rather than FTS5's snippet(), and
// the reason is measured. snippet() is the obvious choice: it returns an
// extract centred on the match and needs no schema change. But its window
// is counted in tokens, SQLite clamps that count to 64, and under the live
// trigram tokenizer a token is one three-character window of the document.
// The result, measured on five live rows at both 64 and 300 requested
// tokens, is a 69-70 byte extract -- too short to carry the sentence the
// match is in, and no more informative than the blind head slice it was
// meant to replace.
//
// highlight() marks every match in place and returns the whole column,
// which costs nothing here (the content is already being read, in
// process, and never leaves it) and gives an exact, tokenizer-accurate
// offset. The window around that offset is then a character budget the
// model controls (memory.SnippetAt), and it behaves identically whatever
// tokenizer the index was built with.
func (s *Store) searchMatching(project, match string, limit int, excludeTypes []string) ([]memory.Row, error) {
	// The exclusion is a filter, applied in SQL beside the existing
	// project and soft-delete filters. It changes which rows are
	// eligible, never their order: the surviving rows come back in the
	// same bm25 sequence they would have without it (R-006).
	args := []any{matchOpen, matchClose, match, project}
	exclusion := ""
	if len(excludeTypes) > 0 {
		exclusion = " AND o.type NOT IN (?" + strings.Repeat(", ?", len(excludeTypes)-1) + ")"
		for _, t := range excludeTypes {
			args = append(args, t)
		}
	}
	args = append(args, limit)

	rows, err := s.db.Query(
		`SELECT o.id, o.title, o.content, o.project,
		        highlight(observations_fts, 1, ?, ?)
		 FROM observations_fts
		 JOIN observations o ON o.id = observations_fts.rowid
		 WHERE observations_fts MATCH ? AND o.project = ? AND o.deleted_at IS NULL`+exclusion+`
		 ORDER BY observations_fts.rank
		 LIMIT ?`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("engram: search observations for project %q: %w", project, err)
	}
	defer rows.Close()

	var results []memory.Row
	for rows.Next() {
		var r memory.Row
		var highlighted string
		if err := rows.Scan(&r.ID, &r.Title, &r.Content, &r.Project, &highlighted); err != nil {
			return nil, fmt.Errorf("engram: scan search row: %w", err)
		}
		r.ContentLength = len(r.Content)
		r.Snippet, r.SnippetTruncated, r.MatchOffset = extract(r.Content, highlighted)
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("engram: iterate search rows: %w", err)
	}
	return results, nil
}

// matchOpen and matchClose bracket each match inside highlight()'s
// output. They are Unicode private-use characters because no text ever
// means them: nothing an observation can be written to say places one, so
// finding one is finding a match boundary. A body that somehow contained
// them anyway would shift the extract's centre and nothing else -- there
// is no parsing here to confuse, only a first offset to look for.
const (
	matchOpen  = ""
	matchClose = ""
)

// extract returns a memory.SnippetBudget-sized window of content centred on
// the first match highlight() marked, whether that window is a fragment, and
// the match's byte offset in content.
//
// highlighted is content with markers inserted, so the marker's index in
// it is the match's index in content: everything before the marker is
// unmodified. A body with no marker (highlight matched in another column,
// say the title) falls back to the head of the content -- the honest
// answer when there is no match position to centre on -- and reports
// offset 0.
func extract(content, highlighted string) (string, bool, int) {
	offset := 0
	if i := strings.Index(highlighted, matchOpen); i >= 0 {
		offset = i
	}
	snippet, truncated := memory.SnippetAt(content, offset, memory.SnippetBudget)
	return snippet, truncated, offset
}

// joinTokens double-quotes each token (doubling any internal quote) and
// joins them with op, so a token starting with "-" (FTS5's NOT operator)
// is always literal text, not query syntax.
func joinTokens(tokens []string, op string) string {
	quoted := make([]string, len(tokens))
	for i, t := range tokens {
		quoted[i] = `"` + strings.ReplaceAll(t, `"`, `""`) + `"`
	}
	return strings.Join(quoted, op)
}
