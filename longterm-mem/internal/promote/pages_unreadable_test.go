package promote

import (
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

	result, err := w.Promote(t.Context(), obsMemory(pageObservation(1)), false)

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

	result, err := w.Promote(t.Context(), obsMemory(three), false)

	if err != nil || result.Action.Kind != ActionUpdated || result.Page.Address != "c-000003" {
		t.Fatalf("Promote = (%+v, %v), want c-000003 updated", result, err)
	}
}
