package main

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The tests of `sync --source memorycard` (S1 c and d of the C22-FU follow-up). They run through the command on
// the world of the promote tests: memorycardFixture's four memories, and the golden world's Engram database.
// Memory 1 and memory 3 are made eligible by a curated topic_key; memory 2 is pinned and bridged by import_map to
// Engram observation 1; memory 4 stays ineligible.

func syncMemorycardWorld(t *testing.T) (*promoteGoldenWorld, string) {
	t.Helper()
	w, db := memorycardWorld(t)
	conn, err := sql.Open("sqlite", db)
	if err != nil {
		t.Fatal(err)
	}
	defer closeDB(t, conn)
	for _, stmt := range []string{
		`update memories set topic_key = 'longterm-mem/card-only' where id = 1`,
		`update memories set topic_key = 'longterm-mem/second-card' where id = 3`,
		`update memories set pinned = 1 where id = 2`,
	} {
		if _, err := conn.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	return w, db
}

func runSync(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	return runPromoteGolden(t, append([]string{"sync", "--project", promoteGoldenProject}, args...))
}

func TestSyncSourceMemorycard_PromotesANeverPromotedEligibleMemory(t *testing.T) {
	w, db := syncMemorycardWorld(t)
	dbBefore := fileHash(t, db)

	code, stdout, stderr := runSync(t, "--source", "memorycard")
	if code != exitOK {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	// Memory 2 is bridged to an observation that has no page, so it is new like memory 1 and memory 3.
	if !strings.Contains(stdout, "promoted 3 memory(ies)") {
		t.Errorf("stdout = %q, want it to say three memories were promoted", stdout)
	}
	names := pageNames(t, w)
	var cards []string
	for _, name := range names {
		if strings.HasPrefix(name, "c-") {
			cards = append(cards, name)
		}
	}
	if len(cards) != 3 {
		t.Fatalf("pages = %v, want the three memories' pages", names)
	}
	first := readVaultFile(t, w, "wiki/memory/"+cards[0])
	if !strings.Contains(first, "source_id: 1\n") || strings.Contains(first, "engram_id") {
		t.Errorf("page of memory 1 = %s", first)
	}
	if fileHash(t, db) != dbBefore {
		t.Errorf("the memorycard database changed")
	}
	// A second run is a no-op: the memories are at their promoted revision.
	treeBefore := renderPromoteGoldenTree(t, w.vaultRoot)
	if code, so, se := runSync(t, "--source", "memorycard"); code != exitOK || !strings.Contains(so, "promoted 0 memory(ies)") {
		t.Fatalf("second run: exit %d\nstdout: %s\nstderr: %s", code, so, se)
	}
	if after := renderPromoteGoldenTree(t, w.vaultRoot); after != treeBefore {
		t.Errorf("the second run changed the vault:\nbefore:\n%s\nafter:\n%s", treeBefore, after)
	}
}

func TestSyncSourceMemorycard_ABridgedMemoryIsCountedAndExitsZero(t *testing.T) {
	w, _ := syncMemorycardWorld(t)
	// Observation 1 gets its page the way it always has; memory 2 is bridged to it.
	seedLegacyPromotedPage(t, w)
	pagePath := filepath.Join(w.vaultRoot, "wiki", "memory", "c-000901.md")
	before := fileHash(t, pagePath)

	// The dry run decides it the same way: the bridged memory is counted, and no page is planned for it.
	if code, so, se := runSync(t, "--source", "memorycard", "--dry-run"); code != exitOK || !strings.Contains(so, "would promote 2 memory(ies)") || !strings.Contains(so, "1 bridged") {
		t.Fatalf("dry run: exit %d\nstdout: %s\nstderr: %s", code, so, se)
	}

	code, stdout, stderr := runSync(t, "--source", "memorycard")
	if code != exitOK {
		t.Fatalf("exit %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "1 bridged") {
		t.Errorf("stdout = %q, want the bridged count", stdout)
	}
	if fileHash(t, pagePath) != before {
		t.Errorf("the bridged memory's page changed")
	}
	var cards int
	for _, name := range pageNames(t, w) {
		if strings.HasPrefix(name, "c-") {
			cards++
		}
	}
	if cards != 3 {
		t.Errorf("pages = %v, want the bridged page and the two eligible memories' pages, and none for memory 2", pageNames(t, w))
	}
}

// A memory mirrored by hand (no import_map row) that holds the content of an observation with a page is bridged
// too: Engram's query, the one `promote --source memorycard` asks, tells the run so.
func TestSyncSourceMemorycard_ASameContentMemoryIsCountedAsBridged(t *testing.T) {
	w, db := syncMemorycardWorld(t)
	addHandMirroredMemories(t, db)
	conn, err := sql.Open("sqlite", db)
	if err != nil {
		t.Fatal(err)
	}
	defer closeDB(t, conn)
	// Memory 5 holds the text of observation 1; make it eligible, as the page of observation 1 is the point.
	if _, err := conn.Exec(`update memories set pinned = 1 where id = 5`); err != nil {
		t.Fatal(err)
	}
	seedLegacyPromotedPage(t, w)

	code, stdout, stderr := runSync(t, "--source", "memorycard")
	if code != exitOK || !strings.Contains(stdout, "2 bridged") {
		t.Fatalf("exit %d, stdout %q, want exit 0 and memories 2 and 5 counted as bridged\nstderr: %s", code, stdout, stderr)
	}
	for _, name := range pageNames(t, w) {
		if strings.HasPrefix(name, "c-") && strings.Contains(readVaultFile(t, w, "wiki/memory/"+name), "source_id: 5\n") {
			t.Errorf("memory 5 got the page %s", name)
		}
	}
}

func TestSyncSourceMemorycard_ALocallyEditedPageIsNamedAndExitsZero(t *testing.T) {
	w, db := syncMemorycardWorld(t)
	if code, so, se := runSync(t, "--source", "memorycard"); code != exitOK {
		t.Fatalf("first run: exit %d: %s %s", code, so, se)
	}
	pages := filepath.Join(w.vaultRoot, "wiki", "memory")
	var edited string
	for _, name := range pageNames(t, w) {
		if strings.HasPrefix(name, "c-") && strings.Contains(readVaultFile(t, w, "wiki/memory/"+name), "source_id: 1\n") {
			edited = filepath.Join(pages, name)
		}
	}
	if edited == "" {
		t.Fatal("no page for memory 1")
	}
	data, err := os.ReadFile(edited)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(edited, append(data, []byte("\nA hand-written note.\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	conn, err := sql.Open("sqlite", db)
	if err != nil {
		t.Fatal(err)
	}
	defer closeDB(t, conn)
	if _, err := conn.Exec(`insert into memory_versions values (1, 3, 'Card Only', 'A newer body.', '')`); err != nil {
		t.Fatal(err)
	}
	before := fileHash(t, edited)

	code, stdout, stderr := runSync(t, "--source", "memorycard")
	if code != exitOK {
		t.Fatalf("exit %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "1 locally edited") {
		t.Errorf("stdout = %q, want the local-edit count", stdout)
	}
	if fileHash(t, edited) != before {
		t.Errorf("the hand-edited page changed")
	}
}

func TestSyncSourceMemorycard_DryRunWritesNothing(t *testing.T) {
	w, db := syncMemorycardWorld(t)
	dbBefore := fileHash(t, db)
	treeBefore := renderPromoteGoldenTree(t, w.vaultRoot)

	code, stdout, stderr := runSync(t, "--source", "memorycard", "--dry-run")
	if code != exitOK {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	for _, want := range []string{"would promote 3 memory(ies)", "would promote: Card Only", "would promote: Pinned Decision", "would promote: Second Card", "nothing was written"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}
	if after := renderPromoteGoldenTree(t, w.vaultRoot); after != treeBefore {
		t.Errorf("--dry-run changed the vault:\nbefore:\n%s\nafter:\n%s", treeBefore, after)
	}
	if fileHash(t, db) != dbBefore {
		t.Errorf("the memorycard database changed")
	}
}

func TestSyncSourceMemorycard_DryRunPredictsWhatTheRunDoes(t *testing.T) {
	syncMemorycardWorld(t)
	_, dry, _ := runSync(t, "--source", "memorycard", "--dry-run")
	_, real, _ := runSync(t, "--source", "memorycard")
	if !strings.Contains(dry, "would promote 3 memory(ies)") || !strings.Contains(real, "promoted 3 memory(ies)") {
		t.Fatalf("dry run %q and run %q disagree on what is promoted", dry, real)
	}
}

// Status propagation (S1 e): a promoted memory that memorycard has since superseded or archived gets its page's
// status and related patched on the next `sync --source memorycard`, frontmatter only.

// cardPagePath is the file of the page that carries source_id id.
func cardPagePath(t *testing.T, w *promoteGoldenWorld, id string) string {
	t.Helper()
	for _, name := range pageNames(t, w) {
		if strings.HasPrefix(name, "c-") && strings.Contains(readVaultFile(t, w, "wiki/memory/"+name), "source_id: "+id+"\n") {
			return filepath.Join(w.vaultRoot, "wiki", "memory", name)
		}
	}
	t.Fatalf("no page for memorycard memory %s among %v", id, pageNames(t, w))
	return ""
}

// pageText is the page at path as a string.
func pageText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// pageBodyOf is what follows the frontmatter block of a page.
func pageBodyOf(t *testing.T, page string) string {
	t.Helper()
	end := strings.Index(page[len("---\n"):], "\n---\n")
	if !strings.HasPrefix(page, "---\n") || end < 0 {
		t.Fatalf("no frontmatter block:\n%s", page)
	}
	return page[len("---\n")+end+len("\n---\n"):]
}

// retire runs a statement against the fixture memorycard database, as memorycard's own supersede or archive does:
// it changes the status and leaves the memory's versions alone.
func retire(t *testing.T, db, stmt string) {
	t.Helper()
	conn, err := sql.Open("sqlite", db)
	if err != nil {
		t.Fatal(err)
	}
	defer closeDB(t, conn)
	if _, err := conn.Exec(stmt); err != nil {
		t.Fatalf("%s: %v", stmt, err)
	}
}

// A superseded memory's page is still `developing` until the next sync, which patches it to `superseded`, links the
// successor's page and leaves the body and every other page alone. The status change adds no version, so the
// promotion pass has nothing to do and says so.
func TestSyncSourceMemorycard_ASupersededMemorysPageIsPatched(t *testing.T) {
	w, db := syncMemorycardWorld(t)
	if code, so, se := runSync(t, "--source", "memorycard"); code != exitOK {
		t.Fatalf("first run: exit %d: %s %s", code, so, se)
	}
	oldPage, successorPage := cardPagePath(t, w, "3"), cardPagePath(t, w, "1")
	before, successorBefore := pageText(t, oldPage), fileHash(t, successorPage)
	if !strings.Contains(before, "status: developing\n") {
		t.Fatalf("setup page is not developing:\n%s", before)
	}
	retire(t, db, `update memories set status = 'superseded', superseded_by = 1 where id = 3`)

	code, stdout, stderr := runSync(t, "--source", "memorycard")
	if code != exitOK {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "promoted 0 memory(ies); patched 1 page(s)") {
		t.Errorf("stdout = %q, want nothing promoted and one page patched", stdout)
	}
	after := pageText(t, oldPage)
	if !strings.Contains(after, "status: superseded\n") {
		t.Errorf("the page is not superseded:\n%s", after)
	}
	successorName := strings.TrimSuffix(filepath.Base(successorPage), ".md")
	if want := "[[" + successorName + "|Card Only]]"; !strings.Contains(after, want) {
		t.Errorf("the page does not link its successor %s:\n%s", want, after)
	}
	if pageBodyOf(t, after) != pageBodyOf(t, before) {
		t.Errorf("the body changed:\n%s", pageBodyOf(t, after))
	}
	if fileHash(t, successorPage) != successorBefore {
		t.Errorf("the successor's page changed")
	}
}

// An archived memory's page becomes archived.
func TestSyncSourceMemorycard_AnArchivedMemorysPageIsPatched(t *testing.T) {
	w, db := syncMemorycardWorld(t)
	if code, so, se := runSync(t, "--source", "memorycard"); code != exitOK {
		t.Fatalf("first run: exit %d: %s %s", code, so, se)
	}
	retire(t, db, `update memories set status = 'archived' where id = 3`)

	code, stdout, stderr := runSync(t, "--source", "memorycard")
	if code != exitOK || !strings.Contains(stdout, "patched 1 page(s)") {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if page := pageText(t, cardPagePath(t, w, "3")); !strings.Contains(page, "status: archived\n") {
		t.Errorf("the page is not archived:\n%s", page)
	}
}

// A page edited by hand gets its frontmatter patched, and its body, the edit included, is not touched.
func TestSyncSourceMemorycard_ALocallyEditedPageStillGetsItsStatusPatch(t *testing.T) {
	w, db := syncMemorycardWorld(t)
	if code, so, se := runSync(t, "--source", "memorycard"); code != exitOK {
		t.Fatalf("first run: exit %d: %s %s", code, so, se)
	}
	path := cardPagePath(t, w, "3")
	edited := pageText(t, path) + "\nA hand-written note.\n"
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	retire(t, db, `update memories set status = 'archived' where id = 3`)

	code, stdout, stderr := runSync(t, "--source", "memorycard")
	if code != exitOK || !strings.Contains(stdout, "patched 1 page(s)") {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	after := pageText(t, path)
	if !strings.Contains(after, "status: archived\n") || pageBodyOf(t, after) != pageBodyOf(t, edited) || !strings.Contains(after, "A hand-written note.") {
		t.Errorf("the edited page:\n%s", after)
	}
}

// The dry run predicts the patches the run makes, names their pages and writes nothing.
func TestSyncSourceMemorycard_DryRunPredictsThePatches(t *testing.T) {
	w, db := syncMemorycardWorld(t)
	if code, so, se := runSync(t, "--source", "memorycard"); code != exitOK {
		t.Fatalf("first run: exit %d: %s %s", code, so, se)
	}
	retire(t, db, `update memories set status = 'archived' where id = 3`)
	treeBefore, dbBefore := renderPromoteGoldenTree(t, w.vaultRoot), fileHash(t, db)
	address := strings.TrimSuffix(filepath.Base(cardPagePath(t, w, "3")), ".md")

	code, stdout, stderr := runSync(t, "--source", "memorycard", "--dry-run")
	if code != exitOK {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	for _, want := range []string{"would promote 0 memory(ies), patch 1 page(s)", "would patch: " + address, "nothing was written"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}
	if after := renderPromoteGoldenTree(t, w.vaultRoot); after != treeBefore {
		t.Errorf("--dry-run changed the vault:\nbefore:\n%s\nafter:\n%s", treeBefore, after)
	}
	if fileHash(t, db) != dbBefore {
		t.Errorf("the memorycard database changed")
	}
	if _, real, _ := runSync(t, "--source", "memorycard"); !strings.Contains(real, "patched 1 page(s)") {
		t.Errorf("the run says %q, the dry run predicted one patch", real)
	}
}

// closeDB closes a fixture connection and fails the test when the close fails.
func closeDB(t *testing.T, conn *sql.DB) {
	t.Helper()
	if err := conn.Close(); err != nil {
		t.Errorf("close the fixture connection: %v", err)
	}
}

// unlistableMemorycardDB is a database memorycard.Open accepts and ListMemories cannot read: it has no memories table.
func unlistableMemorycardDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(realTempDir(t), "memory.db")
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeDB(t, conn)
	if _, err := conn.Exec(`create table unrelated (x integer)`); err != nil {
		t.Fatal(err)
	}
	return path
}

// The doc table promises exit 4 when memorycard cannot be listed. The dry run answers what the run answers for the
// same failure, and neither writes a page.
func TestSyncSourceMemorycard_AnUnlistableStoreIsExit4InTheDryRunAndTheRun(t *testing.T) {
	w, _ := syncMemorycardWorld(t)
	t.Setenv("LONGTERM_MEM_MEMORYCARD_DB", unlistableMemorycardDB(t))
	treeBefore := renderPromoteGoldenTree(t, w.vaultRoot)

	const listFailure = `promote: sync memorycard: list memories for "golden-project": `
	for _, tc := range []struct {
		name         string
		args         []string
		wantStdout   string
		wantStderrIn string
	}{
		{"dry run", []string{"--source", "memorycard", "--dry-run"}, "", "longterm-mem: sync --source memorycard --dry-run: " + listFailure},
		{"run", []string{"--source", "memorycard"}, "longterm-mem: sync --source memorycard promoted 0 memory(ies); patched 0 page(s); skipped 0 bridged; left 0 locally edited page(s) alone\n", "longterm-mem: sync: " + listFailure},
	} {
		code, stdout, stderr := runSync(t, tc.args...)
		if code != exitEngramUnavailable {
			t.Errorf("%s: exit %d, want %d\nstdout: %s\nstderr: %s", tc.name, code, exitEngramUnavailable, stdout, stderr)
		}
		if stdout != tc.wantStdout {
			t.Errorf("%s: stdout = %q, want %q", tc.name, stdout, tc.wantStdout)
		}
		if !strings.HasPrefix(stderr, tc.wantStderrIn) || !strings.Contains(stderr, "no such table: memories") {
			t.Errorf("%s: stderr = %q, want it to start %q and carry the store's own error", tc.name, stderr, tc.wantStderrIn)
		}
	}
	if after := renderPromoteGoldenTree(t, w.vaultRoot); after != treeBefore {
		t.Errorf("an unlistable store changed the vault")
	}
}

// A page that cannot be read may be a memory's own, so the memories are left alone and the run answers exit 6, as
// `promote --source memorycard` does for the same refusal.
func TestSyncSourceMemorycard_AnUnreadablePageWithholdsTheMemoriesWithExit6(t *testing.T) {
	w, _ := syncMemorycardWorld(t)
	pages := filepath.Join(w.vaultRoot, "wiki", "memory")
	if err := os.MkdirAll(pages, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(w.vaultRoot, "nowhere.md"), filepath.Join(pages, "c-000555.md")); err != nil {
		t.Skipf("cannot make a link: %v", err)
	}
	const withheld = "warning: 3 memory(ies) left alone, nothing was written for them: no readable page belongs to them, and a page that could not be read may (memories 1, 2, 3)\n"

	for _, tc := range []struct {
		name       string
		args       []string
		wantStdout string
		wantPrefix string
		wantTail   string
	}{
		{"dry run", []string{"--source", "memorycard", "--dry-run"},
			"longterm-mem: sync --source memorycard --dry-run: would promote 0 memory(ies), patch 0 page(s), skip 1, 0 bridged, into <vault>\nlongterm-mem: sync --source memorycard --dry-run: nothing was written\n",
			"longterm-mem: sync --source memorycard --dry-run:", withheld},
		{"run", []string{"--source", "memorycard"},
			"longterm-mem: sync --source memorycard promoted 0 memory(ies); patched 0 page(s); skipped 0 bridged; left 0 locally edited page(s) alone\n",
			"longterm-mem: sync:", withheld},
	} {
		code, stdout, stderr := runSync(t, tc.args...)
		if code != exitRegistrationConflict {
			t.Errorf("%s: exit %d, want %d\nstdout: %s\nstderr: %s", tc.name, code, exitRegistrationConflict, stdout, stderr)
		}
		if got := w.normalise(stdout); got != tc.wantStdout {
			t.Errorf("%s: stdout = %q, want %q", tc.name, got, tc.wantStdout)
		}
		if !strings.Contains(stderr, tc.wantPrefix+" warning: ") || !strings.Contains(stderr, "c-000555.md") || !strings.HasSuffix(stderr, tc.wantPrefix+" "+tc.wantTail) {
			t.Errorf("%s: stderr = %q, want the unreadable page named and the withheld memories counted", tc.name, stderr)
		}
	}
	if names := pageNames(t, w); len(names) != 1 {
		t.Errorf("pages = %v, want only the unreadable link", names)
	}
}

// The index rebuild runs after the pass, so a failing one is the run's failure: exit 5, and the pages the pass wrote
// stay. The dry run rebuilds nothing, so it has no such failure.
func TestSyncSourceMemorycard_AFailedIndexRebuildIsExit5(t *testing.T) {
	w, _ := syncMemorycardWorld(t)
	if err := os.WriteFile(filepath.Join(w.vaultRoot, "bin", "setup-retrieve.sh"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	if code, stdout, stderr := runSync(t, "--source", "memorycard", "--dry-run"); code != exitOK {
		t.Errorf("dry run: exit %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	code, stdout, stderr := runSync(t, "--source", "memorycard")
	if code != exitVaultSubprocessFailed {
		t.Fatalf("exit %d, want %d\nstdout: %s\nstderr: %s", code, exitVaultSubprocessFailed, stdout, stderr)
	}
	if want := "longterm-mem: sync --source memorycard promoted 3 memory(ies); patched 0 page(s); skipped 0 bridged; left 0 locally edited page(s) alone\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
	if !strings.HasPrefix(stderr, "longterm-mem: sync: ") {
		t.Errorf("stderr = %q, want the failure named", stderr)
	}
}

// degradeFixtureDB makes the SQLite file at path readable only through the immutable=1 fallback: a WAL database
// with no -wal or -shm on disk, in a directory nobody may write.
func degradeFixtureDB(t *testing.T, path string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("running as root ignores directory permissions; the degraded fallback cannot be forced")
	}
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatal(err)
	}
	closeDB(t, conn)
	for _, suffix := range []string{"-wal", "-shm"} {
		_ = os.Remove(path + suffix)
	}
	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
}

// A frozen store is declared on stderr, as `sync` and `promote` declare a frozen Engram, for both stores the run reads.
func TestSyncSourceMemorycard_DeclaresADegradedMemorycard(t *testing.T) {
	_, db := syncMemorycardWorld(t)
	degradeFixtureDB(t, db)

	code, stdout, stderr := runSync(t, "--source", "memorycard", "--dry-run")
	if code != exitOK {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	want := "longterm-mem: sync: memorycard is being read through the immutable=1 fallback, so this run works from the snapshot taken when the connection was opened, not the live database\n"
	if stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
	if !strings.Contains(stdout, "would promote 3 memory(ies)") {
		t.Errorf("stdout = %q, want the run to go on from the snapshot", stdout)
	}
}

// The run reads memorycard alone (S10 c), so a frozen Engram database is not the run's to declare: it is not opened.
// (Previously: the run read Engram for the twins of a memory with no import_map row and declared it frozen.)
func TestSyncSourceMemorycard_ReadsNoEngramDatabaseSoDeclaresNone(t *testing.T) {
	w, _ := syncMemorycardWorld(t)
	degradeFixtureDB(t, w.dbPath)

	code, stdout, stderr := runSync(t, "--source", "memorycard", "--dry-run")
	if code != exitOK || stderr != "" || !strings.Contains(stdout, "would promote 3 memory(ies)") {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s\nwant exit 0, three memories to promote and nothing on stderr: a frozen Engram is not read", code, stdout, stderr)
	}
}

// A store that fails to close is said on stderr, in the style of the other warnings, and is no failure of the run.
type failingCloser struct{}

func (failingCloser) Close() error { return errors.New("disk on fire") }

func TestCloseStoreNamesAFailedClose(t *testing.T) {
	a := app{command: "sync"}
	stderr := captureStderr(t, func() { a.closeStore("memorycard", failingCloser{}) })
	if want := "longterm-mem: sync: warning: close memorycard: disk on fire\n"; stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
	if stderr := captureStderr(t, func() { a.closeStore("memorycard", nopCloser{}) }); stderr != "" {
		t.Errorf("a clean close wrote %q", stderr)
	}
}

type nopCloser struct{}

func (nopCloser) Close() error { return nil }

// Engram and any other name are not stores `sync` reads (S10 a): usage, naming the one there is, and nothing is
// written. (Previously: the two sources were engram and memorycard.)
func TestSyncSource_OnlyMemorycardIsAStore(t *testing.T) {
	for _, name := range []string{"engram", "notes"} {
		t.Run(name, func(t *testing.T) {
			w := enterClockWorld(t)
			treeBefore := renderPromoteGoldenTree(t, w.vaultRoot)
			code, stdout, stderr := runSync(t, "--source", name)
			want := "longterm-mem: sync: --source \"" + name + "\" is not a store: use memorycard\n"
			if code != exitUsage || stdout != "" || stderr != want {
				t.Fatalf("exit %d, stdout %q, stderr %q, want usage and %q", code, stdout, stderr, want)
			}
			if after := renderPromoteGoldenTree(t, w.vaultRoot); after != treeBefore {
				t.Errorf("a refused source changed the vault")
			}
		})
	}
}

// Plain `sync` is `sync --source memorycard` (S10 a): the same words, the same pages, over the same store.
func TestSyncWithoutASourceIsSyncFromMemorycard(t *testing.T) {
	w, _ := syncMemorycardWorld(t)
	codeP, outP, errP := runSync(t, "--dry-run")
	codeN, outN, errN := runSync(t, "--source", "memorycard", "--dry-run")
	if codeP != exitOK || codeP != codeN || outP != outN || errP != errN {
		t.Fatalf("plain sync: exit %d %q %q; --source memorycard: exit %d %q %q", codeP, outP, errP, codeN, outN, errN)
	}
	if !strings.Contains(outP, "sync --source memorycard --dry-run: would promote 3 memory(ies)") {
		t.Errorf("stdout = %q, want the memorycard words", outP)
	}
	if names := pageNames(t, w); len(names) != 0 {
		t.Fatalf("a dry run wrote pages: %v", names)
	}
	code, out, errOut := runSync(t)
	if code != exitOK || !strings.Contains(out, "sync --source memorycard promoted 3 memory(ies)") || errOut != "" {
		t.Errorf("plain sync: exit %d %q %q, want it to promote 3 memories from memorycard", code, out, errOut)
	}
}

// A vault whose pages were written by a release that promoted from Engram (they carry engram_id and
// engram_revision) is read by plain `sync` as it reads any page: the page of observation 1 is memory 2's, so
// memory 2 is bridged, and no page is written for it (S10 d). (Previously: plain `sync` read Engram, and a page
// without memorycard_id was one of an observation.)
func TestSyncWithoutASourceReadsLegacyKeyedPagesFromMemorycard(t *testing.T) {
	w, _ := syncMemorycardWorld(t)
	seedLegacyPromotedPage(t, w)
	pagePath := filepath.Join(w.vaultRoot, "wiki", "memory", "c-000901.md")
	before := fileHash(t, pagePath)

	code, so, se := runSync(t, "--dry-run")
	if code != exitOK || !strings.Contains(so, "sync --source memorycard --dry-run: would promote 2 memory(ies)") || !strings.Contains(so, "1 bridged") {
		t.Fatalf("dry run: exit %d\nstdout: %s\nstderr: %s", code, so, se)
	}
	code, so, se = runSync(t)
	if code != exitOK || !strings.Contains(so, "promoted 2 memory(ies)") || !strings.Contains(so, "1 bridged") {
		t.Fatalf("run: exit %d\nstdout: %s\nstderr: %s", code, so, se)
	}
	if fileHash(t, pagePath) != before {
		t.Errorf("the legacy page was rewritten")
	}
}
