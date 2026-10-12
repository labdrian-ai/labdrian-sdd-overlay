package promote

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// One page of a vault that cannot be read (a link to nothing, a link to a directory) is skipped and named, as the
// doctor names it, and the rest of the vault is promoted. Which observation the unreadable page belongs to is
// not known, because its frontmatter cannot be read: so an observation with no readable page of its own is left
// alone, with nothing written for it, since writing one would make a second page for an observation that may
// already have one; an observation with a readable page is promoted as ever. These tests hold that decision.

// errDangling is what the file system answers for the page that cannot be read.
var errDangling = fmt.Errorf("open memory-vault/wiki/memory/c-000900.md: %w", fs.ErrNotExist)

const unreadableAddress = "c-000900"

// pageCarrying is the page promoted under address for observation id of project at revision.
func pageCarrying(id int64, project, address string, revision int) string {
	return fmt.Sprintf("---\naddress: %s\nengram_id: %d\nengram_revision: %d\nproject: %s\n---\nbody\n", address, id, revision, project)
}

// contentOf is the page as it is on disk.
func contentOf(page Page) string { return page.Frontmatter + page.Body }

// pagesWithOneUnreadable are the pages of a vault in which c-000900 cannot be read, c-000002 is the current page
// of observation 2 and c-000003 the page of observation 3 as it was at revision 1.
func pagesWithOneUnreadable(t *testing.T) (*memPages, memory.Observation, memory.Observation) {
	t.Helper()
	two := pageObservation(2)
	three := pageObservation(3)
	pages := newMemPages(map[string]string{
		unreadableAddress: "unreadable",
		"c-000002":        contentOf(renderedPage(t, two, "c-000002")),
		"c-000003":        contentOf(renderedPage(t, three, "c-000003")),
	})
	pages.readErrs = map[string]error{unreadableAddress: errDangling}
	return pages, two, three
}

func TestActionKindOfAnObservationLeftAloneForAnUnreadablePageIsNamed(t *testing.T) {
	if got := ActionSkippedUnreadablePages.String(); got != "skipped_unreadable_pages" {
		t.Errorf("String() = %q, want skipped_unreadable_pages", got)
	}
	if ActionSkippedUnreadablePages == ActionNone || ActionSkippedUnreadablePages == ActionSkippedLocalEdit {
		t.Errorf("the kind must be its own: it is not a write, and not a refusal of an edit")
	}
}

// The scan keeps the page it could not read apart from the ones it matched, and names it with the cause.
func TestScanPromotedPagesSkipsAPageItCannotReadAndNamesIt(t *testing.T) {
	pages := newMemPages(map[string]string{
		"c-000001": pageCarrying(7, "p", "c-000001", 1),
		"c-000002": "unreadable",
		"c-000003": pageCarrying(8, "p", "c-000003", 1),
		"c-000004": "unreadable too",
	})
	pages.readErrs = map[string]error{"c-000002": errTestPages, "c-000004": errDangling}

	scan, err := scanPromotedPages(pages, 7)

	if err != nil {
		t.Fatalf("scanPromotedPages: %v", err)
	}
	if len(scan.matches) != 1 || scan.matches[0].File != "c-000001.md" {
		t.Errorf("matches = %+v, want c-000001.md alone", scan.matches)
	}
	if len(scan.unreadable) != 2 || scan.unreadable[0].File != "c-000002.md" || scan.unreadable[1].File != "c-000004.md" {
		t.Fatalf("unreadable = %+v, want c-000002.md then c-000004.md, in the order the port lists them", scan.unreadable)
	}
	if !errors.Is(scan.unreadable[0].Err, errTestPages) || !errors.Is(scan.unreadable[1].Err, errDangling) {
		t.Errorf("unreadable = %+v, want each page's own cause kept", scan.unreadable)
	}
}

