package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vault"
)

// The golden files under testdata/pages-golden pin what `longterm-mem promote`, `sync` and `promote reconcile`
// do to a vault whose pages, wiki/memory/<address>.md, are in each state a page can be found in: gone while the
// sidecar still remembers it, a directory, a link to another file, a link to nothing, a link to a directory,
// edited by hand, malformed, claiming an observation without an address or with a revision that is not a number,
// left behind by an observation that moved to another project, superseded by a newer observation, and beside
// files that are not pages; and with the pages directory itself a file, a link, a link to nothing. After the
// commands of a scenario they hold each command's exit code, stdout and stderr and every file of the vault, with
// its mode and, for a link, where it points, character for character. They were recorded from the program as it
// stood before the pages moved behind a port (Phase 9, L3 slice 3), and that change did not alter one byte of
// them. Two were re-recorded afterwards, by the owner's decision of 2026-10-10: 04 and 05, a page that is a link
// to nothing or to a directory, which used to make every promotion and every sync fail for every observation and
// now is skipped and named, as the doctor names it, while the observations that no readable page is the page of
// are left alone (exit 6, nothing written for them). 12, the pages directory that is itself a file, stays a hard
// error as it was, and 24 and 25 were recorded with that decision: an unreadable page beside the page of an observation.
//
// A page that cannot be read because of its mode is not a scenario: a test run as the superuser reads it, so its
// output would depend on who ran the test. A link to nothing and a link to a directory cannot be read by anyone,
// and stand for it.
//
// To rewrite them after a change that is MEANT to alter the output:
//
//	go test ./cmd/longterm-mem -run TestPromotePagesGolden -update-promote-golden
//
// and read the diff before committing it.
type pagesScenario struct {
	name string
	// prepare runs first, against the vault as built, and is not recorded.
	prepare [][]string
	// seed leaves the world in the state the scenario is about, after prepare.
	seed func(t *testing.T, w *promoteGoldenWorld)
	// vaultAs, when set, is the path the commands are told the vault is at, in place of its root: a link to
	// it, a path that is not there. What the golden file records of the vault is still the vault as built.
	vaultAs func(t *testing.T, w *promoteGoldenWorld) string
	// commands are what the golden file records.
	commands [][]string
}

// pageOf is the path of the page promoted under address.
func pageOf(w *promoteGoldenWorld, address string) string {
	return filepath.Join(w.vaultRoot, "wiki", "memory", address+".md")
}

// writeVaultFile writes content to the vault-relative path rel, creating its directories.
func writeVaultFile(t *testing.T, w *promoteGoldenWorld, rel, content string) {
	t.Helper()
	path := filepath.Join(w.vaultRoot, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// movePageToNotes replaces the page promoted under address by a link to the same file under notes/.
func movePageToNotes(t *testing.T, w *promoteGoldenWorld, address string) {
	t.Helper()
	page := pageOf(w, address)
	data, err := os.ReadFile(page)
	if err != nil {
		t.Fatal(err)
	}
	writeVaultFile(t, w, "notes/"+address+".md", string(data))
	if err := os.Remove(page); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, "../../notes/"+address+".md", page)
}

// replacePageWithLink removes the page promoted under address and puts a link to target in its place.
func replacePageWithLink(t *testing.T, w *promoteGoldenWorld, address, target string) {
	t.Helper()
	page := pageOf(w, address)
	if err := os.Remove(page); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, target, page)
}

