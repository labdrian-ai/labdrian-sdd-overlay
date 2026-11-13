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

// A legacy page with no project belongs to no project's count: it is reported once, apart from the per-project numbers,
// with the remedy, never skipped in silence and never counted again in every project's run.
func TestMigrateCardPages_ALegacyPageWithNoProjectIsReportedApartFromTheCounts(t *testing.T) {
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
	if report.Legacy != 0 || len(report.Refused) != 0 || len(report.Migrated) != 0 {
		t.Fatalf("report = %+v, want the page with no project outside the per-project counts and refusals", report)
	}
	if len(report.NoProject) != 1 || report.NoProject[0].Address != "lost" || !strings.Contains(report.NoProject[0].Reason, "add a `project:` line") {
		t.Fatalf("NoProject = %+v, want the page named with the remedy", report.NoProject)
	}
	if m.pages.pages["lost"] != content || m.pages.pages["away"] != other {
		t.Errorf("a page changed")
	}
}

// parseFrontmatterFields reads indented lines too: a block scalar that mentions engram_id beside a real
// memorycard_id must not look like two identities.
func TestMigrateCardPages_AnEngramKeyInsideABlockScalarBesideARealMemorycardIdentity(t *testing.T) {
	m := newMigWorld()
	m.memory(cardOf(7, migProject))
	m.put("both", "memorycard_id: 7\nmemorycard_revision: 2\nnotes: |\n  engram_id: 5\n")
	report, err := m.run(true)
	if err != nil || len(report.Migrated) != 1 || len(report.Refused) != 0 {
		t.Fatalf("report = %+v, err = %v, want the page migrated, not refused as two identities", report, err)
	}
	if page := m.pages.pages["both"]; !strings.Contains(page, "notes: |\n  engram_id: 5\n") || !strings.Contains(page, "source_id: 7\n") {
		t.Errorf("page = %q", page)
	}
}

// stoppedPage leaves a page a run stopped on: its entry already records the rewrite, the page is still legacy, and the
// memory moved on to revision 15003, type decision, afterwards. It answers the writer of a new process.
func stoppedPage(t *testing.T, edit func(page string) string) (*migWorld, CardMigrationReport, error) {
	t.Helper()
	m := newMigWorld()
	first := cardOf(7, migProject)
	first.Revision = 15000
	m.memory(first, 31)
	m.put("legacy", "engram_id: 31\nengram_type: learning\nengram_revision: 1\n")
	m.pages.writeErr = errors.New("disk full")
	if _, err := m.run(true); err == nil {
		t.Fatal("the failed write answered no error")
	}
	moved := cardOf(7, migProject)
	moved.Revision, moved.Type = 15003, "decision"
	m.memory(moved, 31)
	m.pages.writeErr = nil
	m.pages.pages["legacy"] = edit(m.pages.pages["legacy"])
	store, err := m.repo.LoadPrecedence()
	if err != nil {
		t.Fatal(err)
	}
	m.w = &Writer{Pages: m.pages, Precedence: m.repo, Store: store}
	report, err := m.run(true)
	return m, report, err
}

// The recorded revision is searched from the memory's current one down, with no ceiling.
func TestMigrateCardPages_AStoppedPageOfAMemoryWithManyRevisionsIsFinished(t *testing.T) {
	m, report, err := stoppedPage(t, func(page string) string { return page })
	if err != nil || len(report.Migrated) != 1 || len(report.Refused) != 0 {
		t.Fatalf("report = %+v, err = %v, want the stopped page finished", report, err)
	}
	if page := m.pages.pages["legacy"]; !strings.Contains(page, "source_revision: 15000\n") || !strings.Contains(page, "source_type: learning\n") {
		t.Errorf("page = %q, want the recorded identity (revision 15000, learning)", page)
	}
}

// A frontmatter line that is not an identity line, edited after the stop, is no recorded rewrite: the page is refused
// by its own name, and the message says where the way back is.
func TestMigrateCardPages_AStoppedPageWithAnEditedFrontmatterLineIsRefusedAsHalfMigrated(t *testing.T) {
	edited := ""
	m, report, err := stoppedPage(t, func(page string) string {
		edited = strings.Replace(page, "title: \"T\"", "title: \"A new title\"", 1)
		return edited
	})
	r, ok := refusalOf(report, "legacy")
	if err != nil || len(report.Migrated) != 0 || !ok || r.Kind != RefusedHalfMigrated {
		t.Fatalf("report = %+v, err = %v, want the page refused as half-migrated", report, err)
	}
	for _, want := range []string{".vault-meta/card-page-migration-", "restores"} {
		if !strings.Contains(r.Reason, want) {
			t.Errorf("reason %q lacks %q", r.Reason, want)
		}
	}
	if m.pages.pages["legacy"] != edited {
		t.Errorf("the refused page changed")
	}
}

// An edit confined to the identity lines of a stopped page is adopted: those lines are rewritten by the migration
// anyway, and the backup of the run that stopped holds them as they were.
func TestMigrateCardPages_AnEditConfinedToTheIdentityLinesOfAStoppedPageIsAdopted(t *testing.T) {
	m, report, err := stoppedPage(t, func(page string) string {
		return strings.Replace(page, "engram_revision: 1", "engram_revision: 5", 1)
	})
	if err != nil || len(report.Migrated) != 1 || len(report.Refused) != 0 {
		t.Fatalf("report = %+v, err = %v, want the page finished", report, err)
	}
	if page := m.pages.pages["legacy"]; strings.Contains(page, "engram_") || !strings.Contains(page, "source_revision: 15000\n") {
		t.Errorf("page = %q", page)
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