func TestUnreadablePageIsNamedInTheDoctorsWords(t *testing.T) {
	page := UnreadablePage{File: "c-000900.md", Err: errTestPages}
	if got, want := page.String(), "promoted page c-000900.md could not be read: boom"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// The listing of the pages is not a page: a directory that cannot be listed, and a port that was not wired,
// stay errors, because there is no per-page result to salvage.
func TestUnreadablePagesOfAVaultWhoseListingFailsAreNoPages(t *testing.T) {
	pages := newMemPages(map[string]string{"c-000001": "x"})
	pages.listErr = errTestPages
	if got := UnreadablePages(pages); len(got) != 0 {
		t.Errorf("UnreadablePages = %+v, want none: the listing failure is the per-observation failure it always was", got)
	}
	var typedNil *memPages
	if got := UnreadablePages(typedNil); len(got) != 0 {
		t.Errorf("UnreadablePages of a nil pointer = %+v, want none", got)
	}
	if got := UnreadablePages(nil); len(got) != 0 {
		t.Errorf("UnreadablePages of no port = %+v, want none", got)
	}
}

func TestUnreadablePagesNamesEveryPageThePortCouldNotRead(t *testing.T) {
	pages, _, _ := pagesWithOneUnreadable(t)

	got := UnreadablePages(pages)

	if len(got) != 1 || got[0].File != unreadableAddress+".md" || !errors.Is(got[0].Err, errDangling) {
		t.Fatalf("UnreadablePages = %+v, want %s.md with its cause", got, unreadableAddress)
	}
}

// The survey of the unreadable pages is one pass over the vault: it lists the pages once and reads each of them
// once, whatever they hold. (A lookup of an observation's page is a pass of its own; a run makes one survey and
// one lookup per observation, and a snapshot shared among them would be stale once the run had written a page.)
func TestUnreadablePagesReadsEachPageOnce(t *testing.T) {
	pages, _, _ := pagesWithOneUnreadable(t)

	UnreadablePages(pages)

	for _, address := range []string{unreadableAddress, "c-000002", "c-000003"} {
		if got := pages.reads[address]; got != 1 {
			t.Errorf("page %s was read %d times by the survey, want once", address, got)
		}
	}
}

// A page that is readable and carries the observation is found whatever else cannot be read: the unreadable
// page is not its page, or the observation would have two.
func TestFindPromotedPageFindsTheReadablePageBesideAnUnreadableOne(t *testing.T) {
	pages, _, _ := pagesWithOneUnreadable(t)

	page, ok, err := findPromotedPage(pages, pagesProject, 2)

	if err != nil || !ok || page.Address != "c-000002" {
		t.Fatalf("findPromotedPage = (%+v, %v, %v), want c-000002 found", page, ok, err)
	}
}

// No readable page and one that cannot be read: it cannot be told whether the observation is promoted.
func TestFindPromotedPageCannotTellWhenOnlyAnUnreadablePageMightBeTheObservations(t *testing.T) {
	pages, _, _ := pagesWithOneUnreadable(t)

	_, ok, err := findPromotedPage(pages, pagesProject, 1)

	var unknown *unreadablePagesError
	if ok || !errors.As(err, &unknown) {
		t.Fatalf("findPromotedPage = (_, %v, %v), want the state unknown, naming the unreadable page", ok, err)
	}
	if len(unknown.pages) != 1 || unknown.pages[0].File != unreadableAddress+".md" {
		t.Errorf("pages = %+v, want %s.md", unknown.pages, unreadableAddress)
	}
	for _, want := range []string{"no readable page belongs to this observation", "promoted page c-000900.md could not be read", errDangling.Error()} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to say %q", err, want)
		}
	}
}

// With nothing unreadable and no page, the observation is not promoted: the answer it always was.
func TestFindPromotedPageOfAnObservationNoPageCarriesIsNotPromoted(t *testing.T) {
	pages := newMemPages(map[string]string{"c-000002": pageCarrying(2, pagesProject, "c-000002", 1)})

	_, ok, err := findPromotedPage(pages, pagesProject, 1)

	if ok || err != nil {
		t.Fatalf("findPromotedPage = (_, %v, %v), want (false, nil)", ok, err)
	}
}

