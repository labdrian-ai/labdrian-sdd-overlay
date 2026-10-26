package synctrigger

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The log is bounded: when it has reached a mebibyte, the next opening moves it to
// sync-trigger.log.1 (and .1 to .2, .2 to .3, dropping the old .3) and starts a new one, so the
// newest lines are always in sync-trigger.log and at most three older files are kept.
func TestRunChild_RotatesALogThatReachedItsLimitAndKeepsThreeOlderFiles(t *testing.T) {
	o, stateDir := child(t, &fakeRunner{})
	full := bytes.Repeat([]byte("x"), 1<<20)
	for name, content := range map[string][]byte{
		"sync-trigger.log":   full,
		"sync-trigger.log.1": []byte("one\n"),
		"sync-trigger.log.2": []byte("two\n"),
		"sync-trigger.log.3": []byte("three\n"),
	} {
		writeLogFile(t, stateDir, name, content)
	}

	RunChild(o)

	want := stamp(1) + " event=session-end cwd=/project outcome=ok exit=0 duration=1.5s\n"
	if got := logText(t, stateDir); got != want {
		t.Errorf("log = %q, want only the new line %q", got, want)
	}
	for name, want := range map[string]string{
		"sync-trigger.log.1": string(full),
		"sync-trigger.log.2": "one\n",
		"sync-trigger.log.3": "two\n",
	} {
		if got := readLogFile(t, stateDir, name); got != want {
			t.Errorf("%s holds %d bytes, want %d", name, len(got), len(want))
		}
	}
	if _, err := os.Stat(filepath.Join(stateDir, "logs", "sync-trigger.log.4")); !os.IsNotExist(err) {
		t.Errorf("a fourth older file exists (err %v); only three are kept", err)
	}
}

// A log under its limit is appended to, and no older file is made.
func TestRunChild_AppendsToALogUnderItsLimit(t *testing.T) {
	o, stateDir := child(t, &fakeRunner{})
	writeLogFile(t, stateDir, "sync-trigger.log", []byte("before\n"))

	RunChild(o)

	want := "before\n" + stamp(1) + " event=session-end cwd=/project outcome=ok exit=0 duration=1.5s\n"
	if got := logText(t, stateDir); got != want {
		t.Errorf("log = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "logs", "sync-trigger.log.1")); !os.IsNotExist(err) {
		t.Errorf("an older file was made for a log under its limit (err %v)", err)
	}
}

// The parent rotates as the child does: both open the log the same way.
func TestRun_RotatesALogThatReachedItsLimit(t *testing.T) {
	stateDir := t.TempDir()
	full := bytes.Repeat([]byte("x"), 1<<20)
	writeLogFile(t, stateDir, "sync-trigger.log", full)
	runner := &fakeRunner{executable: "/bin/self", childWrites: "child\n"}

	Run(Options{Event: "archive", Cwd: "/project", StateDir: stateDir, Now: stepClock(), Runner: runner, ChildArgv: func(string, string, string) []string { return nil }})

	if got := logText(t, stateDir); got != "child\n" {
		t.Errorf("log = %q, want the child's output in a new log", got)
	}
	if got := readLogFile(t, stateDir, "sync-trigger.log.1"); len(got) != len(full) {
		t.Errorf("sync-trigger.log.1 holds %d bytes, want the full log of %d", len(got), len(full))
	}
}

// The log says which projects were synced, so it is readable by its owner only: a new log is made
// 0600, and one an older version made 0644 is narrowed when it is opened.
func TestRunChild_KeepsTheLogReadableByItsOwnerOnly(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, stateDir string)
	}{
		{"a new log", func(*testing.T, string) {}},
		{"a log an older version made 0644", func(t *testing.T, stateDir string) {
			writeLogFile(t, stateDir, "sync-trigger.log", []byte("before\n"))
			if err := os.Chmod(logPath(stateDir), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o, stateDir := child(t, &fakeRunner{})
			tc.setup(t, stateDir)

			RunChild(o)

			info, err := os.Stat(logPath(stateDir))
			if err != nil {
				t.Fatal(err)
			}
			if mode := info.Mode().Perm(); mode != 0o600 {
				t.Errorf("log mode = %o, want 600", mode)
			}
		})
	}
}

