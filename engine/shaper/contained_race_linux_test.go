//go:build linux

package shaper

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// setContainedOpenHook installs fn as the contained-read test seam and
// restores the no-op seam when the test ends.
func setContainedOpenHook(t *testing.T, fn func(stage, joined string)) {
	t.Helper()
	prev := containedOpenHook
	containedOpenHook = fn
	t.Cleanup(func() { containedOpenHook = prev })
}

// goalV2JSONWithObjective is a matching Goal v2 document whose objective
// distinguishes which file was actually read.
func goalV2JSONWithObjective(objective string) string {
	return strings.Replace(goalV2JSON("standalone-shaper-handoff", "goal-alpha"),
		"Bind the handoff to real intent.", objective, 1)
}

func mustRename(t *testing.T, from, to string) {
	t.Helper()
	if err := os.Rename(from, to); err != nil {
		t.Fatalf("rename %s -> %s: %v", from, to, err)
	}
}

func mustSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink %s -> %s: %v", link, target, err)
	}
}

func TestBindGoalRefusesFinalComponentSwappedForSymlinkBeforeOpen(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeGoalFile(t, root, "goal.json", []byte(goalV2JSONWithObjective("Inside.")))
	outsidePath := writeGoalFile(t, outside, "goal.json", []byte(goalV2JSONWithObjective("Outside.")))

	setContainedOpenHook(t, func(stage, joined string) {
		if stage != "pre-open" {
			return
		}
		if err := os.Remove(joined); err != nil {
			t.Fatalf("remove: %v", err)
		}
		mustSymlink(t, outsidePath, joined)
	})

	got, err := BindGoal(sampleHandoff(), root, "goal.json")
	assertRejectedWithoutPartialBinding(t, got, err, "a goal source swapped for a symlink between check and open")
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("BindGoal error = %v, want it to name the symlink", err)
	}
}

func TestBindGoalRefusesAncestorSwappedToOutsideDirectoryBeforeOpen(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "goals"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeGoalFile(t, filepath.Join(root, "goals"), "goal.json", []byte(goalV2JSONWithObjective("Inside.")))
	writeGoalFile(t, outside, "goal.json", []byte(goalV2JSONWithObjective("Outside.")))

	goalsDir := filepath.Join(root, "goals")
	setContainedOpenHook(t, func(stage, joined string) {
		if stage != "pre-open" {
			return
		}
		mustRename(t, goalsDir, goalsDir+".real")
		mustSymlink(t, outside, goalsDir)
	})

	got, err := BindGoal(sampleHandoff(), root, "goals/goal.json")
	assertRejectedWithoutPartialBinding(t, got, err, "a goal source reached through an ancestor swapped outside the root before open")
}

func TestBindGoalRefusesDoubleToggledAncestorAfterOpen(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	goalsDir := filepath.Join(root, "goals")
	if err := os.MkdirAll(goalsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeGoalFile(t, goalsDir, "goal.json", []byte(goalV2JSONWithObjective("Inside.")))
	writeGoalFile(t, outside, "goal.json", []byte(goalV2JSONWithObjective("Outside.")))

	setContainedOpenHook(t, func(stage, joined string) {
		switch stage {
		case "pre-open":
			mustRename(t, goalsDir, goalsDir+".real")
			mustSymlink(t, outside, goalsDir)
		case "post-open":
			// Swap the real directory back so every by-path check made
			// after the open sees a contained file.
			if err := os.Remove(goalsDir); err != nil {
				t.Fatalf("remove link: %v", err)
			}
			mustRename(t, goalsDir+".real", goalsDir)
		}
	})

	got, err := BindGoal(sampleHandoff(), root, "goals/goal.json")
	assertRejectedWithoutPartialBinding(t, got, err, "a descriptor opened outside the root while the path was toggled back")
	if err != nil && !strings.Contains(err.Error(), "outside the worktree root") {
		t.Errorf("BindGoal error = %v, want the containment refusal", err)
	}
}

func TestBindGoalRefusesFIFOSwappedInBeforeOpenWithoutBlocking(t *testing.T) {
	root := t.TempDir()
	writeGoalFile(t, root, "goal.json", []byte(goalV2JSON("standalone-shaper-handoff", "goal-alpha")))

	var fifo string
	setContainedOpenHook(t, func(stage, joined string) {
		if stage != "pre-open" {
			return
		}
		if err := os.Remove(joined); err != nil {
			t.Fatalf("remove: %v", err)
		}
		if err := syscall.Mkfifo(joined, 0o644); err != nil {
			t.Fatalf("mkfifo: %v", err)
		}
		fifo = joined
	})
	t.Cleanup(func() {
		// Unblock a reader left behind by a failing run.
		if fifo == "" {
			return
		}
		if w, err := os.OpenFile(fifo, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			w.Close()
		}
	})

	type result struct {
		got GoalBinding
		err error
	}
	done := make(chan result, 1)
	go func() {
		got, err := BindGoal(sampleHandoff(), root, "goal.json")
		done <- result{got, err}
	}()

	select {
	case r := <-done:
		assertRejectedWithoutPartialBinding(t, r.got, r.err, "a FIFO swapped in before open")
		if !strings.Contains(r.err.Error(), "regular file") {
			t.Errorf("BindGoal error = %v, want it to require a regular file", r.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("BindGoal blocked reading a FIFO swapped in before open")
	}
}

func TestBindGoalRefusesSourceUnlinkedAfterOpen(t *testing.T) {
	root := t.TempDir()
	writeGoalFile(t, root, "goal.json", []byte(goalV2JSON("standalone-shaper-handoff", "goal-alpha")))

	setContainedOpenHook(t, func(stage, joined string) {
		if stage != "post-open" {
			return
		}
		if err := os.Remove(joined); err != nil {
			t.Fatalf("remove: %v", err)
		}
	})

	got, err := BindGoal(sampleHandoff(), root, "goal.json")
	assertRejectedWithoutPartialBinding(t, got, err, "a goal source unlinked after open")
}

func TestLoadHandoffRefusesFinalComponentSwappedForSymlinkBeforeOpen(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	data := documentWith(t, nil)
	writeGoalFile(t, root, "handoff.json", data)
	outsidePath := writeGoalFile(t, outside, "handoff.json", data)

	setContainedOpenHook(t, func(stage, joined string) {
		if stage != "pre-open" {
			return
		}
		if err := os.Remove(joined); err != nil {
			t.Fatalf("remove: %v", err)
		}
		mustSymlink(t, outsidePath, joined)
	})

	got, err := LoadHandoff(root, "handoff.json")
	assertLoadHandoffRejected(t, got, err, "a handoff swapped for a symlink between check and open")
}
