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
// stood before the pages moved behind a port (Phase 9, L3 slice 3), and that change must not alter one byte of
// them.
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

func pagesScenarios() []pagesScenario {
	promote := []string{"promote", "--project", promoteGoldenProject, "--id", "1"}
	sync := []string{"sync", "--project", promoteGoldenProject}
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
	}
}

func TestPromotePagesGolden(t *testing.T) {
	if !vault.PrerequisitePresent("python3") {
		t.Skip("python3 is not on PATH: the vault's index rebuild runs its scripts under it")
	}
	goldenDir, err := filepath.Abs(filepath.Join("testdata", "pages-golden"))
	if err != nil {
		t.Fatalf("resolve the golden directory: %v", err)
	}
	for _, scenario := range pagesScenarios() {
		t.Run(scenario.name, func(t *testing.T) {
			fixedUmask(t)
			w := newPromoteGoldenWorld(t)
			t.Setenv("HOME", w.home)
			t.Setenv("LONGTERM_MEM_ENGRAM_DB", w.dbPath)
			t.Setenv("LONGTERM_MEM_VAULT", w.vaultRoot)
			t.Setenv("LONGTERM_MEM_VAULTS_FILE", "")
			t.Chdir(t.TempDir())

			for _, args := range scenario.prepare {
				if code, stdout, stderr := runCaptured(t, args); code != exitOK {
					t.Fatalf("preparing with %s: exit %d\nstdout: %s\nstderr: %s", strings.Join(args, " "), code, stdout, stderr)
				}
			}
			if scenario.seed != nil {
				scenario.seed(t, w)
			}

			var out strings.Builder
			for _, args := range scenario.commands {
				code, stdout, stderr := runCaptured(t, args)
				fmt.Fprintf(&out, "args: %s\nexit: %d\n--- stdout ---\n%s--- stderr ---\n%s", strings.Join(args, " "), code, stdout, stderr)
			}
			out.WriteString("--- vault ---\n" + renderVaultWithModes(t, w.vaultRoot))
			checkPromoteGolden(t, goldenDir, scenario.name, temporaryFileName.ReplaceAllString(w.normalise(out.String()), ".tmp-<n>"))
		})
	}
}
