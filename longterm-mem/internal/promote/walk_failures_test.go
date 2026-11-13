package promote

import (
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// A memory that cannot be promoted at all (an id that is not a number, a store whose pages have no format) is
// failed in its own words: it is not told "check promoted state", which is about the pages. The page listing's
// failure keeps those words (the goldens pin them).
func TestSync_AMemoryThatCannotBePromotedAtAllIsNotToldToCheckPromotedState(t *testing.T) {
	for name, m := range map[string]memory.Memory{
		"an id that is not a number":  {Identity: memory.Identity{Source: engramSource, ID: "x1"}, Project: "memorycard", Title: "T", Pinned: true},
		"a store with no page format": {Identity: memory.Identity{Source: "elsewhere", ID: "1"}, Project: "memorycard", Title: "T", Pinned: true},
	} {
		t.Run(name, func(t *testing.T) {
			w := syncCardsWriter(t.TempDir())

			report, err := Sync(t.Context(), Deps{Source: fixedSource{memories: []memory.Memory{m}}, Writer: w}, "memorycard")

			if err == nil || len(report.Failed) != 1 {
				t.Fatalf("Sync = (%+v, %v), want the memory failed", report, err)
			}
			if got := report.Failed[0].Err.Error(); strings.Contains(got, "check promoted state") {
				t.Errorf("failure %q is worded about the state of a page; it is about the memory", got)
			}
		})
	}
}
