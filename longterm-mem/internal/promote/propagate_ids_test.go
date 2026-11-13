package promote

import (
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// A memory whose id is not a number cannot be reported by an id the failures and the patch list record. Both
// propagation walks fail it in its own words, as the promotion walk does, instead of patching its page and naming
// memory 0 in what they report (L26, f2196397-2 F5).
func TestPropagation_AMemoryWhoseIdIsNotANumberIsFailedInItsOwnWordsAndItsPageIsLeftAlone(t *testing.T) {
	for name, tc := range map[string]struct {
		source string
		idKey  string
		walk   func(*Writer, Source) (PropagateReport, error)
	}{
		"memorycard's walk": {
			source: memorycardSource, idKey: "memorycard_id",
			walk: func(w *Writer, src Source) (PropagateReport, error) {
				return PropagateCards(t.Context(), src, w, "memorycard")
			},
		},
		"the Engram dialect's walk": {
			source: engramSource, idKey: "engram_id",
			walk: func(w *Writer, src Source) (PropagateReport, error) {
				return Propagate(t.Context(), Deps{Source: src, Writer: w}, "memorycard")
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			vaultRoot := t.TempDir()
			writeIdentityPage(t, vaultRoot, "c-000001.md", "---\n"+tc.idKey+": x1\nproject: memorycard\naddress: c-000001\nstatus: active\n---\n\nBody.\n")
			before := readPage(t, vaultRoot, "c-000001")
			w := syncCardsWriter(vaultRoot)
			w.Precedence = &memPrecedence{}
			archived := memory.Memory{Identity: memory.Identity{Source: tc.source, ID: "x1"}, Project: "memorycard", Title: "T", Status: memory.StatusArchived}

			report, err := tc.walk(w, fixedSource{memories: []memory.Memory{archived}})

			if err == nil || len(report.Patched) != 0 || len(report.Failed) != 1 {
				t.Fatalf("propagation = (%+v, %v), want nothing patched and the memory failed", report, err)
			}
			if got := report.Failed[0].Err.Error(); !strings.Contains(got, `"x1"`) || !strings.Contains(got, "not a number") {
				t.Errorf("failure = %q, want it to say memory %q has an id that is not a number", got, "x1")
			}
			if after := readPage(t, vaultRoot, "c-000001"); after != before {
				t.Errorf("the page changed:\n%s", after)
			}
		})
	}
}
