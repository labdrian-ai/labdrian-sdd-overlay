package promote

import (
	"errors"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// The safety paths of the migration (S6 (c)): every way a page is refused, counted by kind and named, the pairings
// that must not make two pages of one memory, the ports it needs, and what a run stopped halfway leaves behind.

type fakeBridges struct {
	byEngram map[int64]memory.CardMemory
	byID     map[int64]memory.CardMemory
	err      error
}

func (f *fakeBridges) MemoryByEngramID(id int64) (memory.CardMemory, bool, error) {
	m, ok := f.byEngram[id]
	return m, ok, f.err
}

func (f *fakeBridges) MemoryByID(id int64) (memory.CardMemory, bool, error) {
	m, ok := f.byID[id]
	return m, ok, f.err
}

type fakeBackup struct {
	err   error
	calls [][]string
}

func (f *fakeBackup) BackUp(addresses []string) (string, error) {
	f.calls = append(f.calls, addresses)
	return "backup-dir", f.err
}

const migProject = "p"

type migWorld struct {
	pages  *memPages
	repo   *memPrecedence
	w      *Writer
	cards  *fakeBridges
	backup *fakeBackup
}

func cardOf(id int64, project string) memory.CardMemory {
	return memory.CardMemory{ID: id, Project: project, Type: "learning", Revision: 4}
}

func newMigWorld() *migWorld {
	m := &migWorld{pages: newMemPages(nil), repo: &memPrecedence{}, backup: &fakeBackup{}, cards: &fakeBridges{byEngram: map[int64]memory.CardMemory{}, byID: map[int64]memory.CardMemory{}}}
	m.w = &Writer{Pages: m.pages, Precedence: m.repo, Store: PrecedenceStore{}}
	return m
}

// memory registers a memory of the store: byID always, and byEngram for each observation named.
func (m *migWorld) memory(c memory.CardMemory, observations ...int64) {
	m.cards.byID[c.ID] = c
	for _, o := range observations {
		m.cards.byEngram[o] = c
	}
}

func pageText(address, identity string) string {
	return "---\ntitle: \"T\"\naddress: " + address + "\n" + identity + "project: " + migProject + "\n---\n# T\n\nBody of " + address + ".\n"
}

// put writes a page and records the precedence entry of longterm-mem's last write of it.
func (m *migWorld) put(address, identity string) string {
	content := pageText(address, identity)
	m.pages.pages[address] = content
	block, _ := frontmatterBlock(content)
	m.w.Store.Set(address, PrecedenceEntry{BodyHash: hashText(content[len(block):]), FrontmatterHash: hashText(block), PromotedRevision: 1})
	return content
}

func (m *migWorld) run(apply bool) (CardMigrationReport, error) {
	return MigrateCardPages(m.cards, m.w, m.backup, migProject, apply)
}

func refusalOf(report CardMigrationReport, address string) (MigrationRefusal, bool) {
	for _, r := range report.Refused {
		if r.Address == address {
			return r, true
		}
	}
	return MigrationRefusal{}, false
}

func TestMigrateCardPages_EachRefusalKindIsCountedAndNamed(t *testing.T) {
	m := newMigWorld()
	m.memory(cardOf(1, migProject), 11)
	m.memory(cardOf(2, migProject), 12)
	m.memory(cardOf(4, migProject))
	m.memory(cardOf(5, "other"))
	m.memory(cardOf(8, migProject), 13)
	m.put("edited", "engram_id: 11\nengram_revision: 1\n")
	m.pages.pages["edited"] += "A line a person added.\n"
	untracked := pageText("untracked", "engram_id: 11\n")
	m.pages.pages["untracked"] = untracked
	m.put("noimport", "engram_id: 99\n")
	m.put("nomemory", "memorycard_id: 77\n")
	m.put("owned", "engram_id: 12\n")
	m.put("owner", "source: memorycard\nsource_id: 2\n")
	m.put("badid", "memorycard_id: abc\n")
	m.put("zeroid", "memorycard_id: 0\n")
	m.put("elsewhere", "memorycard_id: 5\n")
	m.put("both", "engram_id: 13\nmemorycard_id: 8\n")
	m.put("fine", "memorycard_id: 4\nmemorycard_revision: 2\nmemorycard_type: decision\n")
	before := map[string]string{}
	for a, c := range m.pages.pages {
		before[a] = c
	}

	report, err := m.run(false)
	if err != nil {
		t.Fatal(err)
	}
	if report.Legacy != 10 || len(report.Migrated) != 1 || report.Migrated[0].Address != "fine" {
		t.Fatalf("legacy %d, migrated %+v, want 10 legacy pages and only fine migrated", report.Legacy, report.Migrated)
	}
	want := map[RefusalKind]int{RefusedEdited: 1, RefusedUntracked: 1, RefusedNoImport: 1, RefusedNoMemory: 1, RefusedHasOwnPage: 1, RefusedOther: 4}
	counts := report.RefusalCounts()
	for kind, n := range want {
		if counts[kind] != n {
			t.Errorf("refusals of kind %q = %d, want %d (%v)", kind, counts[kind], n, counts)
		}
	}
	for address, fragment := range map[string]string{
		"nomemory": "holds no memory 77", "elsewhere": `belongs to project "other"`, "badid": `"abc" is not a memory id`,
		"zeroid": `"0" is not a memory id`, "both": "at once", "owned": "already has the page owner",
	} {
		r, ok := refusalOf(report, address)
		if !ok || !strings.Contains(r.Reason, fragment) {
			t.Errorf("refusal of %s = %+v, want a reason containing %q", address, r, fragment)
		}
	}
	for a, c := range before {
		if m.pages.pages[a] != c {
			t.Errorf("the dry run changed %s", a)
		}
	}
	if len(m.pages.writes) != 0 || m.repo.saves != 0 || len(m.backup.calls) != 0 {
		t.Errorf("the dry run wrote %d pages, saved %d times, backed up %d times", len(m.pages.writes), m.repo.saves, len(m.backup.calls))
	}
}

// Two observations that memorycard imported into one memory: the first page takes it, the second is refused.
func TestMigrateCardPages_TwoLegacyPagesOfOneMemoryAreNotBothMigrated(t *testing.T) {
	m := newMigWorld()
	m.memory(cardOf(6, migProject), 21, 22)
	m.put("a-first", "engram_id: 21\n")
	second := m.put("b-second", "engram_id: 22\n")

	report, err := m.run(true)
	if err != nil {
		t.Fatal(err)
	}
	r, ok := refusalOf(report, "b-second")
	if len(report.Migrated) != 1 || report.Migrated[0].Address != "a-first" || !ok || r.Kind != RefusedHasOwnPage || !strings.Contains(r.Reason, "already the memory of a-first") {
		t.Fatalf("report = %+v, want a-first migrated and b-second refused as a second page of memory 6", report)
	}
	if m.pages.pages["b-second"] != second {
		t.Errorf("the refused page changed")
	}
}

// An Engram page and a memorycard page that name one memory: the memorycard page is the memory's own, the Engram
// page is refused, whatever order they come in.
func TestMigrateCardPages_AnEngramPageAndAMemorycardPageOfOneMemory(t *testing.T) {
	for _, names := range [][2]string{{"a-engram", "b-card"}, {"b-engram", "a-card"}} {
		m := newMigWorld()
		m.memory(cardOf(7, migProject), 31)
		engram, card := names[0], names[1]
		m.put(engram, "engram_id: 31\n")
		m.put(card, "memorycard_id: 7\n")

		report, err := m.run(true)
		if err != nil {
			t.Fatal(err)
		}
		r, ok := refusalOf(report, engram)
		if len(report.Migrated) != 1 || report.Migrated[0].Address != card || !ok || r.Kind != RefusedHasOwnPage {
			t.Errorf("%v: report = %+v, want %s migrated and %s refused as a second page of memory 7", names, report, card, engram)
		}
	}
}

// Two memorycard pages of one memory: neither is the one to keep, so both are refused and the owner decides.
func TestMigrateCardPages_TwoMemorycardPagesOfOneMemoryAreBothRefused(t *testing.T) {
	m := newMigWorld()
	m.memory(cardOf(7, migProject))
	m.put("a", "memorycard_id: 7\n")
	m.put("b", "memorycard_id: 7\n")
	report, err := m.run(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Migrated) != 0 || report.RefusalCounts()[RefusedHasOwnPage] != 2 {
		t.Fatalf("report = %+v, want both pages refused as pages of one memory", report)
	}
}

// A memorycard page that names a memory of the project that already has a store-agnostic page is refused.
func TestMigrateCardPages_AMemorycardPageWhoseMemoryHasAPageOfItsOwnIsRefused(t *testing.T) {
	m := newMigWorld()
	m.memory(cardOf(7, migProject))
	m.put("current", "source: memorycard\nsource_id: 7\n")
	m.put("legacy", "memorycard_id: 7\n")
	report, err := m.run(true)
	if err != nil {
		t.Fatal(err)
	}
	r, ok := refusalOf(report, "legacy")
	if len(report.Migrated) != 0 || !ok || r.Kind != RefusedHasOwnPage || !strings.Contains(r.Reason, "already has the page current") {
		t.Fatalf("report = %+v, want legacy refused because current is memory 7's page", report)
	}
}

// A page whose only legacy key is one that does not name a memory is still a legacy page, counted and refused by
// name, so that the vault can reach zero legacy keys by the owner's hand and not by a miscount.
func TestMigrateCardPages_ALegacyKeyThatNamesNoMemoryIsCountedAndRefused(t *testing.T) {
	for _, identity := range []string{"engram_sync_id: abc\n", "engram_type: learning\n", "memorycard_type: learning\n", "memorycard_revision: 3\n", "engram_revision: 3\n", "engram_sync_id:\n"} {
		m := newMigWorld()
		before := m.put("odd", identity)
		report, err := m.run(true)
		if err != nil {
			t.Fatal(err)
		}
		r, ok := refusalOf(report, "odd")
		if report.Legacy != 1 || !ok || r.Kind != RefusedOther || !strings.Contains(r.Reason, "names no memory") || m.pages.pages["odd"] != before {
			t.Errorf("%q: report = %+v, want 1 legacy page refused as naming no memory and left as it is", identity, report)
		}
	}
}

// An identity key that is indented, or whose value runs over several lines, is not rewritten whole: the page is refused.
func TestMigrateCardPages_AnIdentityTheLineRewriterCannotRewriteWholeIsRefused(t *testing.T) {
	for _, identity := range []string{"memorycard_id: 7\n  memorycard_type: learning\n", "memorycard_id: 7\nmemorycard_type: >\n  learning\n", "engram_id: 31\nengram_type:\n  - a\n"} {
		m := newMigWorld()
		m.memory(cardOf(7, migProject), 31)
		before := m.put("multi", identity)
		report, err := m.run(true)
		if err != nil {
			t.Fatal(err)
		}
		r, ok := refusalOf(report, "multi")
		if len(report.Migrated) != 0 || !ok || r.Kind != RefusedOther || !strings.Contains(r.Reason, "several lines") || m.pages.pages["multi"] != before {
			t.Errorf("%q: report = %+v, want the page refused for its identity lines and left as it is", identity, report)
		}
	}
}

func TestMigrateCardPages_APortThatWasNotWiredIsAnErrorAndNeverAPanic(t *testing.T) {
	m := newMigWorld()
	m.memory(cardOf(7, migProject))
	m.put("legacy", "memorycard_id: 7\n")
	if _, err := MigrateCardPages(nil, m.w, m.backup, migProject, false); err == nil {
		t.Errorf("a migration with no memorycard bridge answered no error")
	}
	var typed *fakeBridges
	if _, err := MigrateCardPages(typed, m.w, m.backup, migProject, false); err == nil {
		t.Errorf("a migration with a nil memorycard bridge answered no error")
	}
	m.w.Store = nil
	if _, err := m.run(true); err == nil {
		t.Errorf("a migration that must write, with no precedence store, answered no error")
	}
}

func TestMigrateCardPages_AFailedLookupIsAnErrorAndWritesNothing(t *testing.T) {
	m := newMigWorld()
	m.memory(cardOf(7, migProject))
	m.cards.err = errors.New("database is locked")
	m.put("legacy", "memorycard_id: 7\n")
	if _, err := m.run(true); err == nil || !strings.Contains(err.Error(), "database is locked") {
		t.Errorf("err = %v, want the lookup failure", err)
	}
	if len(m.pages.writes) != 0 || m.repo.saves != 0 {
		t.Errorf("a failed lookup wrote %d pages and saved %d times", len(m.pages.writes), m.repo.saves)
	}
}

// The sidecar is persisted before the page is written: a page whose write failed is then a page the next run
// recognizes as its own and finishes, and the error says so.
func TestMigrateCardPages_AFailedPageWriteIsFinishedByARerun(t *testing.T) {
	m := newMigWorld()
	m.memory(cardOf(7, migProject))
	m.put("legacy", "memorycard_id: 7\nmemorycard_type: learning\nmemorycard_revision: 2\n")
	m.pages.writeErr = errors.New("disk full")

	report, err := m.run(true)
	if err == nil || !strings.Contains(err.Error(), "disk full") || !strings.Contains(err.Error(), "rerun") || !strings.Contains(err.Error(), "backup-dir") {
		t.Fatalf("err = %v, want the write failure, the backup location and that a rerun finishes it", err)
	}
	if len(report.Migrated) != 0 || !strings.Contains(m.pages.pages["legacy"], "memorycard_id: 7") {
		t.Fatalf("report = %+v, page = %q, want nothing migrated and the page as it was", report, m.pages.pages["legacy"])
	}

	// A new process: the writer loads the sidecar the failed run persisted.
	m.pages.writeErr = nil
	store, err := m.repo.LoadPrecedence()
	if err != nil {
		t.Fatal(err)
	}
	m.w = &Writer{Pages: m.pages, Precedence: m.repo, Store: store}
	report, err = m.run(true)
	if err != nil || len(report.Migrated) != 1 || len(report.Refused) != 0 {
		t.Fatalf("rerun: report = %+v, err = %v, want the page migrated", report, err)
	}
	page := m.pages.pages["legacy"]
	block, _ := frontmatterBlock(page)
	if !strings.Contains(page, "source: memorycard\nsource_id: 7\nsource_type: learning\nsource_revision: 2\n") || strings.Contains(page, "memorycard_id") {
		t.Errorf("page = %q, want the store-agnostic keys", page)
	}
	if entry, _ := store.Get("legacy"); !entry.MatchesPage(page) || entry.FrontmatterHash != hashText(block) {
		t.Errorf("the entry %+v does not match the migrated page", entry)
	}
	// And the page that stopped the run is not a hand edit: a page someone did edit in that window is still refused.
	again, err := m.run(true)
	if err != nil || again.Legacy != 0 {
		t.Errorf("a third run: report = %+v, err = %v, want no legacy page left", again, err)
	}
}

// A failure to persist the sidecar leaves the page unwritten: the next run starts from the page as it was.
func TestMigrateCardPages_AFailedSidecarSaveLeavesThePageAsItWasAndARerunMigratesIt(t *testing.T) {
	m := newMigWorld()
	m.memory(cardOf(7, migProject))
	before := m.put("legacy", "memorycard_id: 7\n")
	m.repo.saveErr = errors.New("read-only file system")

	_, err := m.run(true)
	if err == nil || !strings.Contains(err.Error(), "read-only file system") || !strings.Contains(err.Error(), "not written") {
		t.Fatalf("err = %v, want the save failure and that the page was not written", err)
	}
	if m.pages.pages["legacy"] != before || len(m.pages.writes) != 0 {
		t.Fatalf("the page changed or was written (%d writes) although the sidecar was not saved", len(m.pages.writes))
	}
	if entry, _ := m.w.Store.Get("legacy"); !entry.MatchesPage(before) {
		t.Errorf("the entry in memory was left at %+v, want what it was", entry)
	}

	m.repo.saveErr = nil
	report, err := m.run(true)
	if err != nil || len(report.Migrated) != 1 {
		t.Fatalf("rerun: report = %+v, err = %v", report, err)
	}
}

// An entry that moved, with a page that is neither the old page nor what the rerun would write, is a hand edit.
func TestMigrateCardPages_ARecordedEntryDoesNotAdoptAPageThatIsNotTheOneItRecorded(t *testing.T) {
	m := newMigWorld()
	m.memory(cardOf(7, migProject))
	original := m.put("legacy", "memorycard_id: 7\n")
	block, _ := frontmatterBlock(original)
	entry, _ := m.w.Store.Get("legacy")
	entry.FrontmatterHash = hashText(rewrittenIdentity(block, 7, "", ""))
	m.w.Store.Set("legacy", entry)
	m.pages.pages["legacy"] = original + "A line a person added.\n"

	report, err := m.run(true)
	if err != nil {
		t.Fatal(err)
	}
	if r, ok := refusalOf(report, "legacy"); len(report.Migrated) != 0 || !ok || r.Kind != RefusedEdited {
		t.Fatalf("report = %+v, want the edited page refused", report)
	}
}