// A page left behind by an observation that moved is looked for among the pages that can be read; one that
// cannot be read is not a failure of the move, and has no address to supersede.
func TestFindMovedPagesIgnoresAPageItCannotRead(t *testing.T) {
	pages := newMemPages(map[string]string{
		"c-000001": pageCarrying(7, "old-project", "c-000001", 1),
		"c-000002": "unreadable",
	})
	pages.readErrs = map[string]error{"c-000002": errDangling}

	moved, err := findMovedPages(pages, "new-project", 7)

	if err != nil || len(moved) != 1 || moved[0].Address != "c-000001" {
		t.Fatalf("findMovedPages = (%+v, %v), want c-000001 alone", moved, err)
	}
}

// An observation whose page cannot be told from the unreadable one is left alone: no address is taken, nothing
// is persisted or written, and the diagnostic says which page and why.
func TestPromoteLeavesAnObservationAloneWhenOnlyAnUnreadablePageMightBeItsPage(t *testing.T) {
	pages, _, _ := pagesWithOneUnreadable(t)
	w, repo, allocator := pagesWriter(t, pages)

	result, err := w.Promote(t.Context(), pageObservation(1), false)

	if err != nil {
		t.Fatalf("Promote: %v, want the observation reported as left alone", err)
	}
	if result.Action.Kind != ActionSkippedUnreadablePages || result.Page.Address != "" {
		t.Fatalf("result = %+v, want ActionSkippedUnreadablePages and no page", result)
	}
	diagnostic := result.Action.Diagnostic
	if diagnostic == nil || diagnostic.Rule != "unreadable-pages" {
		t.Fatalf("diagnostic = %+v, want rule unreadable-pages", diagnostic)
	}
	for _, want := range []string{"observation 1 was not promoted", "promoted page c-000900.md could not be read", errDangling.Error(), "nothing was written for it"} {
		if !strings.Contains(diagnostic.Detail, want) {
			t.Errorf("detail = %q, want it to say %q", diagnostic.Detail, want)
		}
	}
	if allocator.asked != 0 || repo.saves != 0 || len(pages.writes) != 0 {
		t.Errorf("allocator asked %d times, precedence saved %d times, writes = %+v, want nothing", allocator.asked, repo.saves, pages.writes)
	}
}

// An observation with a readable page is promoted beside an unreadable one.
func TestPromoteUpdatesAReadablePageBesideAnUnreadableOne(t *testing.T) {
	pages, _, three := pagesWithOneUnreadable(t)
	w, _, _ := pagesWriter(t, pages)
	first := renderedPage(t, three, "c-000003")
	w.Store.Set("c-000003", entryFor(first))
	three.RevisionCount = 2
	three.Content = "Revised body."

	result, err := w.Promote(t.Context(), three, false)

	if err != nil || result.Action.Kind != ActionUpdated || result.Page.Address != "c-000003" {
		t.Fatalf("Promote = (%+v, %v), want c-000003 updated", result, err)
	}
}

// Sync steps over the page it cannot read, names it, promotes what it can and leaves the rest alone: nothing
// fails, and the index and the sync state are those of a run that finished.
func TestSyncSkipsAnUnreadablePageAndLeavesAloneTheObservationsItMightBelongTo(t *testing.T) {
	pages, two, three := pagesWithOneUnreadable(t)
	w, _, allocator := pagesWriter(t, pages)
	w.Store.Set("c-000003", entryFor(renderedPage(t, three, "c-000003")))
	three.RevisionCount = 2
	three.Content = "Revised body."
	ineligible := memory.Observation{ID: 4, Type: "discovery", Title: "Process note", Content: "Not curated.", Project: pagesProject, RevisionCount: 1, TopicKey: "sdd/some-change/progress"}
	rebuilt := false
	deps := Deps{Memory: &fakeMemory{live: []memory.Observation{pageObservation(1), two, three, ineligible}}, Writer: w, Index: IndexRebuilderFunc(func(context.Context) error { rebuilt = true; return nil })}

	report, err := Sync(t.Context(), deps, pagesProject)

	if err != nil {
		t.Fatalf("Sync: %v, want a run that finished", err)
	}
	if len(report.Failed) != 0 {
		t.Errorf("Failed = %+v, want none: leaving an observation alone is not failing it", report.Failed)
	}
	if len(report.Withheld) != 1 || report.Withheld[0] != 1 {
		t.Errorf("Withheld = %v, want [1]", report.Withheld)
	}
	if len(report.Promoted) != 1 || report.Promoted[0].Page.Address != "c-000003" || report.Skipped != 2 {
		t.Errorf("Promoted = %+v, Skipped = %d, want c-000003 promoted and the current and the ineligible skipped", report.Promoted, report.Skipped)
	}
	if len(report.Unreadable) != 1 || report.Unreadable[0].File != unreadableAddress+".md" {
		t.Errorf("Unreadable = %+v, want %s.md named", report.Unreadable, unreadableAddress)
	}
	if allocator.asked != 0 {
		t.Errorf("the allocator was asked %d times, want none: no second page is made for an observation that may have one", allocator.asked)
	}
	if !rebuilt {
		t.Errorf("the index was not rebuilt: the run finished")
	}
}

