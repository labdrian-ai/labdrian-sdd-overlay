package shaper

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeGoalFile(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write goal file %s: %v", path, err)
	}
	return path
}

func goalV2JSON(projectID, goalID string) string {
	return `{"version":2,"project_id":"` + projectID + `","goal_id":"` + goalID + `",` +
		`"objective":"Bind the handoff to real intent.","scope":"One project.",` +
		`"constraints":[],"non_goals":[],"acceptance_criteria":["Binding succeeds."],` +
		`"memory_scope":"Project-scoped.","runtime_scope":"Deferred.","delivery_boundary":"No delivery."}`
}

func TestBindGoalHappyPathReturnsExactBytesAndCleanedPath(t *testing.T) {
	root := t.TempDir()
	data := []byte(goalV2JSON("standalone-shaper-handoff", "goal-alpha"))
	writeGoalFile(t, root, "goal.json", data)
	h := sampleHandoff()

	got, err := BindGoal(h, root, "goal.json")
	if err != nil {
		t.Fatalf("BindGoal: %v", err)
	}
	if got.SourcePath != "goal.json" {
		t.Errorf("SourcePath = %q, want %q", got.SourcePath, "goal.json")
	}
	if string(got.GoalBytes) != string(data) {
		t.Errorf("GoalBytes = %q, want %q", got.GoalBytes, data)
	}
	if got.Goal.ProjectID != h.ProjectID || got.Goal.GoalID != h.GoalID {
		t.Errorf("Goal identity = (%q, %q), want (%q, %q)", got.Goal.ProjectID, got.Goal.GoalID, h.ProjectID, h.GoalID)
	}
}

func TestBindGoalCleansNestedRelativePath(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data := []byte(goalV2JSON("standalone-shaper-handoff", "goal-alpha"))
	writeGoalFile(t, filepath.Join(root, "sub"), "goal.json", data)
	h := sampleHandoff()

	got, err := BindGoal(h, root, "./sub/goal.json")
	if err != nil {
		t.Fatalf("BindGoal: %v", err)
	}
	if got.SourcePath != filepath.Clean("./sub/goal.json") {
		t.Errorf("SourcePath = %q, want %q", got.SourcePath, filepath.Clean("./sub/goal.json"))
	}
}

func TestBindGoalRejectsAbsoluteGoalPath(t *testing.T) {
	root := t.TempDir()
	data := []byte(goalV2JSON("standalone-shaper-handoff", "goal-alpha"))
	absPath := writeGoalFile(t, root, "goal.json", data)
	h := sampleHandoff()

	if _, err := BindGoal(h, root, absPath); err == nil {
		t.Fatal("BindGoal accepted an absolute goalPath")
	}
}

func TestBindGoalRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	data := []byte(goalV2JSON("standalone-shaper-handoff", "goal-alpha"))
	writeGoalFile(t, outside, "goal.json", data)
	h := sampleHandoff()

	traversal := filepath.Join("..", filepath.Base(outside), "goal.json")
	if _, err := BindGoal(h, root, traversal); err == nil {
		t.Fatal("BindGoal accepted a traversing goalPath")
	}
}

func TestBindGoalRejectsCleanedDotPath(t *testing.T) {
	root := t.TempDir()
	h := sampleHandoff()
	if _, err := BindGoal(h, root, "."); err == nil {
		t.Fatal("BindGoal accepted goalPath that cleans to \".\"")
	}
}

func TestBindGoalRejectsMissingFile(t *testing.T) {
	root := t.TempDir()
	h := sampleHandoff()
	if _, err := BindGoal(h, root, "absent.json"); err == nil {
		t.Fatal("BindGoal accepted a missing goal file")
	}
}

func TestBindGoalRejectsSymlinkFileInsideRoot(t *testing.T) {
	root := t.TempDir()
	real := t.TempDir()
	data := []byte(goalV2JSON("standalone-shaper-handoff", "goal-alpha"))
	realPath := writeGoalFile(t, real, "goal.json", data)

	link := filepath.Join(root, "goal.json")
	if err := os.Symlink(realPath, link); err != nil {
		t.Skipf("symlink not supported in this environment: %v", err)
	}
	h := sampleHandoff()

	if _, err := BindGoal(h, root, "goal.json"); err == nil {
		t.Fatal("BindGoal accepted a symlinked goal source pointing inside root")
	}
}

