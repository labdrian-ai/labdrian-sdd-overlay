package main

import "testing"

// The golden files under testdata/syncstate-golden pin what `longterm-mem sync` and `status` do about the
// record of the last sync, .vault-meta/longterm-mem-sync-state.json, in each state the record can be found in: no
// record yet, a record of an earlier sync, a record that is not JSON, is empty, is null or does not hold a
// completion time, a record that is a directory, and a .vault-meta that is a file. (The record as a link, and the
// vault root as a link, a path that is not there and a file, are in promote_vaultlinks_golden_test.go, which
// writes to the same directory.) After the commands of a scenario they hold each command's exit code, stdout and
// stderr and every file of the vault, with its mode and, for a link, where it points, character for character.
// They were recorded from the program as it stood before the sync state moved behind ports (Phase 9, L3
// slice 4), and that change did not alter one byte of them.
//
// A record that cannot be read or written because of its mode is not a scenario: a test run as the superuser
// can, so its output would depend on who ran the test. A directory, a link to nothing and a file under a file
// cannot be written by anyone, and stand for it.
//
// To rewrite them after a change that is MEANT to alter the output:
//
//	go test ./cmd/longterm-mem -run 'TestPromoteSyncState|TestPromoteVaultLinks' -update-promote-golden
//
// and read the diff before committing it.

// syncStateFile is the vault-relative path of the record of the last sync.
const syncStateFile = ".vault-meta/longterm-mem-sync-state.json"

// earlierSync is the record an earlier sync left.
const earlierSync = "{\n  \"schema\": 1,\n  \"last_sync_completed_at\": \"2026-01-01T00:00:00Z\"\n}\n"

func syncStateScenarios() []pagesScenario {
	status := []string{"status", "--project", promoteGoldenProject}
	sync := []string{"sync", "--project", promoteGoldenProject}
	dryRun := []string{"sync", "--project", promoteGoldenProject, "--dry-run"}
	statusSyncStatus := [][]string{status, sync, status}

	return []pagesScenario{
		{name: "01-no-sync-yet-a-dry-run-and-a-sync", commands: [][]string{status, dryRun, status, sync, status}},
		{name: "02-the-record-of-an-earlier-sync", seed: func(t *testing.T, w *promoteGoldenWorld) {
			writeVaultFile(t, w, syncStateFile, earlierSync)
		}, commands: statusSyncStatus},
		{name: "03-a-record-that-is-not-json", seed: func(t *testing.T, w *promoteGoldenWorld) {
			writeVaultFile(t, w, syncStateFile, "{not json")
		}, commands: statusSyncStatus},
		{name: "04-an-empty-record", seed: func(t *testing.T, w *promoteGoldenWorld) {
			writeVaultFile(t, w, syncStateFile, "")
		}, commands: statusSyncStatus},
		{name: "05-a-record-that-is-null", seed: func(t *testing.T, w *promoteGoldenWorld) {
			writeVaultFile(t, w, syncStateFile, "null")
		}, commands: statusSyncStatus},
		{name: "06-a-record-whose-completion-is-not-a-string", seed: func(t *testing.T, w *promoteGoldenWorld) {
			writeVaultFile(t, w, syncStateFile, `{"last_sync_completed_at":5}`)
		}, commands: statusSyncStatus},
		{name: "07-a-record-that-is-a-directory", seed: func(t *testing.T, w *promoteGoldenWorld) {
			directoryInVault(t, w, syncStateFile)
		}, commands: statusSyncStatus},
		{name: "10-a-vault-meta-directory-that-is-a-file", seed: func(t *testing.T, w *promoteGoldenWorld) {
			writeVaultFile(t, w, ".vault-meta", "not a directory")
		}, commands: statusSyncStatus},
	}
}

func TestPromoteSyncStateGolden(t *testing.T) {
	runGoldenScenarios(t, "syncstate-golden", syncStateScenarios())
}
