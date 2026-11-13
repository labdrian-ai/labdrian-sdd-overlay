package promote

import (
	"errors"
	"strings"
	"testing"
)

// What a run stopped halfway leaves, and the pages whose legacy keys the migration must account for.

// A run that stopped after persisting the entry and before writing the page is finished by the rerun whatever the
// memory became in between: its revision or its type changing does not turn the page into a hand edit.
func TestMigrateCardPages_ARerunFinishesAStoppedPageAfterTheMemoryMovedOn(t *testing.T) {
	m := newMigWorld()
	m.memory(cardOf(7, migProject), 31)
	m.put("legacy", "engram_id: 31\nengram_type: learning\nengram_revision: 1\n")
	m.pages.writeErr = errors.New("disk full")
	if _, err := m.run(true); err == nil {
		t.Fatal("the failed write answered no error")
	}

	// The memory is revised, and retyped, before the rerun.
	moved := cardOf(7, migProject)
	moved.Revision, moved.Type = 9, "decision"
	m.memory(moved, 31)
	m.pages.writeErr = nil
	store, err := m.repo.LoadPrecedence()
	if err != nil {
		t.Fatal(err)
	}
	m.w = &Writer{Pages: m.pages, Precedence: m.repo, Store: store}

	report, err := m.run(true)
	if err != nil || len(report.Migrated) != 1 || len(report.Refused) != 0 {
		t.Fatalf("rerun: report = %+v, err = %v, want the stopped page finished", report, err)
	}
	page := m.pages.pages["legacy"]
	if !strings.Contains(page, "source: memorycard\nsource_id: 7\n") || strings.Contains(page, "engram_id") {
		t.Errorf("page = %q, want the store-agnostic keys", page)
	}
	if entry, _ := store.Get("legacy"); !entry.MatchesPage(page) {
		t.Errorf("the entry %+v does not match the finished page", entry)
	}
	if again, err := m.run(true); err != nil || again.Legacy != 0 {
		t.Errorf("third run: report = %+v, err = %v, want nothing left", again, err)
	}
}

// A stopped page with a hand edit in its body is still refused after the memory moved on.
func TestMigrateCardPages_AStoppedPageThatWasEditedMeanwhileIsStillRefused(t *testing.T) {
	m := newMigWorld()
	m.memory(cardOf(7, migProject), 31)
	m.put("legacy", "engram_id: 31\nengram_revision: 1\n")
	m.pages.writeErr = errors.New("disk full")
	if _, err := m.run(true); err == nil {
		t.Fatal("the failed write answered no error")
	}
	m.pages.writeErr = nil
	m.pages.pages["legacy"] += "A line a person added.\n"
	store, _ := m.repo.LoadPrecedence()
	m.w = &Writer{Pages: m.pages, Precedence: m.repo, Store: store}
	report, err := m.run(true)
	if r, ok := refusalOf(report, "legacy"); err != nil || len(report.Migrated) != 0 || !ok || r.Kind != RefusedEdited {
		t.Fatalf("report = %+v, err = %v, want the edited page refused", report, err)
	}
}

func TestMigrateCardPages_AFailedSaveSaysWhatItCannotKnow(t *testing.T) {
	m := newMigWorld()
	m.memory(cardOf(7, migProject))
	m.put("a", "memorycard_id: 7\n")
	m.repo.saveErr = errors.New("read-only file system")
	_, err := m.run(true)
	if err == nil || !strings.Contains(err.Error(), "may or may not") || !strings.Contains(err.Error(), "backup-dir") {
		t.Fatalf("err = %v, want it to say the sidecar on disk may or may not hold the entry, and where the backup is", err)
	}
}

// A legacy page with no project is still a legacy page: counted and refused by name, never skipped in silence.
func TestMigrateCardPages_ALegacyPageWithNoProjectIsCountedAndRefused(t *testing.T) {
	m := newMigWorld()
	m.memory(cardOf(7, migProject))
	content := "---\ntitle: \"T\"\naddress: lost\nmemorycard_id: 7\n---\n# T\n\nBody.\n"
	m.pages.pages["lost"] = content
	other := "---\ntitle: \"T\"\naddress: away\nmemorycard_id: 7\nproject: elsewhere\n---\n# T\n"
	m.pages.pages["away"] = other
	report, err := m.run(true)
	if err != nil {
		t.Fatal(err)
	}
	r, ok := refusalOf(report, "lost")
	if report.Legacy != 1 || !ok || r.Kind != RefusedOther || !strings.Contains(r.Reason, "no project") || m.pages.pages["lost"] != content {
		t.Fatalf("report = %+v, want the page with no project counted, refused as having no project and left as it is (a page of another project is not this run's)", report)
	}
	if m.pages.pages["away"] != other {
		t.Errorf("a page of another project changed")
	}
}

// Only a top-level key is a legacy key: a line that merely looks like one inside a block scalar is text.
func TestMigrateCardPages_AKeyInsideABlockScalarIsNoLegacyKey(t *testing.T) {
	m := newMigWorld()
	m.memory(cardOf(7, migProject))
	m.put("mention", "notes: |\n  engram_id: 5\n  memorycard_type: x\n")
	report, err := m.run(true)
	if err != nil || report.Legacy != 0 || len(report.Refused) != 0 {
		t.Fatalf("report = %+v, err = %v, want a page that only mentions the keys left out of the count", report, err)
	}

	// A page that has a real identity beside such a mention migrates, and the mention stays as it was.
	m.put("real", "memorycard_id: 7\nnotes: |\n  memorycard_type: x\n")
	report, err = m.run(true)
	if err != nil || len(report.Migrated) != 1 || report.Legacy != 1 {
		t.Fatalf("report = %+v, err = %v, want the page with the real identity migrated", report, err)
	}
	if page := m.pages.pages["real"]; !strings.Contains(page, "notes: |\n  memorycard_type: x\n") || strings.Contains(page, "\nmemorycard_id") {
		t.Errorf("page = %q, want the identity replaced and the scalar kept", page)
	}
}
