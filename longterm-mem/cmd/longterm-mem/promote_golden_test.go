package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vault"
)

// The golden files under testdata/promote-golden pin what `longterm-mem promote` and `longterm-mem sync`
// do to a vault: after each step of one scenario, the exit code, stdout and stderr of the command and
// every file the vault holds, character for character. They were recorded from the program as it stood
// before the promote package took its address allocator and its clock as injected ports (Phase 9, L2),
// and that change must not alter one byte of them, apart from the day the test
// runs (in UTC) and the instants of that day, which are replaced by a marker; any other date stays
// as written, so a clock that answered a different day would fail them.
//
// To rewrite them after a change that is MEANT to alter the output:
//
//	go test ./cmd/longterm-mem -run TestPromoteGolden -update-promote-golden
//
// and read the diff before committing it.
var updatePromoteGolden = flag.Bool("update-promote-golden", false, "rewrite the golden files of promote and sync")

const promoteGoldenProject = "golden-project"

// promoteGoldenAllocator is a vault allocator script that hands out c-000901, c-000902, ... from a
// counter kept in the vault, so the scenario allocates a different address for each page.
const promoteGoldenAllocator = `#!/bin/sh
n=900
[ -f .allocator-counter ] && read n < .allocator-counter
n=$((n+1))
echo "$n" > .allocator-counter
printf 'c-%06d\n' "$n"
`

