package promote

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// These tests hold promotion to the port it owns for the pages: what it asks of them, in what order, what it
// says when they fail, and what it does when none was wired. They run against memPages, which tells what it was
// asked and fails on demand; the pages on a disk are the adapter's (internal/vaultfs) and the commands' goldens.

var errTestPages = errors.New("boom")

const pagesProject = "labdrian-sdd-overlay"

// pageObservation is an observation a Writer promotes to the page testAddress.
func pageObservation(id int64) memory.Observation {
	return memory.Observation{ID: id, Type: "decision", Title: "Through The Port", Content: "Body.", Project: pagesProject, RevisionCount: 1, TopicKey: "longterm-mem/port-fixture"}
}

// pagesWriter is a Writer over pages whose other ports are fakes; the vault root is a temporary directory,
// which the catalog and the log are still written under.
func pagesWriter(t *testing.T, pages PageRepository) (*Writer, *memPrecedence, *countingAddresses) {
	t.Helper()
	repo := &memPrecedence{}
	allocator := &countingAddresses{AddressAllocator: staticAddress(testAddress)}
	return &Writer{VaultRoot: t.TempDir(), Pages: pages, Store: PrecedenceStore{}, Precedence: repo, Clock: &fakeClock{at: testInstant}, AddressMap: &memAddressMap{}, Addresses: allocator}, repo, allocator
}

// renderedPage is the page of obs under address, as a promotion writes it.
func renderedPage(t *testing.T, obs memory.Observation, address string) Page {
	t.Helper()
	page, err := EmitPage(obs, address, nil, testInstant)
	if err != nil {
		t.Fatalf("EmitPage: %v", err)
	}
	return page
}

// A page that is not there yet is created through the port, whole, after the port was asked whether it exists:
// no file of the pages is touched by the promotion itself.
func TestPromote_CreatesThePageThroughThePort(t *testing.T) {
	pages := newMemPages(nil)
	w, _, _ := pagesWriter(t, pages)

	result, err := w.Promote(t.Context(), pageObservation(501), false)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}

	if result.Action.Kind != ActionCreated {
		t.Fatalf("action = %v, want ActionCreated", result.Action.Kind)
	}
	if len(pages.exists) != 1 || pages.exists[0] != testAddress {
		t.Errorf("PageExists was asked about %v, want [%s]", pages.exists, testAddress)
	}
	want := pageWrite{address: testAddress, content: result.Page.Frontmatter + result.Page.Body}
	if len(pages.writes) != 1 || pages.writes[0] != want {
		t.Errorf("writes = %+v, want exactly %+v", pages.writes, want)
	}
	if _, err := os.Stat(filepath.Join(w.VaultRoot, "wiki", "memory")); !os.IsNotExist(err) {
		t.Errorf("the promotion made the pages directory itself (stat err = %v): the pages are the port's", err)
	}
}

// A page that is there is updated through the port: read, compared with what the sidecar remembers, and
// replaced whole.
func TestPromote_UpdatesThePageThroughThePort(t *testing.T) {
	obs := pageObservation(502)
	first := renderedPage(t, obs, testAddress)
	pages := newMemPages(map[string]string{testAddress: first.Frontmatter + first.Body})
	w, _, _ := pagesWriter(t, pages)
	w.Store.Set(testAddress, entryFor(first))
	// Another revision of the observation, found under the address its page already has.
	obs.RevisionCount = 2
	obs.Content = "Revised body."

	result, err := w.Promote(t.Context(), obs, false)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}

	if result.Action.Kind != ActionUpdated || result.Page.Address != testAddress {
		t.Fatalf("result = %+v, want ActionUpdated of %s", result, testAddress)
	}
	if pages.reads[testAddress] == 0 {
		t.Errorf("the page was never read through the port")
	}
	if len(pages.writes) != 1 || pages.writes[0].address != testAddress || !strings.Contains(pages.writes[0].content, "Revised body.") {
		t.Errorf("writes = %+v, want the revised page written once under %s", pages.writes, testAddress)
	}
}

