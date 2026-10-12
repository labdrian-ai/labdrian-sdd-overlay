package memory_test

import (
	"reflect"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// The domain model carries no wire format. How a standing, a row or an observation is written on the query
// command and the MCP tools is the surface's to say (internal/query owns the shape those carry, Phase 9, L4), so
// that renaming a field of the wire does not touch the domain and a change to the domain does not change what a
// client reads.
func TestTheDomainModelCarriesNoWireFormat(t *testing.T) {
	for _, model := range []any{
		memory.Observation{}, memory.Row{}, memory.SearchResult{}, memory.Standing{}, memory.Neighbour{}, memory.Edge{},
	} {
		typ := reflect.TypeOf(model)
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			if tag, ok := field.Tag.Lookup("json"); ok {
				t.Errorf("%s.%s has the json tag %q: a wire format belongs to the surface that sends it", typ.Name(), field.Name, tag)
			}
		}
	}
}
