package engram

import "github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"

// Transitional names. The domain model moved to internal/memory, and the consumers of this package
// are switched to it one at a time; until the last one is, these aliases and forwards keep them
// compiling. This file is deleted with that last switch.

type (
	Observation  = memory.Observation
	Row          = memory.Row
	SearchResult = memory.SearchResult
	Neighbour    = memory.Neighbour
	Standing     = memory.Standing
	Edge         = memory.Edge
)

const (
	MatchAll      = memory.MatchAll
	MatchAny      = memory.MatchAny
	SnippetBudget = memory.SnippetBudget
)

// SearchTokens forwards to memory.SearchTokens.
func SearchTokens(query string) (tokens, dropped []string) { return memory.SearchTokens(query) }

// SnippetAt forwards to memory.SnippetAt.
func SnippetAt(content string, offset, budget int) (string, bool) {
	return memory.SnippetAt(content, offset, budget)
}