// A page that cannot be found out to be there or not is not created over: the failure names the page's
// location and nothing is persisted or written.
func TestPromote_AFailureToTellWhetherThePageIsThereWritesNothing(t *testing.T) {
	pages := newMemPages(nil)
	pages.existsErr = errTestPages
	w, repo, _ := pagesWriter(t, pages)

	_, err := w.Promote(t.Context(), pageObservation(503), false)

	want := "promote: stat " + pages.PageLocation(testAddress) + ": boom"
	if err == nil || err.Error() != want {
		t.Fatalf("Promote error = %v, want %q", err, want)
	}
	if !errors.Is(err, errTestPages) {
		t.Errorf("the cause is lost: %v", err)
	}
	if len(pages.writes) != 0 || repo.saves != 0 {
		t.Errorf("writes = %+v, precedence saves = %d, want neither", pages.writes, repo.saves)
	}
}

// A Writer built without its pages is refused before anything is written and before an address is spent, and
// says which port it lacks; so is a patch of the pages.
func TestWriterWithoutPagesIsRefusedBeforeAnythingIsDone(t *testing.T) {
	var typedNil *memPages
	for name, pages := range map[string]PageRepository{"absent": nil, "a nil pointer": typedNil} {
		t.Run(name, func(t *testing.T) {
			w, repo, allocator := pagesWriter(t, pages)

			if _, err := w.Promote(t.Context(), pageObservation(504), false); !errors.Is(err, errNoPageRepository) {
				t.Errorf("Promote error = %v, want errNoPageRepository", err)
			}
			if allocator.asked != 0 || repo.saves != 0 {
				t.Errorf("allocator asked %d times, precedence saved %d times, want neither", allocator.asked, repo.saves)
			}
			if _, err := Sync(t.Context(), Deps{Memory: &fakeMemory{}, Writer: w}, pagesProject); !errors.Is(err, errNoPageRepository) {
				t.Errorf("Sync error = %v, want errNoPageRepository", err)
			}
			if _, err := Propagate(t.Context(), Deps{Memory: &fakeMemory{}, Writer: w}, pagesProject); !errors.Is(err, errNoPageRepository) {
				t.Errorf("Propagate error = %v, want errNoPageRepository", err)
			}
		})
	}
}

// The plan of a sync has no refusal of its own: a Writer without pages makes each observation's promoted state
// unreadable, which the plan reports as it reports any other failure.
func TestPlanWithoutPagesReportsEachObservationItCouldNotCheck(t *testing.T) {
	w, _, _ := pagesWriter(t, nil)
	mem := &fakeMemory{live: []memory.Observation{pageObservation(505)}}

	plan, err := Plan(t.Context(), Deps{Memory: mem, Writer: w}, pagesProject)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if len(plan.Failed) != 1 || !errors.Is(plan.Failed[0].Err, errNoPageReader) {
		t.Fatalf("Failed = %+v, want the one observation, unreadable for want of pages", plan.Failed)
	}
}

// The scan reads every page the port lists, once and in its order, and keeps those that carry the engram_id.
func TestScanPromotedPagesReadsEveryListedPageOnceAndKeepsTheMatches(t *testing.T) {
	carries := func(id int, project, address string) string {
		return fmt.Sprintf("---\naddress: %s\nengram_id: %d\nproject: %s\n---\nbody\n", address, id, project)
	}
	pages := newMemPages(map[string]string{
		"c-000001": carries(7, "p", "c-000001"),
		"c-000002": "no frontmatter at all\n",
		"c-000003": carries(8, "p", "c-000003"),
		"c-000004": carries(7, "q", "c-000004"),
	})

	scan, err := scanPromotedPages(pages, 7)
	if err != nil {
		t.Fatalf("scanPromotedPages: %v", err)
	}

	matches := scan.matches
	if len(matches) != 2 || matches[0].File != "c-000001.md" || matches[1].File != "c-000004.md" || matches[1].Project != "q" {
		t.Fatalf("matches = %+v, want c-000001.md then c-000004.md (project q)", matches)
	}
	for address := range pages.pages {
		if pages.reads[address] != 1 {
			t.Errorf("%s was read %d times, want once", address, pages.reads[address])
		}
	}
}

// The scan's failures name what failed in the words promotion has always used, around the cause the port gave.
// A page that cannot be read is not among them: it is skipped and named, not a failure of the scan
// (TestScanPromotedPagesSkipsAPageItCannotReadAndNamesIt), since the owner's decision of 2026-10-10.
func TestScanPromotedPagesNamesWhatItCouldNotList(t *testing.T) {
	t.Run("list", func(t *testing.T) {
		pages := newMemPages(nil)
		pages.listErr = errTestPages

		_, err := scanPromotedPages(pages, 7)

		if want := "promote: list " + pages.PagesLocation() + ": boom"; err == nil || err.Error() != want {
			t.Fatalf("error = %v, want %q", err, want)
		}
	})
	t.Run("no reader", func(t *testing.T) {
		var typedNil *memPages
		for name, reader := range map[string]PageReader{"absent": nil, "a nil pointer": typedNil} {
			if _, err := scanPromotedPages(reader, 7); !errors.Is(err, errNoPageReader) {
				t.Errorf("%s: error = %v, want errNoPageReader", name, err)
			}
		}
	})
}

