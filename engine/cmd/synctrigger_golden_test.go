//go:build unix

package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

// The golden files under testdata/synctrigger-golden record what 'sync-trigger' does: the exit
// code and the streams of the command a hook starts, the line the detached child appends to the
// log, what the child ran (the longterm-mem it found, where, and with which arguments), and the
// files the run left under the state directory. The command always exits 0 to its caller; what
// went wrong is said in the log and, before the log is open, on stderr.
//
// The command is run as the program: the test builds the engine binary once and starts it in a
// throwaway world, and the parent re-executes that same binary as the detached child, so what is
// compared is the whole chain a hook sets going, including the command line the parent hands the
// child. The files were recorded from the program as it was before Phase 9 unit H31
// (docs/architecture/hexagonal-target.md) made that command line the caller's to give, and a
// change to a byte of any of them fails here. Rewrite them deliberately with
//
//	go test ./cmd -run TestSyncTriggerGolden -update-synctrigger-golden
//
// and read the diff before committing it.
//
// The helpers this file shares with the other golden tests are where they are defined:
// engineBinary and goldenEnvironment in review_receipt_golden_test.go (the binary built once for
// the run, and the environment the program and git run in), ensureNewline and goldenDifference in
// shaper_golden_test.go (the end of a stream, and the first difference of two transcripts).

// updateSyncTriggerGolden is the flag that rewrites the golden files.
var updateSyncTriggerGolden = flag.Bool("update-synctrigger-golden", false, "rewrite the golden files of sync-trigger")

// syncTriggerLogWait bounds the wait for the detached child to write its line.
const syncTriggerLogWait = 20 * time.Second