// A page that becomes unreadable between the run's decision and the promotion itself is the same case, found by
// the promotion: the observation is left alone, and the run does not count it as promoted.
func TestSyncLeavesAloneAnObservationWhoseDecisionTheUnreadablePageChanged(t *testing.T) {
	// The page of another observation, readable when the run surveys the pages and when it decides (its first two
	// reads) and not when the promotion reads them.
	pages := newMemPages(map[string]string{"c-000010": pageCarrying(99, pagesProject, "c-000010", 1)})
	pages.readHook = func(address string, nth int) error {
		if nth >= 3 {
			return errDangling
		}
		return nil
	}
	w, repo, allocator := pagesWriter(t, pages)

	report, err := Sync(t.Context(), Deps{Memory: &fakeMemory{live: []memory.Observation{pageObservation(1)}}, Writer: w}, pagesProject)

	if err != nil || len(report.Failed) != 0 {
		t.Fatalf("Sync = (%+v, %v), want a run that finished", report, err)
	}
	if len(report.Promoted) != 0 || len(report.Withheld) != 1 || report.Withheld[0] != 1 {
		t.Errorf("Promoted = %+v, Withheld = %v, want observation 1 left alone and not counted as promoted", report.Promoted, report.Withheld)
	}
	if allocator.asked != 0 || repo.saves != 0 || len(pages.writes) != 0 {
		t.Errorf("allocator asked %d times, precedence saved %d times, writes = %+v, want nothing", allocator.asked, repo.saves, pages.writes)
	}
}

// The plan predicts the run: the same observations promoted, the same left alone, the same page named.
func TestPlanPredictsWhatSyncDoesBesideAnUnreadablePage(t *testing.T) {
	pages, two, three := pagesWithOneUnreadable(t)
	w, _, _ := pagesWriter(t, pages)
	three.RevisionCount = 2
	live := []memory.Observation{pageObservation(1), two, three}
	// The propagation pass walks every observation, ineligible ones too: this one has no page that can be told
	// from the unreadable one, which only that pass sees.
	ineligible := memory.Observation{ID: 4, Type: "discovery", Title: "Process note", Content: "Not curated.", Project: pagesProject, RevisionCount: 1, TopicKey: "sdd/some-change/progress"}
	deps := Deps{Memory: &fakeMemory{live: live, all: append(append([]memory.Observation(nil), live...), ineligible)}, Writer: w}

	plan, err := Plan(t.Context(), deps, pagesProject)

	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(plan.Failed) != 0 {
		t.Errorf("Failed = %+v, want none", plan.Failed)
	}
	if plan.WouldPromote != 1 || plan.Skipped != 1 || len(plan.Titles) != 1 {
		t.Errorf("WouldPromote = %d, Skipped = %d, Titles = %v, want observation 3 alone promoted and observation 2 skipped", plan.WouldPromote, plan.Skipped, plan.Titles)
	}
	if len(plan.Withheld) != 2 || plan.Withheld[0] != 1 || plan.Withheld[1] != 4 {
		t.Errorf("Withheld = %v, want [1 4]: observation 1 once although both passes left it alone, then the one only the propagation pass sees", plan.Withheld)
	}
	if len(plan.Unreadable) != 1 || plan.Unreadable[0].File != unreadableAddress+".md" {
		t.Errorf("Unreadable = %+v, want %s.md named", plan.Unreadable, unreadableAddress)
	}
	if len(pages.writes) != 0 {
		t.Errorf("writes = %+v: a plan writes nothing", pages.writes)
	}
}

