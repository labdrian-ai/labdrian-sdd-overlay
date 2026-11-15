package ops

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/opstest"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
)

// promoted-pages-eligible joins each promoted page with the memory it was promoted from (the page's
// engram_id) and reports, as an advisory warning, every page whose memory promote.Eligible(obs, false)
// refuses: sync no longer refreshes it (issue #293). These tests drive it through Doctor, with the pages a reader
// answers and the memories a function answers, and name no check function of the package.

// promotedFrom is the page a promotion writes for obs under address, as the pair newFakePages takes: the address
// and the whole text of the file.
func promotedFrom(t *testing.T, obs memory.Observation, address string) [2]string {
	t.Helper()
	obs.Type, obs.Title, obs.Content, obs.Project, obs.RevisionCount = "decision", "Widget Decision", "Body content.", doctorTestProject, 1
	page, err := promote.EmitPage(opstest.CardMemory(obs), address, nil, opstest.Instant())
	if err != nil {
		t.Fatalf("EmitPage: %v", err)
	}
	return [2]string{address, page.Frontmatter + page.Body}
}

// memoriesOf answers the live memories it holds and records the projects it was asked for.
type memoriesOf struct {
	rows  []MemoryEligibility
	err   error
	asked []string
}

func (o *memoriesOf) read(project string) ([]MemoryEligibility, error) {
	o.asked = append(o.asked, project)
	return o.rows, o.err
}

// eligibilityCheckOf is the promoted-pages-eligible check of a Doctor whose pages are the reader's and whose
// live memories are the function's, the rest of the vault being whatever pagesDoctorDeps builds.
func eligibilityCheckOf(t *testing.T, pages PageReader, memories func(string) ([]MemoryEligibility, error)) Check {
	t.Helper()
	deps := pagesDoctorDeps(t, pages)
	deps.LiveMemoryEligibility = memories
	report, err := Doctor(context.Background(), deps, doctorTestProject)
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}
	return checkStatus(t, report.Checks, CheckPromotedPagesEligible)
}

// A page whose memory is not pinned and has no curated topic_key is frozen, and the doctor names the page,
// the memory, why it is not eligible and the two ways out, as a warning.
func TestDoctorWarnsOfAPageWhoseMemoryIsNotEligible(t *testing.T) {
	for _, tc := range []struct {
		name string
		obs  MemoryEligibility
		want string
	}{
		{
			name: "no topic_key",
			obs:  MemoryEligibility{ID: 41},
			want: "c-000012: memory 41 is not pinned and has no topic_key, so sync does not refresh this page; run `longterm-mem promote --source memorycard --id 41` to refresh it now, or pin memory 41 in memorycard so sync keeps it current",
		},
		{
			name: "a whitespace topic_key is no topic_key",
			obs:  MemoryEligibility{ID: 41, TopicKey: "  "},
			want: "c-000012: memory 41 is not pinned and has no topic_key, so sync does not refresh this page; run `longterm-mem promote --source memorycard --id 41` to refresh it now, or pin memory 41 in memorycard so sync keeps it current",
		},
		{
			name: "an sdd/ topic",
			obs:  MemoryEligibility{ID: 41, TopicKey: "sdd/some-change/tasks"},
			want: "c-000012: memory 41 is not pinned and its topic_key \"sdd/some-change/tasks\" is not a curated topic, so sync does not refresh this page; run `longterm-mem promote --source memorycard --id 41` to refresh it now, or pin memory 41 in memorycard so sync keeps it current",
		},
		{
			name: "a review/ topic",
			obs:  MemoryEligibility{ID: 41, TopicKey: "review/some-change/verdict"},
			want: "c-000012: memory 41 is not pinned and its topic_key \"review/some-change/verdict\" is not a curated topic, so sync does not refresh this page; run `longterm-mem promote --source memorycard --id 41` to refresh it now, or pin memory 41 in memorycard so sync keeps it current",
		},
		{
			name: "a delivery/ topic",
			obs:  MemoryEligibility{ID: 41, TopicKey: "delivery/some-change/receipt"},
			want: "c-000012: memory 41 is not pinned and its topic_key \"delivery/some-change/receipt\" is not a curated topic, so sync does not refresh this page; run `longterm-mem promote --source memorycard --id 41` to refresh it now, or pin memory 41 in memorycard so sync keeps it current",
		},
		{
			// A page promoted by an explicit `promote --id N` of a memory that never was eligible: nothing on
			// the page records that it was promoted explicitly, so it is flagged all the same, until the
			// memory is pinned.
			name: "a page promoted explicitly",
			obs:  MemoryEligibility{ID: 7},
			want: "c-000012: memory 7 is not pinned and has no topic_key, so sync does not refresh this page; run `longterm-mem promote --source memorycard --id 7` to refresh it now, or pin memory 7 in memorycard so sync keeps it current",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pages := newFakePages(promotedFrom(t, memory.Observation{ID: tc.obs.ID}, "c-000012"))
			memories := &memoriesOf{rows: []MemoryEligibility{tc.obs}}

			got := eligibilityCheckOf(t, pages, memories.read)

			want := Check{Name: CheckPromotedPagesEligible, Status: CheckWarned, Detail: tc.want}
			if got != want {
				t.Errorf("promoted-pages-eligible = %+v, want %+v", got, want)
			}
		})
	}
}