func TestBindGoalRejectsSymlinkedAncestorDirectoryEscapingRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	data := []byte(goalV2JSON("standalone-shaper-handoff", "goal-alpha"))
	writeGoalFile(t, outside, "goal.json", data)

	linkedDir := filepath.Join(root, "escape")
	if err := os.Symlink(outside, linkedDir); err != nil {
		t.Skipf("symlink not supported in this environment: %v", err)
	}
	h := sampleHandoff()

	if _, err := BindGoal(h, root, "escape/goal.json"); err == nil {
		t.Fatal("BindGoal accepted a source reached through a symlinked ancestor directory escaping root")
	}
}

func TestBindGoalRejectsDirectoryInsteadOfFile(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "goal.json"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	h := sampleHandoff()

	if _, err := BindGoal(h, root, "goal.json"); err == nil {
		t.Fatal("BindGoal accepted a directory in place of a file")
	}
}

func TestBindGoalRejectsInvalidGoalJSON(t *testing.T) {
	root := t.TempDir()
	writeGoalFile(t, root, "goal.json", []byte(`{"version":`))
	h := sampleHandoff()

	if _, err := BindGoal(h, root, "goal.json"); err == nil {
		t.Fatal("BindGoal accepted invalid Goal JSON")
	}
}

func TestBindGoalRejectsGoalVersionOne(t *testing.T) {
	root := t.TempDir()
	v1 := `{"version":1,"project_id":"standalone-shaper-handoff","objective":"o","scope":"s",` +
		`"constraints":[],"non_goals":[],"acceptance_criteria":["a"],` +
		`"memory_scope":"m","runtime_scope":"r","delivery_boundary":"d"}`
	writeGoalFile(t, root, "goal.json", []byte(v1))
	h := sampleHandoff()

	if _, err := BindGoal(h, root, "goal.json"); err == nil {
		t.Fatal("BindGoal accepted a version 1 Goal")
	}
}

func TestBindGoalRejectsProjectIDMismatch(t *testing.T) {
	root := t.TempDir()
	writeGoalFile(t, root, "goal.json", []byte(goalV2JSON("other-project", "goal-alpha")))
	h := sampleHandoff()

	_, err := BindGoal(h, root, "goal.json")
	if err == nil {
		t.Fatal("BindGoal accepted a project_id mismatch")
	}
	if !strings.Contains(err.Error(), "project_id") {
		t.Errorf("BindGoal error = %v, want it to name project_id", err)
	}
}

func TestBindGoalRejectsGoalIDMismatch(t *testing.T) {
	root := t.TempDir()
	writeGoalFile(t, root, "goal.json", []byte(goalV2JSON("standalone-shaper-handoff", "goal-beta")))
	h := sampleHandoff()

	_, err := BindGoal(h, root, "goal.json")
	if err == nil {
		t.Fatal("BindGoal accepted a goal_id mismatch")
	}
	if !strings.Contains(err.Error(), "goal_id") {
		t.Errorf("BindGoal error = %v, want it to name goal_id", err)
	}
}

func TestBindGoalRejectsEmptyWorktreeRoot(t *testing.T) {
	h := sampleHandoff()
	if _, err := BindGoal(h, "", "goal.json"); err == nil {
		t.Fatal("BindGoal accepted an empty worktreeRoot")
	}
}

func TestBindGoalRejectsRelativeWorktreeRoot(t *testing.T) {
	h := sampleHandoff()
	if _, err := BindGoal(h, "relative/root", "goal.json"); err == nil {
		t.Fatal("BindGoal accepted a relative worktreeRoot")
	}
}

func TestBindGoalRejectsEmptyGoalPath(t *testing.T) {
	root := t.TempDir()
	h := sampleHandoff()
	if _, err := BindGoal(h, root, ""); err == nil {
		t.Fatal("BindGoal accepted an empty goalPath")
	}
}
