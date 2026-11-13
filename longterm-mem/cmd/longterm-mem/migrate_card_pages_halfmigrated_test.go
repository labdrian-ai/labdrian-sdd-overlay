package main

import (
	"strings"
	"testing"
)

// halfMigratedWorld is a vault with one page whose body is the one its precedence entry records and whose frontmatter
// is not: a title edited by hand.
func halfMigratedWorld(t *testing.T) *promoteGoldenWorld {
	t.Helper()
	w, _ := memorycardWorld(t)
	page := legacyCardPage("c-000970", "Half", 2, "learning", "3")
	recordPage(t, w, "c-000970", page, true)
	writeVaultFile(t, w, "wiki/memory/c-000970.md", strings.Replace(page, "title: \"Half\"", "title: \"Half, renamed\"", 1))
	return w
}

// The refusal of a half-migrated page lists the backup directories there are, so the way back is a path and not a pattern.
func TestMigrateCardPages_AHalfMigratedRefusalListsTheBackupsThatExist(t *testing.T) {
	w := halfMigratedWorld(t)
	writeVaultFile(t, w, ".vault-meta/card-page-migration-20300101T000000Z/wiki/memory/c-000970.md", "x")
	writeVaultFile(t, w, ".vault-meta/card-page-migration-20300102T000000Z-2/wiki/memory/c-000970.md", "x")
	writeVaultFile(t, w, ".vault-meta/sync-state.json", "{}")

	code, stdout, stderr := runMigrate(t)
	if code != exitRegistrationConflict {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	for _, want := range []string{
		"refused c-000970: ",
		"backups present: .vault-meta/card-page-migration-20300101T000000Z/, .vault-meta/card-page-migration-20300102T000000Z-2/",
	} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr lacks %q:\n%s", want, stderr)
		}
	}
	if strings.Contains(stderr, "sync-state.json") {
		t.Errorf("stderr = %s, want only the backups of this migration listed", stderr)
	}
}

func TestMigrateCardPages_AHalfMigratedRefusalSaysWhenNoBackupExists(t *testing.T) {
	halfMigratedWorld(t)
	code, stdout, stderr := runMigrate(t)
	if code != exitRegistrationConflict || !strings.Contains(stderr, "backups present: none") {
		t.Fatalf("exit %d, stdout %q, stderr %q, want it to say no backup directory is there", code, stdout, stderr)
	}
}

// Only a half-migrated refusal carries the list; the others are as they were.
func TestMigrateCardPages_OtherRefusalsDoNotListBackups(t *testing.T) {
	w, _ := migrationWorld(t)
	writeVaultFile(t, w, ".vault-meta/card-page-migration-20300101T000000Z/wiki/memory/c-000907.md", "x")
	_, _, stderr := runMigrate(t)
	if strings.Contains(stderr, "backups present") {
		t.Errorf("stderr = %s, want no list on a refusal that is not half-migrated", stderr)
	}
}
