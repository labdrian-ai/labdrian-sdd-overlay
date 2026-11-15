package ops

import (
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// The doctor joins a page to its memory by the store-agnostic identity the page names (`source: memorycard` and
// `source_id`) alone: the same page with its identity keys turned into the retired spelling names no memory, so it is
// not judged.
func TestDoctorJoinsAPageToItsMemoryByItsStoreAgnosticIdentityAlone(t *testing.T) {
	const want = "c-000012: memory 41 is not pinned and has no topic_key, so sync does not refresh this page; run `longterm-mem promote --source memorycard --id 41` to refresh it now, or pin memory 41 in memorycard so sync keeps it current"
	agnostic := promotedFrom(t, memory.Observation{ID: 41}, "c-000012")
	if !strings.Contains(agnostic[1], "source: memorycard\nsource_id: 41\n") {
		t.Fatalf("fixture lacks the store-agnostic identity keys:\n%s", agnostic[1])
	}
	retired := [2]string{agnostic[0], strings.Replace(agnostic[1], "source: memorycard\nsource_id: 41\n", "memorycard_id: 41\n", 1)}
	if retired[1] == agnostic[1] {
		t.Fatalf("fixture did not change the identity keys:\n%s", retired[1])
	}
	for name, tc := range map[string]struct {
		page [2]string
		want Check
	}{
		"store-agnostic keys": {agnostic, Check{Name: CheckPromotedPagesEligible, Status: CheckWarned, Detail: want}},
		"retired keys":        {retired, Check{Name: CheckPromotedPagesEligible, Status: CheckPassed}},
	} {
		t.Run(name, func(t *testing.T) {
			memories := &memoriesOf{rows: []MemoryEligibility{{ID: 41}}}
			if got := eligibilityCheckOf(t, newFakePages(tc.page), memories.read); got != tc.want {
				t.Errorf("promoted-pages-eligible = %+v, want %+v", got, tc.want)
			}
		})
	}
}
