package promote

import (
	"errors"
	"strings"
	"testing"
)

// A page that carries a legacy key whose frontmatter cannot be read is a legacy page the migration cannot rewrite:
// counted and refused by name, never skipped in silence.

const unreadableFragment = "unreadable frontmatter (CRLF line endings or no closing ---)"

func TestMigrateCardPages_APageWithLegacyKeysAndUnreadableFrontmatterIsCountedAndRefusedByName(t *testing.T) {
	m := newMigWorld()
	m.memory(cardOf(7, migProject))
	m.put("fine", "memorycard_id: 7\n")
	crlf := "---\r\ntitle: \"T\"\r\naddress: crlf\r\nmemorycard_id: 7\r\nproject: " + migProject + "\r\n---\r\n# T\r\n"
	open := "---\ntitle: \"T\"\naddress: open\nengram_id: 5\nproject: " + migProject + "\n# T\n\nBody, and no closing line.\n"
	away := "---\r\ntitle: \"T\"\r\nmemorycard_id: 7\r\nproject: elsewhere\r\n---\r\n"
	lost := "---\r\ntitle: \"T\"\r\nmemorycard_id: 7\r\n---\r\n"
	noKey := "---\r\ntitle: \"T\"\r\nproject: " + migProject + "\r\n---\r\n# T\r\n"
	prose := "# T\n\nengram_id: 5\nmemorycard_id: 7\n"
	for address, content := range map[string]string{"crlf": crlf, "open": open, "away": away, "lost": lost, "nokey": noKey, "prose": prose} {
		m.pages.pages[address] = content
	}
	before := map[string]string{}
	for a, c := range m.pages.pages {
		before[a] = c
	}

	report, err := m.run(true)
	if err != nil {
		t.Fatal(err)
	}
	if report.Legacy != 3 || len(report.Migrated) != 1 || len(report.Refused) != 2 {
		t.Fatalf("report = %+v, want 3 legacy pages: fine migrated, crlf and open refused", report)
	}
	for _, address := range []string{"crlf", "open"} {
		r, ok := refusalOf(report, address)
		if !ok || r.Kind != RefusedUnreadable || !strings.Contains(r.Reason, unreadableFragment) {
			t.Errorf("refusal of %s = %+v, want the unreadable kind with %q", address, r, unreadableFragment)
		}
	}
	if len(report.NoProject) != 1 || report.NoProject[0].Address != "lost" {
		t.Errorf("NoProject = %+v, want the CRLF page with no project", report.NoProject)
	}
	for _, address := range []string{"crlf", "open", "away", "lost", "nokey", "prose"} {
		if m.pages.pages[address] != before[address] {
			t.Errorf("%s changed", address)
		}
	}
}

// A page that cannot be read is no recovery either: the second run still names it.
func TestMigrateCardPages_AnUnreadablePageIsRefusedAgainOnTheNextRun(t *testing.T) {
	m := newMigWorld()
	m.pages.pages["crlf"] = "---\r\nmemorycard_id: 7\r\nproject: " + migProject + "\r\n---\r\n"
	for i := 0; i < 2; i++ {
		report, err := m.run(true)
		if err != nil || report.Legacy != 1 || len(report.Refused) != 1 || report.Refused[0].Kind != RefusedUnreadable {
			t.Fatalf("run %d: report = %+v, err = %v", i+1, report, err)
		}
	}
}

// Recovery also tries an empty type: a memory whose type was empty when a run stopped, and has one now.
func TestMigrateCardPages_AStoppedPageRecordedWithAnEmptyTypeIsFinished(t *testing.T) {
	m := newMigWorld()
	first := cardOf(7, migProject)
	first.Type, first.Revision = "", 3
	m.memory(first, 31)
	m.put("legacy", "engram_id: 31\nengram_type: learning\nengram_revision: 1\n")
	m.pages.writeErr = errors.New("disk full")
	if _, err := m.run(true); err == nil {
		t.Fatal("the failed write answered no error")
	}
	moved := cardOf(7, migProject)
	moved.Type, moved.Revision = "decision", 6
	m.memory(moved, 31)
	m.pages.writeErr = nil
	store, err := m.repo.LoadPrecedence()
	if err != nil {
		t.Fatal(err)
	}
	m.w = &Writer{Pages: m.pages, Precedence: m.repo, Store: store}
	report, err := m.run(true)
	if err != nil || len(report.Migrated) != 1 || len(report.Refused) != 0 {
		t.Fatalf("report = %+v, err = %v, want the stopped page finished", report, err)
	}
	if page := m.pages.pages["legacy"]; strings.Contains(page, "source_type:") || !strings.Contains(page, "source_revision: 3\n") {
		t.Errorf("page = %q, want the recorded identity (no type, revision 3)", page)
	}
}

// Recovery also tries an empty revision.
func TestMigrateCardPages_AStoppedPageRecordedWithAnEmptyRevisionIsFinished(t *testing.T) {
	m := newMigWorld()
	m.memory(cardOf(7, migProject), 31)
	content := m.put("legacy", "engram_id: 31\nengram_type: learning\nengram_revision: 1\n")
	block, _ := frontmatterBlock(content)
	entry, _ := m.w.Store.Get("legacy")
	entry.FrontmatterHash = hashText(rewrittenIdentity(block, 7, "", "learning"))
	m.w.Store.Set("legacy", entry)
	report, err := m.run(true)
	if err != nil || len(report.Migrated) != 1 || len(report.Refused) != 0 {
		t.Fatalf("report = %+v, err = %v, want the stopped page finished", report, err)
	}
	if page := m.pages.pages["legacy"]; strings.Contains(page, "source_revision:") || !strings.Contains(page, "source_type: learning\n") {
		t.Errorf("page = %q, want the recorded identity (learning, no revision)", page)
	}
}
