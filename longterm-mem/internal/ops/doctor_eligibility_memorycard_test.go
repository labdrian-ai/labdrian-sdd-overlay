package ops

import (
	"strconv"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/opstest"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
)

// promotedFromMemory is the page a promotion writes for a memorycard memory under address, as the pair newFakePages
// takes: the address and the whole text of the file.
func promotedFromMemory(t *testing.T, id int64, address string) [2]string {
	t.Helper()
	page, err := promote.EmitPage(memory.Memory{Identity: memory.Identity{Source: "memorycard", ID: strconv.FormatInt(id, 10)}, Project: doctorTestProject, Type: "decision", Title: "Widget Decision", Body: "Body content.", Revision: 1, Status: memory.StatusActive}, address, nil, opstest.Instant())
	if err != nil {
		t.Fatalf("EmitPage: %v", err)
	}
	return [2]string{address, page.Frontmatter + page.Body}
}

// S11: a memorycard page is joined to its memorycard memory by memorycard_id, and the advice names a command that
// works: the memory's id and memorycard, not an Engram pin.
func TestDoctorJoinsAMemorycardPageToItsMemory(t *testing.T) {
	pages := newFakePages(promotedFromMemory(t, 12, "c-000012"))
	memories := &observationsOf{rows: []MemoryEligibility{{ID: 12}}}

	got := eligibilityCheckOf(t, pages, memories.read)

	want := Check{Name: CheckPromotedPagesEligible, Status: CheckWarned, Detail: "c-000012: memory 12 is not pinned and has no topic_key, so sync does not refresh this page; run `longterm-mem promote --source memorycard --id 12` to refresh it now, or pin memory 12 in memorycard so sync keeps it current"}
	if got != want {
		t.Errorf("promoted-pages-eligible = %+v, want %+v", got, want)
	}
}

// S11: a page of an earlier release names an Engram observation, and is joined to the memory memorycard imported from
// it: the memory's own id is the one the advice names, not the observation's.
func TestDoctorJoinsALegacyEngramPageToTheMemoryImportedFromItsObservation(t *testing.T) {
	pages := newFakePages(promotedFrom(t, memory.Observation{ID: 41}, "c-000012"))
	memories := &observationsOf{rows: []MemoryEligibility{{ID: 7, EngramID: 41}}}

	got := eligibilityCheckOf(t, pages, memories.read)

	want := Check{Name: CheckPromotedPagesEligible, Status: CheckWarned, Detail: "c-000012: memory 7 is not pinned and has no topic_key, so sync does not refresh this page; the page predates memorycard identity and is refreshed only after the one-time vault migration; pin memory 7 in memorycard so sync keeps it current after it"}
	if got != want {
		t.Errorf("promoted-pages-eligible = %+v, want %+v", got, want)
	}
}

// S11, the coordinator's decision for L56: for a page of an earlier release the advice names no promote command, which the
// memory's bridge to that page would refuse; it says the page is refreshed after the one-time migration, and the pin
// in memorycard is the way out that works.
func TestDoctorAdvisesALegacyPageNoPromoteCommand(t *testing.T) {
	pages := newFakePages(promotedFrom(t, memory.Observation{ID: 41}, "c-000012"))
	memories := &observationsOf{rows: []MemoryEligibility{{ID: 7, EngramID: 41}}}

	got := eligibilityCheckOf(t, pages, memories.read)

	want := Check{Name: CheckPromotedPagesEligible, Status: CheckWarned, Detail: "c-000012: memory 7 is not pinned and has no topic_key, so sync does not refresh this page; the page predates memorycard identity and is refreshed only after the one-time vault migration; pin memory 7 in memorycard so sync keeps it current after it"}
	if got != want {
		t.Errorf("promoted-pages-eligible = %+v, want %+v", got, want)
	}
	if strings.Contains(got.Detail, "promote") {
		t.Errorf("the advice names a promote command: %q", got.Detail)
	}
}

