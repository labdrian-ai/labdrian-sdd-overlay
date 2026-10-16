package main

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/synctrigger"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/synctrigger/osproc"
)

// recordingSyncRunner is the Runner of sync-trigger for a test that wants to see what the verb
// decides without a process: it records what it was asked and answers what it was told to.
type recordingSyncRunner struct {
	executable string
	result     synctrigger.Result

	detached []detachedProgram
	ran      []ranProgram
}

type detachedProgram struct {
	program string
	args    []string
	log     string
}

type ranProgram struct {
	program string
	args    []string
	dir     string
}

func (r *recordingSyncRunner) Executable() (string, error) { return r.executable, nil }

func (r *recordingSyncRunner) Detach(program string, args []string, log *os.File) error {
	r.detached = append(r.detached, detachedProgram{program, args, log.Name()})
	return nil
}

func (r *recordingSyncRunner) Run(program string, args []string, dir string, _ time.Duration, _ io.Writer) (synctrigger.Result, error) {
	r.ran = append(r.ran, ranProgram{program, args, dir})
	return r.result, nil
}

// The verb gives the trigger the clock and the Runner of its deps. A parent starts the program it
// finds itself as with the command line this package writes, and a child runs the sync the state
// directory holds: no process is started, and the log is dated by the clock of the deps.
func TestRunSyncTriggerCore_GivesTheTriggerTheClockAndTheRunnerOfItsDeps(t *testing.T) {
	moment := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	reads := 0
	d := testDeps()
	d.now = func() time.Time {
		defer func() { reads++ }()
		return moment.Add(time.Duration(reads) * time.Second)
	}
	project, stateDir := t.TempDir(), t.TempDir()

	t.Run("the parent", func(t *testing.T) {
		runner := &recordingSyncRunner{executable: "/the/engine"}
		d := d
		d.syncRunner = runner
		exitCode := -1

		runSyncTriggerCore(d, []string{"--event", "archive", "--cwd", project, "--state-dir", stateDir}, io.Discard, func(code int) { exitCode = code })

		if exitCode != 0 {
			t.Errorf("exit = %d, want 0", exitCode)
		}
		want := []detachedProgram{{"/the/engine", syncTriggerChildArgv("archive", project, stateDir), filepath.Join(stateDir, "logs", "sync-trigger.log")}}
		if !reflect.DeepEqual(runner.detached, want) {
			t.Errorf("started %+v, want %+v", runner.detached, want)
		}
		if len(runner.ran) != 0 {
			t.Errorf("the parent ran a program: %+v", runner.ran)
		}
	})

	t.Run("the child", func(t *testing.T) {
		runner := &recordingSyncRunner{result: synctrigger.Result{Exit: 4}}
		d := d
		d.syncRunner = runner
		reads = 0
		binary := filepath.Join(stateDir, "bin", "longterm-mem")
		if err := os.MkdirAll(filepath.Dir(binary), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		exitCode := -1

		runSyncTriggerCore(d, []string{"--child", "--event", "archive", "--cwd", project, "--state-dir", stateDir}, io.Discard, func(code int) { exitCode = code })

		if exitCode != 0 {
			t.Errorf("exit = %d, want 0", exitCode)
		}
		want := []ranProgram{{binary, []string{"sync"}, project}}
		if !reflect.DeepEqual(runner.ran, want) {
			t.Errorf("ran %+v, want %+v", runner.ran, want)
		}
		log, err := os.ReadFile(filepath.Join(stateDir, "logs", "sync-trigger.log"))
		if err != nil {
			t.Fatal(err)
		}
		wantLine := "2026-03-04T05:06:08Z event=archive cwd=" + project + " outcome=failure:engram-unavailable exit=4 duration=1s\n"
		if !strings.HasSuffix(string(log), wantLine) {
			t.Errorf("log = %q, want it to end with %q", log, wantLine)
		}
	})
}

// A deps that gives neither is the program's: the machine's clock and the operating system's
// processes, and the verb still never fails its caller.
func TestRunSyncTriggerCore_WithADepsThatGivesNeitherUsesTheMachinesOwn(t *testing.T) {
	d := testDeps()
	if d.now != nil || d.syncRunner != nil {
		t.Fatal("testDeps sets a clock or a runner; this test is about the deps that set none")
	}
	if got := d.clock()(); time.Since(got) > time.Minute || time.Until(got) > time.Minute {
		t.Errorf("the default clock says %v, want the time of the machine", got)
	}
	if _, ok := d.syncTriggerRunner().(osproc.Runner); !ok {
		t.Errorf("the default Runner is %T, want the operating system's", d.syncTriggerRunner())
	}
}
