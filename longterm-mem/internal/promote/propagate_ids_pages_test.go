package promote

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultlayout"
)

// The two propagation walks, for a memory whose id is not a number, whatever state the pages are in: the failure says
// what the id is (SyncFailure.ObservationID is 0 for it, and the error names the real one). They are told apart
// by the memory they are handed: a retired memory with an id that is no number, over pages that cannot be listed or
// over a page that cannot be read.
func propagationWalks(t *testing.T) map[string]struct {
	source string
	walk   func(*Writer, Source) (PropagateReport, error)
} {
	return map[string]struct {
		source string
		walk   func(*Writer, Source) (PropagateReport, error)
	}{
		"memorycard's walk": {memorycardSource, func(w *Writer, src Source) (PropagateReport, error) {
			return PropagateCards(t.Context(), src, w, "memorycard")
		}},
		"the Engram dialect's walk": {engramSource, func(w *Writer, src Source) (PropagateReport, error) {
			return Propagate(t.Context(), Deps{Source: src, Writer: w}, "memorycard")
		}},
	}
}

func TestPropagation_AnUnlistablePagesDirectoryStillNamesAnIdThatIsNotANumber(t *testing.T) {
	for name, tc := range propagationWalks(t) {
		t.Run(name, func(t *testing.T) {
			w := syncCardsWriter(t.TempDir())
			w.Pages = failingPages{PageRepository: w.Pages}
			archived := memory.Memory{Identity: memory.Identity{Source: tc.source, ID: "x1"}, Project: "memorycard", Title: "T", Status: memory.StatusArchived}

			report, err := tc.walk(w, fixedSource{memories: []memory.Memory{archived}})

			if err == nil || len(report.Failed) != 1 {
				t.Fatalf("propagation = (%+v, %v), want the memory failed", report, err)
			}
			got := report.Failed[0].Err.Error()
			if !strings.Contains(got, "pages unreadable") || !strings.Contains(got, `"x1"`) || !strings.Contains(got, "not a number") {
				t.Errorf("failure = %q, want the unlistable pages and the id %q that is not a number", got, "x1")
			}
		})
	}
}

// A page that cannot be read may be the page of a memory with no readable page, so the memory is left alone and named
// as withheld; a memory whose id is no number cannot be named as withheld, which records the id, and is failed in its
// own words.
func TestPropagation_AnUnreadablePageWithholdsANumericMemoryAndFailsOneWhoseIdIsNoNumber(t *testing.T) {
	for name, tc := range propagationWalks(t) {
		t.Run(name, func(t *testing.T) {
			vaultRoot := t.TempDir()
			w := syncCardsWriter(vaultRoot)
			pages := filepath.Join(vaultRoot, vaultlayout.PagesDir)
			if err := os.MkdirAll(pages, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(vaultRoot, "nowhere.md"), filepath.Join(pages, "c-000555.md")); err != nil {
				t.Skipf("cannot make a link: %v", err)
			}
			numeric := memory.Memory{Identity: memory.Identity{Source: tc.source, ID: "43"}, Project: "memorycard", Title: "T", Status: memory.StatusArchived}
			odd := memory.Memory{Identity: memory.Identity{Source: tc.source, ID: "x1"}, Project: "memorycard", Title: "T", Status: memory.StatusArchived}

			report, _ := tc.walk(w, fixedSource{memories: []memory.Memory{numeric, odd}})

			if len(report.Withheld) != 1 || report.Withheld[0] != 43 {
				t.Errorf("withheld = %v, want memory 43 only", report.Withheld)
			}
			if len(report.Failed) != 1 || !strings.Contains(report.Failed[0].Err.Error(), `"x1"`) || !strings.Contains(report.Failed[0].Err.Error(), "not a number") {
				t.Errorf("failed = %+v, want the memory whose id %q is not a number, in its own words", report.Failed, "x1")
			}
		})
	}
}
