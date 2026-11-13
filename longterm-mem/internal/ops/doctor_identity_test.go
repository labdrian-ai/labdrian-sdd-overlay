package ops

import (
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// The doctor joins a page to its observation by the identity the page names, whichever spelling of the keys
// names it: the same finding for a page with `engram_id` and for the same page with `source: engram` and
// `source_id`.
func TestDoctorJoinsAPageToItsObservationByItsStoreAgnosticIdentity(t *testing.T) {
	const want = "c-000012: memory 41 is not pinned and has no topic_key, so sync does not refresh this page; run `longterm-mem promote --source memorycard --id 41` to refresh it now, or pin memory 41 in memorycard so sync keeps it current"
	legacy := promotedFrom(t, memory.Observation{ID: 41}, "c-000012")
	agnostic := [2]string{legacy[0], strings.Replace(legacy[1], "engram_id: 41\n", "source: engram\nsource_id: 41\n", 1)}
	if agnostic[1] == legacy[1] || strings.Contains(agnostic[1], "engram_id") {
		t.Fatalf("fixture did not change the identity keys:\n%s", agnostic[1])
	}
	for name, page := range map[string][2]string{"legacy keys": legacy, "store-agnostic keys": agnostic} {
		t.Run(name, func(t *testing.T) {
			observations := &observationsOf{rows: []MemoryEligibility{{ID: 41, EngramID: 41}}}
			got := eligibilityCheckOf(t, newFakePages(page), observations.read)
			if wantCheck := (Check{Name: CheckPromotedPagesEligible, Status: CheckWarned, Detail: want}); got != wantCheck {
				t.Errorf("promoted-pages-eligible = %+v, want %+v", got, wantCheck)
			}
		})
	}
}