// A page whose memory is pinned, or carries a curated topic_key, is kept current by sync: it passes, with no
// detail.
func TestDoctorPassesAPageWhoseMemoryIsEligible(t *testing.T) {
	for _, tc := range []struct {
		name string
		obs  MemoryEligibility
	}{
		{"pinned", MemoryEligibility{ID: 41, Pinned: true}},
		{"a curated topic", MemoryEligibility{ID: 41, TopicKey: "longterm-mem/promotion-eligibility-policy"}},
		{"a curated topic of a goal", MemoryEligibility{ID: 41, TopicKey: "goal:g-42/longterm-mem/policy"}},
		{"pinned, whatever the topic", MemoryEligibility{ID: 41, Pinned: true, TopicKey: "sdd/some-change/tasks"}},
		{"sdd-init/ is not sdd/", MemoryEligibility{ID: 41, TopicKey: "sdd-init/onboarding"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pages := newFakePages(promotedFrom(t, memory.Observation{ID: tc.obs.ID}, "c-000012"))
			memories := &memoriesOf{rows: []MemoryEligibility{tc.obs}}

			got := eligibilityCheckOf(t, pages, memories.read)

			if want := (Check{Name: CheckPromotedPagesEligible, Status: CheckPassed}); got != want {
				t.Errorf("promoted-pages-eligible = %+v, want %+v", got, want)
			}
		})
	}
}

// The rule is promotion's own: whatever promote.Eligible refuses, with no explicit call, is a warning, and
// whatever it accepts passes. A table over the memories Eligible is held to by its own test would repeat
// that test here; this one asks the two to agree on a spread of them.
func TestDoctorAgreesWithPromoteEligible(t *testing.T) {
	for _, obs := range []memory.Observation{
		{ID: 41},
		{ID: 41, Pinned: true},
		{ID: 41, TopicKey: "foo"},
		{ID: 41, TopicKey: "sdd"},
		{ID: 41, TopicKey: "/sdd/x"},
		{ID: 41, TopicKey: "/"},
		{ID: 41, TopicKey: "SDD/x"},
		{ID: 41, TopicKey: "sddx/whatever"},
		{ID: 41, TopicKey: "goal:g-42/"},
		{ID: 41, TopicKey: "goal:g-42/delivery/receipt"},
		{ID: 41, TopicKey: "goal:g-42/notes"},
		{ID: 41, Pinned: true, TopicKey: "review/some-change/verdict"},
	} {
		pages := newFakePages(promotedFrom(t, memory.Observation{ID: obs.ID}, "c-000012"))
		memories := &memoriesOf{rows: []MemoryEligibility{{ID: obs.ID, Pinned: obs.Pinned, TopicKey: obs.TopicKey}}}

		got := eligibilityCheckOf(t, pages, memories.read)

		wantStatus := CheckWarned
		if promote.Eligible(opstest.CardMemory(obs), false) {
			wantStatus = CheckPassed
		}
		if got.Status != wantStatus {
			t.Errorf("pinned=%v topic_key=%q: promoted-pages-eligible = %+v, want %s (promote.Eligible says %v)", obs.Pinned, obs.TopicKey, got, wantStatus, promote.Eligible(opstest.CardMemory(obs), false))
		}
	}
}