// Reconcile reads the page it adopts through the port, answers a page that is not there with ErrPageNotFound
// whatever the cause is called, and words any other failure around the page's location.
func TestReconcileReadsThePageThroughThePort(t *testing.T) {
	obs := pageObservation(506)
	page := renderedPage(t, obs, testAddress)

	t.Run("adopts", func(t *testing.T) {
		pages := newMemPages(map[string]string{testAddress: page.Frontmatter + page.Body})
		repo := &memPrecedence{}

		outcome, err := Reconcile(pages, pagesProject, testAddress, repo)

		if err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if !outcome.Adopted || outcome.Path != "wiki/memory/"+testAddress+".md" || outcome.PromotedRevision != 1 {
			t.Errorf("outcome = %+v, want the page adopted at revision 1 under wiki/memory/%s.md", outcome, testAddress)
		}
		if _, ok := repo.persisted.Get(testAddress); !ok {
			t.Errorf("the adopted entry was not persisted: %+v", repo.persisted)
		}
		if len(pages.writes) != 0 {
			t.Errorf("reconcile wrote %+v: it only reads the page", pages.writes)
		}
	})
	t.Run("a page that is not there", func(t *testing.T) {
		_, err := Reconcile(newMemPages(nil), pagesProject, testAddress, &memPrecedence{})
		if !errors.Is(err, ErrPageNotFound) {
			t.Fatalf("error = %v, want ErrPageNotFound", err)
		}
	})
	t.Run("a page that cannot be read", func(t *testing.T) {
		pages := newMemPages(map[string]string{testAddress: page.Frontmatter + page.Body})
		pages.readErrs = map[string]error{testAddress: errTestPages}

		_, err := Reconcile(pages, pagesProject, testAddress, &memPrecedence{})

		if want := "promote: read " + pages.PageLocation(testAddress) + ": boom"; err == nil || err.Error() != want || errors.Is(err, ErrPageNotFound) {
			t.Fatalf("error = %v, want %q, which is not ErrPageNotFound", err, want)
		}
	})
	t.Run("a page with no frontmatter names its location", func(t *testing.T) {
		pages := newMemPages(map[string]string{testAddress: "text only\n"})

		_, err := Reconcile(pages, pagesProject, testAddress, &memPrecedence{})

		if !errors.Is(err, ErrUnusablePage) || !strings.Contains(err.Error(), pages.PageLocation(testAddress)+" has no parseable frontmatter block") {
			t.Fatalf("error = %v, want ErrUnusablePage naming %s", err, pages.PageLocation(testAddress))
		}
	})
	t.Run("no reader", func(t *testing.T) {
		var typedNil *memPages
		for name, reader := range map[string]PageReader{"absent": nil, "a nil pointer": typedNil} {
			if _, err := Reconcile(reader, pagesProject, testAddress, &memPrecedence{}); !errors.Is(err, errNoPageReader) {
				t.Errorf("%s: error = %v, want errNoPageReader", name, err)
			}
		}
	})
	t.Run("an address of the wrong shape never reaches the pages", func(t *testing.T) {
		pages := newMemPages(nil)

		_, err := Reconcile(pages, pagesProject, "../escape", &memPrecedence{})

		if !errors.Is(err, ErrInvalidAddress) || len(pages.reads) != 0 || len(pages.exists) != 0 {
			t.Fatalf("error = %v, reads = %v, exists = %v, want ErrInvalidAddress and no call of the pages", err, pages.reads, pages.exists)
		}
	})
}

// wikilinkRuleOf is what the wikilink rule of the page check reports for a page that links to each address.
func wikilinkRuleOf(pages PageExistence, links ...string) []Diagnostic {
	body := "# Page\n"
	for _, address := range links {
		body += "[[" + address + "|Linked]]\n"
	}
	page := Page{Address: testAddress, Path: "wiki/memory/" + testAddress + ".md", Frontmatter: "---\naddress: " + testAddress + "\n---\n", Body: body}
	var found []Diagnostic
	for _, d := range LintPage(page, pages, &memAddressMap{}, nil) {
		if d.Rule == "wikilink-resolvability" {
			found = append(found, d)
		}
	}
	return found
}

