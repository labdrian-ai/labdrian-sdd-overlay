package memory

import "unicode/utf8"

// SnippetBudget is the number of characters an extract may carry.
//
// It is derived, not chosen. The response ceiling this module enforces is ~2,000 tokens, and a
// default query returns at most DefaultTopN rows from each of two sources; at ~4 bytes per token
// that leaves each memory row roughly 480 characters if the ceiling is to bind only on unusual
// shapes rather than on every ordinary call. 480 characters is also about three to four lines of
// the structured markdown these bodies are written in -- enough to carry the sentence a match sits
// in, which the vault's own 200-character cap frequently is not for this content.
const SnippetBudget = 480

// TruncationMark is what a cut edge looks like to a person reading the text. It is not decoration
// and it is not optional: a preview a reader cannot tell from a whole memory is worse than no
// preview, because a decision then gets made on a fragment that looked complete. The
// machine-readable half of the same statement is Row.SnippetTruncated and Row.ContentLength -- a
// marker in prose is not something a program can act on, and a boolean is not something a person
// reads.
const TruncationMark = "…"

// SnippetAt returns a budget-sized window of content centred on offset (a byte index into content),
// and whether that window is a fragment of the whole.
//
// It is the one rule for turning a body into a snippet, used by a store that finds the match offset
// and by query's response assembly, which re-renders a row's snippet at a different budget once the
// byte ceiling is allocated across rows (design: "the snippet budget is allocated per ROW, not per
// source"), without re-running the search that produced content. offset is the match's position in
// content; a row with no lexical match position -- the embedding arm has none, a cosine match is not
// a location in text -- passes 0, which renders an honest head slice rather than inventing a
// position the retrieval method cannot support.
func SnippetAt(content string, offset, budget int) (string, bool) {
	if len(content) <= budget {
		return content, false
	}

	// Centre the window on offset, then pull it back inside the body at both ends.
	start := offset - budget/2
	if start < 0 {
		start = 0
	}
	if start+budget > len(content) {
		start = len(content) - budget
	}
	end := start + budget

	// Never cut a rune in half: a snippet is text a person reads, and a severed multi-byte
	// character renders as a replacement glyph.
	for start > 0 && !utf8.RuneStart(content[start]) {
		start--
	}
	for end < len(content) && !utf8.RuneStart(content[end]) {
		end++
	}

	snippet := content[start:end]
	if start > 0 {
		snippet = TruncationMark + snippet
	}
	if end < len(content) {
		snippet += TruncationMark
	}
	return snippet, true
}