// A page whose memory is not among the live ones, deleted or the row of another project, is not this check's
// concern: it is skipped, and the pages after it are still judged.
func TestDoctorSkipsAPageWhoseMemoryIsNotLive(t *testing.T) {
	pages := newFakePages(
		promotedFrom(t, memory.Observation{ID: 90}, "c-000010"),
		promotedFrom(t, memory.Observation{ID: 41}, "c-000011"),
	)
	memories := &memoriesOf{rows: []MemoryEligibility{{ID: 41}}}

	got := eligibilityCheckOf(t, pages, memories.read)

	if got.Status != CheckWarned || !strings.HasPrefix(got.Detail, "c-000011: memory 41 ") {
		t.Fatalf("promoted-pages-eligible = %+v, want a warning about c-000011 alone", got)
	}
	if strings.Contains(got.Detail, "c-000010") || strings.Contains(got.Detail, "memory 90") {
		t.Errorf("promoted-pages-eligible detail %q names the page whose memory is not live", got.Detail)
	}

	only := newFakePages(promotedFrom(t, memory.Observation{ID: 90}, "c-000010"))
	if got := eligibilityCheckOf(t, only, (&memoriesOf{rows: []MemoryEligibility{{ID: 41}}}).read); got.Status != CheckPassed || got.Detail != "" {
		t.Errorf("promoted-pages-eligible = %+v, want PASS: the one page's memory is not live", got)
	}
}

// A page that names no memory has none to join with: it is skipped, never read as memory 0.
func TestDoctorSkipsAPageThatNamesNoMemory(t *testing.T) {
	pages := newFakePages(
		[2]string{"c-000050", "Just text, no frontmatter.\n"},
		[2]string{"c-000051", "---\naddress: c-000051\n---\nbody\n"},
		[2]string{"c-000052", "---\naddress: c-000052\nengram_id: not-a-number\n---\nbody\n"},
		[2]string{"c-000053", ""},
		[2]string{"c-000054", "---\naddress: c-000054\nsource: memorycard\nsource_id: not-a-number\n---\nbody\n"},
	)
	// The one row Doctor could join those pages to, if it read an absent or unparseable source_id, or the retired engram_id, as 0.
	memories := &memoriesOf{rows: []MemoryEligibility{{ID: 0}}}

	got := eligibilityCheckOf(t, pages, memories.read)

	if want := (Check{Name: CheckPromotedPagesEligible, Status: CheckPassed}); got != want {
		t.Errorf("promoted-pages-eligible = %+v, want %+v", got, want)
	}
	if len(memories.asked) != 0 {
		t.Errorf("the live memories were asked for %v: no page names one, so there is nothing to ask", memories.asked)
	}
}