// The wikilink rule asks the port about each link, and a link the port cannot answer for is a link that does
// not resolve.
func TestLintPageAsksThePortAboutEachWikilink(t *testing.T) {
	t.Run("a link that resolves and one that does not", func(t *testing.T) {
		pages := newMemPages(map[string]string{"c-000007": "x"})

		found := wikilinkRuleOf(pages, "c-000007", "c-000008")

		if len(found) != 1 || found[0].Detail != "[[c-000008|Linked]] does not resolve to an existing file" {
			t.Fatalf("diagnostics = %+v, want only the link to c-000008", found)
		}
		if len(pages.exists) != 2 || pages.exists[0] != "c-000007" || pages.exists[1] != "c-000008" {
			t.Errorf("PageExists was asked about %v, want both links in order", pages.exists)
		}
	})
	t.Run("a link the port cannot answer for", func(t *testing.T) {
		pages := newMemPages(map[string]string{"c-000007": "x"})
		pages.existsErr = errTestPages

		if found := wikilinkRuleOf(pages, "c-000007"); len(found) != 1 {
			t.Fatalf("diagnostics = %+v, want the link reported as not resolving", found)
		}
	})
	t.Run("no port, with a link to resolve", func(t *testing.T) {
		var typedNil *memPages
		for name, pages := range map[string]PageExistence{"absent": nil, "a nil pointer": typedNil} {
			found := wikilinkRuleOf(pages, "c-000007", "c-000008")
			if len(found) != 1 || found[0].Detail != errNoPageReader.Error() {
				t.Errorf("%s: diagnostics = %+v, want one finding naming the missing port", name, found)
			}
		}
	})
	t.Run("no port, and no link to resolve", func(t *testing.T) {
		if found := wikilinkRuleOf(nil); len(found) != 0 {
			t.Fatalf("diagnostics = %+v, want none: a page without links needs no pages", found)
		}
	})
}

// PatchStatusFields reads and writes the page through the port, writes nothing when the patch changes nothing,
// and names the page's location when it cannot.
func TestPatchStatusFieldsThroughThePort(t *testing.T) {
	page := renderedPage(t, pageObservation(507), testAddress)
	content := page.Frontmatter + page.Body

	t.Run("writes the patched page once, and nothing for a patch that changes nothing", func(t *testing.T) {
		pages := newMemPages(map[string]string{testAddress: content})

		if _, _, err := PatchStatusFields(pages, testAddress, "archived", nil); err != nil {
			t.Fatalf("PatchStatusFields: %v", err)
		}
		if len(pages.writes) != 1 || !strings.Contains(pages.writes[0].content, "status: archived") || !strings.HasSuffix(pages.writes[0].content, page.Body) {
			t.Fatalf("writes = %+v, want the page written once with its status archived and its body as it was", pages.writes)
		}
		if _, _, err := PatchStatusFields(pages, testAddress, "archived", nil); err != nil {
			t.Fatalf("PatchStatusFields (again): %v", err)
		}
		if len(pages.writes) != 1 {
			t.Errorf("a patch that changed nothing wrote the page again: %d writes", len(pages.writes))
		}
	})
	t.Run("a page that cannot be read", func(t *testing.T) {
		pages := newMemPages(map[string]string{testAddress: content})
		pages.readErrs = map[string]error{testAddress: errTestPages}

		_, _, err := PatchStatusFields(pages, testAddress, "archived", nil)

		if want := "promote: read " + pages.PageLocation(testAddress) + ": boom"; err == nil || err.Error() != want {
			t.Fatalf("error = %v, want %q", err, want)
		}
	})
	t.Run("a page with no frontmatter is not written", func(t *testing.T) {
		pages := newMemPages(map[string]string{testAddress: "text only\n"})

		_, _, err := PatchStatusFields(pages, testAddress, "archived", nil)

		if want := "promote: " + pages.PageLocation(testAddress) + " has no parseable frontmatter block"; err == nil || err.Error() != want || len(pages.writes) != 0 {
			t.Fatalf("error = %v, writes = %+v, want %q and no write", err, pages.writes, want)
		}
	})
	t.Run("a write that fails is the port's failure, untouched", func(t *testing.T) {
		pages := newMemPages(map[string]string{testAddress: content})
		pages.writeErr = errTestPageWrite

		if _, _, err := PatchStatusFields(pages, testAddress, "archived", nil); err != errTestPageWrite {
			t.Fatalf("error = %v, want the port's own %v", err, errTestPageWrite)
		}
	})
	t.Run("no repository", func(t *testing.T) {
		var typedNil *memPages
		for name, pages := range map[string]PageRepository{"absent": nil, "a nil pointer": typedNil} {
			if _, _, err := PatchStatusFields(pages, testAddress, "archived", nil); !errors.Is(err, errNoPageRepository) {
				t.Errorf("%s: error = %v, want errNoPageRepository", name, err)
			}
		}
	})
}

