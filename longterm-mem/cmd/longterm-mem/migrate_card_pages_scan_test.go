package main

import (
	"strings"
	"testing"
)

// A legacy page with no project belongs to no project's run: every run prints it once, as a note on stdout (not also on
// stderr), apart from the per-project numbers, with its remedy; it is left alone and makes no exit code of its own. The
// exit code 6 here comes from the four pages the world refuses.
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
		"note: 1 page(s) carry legacy keys and name no project, so no project's run counts them: c-000960; add a `project:` line to each by hand, then run again\n",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stderr, "c-000960") {
		t.Errorf("stderr = %s, want the note on stdout only", stderr)
	}
	if got := readVaultFile(t, w, "wiki/memory/c-000960.md"); got != lost {
		t.Errorf("the page changed")
	}
}

// What the command sees of the vault, which the rehearsal script's own count has to agree with: the pages that are
// regular .md files directly in wiki/memory. A page in a subdirectory is no page of the vault and is not seen. A page
// with a legacy key whose frontmatter cannot be read (CRLF line endings, a block with no closing `---`) is counted as
// a legacy page and refused by name, never skipped in silence.
func TestMigrateCardPages_AnUnreadablePageIsRefusedByNameAndASubdirectoryPageIsNotSeen(t *testing.T) {
	w, _ := migrationWorld(t)
	crlf := strings.ReplaceAll(legacyEngramPage("c-000950", "Crlf Page", 1, "decision", "1"), "\n", "\r\n")
	writeVaultFile(t, w, "wiki/memory/c-000950.md", crlf)
	writeVaultFile(t, w, "wiki/memory/archive/c-000951.md", legacyEngramPage("c-000951", "Deep Page", 1, "decision", "1"))
	before := cmdVaultSnapshot(t, w.vaultRoot)

	code, stdout, stderr := runMigrate(t, "--apply")
	if code != exitRegistrationConflict || !strings.Contains(stdout, "migrated 3 of 8 page(s) carrying legacy keys") {
		t.Fatalf("exit %d, stdout %q, stderr %q, want the seven legacy pages and the CRLF one, and not the one in the subdirectory", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "refused c-000950: unreadable frontmatter (CRLF line endings or no closing ---)") {
		t.Errorf("stderr = %s, want the CRLF page refused by name", stderr)
	}
	if !strings.Contains(stdout, "  refused, frontmatter cannot be read (CRLF line endings or no closing ---): 1\n") {
		t.Errorf("stdout = %s, want the count of the kind", stdout)
	}
	after := cmdVaultSnapshot(t, w.vaultRoot)
	for _, rel := range []string{"wiki/memory/c-000950.md", "wiki/memory/archive/c-000951.md"} {
		if after[rel] != before[rel] {
			t.Errorf("%s changed", rel)
		}
	}
}

// A page with no project is no project's refusal: with nothing else refused, the run exits 0 and still prints the note.
func TestMigrateCardPages_APageWithNoProjectDoesNotMakeTheExitCode(t *testing.T) {
	w, _ := memorycardWorld(t)
	recordPage(t, w, "c-000903", legacyCardPage("c-000903", "Card Only", 1, "learning", "3"), true)
	lost := strings.Replace(legacyCardPage("c-000960", "Lost", 2, "learning", "3"), "project: "+promoteGoldenProject+"\n", "", 1)
	recordPage(t, w, "c-000960", lost, true)

	code, stdout, stderr := runMigrate(t)
	if code != exitOK || stderr != "" {
		t.Fatalf("exit %d, stderr %q, stdout %q, want 0 and nothing on stderr", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "note: 1 page(s) carry legacy keys and name no project") || !strings.Contains(stdout, "c-000960") {
		t.Errorf("stdout = %s, want the note", stdout)
	}
}

// A run that fails still prints the note once: the failure (here, a backup that cannot be made) is no reason to hide the
// pages no project's run counts.
func TestMigrateCardPages_AFailedRunStillPrintsTheNoProjectNoteOnce(t *testing.T) {
	w, _ := memorycardWorld(t)
	recordPage(t, w, "c-000903", legacyCardPage("c-000903", "Card Only", 1, "learning", "3"), true)
	lost := strings.Replace(legacyCardPage("c-000960", "Lost", 2, "learning", "3"), "project: "+promoteGoldenProject+"\n", "", 1)
	recordPage(t, w, "c-000960", lost, true)
	// .vault-meta is a file, so the directory of the backup cannot be made and nothing is written.
	writeVaultFile(t, w, ".vault-meta", "not a directory")

	code, stdout, stderr := runMigrate(t, "--apply")
	if code == exitOK {
		t.Fatalf("exit %d, stdout %q, stderr %q, want the failure of the backup", code, stdout, stderr)
	}
	if n := strings.Count(stdout, "note: 1 page(s) carry legacy keys and name no project"); n != 1 {
		t.Errorf("the note was printed %d times, want once; stdout %q, stderr %q", n, stdout, stderr)
	}
}

// Every run prints the note, a run of a project with no legacy page of its own included.
func TestMigrateCardPages_ARunWithNoLegacyPageStillPrintsTheNoProjectNote(t *testing.T) {
	w, _ := memorycardWorld(t)
	lost := strings.Replace(legacyCardPage("c-000960", "Lost", 2, "learning", "3"), "project: "+promoteGoldenProject+"\n", "", 1)
	recordPage(t, w, "c-000960", lost, true)
	code, stdout, stderr := runMigrate(t)
	if code != exitOK || stderr != "" || !strings.Contains(stdout, "note: 1 page(s) carry legacy keys and name no project") {
		t.Errorf("exit %d, stdout %q, stderr %q, want the note and exit 0", code, stdout, stderr)
	}
}