// The memory a page was promoted from is the source_id of its frontmatter and nothing its body says: lines of prose that
// look like the identity (source_id, or the retired engram_id) do not move the page to another memory.
func TestDoctorReadsTheMemoryOfAPageFromItsFrontmatterAlone(t *testing.T) {
	for _, line := range []string{"source_id: 99", "engram_id: 99"} {
		t.Run(line, func(t *testing.T) {
			page := promotedFrom(t, memory.Observation{ID: 21}, "c-000001")
			page[1] += "\n" + line + "\n"
			memories := &memoriesOf{rows: []MemoryEligibility{{ID: 21}, {ID: 99, Pinned: true}}}

			got := eligibilityCheckOf(t, newFakePages(page), memories.read)

			if got.Status != CheckWarned || !strings.HasPrefix(got.Detail, "c-000001: memory 21 ") || strings.Contains(got.Detail, "memory 99") {
				t.Errorf("promoted-pages-eligible = %+v, want a warning about memory 21, the page's own", got)
			}
		})
	}
}

// Every frozen page is named, in the order the reader lists the pages, joined as the other checks join their
// findings; the pages that are kept current are not named.
func TestDoctorNamesEveryFrozenPageInTheOrderOfThePages(t *testing.T) {
	pages := newFakePages(
		promotedFrom(t, memory.Observation{ID: 21}, "c-000001"),
		promotedFrom(t, memory.Observation{ID: 22}, "c-000002"),
		promotedFrom(t, memory.Observation{ID: 23}, "c-000003"),
	)
	memories := &memoriesOf{rows: []MemoryEligibility{
		{ID: 23, TopicKey: "sdd/x/tasks"},
		{ID: 22, Pinned: true},
		{ID: 21},
	}}

	got := eligibilityCheckOf(t, pages, memories.read)

	const first = "c-000001: memory 21 is not pinned and has no topic_key, so sync does not refresh this page; run `longterm-mem promote --source memorycard --id 21` to refresh it now, or pin memory 21 in memorycard so sync keeps it current"
	const third = "c-000003: memory 23 is not pinned and its topic_key \"sdd/x/tasks\" is not a curated topic, so sync does not refresh this page; run `longterm-mem promote --source memorycard --id 23` to refresh it now, or pin memory 23 in memorycard so sync keeps it current"
	if got.Status != CheckWarned || got.Detail != first+"; "+third {
		t.Errorf("promoted-pages-eligible = %+v, want a warning with %q then %q", got, first, third)
	}
}

// The memories are asked for once, for the project of the call, and not at all when there is no page to
// judge.
func TestDoctorAsksForTheMemoriesOfTheProjectOnceAndOnlyWhenAPageNeedsThem(t *testing.T) {
	t.Run("a page names a memory", func(t *testing.T) {
		pages := newFakePages(
			promotedFrom(t, memory.Observation{ID: 21}, "c-000001"),
			promotedFrom(t, memory.Observation{ID: 22}, "c-000002"),
		)
		memories := &memoriesOf{}

		eligibilityCheckOf(t, pages, memories.read)

		if len(memories.asked) != 1 || memories.asked[0] != doctorTestProject {
			t.Errorf("the memories were asked for %v, want once, for %q", memories.asked, doctorTestProject)
		}
	})
	t.Run("no page", func(t *testing.T) {
		memories := &memoriesOf{err: errors.New("it must not be read")}

		got := eligibilityCheckOf(t, newFakePages(), memories.read)

		if want := (Check{Name: CheckPromotedPagesEligible, Status: CheckPassed}); got != want {
			t.Errorf("promoted-pages-eligible = %+v, want %+v", got, want)
		}
		if len(memories.asked) != 0 {
			t.Errorf("the memories were asked for %v with no page to judge", memories.asked)
		}
	})
}

// A page that cannot be read is the page checks' finding, in the words they have always used: it is not named
// here, and it does not hide the pages after it.
func TestDoctorLeavesAnUnreadablePageToThePageChecks(t *testing.T) {
	pages := newFakePages(
		[2]string{"c-000050", ""},
		promotedFrom(t, memory.Observation{ID: 21}, "c-000051"),
	)
	pages.readErrs = map[string]error{"c-000050": errors.New("read c-000050.md: is a directory")}
	memories := &memoriesOf{rows: []MemoryEligibility{{ID: 21}}}

	got := eligibilityCheckOf(t, pages, memories.read)

	if got.Status != CheckWarned || !strings.HasPrefix(got.Detail, "c-000051: memory 21 ") || strings.Contains(got.Detail, "c-000050") {
		t.Errorf("promoted-pages-eligible = %+v, want a warning about c-000051 alone", got)
	}
}

