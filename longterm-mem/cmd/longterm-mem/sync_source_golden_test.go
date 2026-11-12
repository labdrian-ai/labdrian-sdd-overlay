package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// TestSyncSourceEngramDefaultGolden pins what `sync` and `sync --dry-run` answer with no --source, and with
// --source engram, against fixed expectations: the stdout, stderr, exit code and vault of each step, in the world of
// the promote goldens. The default is what engine/synctrigger runs, so it is held to files recorded from the program
// that had no --source, never compared with itself. Record with:
//
//	go test ./cmd/longterm-mem -run TestSyncSourceEngramDefaultGolden -update-promote-golden
//
// and read the diff before committing it.
func TestSyncSourceEngramDefaultGolden(t *testing.T) {
	// Resolved before the world changes the working directory.
	goldenDir, err := filepath.Abs(filepath.Join("testdata", "sync-source-golden"))
	if err != nil {
		t.Fatalf("resolve the golden directory: %v", err)
	}
	w := enterClockWorld(t)
	sync := func(extra ...string) []string {
		return append([]string{"sync", "--project", promoteGoldenProject}, extra...)
	}
	for _, step := range []struct {
		name string
		args []string
	}{
		{"01-plain-sync-dry-run", sync("--dry-run")},
		{"02-source-engram-dry-run", sync("--source", "engram", "--dry-run")},
		{"03-plain-sync", sync()},
		{"04-source-engram-with-nothing-new", sync("--source", "engram")},
	} {
		code, stdout, stderr := runPromoteGolden(t, step.args)
		got := w.normalise(fmt.Sprintf("args: %s\nexit: %d\n--- stdout ---\n%s--- stderr ---\n%s--- vault ---\n%s",
			strings.Join(step.args, " "), code, stdout, stderr, renderPromoteGoldenTree(t, w.vaultRoot)))
		checkPromoteGolden(t, goldenDir, step.name, got)
	}
}
