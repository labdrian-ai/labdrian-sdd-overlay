package ops

import (
	"strconv"
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

	want := Check{Name: CheckPromotedPagesEligible, Status: CheckWarned, Detail: "c-000012: memory 7 is not pinned and has no topic_key, so sync does not refresh this page; run `longterm-mem promote --source memorycard --id 7` to refresh it now, or pin memory 7 in memorycard so sync keeps it current"}
	if got != want {
		t.Errorf("promoted-pages-eligible = %+v, want %+v", got, want)
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
