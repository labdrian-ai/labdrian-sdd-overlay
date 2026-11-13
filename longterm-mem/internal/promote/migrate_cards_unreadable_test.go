package promote

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// A page that carries a legacy key whose frontmatter cannot be read is a legacy page the migration cannot rewrite:
// counted and refused by name, never skipped in silence.

const unreadableFragment = "unreadable frontmatter (CRLF line endings, no closing ---, a BOM, blank line or spaces before the opening ---, spaces after an opening or closing ---, or a line ending in --- inside the frontmatter)"

func TestMigrateCardPages_APageWithLegacyKeysAndUnreadableFrontmatterIsCountedAndRefusedByName(t *testing.T) {
	m := newMigWorld()
	m.memory(cardOf(7, migProject))
	m.put("fine", "memorycard_id: 7\n")
	crlf := "---\r\ntitle: \"T\"\r\naddress: crlf\r\nmemorycard_id: 7\r\nproject: " + migProject + "\r\n---\r\n# T\r\n"
	open := "---\ntitle: \"T\"\naddress: open\nengram_id: 5\nproject: " + migProject + "\n# T\n\nBody, and no closing line.\n"
	away := "---\r\ntitle: \"T\"\r\nmemorycard_id: 7\r\nproject: elsewhere\r\n---\r\n"
	lost := "---\r\ntitle: \"T\"\r\nmemorycard_id: 7\r\n---\r\n"
	noKey := "---\r\ntitle: \"T\"\r\nproject: " + migProject + "\r\n---\r\n# T\r\n"
	// No frontmatter: a page whose first non-blank line is not --- is no promoted page, whatever its text quotes.
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
		t.Errorf("NoProject = %+v, want the CRLF page with no project alone (the prose page has no frontmatter and is ignored)", report.NoProject)
	}
	for _, address := range []string{"crlf", "open", "away", "lost", "nokey", "prose"} {
		if m.pages.pages[address] != before[address] {
			t.Errorf("%s changed", address)
		}
	}
}

// Whatever the first line of an unreadable page is (a byte order mark, a blank line, spaces), a top-level legacy key
// makes it a legacy page, counted in the project its readable `project:` line names, else among the pages with none.
func TestMigrateCardPages_AnUnreadablePageIsCountedWhateverItsFirstLineIs(t *testing.T) {
	for name, content := range map[string]string{
		"bom":     "\xef\xbb\xbf---\r\nmemorycard_id: 7\r\nproject: " + migProject + "\r\n---\r\n# T\r\n",
		"blank":   "\n---\r\nmemorycard_id: 7\r\nproject: " + migProject + "\r\n---\r\n# T\r\n",
		"spaces":  "  ---\r\nmemorycard_id: 7\r\nproject: " + migProject + "\r\n---\r\n# T\r\n",
		"bomopen": "\xef\xbb\xbf---\nengram_id: 5\nproject: " + migProject + "\n# T\n",
	} {
		m := newMigWorld()
		m.memory(cardOf(7, migProject))
		m.pages.pages["page"] = content
		report, err := m.run(true)
		if err != nil || report.Legacy != 1 || len(report.Refused) != 1 || report.Refused[0].Kind != RefusedUnreadable || len(report.NoProject) != 0 {
			t.Errorf("%s: report = %+v, err = %v, want the page counted and refused as unreadable", name, report, err)
		}
		if m.pages.pages["page"] != content {
			t.Errorf("%s: the page changed", name)
		}
	}
	m := newMigWorld()
	m.pages.pages["nameless"] = "\xef\xbb\xbf---\r\nmemorycard_id: 7\r\n---\r\n"
	if report, err := m.run(false); err != nil || report.Legacy != 0 || len(report.NoProject) != 1 {
		t.Errorf("report = %+v, err = %v, want the nameless page among the pages with no project", report, err)
	}
}

