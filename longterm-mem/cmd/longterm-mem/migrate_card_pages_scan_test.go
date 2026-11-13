package main

import (
	"strings"
	"testing"
)

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
