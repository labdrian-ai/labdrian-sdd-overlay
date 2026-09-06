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
func (s *Store) searchMatching(project, match string, limit int) ([]Row, error) {
	rows, err := s.db.Query(
		`SELECT o.id, o.title, o.content, o.project,
		        highlight(observations_fts, 1, ?, ?)
		 FROM observations_fts
		 JOIN observations o ON o.id = observations_fts.rowid
		 WHERE observations_fts MATCH ? AND o.project = ? AND o.deleted_at IS NULL
		 ORDER BY observations_fts.rank
		 LIMIT ?`,
		matchOpen, matchClose, match, project, limit,
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
		r.Snippet, r.SnippetTruncated = extract(r.Content, highlighted)
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
// first match highlight() marked, and whether that window is a fragment.
//
// highlighted is content with markers inserted, so the marker's index in
// it is the match's index in content: everything before the marker is
// unmodified. A body with no marker (highlight matched in another column,
// say the title) falls back to the head of the content -- the honest
// answer when there is no match position to centre on.
func extract(content, highlighted string) (string, bool) {
	if len(content) <= SnippetBudget {
		return content, false
	}

	start := 0
	if i := strings.Index(highlighted, matchOpen); i >= 0 {
		// Centre the window on the match, then pull it back inside the
		// body at both ends.
		start = i - SnippetBudget/2
		if start < 0 {
			start = 0
		}
		if start+SnippetBudget > len(content) {
			start = len(content) - SnippetBudget
		}
	}
	end := start + SnippetBudget

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
