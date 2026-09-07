package query

import (
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
)

// routeRank1 decides which source's top row should occupy rank 1 when more
// than one source is requested (R-059).
//
// It is a heuristic over query SHAPE and FTS's own match-mode signal,
// nothing else. It does not compute or compare a relevance score (D8 is
// untouched by it), and a wrong decision only affects which row is first:
// R-058's union guarantee holds regardless, because the merge already
// contains both sources' top-`top` rows before this gate is asked
// anything.
//
// The two conditions are ORed, not chained as an early return: an
// identifier-shaped token -- an interior CamelCase boundary, a `/`, a `_`,
// a `(`, a `)`, or a dotted `word.word` -- routes to the FTS source, and so
// does matchMode == engram.MatchAll -- FTS matched the query PRECISELY,
// every token required, without needing to widen. Only when NEITHER holds
// does rank 1 go to the embedding source.
//
// This was wrong once already, in the direction the natural first idea
// gets it wrong (openspec/decisions/union-retrieval.md §4.3): using
// engram.MatchAny (FTS had to WIDEN because the precise AND search found
// nothing) as a signal FOR the embedding arm looked elegant -- "FTS's own
// admission of weakness" -- and was rejected because ~90% of realistic
// identifier questions ALSO widen (a multi-token natural-language question
// about a symbol rarely AND-matches every one of its own words), so that
// rule alone misroutes the majority of realistic identifier queries to the
// arm measured at 0% identifier@1. A second, precisely inverted mistake
// checked matchMode == engram.MatchAny as an early return FOR the FTS
// source -- which pre-empts the token-shape rule below it rather than
// being ORed with it, and since EVERY natural-language paraphrase question
// also widens (100% of the frozen blind validation set), that early return
// collapsed paraphrase routing accuracy to 22% (see
// TestGateRoutingAccuracyOnBlindSet's mutation proof).
//
// The corrected form here -- shape OR matchMode==MatchAll -- is tied on
// the blind set with a simpler shape-only rule (both land at the same
// count of right decisions once the early-return defect is removed; see
// git history for the measurement). It is shipped anyway, over shape-only,
// because it is the decision record's own validated rule and a strict
// superset of shape-only's FTS-routing conditions: it can only route MORE
// precisely-matched queries to FTS, never fewer, and the decision record's
// own realistic-query measurement needed exactly that (§4.3: shape-only
// alone misrouted "R-021 exec allowlist", 8/9; shape-OR-match-mode got
// 9/9). Nothing in the blind set exercises that difference (every blind
// paraphrase query widens, so the match-mode clause never fires for
// paraphrase either way here), so the tie is real and the choice rests on
// that outside evidence, not on this set.
func routeRank1(tokens []string, matchMode string) string {
	for _, t := range tokens {
		if isIdentifierShaped(t) {
			return SourceEngramFTS
		}
	}
	if matchMode == engram.MatchAll {
		return SourceEngramFTS
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
