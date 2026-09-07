package engram

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Row is one FTS5 search match, returned in Engram's own bm25 rank order
// (best match first) rather than insertion order (R-006).
type Row struct {
	ID      int64
	Title   string
	Content string
	Project string
	// Snippet is an extract of Content centred on the first match, cut to
	// SnippetBudget characters. It exists so a caller can show why a row
	// matched without shipping Content, whose measured p90 on the live
	// corpus is 33,991 bytes.
	Snippet string
	// SnippetTruncated reports that Snippet is a fragment of Content, not
	// the whole of it.
	SnippetTruncated bool
	// ContentLength is len(Content) in bytes: how much a truncated
	// Snippet is not showing.
	ContentLength int
	// MatchOffset is Content's byte index of the first FTS match, the same
	// position Snippet was centred on. It lets a caller re-render the
	// snippet at a different budget (query's budget-before-render
	// allocation) without re-running the search. A row with no match
	// position to report is 0, the same head-of-content fallback extract
	// already uses.
	MatchOffset int
}

// SnippetBudget is the number of characters an extract may carry.
//
// It is derived, not chosen. The response ceiling this module enforces is
// ~2,000 tokens, and a default query returns at most DefaultTopN rows from
// each of two sources; at ~4 bytes per token that leaves each Engram row
// roughly 480 characters if the ceiling is to bind only on unusual shapes
// rather than on every ordinary call. 480 characters is also about three
// to four lines of the structured markdown these bodies are written in --
// enough to carry the sentence a match sits in, which the vault's own
// 200-character cap frequently is not for this content.
const SnippetBudget = 480

// truncationMark is what a cut edge looks like to a person reading the
// text. It is not decoration and it is not optional: a preview a reader
// cannot tell from a whole memory is worse than no preview, because a
// decision then gets made on a fragment that looked complete. The
// machine-readable half of the same statement is SnippetTruncated and
// ContentLength -- a marker in prose is not something a program can act
// on, and a boolean is not something a person reads.
const truncationMark = "\u2026"

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
func (s *Store) Search(project, query string, limit int, excludeTypes ...string) (SearchResult, error) {
	tokens, dropped := SearchTokens(query)
	if len(tokens) == 0 {
		return SearchResult{MatchMode: MatchAll}, nil
	}
	result := SearchResult{MatchMode: MatchAll, DroppedTokens: dropped}

	rows, err := s.searchMatching(project, joinTokens(tokens, " AND "), limit, excludeTypes)
	if err != nil {
		return SearchResult{}, err
	}
	if len(rows) > 0 || len(tokens) == 1 {
		// One token: "any" and "all" are the same query, so retrying
		// would repeat the work and misreport an unwidened search.
		result.Rows = rows
		return result, nil
	}

	rows, err = s.searchMatching(project, joinTokens(tokens, " OR "), limit, excludeTypes)
	if err != nil {
		return SearchResult{}, err
	}
	result.Rows = rows
	result.MatchMode = MatchAny
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
// offset. The window around that offset is then a character budget this
// package controls, and it behaves identically whatever tokenizer the
// index was built with.
func (s *Store) searchMatching(project, match string, limit int, excludeTypes []string) ([]Row, error) {
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

	var results []Row
	for rows.Next() {
		var r Row
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
	matchOpen  = "\ue000"
	matchClose = "\ue001"
)

// extract returns a SnippetBudget-sized window of content centred on the
// first match highlight() marked, whether that window is a fragment, and
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
	snippet, truncated := SnippetAt(content, offset, SnippetBudget)
	return snippet, truncated, offset
}

// SnippetAt returns a budget-sized window of content centred on offset (a
// byte index into content), and whether that window is a fragment of the
// whole.
//
// It is exported so query's response assembly can re-render a row's
// snippet at a different budget once the byte ceiling is allocated across
// rows (design: "the snippet budget is allocated per ROW, not per
// source"), without re-running the search that produced content. offset is
// the match's position in content; a row with no lexical match position --
// the embedding arm has none, a cosine match is not a location in text --
// passes 0, which renders an honest head slice rather than inventing a
// position the retrieval method cannot support.
func SnippetAt(content string, offset, budget int) (string, bool) {
	if len(content) <= budget {
		return content, false
	}

	// Centre the window on offset, then pull it back inside the body at
	// both ends.
	start := offset - budget/2
	if start < 0 {
		start = 0
	}
	if start+budget > len(content) {
		start = len(content) - budget
	}
	end := start + budget

	// Never cut a rune in half: a snippet is text a person reads, and a
	// severed multi-byte character renders as a replacement glyph.
	for start > 0 && !utf8.RuneStart(content[start]) {
		start--
	}
	for end < len(content) && !utf8.RuneStart(content[end]) {
		end++
	}

	snippet := content[start:end]
	if start > 0 {
		snippet = truncationMark + snippet
	}
	if end < len(content) {
		snippet += truncationMark
	}
	return snippet, true
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