// What the check cannot read, it says so as a warning, in the words of the port that failed, and never as a
// failure: it is advisory, so it cannot be the reason a doctor fails.
func TestDoctorSaysEligibilityCannotBeJudgedAndNeverFailsForIt(t *testing.T) {
	onePage := func() *fakePages { return newFakePages(promotedFrom(t, memory.Observation{ID: 21}, "c-000001")) }
	listFails := onePage()
	listFails.listErr = errors.New("boom")
	var typedNilPages *fakePages

	for _, tc := range []struct {
		name     string
		pages    PageReader
		memories func(string) ([]MemoryEligibility, error)
		want     string
	}{
		{
			name:     "the memories cannot be read",
			pages:    onePage(),
			memories: (&memoriesOf{err: errors.New("database is locked")}).read,
			want:     "eligibility cannot be judged: could not read the live memories of labdrian-sdd-overlay: database is locked",
		},
		{
			name:     "no reader of the memories was wired",
			pages:    onePage(),
			memories: nil,
			want:     "eligibility cannot be judged: ops: no memory eligibility reader was wired",
		},
		{
			name:     "the pages cannot be listed",
			pages:    listFails,
			memories: (&memoriesOf{}).read,
			want:     "eligibility cannot be judged: ops: list promoted pages in /fake/vault/wiki/memory: boom",
		},
		{
			name:     "no page reader was wired",
			pages:    nil,
			memories: (&memoriesOf{}).read,
			want:     "eligibility cannot be judged: ops: no page reader was wired",
		},
		{
			name:     "a page reader that is a nil pointer",
			pages:    typedNilPages,
			memories: (&memoriesOf{}).read,
			want:     "eligibility cannot be judged: ops: no page reader was wired",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := eligibilityCheckOf(t, tc.pages, tc.memories)

			want := Check{Name: CheckPromotedPagesEligible, Status: CheckWarned, Detail: tc.want}
			if got != want {
				t.Errorf("promoted-pages-eligible = %+v, want %+v", got, want)
			}
		})
	}
}

// A Doctor built without a reader of the memories has nothing to ask when no page names one, and says
// nothing: the missing wiring is a finding only when it stops a page being judged.
func TestDoctorWithoutAMemoryReaderPassesWhenNoPageNamesAMemory(t *testing.T) {
	got := eligibilityCheckOf(t, newFakePages(), nil)

	if want := (Check{Name: CheckPromotedPagesEligible, Status: CheckPassed}); got != want {
		t.Errorf("promoted-pages-eligible = %+v, want %+v", got, want)
	}
}

// The check is advisory in every state it can be in: over a spread of vaults and failures it answers PASS or warn
// and never FAIL, so the exit code of the command, which counts FAIL alone, cannot be changed by it.
func TestPromotedPagesEligibleNeverAnswersFAIL(t *testing.T) {
	failing := newFakePages(promotedFrom(t, memory.Observation{ID: 21}, "c-000001"))
	failing.listErr = errors.New("boom")
	for name, check := range map[string]Check{
		"frozen":              eligibilityCheckOf(t, newFakePages(promotedFrom(t, memory.Observation{ID: 21}, "c-000001")), (&memoriesOf{rows: []MemoryEligibility{{ID: 21}}}).read),
		"healthy":             eligibilityCheckOf(t, newFakePages(promotedFrom(t, memory.Observation{ID: 21}, "c-000001")), (&memoriesOf{rows: []MemoryEligibility{{ID: 21, Pinned: true}}}).read),
		"engram unreadable":   eligibilityCheckOf(t, newFakePages(promotedFrom(t, memory.Observation{ID: 21}, "c-000001")), (&memoriesOf{err: errors.New("boom")}).read),
		"reader not wired":    eligibilityCheckOf(t, newFakePages(promotedFrom(t, memory.Observation{ID: 21}, "c-000001")), nil),
		"pages not listable":  eligibilityCheckOf(t, failing, (&memoriesOf{}).read),
		"pages reader absent": eligibilityCheckOf(t, nil, (&memoriesOf{}).read),
	} {
		if check.Status == CheckFailed {
			t.Errorf("%s: promoted-pages-eligible = %+v, want PASS or warn, never FAIL", name, check)
		}
	}
}