var (
	syncTriggerTimestamp = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z `)
	syncTriggerDuration  = regexp.MustCompile(`duration=\S+`)
)

// syncTriggerWorld is one case: the directories it names, the script that stands for
// longterm-mem, and the transcript.
type syncTriggerWorld struct {
	t     *testing.T
	bin   string
	state string // the overlay state directory
	cwd   string // the project directory the sync runs in
	home  string // the HOME the program runs with
	calls string // where the script records what it was started with
	b     strings.Builder
}

func newSyncTriggerWorld(t *testing.T) *syncTriggerWorld {
	t.Helper()
	dir := func() string {
		d, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	w := &syncTriggerWorld{t: t, bin: engineBinary(t), state: dir(), cwd: dir(), home: dir()}
	for _, pair := range [][2]string{{w.state, w.cwd}, {w.state, w.home}, {w.cwd, w.home}} {
		if pathsNest(pair[0], pair[1]) {
			t.Fatalf("the directories of the world %q and %q nest, so the transcript could not tell them apart", pair[0], pair[1])
		}
	}
	w.calls = filepath.Join(w.state, "calls.txt")
	return w
}

// memory writes the script that stands for longterm-mem into dir/bin: it records where it ran
// and with what, prints stderr text on stderr, and exits with code. mode is the file mode. The
// script is all the child finds to run: nothing here starts the real longterm-mem.
func (w *syncTriggerWorld) memory(dir string, code int, stderr string, mode os.FileMode) {
	w.t.Helper()
	script := "#!/bin/sh\n" +
		"printf 'ran in %s with: %s\\n' \"$(pwd -P)\" \"$*\" >> '" + w.calls + "'\n"
	if stderr != "" {
		script += "printf '%s\\n' '" + stderr + "' >&2\n"
	}
	script += fmt.Sprintf("exit %d\n", code)
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		w.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", "longterm-mem"), []byte(script), mode); err != nil {
		w.t.Fatal(err)
	}
}

// run records one invocation of 'sync-trigger <args>' started in dir. When waitIn is not empty
// the command is a parent, which returns before its child has written the log, so the case waits
// for the child's line in the log under waitIn (the state directory the run was given or defaults to).
func (w *syncTriggerWorld) run(dir, waitIn string, args ...string) {
	w.t.Helper()
	cmd := exec.Command(w.bin, append([]string{"sync-trigger"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(goldenEnvironment(), "HOME="+w.home)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	code := 0
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			w.t.Fatalf("run sync-trigger %v: %v", args, err)
		}
		code = exit.ExitCode()
	}
	fmt.Fprintf(&w.b, "$ sync-trigger %s\nexit: %d\n--- stdout ---\n%s--- stderr ---\n%s\n", strings.Join(args, " "), code, ensureNewline(stdout.String()), ensureNewline(stderr.String()))
	if waitIn != "" {
		w.waitForLog(waitIn)
	}
}

// logHoldsAChildLine says whether the log holds a complete line of the child: one with its
// outcome, ended by a newline. The child writes the line after it has run, and what the script
// recorded is written before it, so a complete line means the case can read both.
func logHoldsAChildLine(log []byte) bool {
	for _, line := range bytes.SplitAfter(log, []byte("\n")) {
		if bytes.HasSuffix(line, []byte("\n")) && bytes.Contains(line, []byte("outcome=")) && bytes.Contains(line, []byte("exit=")) {
			return true
		}
	}
	return false
}

// waitForLog waits until the log under root holds a complete line of the child. It looks in that
// directory only, so a log of another directory cannot satisfy it.
func (w *syncTriggerWorld) waitForLog(root string) {
	w.t.Helper()
	deadline := time.Now().Add(syncTriggerLogWait)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(filepath.Join(root, "logs", "sync-trigger.log")); err == nil && logHoldsAChildLine(data) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	w.t.Fatalf("the child wrote no line to the log under %s within %s", root, syncTriggerLogWait)
}

// report records the log under root (with the time and the duration of each line left out), what
// the script was started with, and every path under root.
func (w *syncTriggerWorld) report(label, root string) {
	w.t.Helper()
	fmt.Fprintf(&w.b, "=== %s ===\n", label)
	log, err := os.ReadFile(filepath.Join(root, "logs", "sync-trigger.log"))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		fmt.Fprintln(&w.b, "log: (none)")
	case err != nil:
		w.t.Fatal(err)
	default:
		for _, line := range strings.Split(strings.TrimRight(string(log), "\n"), "\n") {
			line = syncTriggerTimestamp.ReplaceAllString(line, "<TIME> ")
			line = syncTriggerDuration.ReplaceAllString(line, "duration=<DURATION>")
			fmt.Fprintf(&w.b, "log: %s\n", line)
		}
	}
	if data, err := os.ReadFile(w.calls); err == nil {
		fmt.Fprintf(&w.b, "longterm-mem: %s", ensureNewline(string(data)))
	} else {
		fmt.Fprintln(&w.b, "longterm-mem: (never started)")
	}
	var paths []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		if rel == "." || rel == "calls.txt" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			paths = append(paths, rel+"/")
		} else {
			paths = append(paths, fmt.Sprintf("%s %04o", rel, info.Mode().Perm()))
		}
		return nil
	})
	sort.Strings(paths)
	if len(paths) == 0 {
		paths = []string{"(empty)"}
	}
	fmt.Fprintf(&w.b, "files: %s\n\n", strings.Join(paths, ", "))
}

// pathName is a path of the world and the placeholder a transcript writes in its place.
type pathName struct{ path, name string }

// maskPaths writes each path of names as its placeholder, the longest first, so that a path inside
// another is named as itself.
func maskPaths(text string, names []pathName) string {
	ordered := append([]pathName(nil), names...)
	sort.SliceStable(ordered, func(i, j int) bool { return len(ordered[i].path) > len(ordered[j].path) })
	for _, n := range ordered {
		text = strings.ReplaceAll(text, n.path, n.name)
	}
	return text
}

// pathsNest says whether one path is the other or inside it (a shared prefix of the spelling, as in
// /w/a and /w/ab, is not nesting).
func pathsNest(a, b string) bool {
	inside := func(outer, inner string) bool {
		rel, err := filepath.Rel(outer, inner)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	}
	return inside(a, b) || inside(b, a)
}

func (w *syncTriggerWorld) text() string {
	return maskPaths(w.b.String(), []pathName{{w.state, "<STATE>"}, {w.cwd, "<CWD>"}, {w.home, "<HOME>"}})
}

type syncTriggerCase struct {
	name string
	run  func(w *syncTriggerWorld)
}

func syncTriggerCases() []syncTriggerCase {
	return []syncTriggerCase{
		{"session-end-runs-the-sync-in-the-project", func(w *syncTriggerWorld) {
			w.memory(w.state, 0, "", 0o755)
			w.run(w.home, w.state, "--event", "session-end", "--cwd", w.cwd, "--state-dir", w.state)
			w.report("after the parent and its child", w.state)
		}},
		{"archive-runs-the-sync-in-the-project", func(w *syncTriggerWorld) {
			w.memory(w.state, 0, "", 0o755)
			w.run(w.home, w.state, "--event", "archive", "--cwd", w.cwd, "--state-dir", w.state)
			w.report("after the parent and its child", w.state)
		}},
		{"a-relative-cwd-is-made-absolute", func(w *syncTriggerWorld) {
			w.memory(w.state, 0, "", 0o755)
			w.run(w.cwd, w.state, "--event", "session-end", "--cwd", ".", "--state-dir", w.state)
			w.report("after the parent and its child", w.state)
		}},
		{"the-state-directory-defaults-to-the-home", func(w *syncTriggerWorld) {
			home := filepath.Join(w.home, ".labdrian-overlay")
			w.memory(home, 0, "", 0o755)
			w.run(w.cwd, home, "--event", "session-end", "--cwd", w.cwd)
			w.report("after the parent and its child (the home's overlay directory)", home)
		}},
		{"an-unknown-event-is-a-usage-line-on-stderr", func(w *syncTriggerWorld) {
			w.run(w.home, "", "--event", "bogus", "--cwd", w.cwd, "--state-dir", w.state)
			w.report("nothing is opened", w.state)
		}},
		{"no-event-is-a-usage-line-on-stderr", func(w *syncTriggerWorld) {
			w.run(w.home, "", "--cwd", w.cwd, "--state-dir", w.state)
			w.report("nothing is opened", w.state)
		}},
		{"no-cwd-is-a-usage-line-on-stderr", func(w *syncTriggerWorld) {
			w.run(w.home, "", "--event", "session-end", "--state-dir", w.state)
			w.report("nothing is opened", w.state)
		}},
		{"no-longterm-mem-is-skipped", func(w *syncTriggerWorld) {
			w.run(w.home, w.state, "--event", "session-end", "--cwd", w.cwd, "--state-dir", w.state)
			w.report("after the parent and its child", w.state)
		}},
		{"a-longterm-mem-that-cannot-run-is-reported", func(w *syncTriggerWorld) {
			w.memory(w.state, 0, "", 0o644)
			w.run(w.home, w.state, "--event", "session-end", "--cwd", w.cwd, "--state-dir", w.state)
			w.report("after the parent and its child", w.state)
		}},
		{"the-child-reports-no-vault", func(w *syncTriggerWorld) {
			w.memory(w.state, 3, "", 0o755)
			w.run(w.home, "", "--child", "--event", "session-end", "--cwd", w.cwd, "--state-dir", w.state)
			w.report("after the child", w.state)
		}},
		{"the-child-reports-no-project", func(w *syncTriggerWorld) {
			w.memory(w.state, 2, "project could not be resolved from the working directory", 0o755)
			w.run(w.home, "", "--child", "--event", "archive", "--cwd", w.cwd, "--state-dir", w.state)
			w.report("after the child", w.state)
		}},
		{"the-child-reports-a-usage-error", func(w *syncTriggerWorld) {
			w.memory(w.state, 2, "flag provided but not defined", 0o755)
			w.run(w.home, "", "--child", "--event", "session-end", "--cwd", w.cwd, "--state-dir", w.state)
			w.report("after the child", w.state)
		}},
		{"the-child-reports-engram-unavailable", func(w *syncTriggerWorld) {
			w.memory(w.state, 4, "", 0o755)
			w.run(w.home, "", "--child", "--event", "session-end", "--cwd", w.cwd, "--state-dir", w.state)
			w.report("after the child", w.state)
		}},
		{"the-child-reports-a-vault-failure", func(w *syncTriggerWorld) {
			w.memory(w.state, 5, "", 0o755)
			w.run(w.home, "", "--child", "--event", "session-end", "--cwd", w.cwd, "--state-dir", w.state)
			w.report("after the child", w.state)
		}},
		{"the-child-reports-any-other-exit", func(w *syncTriggerWorld) {
			w.memory(w.state, 7, "", 0o755)
			w.run(w.home, "", "--child", "--event", "session-end", "--cwd", w.cwd, "--state-dir", w.state)
			w.report("after the child", w.state)
		}},
		{"the-child-without-an-event-never-runs-the-sync", func(w *syncTriggerWorld) {
			w.memory(w.state, 0, "", 0o755)
			w.run(w.home, "", "--child", "--cwd", w.cwd, "--state-dir", w.state)
			w.report("after the child", w.state)
		}},
	}
}

// TestSyncTriggerGolden runs every case and compares its transcript with its golden file.
func TestSyncTriggerGolden(t *testing.T) {
	for _, tc := range syncTriggerCases() {
		t.Run(tc.name, func(t *testing.T) {
			w := newSyncTriggerWorld(t)
			tc.run(w)
			checkSyncTriggerGolden(t, tc.name, w.text())
		})
	}
}

func checkSyncTriggerGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "synctrigger-golden", name+".golden")
	if *updateSyncTriggerGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden file: %v (record it with -update-synctrigger-golden)", err)
	}
	if diff := goldenDifference(name, got, string(want)); diff != "" {
		t.Fatal(diff)
	}
}

// TestSyncTriggerGoldenCasesAreDistinctFiles guards the case list: two cases of one name would
// share a golden file, and a file that belongs to no case would pin nothing.
func TestSyncTriggerGoldenCasesAreDistinctFiles(t *testing.T) {
	seen := map[string]bool{}
	for _, tc := range syncTriggerCases() {
		if seen[tc.name] {
			t.Errorf("two cases are named %q", tc.name)
		}
		seen[tc.name] = true
	}
	entries, err := os.ReadDir(filepath.Join("testdata", "synctrigger-golden"))
	if err != nil {
		t.Fatalf("the golden files are missing, and with them the check that none belongs to no case: %v (record them with -update-synctrigger-golden)", err)
	}
	for _, e := range entries {
		if name := strings.TrimSuffix(e.Name(), ".golden"); !seen[name] {
			t.Errorf("testdata/synctrigger-golden/%s belongs to no case", e.Name())
		}
	}
}
