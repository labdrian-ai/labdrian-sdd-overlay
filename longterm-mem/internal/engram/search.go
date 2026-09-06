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

// MatchMode reports how strictly a search's tokens were combined.
const (
	// MatchAll required every token: the precise reading of the query.
	MatchAll = "all"
	// MatchAny required only one: the widened reading, used only when
	// MatchAll found nothing at all.
	MatchAny = "any"
)

// SearchResult is what Search found and how it had to ask.
//
// The two are inseparable on purpose. Rows alone cannot distinguish a
// precise hit from a deliberately broadened one, and cannot show that
// some of the caller's own words were never searched -- both of which
// change how much the results are worth trusting.
type SearchResult struct {
	// Rows are the matches, in Engram's own bm25 order.
	Rows []Row
	// MatchMode is MatchAll or MatchAny.
	MatchMode string
	// DroppedTokens are the caller's tokens removed as stopwords, in the
	// order they were written. Empty when nothing was dropped.
	DroppedTokens []string
}

// stopwords are the high-frequency English function words stripped from a
// query before its tokens are combined.
//
// They are stripped for one measured reason: AND-joining them is what
// turns a question into silence. They are not stripped to save work, and
// stripping alone does not fix the problem -- on the live corpus the
// eight-token question "what conventions apply when editing the register
// writer" returned 0 rows AND-joined, and still returned 0 rows AND-joined
// after stripping. What stripping buys is a better precise attempt and a
// less diluted widened one: fewer terms that appear in nearly every
// document, so the query that survives is made of the words that carry the
// question's meaning.
var stopwords = map[string]bool{
	"a": true, "about": true, "all": true, "an": true, "and": true,
	"any": true, "are": true, "as": true, "at": true, "be": true,
	"been": true, "but": true, "by": true, "can": true, "did": true,
	"do": true, "does": true, "for": true, "from": true, "had": true,
	"has": true, "have": true, "how": true, "i": true, "if": true,
	"in": true, "into": true, "is": true, "it": true, "its": true,
	"me": true, "my": true, "no": true, "not": true, "of": true,
	"on": true, "or": true, "our": true, "should": true, "so": true,
	"some": true, "than": true, "that": true, "the": true, "their": true,
	"them": true, "then": true, "there": true, "these": true, "they": true,
	"this": true, "to": true, "up": true, "was": true, "we": true,
	"were": true, "what": true, "when": true, "where": true, "which": true,
	"who": true, "why": true, "will": true, "with": true, "would": true,
	"you": true, "your": true,
}

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
func (s *Store) Search(project, query string, limit int) (SearchResult, error) {
	tokens, dropped := searchTokens(query)
	if len(tokens) == 0 {
		return SearchResult{MatchMode: MatchAll}, nil
	}
	result := SearchResult{MatchMode: MatchAll, DroppedTokens: dropped}

	rows, err := s.searchMatching(project, joinTokens(tokens, " AND "), limit)
	if err != nil {
		return SearchResult{}, err
	}
	if len(rows) > 0 || len(tokens) == 1 {
		// One token: "any" and "all" are the same query, so retrying
		// would repeat the work and misreport an unwidened search.
		result.Rows = rows
		return result, nil
	}

	rows, err = s.searchMatching(project, joinTokens(tokens, " OR "), limit)
	if err != nil {
		return SearchResult{}, err
	}
	result.Rows = rows
	result.MatchMode = MatchAny
	return result, nil
}

// searchMatching runs one FTS5 MATCH expression.
func (s *Store) searchMatching(project, match string, limit int) ([]Row, error) {
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

// searchTokens splits query into the tokens actually searched, and the
// stopwords removed from it.
//
// A query made entirely of stopwords keeps every one of them: there is
// nothing else the caller can have meant by it, and answering a
// deliberate query with an empty one is the failure this whole change
// exists to remove.
func searchTokens(query string) (tokens, dropped []string) {
	fields := strings.Fields(query)
	if len(fields) == 0 {
		return nil, nil
	}
	for _, f := range fields {
		if stopwords[strings.ToLower(f)] {
			dropped = append(dropped, f)
			continue
		}
		tokens = append(tokens, f)
	}
	if len(tokens) == 0 {
		return fields, nil
	}
	return tokens, dropped
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