// promoteGoldenInstant is an instant the clock writes, to the second and in UTC: the sentinel of the
// provisioned index and the completion time of a sync.
var promoteGoldenInstant = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z`)

type promoteGoldenStep struct {
	name string
	// before runs against the world first: it edits the Engram database or a page, as a person would.
	before func(t *testing.T, w *promoteGoldenWorld)
	args   []string
}

type promoteGoldenWorld struct {
	vaultRoot string
	dbPath    string
	home      string
	// began is when the world was built; now reads the clock when the output is normalised. Every UTC
	// day from one to the other is a day the run can have written.
	began time.Time
	now   func() time.Time
}

// realTempDir is a fresh temporary directory named by its resolved path: /var is /private/var on macOS,
// and the path the program prints must be the one normalise replaces, whichever way the program names it.
func realTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve the temporary directory: %v", err)
	}
	return dir
}

// TestPromoteGoldenNormalise_ARunThatCrossesMidnightStillMatches: the days a run can have written are
// every UTC day from the moment the world was built to the moment the output is normalised, not only the
// day the normalising happens to run on. A run that starts before 00:00 UTC and ends after it wrote
// dates of both days, and neither may make the comparison fail; a day outside that window still must.
func TestPromoteGoldenNormalise_ARunThatCrossesMidnightStillMatches(t *testing.T) {
	now := time.Date(2026, 8, 2, 0, 0, 5, 0, time.UTC)
	w := &promoteGoldenWorld{vaultRoot: "/vault", home: "/home", dbPath: "/db", began: now.Add(-10 * time.Second), now: func() time.Time { return now }}

	got := w.normalise("created: 2026-08-01\nupdated: 2026-08-02\nlogged 2026-08-01T23:59:58Z, synced 2026-08-02T00:00:04Z\nstale: 2026-07-31 2026-07-31T10:00:00Z\n")
	want := "created: <today>\nupdated: <today>\nlogged <now>, synced <now>\nstale: 2026-07-31 2026-07-31T10:00:00Z\n"
	if got != want {
		t.Errorf("normalise =\n%s\nwant\n%s", got, want)
	}
}

func promoteGoldenSteps() []promoteGoldenStep {
	promote := func(id string) []string {
		return []string{"promote", "--project", promoteGoldenProject, "--id", id}
	}
	sync := func(extra ...string) []string {
		return append([]string{"sync", "--project", promoteGoldenProject}, extra...)
	}
	return []promoteGoldenStep{
		{name: "01-promote-an-ineligible-observation-explicitly", args: promote("3")},
		{name: "02-promote-it-again", args: promote("3")},
		{name: "03-sync-dry-run", args: sync("--dry-run")},
		{name: "04-sync-promotes-the-eligible-ones", args: sync()},
		{name: "05-sync-with-nothing-new", args: sync()},
		{
			name: "06-sync-after-a-revision-and-a-soft-delete",
			before: func(t *testing.T, w *promoteGoldenWorld) {
				w.execSQL(t, `UPDATE observations SET revision_count = 2, content = 'Revised body.' WHERE id = 1`)
				w.execSQL(t, `UPDATE observations SET deleted_at = '2026-08-02 00:00:00' WHERE id = 2`)
			},
			args: sync(),
		},
		{
			name: "07-promote-a-page-edited-by-hand-is-refused",
			before: func(t *testing.T, w *promoteGoldenWorld) {
				page := filepath.Join(w.vaultRoot, "wiki", "memory", "c-000901.md")
				data, err := os.ReadFile(page)
				if err != nil {
					t.Fatalf("read the page to edit: %v", err)
				}
				if err := os.WriteFile(page, append(data, []byte("\nA line a person added.\n")...), 0o644); err != nil {
					t.Fatalf("edit the page: %v", err)
				}
			},
			args: promote("3"),
		},
		{name: "08-promote-an-observation-that-does-not-exist", args: promote("999")},
	}
}

func TestPromoteGolden(t *testing.T) {
	if !vault.PrerequisitePresent("python3") {
		t.Skip("python3 is not on PATH: the vault's index rebuild runs its scripts under it")
	}
	goldenDir, err := filepath.Abs(filepath.Join("testdata", "promote-golden"))
	if err != nil {
		t.Fatalf("resolve the golden directory: %v", err)
	}
	w := newPromoteGoldenWorld(t)
	// The environment is the test's own: nothing here reads the owner's HOME, vault registry or Engram
	// database.
	t.Setenv("HOME", w.home)
	t.Setenv("LONGTERM_MEM_ENGRAM_DB", w.dbPath)
	t.Setenv("LONGTERM_MEM_VAULT", w.vaultRoot)
	t.Setenv("LONGTERM_MEM_VAULTS_FILE", "")
	// A directory that is not a repository, so no warning about the working directory is part of the output.
	t.Chdir(t.TempDir())

	for _, step := range promoteGoldenSteps() {
		if step.before != nil {
			step.before(t, w)
		}
		code, stdout, stderr := runCaptured(t, step.args)
		got := w.normalise(fmt.Sprintf("args: %s\nexit: %d\n--- stdout ---\n%s--- stderr ---\n%s--- vault ---\n%s",
			strings.Join(step.args, " "), code, stdout, stderr, renderPromoteGoldenTree(t, w.vaultRoot)))
		checkPromoteGolden(t, goldenDir, step.name, got)
	}
}

func newPromoteGoldenWorld(t *testing.T) *promoteGoldenWorld {
	t.Helper()
	// The roots are the paths the program sees and prints; resolving their symlinks (/var is /private/var on
	// macOS) makes the path in the output the one normalise replaces, whichever way the program names it.
	w := &promoteGoldenWorld{vaultRoot: realTempDir(t), home: realTempDir(t), began: time.Now().UTC(), now: time.Now}
	writeExecutable := func(rel, body string) {
		path := filepath.Join(w.vaultRoot, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	writeExecutable("scripts/allocate-address.sh", promoteGoldenAllocator)
	writeExecutable("bin/setup-retrieve.sh", "#!/bin/sh\nexit 0\n")
	writeExecutable("scripts/contextual-prefix.py", "")
	writeExecutable("scripts/bm25-index.py", "")

	schema, err := os.ReadFile(filepath.Join("..", "..", "internal", "engram", "testdata", "schema.sql"))
	if err != nil {
		t.Fatalf("read the engram schema fixture: %v", err)
	}
	w.dbPath = filepath.Join(realTempDir(t), "engram.db")
	w.execSQL(t, string(schema))
	// Observation 1 is pinned, 2 carries a curated topic key, 3 is neither (only an explicit promotion
	// writes it), 4 is bookkeeping a sync leaves alone.
	for _, row := range []struct {
		syncID, kind, title, content, topicKey string
		pinned                                 int
	}{
		{"sync-1", "decision", "Pinned Decision", "A decision somebody pinned.", "", 1},
		{"sync-2", "decision", "Curated Note", "A note with a curated topic key.", "longterm-mem/curated-note", 0},
		{"sync-3", "discovery", "Below Threshold", "A discovery nobody curated.", "", 0},
		{"sync-4", "decision", "Process Bookkeeping", "Bookkeeping a sync leaves alone.", "sdd/some-change/proposal", 0},
	} {
		w.execSQL(t, `INSERT INTO observations (session_id, sync_id, type, title, content, project, revision_count, pinned, created_at, topic_key)
			VALUES ('sess-1', ?, ?, ?, ?, ?, 1, ?, '2026-08-01 00:00:00', NULLIF(?, ''))`,
			row.syncID, row.kind, row.title, row.content, promoteGoldenProject, row.pinned, row.topicKey)
	}
	return w
}

func (w *promoteGoldenWorld) execSQL(t *testing.T, statement string, args ...any) {
	t.Helper()
	db, err := sql.Open("sqlite", w.dbPath)
	if err != nil {
		t.Fatalf("open the fixture database: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(statement, args...); err != nil {
		t.Fatalf("execute %q: %v", statement, err)
	}
}

// normalise replaces what differs from one run to the next: the temporary paths and the day (or
// days) of the run.
func (w *promoteGoldenWorld) normalise(text string) string {
	text = strings.ReplaceAll(text, w.vaultRoot, "<vault>")
	text = strings.ReplaceAll(text, w.home, "<home>")
	text = strings.ReplaceAll(text, w.dbPath, "<engram-db>")
	// Only the UTC days the run can have written become a marker: any other date in the output (a clock
	// that answered the wrong day, or in local time near midnight) stays as written and fails the
	// comparison. A run that crosses 00:00 UTC wrote two days, and both are expected.
	days := map[string]bool{}
	for day := w.began.UTC().Truncate(24 * time.Hour); !day.After(w.now().UTC()); day = day.AddDate(0, 0, 1) {
		days[day.Format("2006-01-02")] = true
	}
	text = promoteGoldenInstant.ReplaceAllStringFunc(text, func(instant string) string {
		if days[instant[:len("2006-01-02")]] {
			return "<now>"
		}
		return instant
	})
	for day := range days {
		text = strings.ReplaceAll(text, day, "<today>")
	}
	return text
}

// renderPromoteGoldenTree lists every file of the vault, sorted, with its content.
func renderPromoteGoldenTree(t *testing.T, root string) string {
	t.Helper()
	snapshot := cmdVaultSnapshot(t, root)
	paths := make([]string, 0, len(snapshot))
	for path := range snapshot {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	var b strings.Builder
	for _, path := range paths {
		fmt.Fprintf(&b, "=== %s ===\n%s", filepath.ToSlash(path), snapshot[path])
		if !strings.HasSuffix(snapshot[path], "\n") {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// checkPromoteGolden compares got with the golden file of the step, or rewrites that file when the
// update flag is given.
func checkPromoteGolden(t *testing.T, dir, name, got string) {
	t.Helper()
	path := filepath.Join(dir, name+".golden")
	if *updatePromoteGolden {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create the golden directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden file: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden file: %v (record it with -update-promote-golden)", err)
	}
	if string(want) != got {
		t.Errorf("the step %q changed from its golden file %s:\n--- want ---\n%s\n--- got ---\n%s", name, path, want, got)
	}
}
