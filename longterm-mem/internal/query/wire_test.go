package query

import (
	"encoding/json"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// The standing of a row is part of what the query command and the MCP query tool print as JSON; its field names
// are the wire format a client reads. The query package owns that format, over the domain's Standing, which
// carries none.
func TestStandingKeepsItsWireFormat(t *testing.T) {
	standing := Standing{
		SupersededBy:  []Neighbour{{ID: 9, Title: "new"}},
		ConflictsWith: []Neighbour{{ID: 11, Title: "other"}},
		Unjudged:      []Neighbour{{ID: 13, Title: "maybe"}},
	}
	got, err := json.Marshal(standing)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	const want = `{"superseded_by":[{"id":9,"title":"new"}],"conflicts_with":[{"id":11,"title":"other"}],"unjudged":[{"id":13,"title":"maybe"}]}`
	if string(got) != want {
		t.Fatalf("json = %s, want %s", got, want)
	}
	empty, err := json.Marshal(Standing{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(empty) != `{}` {
		t.Fatalf("an empty standing marshals as %s, want {}: a row with nothing to report carries no noise", empty)
	}
}

// What the memory says about a row reaches the wire as it is said, in the three lists, in the order the memory
// gave them, with the ids and titles of the neighbours.
func TestTheDomainsStandingIsWrittenInTheWireFormat(t *testing.T) {
	domain := memory.Standing{
		SupersededBy:  []memory.Neighbour{{ID: 9, Title: "first"}, {ID: 8, Title: "second"}},
		ConflictsWith: []memory.Neighbour{{ID: 11, Title: "other"}},
		Unjudged:      []memory.Neighbour{{ID: 13, Title: "maybe"}},
	}

	got, err := json.Marshal(standingOf(domain))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	const want = `{"superseded_by":[{"id":9,"title":"first"},{"id":8,"title":"second"}],"conflicts_with":[{"id":11,"title":"other"}],"unjudged":[{"id":13,"title":"maybe"}]}`
	if string(got) != want {
		t.Fatalf("json = %s, want %s", got, want)
	}
}

// A list the memory left unsaid stays unsaid: it is absent from the wire, not an empty list.
func TestAListTheMemoryLeftUnsaidIsAbsentFromTheWire(t *testing.T) {
	wire := standingOf(memory.Standing{ConflictsWith: []memory.Neighbour{{ID: 11, Title: "other"}}})
	if wire.SupersededBy != nil || wire.Unjudged != nil {
		t.Errorf("the lists left unsaid are %v and %v, want both nil", wire.SupersededBy, wire.Unjudged)
	}
	got, err := json.Marshal(wire)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if want := `{"conflicts_with":[{"id":11,"title":"other"}]}`; string(got) != want {
		t.Fatalf("json = %s, want %s", got, want)
	}
}
