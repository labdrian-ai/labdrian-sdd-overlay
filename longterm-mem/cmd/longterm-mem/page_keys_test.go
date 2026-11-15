package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/opstest"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
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

// legacyCardPage is a page as `promote --source memorycard` wrote it before the store-agnostic keys: the memorycard_*
// keys of memory 1. The legacy keys are retired (S7), so it names no memory.
func legacyCardPage(address string) string {
	return "---\ntype: concept\ntitle: \"Card Only\"\naddress: " + address + "\naliases:\n  - \"Card Only\"\ncreated: 2026-09-01\nupdated: 2026-09-02\ntags:\n  - \"memory\"\n  - \"learning\"\nstatus: developing\nrelated: []\nsources: []\n" +
		"memorycard_id: 1\nmemorycard_type: learning\nmemorycard_revision: 3\nproject: " + promoteGoldenProject + "\n---\n# Card Only\n\nLives only in memorycard.\n\n---\nPromoted from memorycard memory 1, revision 3.\n"
}

// A page of the legacy keys is no promoted page: a dry run counts its memory as one to promote, the explicit promote
// of the memory writes it a page of its own at the next address, and the legacy page stays as it was.
func TestALegacyCardPageIsNoPromotedPageOfItsMemory(t *testing.T) {
	w, _ := syncMemorycardWorld(t)
	legacy := legacyCardPage("c-000950")
	writeVaultFile(t, w, "wiki/memory/c-000950.md", legacy)

	code, stdout, stderr := runPromoteGolden(t, []string{"sync", "--dry-run", "--project", promoteGoldenProject, "--source", "memorycard"})
	if code != exitOK || !strings.Contains(stdout, "would promote: Card Only") || strings.Contains(stdout, "already promoted") {
		t.Fatalf("sync --dry-run: exit %d, stdout %q, stderr %q, want memory 1 among those to promote", code, stdout, stderr)
	}

	code, stdout, stderr = runPromoteGolden(t, []string{"promote", "--project", promoteGoldenProject, "--source", "memorycard", "--id", "1"})
	if code != exitOK || !strings.Contains(stdout, "(created)") || strings.Contains(stdout, "already promoted") {
		t.Fatalf("promote --id 1: exit %d, stdout %q, stderr %q, want a page created", code, stdout, stderr)
	}
	if got := readVaultFile(t, w, "wiki/memory/c-000950.md"); got != legacy {
		t.Errorf("the legacy page changed:\n%s", got)
	}
	names := pageNames(t, w)
	if len(names) != 2 {
		t.Fatalf("pages = %v, want the legacy page and the new one", names)
	}
	requireStoreAgnosticIdentity(t, readVaultFile(t, w, "wiki/memory/c-000901.md"), "3")
}

// A legacy page is no patch target either: the status of its memory moves to archived and `sync` patches nothing of
// it, so the page keeps the status it had.
func TestALegacyCardPageIsNotPatchedWhenItsMemoryIsArchived(t *testing.T) {
	w, db := syncMemorycardWorld(t)
	legacy := legacyCardPage("c-000950")
	writeVaultFile(t, w, "wiki/memory/c-000950.md", legacy)
	execOn(t, db, `UPDATE memories SET status = 'archived' WHERE id = 1`)

	code, stdout, stderr := runPromoteGolden(t, []string{"sync", "--project", promoteGoldenProject, "--source", "memorycard"})
	if code != exitOK || strings.Contains(stdout, "patched 1 page(s)") {
		t.Fatalf("sync: exit %d, stdout %q, stderr %q, want no page patched", code, stdout, stderr)
	}
	if got := readVaultFile(t, w, "wiki/memory/c-000950.md"); got != legacy {
		t.Errorf("the legacy page changed:\n%s", got)
	}
}

// The one-time migration of the legacy pages is retired with them: its command is no subcommand any more.
func TestMigrateCardPagesIsNoSubcommandAnyMore(t *testing.T) {
	_, _ = memorycardWorld(t)
	code, stdout, stderr := runPromoteGolden(t, []string{"migrate-card-pages", "--project", promoteGoldenProject})
	if code != exitUsage || stdout != "" || !strings.Contains(stderr, `unknown subcommand "migrate-card-pages"`) {
		t.Errorf("migrate-card-pages: exit %d, stdout %q, stderr %q, want exit %d and the unknown subcommand refusal", code, stdout, stderr, exitUsage)
	}
}

// cardPageMemory is a memorycard memory as promotion reads it, built from the observation-shaped rows the fixtures of
// the commands' tests are written in: what a test needs to render the page of a memory (promote.EmitPage).
func cardPageMemory(o memory.Observation) memory.Memory {
	m := opstest.CardMemory(o)
	m.Identity.Source = "memorycard"
	return m
}

// writeCardPage writes the minimal promoted page of a memorycard memory under address and title, the way promote
// renders it, and returns it: doctor's address-map and registration checks read exactly this on-disk shape.
func writeCardPage(t *testing.T, vaultRoot, address, title string) promote.Page {
	t.Helper()
	m := memory.Memory{Identity: memory.Identity{Source: "memorycard", ID: "1"}, Type: "decision", Title: title, Body: "Body content.", Project: "labdrian-sdd-overlay", Revision: 1}
	page, err := promote.EmitPage(m, address, nil, opstest.Instant())
	if err != nil {
		t.Fatalf("EmitPage: %v", err)
	}
	full := filepath.Join(vaultRoot, page.Path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(page.Frontmatter+page.Body), 0o644); err != nil {
		t.Fatal(err)
	}
	return page
}
