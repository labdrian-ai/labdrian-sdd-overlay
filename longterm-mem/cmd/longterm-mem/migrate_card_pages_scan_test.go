package main

import (
	"strings"
	"testing"
)

// A legacy page with no project is reported once, apart from the per-project numbers, with its remedy; it is left alone
// and, as every refusal, makes the exit code 6.
func TestMigrateCardPages_APageWithNoProjectIsReportedApartFromTheCounts(t *testing.T) {
	w, _ := migrationWorld(t)
	lost := strings.Replace(legacyCardPage("c-000960", "Lost", 1, "learning", "3"), "project: "+promoteGoldenProject+"\n", "", 1)
	recordPage(t, w, "c-000960", lost, true)

	code, stdout, stderr := runMigrate(t)
	if code != exitRegistrationConflict {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	for _, want := range []string{
		"7 page(s) carry legacy keys in " + w.vaultRoot + ": would migrate 3, would refuse 4\n",
		"1 page(s) have no project (no project, not counted per project): c-000960\n",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}
	if !strings.Contains(stderr, "no project, not counted per project) c-000960: it carries legacy keys and has no project field: add a `project:` line") {
		t.Errorf("stderr = %s, want the page named with its remedy", stderr)
	}
	if got := readVaultFile(t, w, "wiki/memory/c-000960.md"); got != lost {
		t.Errorf("the page changed")
	}
}

// What the command sees of the vault, which the rehearsal script's own count has to agree with: the pages that are
// regular .md files directly in wiki/memory, whose frontmatter is delimited by `---` lines ending in a line feed. A
// page with CRLF line endings has no frontmatter the command can read, and a page in a subdirectory is no page of the
// vault: neither is counted, migrated or refused.
func TestMigrateCardPages_ACrlfPageAndASubdirectoryPageAreNotSeen(t *testing.T) {
	w, _ := migrationWorld(t)
	crlf := strings.ReplaceAll(legacyEngramPage("c-000950", "Crlf Page", 1, "decision", "1"), "\n", "\r\n")
	writeVaultFile(t, w, "wiki/memory/c-000950.md", crlf)
	writeVaultFile(t, w, "wiki/memory/archive/c-000951.md", legacyEngramPage("c-000951", "Deep Page", 1, "decision", "1"))
	before := cmdVaultSnapshot(t, w.vaultRoot)

	code, stdout, stderr := runMigrate(t, "--apply")
	if code != exitRegistrationConflict || !strings.Contains(stdout, "migrated 3 of 7 page(s) carrying legacy keys") {
		t.Fatalf("exit %d, stdout %q, stderr %q, want the same seven legacy pages as without the two", code, stdout, stderr)
	}
	after := cmdVaultSnapshot(t, w.vaultRoot)
	for _, rel := range []string{"wiki/memory/c-000950.md", "wiki/memory/archive/c-000951.md"} {
		if after[rel] != before[rel] {
			t.Errorf("%s changed", rel)
		}
	}
}
