package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// The golden files under testdata/query-golden pin what `longterm-mem query` prints for an Engram
// database: the exit code, stdout and stderr of each case, character for character. They were
// recorded from the program as it stood before the engram domain model moved to internal/memory
// (Phase 9, L1), and that move must not change one byte of them. They are the guard of the engram
// reader's observable behaviour: how a query is tokenized and widened, which rows are excluded, what
// a snippet looks like (its budget, its cut, the marks that say it was cut), and what the relation
// ledger says about a row.
//
// To rewrite them after a change that is MEANT to alter the output:
//
//	go test ./cmd/longterm-mem -run TestQueryGolden -update-query-golden
//
// and read the diff before committing it.
var updateQueryGolden = flag.Bool("update-query-golden", false, "rewrite the golden files of the query command")

// queryGoldenProject is the project the seed writes its observations under.
const queryGoldenProject = "goldenproj"

type queryGoldenCase struct {
	name string
	args []string
}

func queryGoldenCases() []queryGoldenCase {
	query := func(rest ...string) []string {
		return append([]string{"query", "--project", queryGoldenProject}, rest...)
	}
	return []queryGoldenCase{
		{"text-a-needle-in-a-long-multibyte-body", query("dragonscale")},
		{"json-a-needle-in-a-long-multibyte-body", query("--json", "dragonscale")},
		{"json-a-natural-question-is-widened", query("--json", "what conventions apply when editing the register writer")},
		{"text-two-words-both-required", query("canonical identity")},
		{"json-types-left-out", query("--exclude-types", "session_summary,decision", "--json", "dragonscale")},
		{"text-a-superseded-decision-says-so", query("alpha-cache")},
		{"json-a-superseded-decision-says-so", query("--json", "cache")},
		{"text-a-conflict-names-both-sides", query("gamma-conflict")},
		{"json-an-undecided-conflict-is-not-silence", query("--json", "delta-pending")},
		{"json-one-row-of-a-multibyte-body", query("--json", "--top", "1", "zebracorn")},
		{"json-only-stopwords-still-search", query("--json", "the and of")},
		{"json-nothing-matches", query("--json", "nonexistentxyz")},
		{"json-a-row-without-a-sync-id", query("--json", "epsilon-legacy")},
		{"text-a-withdrawn-relation-is-not-reported", query("zeta-withdrawn")},
		{"text-a-query-that-starts-with-a-dash", query("--", "-foo")},
		{"json-any-token-when-all-find-nothing", query("--json", "dragonscale zebracorn unseenwordq")},
		{"json-another-project-sees-only-its-own", []string{"query", "--project", "otherproj", "--json", "dragonscale"}},
		{"usage-no-query-text", query()},
	}
}

func TestQueryGolden(t *testing.T) {
	dbPath := queryGoldenDatabase(t)
	// Absolute, because each case changes the working directory.
	goldenDir, err := filepath.Abs(filepath.Join("testdata", "query-golden"))
	if err != nil {
		t.Fatalf("resolve the golden directory: %v", err)
	}
	for _, tc := range queryGoldenCases() {
		t.Run(tc.name, func(t *testing.T) {
			// The environment is set per case and torn down with it: nothing here reads the
			// owner's HOME, vault registry or Engram database.
			t.Setenv("HOME", t.TempDir())
			t.Setenv("LONGTERM_MEM_ENGRAM_DB", dbPath)
			t.Setenv("LONGTERM_MEM_VAULT", t.TempDir())
			t.Setenv("LONGTERM_MEM_VAULTS_FILE", "")
			// A directory that is not a repository: the correspondence warning between --project
			// and the working directory is not part of what is pinned here.
			t.Chdir(t.TempDir())

			code, stdout, stderr := runCaptured(t, tc.args)
			got := fmt.Sprintf("args: %s\nexit: %d\n--- stdout ---\n%s--- stderr ---\n%s", strings.Join(tc.args, " "), code, stdout, stderr)
			checkQueryGolden(t, goldenDir, tc.name, got)
		})
	}
}

// queryGoldenDatabase builds the Engram database the cases read, from the schema fixture of the engram
// package and testdata/query-golden/seed.sql, under a temporary directory.
func queryGoldenDatabase(t *testing.T) string {
	t.Helper()
	schema, err := os.ReadFile(filepath.Join("..", "..", "internal", "engram", "testdata", "schema.sql"))
	if err != nil {
		t.Fatalf("read the engram schema fixture: %v", err)
	}
	seed, err := os.ReadFile(filepath.Join("testdata", "query-golden", "seed.sql"))
	if err != nil {
		t.Fatalf("read the seed: %v", err)
	}
	path := filepath.Join(t.TempDir(), "engram.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open the fixture database: %v", err)
	}
	defer db.Close()
	// The schema comes first: the seed inserts into its tables.
	for _, step := range []struct{ name, script string }{{"schema", string(schema)}, {"seed", string(seed)}} {
		if _, err := db.Exec(step.script); err != nil {
			t.Fatalf("apply the %s: %v", step.name, err)
		}
	}
	return path
}

// runCaptured runs the command in process and returns its exit code, stdout and stderr.
func runCaptured(t *testing.T, args []string) (code int, stdout, stderr string) {
	t.Helper()
	stdout, stderr = captureStreams(t, func() { code = run(args) })
	return code, stdout, stderr
}

// captureStreams runs fn with os.Stdout and os.Stderr redirected to files and returns what fn wrote to
// each. The streams are restored by a deferred call, so a command that panics does not leave them
// pointing at the capture files for the rest of the process (the test framework reports through them).
func captureStreams(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()
	dir := t.TempDir()
	outFile, err := os.Create(filepath.Join(dir, "stdout"))
	if err != nil {
		t.Fatalf("create the stdout capture: %v", err)
	}
	errFile, err := os.Create(filepath.Join(dir, "stderr"))
	if err != nil {
		t.Fatalf("create the stderr capture: %v", err)
	}
	func() {
		realOut, realErr := os.Stdout, os.Stderr
		defer func() { os.Stdout, os.Stderr = realOut, realErr }()
		os.Stdout, os.Stderr = outFile, errFile
		fn()
	}()
	if err := outFile.Close(); err != nil {
		t.Fatalf("close the stdout capture: %v", err)
	}
	if err := errFile.Close(); err != nil {
		t.Fatalf("close the stderr capture: %v", err)
	}
	outData, err := os.ReadFile(filepath.Join(dir, "stdout"))
	if err != nil {
		t.Fatalf("read the stdout capture: %v", err)
	}
	errData, err := os.ReadFile(filepath.Join(dir, "stderr"))
	if err != nil {
		t.Fatalf("read the stderr capture: %v", err)
	}
	return string(outData), string(errData)
}

// checkQueryGolden compares got with the golden file of the case, or rewrites that file when the update
// flag is given.
func checkQueryGolden(t *testing.T, dir, name, got string) {
	t.Helper()
	path := filepath.Join(dir, name+".golden")
	if *updateQueryGolden {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden file: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden file: %v (record it with -update-query-golden)", err)
	}
	if string(want) != got {
		t.Errorf("the output of %q changed from its golden file %s:\n--- want ---\n%s\n--- got ---\n%s", name, path, want, got)
	}
}