// appendToPage adds a line to the page promoted under address, as a person editing it would.
func appendToPage(t *testing.T, w *promoteGoldenWorld, address string) {
	t.Helper()
	page := pageOf(w, address)
	data, err := os.ReadFile(page)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(page, append(data, []byte("\nA line a person added.\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
}

// removeSidecar deletes longterm-mem's own record of what it wrote, so that no page is known to be its own.
func removeSidecar(t *testing.T, w *promoteGoldenWorld) {
	t.Helper()
	if err := os.Remove(filepath.Join(w.vaultRoot, ".raw", ".longterm-mem-manifest.json")); err != nil {
		t.Fatal(err)
	}
}

func pagesScenarios() []pagesScenario {
	promote := []string{"promote", "--project", promoteGoldenProject, "--id", "1"}
	sync := []string{"sync", "--project", promoteGoldenProject}
	reconcile := []string{"promote", "reconcile", "--project", promoteGoldenProject, "c-000901"}
	revise := func(t *testing.T, w *promoteGoldenWorld) {
		w.execSQL(t, `UPDATE observations SET revision_count = 2, content = 'Revised body.' WHERE id = 1`)
	}
	withoutAddress := "---\nengram_id: 1\nproject: " + promoteGoldenProject + "\n---\nA page that names no address.\n"
	withBadRevision := "---\naddress: c-000500\nengram_id: 1\nengram_revision: x\nproject: " + promoteGoldenProject + "\n---\nA page whose revision is not a number.\n"

	return []pagesScenario{
		{name: "01-the-page-is-gone-but-the-sidecar-remembers-it", prepare: [][]string{promote}, seed: func(t *testing.T, w *promoteGoldenWorld) {
			if err := os.Remove(pageOf(w, "c-000901")); err != nil {
				t.Fatal(err)
			}
		}, commands: [][]string{promote, sync}},
		{name: "02-the-page-is-a-directory", seed: func(t *testing.T, w *promoteGoldenWorld) {
			if err := os.MkdirAll(pageOf(w, "c-000901"), 0o755); err != nil {
				t.Fatal(err)
			}
		}, commands: [][]string{promote, sync}},
		{name: "03-the-page-is-a-link-to-another-file-of-the-vault", prepare: [][]string{promote}, seed: func(t *testing.T, w *promoteGoldenWorld) {
			movePageToNotes(t, w, "c-000901")
			revise(t, w)
		}, commands: [][]string{promote, sync}},
		{name: "04-the-page-is-a-link-to-nothing", prepare: [][]string{promote}, seed: func(t *testing.T, w *promoteGoldenWorld) {
			replacePageWithLink(t, w, "c-000901", "../../notes/absent.md")
		}, commands: [][]string{promote, sync}},
		{name: "05-the-page-is-a-link-to-a-directory", prepare: [][]string{promote}, seed: func(t *testing.T, w *promoteGoldenWorld) {
			if err := os.MkdirAll(filepath.Join(w.vaultRoot, "notes", "folder"), 0o755); err != nil {
				t.Fatal(err)
			}
			replacePageWithLink(t, w, "c-000901", "../../notes/folder")
		}, commands: [][]string{promote, sync}},
		{name: "06-the-page-was-edited-by-hand-and-the-observation-revised", prepare: [][]string{promote}, seed: func(t *testing.T, w *promoteGoldenWorld) {
			appendToPage(t, w, "c-000901")
			revise(t, w)
		}, commands: [][]string{promote, sync}},
		{name: "07-an-edited-page-whose-observation-was-then-deleted", prepare: [][]string{promote}, seed: func(t *testing.T, w *promoteGoldenWorld) {
			appendToPage(t, w, "c-000901")
			w.execSQL(t, `UPDATE observations SET deleted_at = '2026-08-02 00:00:00' WHERE id = 1`)
		}, commands: [][]string{sync}},
		{name: "08-text-without-frontmatter-where-the-new-page-belongs", seed: func(t *testing.T, w *promoteGoldenWorld) {
			writeVaultFile(t, w, "wiki/memory/c-000901.md", "Just text, no frontmatter.\n")
		}, commands: [][]string{promote, sync}},
		{name: "09-a-page-for-the-observation-that-names-no-address", seed: func(t *testing.T, w *promoteGoldenWorld) {
			writeVaultFile(t, w, "wiki/memory/c-000500.md", withoutAddress)
		}, commands: [][]string{promote, sync}},
		{name: "10-a-page-for-the-observation-with-a-revision-that-is-not-a-number", seed: func(t *testing.T, w *promoteGoldenWorld) {
			writeVaultFile(t, w, "wiki/memory/c-000500.md", withBadRevision)
		}, commands: [][]string{promote, sync}},
		{name: "11-an-observation-moved-to-another-project-supersedes-its-old-page", prepare: [][]string{promote}, seed: func(t *testing.T, w *promoteGoldenWorld) {
			w.execSQL(t, `UPDATE observations SET project = 'other-project' WHERE id = 1`)
		}, commands: [][]string{
			{"promote", "--project", "other-project", "--id", "1"},
			{"sync", "--project", "other-project"},
		}},
		{name: "12-the-pages-directory-is-a-file", seed: func(t *testing.T, w *promoteGoldenWorld) {
			writeVaultFile(t, w, "wiki/memory", "not a directory")
		}, commands: [][]string{promote, sync}},
		{name: "13-the-pages-directory-is-a-link-to-a-directory", seed: func(t *testing.T, w *promoteGoldenWorld) {
			if err := os.MkdirAll(filepath.Join(w.vaultRoot, "notes", "pages"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(w.vaultRoot, "wiki"), 0o755); err != nil {
				t.Fatal(err)
			}
			symlinkOrSkip(t, "../notes/pages", filepath.Join(w.vaultRoot, "wiki", "memory"))
		}, commands: [][]string{promote, sync}},
		{name: "14-the-pages-directory-is-a-link-to-nothing", seed: func(t *testing.T, w *promoteGoldenWorld) {
			if err := os.MkdirAll(filepath.Join(w.vaultRoot, "wiki"), 0o755); err != nil {
				t.Fatal(err)
			}
			symlinkOrSkip(t, "../notes/absent", filepath.Join(w.vaultRoot, "wiki", "memory"))
		}, commands: [][]string{promote, sync}},
		{name: "15-files-beside-the-pages-that-are-not-pages", seed: func(t *testing.T, w *promoteGoldenWorld) {
			writeVaultFile(t, w, "wiki/memory/notes.txt", "---\nengram_id: 1\n---\n")
			writeVaultFile(t, w, "wiki/memory/C-000999.MD", "---\nengram_id: 1\n---\n")
			if err := os.MkdirAll(filepath.Join(w.vaultRoot, "wiki", "memory", "folder.md"), 0o755); err != nil {
				t.Fatal(err)
			}
		}, commands: [][]string{promote, sync}},
		{name: "16-reconcile-a-page-that-is-gone", commands: [][]string{reconcile}},
		{name: "17-reconcile-a-page-without-frontmatter", prepare: [][]string{promote}, seed: func(t *testing.T, w *promoteGoldenWorld) {
			writeVaultFile(t, w, "wiki/memory/c-000901.md", "Just text, no frontmatter.\n")
		}, commands: [][]string{reconcile}},
		{name: "18-reconcile-an-edited-page-nobody-recorded-adopts-it", prepare: [][]string{promote}, seed: func(t *testing.T, w *promoteGoldenWorld) {
			removeSidecar(t, w)
			appendToPage(t, w, "c-000901")
		}, commands: [][]string{reconcile}},
		{name: "19-reconcile-an-edited-page-with-a-recorded-revision-is-refused", prepare: [][]string{promote}, seed: func(t *testing.T, w *promoteGoldenWorld) {
			appendToPage(t, w, "c-000901")
		}, commands: [][]string{reconcile}},
		{name: "20-reconcile-a-page-that-is-a-link", prepare: [][]string{promote}, seed: func(t *testing.T, w *promoteGoldenWorld) {
			removeSidecar(t, w)
			movePageToNotes(t, w, "c-000901")
		}, commands: [][]string{reconcile}},
		{name: "21-reconcile-a-page-that-is-a-directory", seed: func(t *testing.T, w *promoteGoldenWorld) {
			if err := os.MkdirAll(pageOf(w, "c-000901"), 0o755); err != nil {
				t.Fatal(err)
			}
		}, commands: [][]string{reconcile}},
		{name: "22-reconcile-a-page-of-another-project", prepare: [][]string{promote}, seed: func(t *testing.T, w *promoteGoldenWorld) {
			removeSidecar(t, w)
			w.execSQL(t, `UPDATE observations SET project = 'other-project' WHERE id = 1`)
		}, commands: [][]string{{"promote", "reconcile", "--project", "other-project", "c-000901"}}},
		{name: "23-an-observation-superseded-by-a-newer-one", prepare: [][]string{sync}, seed: func(t *testing.T, w *promoteGoldenWorld) {
			w.execSQL(t, `UPDATE observations SET created_at = '2026-08-05 00:00:00' WHERE id = 2`)
			w.execSQL(t, `INSERT INTO memory_relations (sync_id, source_id, target_id, relation, judgment_status)
				VALUES ('rel-1', 'sync-2', 'sync-1', 'supersedes', 'judged')`)
		}, commands: [][]string{sync}},
		// A page that cannot be read beside the page of an observation that can: the observation is promoted as
		// ever, the page is named and skipped, and the observation no readable page is the page of is left alone,
		// because the unreadable page may be its own (decided 2026-10-10). The dry run predicts the run.
		{name: "24-a-page-that-cannot-be-read-beside-the-page-of-an-observation", prepare: [][]string{promote}, seed: func(t *testing.T, w *promoteGoldenWorld) {
			symlinkOrSkip(t, "../../notes/absent.md", pageOf(w, "c-000999"))
			revise(t, w)
		}, commands: [][]string{promote, {"sync", "--project", promoteGoldenProject, "--dry-run"}, sync}},
		// Every observation sync promotes has its page, and one more page cannot be read. The promotion pass
		// leaves no observation alone, so the run has no "nothing was written for them" warning. The propagation
		// pass still cannot tell the observations that have no page (the ones sync never promotes, 3 and 4) from
		// the unreadable one's, so it leaves those alone, "nothing was patched for them", and that warning is
		// what makes the run exit 6. The golden holds the whole vault: the pages, the sidecar, the catalog and
		// the log show that nothing was written for 3 and 4.
		{name: "25-a-page-that-cannot-be-read-beside-the-pages-of-every-promoted-observation", prepare: [][]string{sync}, seed: func(t *testing.T, w *promoteGoldenWorld) {
			symlinkOrSkip(t, "../../notes/absent.md", pageOf(w, "c-000999"))
		}, commands: [][]string{sync}},
		// An observation whose page cannot be told apart is not the only thing a run can meet. Here a page claims
		// observation 1 and names no address, so its promoted state cannot be read (a failure), and a page that
		// cannot be read withholds observation 2: the failure outranks the observations left alone, in the dry
		// run as in the run, which exit 1 where a run with only the withheld ones exits 6 (scenarios 24 and 25).
		{name: "26-a-failure-beside-an-observation-left-alone", seed: func(t *testing.T, w *promoteGoldenWorld) {
			writeVaultFile(t, w, "wiki/memory/c-000500.md", withoutAddress)
			symlinkOrSkip(t, "../../notes/absent.md", pageOf(w, "c-000999"))
		}, commands: [][]string{{"sync", "--project", promoteGoldenProject, "--dry-run"}, sync}},
	}
}

func TestPromotePagesGolden(t *testing.T) {
	runGoldenScenarios(t, "pages-golden", pagesScenarios())
}

// runGoldenScenarios runs each scenario in a world of its own and compares what the commands printed and the
// vault they left with the golden file of the scenario under testdata/<goldenSubdir>. The sets of golden files
// that hold the commands to the vault's files (the pages, the catalog and the log, the sync state) share it.
func runGoldenScenarios(t *testing.T, goldenSubdir string, scenarios []pagesScenario) {
	t.Helper()
	if !vault.PrerequisitePresent("python3") {
		t.Skip("python3 is not on PATH: the vault's index rebuild runs its scripts under it")
	}
	goldenDir, err := filepath.Abs(filepath.Join("testdata", goldenSubdir))
	if err != nil {
		t.Fatalf("resolve the golden directory: %v", err)
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			fixedUmask(t)
			w := newPromoteGoldenWorld(t)
			t.Setenv("HOME", w.home)
			t.Setenv("LONGTERM_MEM_ENGRAM_DB", w.dbPath)
			t.Setenv("LONGTERM_MEM_VAULT", w.vaultRoot)
			t.Setenv("LONGTERM_MEM_VAULTS_FILE", "")
			t.Chdir(t.TempDir())

			for _, args := range scenario.prepare {
				if code, stdout, stderr := runPromoteGolden(t, args); code != exitOK {
					t.Fatalf("preparing with %s: exit %d\nstdout: %s\nstderr: %s", strings.Join(args, " "), code, stdout, stderr)
				}
			}
			if scenario.seed != nil {
				scenario.seed(t, w)
			}
			if scenario.vaultAs != nil {
				t.Setenv("LONGTERM_MEM_VAULT", scenario.vaultAs(t, w))
			}

			var out strings.Builder
			for _, args := range scenario.commands {
				code, stdout, stderr := runPromoteGolden(t, args)
				fmt.Fprintf(&out, "args: %s\nexit: %d\n--- stdout ---\n%s--- stderr ---\n%s", strings.Join(args, " "), code, stdout, stderr)
			}
			out.WriteString("--- vault ---\n" + renderVaultWithModes(t, w.vaultRoot))
			checkPromoteGolden(t, goldenDir, scenario.name, temporaryFileName.ReplaceAllString(w.normalise(out.String()), ".tmp-<n>"))
		})
	}
}