// Propagate patches the page it can read, and leaves alone, without failing it, the observation whose page it
// cannot tell from the unreadable one.
func TestPropagatePatchesWhatItCanReadBesideAnUnreadablePage(t *testing.T) {
	pages, two, _ := pagesWithOneUnreadable(t)
	w, repo, _ := pagesWriter(t, pages)
	w.Store.Set("c-000002", entryFor(renderedPage(t, two, "c-000002")))
	two.DeletedAt = "2026-08-02 00:00:00"
	gone := pageObservation(6)
	gone.DeletedAt = "2026-08-02 00:00:00"
	deps := Deps{Memory: &fakeMemory{all: []memory.Observation{two, gone}}, Writer: w}

	report, err := Propagate(t.Context(), deps, pagesProject)

	if err != nil {
		t.Fatalf("Propagate: %v, want a run that finished", err)
	}
	if len(report.Patched) != 1 || report.Patched[0] != "c-000002" || len(report.Failed) != 0 {
		t.Errorf("Patched = %v, Failed = %+v, want c-000002 patched and nothing failed", report.Patched, report.Failed)
	}
	if len(report.Withheld) != 1 || report.Withheld[0] != 6 {
		t.Errorf("Withheld = %v, want [6]", report.Withheld)
	}
	if repo.saves != 1 {
		t.Errorf("the precedence store was saved %d times, want once", repo.saves)
	}
}

// The status of a superseded observation lands even when its successor's page cannot be told from the unreadable
// one: the link is a convenience, the status is the warning, and a later run adds the link.
func TestPropagateSupersedesWithoutALinkWhenTheSuccessorsPageIsUnreadable(t *testing.T) {
	pages, _, _ := pagesWithOneUnreadable(t)
	older := pageObservation(2)
	older.SyncID, older.CreatedAt = "sync-old", "2026-08-01 00:00:00"
	newer := pageObservation(9)
	newer.SyncID, newer.CreatedAt = "sync-new", "2026-08-05 00:00:00"
	w, _, _ := pagesWriter(t, pages)
	w.Store.Set("c-000002", entryFor(renderedPage(t, older, "c-000002")))
	mem := &fakeMemory{all: []memory.Observation{older, newer}, edges: map[int64][]memory.Edge{2: {{Relation: "supersedes", SourceSyncID: "sync-new", TargetSyncID: "sync-old"}}}}

	report, err := Propagate(t.Context(), Deps{Memory: mem, Writer: w}, pagesProject)

	if err != nil || len(report.Failed) != 0 || len(report.Patched) != 1 || report.Patched[0] != "c-000002" {
		t.Fatalf("Propagate = (%+v, %v), want c-000002 superseded and nothing failed", report, err)
	}
	if len(report.Withheld) != 1 || report.Withheld[0] != 9 {
		t.Errorf("Withheld = %v, want [9]: the newer observation has no page that can be told from the unreadable one", report.Withheld)
	}
	patched := pages.pages["c-000002"]
	if !strings.Contains(patched, "status: superseded") || !strings.Contains(patched, "related: []") {
		t.Errorf("the older page = %q, want it superseded with no link", patched)
	}
}

// A pages directory that cannot be listed is not a page: every observation fails as it always did, and nothing is
// left alone for want of a page that was never named.
func TestSyncStillFailsEveryObservationWhenThePagesCannotBeListed(t *testing.T) {
	pages := newMemPages(nil)
	pages.listErr = errTestPages
	w, _, _ := pagesWriter(t, pages)
	deps := Deps{Memory: &fakeMemory{live: []memory.Observation{pageObservation(1), pageObservation(2)}}, Writer: w}

	report, err := Sync(t.Context(), deps, pagesProject)

	if err == nil || len(report.Failed) != 2 || len(report.Withheld) != 0 || len(report.Unreadable) != 0 {
		t.Fatalf("Sync = (%+v, %v), want both observations failed, none left alone and no page named", report, err)
	}
	if !errors.Is(report.Failed[0].Err, errTestPages) {
		t.Errorf("failure = %v, want the listing's own cause", report.Failed[0].Err)
	}
}
