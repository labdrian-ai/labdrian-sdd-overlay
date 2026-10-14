package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The golden files under testdata/failures-golden pin what `sync`, `promote` and `index --embeddings` print and
// leave when something they depend on fails: the exit code each failure is given, the words of stderr, what the
// run printed on stdout before it failed, and the vault after it. The other sets of golden files pin the commands
// where they succeed, and the exit codes of a few failures; these pin the whole of the rest, because the
// orchestration of a sync (both passes always run, the failures of the two are reported together, the exit code
// is chosen by what failed) was code in the command package and moved into use cases in Phase 9, L8, which must
// not change one byte of what an operator sees. They were recorded from the program as it stood before that move.
//
// To rewrite them after a change that is MEANT to alter the output:
//
//	go test ./cmd/longterm-mem -run TestFailuresGolden -update-promote-golden
//
// and read the diff before committing it.
func TestFailuresGolden(t *testing.T) {
	sync := []string{"sync", "--project", promoteGoldenProject}
	dryRun := []string{"sync", "--project", promoteGoldenProject, "--dry-run"}
	promote := []string{"promote", "--project", promoteGoldenProject, "--id", "1"}
	build := []string{"index", "--project", promoteGoldenProject, "--embeddings", "--embed-dimension", "3"}

	// nowhere is a path that names nothing, under the temporary directory of the world, so that the words of an
	// error are the same wherever the world is (the world's own directories are masked).
	nowhere := func(w *promoteGoldenWorld, name string) string { return filepath.Join(w.home, "no-such-place", name) }

	runGoldenScenarios(t, "failures-golden", []pagesScenario{
		// Engram answers the listing and then cannot answer what propagation asks of it: both passes run, the
		// first one's work stays, and the exit code is the one for a memory that cannot be read.
		{name: "01-sync-when-the-relations-of-memory-cannot-be-read", seed: func(t *testing.T, w *promoteGoldenWorld) {
			w.execSQL(t, `DROP TABLE memory_relations`)
		}, commands: [][]string{sync}},

		// Engram cannot list the observations at all: nothing is promoted, and the dry run, which asks the same
		// question, fails with the code of the run that could not plan, not of the memory.
		{name: "02-sync-when-the-observations-cannot-be-listed", seed: func(t *testing.T, w *promoteGoldenWorld) {
			w.execSQL(t, `DROP TABLE observations`)
		}, commands: [][]string{sync, dryRun}},

		// One observation whose page cannot be decided about fails on every run; the others are promoted and the
		// failure is the run's, with the code for a fault of longterm-mem's own.
		{name: "03-sync-with-an-observation-that-cannot-be-decided-about", prepare: [][]string{sync}, seed: func(t *testing.T, w *promoteGoldenWorld) {
			page := pageOf(w, "c-000901")
			data, err := os.ReadFile(page)
			if err != nil {
				t.Fatal(err)
			}
			broken := strings.Replace(string(data), "engram_revision: 1", "engram_revision: not-a-number", 1)
			if broken == string(data) {
				t.Fatalf("the page has no engram_revision to break:\n%s", data)
			}
			if err := os.WriteFile(page, []byte(broken), 0o644); err != nil {
				t.Fatal(err)
			}
			w.execSQL(t, `UPDATE observations SET revision_count = 2 WHERE id = 2`)
		}, commands: [][]string{sync, dryRun}},

		// longterm-mem's own record of what it wrote cannot be read: no command that writes gets as far as a page.
		{name: "04-sync-and-promote-with-a-record-that-does-not-parse", seed: func(t *testing.T, w *promoteGoldenWorld) {
			writeVaultFile(t, w, ".raw/.longterm-mem-manifest.json", "{ not json")
		}, commands: [][]string{sync, promote, dryRun}},

		// The vault the commands are told of is not there.
		{name: "05-a-vault-that-is-not-there", vaultAs: func(t *testing.T, w *promoteGoldenWorld) string {
			return nowhere(w, "vault")
		}, commands: [][]string{sync, dryRun, promote}},

		// Engram's database cannot be opened.
		{name: "06-an-engram-database-that-cannot-be-opened", env: func(t *testing.T, w *promoteGoldenWorld) {
			t.Setenv("LONGTERM_MEM_ENGRAM_DB", nowhere(w, "engram.db"))
		}, commands: [][]string{sync, dryRun, promote, build}},

		// The embedding index cannot be built from observations that cannot be listed.
		{name: "07-an-embedding-index-from-observations-that-cannot-be-listed", seed: func(t *testing.T, w *promoteGoldenWorld) {
			w.execSQL(t, `DROP TABLE observations`)
		}, commands: [][]string{build}},
	})
}
