package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The golden files recorded here, under testdata/syncstate-golden beside those of promote_syncstate_golden_test.go
// (they share the directory, and the header there says how they were recorded and how to rewrite them), pin
// what `longterm-mem promote`, `sync`, `status` and `doctor` do when the record of the last sync is a link to a
// file or to nothing, when .vault-meta is a link to a directory, and when the path they are told the vault is at
// is a link to it, is not there, or is a file.

func vaultLinksScenarios() []pagesScenario {
	status := []string{"status", "--project", promoteGoldenProject}
	doctor := []string{"doctor", "--project", promoteGoldenProject}
	promote := []string{"promote", "--project", promoteGoldenProject, "--id", "1"}
	sync := []string{"sync", "--project", promoteGoldenProject}
	statusSyncStatus := [][]string{status, sync, status}

	return []pagesScenario{
		{name: "08-a-record-that-is-a-link-to-a-file", seed: func(t *testing.T, w *promoteGoldenWorld) {
			writeVaultFile(t, w, "notes/state.json", earlierSync)
			linkInVault(t, w, syncStateFile, "../notes/state.json")
		}, commands: statusSyncStatus},
		{name: "09-a-record-that-is-a-link-to-nothing", seed: func(t *testing.T, w *promoteGoldenWorld) {
			linkInVault(t, w, syncStateFile, "../notes/absent.json")
		}, commands: statusSyncStatus},
		{name: "11-a-vault-meta-directory-that-is-a-link-to-a-directory", seed: func(t *testing.T, w *promoteGoldenWorld) {
			writeVaultFile(t, w, "notes/meta/longterm-mem-sync-state.json", earlierSync)
			symlinkOrSkip(t, "notes/meta", filepath.Join(w.vaultRoot, ".vault-meta"))
		}, commands: statusSyncStatus},
		{name: "12-a-vault-root-that-is-a-link-to-the-vault", vaultAs: func(t *testing.T, w *promoteGoldenWorld) string {
			link := filepath.Join(w.home, "vault-link")
			symlinkOrSkip(t, w.vaultRoot, link)
			return link
		}, commands: [][]string{promote, sync, status, doctor}},
		{name: "13-a-vault-root-that-is-not-there", vaultAs: func(t *testing.T, w *promoteGoldenWorld) string {
			return filepath.Join(w.vaultRoot, "absent")
		}, commands: [][]string{promote, sync, status, doctor}},
		{name: "14-a-vault-root-that-is-a-file", vaultAs: func(t *testing.T, w *promoteGoldenWorld) string {
			file := filepath.Join(w.home, "vault-file")
			if err := os.WriteFile(file, []byte("not a directory"), 0o644); err != nil {
				t.Fatal(err)
			}
			return file
		}, commands: [][]string{promote, sync, status, doctor}},
	}
}

func TestPromoteVaultLinksGolden(t *testing.T) {
	runGoldenScenarios(t, "syncstate-golden", vaultLinksScenarios())
}
