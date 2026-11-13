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

// promoted-pages-eligible joins each promoted page with the observation it was promoted from (the page's
// engram_id) and reports, as an advisory warning, every page whose observation promote.Eligible(obs, false)
// refuses: sync no longer refreshes it (issue #293). These tests drive it through Doctor, with the pages a reader
// answers and the observations a function answers, and name no check function of the package.

// promotedFrom is the page a promotion writes for obs under address, as the pair newFakePages takes: the address
// and the whole text of the file.
func promotedFrom(t *testing.T, obs memory.Observation, address string) [2]string {
	t.Helper()
	obs.Type, obs.Title, obs.Content, obs.Project, obs.RevisionCount = "decision", "Widget Decision", "Body content.", doctorTestProject, 1
	page, err := promote.EmitPage(opstest.EngramMemory(obs), address, nil, opstest.Instant())
	if err != nil {
		t.Fatalf("EmitPage: %v", err)
	}
	return [2]string{address, page.Frontmatter + page.Body}
}

// observationsOf answers the live observations it holds and records the projects it was asked for.
type observationsOf struct {
	rows  []MemoryEligibility
	err   error
	asked []string
}

func (o *observationsOf) read(project string) ([]MemoryEligibility, error) {
	o.asked = append(o.asked, project)
	return o.rows, o.err
}

// eligibilityCheckOf is the promoted-pages-eligible check of a Doctor whose pages are the reader's and whose
// live observations are the function's, the rest of the vault being whatever pagesDoctorDeps builds.
func eligibilityCheckOf(t *testing.T, pages PageReader, observations func(string) ([]MemoryEligibility, error)) Check {
	t.Helper()
	deps := pagesDoctorDeps(t, pages)
	deps.LiveMemoryEligibility = observations
	report, err := Doctor(context.Background(), deps, doctorTestProject)
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}
	return checkStatus(t, report.Checks, CheckPromotedPagesEligible)
}

// A page whose observation is not pinned and has no curated topic_key is frozen, and the doctor names the page,
// the observation, why it is not eligible and the two ways out, as a warning.
func TestDoctorWarnsOfAPageWhoseObservationIsNotEligible(t *testing.T) {
	for _, tc := range []struct {
		name string
		obs  MemoryEligibility
		want string
	}{
		{
			name: "no topic_key",
			obs:  MemoryEligibility{ID: 41, EngramID: 41},
			want: "c-000012: memory 41 is not pinned and has no topic_key, so sync does not refresh this page; the page predates memorycard identity and is refreshed only after the one-time vault migration; pin memory 41 in memorycard so sync keeps it current after it",
		},
		{
			name: "a whitespace topic_key is no topic_key",
			obs:  MemoryEligibility{ID: 41, EngramID: 41, TopicKey: "  "},
			want: "c-000012: memory 41 is not pinned and has no topic_key, so sync does not refresh this page; the page predates memorycard identity and is refreshed only after the one-time vault migration; pin memory 41 in memorycard so sync keeps it current after it",
		},
		{
			name: "an sdd/ topic",
			obs:  MemoryEligibility{ID: 41, EngramID: 41, TopicKey: "sdd/some-change/tasks"},
			want: "c-000012: memory 41 is not pinned and its topic_key \"sdd/some-change/tasks\" is not a curated topic, so sync does not refresh this page; the page predates memorycard identity and is refreshed only after the one-time vault migration; pin memory 41 in memorycard so sync keeps it current after it",
		},
		{
			name: "a review/ topic",
			obs:  MemoryEligibility{ID: 41, EngramID: 41, TopicKey: "review/some-change/verdict"},
			want: "c-000012: memory 41 is not pinned and its topic_key \"review/some-change/verdict\" is not a curated topic, so sync does not refresh this page; the page predates memorycard identity and is refreshed only after the one-time vault migration; pin memory 41 in memorycard so sync keeps it current after it",
		},
		{
			name: "a delivery/ topic",
			obs:  MemoryEligibility{ID: 41, EngramID: 41, TopicKey: "delivery/some-change/receipt"},
			want: "c-000012: memory 41 is not pinned and its topic_key \"delivery/some-change/receipt\" is not a curated topic, so sync does not refresh this page; the page predates memorycard identity and is refreshed only after the one-time vault migration; pin memory 41 in memorycard so sync keeps it current after it",
		},
		{
			// A page promoted by an explicit `promote --id N` of an observation that never was eligible: nothing on
			// the page records that it was promoted explicitly, so it is flagged all the same, until the
			// observation is pinned.
			name: "a page promoted explicitly",
			obs:  MemoryEligibility{ID: 7, EngramID: 7},
			want: "c-000012: memory 7 is not pinned and has no topic_key, so sync does not refresh this page; the page predates memorycard identity and is refreshed only after the one-time vault migration; pin memory 7 in memorycard so sync keeps it current after it",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pages := newFakePages(promotedFrom(t, memory.Observation{ID: tc.obs.ID}, "c-000012"))
			observations := &observationsOf{rows: []MemoryEligibility{tc.obs}}

			got := eligibilityCheckOf(t, pages, observations.read)

			want := Check{Name: CheckPromotedPagesEligible, Status: CheckWarned, Detail: tc.want}
			if got != want {
				t.Errorf("promoted-pages-eligible = %+v, want %+v", got, want)
			}
		})
	}
}

