package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// The tests of `promote --source memorycard`. They run through the command, on the world of the golden tests (a
// vault, an Engram database with four observations) plus a memorycard database of their own, whose path is
// LONGTERM_MEM_MEMORYCARD_DB.

// memorycardFixture builds a memorycard database with the tables the adapter reads and these memories: 1 and 3
// are plain, 2 was imported from Engram observation 1 (bridged: observation 1 gets a page in the scenarios that
// promote it), 4 was imported from observation 3, which never gets a page.
func memorycardFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(realTempDir(t), "memory.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, stmt := range []string{
		`create table memories (id integer primary key autoincrement, project text not null, scope text not null default 'project',
			type text not null default 'manual', title text not null, body text not null, topic_key text, content_hash text not null default '',
			pinned integer not null default 0, status text not null default 'active', superseded_by integer, created_at text not null default '', updated_at text not null default '')`,
		`create table memory_versions (memory_id integer not null, version integer not null, title text not null, body text not null, saved_at text not null, primary key (memory_id, version))`,
		`create table import_map (source text not null, source_key text not null, part integer not null default 1, memory_id integer not null, primary key (source, source_key, part))`,
		`insert into memories(id, project, type, title, body) values
			(1, 'golden-project', 'learning', 'Card Only', 'Lives only in memorycard.'),
			(2, 'golden-project', 'decision', 'Pinned Decision', 'A decision somebody pinned.'),
			(3, 'golden-project', 'learning', 'Second Card', 'Another memorycard memory.'),
			(4, 'golden-project', 'discovery', 'Below Threshold', 'A discovery nobody curated.')`,
		`insert into memory_versions values (1, 1, 'Card Only', 'Lives only in memorycard.', ''), (1, 2, 'Card Only', 'Lives only in memorycard.', '')`,
		`insert into import_map values ('engram', '1', 1, 2), ('engram', '3', 1, 4)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	return path
}

func memorycardWorld(t *testing.T) (*promoteGoldenWorld, string) {
	t.Helper()
	w := enterClockWorld(t)
	db := memorycardFixture(t)
	t.Setenv("LONGTERM_MEM_MEMORYCARD_DB", db)
	return w, db
}

func pageNames(t *testing.T, w *promoteGoldenWorld) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(w.vaultRoot, "wiki", "memory"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func fileHash(t *testing.T, path string) [32]byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(data)
}

func TestPromoteSourceMemorycard_CreatesOnePage(t *testing.T) {
	w, db := memorycardWorld(t)
	dbBefore := fileHash(t, db)

	code, stdout, stderr := runPromoteGolden(t, []string{"promote", "--project", promoteGoldenProject, "--source", "memorycard", "--id", "1"})
	if code != exitOK {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "promoted c-000901 (created)") {
		t.Errorf("stdout = %q, want the promoted address", stdout)
	}
	names := pageNames(t, w)
	if len(names) != 1 {
		t.Fatalf("pages = %v, want one", names)
	}
	page := readVaultFile(t, w, "wiki/memory/c-000901.md")
	for _, want := range []string{"memorycard_id: 1\n", "memorycard_revision: 3\n", "memorycard_type: learning\n", "Lives only in memorycard."} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q:\n%s", want, page)
		}
	}
	if strings.Contains(page, "engram_id") {
		t.Errorf("a memorycard page must not carry engram_id:\n%s", page)
	}
	if fileHash(t, db) != dbBefore {
		t.Errorf("the memorycard database changed")
	}
}

func TestPromoteSourceMemorycard_TwiceLeavesOnePage(t *testing.T) {
	w, _ := memorycardWorld(t)
	args := []string{"promote", "--project", promoteGoldenProject, "--source", "memorycard", "--id", "1"}
	if code, so, se := runPromoteGolden(t, args); code != exitOK {
		t.Fatalf("first exit %d: %s %s", code, so, se)
	}
	pagePath := filepath.Join(w.vaultRoot, "wiki", "memory", "c-000901.md")
	before := fileHash(t, pagePath)

	code, stdout, stderr := runPromoteGolden(t, args)
	if code != exitOK {
		t.Fatalf("second exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "c-000901") || !strings.Contains(stdout, "already promoted") {
		t.Errorf("stdout = %q, want it to say the page it already has", stdout)
	}
	if names := pageNames(t, w); len(names) != 1 {
		t.Fatalf("pages = %v, want one", names)
	}
	if fileHash(t, pagePath) != before {
		t.Errorf("the second promote wrote the page")
	}
}

func TestPromoteSourceMemorycard_BridgedMemoryIsRefused(t *testing.T) {
	w, _ := memorycardWorld(t)
	// Observation 1 gets its page the way it always has.
	seedLegacyPromotedPage(t, w)
	pagePath := filepath.Join(w.vaultRoot, "wiki", "memory", "c-000901.md")
	before := fileHash(t, pagePath)
	treeBefore := renderPromoteGoldenTree(t, w.vaultRoot)

	code, stdout, stderr := runPromoteGolden(t, []string{"promote", "--project", promoteGoldenProject, "--source", "memorycard", "--id", "2"})
	if code != exitRegistrationConflict {
		t.Fatalf("exit %d, want %d\nstdout: %s\nstderr: %s", code, exitRegistrationConflict, stdout, stderr)
	}
	if !strings.Contains(stderr, "c-000901") {
		t.Errorf("stderr = %q, want it to name the existing page c-000901", stderr)
	}
	if names := pageNames(t, w); len(names) != 1 {
		t.Errorf("pages = %v, want only the existing one", names)
	}
	if fileHash(t, pagePath) != before {
		t.Errorf("the existing page changed")
	}
	if after := renderPromoteGoldenTree(t, w.vaultRoot); after != treeBefore {
		t.Errorf("the vault changed:\nbefore:\n%s\nafter:\n%s", treeBefore, after)
	}
}

// A memory mirrored by hand (no import_map row) whose content hash is that of an imported memory, whose observation
// has a page, is refused like a bridged one. Memory 5 holds the text of memory 2 (observation 1) in other case and
// spacing, memory 6 the text of memory 4 (observation 3, which has no page). Memorycard says which observation holds
// which text from the imported memories themselves: their stored hashes are empty here and are not read.
func addHandMirroredMemories(t *testing.T, dbPath string) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// The stored hash is memorycard's: lowercased, white space collapsed over title + "\n" + body.
	for _, row := range []struct {
		id          int
		title, body string
	}{
		{5, "pinned   DECISION", "A decision\nsomebody pinned."},
		{6, "Below Threshold", "A discovery nobody curated."},
	} {
		if _, err := db.Exec(`insert into memories(id, project, type, title, body, content_hash) values (?, 'golden-project', 'decision', ?, ?, ?)`,
			row.id, row.title, row.body, memory.CardContentHash(row.title, row.body)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPromoteSourceMemorycard_SameContentAsAnObservationWithAPageIsRefused(t *testing.T) {
	w, db := memorycardWorld(t)
	addHandMirroredMemories(t, db)
	seedLegacyPromotedPage(t, w)
	pagePath := filepath.Join(w.vaultRoot, "wiki", "memory", "c-000901.md")
	before := fileHash(t, pagePath)
	treeBefore := renderPromoteGoldenTree(t, w.vaultRoot)

	code, stdout, stderr := runPromoteGolden(t, []string{"promote", "--project", promoteGoldenProject, "--source", "memorycard", "--id", "5"})
	if code != exitRegistrationConflict {
		t.Fatalf("exit %d, want %d\nstdout: %s\nstderr: %s", code, exitRegistrationConflict, stdout, stderr)
	}
	if !strings.Contains(stderr, "c-000901") || !strings.Contains(stderr, "same content") {
		t.Errorf("stderr = %q, want it to name the existing page c-000901 and the shared content", stderr)
	}
	if fileHash(t, pagePath) != before || renderPromoteGoldenTree(t, w.vaultRoot) != treeBefore {
		t.Errorf("the vault changed")
	}

	// The same text as an observation with no page is not a duplicate of a page.
	if code, so, se := runPromoteGolden(t, []string{"promote", "--project", promoteGoldenProject, "--source", "memorycard", "--id", "6"}); code != exitOK {
		t.Fatalf("memory 6: exit %d: %s %s", code, so, se)
	}
}

// A memory imported from an observation that has no page is not bridged to a page: it is promoted.
func TestPromoteSourceMemorycard_ImportedWithoutAPageIsPromoted(t *testing.T) {
	w, _ := memorycardWorld(t)
	code, stdout, stderr := runPromoteGolden(t, []string{"promote", "--project", promoteGoldenProject, "--source", "memorycard", "--id", "4"})
	if code != exitOK {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if names := pageNames(t, w); len(names) != 1 {
		t.Fatalf("pages = %v, want one", names)
	}
}

// A page that cannot be read may be the memory's own page: nothing is written, and the line names the memory.
func TestPromoteSourceMemorycard_AnUnreadablePageLeavesTheMemoryAlone(t *testing.T) {
	w, _ := memorycardWorld(t)
	pages := filepath.Join(w.vaultRoot, "wiki", "memory")
	if err := os.MkdirAll(pages, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(w.vaultRoot, "nowhere.md"), filepath.Join(pages, "c-000555.md")); err != nil {
		t.Skipf("cannot make a link: %v", err)
	}
	code, stdout, stderr := runPromoteGolden(t, []string{"promote", "--project", promoteGoldenProject, "--source", "memorycard", "--id", "1"})
	if code != exitRegistrationConflict {
		t.Fatalf("exit %d, want %d\nstdout: %s\nstderr: %s", code, exitRegistrationConflict, stdout, stderr)
	}
	if !strings.Contains(stdout, "refused memorycard memory 1") || !strings.Contains(stderr, "c-000555.md") {
		t.Errorf("stdout %q, stderr %q: want the memory refused and the unreadable page named", stdout, stderr)
	}
	if names := pageNames(t, w); len(names) != 1 {
		t.Errorf("pages = %v, want only the unreadable link", names)
	}
}

func TestPromoteSourceMemorycard_UnknownMemoryIsNotFound(t *testing.T) {
	memorycardWorld(t)
	code, _, stderr := runPromoteGolden(t, []string{"promote", "--project", promoteGoldenProject, "--source", "memorycard", "--id", "99"})
	if code != exitNotFound {
		t.Fatalf("exit %d, want %d (stderr: %s)", code, exitNotFound, stderr)
	}
}

// The MCP promote tool names its source, as the command does: memorycard promotes, and no source, or Engram, is
// refused before any store is opened. Memory 2 was imported from observation 1, which has a page (written by a
// release that promoted from Engram): the tool refuses it, with the page named.
func TestTheMCPPromoteToolPromotesFromMemorycardAndNothingElse(t *testing.T) {
	w, _ := memorycardWorld(t)
	seedLegacyPromotedPage(t, w)
	tool := commands{newEmbedClient: testEmbedClient, clock: promoteGoldenClock}.mcpPromote()

	if _, err := tool(context.Background(), promoteGoldenProject, "memorycard", 1); err != nil {
		t.Fatalf("memorycard: %v", err)
	}
	if page := readVaultFile(t, w, "wiki/memory/c-000902.md"); !strings.Contains(page, "memorycard_id: 1\n") || strings.Contains(page, "engram_id") {
		t.Errorf("page promoted from memorycard = %s", page)
	}
	if _, err := tool(context.Background(), promoteGoldenProject, "memorycard", 2); err == nil || !strings.Contains(err.Error(), "c-000901") {
		t.Errorf("bridged memory over MCP: err = %v, want a refusal naming c-000901", err)
	}
	pagesBefore := pageNames(t, w)
	for source, want := range map[string]string{
		"":       "mcpserver: promote: source is required: use source memorycard",
		"engram": `mcpserver: promote: source "engram" is not a store: use memorycard`,
	} {
		if _, err := tool(context.Background(), promoteGoldenProject, source, 3); err == nil || err.Error() != want {
			t.Errorf("source %q: err = %v, want %q", source, err, want)
		}
	}
	if after := pageNames(t, w); strings.Join(after, " ") != strings.Join(pagesBefore, " ") {
		t.Errorf("a refused source wrote pages: %v, was %v", after, pagesBefore)
	}
}

// `promote` without a source is refused (S10 b): the id means what the store says it means, so the command asks.
func TestPromoteSource_IsRequired(t *testing.T) {
	w := enterClockWorld(t)
	treeBefore := renderPromoteGoldenTree(t, w.vaultRoot)
	code, stdout, stderr := runPromoteGolden(t, []string{"promote", "--project", promoteGoldenProject, "--id", "1"})
	if code != exitUsage || stdout != "" || stderr != "longterm-mem: promote: --source is required: use --source memorycard\n" {
		t.Fatalf("exit %d, stdout %q, stderr %q, want usage asking for --source memorycard", code, stdout, stderr)
	}
	if after := renderPromoteGoldenTree(t, w.vaultRoot); after != treeBefore {
		t.Errorf("a refused promote changed the vault")
	}
}

// Engram and any other name are not stores `promote` reads (S10 a): usage, naming the one there is.
func TestPromoteSource_OnlyMemorycardIsAStore(t *testing.T) {
	for _, name := range []string{"engram", "notes"} {
		t.Run(name, func(t *testing.T) {
			w := enterClockWorld(t)
			code, stdout, stderr := runPromoteGolden(t, []string{"promote", "--project", promoteGoldenProject, "--source", name, "--id", "1"})
			want := "longterm-mem: promote: --source \"" + name + "\" is not a store: use memorycard\n"
			if code != exitUsage || stdout != "" || stderr != want {
				t.Fatalf("exit %d, stdout %q, stderr %q, want usage and %q", code, stdout, stderr, want)
			}
			if names := pageNames(t, w); len(names) != 0 {
				t.Errorf("pages = %v, want none", names)
			}
		})
	}
}
