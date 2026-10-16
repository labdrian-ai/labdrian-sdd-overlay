package osproc_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/synctrigger"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/synctrigger/osproc"
)

// These run the trigger over this adapter and the clock of the machine, the way the program does,
// and read what it leaves in the log. They are the few tests that put the two together.

// lastLogLine is the last line of the log under stateDir, "" if there is none.
func lastLogLine(stateDir string) string {
	data, err := os.ReadFile(filepath.Join(stateDir, "logs", "sync-trigger.log"))
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	return lines[len(lines)-1]
}

func options(stateDir string) synctrigger.Options {
	return synctrigger.Options{Event: "session-end", Cwd: stateDir, StateDir: stateDir, Now: time.Now, Runner: osproc.Runner{}}
}

// The child Run starts is a program of the machine that writes to the very log Run opened.
func TestRunDetachesAChildThatWritesTheLogWithin2Seconds(t *testing.T) {
	stateDir := t.TempDir()
	// Run invokes the child as the caller's command line says; here that is `sync-trigger --event
	// <event> --cwd <cwd> --state-dir <dir> --child`, so $3 is the event and $5 the cwd.
	self := script(t, `echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) event=$3 cwd=$5 outcome=ok exit=0 duration=1ms" >> "`+stateDir+`/logs/sync-trigger.log"`+"\n")
	o := options(stateDir)
	o.Self = self
	o.ChildArgv = func(event, cwd, state string) []string {
		return []string{"sync-trigger", "--event", event, "--cwd", cwd, "--state-dir", state, "--child"}
	}

	if got := synctrigger.Run(o); got != 0 {
		t.Fatalf("Run() = %d, want 0", got)
	}

	deadline := time.Now().Add(2 * time.Second)
	var line string
	for time.Now().Before(deadline) {
		if line = lastLogLine(stateDir); strings.Contains(line, "outcome=ok") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(line, "outcome=ok") || !strings.Contains(line, "event=session-end cwd="+stateDir) {
		t.Fatalf("log line after 2s = %q, want the child's line for session-end in %s", line, stateDir)
	}
}

func TestRunSaysAChildItCannotStartOnStderrAndInTheLog(t *testing.T) {
	stateDir := t.TempDir()
	self := filepath.Join(t.TempDir(), "not-executable")
	if err := os.WriteFile(self, []byte("#!/bin/sh\nexit 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stderr strings.Builder
	o := options(stateDir)
	o.Self, o.Stderr = self, &stderr
	o.ChildArgv = func(string, string, string) []string { return []string{"x"} }

	if got := synctrigger.Run(o); got != 0 {
		t.Fatalf("Run() = %d, want 0", got)
	}

	if line := lastLogLine(stateDir); !strings.Contains(line, "outcome=error:spawn") {
		t.Errorf("log line = %q, want outcome=error:spawn", line)
	}
	if !strings.HasPrefix(stderr.String(), "sync-trigger: error:spawn: ") {
		t.Errorf("stderr = %q, want it to say error:spawn", stderr.String())
	}
}

// What the program prints on standard error reaches the log before the line that sums the run up,
// and the words are the ones the exit code 2 is told apart by.
func TestRunChildPutsTheProgramsStandardErrorInTheLogBeforeTheOutcomeLine(t *testing.T) {
	stateDir := t.TempDir()
	o := options(stateDir)
	o.Binary = script(t, "echo 'sync: could not be resolved from the working directory' >&2\nexit 2\n")

	got := synctrigger.RunChild(o)

	if got.Kind != "skip:no-project" || got.Exit != 2 {
		t.Fatalf("RunChild = %+v, want skip:no-project with exit 2", got)
	}
	data, err := os.ReadFile(filepath.Join(stateDir, "logs", "sync-trigger.log"))
	if err != nil {
		t.Fatal(err)
	}
	said, outcome := strings.Index(string(data), "could not be resolved"), strings.Index(string(data), "outcome=skip:no-project")
	if said < 0 || outcome < 0 || said > outcome {
		t.Errorf("log = %q, want the program's words before the summary line", data)
	}
}

// A sync that outlives its time is a timeout in the log, and it ends the run at the time and not
// when the sync would have.
func TestRunChildReportsASyncThatOutlivesItsTimeAsATimeout(t *testing.T) {
	stateDir := t.TempDir()
	o := options(stateDir)
	o.Binary = script(t, "sleep 30\n")
	o.Timeout = 200 * time.Millisecond

	start := time.Now()
	got := synctrigger.RunChild(o)
	elapsed := time.Since(start)

	if got.Kind != "timeout" {
		t.Fatalf("RunChild = %+v, want timeout", got)
	}
	if elapsed > 10*time.Second {
		t.Errorf("RunChild took %s, want it bound by the timeout and not by the sync", elapsed)
	}
	if line := lastLogLine(stateDir); !strings.Contains(line, "outcome=timeout") {
		t.Errorf("log line = %q, want outcome=timeout", line)
	}
}