// UpdateInPlace holds a page it cannot prove its own: nothing is written, and the refusal names the page's
// location. A read or write that fails is the port's failure, worded around the page or untouched.
func TestUpdateInPlaceThroughThePort(t *testing.T) {
	obs := pageObservation(508)
	first := renderedPage(t, obs, testAddress)
	obs.RevisionCount = 2
	obs.Content = "Revised body."
	second := renderedPage(t, obs, testAddress)

	t.Run("a page edited by hand is refused and not written", func(t *testing.T) {
		pages := newMemPages(map[string]string{testAddress: first.Frontmatter + first.Body + "\nA line a person added.\n"})
		store := PrecedenceStore{}
		store.Set(testAddress, entryFor(first))

		action, err := UpdateInPlace(pages, store, second)

		if err != nil || action.Kind != ActionSkippedLocalEdit {
			t.Fatalf("action = %+v, err = %v, want ActionSkippedLocalEdit", action, err)
		}
		if want := pages.PageLocation(testAddress) + " (address " + testAddress + ") was edited locally"; action.Diagnostic == nil || !strings.Contains(action.Diagnostic.Detail, want) {
			t.Errorf("diagnostic = %+v, want it to name %q", action.Diagnostic, want)
		}
		if len(pages.writes) != 0 {
			t.Errorf("writes = %+v, want none", pages.writes)
		}
	})
	t.Run("a page that cannot be read", func(t *testing.T) {
		pages := newMemPages(map[string]string{testAddress: first.Frontmatter + first.Body})
		pages.readErrs = map[string]error{testAddress: errTestPages}

		_, err := UpdateInPlace(pages, PrecedenceStore{}, second)

		if want := "promote: read " + pages.PageLocation(testAddress) + ": boom"; err == nil || err.Error() != want {
			t.Fatalf("error = %v, want %q", err, want)
		}
	})
	t.Run("a write that fails leaves the store as it was", func(t *testing.T) {
		pages := newMemPages(map[string]string{testAddress: first.Frontmatter + first.Body})
		pages.writeErr = errTestPageWrite
		store := PrecedenceStore{}
		store.Set(testAddress, entryFor(first))

		_, err := UpdateInPlace(pages, store, second)

		if err != errTestPageWrite {
			t.Fatalf("error = %v, want the port's own %v", err, errTestPageWrite)
		}
		if got, _ := store.Get(testAddress); got != entryFor(first) {
			t.Errorf("the store records %+v after a write that failed, want it as it was", got)
		}
	})
	t.Run("no repository", func(t *testing.T) {
		var typedNil *memPages
		for name, pages := range map[string]PageRepository{"absent": nil, "a nil pointer": typedNil} {
			if _, err := UpdateInPlace(pages, PrecedenceStore{}, second); !errors.Is(err, errNoPageRepository) {
				t.Errorf("%s: error = %v, want errNoPageRepository", name, err)
			}
		}
	})
}

