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

// S7: a page that carries only the retired keys (engram_id, memorycard_id) names no memory, so the check does not judge
// it, whatever memory has that id and whatever its eligibility: the page is no promoted page of any memory.
func TestDoctorDoesNotJudgeAPageThatCarriesOnlyTheRetiredKeys(t *testing.T) {
	for name, keys := range map[string]string{
		"engram keys":     "engram_id: 41\nengram_revision: 1\nengram_type: decision\n",
		"memorycard keys": "memorycard_id: 41\nmemorycard_revision: 1\nmemorycard_type: decision\n",
	} {
		t.Run(name, func(t *testing.T) {
			page := [2]string{"c-000012", "---\naddress: c-000012\n" + keys + "project: " + doctorTestProject + "\n---\nBody.\n"}
			memories := &observationsOf{rows: []MemoryEligibility{{ID: 41}}}

			got := eligibilityCheckOf(t, newFakePages(page), memories.read)

			if want := (Check{Name: CheckPromotedPagesEligible, Status: CheckPassed}); got != want {
				t.Errorf("promoted-pages-eligible = %+v, want %+v: the page names no memory", got, want)
			}
			if memories.asked != nil {
				t.Errorf("the live memories were asked for (%v) though no page names a memory", memories.asked)
			}
		})
	}
}

// F3: a page is judged by its live memory, never a superseded one: a memorycard page whose memory is superseded is no
// concern of the check.
func TestDoctorDoesNotJudgeAPageWhoseMemoryIsSuperseded(t *testing.T) {
	pages := newFakePages(promotedFromMemory(t, 12, "c-000012"))
	memories := &observationsOf{rows: []MemoryEligibility{{ID: 12, Superseded: true}}}

	got := eligibilityCheckOf(t, pages, memories.read)

	if want := (Check{Name: CheckPromotedPagesEligible, Status: CheckPassed}); got != want {
		t.Errorf("promoted-pages-eligible = %+v, want %+v", got, want)
	}
}

// An eligible memory passes.
func TestDoctorPassesAMemorycardPageWhoseMemoryIsPinned(t *testing.T) {
	pages := newFakePages(promotedFromMemory(t, 12, "c-000012"), promotedFromMemory(t, 7, "c-000013"))
	memories := &observationsOf{rows: []MemoryEligibility{{ID: 12, Pinned: true}, {ID: 7, TopicKey: "longterm-mem/policy"}}}

	got := eligibilityCheckOf(t, pages, memories.read)

	if want := (Check{Name: CheckPromotedPagesEligible, Status: CheckPassed}); got != want {
		t.Errorf("promoted-pages-eligible = %+v, want %+v", got, want)
	}
}
