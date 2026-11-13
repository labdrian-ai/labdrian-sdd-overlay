package main

import (
	"database/sql"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The lineage and the mirrors of a memorycard memory are read from memorycard alone (S10 c and d), so the bridge
// holds with no Engram database. These tests run through the commands on the world of the sync tests, and point
// LONGTERM_MEM_ENGRAM_DB at a file that does not exist once the page of observation 1 is written.

func withNoEngramDatabase(t *testing.T) {
	t.Helper()
	t.Setenv(engramDBEnvVar, filepath.Join(t.TempDir(), "absent", "engram.db"))
}

func execOnMemorycard(t *testing.T, dbPath string, stmts ...string) {
	t.Helper()
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer closeDB(t, conn)
	for _, stmt := range stmts {
		if _, err := conn.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
}

func pageOfMemory(t *testing.T, w *promoteGoldenWorld, id string) string {
	t.Helper()
	for _, name := range pageNames(t, w) {
		if !strings.HasPrefix(name, "c-") {
			continue
		}
		// The exact front-matter line: "memorycard_id: 1" is not the page of memory 10.
		if slices.Contains(strings.Split(readVaultFile(t, w, "wiki/memory/"+name), "\n"), "memorycard_id: "+id) {
			return name
		}
	}
	return ""
}

// Memory 5 holds the content of memory 2, which was imported from observation 1, and has no import_map row. The
// page of observation 1 is the page of that content, so memory 5 is bridged to it, in a dry run, a sync and an
// explicit promote, without Engram's database: memorycard says which memory was imported from which observation.
func TestMemorycardMirrorsAreFoundInMemorycardAlone(t *testing.T) {
	w, db := syncMemorycardWorld(t)
	addHandMirroredMemories(t, db)
	execOnMemorycard(t, db, `update memories set pinned = 1 where id = 5`)
	if code, so, se := runPromoteGolden(t, []string{"promote", "--project", promoteGoldenProject, "--id", "1"}); code != exitOK {
		t.Fatalf("promote observation 1: exit %d: %s %s", code, so, se)
	}
	withNoEngramDatabase(t)
	treeBefore := renderPromoteGoldenTree(t, w.vaultRoot)

	code, stdout, stderr := runSync(t, "--source", "memorycard", "--dry-run")
	if code != exitOK || !strings.Contains(stdout, "2 bridged") || !strings.Contains(stdout, "would promote 2 memory(ies)") || stderr != "" {
		t.Fatalf("dry run: exit %d, stdout %q, stderr %q, want exit 0, memories 2 and 5 counted as bridged and nothing on stderr", code, stdout, stderr)
	}
	if after := renderPromoteGoldenTree(t, w.vaultRoot); after != treeBefore {
		t.Errorf("the dry run changed the vault")
	}

	code, stdout, stderr = runSync(t, "--source", "memorycard")
	if code != exitOK || !strings.Contains(stdout, "promoted 2 memory(ies)") || !strings.Contains(stdout, "2 bridged") || stderr != "" {
		t.Fatalf("sync: exit %d, stdout %q, stderr %q, want exit 0, 2 promoted and memories 2 and 5 counted as bridged", code, stdout, stderr)
	}
	for _, id := range []string{"2", "5"} {
		if page := pageOfMemory(t, w, id); page != "" {
			t.Errorf("memory %s got the page %s, it is bridged to the page of observation 1", id, page)
		}
	}

	code, stdout, stderr = runPromoteGolden(t, []string{"promote", "--project", promoteGoldenProject, "--source", "memorycard", "--id", "5"})
	if code != exitRegistrationConflict || !strings.Contains(stderr, "c-000901") || !strings.Contains(stderr, "same content") || stdout != "" {
		t.Errorf("promote 5: exit %d, stdout %q, stderr %q, want exit %d naming c-000901 and the shared content", code, stdout, stderr, exitRegistrationConflict)
	}
}

// Observation 3 was imported as two parts: memory 4 (part 1) and memory 7 (part 2). Memory 4 has a page of its own,
// keyed by its memory, as every page is after the migration, so memory 7, which has none, is the same piece of
// memory as a page that exists: bridged, with no Engram database to say so.
func TestAMemoryOfAnotherPartOfAnObservationIsBridgedToThePageOfItsSiblingPart(t *testing.T) {
	w, db := syncMemorycardWorld(t)
	execOnMemorycard(t, db,
		`insert into memories(id, project, type, title, body, pinned) values (7, 'golden-project', 'decision', 'Part Two', 'The second part of observation 3.', 1)`,
		`insert into import_map values ('engram', '3', 2, 7)`)
	if code, so, se := runPromoteGolden(t, []string{"promote", "--project", promoteGoldenProject, "--source", "memorycard", "--id", "4"}); code != exitOK {
		t.Fatalf("promote memory 4: exit %d: %s %s", code, so, se)
	}
	withNoEngramDatabase(t)

	code, stdout, stderr := runSync(t, "--source", "memorycard", "--dry-run")
	if code != exitOK || !strings.Contains(stdout, "would promote 3 memory(ies)") || !strings.Contains(stdout, "1 bridged") || stderr != "" {
		t.Fatalf("dry run: exit %d, stdout %q, stderr %q, want memories 1, 2 and 3 promoted and memory 7 counted as bridged", code, stdout, stderr)
	}
	code, stdout, stderr = runSync(t, "--source", "memorycard")
	if code != exitOK || !strings.Contains(stdout, "promoted 3 memory(ies)") || !strings.Contains(stdout, "1 bridged") || stderr != "" {
		t.Fatalf("sync: exit %d, stdout %q, stderr %q, want memories 1, 2 and 3 promoted and memory 7 counted as bridged", code, stdout, stderr)
	}
	if page := pageOfMemory(t, w, "7"); page != "" {
		t.Errorf("memory 7 got the page %s, it is bridged to the page of memory 4", page)
	}
	if page := pageOfMemory(t, w, "4"); page == "" {
		t.Errorf("memory 4 lost its page")
	}
}

// The memorycard commands and the MCP tool open memorycard alone: an Engram database that is not there is no
// failure of a memorycard sync or promote.
func TestMemorycardSyncAndPromoteNeedNoEngramDatabase(t *testing.T) {
	_, _ = syncMemorycardWorld(t)
	withNoEngramDatabase(t)

	if code, so, se := runSync(t, "--source", "memorycard"); code != exitOK || !strings.Contains(so, "promoted 3 memory(ies)") || se != "" {
		t.Errorf("sync: exit %d, stdout %q, stderr %q, want exit 0 and the three eligible memories promoted", code, so, se)
	}
	if code, so, se := runPromoteGolden(t, []string{"promote", "--project", promoteGoldenProject, "--source", "memorycard", "--id", "4"}); code != exitOK || se != "" {
		t.Errorf("promote 4: exit %d, stdout %q, stderr %q, want exit 0", code, so, se)
	}
}
