package engram

import "strings"

// SearchTokens splits query into the tokens actually searched, and the
// stopwords removed from it.
//
// It splits on whitespace only (strings.Fields), never on punctuation
// inside a token: "search.go:181" is one token, and "a/b c" is two,
// "a/b" and "c". A tokenizer that instead split on punctuation would
// break apart the identifier shapes (paths, dotted names) this module
// depends on elsewhere -- the rank-1 routing gate (R-059) reads exactly
// this shape to decide which source to trust for rank 1.
//
// It is exported so any caller outside this package -- notably a test
// harness that must reproduce Search's own query semantics -- calls the
// exact function production uses, rather than a reimplementation that can
// silently drift from it (design: "production's own tokenizer is
// exported, not re-implemented").
//
// A query made entirely of stopwords keeps every one of them: there is
// nothing else the caller can have meant by it, and answering a
// deliberate query with an empty one is the failure this whole change
// exists to remove.
func SearchTokens(query string) (tokens, dropped []string) {
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