// Only a page whose first non-blank line is --- can be a legacy page. Any other page has no frontmatter, as every reader of
// longterm-mem sees it, and its text is never scanned, however it quotes a key: it is no promoted page and cannot be
// promoted twice. An unreadable page is scanned up to its first closing rule only.
func TestMigrateCardPages_OnlyAPageThatOpensWithARuleCanBeALegacyPage(t *testing.T) {
	for name, content := range map[string]string{
		"a plain note quoting keys":     "# Notes\n\nengram_id: 5\nproject: " + migProject + "\n",
		"a key before the first rule":   "engram_id: 5\nproject: " + migProject + "\n---\ntitle: x\n---\nbody\n",
		"text before a valid block":     "Some text\n---\nmemorycard_id: 7\nproject: " + migProject + "\n---\nbody\n",
		"a key quoted after the rule":   "\xef\xbb\xbf---\r\ntitle: t\r\n---\r\n# T\r\n\r\nengram_id: 5\r\nproject: " + migProject + "\r\n",
		"a valid block, key in body":    "---\ntitle: t\nproject: " + migProject + "\n---\nengram_id: 5\nmemorycard_id: 7\n",
		"only text before an open rule": "intro\n---\nmemorycard_id: 7\nproject: " + migProject + "\n",
	} {
		m := newMigWorld()
		m.memory(cardOf(7, migProject))
		m.pages.pages["page"] = content
		report, err := m.run(true)
		if err != nil || report.Legacy != 0 || len(report.Refused) != 0 || len(report.Migrated) != 0 || len(report.NoProject) != 0 {
			t.Errorf("%s: report = %+v, err = %v, want the page ignored", name, report, err)
		}
		if m.pages.pages["page"] != content {
			t.Errorf("%s: the page changed", name)
		}
	}
}

// A page whose frontmatter has a line that ends in --- is split differently by the shared reader (MatchesPage and every
// other reader take the first --- line feed anywhere) than by the migration's line-anchored closing rule, so a rewrite
// would make the shared reader see the page as edited. It is counted and refused by name, and never rewritten, when a
// top-level legacy key comes before the line-anchored closing rule.
func TestMigrateCardPages_AFrontmatterLineEndingInRulesIsRefusedAsUnreadableAndNeverRewritten(t *testing.T) {
	for name, extra := range map[string]string{"a word ending in ---": "note: a---\n", "a line of four dashes": "----\n"} {
		m := newMigWorld()
		m.memory(cardOf(7, migProject))
		content := m.put("page", "memorycard_id: 7\n"+extra)
		report, err := m.run(true)
		r, ok := refusalOf(report, "page")
		if err != nil || report.Legacy != 1 || len(report.Migrated) != 0 || !ok || r.Kind != RefusedUnreadable || !strings.Contains(r.Reason, unreadableFragment) {
			t.Errorf("%s: report = %+v, err = %v, want the tracked page refused as unreadable", name, report, err)
		}
		if m.pages.pages["page"] != content {
			t.Errorf("%s: the page changed", name)
		}
		if len(m.backup.calls) != 0 {
			t.Errorf("%s: a backup was asked for a page that is never rewritten", name)
		}
	}
}

// The identity the migration writes comes from the memory (its type) and must not end the frontmatter early for the
// shared reader: a type that ends in --- would make frontmatterBlock split the rewritten page elsewhere, so the page would
// fail MatchesPage after the rewrite. Such a page is refused by name and never written, a stopped one included.
func TestMigrateCardPages_AMemoryTypeEndingInRulesMakesARefusalAndNoWrite(t *testing.T) {
	m := newMigWorld()
	card := cardOf(7, migProject)
	card.Type = "learning---"
	m.memory(card, 31)
	m.memory(cardOf(8, migProject), 32)
	content := m.put("dashes", "engram_id: 31\nengram_revision: 1\n")
	m.put("fine", "engram_id: 32\nengram_revision: 1\n")
	report, err := m.run(true)
	r, ok := refusalOf(report, "dashes")
	if err != nil || !ok || r.Kind != RefusedOther || !strings.Contains(r.Reason, "read the rewritten page differently") || len(report.Migrated) != 1 || report.Migrated[0].Address != "fine" {
		t.Fatalf("report = %+v, err = %v, want the page of the odd type refused and the other migrated", report, err)
	}
	if m.pages.pages["dashes"] != content {
		t.Errorf("the refused page changed")
	}
	if entry, _ := m.w.Store.Get("dashes"); !entry.MatchesPage(content) {
		t.Errorf("the entry of the refused page changed: %+v", entry)
	}
	if page := m.pages.pages["fine"]; !strings.Contains(page, "source_id: 8\n") {
		t.Errorf("the other page was not migrated: %q", page)
	}
	if len(m.backup.calls) != 1 || !slices.Equal(m.backup.calls[0], []string{"fine"}) {
		t.Errorf("backup calls = %v, want one call naming only the page that is rewritten", m.backup.calls)
	}
}