// Propagate patches the pages through the port, and a page it cannot patch is a failure of that observation
// worded around the page's location, while the run goes on.
func TestPropagatePatchesThePagesThroughThePort(t *testing.T) {
	obs := pageObservation(509)
	obs.DeletedAt = "2026-08-02 00:00:00"
	page := renderedPage(t, obs, testAddress)
	mem := &fakeMemory{all: []memory.Observation{obs}}

	t.Run("patches", func(t *testing.T) {
		pages := newMemPages(map[string]string{testAddress: page.Frontmatter + page.Body})
		w, repo, _ := pagesWriter(t, pages)

		report, err := Propagate(t.Context(), Deps{Memory: mem, Writer: w}, pagesProject)

		if err != nil || len(report.Patched) != 1 || report.Patched[0] != testAddress {
			t.Fatalf("report = %+v, err = %v, want %s patched", report, err, testAddress)
		}
		if len(pages.writes) != 1 || !strings.Contains(pages.writes[0].content, "status: archived") {
			t.Errorf("writes = %+v, want the page written once, archived", pages.writes)
		}
		if repo.saves != 1 {
			t.Errorf("the precedence store was saved %d times, want once", repo.saves)
		}
	})
	t.Run("a page that cannot be read when it is patched", func(t *testing.T) {
		pages := newMemPages(map[string]string{testAddress: page.Frontmatter + page.Body})
		// The scan for the observation's page reads it first; the patch reads it again, and that read fails.
		pages.readHook = func(address string, nth int) error {
			if nth == 2 {
				return errTestPages
			}
			return nil
		}
		w, _, _ := pagesWriter(t, pages)

		report, err := Propagate(t.Context(), Deps{Memory: mem, Writer: w}, pagesProject)

		if err == nil || len(report.Failed) != 1 {
			t.Fatalf("report = %+v, err = %v, want the one observation failed", report, err)
		}
		want := "patch " + pages.PageLocation(testAddress) + ": promote: read " + pages.PageLocation(testAddress) + ": boom"
		if report.Failed[0].Err.Error() != want {
			t.Errorf("failure = %q, want %q", report.Failed[0].Err, want)
		}
		if len(pages.writes) != 0 {
			t.Errorf("writes = %+v, want none", pages.writes)
		}
	})
}

// An observation that moved to another project leaves pages behind, and every one of them is superseded. A page
// that cannot be read when its turn comes is that page's failure, worded around its location: the ones before
// and after it are still patched, and the run reports the failure.
func TestPromoteSupersedesEveryMovedPageEvenWhenOneCannotBeRead(t *testing.T) {
	old := pageObservation(510)
	old.Project = "p-old-a"
	oldA := renderedPage(t, old, "c-000001")
	old.Project = "p-old-b"
	oldB := renderedPage(t, old, "c-000002")
	pages := newMemPages(map[string]string{
		"c-000001": oldA.Frontmatter + oldA.Body,
		"c-000002": oldB.Frontmatter + oldB.Body,
	})
	// The scans read each old page first, however many there are; the supersession reads c-000001 once the new
	// page has been written, and that read fails.
	pages.readHook = func(address string, _ int) error {
		if address == "c-000001" && len(pages.writes) > 0 {
			return errTestPages
		}
		return nil
	}
	w, repo, _ := pagesWriter(t, pages)
	moved := pageObservation(510)
	moved.Project = "p-new"

	_, err := w.Promote(t.Context(), moved, false)

	want := "promote: read superseded page " + pages.PageLocation("c-000001") + ": boom"
	if err == nil || err.Error() != want {
		t.Fatalf("Promote error = %v, want %q", err, want)
	}
	if !strings.Contains(pages.pages["c-000002"], "status: superseded") {
		t.Errorf("c-000002 was not superseded although only c-000001 failed:\n%s", pages.pages["c-000002"])
	}
	if strings.Contains(pages.pages["c-000001"], "status: superseded") {
		t.Errorf("c-000001 was patched although it could not be read")
	}
	if repo.saves != 2 {
		t.Errorf("the precedence store was saved %d times, want twice: the new page's fingerprint before it was written, and the page that was patched after", repo.saves)
	}
}

// The listing of the port is what the promotion's checks act on, so a port that has nothing to list leaves a
// vault in which nothing is promoted, whatever else is on a disk.
func TestPromoteFindsNoPageTheListingDoesNotName(t *testing.T) {
	obs := pageObservation(511)
	page := renderedPage(t, obs, testAddress)
	onDisk := t.TempDir()
	if err := os.MkdirAll(filepath.Join(onDisk, "wiki", "memory"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(onDisk, "wiki", "memory", testAddress+".md"), []byte(page.Frontmatter+page.Body), 0o644); err != nil {
		t.Fatal(err)
	}
	pages := newMemPages(nil)
	w, _, _ := pagesWriter(t, pages)
	w.VaultRoot = onDisk

	result, err := w.Promote(context.Background(), obs, false)

	if err != nil || result.Action.Kind != ActionCreated {
		t.Fatalf("result = %+v, err = %v, want a create: the page on the disk of the vault root is not the port's", result, err)
	}
}