// A page whose observation is pinned, or carries a curated topic_key, is kept current by sync: it passes, with no
// detail.
func TestDoctorPassesAPageWhoseObservationIsEligible(t *testing.T) {
	for _, tc := range []struct {
		name string
		obs  MemoryEligibility
	}{
		{"pinned", MemoryEligibility{ID: 41, EngramID: 41, Pinned: true}},
		{"a curated topic", MemoryEligibility{ID: 41, EngramID: 41, TopicKey: "longterm-mem/promotion-eligibility-policy"}},
		{"a curated topic of a goal", MemoryEligibility{ID: 41, EngramID: 41, TopicKey: "goal:g-42/longterm-mem/policy"}},
		{"pinned, whatever the topic", MemoryEligibility{ID: 41, EngramID: 41, Pinned: true, TopicKey: "sdd/some-change/tasks"}},
		{"sdd-init/ is not sdd/", MemoryEligibility{ID: 41, EngramID: 41, TopicKey: "sdd-init/onboarding"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pages := newFakePages(promotedFrom(t, memory.Observation{ID: tc.obs.ID}, "c-000012"))
			observations := &observationsOf{rows: []MemoryEligibility{tc.obs}}

			got := eligibilityCheckOf(t, pages, observations.read)

			if want := (Check{Name: CheckPromotedPagesEligible, Status: CheckPassed}); got != want {
				t.Errorf("promoted-pages-eligible = %+v, want %+v", got, want)
			}
		})
	}
}

// The rule is promotion's own: whatever promote.Eligible refuses, with no explicit call, is a warning, and
// whatever it accepts passes. A table over the observations Eligible is held to by its own test would repeat
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
		observations := &observationsOf{rows: []MemoryEligibility{{ID: obs.ID, EngramID: obs.ID, Pinned: obs.Pinned, TopicKey: obs.TopicKey}}}

		got := eligibilityCheckOf(t, pages, observations.read)

		wantStatus := CheckWarned
		if promote.Eligible(opstest.EngramMemory(obs), false) {
			wantStatus = CheckPassed
		}
		if got.Status != wantStatus {
			t.Errorf("pinned=%v topic_key=%q: promoted-pages-eligible = %+v, want %s (promote.Eligible says %v)", obs.Pinned, obs.TopicKey, got, wantStatus, promote.Eligible(opstest.EngramMemory(obs), false))
		}
	}
}

// A page whose observation is not among the live ones, deleted or the row of another project, is not this
// check's concern: it is skipped, and the pages after it are still judged.
func TestDoctorSkipsAPageWhoseObservationIsNotLive(t *testing.T) {
	pages := newFakePages(
		promotedFrom(t, memory.Observation{ID: 90}, "c-000010"),
		promotedFrom(t, memory.Observation{ID: 41}, "c-000011"),
	)
	observations := &observationsOf{rows: []MemoryEligibility{{ID: 41, EngramID: 41}}}

	got := eligibilityCheckOf(t, pages, observations.read)

	if got.Status != CheckWarned || !strings.HasPrefix(got.Detail, "c-000011: memory 41 ") {
		t.Fatalf("promoted-pages-eligible = %+v, want a warning about c-000011 alone", got)
	}
	if strings.Contains(got.Detail, "c-000010") || strings.Contains(got.Detail, "memory 90") {
		t.Errorf("promoted-pages-eligible detail %q names the page whose observation is not live", got.Detail)
	}

	only := newFakePages(promotedFrom(t, memory.Observation{ID: 90}, "c-000010"))
	if got := eligibilityCheckOf(t, only, (&observationsOf{rows: []MemoryEligibility{{ID: 41, EngramID: 41}}}).read); got.Status != CheckPassed || got.Detail != "" {
		t.Errorf("promoted-pages-eligible = %+v, want PASS: the one page's observation is not live", got)
	}
}

