package main

import (
	"strings"
	"testing"
)

// The pages longterm-mem writes name their memory with the store-agnostic keys only (S6 (b), D1 of the store-agnostic
// promotion): `source`, `source_id`, `source_revision` and `source_type`, with `source: memorycard`. Each test drives
// one way of writing a page through the command: a new page, an update in place and a status patch.

// storeAgnosticKeys are the lines a page of memory 1 (a learning, at revision 3) carries to name it.
var storeAgnosticKeys = []string{"source: memorycard\n", "source_id: 1\n", "source_revision: 3\n", "source_type: learning\n"}

// legacyPageKeys are the keys no page is written with any more.
var legacyPageKeys = []string{"memorycard_id", "memorycard_revision", "memorycard_type", "engram_id", "engram_revision", "engram_type", "engram_sync_id"}

func requireStoreAgnosticIdentity(t *testing.T, page string, revision string) {
	t.Helper()
	for _, want := range storeAgnosticKeys {
		if strings.Contains(want, "source_revision") {
			want = "source_revision: " + revision + "\n"
		}
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q:\n%s", want, page)
		}
	}
	for _, key := range legacyPageKeys {
		if strings.Contains(page, key) {
			t.Errorf("page carries the legacy key %q:\n%s", key, page)
		}
	}
	if n := strings.Count(page, "source: "); n != 1 {
		t.Errorf("page says `source:` %d times, want once:\n%s", n, page)
	}
}

func TestAPageWrittenNewCarriesOnlyTheStoreAgnosticKeys(t *testing.T) {
	w, _ := memorycardWorld(t)
	if code, so, se := runPromoteGolden(t, []string{"promote", "--project", promoteGoldenProject, "--source", "memorycard", "--id", "1"}); code != exitOK {
		t.Fatalf("exit %d: %s %s", code, so, se)
	}
	requireStoreAgnosticIdentity(t, readVaultFile(t, w, "wiki/memory/c-000901.md"), "3")
}

func TestAPageUpdatedInPlaceCarriesOnlyTheStoreAgnosticKeys(t *testing.T) {
	w, db := memorycardWorld(t)
	args := []string{"promote", "--project", promoteGoldenProject, "--source", "memorycard", "--id", "1"}
	if code, so, se := runPromoteGolden(t, args); code != exitOK {
		t.Fatalf("first promote: exit %d: %s %s", code, so, se)
	}
	execOn(t, db, `INSERT INTO memory_versions (memory_id, version, title, body, saved_at) VALUES (1, 3, 'Card Only', 'Lives only in memorycard.', '')`)
	execOn(t, db, `UPDATE memories SET body = ? WHERE id = 1`, "A body revised once.")
	code, stdout, stderr := runPromoteGolden(t, args)
	if code != exitOK || !strings.Contains(stdout, "(updated)") {
		t.Fatalf("second promote: exit %d, stdout %q, stderr %q, want the page updated", code, stdout, stderr)
	}
	page := readVaultFile(t, w, "wiki/memory/c-000901.md")
	requireStoreAgnosticIdentity(t, page, "4")
	if !strings.Contains(page, "A body revised once.") {
		t.Errorf("the page does not hold the revised body:\n%s", page)
	}
}

func TestAPageWhoseStatusIsPatchedCarriesOnlyTheStoreAgnosticKeys(t *testing.T) {
	w, db := memorycardWorld(t)
	if code, so, se := runPromoteGolden(t, []string{"promote", "--project", promoteGoldenProject, "--source", "memorycard", "--id", "1"}); code != exitOK {
		t.Fatalf("promote: exit %d: %s %s", code, so, se)
	}
	execOn(t, db, `UPDATE memories SET status = 'archived' WHERE id = 1`)
	code, stdout, stderr := runPromoteGolden(t, []string{"sync", "--project", promoteGoldenProject, "--source", "memorycard"})
	if code != exitOK || !strings.Contains(stdout, "patched 1 page(s)") {
		t.Fatalf("sync: exit %d, stdout %q, stderr %q, want one page patched", code, stdout, stderr)
	}
	page := readVaultFile(t, w, "wiki/memory/c-000901.md")
	if !strings.Contains(page, "status: archived\n") {
		t.Errorf("the page was not patched to archived:\n%s", page)
	}
	requireStoreAgnosticIdentity(t, page, "3")
}
