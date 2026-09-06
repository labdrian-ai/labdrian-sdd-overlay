package query

import "strings"

// routeRank1 decides which source's top row should occupy rank 1 when more
// than one source is requested (R-059).
//
// It is a heuristic over query SHAPE, nothing else. It does not compute or
// compare a relevance score (D8 is untouched by it), and a wrong decision
// only affects which row is first: R-058's union guarantee holds
// regardless, because the merge already contains both sources' top-`top`
// rows before this gate is asked anything.
//
// matchMode is engram.MatchAll or engram.MatchAny (the caller's own
// widened-search signal): MatchAny already means the precise reading found
// nothing, which is itself evidence the query leans lexical/identifier
// rather than paraphrase, so it alone routes to the FTS source.
//
// An identifier-shaped token -- an interior CamelCase boundary, a `/`, a
// `_`, a `(`, a `)`, or a dotted `word.word` -- routes to the FTS source.
// Anything else routes to the embedding source. This function computes the
// decision; PR-4's validation (openspec/decisions/union-retrieval-gate-
// validation.md) decides, separately, whether it is ever wired live.
func routeRank1(tokens []string, matchMode string) string {
	if matchMode == "any" {
		return SourceEngramFTS
	}
	for _, t := range tokens {
		if isIdentifierShaped(t) {
			return SourceEngramFTS
		}
	}
	return SourceEngramEmbed
}

// isIdentifierShaped reports whether token carries a shape natural
// language rarely does: an interior CamelCase boundary, a path/underscore
// separator, a call-shaped parenthesis, or a dotted `word.word` -- the
// shapes a symbol name, a file path, or a function call take, and the
// shapes bm25 lexical matching finds exactly and cosine similarity finds
// only approximately.
func isIdentifierShaped(token string) bool {
	if strings.ContainsAny(token, "/_()") {
		return true
	}
	if hasDottedWord(token) {
		return true
	}
	return hasInteriorCamelCaseBoundary(token)
}

// hasDottedWord reports a `word.word` shape: a `.` with a letter or digit
// on both sides, distinct from trailing sentence punctuation like
// "writer." or a bare ellipsis. Byte indexing is deliberate: the shape
// this looks for is ASCII punctuation around ASCII/digit boundaries, and a
// full UTF-8 decode buys nothing a caller of this gate needs.
func hasDottedWord(token string) bool {
	for i := 0; i < len(token); i++ {
		if token[i] != '.' {
			continue
		}
		if i == 0 || i == len(token)-1 {
			continue
		}
		if isAlnum(rune(token[i-1])) && isAlnum(rune(token[i+1])) {
			return true
		}
	}
	return false
}

// hasInteriorCamelCaseBoundary reports a lowercase-to-uppercase transition
// that is not the first character -- "search.go"'s "S" or a bare
// "MyType"'s "T" -- the boundary an identifier has and an English sentence
// does not.
func hasInteriorCamelCaseBoundary(token string) bool {
	runes := []rune(token)
	for i := 1; i < len(runes); i++ {
		if isLower(runes[i-1]) && isUpper(runes[i]) {
			return true
		}
	}
	return false
}

func isAlnum(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

func isLower(r rune) bool { return r >= 'a' && r <= 'z' }
func isUpper(r rune) bool { return r >= 'A' && r <= 'Z' }
