package memory_test

import (
	"encoding/json"
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

// The standing is part of what the query command and the MCP query tool print as JSON; its field names are
// the wire format a client reads.
func TestStandingKeepsItsWireFormat(t *testing.T) {
	standing := memory.Standing{
		SupersededBy:  []memory.Neighbour{{ID: 9, Title: "new"}},
		ConflictsWith: []memory.Neighbour{{ID: 11, Title: "other"}},
		Unjudged:      []memory.Neighbour{{ID: 13, Title: "maybe"}},
	}
	got, err := json.Marshal(standing)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	const want = `{"superseded_by":[{"id":9,"title":"new"}],"conflicts_with":[{"id":11,"title":"other"}],"unjudged":[{"id":13,"title":"maybe"}]}`
	if string(got) != want {
		t.Fatalf("json = %s, want %s", got, want)
	}
	empty, err := json.Marshal(memory.Standing{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(empty) != `{}` {
		t.Fatalf("an empty standing marshals as %s, want {}: a row with nothing to report carries no noise", empty)
	}
}
