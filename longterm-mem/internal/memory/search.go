package memory

// Row is one lexical search match, returned in the store's own rank order (best match first) rather
// than insertion order (R-006).
type Row struct {
	ID      int64
	Title   string
	Content string
	Project string
	// Snippet is an extract of Content centred on the first match, cut to SnippetBudget characters.
	// It exists so a caller can show why a row matched without shipping Content, whose measured p90
	// on the live corpus is 33,991 bytes.
	Snippet string
	// SnippetTruncated reports that Snippet is a fragment of Content, not the whole of it.
	SnippetTruncated bool
	// ContentLength is len(Content) in bytes: how much a truncated Snippet is not showing.
	ContentLength int
	// MatchOffset is Content's byte index of the first match, the same position Snippet was centred
	// on. It lets a caller re-render the snippet at a different budget (query's budget-before-render
	// allocation) without re-running the search. A row with no match position to report is 0, the
	// same head-of-content fallback a snippet uses.
	MatchOffset int
}

// MatchMode reports how strictly a search's tokens were combined.
const (
	// MatchAll required every token: the precise reading of the query.
	MatchAll = "all"
	// MatchAny required only one: the widened reading, used only when MatchAll found nothing at all.
	MatchAny = "any"
)

// SearchResult is what a search found and how it had to ask.
//
// The two are inseparable on purpose. Rows alone cannot distinguish a precise hit from a
// deliberately broadened one, and cannot show that some of the caller's own words were never
// searched -- both of which change how much the results are worth trusting.
type SearchResult struct {
	// Rows are the matches, in the store's own rank order.
	Rows []Row
	// MatchMode is MatchAll or MatchAny.
	MatchMode string
	// DroppedTokens are the caller's tokens removed as stopwords, in the order they were written.
	// Empty when nothing was dropped.
	DroppedTokens []string
}
