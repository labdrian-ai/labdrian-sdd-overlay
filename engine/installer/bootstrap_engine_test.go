package installer_test

// bootstrap asks for the engine before it does anything else (cmd_bootstrap): it writes a
// repository and commits to it, and it asks the engine where each row goes, so a machine that has
// no engine must find out first. ensure_engine_binary says what it does (that it is building the
// engine, or rebuilding one older than its source) with `info`, which writes to stdout, so the
// script sends its words to stderr: bootstrap's stdout is the story of the repository, and the
// words of the check must not be in it.
//
// These tests hold that, with the engine there and current, older than its source, and not there,
// the last two with a `go` first on the PATH that only copies the engine of this checkout where it
// is told to build one. They also hold that a second bootstrap of a repository that is already
// bootstrapped does what the script did before it asked the engine anything (the record was made
// from the script of 57560e3).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/enginetest"
)

// installFakeGo puts a `go` first on the PATH of the world that records its arguments, one call a
// line, and builds by copying the engine of this checkout to the path after -o. It returns the log.
func (w *pinWorld) installFakeGo() string {
	w.t.Helper()
	dir := w.t.TempDir()
	log := filepath.Join(dir, "go.log")
	script := "#!/bin/sh\n" +
		"echo \"$*\" >> \"$FAKE_GO_LOG\"\n" +
		"out=\n" +
		"while [ $# -gt 0 ]; do\n" +
		"  if [ \"$1\" = \"-o\" ]; then out=\"$2\"; fi\n" +
		"  shift\n" +
		"done\n" +
		"cp \"$FAKE_GO_ENGINE\" \"$out\"\n"
	if err := os.WriteFile(filepath.Join(dir, "go"), []byte(script), 0o755); err != nil {
		w.t.Fatalf("write the fake go: %v", err)
	}
	w.env = append(w.env, "FAKE_GO_LOG="+log, "FAKE_GO_ENGINE="+enginetest.Binary(w.t))
	w.changePath(func(old string) string { return dir + string(os.PathListSeparator) + old })
	return log
}

// goCalls are the calls a fake go recorded, normalized, in order.
func (w *pinWorld) goCalls(log string) []string {
	w.t.Helper()
	body, err := os.ReadFile(log)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		w.t.Fatal(err)
	}
	var calls []string
	for _, line := range strings.Split(strings.TrimSuffix(string(body), "\n"), "\n") {
		calls = append(calls, w.normalize(line))
	}
	return calls
}

func TestBootstrapSaysWhatItDoesToTheEngineOnStderrAndGoesOn(t *testing.T) {
	longAgo := time.Now().Add(-2 * time.Hour)
	for _, tc := range []struct {
		name       string
		prepare    func(t *testing.T, w *pinWorld, engine string)
		wantStderr string
		wantBuilds int
	}{
		{"the engine is there and current", func(t *testing.T, w *pinWorld, engine string) {}, "", 0},
		{"the engine is older than its source", func(t *testing.T, w *pinWorld, engine string) {
			if err := os.Chtimes(engine, longAgo, longAgo); err != nil {
				t.Fatal(err)
			}
			w.write(w.overlay, "engine/main.go", "package main\n") // newer than the engine
		}, "==> Engine binary is out of date; rebuilding...\n", 1},
		{"the engine is not there", func(t *testing.T, w *pinWorld, engine string) {
			if err := os.Remove(engine); err != nil {
				t.Fatal(err)
			}
			w.write(w.overlay, "engine/main.go", "package main\n") // the source it is built from
		}, "==> Engine binary not found; building...\n", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newBootstrapWorld(t)
			log := w.installFakeGo()
			engine := filepath.Join(w.home, ".claude", "bin", "gentle-ai-overlay")
			tc.prepare(t, w, engine)

			run := w.run("bootstrap")

			if run.Exit != 0 {
				t.Fatalf("bootstrap = %s", pinRunText(run))
			}
			if strings.Contains(run.Stdout, "Engine binary") {
				t.Errorf("the words of the engine check are on stdout:\n%s", run.Stdout)
			}
			if run.Stderr != tc.wantStderr {
				t.Errorf("stderr = %q, want %q: the words of the engine check go to stderr, and a current engine has none", run.Stderr, tc.wantStderr)
			}
			// It went on as it does with the engine there: the story of the repository is the pinned one.
			if !strings.HasPrefix(recordedPins["bootstrap/run"], pinStdoutText(run)) {
				t.Errorf("stdout differs from the stdout of bootstrap/run\n--- got\n%s", run.Stdout)
			}
			calls := w.goCalls(log)
			if len(calls) != tc.wantBuilds {
				t.Fatalf("go was called %d times, want %d: %q", len(calls), tc.wantBuilds, calls)
			}
			if tc.wantBuilds == 1 && calls[0] != "build -o $HOME/.claude/bin/gentle-ai-overlay ./cmd/" {
				t.Errorf("go was called as %q, want a build of the engine where the script looks for it", calls[0])
			}
			if info, err := os.Stat(engine); err != nil || info.Mode()&0o111 == 0 {
				t.Errorf("the engine is not an executable at %s afterwards: %v", engine, err)
			}
		})
	}
}

// bootstrapStoppedAtItsFirstCommit is what a second bootstrap of a repository says when it stops at
// the guard of its first commit. The guard is `git log --oneline | head -1 | grep -q .` under
// pipefail: on a repository that has commits, head and grep leave as soon as they have a line, and
// when git log is still writing it is killed by SIGPIPE, the pipeline fails, the guard reads "no
// commit yet" and bootstrap tries to commit what is already committed. That is the script's own
// race, older than the engine: the script of 57560e3 stops there 9 times in 30. It does not
// depend on what the engine says, so the test lets it happen and tries again.
const bootstrapStoppedAtItsFirstCommit = "exit 1\n--- stdout\n==> Bootstrapping overlay repo at $OVERLAY\nOn branch main\nnothing to commit, working tree clean\n--- stderr\n(nothing)\n"

func TestBootstrapAgainOnABootstrappedRepositoryDoesWhatItDidBefore(t *testing.T) {
	w := newBootstrapWorld(t)
	if first := w.run("bootstrap"); first.Exit != 0 {
		t.Fatalf("the first bootstrap = %s", pinRunText(first))
	}

	var second pinRun
	for attempt := 1; ; attempt++ {
		second = w.run("bootstrap")
		if pinRunText(second) != bootstrapStoppedAtItsFirstCommit {
			break // it went on, or it stopped in some other way, which the pin below says
		}
		if attempt == 25 {
			t.Fatalf("the second bootstrap stopped at its first commit %d times in a row", attempt)
		}
	}

	pin(t, "bootstrap-again/run", pinRunText(second))
	pin(t, "bootstrap-again/upstream", w.branchFiles("upstream"))
	pin(t, "bootstrap-again/main", w.branchFiles("main"))
	if strings.Contains(second.Stdout+second.Stderr, "Engine binary") {
		t.Errorf("the second bootstrap said something of the engine it already has:\n%s", pinRunText(second))
	}
}