// A cwd or an event that would end the line or read as another field is written quoted, so no
// directory name can forge a line of the log; a plain path is written as it always was.
func TestAppendLog_QuotesAFieldThatCouldForgeALine(t *testing.T) {
	for _, tc := range []struct {
		name, event, cwd, want string
	}{
		{"a plain path", "archive", "/p/q", "event=archive cwd=/p/q outcome"},
		{"a newline that would start a forged line", "archive", "/p\n2026-01-01T00:00:00Z event=archive cwd=/x outcome=ok", `event=archive cwd="/p\n2026-01-01T00:00:00Z event=archive cwd=/x outcome=ok" outcome`},
		{"a space that would read as another field", "archive", "/p outcome=ok", `event=archive cwd="/p outcome=ok" outcome`},
		{"a quote", "archive", `/p"q`, `event=archive cwd="/p\"q" outcome`},
		{"an event the child was started with by hand", "bo\ngus", "/p", `event="bo\ngus" cwd=/p outcome`},
		{"an empty cwd, as before", "archive", "", "event=archive cwd= outcome"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			appendLog(&b, epoch, tc.event, tc.cwd, "ok", 0, 0)
			line := b.String()
			if !strings.Contains(line, tc.want) || strings.Count(line, "\n") != 1 {
				t.Errorf("line = %q, want one line holding %q", line, tc.want)
			}
		})
	}
}

// What the program says on standard error is all in the log, but only its first 64 KiB are kept
// to read the exit code 2 with, so a program that prints without end cannot fill the memory of the
// child.
func TestRunChild_KeepsABoundedCopyOfWhatTheProgramSays(t *testing.T) {
	notProject := "longterm-mem: sync: --project is required: it could not be resolved from the working directory\n"
	for _, tc := range []struct {
		name   string
		prints string
		want   string
	}{
		{"the sentence first, then much more", notProject + strings.Repeat("y", 1<<20), "skip:no-project"},
		{"the sentence only after the first 64 KiB", strings.Repeat("y", 64<<10) + notProject, "error:usage"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o, stateDir := child(t, &fakeRunner{result: Result{Exit: 2}, prints: tc.prints})

			got := RunChild(o)

			if got.Kind != tc.want {
				t.Errorf("RunChild = %+v, want %s", got, tc.want)
			}
			if text := logText(t, stateDir); !strings.HasPrefix(text, tc.prints) {
				t.Errorf("the log does not hold all %d bytes the program said", len(tc.prints))
			}
		})
	}
}

// A state directory that is not absolute (HOME unset, so the caller could not resolve the
// default) is no place to write: nothing is written under the working directory, the parent
// starts no child and says why on stderr, and the child runs no binary.
func TestARelativeStateDirectoryWritesNothingAndRunsNothing(t *testing.T) {
	work := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(oldWd) })

	for _, stateDir := range []string{"", "relative"} {
		var stderr bytes.Buffer
		runner := &fakeRunner{executable: "/bin/self"}
		Run(Options{Event: "archive", Cwd: "/project", StateDir: stateDir, Stderr: &stderr, Now: stepClock(), Runner: runner, ChildArgv: func(string, string, string) []string { return nil }})
		if len(runner.detached) != 0 {
			t.Errorf("state dir %q: the parent started a child: %+v", stateDir, runner.detached)
		}
		if !strings.Contains(stderr.String(), "sync-trigger: error:log: ") || !strings.Contains(stderr.String(), "not absolute") {
			t.Errorf("state dir %q: stderr = %q, want an error:log that names the relative state directory", stateDir, stderr.String())
		}

		runner = &fakeRunner{}
		got := RunChild(Options{Event: "archive", Cwd: "/project", StateDir: stateDir, Now: stepClock(), Runner: runner})
		if got.Kind != "skip:no-binary" || len(runner.ran) != 0 {
			t.Errorf("state dir %q: RunChild = %+v, ran %+v; want skip:no-binary and nothing run", stateDir, got, runner.ran)
		}
	}

	entries, err := os.ReadDir(work)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("the working directory holds %d entries, want none", len(entries))
	}
}

func writeLogFile(t *testing.T, stateDir, name string, content []byte) {
	t.Helper()
	dir := filepath.Join(stateDir, "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readLogFile(t *testing.T, stateDir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(stateDir, "logs", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}