// F3: a page is judged by the live memories of its observation, never a superseded one. One observation imported as
// several parts is eligible when any live part is, and a warning names the first live part; a memorycard page whose
// memory is superseded is no concern of the check.
func TestDoctorJudgesAPageByTheLiveMemoriesOfItsObservation(t *testing.T) {
	for name, tc := range map[string]struct {
		page string
		rows []MemoryEligibility
		want string
	}{
		"a superseded part is never flagged":  {"legacy", []MemoryEligibility{{ID: 7, EngramID: 41, Superseded: true}}, ""},
		"the live part is the one named":      {"legacy", []MemoryEligibility{{ID: 7, EngramID: 41, Superseded: true}, {ID: 8, EngramID: 41}}, "c-000012: memory 8 is not pinned"},
		"any live part eligible passes":       {"legacy", []MemoryEligibility{{ID: 7, EngramID: 41}, {ID: 8, EngramID: 41, Pinned: true}}, ""},
		"no live part eligible warns":         {"legacy", []MemoryEligibility{{ID: 7, EngramID: 41}, {ID: 8, EngramID: 41}}, "c-000012: memory 7 is not pinned"},
		"a superseded memorycard memory":      {"memorycard", []MemoryEligibility{{ID: 12, Superseded: true}}, ""},
		"the named part gives its own reason": {"legacy", []MemoryEligibility{{ID: 7, EngramID: 41}, {ID: 8, EngramID: 41, TopicKey: "sdd/x"}}, "c-000012: memory 7 is not pinned and has no topic_key"},
	} {
		t.Run(name, func(t *testing.T) {
			page := promotedFrom(t, memory.Observation{ID: 41}, "c-000012")
			if tc.page == "memorycard" {
				page = promotedFromMemory(t, 12, "c-000012")
			}
			got := eligibilityCheckOf(t, newFakePages(page), (&observationsOf{rows: tc.rows}).read)

			if tc.want == "" {
				if got.Status != CheckPassed {
					t.Errorf("promoted-pages-eligible = %+v, want a pass", got)
				}
				return
			}
			if got.Status != CheckWarned || !strings.HasPrefix(got.Detail, tc.want) {
				t.Errorf("promoted-pages-eligible = %+v, want a warning starting %q", got, tc.want)
			}
		})
	}
}

// A memorycard page is not joined through the Engram id of another memory: memory 41 (not imported from anywhere)
// does not answer for a page that names memory 12, and a memory whose EngramID is 12 is not memory 12.
func TestDoctorDoesNotJoinAMemorycardPageThroughAnEngramId(t *testing.T) {
	pages := newFakePages(promotedFromMemory(t, 12, "c-000012"))
	memories := &observationsOf{rows: []MemoryEligibility{{ID: 99, EngramID: 12}}}

	got := eligibilityCheckOf(t, pages, memories.read)

	if want := (Check{Name: CheckPromotedPagesEligible, Status: CheckPassed}); got != want {
		t.Errorf("promoted-pages-eligible = %+v, want %+v: the page names memorycard memory 12, which is not among the live ones", got, want)
	}
}

// An eligible memory passes whichever identity the page carries.
func TestDoctorPassesAMemorycardPageWhoseMemoryIsPinned(t *testing.T) {
	pages := newFakePages(promotedFromMemory(t, 12, "c-000012"), promotedFrom(t, memory.Observation{ID: 41}, "c-000013"))
	memories := &observationsOf{rows: []MemoryEligibility{{ID: 12, Pinned: true}, {ID: 7, EngramID: 41, TopicKey: "longterm-mem/policy"}}}

	got := eligibilityCheckOf(t, pages, memories.read)

	if want := (Check{Name: CheckPromotedPagesEligible, Status: CheckPassed}); got != want {
		t.Errorf("promoted-pages-eligible = %+v, want %+v", got, want)
	}
}
