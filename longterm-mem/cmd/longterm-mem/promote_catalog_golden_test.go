package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The golden files under testdata/catalog-golden pin what `longterm-mem promote`, `sync`, `doctor` and `status`
// do to a vault whose catalog, wiki/index.md, and log, wiki/log.md, are in each state a file can be found in:
// written by hand, a link to another file, a link to nothing, a directory, a file the wiki directory is, a
// block of the catalog that was left open, a log that is not in order, an empty file. After the commands of a
// scenario they hold each command's exit code, stdout and stderr and every file of the vault, with its mode and,
// for a link, where it points, character for character. They were recorded from the program as it stood before
// the catalog and the log moved behind ports (Phase 9, L3 slice 4), and that change did not alter one byte of
// them.
//
// A file that cannot be read or written because of its mode is not a scenario: a test run as the superuser
// can, so its output would depend on who ran the test. A link to nothing, a directory and a file under a file
// cannot be written by anyone, and stand for it.
//
// To rewrite them after a change that is MEANT to alter the output:
//
//	go test ./cmd/longterm-mem -run TestPromoteCatalogGolden -update-promote-golden
//
// and read the diff before committing it.

// linkInVault makes the vault-relative path rel a link to target, creating the directory it is in.
func linkInVault(t *testing.T, w *promoteGoldenWorld, rel, target string) {
	t.Helper()
	path := filepath.Join(w.vaultRoot, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, target, path)
}

// directoryInVault makes the vault-relative path rel a directory, creating the ones above it.
func directoryInVault(t *testing.T, w *promoteGoldenWorld, rel string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(w.vaultRoot, filepath.FromSlash(rel)), 0o755); err != nil {
		t.Fatal(err)
	}
}

// removeFromVault removes the vault-relative path rel, whatever it is: a file, a link, a directory and what it
// holds.
func removeFromVault(t *testing.T, w *promoteGoldenWorld, rel string) {
	t.Helper()
	if err := os.RemoveAll(filepath.Join(w.vaultRoot, filepath.FromSlash(rel))); err != nil {
		t.Fatal(err)
	}
}

func catalogScenarios() []pagesScenario {
	promote := []string{"promote", "--project", promoteGoldenProject, "--id", "1"}
	sync := []string{"sync", "--project", promoteGoldenProject}
	openBlock := "<!-- longterm-mem:begin -->\n- [[c-000001|An entry above the end of the block]]\n"

	return []pagesScenario{
		{name: "01-an-index-and-a-log-written-by-hand", seed: func(t *testing.T, w *promoteGoldenWorld) {
			writeVaultFile(t, w, "wiki/index.md", "# My index\n\nNotes I wrote by hand.\n")
			writeVaultFile(t, w, "wiki/log.md", "# My log\n\n## [2026-01-01] ingest | Something\n\nA line.\n")
		}, commands: [][]string{promote, sync}},
		{name: "02-an-index-that-is-a-link-to-another-file", seed: func(t *testing.T, w *promoteGoldenWorld) {
			writeVaultFile(t, w, "notes/index.md", "# Index kept under notes\n")
			linkInVault(t, w, "wiki/index.md", "../notes/index.md")
		}, commands: [][]string{promote, sync}},
		{name: "03-an-index-that-is-a-link-to-nothing", seed: func(t *testing.T, w *promoteGoldenWorld) {
			linkInVault(t, w, "wiki/index.md", "../notes/absent.md")
		}, commands: [][]string{promote, sync}},
		{name: "04-an-index-that-is-a-directory", seed: func(t *testing.T, w *promoteGoldenWorld) {
			directoryInVault(t, w, "wiki/index.md")
		}, commands: [][]string{promote, sync}},
		{name: "05-a-block-of-the-index-that-was-left-open", seed: func(t *testing.T, w *promoteGoldenWorld) {
			writeVaultFile(t, w, "wiki/index.md", openBlock)
		}, commands: [][]string{promote, sync}},
		{name: "06-a-log-that-is-a-link-to-another-file", seed: func(t *testing.T, w *promoteGoldenWorld) {
			writeVaultFile(t, w, "notes/log.md", "# Log kept under notes\n")
			linkInVault(t, w, "wiki/log.md", "../notes/log.md")
		}, commands: [][]string{promote, sync}},
		{name: "07-a-log-that-is-a-link-to-nothing", seed: func(t *testing.T, w *promoteGoldenWorld) {
			linkInVault(t, w, "wiki/log.md", "../notes/absent.md")
		}, commands: [][]string{promote, sync}},
		{name: "08-a-log-that-is-a-directory", seed: func(t *testing.T, w *promoteGoldenWorld) {
			directoryInVault(t, w, "wiki/log.md")
		}, commands: [][]string{promote, sync}},
		{name: "09-a-wiki-directory-that-is-a-file", seed: func(t *testing.T, w *promoteGoldenWorld) {
			writeVaultFile(t, w, "wiki", "not a directory")
		}, commands: [][]string{promote, sync}},
	}
}

func TestPromoteCatalogGolden(t *testing.T) {
	runGoldenScenarios(t, "catalog-golden", catalogScenarios())
}