// The same refusal on the recovery path: a run stopped after saving the entry of a rewrite that carries the odd type,
// and the page on disk is still the legacy one. The rerun finds the recorded rewrite through recordedRewrite and must
// still refuse it, never write it and never back it up. It is a pin: it passes on the code that checks the read-back
// on this path too. With that check disabled in a scratch edit the page is migrated and the test fails (L71).
func TestMigrateCardPages_AStoppedPageWhoseRecordedRewriteEndsInRulesIsRefusedAndNotBackedUp(t *testing.T) {
	m := newMigWorld()
	card := cardOf(7, migProject)
	card.Type = "learning---"
	m.memory(card, 31)
	content := m.put("stopped", "engram_id: 31\nengram_revision: 1\n")
	block, ok := frontmatterBlock(content)
	if !ok {
		t.Fatalf("setup: the page has no readable block: %q", content)
	}
	recorded, tracked := m.w.Store.Get("stopped")
	if !tracked {
		t.Fatal("setup: the page has no precedence entry")
	}
	recorded.FrontmatterHash = hashText(rewrittenIdentity(block, 7, "1", "learning---"))
	m.w.Store.Set("stopped", recorded)
	m.memory(cardOf(8, migProject), 32)
	m.put("fine", "engram_id: 32\nengram_revision: 1\n")

	report, err := m.run(true)
	r, ok := refusalOf(report, "stopped")
	if err != nil || !ok || r.Kind != RefusedOther || !strings.Contains(r.Reason, "read the rewritten page differently") || len(report.Migrated) != 1 || report.Migrated[0].Address != "fine" {
		t.Fatalf("report = %+v, err = %v, want the stopped page refused as other and the other page migrated", report, err)
	}
	if m.pages.pages["stopped"] != content {
		t.Errorf("the refused page changed")
	}
	if entry, _ := m.w.Store.Get("stopped"); entry != recorded {
		t.Errorf("the entry of the refused page changed: %+v", entry)
	}
	if len(m.backup.calls) != 1 || !slices.Equal(m.backup.calls[0], []string{"fine"}) {
		t.Errorf("backup calls = %v, want one call naming only the page that is rewritten", m.backup.calls)
	}
}

// Trailing spaces or tabs on an opening or closing --- line do not make a page invisible: the unreadable scan takes them
// off, so `--- ` opens and closes like `---`. The readable rule stays exact.
func TestMigrateCardPages_TrailingSpacesOnARuleLineLeaveAnUnreadablePageCounted(t *testing.T) {
	for name, content := range map[string]string{
		"spaces on both":   "--- \nmemorycard_id: 7\nproject: " + migProject + "\n--- \n# T\n",
		"tab on opening":   "---\t\nmemorycard_id: 7\nproject: " + migProject + "\n---\n# T\n",
		"spaces closing":   "---\nmemorycard_id: 7\nproject: " + migProject + "\n---  \n# T\n",
		"closing then key": "---\ntitle: t\n---\t\n# T\n\nengram_id: 5\nproject: " + migProject + "\n",
	} {
		m := newMigWorld()
		m.memory(cardOf(7, migProject))
		m.pages.pages["page"] = content
		report, err := m.run(true)
		wantCounted := name != "closing then key"
		if err != nil || (report.Legacy == 1) != wantCounted || (wantCounted && (len(report.Refused) != 1 || report.Refused[0].Kind != RefusedUnreadable)) {
			t.Errorf("%s: report = %+v, err = %v, want counted %v", name, report, err, wantCounted)
		}
		if m.pages.pages["page"] != content {
			t.Errorf("%s: the page changed", name)
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
