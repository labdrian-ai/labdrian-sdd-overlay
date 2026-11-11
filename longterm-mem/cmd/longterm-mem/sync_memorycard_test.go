package main

import (
	"database/sql"
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
	defer conn.Close()
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
	if !strings.Contains(first, "memorycard_id: 1\n") || strings.Contains(first, "engram_id") {
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
	if code, so, se := runPromoteGolden(t, []string{"promote", "--project", promoteGoldenProject, "--id", "1"}); code != exitOK {
		t.Fatalf("promote observation 1: exit %d: %s %s", code, so, se)
	}
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
	defer conn.Close()
	// Memory 5 holds the text of observation 1; make it eligible, as the page of observation 1 is the point.
	if _, err := conn.Exec(`update memories set pinned = 1 where id = 5`); err != nil {
		t.Fatal(err)
	}
	if code, so, se := runPromoteGolden(t, []string{"promote", "--project", promoteGoldenProject, "--id", "1"}); code != exitOK {
		t.Fatalf("promote observation 1: exit %d: %s %s", code, so, se)
	}

	code, stdout, stderr := runSync(t, "--source", "memorycard")
	if code != exitOK || !strings.Contains(stdout, "2 bridged") {
		t.Fatalf("exit %d, stdout %q, want exit 0 and memories 2 and 5 counted as bridged\nstderr: %s", code, stdout, stderr)
	}
	for _, name := range pageNames(t, w) {
		if strings.HasPrefix(name, "c-") && strings.Contains(readVaultFile(t, w, "wiki/memory/"+name), "memorycard_id: 5\n") {
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
		if strings.HasPrefix(name, "c-") && strings.Contains(readVaultFile(t, w, "wiki/memory/"+name), "memorycard_id: 1\n") {
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
	defer conn.Close()
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

func TestSyncSource_UnknownSourceIsUsage(t *testing.T) {
	w := enterClockWorld(t)
	treeBefore := renderPromoteGoldenTree(t, w.vaultRoot)
	code, stdout, stderr := runSync(t, "--source", "notes")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d\nstdout: %s\nstderr: %s", code, exitUsage, stdout, stderr)
	}
	if !strings.Contains(stderr, `--source "notes"`) || !strings.Contains(stderr, "engram") || !strings.Contains(stderr, "memorycard") {
		t.Errorf("stderr = %q, want it to name the bad value and the two sources", stderr)
	}
	if after := renderPromoteGoldenTree(t, w.vaultRoot); after != treeBefore {
		t.Errorf("a refused source changed the vault")
	}
}

// Naming the default source changes nothing: the command is the one that has always synced from Engram.
func TestSyncSource_EngramIsTheDefault(t *testing.T) {
	enterClockWorld(t)
	_, plainOut, plainErr := runSync(t, "--dry-run")
	code, namedOut, namedErr := runSync(t, "--source", "engram", "--dry-run")
	if code != exitOK || plainOut != namedOut || plainErr != namedErr {
		t.Fatalf("--source engram differs from plain sync:\nplain: %q / %q\nnamed: %q / %q (exit %d)", plainOut, plainErr, namedOut, namedErr, code)
	}
}
