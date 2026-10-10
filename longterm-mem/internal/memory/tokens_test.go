package memory_test

import (
	"reflect"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// TestSearchTokensSplitsOnFieldsNotPunctuation pins the divergence the design calls out: a tokenizer
// that split on punctuation would tear apart exactly the identifier shapes (paths, dotted names) the
// rank-1 routing gate (R-059) reads to decide which source to trust for rank 1. SearchTokens splits on
// whitespace only.
func TestSearchTokensSplitsOnFieldsNotPunctuation(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  []string
	}{
		{"a path with a colon and line number stays one token", "search.go:181", []string{"search.go:181"}},
		{"whitespace splits tokens, punctuation inside one does not", "a/b c", []string{"a/b", "c"}},
		{"a leading dash stays part of the token", "-foo bar", []string{"-foo", "bar"}},
		{"tabs and newlines split like spaces", "one\ttwo\nthree", []string{"one", "two", "three"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := memory.SearchTokens(tc.query)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("SearchTokens(%q) = %v, want %v", tc.query, got, tc.want)
			}
		})
	}
}

func TestSearchTokensOfNothingAreNothing(t *testing.T) {
	for _, query := range []string{"", "   ", "\t\n"} {
		tokens, dropped := memory.SearchTokens(query)
		if tokens != nil || dropped != nil {
			t.Errorf("SearchTokens(%q) = %v, %v, want nil, nil", query, tokens, dropped)
		}
	}
}

// Stopwords are reported as they were written, in the order they were written, and the tokens that
// remain keep their case: the caller is told which of its own words were never searched.
func TestSearchTokensReportsTheStopwordsItDroppedAsWritten(t *testing.T) {
	tokens, dropped := memory.SearchTokens("What is the Register writer")
	if want := []string{"Register", "writer"}; !reflect.DeepEqual(tokens, want) {
		t.Errorf("tokens = %v, want %v", tokens, want)
	}
	if want := []string{"What", "is", "the"}; !reflect.DeepEqual(dropped, want) {
		t.Errorf("dropped = %v, want %v", dropped, want)
	}
}

// A query made entirely of stopwords keeps every one of them: answering a deliberate query with an empty
// one is the failure the stopword list exists to remove, and nothing was dropped, so nothing is reported.
func TestSearchTokensKeepsAQueryMadeOfStopwordsOnly(t *testing.T) {
	tokens, dropped := memory.SearchTokens("the and of")
	if want := []string{"the", "and", "of"}; !reflect.DeepEqual(tokens, want) {
		t.Errorf("tokens = %v, want %v", tokens, want)
	}
	if dropped != nil {
		t.Errorf("dropped = %v, want none: every word was kept", dropped)
	}
}

// stopwords is the list the query path strips. It is spelled out here so that a word added to or lost
// from the list is a failing test, not a silent change in what a query searches.
var stopwords = []string{
	"a", "about", "all", "an", "and", "any", "are", "as", "at", "be", "been", "but", "by", "can", "did",
	"do", "does", "for", "from", "had", "has", "have", "how", "i", "if", "in", "into", "is", "it", "its",
	"me", "my", "no", "not", "of", "on", "or", "our", "should", "so", "some", "than", "that", "the",
	"their", "them", "then", "there", "these", "they", "this", "to", "up", "was", "we", "were", "what",
	"when", "where", "which", "who", "why", "will", "with", "would", "you", "your",
}

func TestSearchTokensDropsEveryStopwordAndOnlyThose(t *testing.T) {
	for _, word := range stopwords {
		tokens, dropped := memory.SearchTokens(word + " register")
		if !reflect.DeepEqual(tokens, []string{"register"}) || !reflect.DeepEqual(dropped, []string{word}) {
			t.Errorf("SearchTokens(%q + \" register\") = %v, %v, want the stopword dropped and register kept", word, tokens, dropped)
		}
	}
	for _, word := range []string{"register", "writer", "cache", "use", "after", "before", "work"} {
		tokens, dropped := memory.SearchTokens(word + " register")
		if len(dropped) != 0 || len(tokens) != 2 {
			t.Errorf("SearchTokens(%q + \" register\") = %v, %v, want both words kept: %q is not a stopword", word, tokens, dropped, word)
		}
	}
}
