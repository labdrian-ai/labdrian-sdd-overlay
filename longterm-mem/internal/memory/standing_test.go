package memory_test

import (
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

func TestStandingIsEmptyOnlyWhenThereIsNothingToTellAReader(t *testing.T) {
	someone := []memory.Neighbour{{ID: 7, Title: "another memory"}}
	cases := []struct {
		name     string
		standing memory.Standing
		empty    bool
	}{
		{"nothing at all", memory.Standing{}, true},
		{"only empty lists", memory.Standing{SupersededBy: []memory.Neighbour{}, ConflictsWith: nil, Unjudged: []memory.Neighbour{}}, true},
		{"replaced by another", memory.Standing{SupersededBy: someone}, false},
		{"in conflict with another", memory.Standing{ConflictsWith: someone}, false},
		{"flagged and never judged", memory.Standing{Unjudged: someone}, false},
	}
	for _, tc := range cases {
		if got := tc.standing.Empty(); got != tc.empty {
			t.Errorf("%s: Empty() = %v, want %v", tc.name, got, tc.empty)
		}
	}
}
