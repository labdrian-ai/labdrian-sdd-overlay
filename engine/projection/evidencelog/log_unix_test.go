//go:build linux || darwin

package evidencelog_test

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/statestore"
)

// The failure paths of Append (C23 M7 review B, R3-no-failure-path-test): whatever stands at the
// sidecar's path that is not a regular file is refused, and nothing hangs on it.

func TestAppendRefusesAFIFOAtTheSidecarWithoutBlocking(t *testing.T) {
	log, path := newLog(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Skipf("cannot make a FIFO here: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- log.Append(record("toolu_1")) }()
	select {
	case err := <-done:
		if !errors.Is(err, statestore.ErrNotRegular) {
			t.Errorf("Append over a FIFO = %v, want ErrNotRegular", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Append blocked on a FIFO")
	}
}

func TestAppendRefusesADirectoryAtTheSidecar(t *testing.T) {
	log, path := newLog(t)
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := log.Append(record("toolu_1")); err == nil {
		t.Error("appended to a directory")
	}
}

func TestAppendRefusesASymlinkedWorkflowsDirectory(t *testing.T) {
	log, path := newLog(t)
	projectDir := filepath.Dir(path)
	if err := os.MkdirAll(filepath.Dir(projectDir), 0o700); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	if err := os.Symlink(target, projectDir); err != nil {
		t.Fatal(err)
	}
	if err := log.Append(record("toolu_1")); err == nil {
		t.Error("appended through a symlinked directory")
	}
	if entries, _ := os.ReadDir(target); len(entries) != 0 {
		t.Errorf("the symlink's target was written: %v", entries)
	}
}

// A line torn by an append that was cut short has no newline; the next record must start a line of
// its own, so it still parses (C23 M7 review B, R1-001).
func TestAppendStartsANewLineAfterATornOne(t *testing.T) {
	log, path := newLog(t)
	if err := log.Append(record("toolu_1")); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"version":1,"at":"2026-`); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := log.Append(record("toolu_3")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := splitLines(string(data))
	if len(lines) != 3 || lines[1] != `{"version":1,"at":"2026-` {
		t.Fatalf("lines = %q, want the torn one alone between two records", lines)
	}
	if got := decodeLine(t, lines[2])["call_id"]; got != "toolu_3" {
		t.Errorf("record after the torn line has call_id %v, want toolu_3", got)
	}
}
