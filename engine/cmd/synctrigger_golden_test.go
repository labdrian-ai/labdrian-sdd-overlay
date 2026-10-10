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
// The longterm-mem the child finds is a script the case writes: it records its working directory
// and arguments and exits with the code the case chose. Nothing here starts the real one.
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
	w := &syncTriggerWorld{t: t, bin: reviewReceiptBinary(t), state: dir(), cwd: dir(), home: dir()}
	w.calls = filepath.Join(w.state, "calls.txt")
	return w
}

// memory writes the script that stands for longterm-mem into dir/bin: it records where it ran
// and with what, prints stderr text on stderr, and exits with code. mode is the file mode.
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

// run records one invocation of 'sync-trigger <args>' started in dir. When wait is set the
// command is a parent, which returns before its child has written the log, so the case waits for
// the line.
func (w *syncTriggerWorld) run(dir string, wait bool, args ...string) {
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
	if wait {
		w.waitForLog()
	}
}

// waitForLog waits until the log holds a line of the child.
func (w *syncTriggerWorld) waitForLog() {
	w.t.Helper()
	deadline := time.Now().Add(syncTriggerLogWait)
	for time.Now().Before(deadline) {
		for _, root := range []string{w.state, filepath.Join(w.home, ".labdrian-overlay")} {
			if data, err := os.ReadFile(filepath.Join(root, "logs", "sync-trigger.log")); err == nil && bytes.Contains(data, []byte("outcome=")) {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	w.t.Fatalf("the child wrote no line to the log within %s", syncTriggerLogWait)
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

func (w *syncTriggerWorld) text() string {
	text := w.b.String()
	for _, n := range []struct{ path, name string }{{w.state, "<STATE>"}, {w.cwd, "<CWD>"}, {w.home, "<HOME>"}} {
		text = strings.ReplaceAll(text, n.path, n.name)
	}
	return text
}

type syncTriggerCase struct {
	name string
	run  func(w *syncTriggerWorld)
}

func syncTriggerCases() []syncTriggerCase {
	return []syncTriggerCase{
		{"session-end-runs-the-sync-in-the-project", func(w *syncTriggerWorld) {
			w.memory(w.state, 0, "", 0o755)
			w.run(w.home, true, "--event", "session-end", "--cwd", w.cwd, "--state-dir", w.state)
			w.report("after the parent and its child", w.state)
		}},
		{"archive-runs-the-sync-in-the-project", func(w *syncTriggerWorld) {
			w.memory(w.state, 0, "", 0o755)
			w.run(w.home, true, "--event", "archive", "--cwd", w.cwd, "--state-dir", w.state)
			w.report("after the parent and its child", w.state)
		}},
		{"a-relative-cwd-is-made-absolute", func(w *syncTriggerWorld) {
			w.memory(w.state, 0, "", 0o755)
			w.run(w.cwd, true, "--event", "session-end", "--cwd", ".", "--state-dir", w.state)
			w.report("after the parent and its child", w.state)
		}},
		{"the-state-directory-defaults-to-the-home", func(w *syncTriggerWorld) {
			home := filepath.Join(w.home, ".labdrian-overlay")
			w.memory(home, 0, "", 0o755)
			w.run(w.cwd, true, "--event", "session-end", "--cwd", w.cwd)
			w.report("after the parent and its child (the home's overlay directory)", home)
		}},
		{"an-unknown-event-is-a-usage-line-on-stderr", func(w *syncTriggerWorld) {
			w.run(w.home, false, "--event", "bogus", "--cwd", w.cwd, "--state-dir", w.state)
			w.report("nothing is opened", w.state)
		}},
		{"no-event-is-a-usage-line-on-stderr", func(w *syncTriggerWorld) {
			w.run(w.home, false, "--cwd", w.cwd, "--state-dir", w.state)
			w.report("nothing is opened", w.state)
		}},
		{"no-cwd-is-a-usage-line-on-stderr", func(w *syncTriggerWorld) {
			w.run(w.home, false, "--event", "session-end", "--state-dir", w.state)
			w.report("nothing is opened", w.state)
		}},
		{"no-longterm-mem-is-skipped", func(w *syncTriggerWorld) {
			w.run(w.home, true, "--event", "session-end", "--cwd", w.cwd, "--state-dir", w.state)
			w.report("after the parent and its child", w.state)
		}},
		{"a-longterm-mem-that-cannot-run-is-reported", func(w *syncTriggerWorld) {
			w.memory(w.state, 0, "", 0o644)
			w.run(w.home, true, "--event", "session-end", "--cwd", w.cwd, "--state-dir", w.state)
			w.report("after the parent and its child", w.state)
		}},
		{"the-child-reports-no-vault", func(w *syncTriggerWorld) {
			w.memory(w.state, 3, "", 0o755)
			w.run(w.home, false, "--child", "--event", "session-end", "--cwd", w.cwd, "--state-dir", w.state)
			w.report("after the child", w.state)
		}},
		{"the-child-reports-no-project", func(w *syncTriggerWorld) {
			w.memory(w.state, 2, "project could not be resolved from the working directory", 0o755)
			w.run(w.home, false, "--child", "--event", "archive", "--cwd", w.cwd, "--state-dir", w.state)
			w.report("after the child", w.state)
		}},
		{"the-child-reports-a-usage-error", func(w *syncTriggerWorld) {
			w.memory(w.state, 2, "flag provided but not defined", 0o755)
			w.run(w.home, false, "--child", "--event", "session-end", "--cwd", w.cwd, "--state-dir", w.state)
			w.report("after the child", w.state)
		}},
		{"the-child-reports-engram-unavailable", func(w *syncTriggerWorld) {
			w.memory(w.state, 4, "", 0o755)
			w.run(w.home, false, "--child", "--event", "session-end", "--cwd", w.cwd, "--state-dir", w.state)
			w.report("after the child", w.state)
		}},
		{"the-child-reports-a-vault-failure", func(w *syncTriggerWorld) {
			w.memory(w.state, 5, "", 0o755)
			w.run(w.home, false, "--child", "--event", "session-end", "--cwd", w.cwd, "--state-dir", w.state)
			w.report("after the child", w.state)
		}},
		{"the-child-reports-any-other-exit", func(w *syncTriggerWorld) {
			w.memory(w.state, 7, "", 0o755)
			w.run(w.home, false, "--child", "--event", "session-end", "--cwd", w.cwd, "--state-dir", w.state)
			w.report("after the child", w.state)
		}},
		{"the-child-without-an-event-never-runs-the-sync", func(w *syncTriggerWorld) {
			w.memory(w.state, 0, "", 0o755)
			w.run(w.home, false, "--child", "--cwd", w.cwd, "--state-dir", w.state)
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
		t.Skipf("no golden files yet: %v", err)
	}
	for _, e := range entries {
		if name := strings.TrimSuffix(e.Name(), ".golden"); !seen[name] {
			t.Errorf("testdata/synctrigger-golden/%s belongs to no case", e.Name())
		}
	}
}
