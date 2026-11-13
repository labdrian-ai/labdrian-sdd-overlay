package promote

import (
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// A memory that cannot be promoted at all (an id that is not a number, a store whose pages have no format) is
// failed in its own words: it is not told "check promoted state", which is about the pages, and the failure names
// what could not be promoted. The page listing's failure keeps those words (the goldens pin them).
func TestSync_AMemoryThatCannotBePromotedAtAllIsNotToldToCheckPromotedState(t *testing.T) {
	for name, tc := range map[string]struct {
		m memory.Memory
		// names is what the failure says could not be promoted: the id that is no number, the store with no format.
		names string
	}{
		"an id that is not a number":  {memory.Memory{Identity: memory.Identity{Source: engramSource, ID: "x1"}, Project: "memorycard", Title: "T", Pinned: true}, `"x1"`},
		"a store with no page format": {memory.Memory{Identity: memory.Identity{Source: "elsewhere", ID: "1"}, Project: "memorycard", Title: "T", Pinned: true}, `"elsewhere"`},
	} {
		t.Run(name, func(t *testing.T) {
			w := syncCardsWriter(t.TempDir())

			report, err := SyncCardsProject(t.Context(), fixedSource{memories: []memory.Memory{tc.m}}, w, nil, "memorycard")

			if err == nil || len(report.Failed) != 1 {
				t.Fatalf("Sync = (%+v, %v), want the memory failed", report, err)
			}
			got := report.Failed[0].Err.Error()
			if strings.Contains(got, "check promoted state") {
				t.Errorf("failure %q is worded about the state of a page; it is about the memory", got)
			}
			if !strings.Contains(got, tc.names) {
				t.Errorf("failure %q does not name %s", got, tc.names)
			}
		})
	}
}