// A page that names no observation has none to join with: it is skipped, never read as observation 0.
func TestDoctorSkipsAPageThatNamesNoObservation(t *testing.T) {
	pages := newFakePages(
		[2]string{"c-000050", "Just text, no frontmatter.\n"},
		[2]string{"c-000051", "---\naddress: c-000051\n---\nbody\n"},
		[2]string{"c-000052", "---\naddress: c-000052\nengram_id: not-a-number\n---\nbody\n"},
		[2]string{"c-000053", ""},
	)
	// The one row Doctor could join those pages to, if it read an absent or unparseable engram_id as 0.
	observations := &observationsOf{rows: []MemoryEligibility{{ID: 0, EngramID: 0}}}

	got := eligibilityCheckOf(t, pages, observations.read)

	if want := (Check{Name: CheckPromotedPagesEligible, Status: CheckPassed}); got != want {
		t.Errorf("promoted-pages-eligible = %+v, want %+v", got, want)
	}
	if len(observations.asked) != 0 {
		t.Errorf("the observations were asked for %v: no page names one, so there is nothing to ask", observations.asked)
	}
}

// The observation a page was promoted from is the engram_id of its frontmatter and nothing its body says: a line
// of prose that looks like the field does not move the page to another observation.
func TestDoctorReadsTheObservationOfAPageFromItsFrontmatterAlone(t *testing.T) {
	page := promotedFrom(t, memory.Observation{ID: 21}, "c-000001")
	page[1] += "\nengram_id: 99\n"
	observations := &observationsOf{rows: []MemoryEligibility{{ID: 21, EngramID: 21}, {ID: 99, EngramID: 99, Pinned: true}}}

	got := eligibilityCheckOf(t, newFakePages(page), observations.read)

	if got.Status != CheckWarned || !strings.HasPrefix(got.Detail, "c-000001: memory 21 ") || strings.Contains(got.Detail, "memory 99") {
		t.Errorf("promoted-pages-eligible = %+v, want a warning about memory 21, the page's own", got)
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
	observations := &observationsOf{rows: []MemoryEligibility{
		{ID: 23, EngramID: 23, TopicKey: "sdd/x/tasks"},
		{ID: 22, EngramID: 22, Pinned: true},
		{ID: 21, EngramID: 21},
	}}

	got := eligibilityCheckOf(t, pages, observations.read)

	const first = "c-000001: memory 21 is not pinned and has no topic_key, so sync does not refresh this page; the page predates memorycard identity and is refreshed only after the one-time vault migration; pin memory 21 in memorycard so sync keeps it current after it"
	const third = "c-000003: memory 23 is not pinned and its topic_key \"sdd/x/tasks\" is not a curated topic, so sync does not refresh this page; the page predates memorycard identity and is refreshed only after the one-time vault migration; pin memory 23 in memorycard so sync keeps it current after it"
	if got.Status != CheckWarned || got.Detail != first+"; "+third {
		t.Errorf("promoted-pages-eligible = %+v, want a warning with %q then %q", got, first, third)
	}
}

// The observations are asked for once, for the project of the call, and not at all when there is no page to
// judge.
func TestDoctorAsksForTheObservationsOfTheProjectOnceAndOnlyWhenAPageNeedsThem(t *testing.T) {
	t.Run("a page names an observation", func(t *testing.T) {
		pages := newFakePages(
			promotedFrom(t, memory.Observation{ID: 21}, "c-000001"),
			promotedFrom(t, memory.Observation{ID: 22}, "c-000002"),
		)
		observations := &observationsOf{}

		eligibilityCheckOf(t, pages, observations.read)

		if len(observations.asked) != 1 || observations.asked[0] != doctorTestProject {
			t.Errorf("the observations were asked for %v, want once, for %q", observations.asked, doctorTestProject)
		}
	})
	t.Run("no page", func(t *testing.T) {
		observations := &observationsOf{err: errors.New("it must not be read")}

		got := eligibilityCheckOf(t, newFakePages(), observations.read)

		if want := (Check{Name: CheckPromotedPagesEligible, Status: CheckPassed}); got != want {
			t.Errorf("promoted-pages-eligible = %+v, want %+v", got, want)
		}
		if len(observations.asked) != 0 {
			t.Errorf("the observations were asked for %v with no page to judge", observations.asked)
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
	observations := &observationsOf{rows: []MemoryEligibility{{ID: 21, EngramID: 21}}}

	got := eligibilityCheckOf(t, pages, observations.read)

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
		name         string
		pages        PageReader
		observations func(string) ([]MemoryEligibility, error)
		want         string
	}{
		{
			name:         "the observations cannot be read",
			pages:        onePage(),
			observations: (&observationsOf{err: errors.New("database is locked")}).read,
			want:         "eligibility cannot be judged: could not read the live memories of labdrian-sdd-overlay: database is locked",
		},
		{
			name:         "no reader of the observations was wired",
			pages:        onePage(),
			observations: nil,
			want:         "eligibility cannot be judged: ops: no memory eligibility reader was wired",
		},
		{
			name:         "the pages cannot be listed",
			pages:        listFails,
			observations: (&observationsOf{}).read,
			want:         "eligibility cannot be judged: ops: list promoted pages in /fake/vault/wiki/memory: boom",
		},
		{
			name:         "no page reader was wired",
			pages:        nil,
			observations: (&observationsOf{}).read,
			want:         "eligibility cannot be judged: ops: no page reader was wired",
		},
		{
			name:         "a page reader that is a nil pointer",
			pages:        typedNilPages,
			observations: (&observationsOf{}).read,
			want:         "eligibility cannot be judged: ops: no page reader was wired",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := eligibilityCheckOf(t, tc.pages, tc.observations)

			want := Check{Name: CheckPromotedPagesEligible, Status: CheckWarned, Detail: tc.want}
			if got != want {
				t.Errorf("promoted-pages-eligible = %+v, want %+v", got, want)
			}
		})
	}
}

// A Doctor built without a reader of the observations has nothing to ask when no page names one, and says
// nothing: the missing wiring is a finding only when it stops a page being judged.
func TestDoctorWithoutAnObservationReaderPassesWhenNoPageNamesAnObservation(t *testing.T) {
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
		"frozen":              eligibilityCheckOf(t, newFakePages(promotedFrom(t, memory.Observation{ID: 21}, "c-000001")), (&observationsOf{rows: []MemoryEligibility{{ID: 21, EngramID: 21}}}).read),
		"healthy":             eligibilityCheckOf(t, newFakePages(promotedFrom(t, memory.Observation{ID: 21}, "c-000001")), (&observationsOf{rows: []MemoryEligibility{{ID: 21, EngramID: 21, Pinned: true}}}).read),
		"engram unreadable":   eligibilityCheckOf(t, newFakePages(promotedFrom(t, memory.Observation{ID: 21}, "c-000001")), (&observationsOf{err: errors.New("boom")}).read),
		"reader not wired":    eligibilityCheckOf(t, newFakePages(promotedFrom(t, memory.Observation{ID: 21}, "c-000001")), nil),
		"pages not listable":  eligibilityCheckOf(t, failing, (&observationsOf{}).read),
		"pages reader absent": eligibilityCheckOf(t, nil, (&observationsOf{}).read),
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
	deps.LiveMemoryEligibility = (&observationsOf{rows: []MemoryEligibility{{ID: 21, EngramID: 21}}}).read

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
// new one is the last. A vault whose every observation is pinned passes all of them.
func TestDoctorKeepsItsEightChecksInTheirOrderAndAddsTheEligibilityCheckLast(t *testing.T) {
	index, live := newHealthyEmbeddingDeps(t)
	deps := doctorDepsWith(t, index, live...)
	// The healthy vault's one page is promoted from observation 1.
	deps.LiveMemoryEligibility = (&observationsOf{rows: []MemoryEligibility{{ID: 1, EngramID: 1, Pinned: true}}}).read

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
	report := DoctorReport{Project: "p", Checks: []Check{{Name: CheckPromotedPagesEligible, Status: CheckWarned, Detail: "c-000001: observation 1 is frozen"}}}

	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("encode the report: %v", err)
	}

	const want = `{"project":"p","checks":[{"name":"promoted-pages-eligible","status":"warn","detail":"c-000001: observation 1 is frozen"}]}`
	if string(encoded) != want {
		t.Errorf("the report encodes as\n%s\nwant\n%s", encoded, want)
	}
	if CheckWarned != "warn" {
		t.Errorf("CheckWarned = %q, want %q", CheckWarned, "warn")
	}
}