// A warning is one finding among the others, not a stop: a vault with a frozen page and a broken address map
// reports both, each at its own severity.
func TestDoctorReportsAWarningBesideAFailure(t *testing.T) {
	deps := pagesDoctorDeps(t, newFakePages(promotedFrom(t, memory.Observation{ID: 21}, "c-000001")))
	deps.LiveMemoryEligibility = (&memoriesOf{rows: []MemoryEligibility{{ID: 21}}}).read

	report, err := Doctor(context.Background(), deps, doctorTestProject)
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}

	if got := checkStatus(t, report.Checks, CheckAddressMapIntegrity); got.Status != CheckFailed {
		t.Errorf("address-map-integrity = %+v, want FAIL: the page this fixture lists has no address map entry", got)
	}
	if got := checkStatus(t, report.Checks, CheckPromotedPagesEligible); got.Status != CheckWarned {
		t.Errorf("promoted-pages-eligible = %+v, want warn beside the failure", got)
	}
}

// The checks Doctor ran before the eligibility check was added keep their names, their order and their place at
// the head of the report, so whoever reads the report by position or by name finds them where they were, and the
// new one is the last. A vault whose every memory is pinned passes all of them.
func TestDoctorKeepsItsEightChecksInTheirOrderAndAddsTheEligibilityCheckLast(t *testing.T) {
	index, live := newHealthyEmbeddingDeps(t)
	deps := doctorDepsWith(t, index, live...)
	// The healthy vault's one page is promoted from memory 1.
	deps.LiveMemoryEligibility = (&memoriesOf{rows: []MemoryEligibility{{ID: 1, Pinned: true}}}).read

	report, err := Doctor(context.Background(), deps, doctorTestProject)
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}

	want := []string{
		CheckVaultConfigResolvable, CheckAddressMapIntegrity, CheckWikiRegistrationConsistency, CheckPrecedenceSidecarConsistency,
		CheckRuntimePrerequisites, CheckEmbeddingIndexPresent, CheckEmbeddingIndexFresh, CheckEmbeddingBackendReachable,
		CheckPromotedPagesEligible,
	}
	if len(report.Checks) != len(want) {
		t.Fatalf("Doctor reported %d checks, want %d: %+v", len(report.Checks), len(want), report.Checks)
	}
	for i, check := range report.Checks {
		if check.Name != want[i] || check.Status != CheckPassed || check.Detail != "" {
			t.Errorf("check %d = %+v, want %s to PASS with no detail", i, check, want[i])
		}
	}
}

// The status of an advisory check is the word "warn" in the JSON the command prints, and the name of the
// check and its detail are the keys they have always been.
func TestAWarningIsTheWordWarnInTheJSON(t *testing.T) {
	report := DoctorReport{Project: "p", Checks: []Check{{Name: CheckPromotedPagesEligible, Status: CheckWarned, Detail: "c-000001: memory 1 is frozen"}}}

	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("encode the report: %v", err)
	}

	const want = `{"project":"p","checks":[{"name":"promoted-pages-eligible","status":"warn","detail":"c-000001: memory 1 is frozen"}]}`
	if string(encoded) != want {
		t.Errorf("the report encodes as\n%s\nwant\n%s", encoded, want)
	}
	if CheckWarned != "warn" {
		t.Errorf("CheckWarned = %q, want %q", CheckWarned, "warn")
	}
}
