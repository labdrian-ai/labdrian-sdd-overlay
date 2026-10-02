package shaper

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// BindGoal parses and binds what a ContainedSource returns; reading the file is the
// source's. These tests give it a fake source, so they prove what the domain decides on
// its own: which path it asks for, what it makes of the bytes, and what it refuses. The
// read of real files (symlinks, directories, a FIFO, a swapped ancestor) is proved against
// the file-backed source, in engine/shaper/fsadapter, through these same functions.

// assertRejectedWithoutPartialBinding fails unless BindGoal returned an error
// together with the zero GoalBinding, so no rejection leaks partial state.
func assertRejectedWithoutPartialBinding(t *testing.T, got GoalBinding, err error, what string) {
	t.Helper()
	if err == nil {
		t.Fatalf("BindGoal accepted %s", what)
	}
	if !reflect.DeepEqual(got, GoalBinding{}) {
		t.Errorf("BindGoal returned a partial GoalBinding alongside error %v: %#v", err, got)
	}
}

func goalV2JSON(projectID, goalID string) string {
	return `{"version":2,"project_id":"` + projectID + `","goal_id":"` + goalID + `",` +
		`"objective":"Bind the handoff to real intent.","scope":"One project.",` +
		`"constraints":[],"non_goals":[],"acceptance_criteria":["Binding succeeds."],` +
		`"memory_scope":"Project-scoped.","runtime_scope":"Deferred.","delivery_boundary":"No delivery."}`
}

func TestBindGoalHappyPathReturnsExactBytesAndCleanedPath(t *testing.T) {
	data := []byte(goalV2JSON("standalone-shaper-handoff", "goal-alpha"))
	src := &fakeSource{files: map[string][]byte{"goal.json": data}}
	h := sampleHandoff()

	got, err := BindGoal(src, h, sourceRoot, "goal.json")
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

// The source is asked for the cleaned path, under the root it was given, and told what the
// bytes are for: it is the domain that decides what to read, not the caller's spelling.
func TestBindGoalAsksTheSourceForTheCleanedPathUnderItsLabel(t *testing.T) {
	data := []byte(goalV2JSON("standalone-shaper-handoff", "goal-alpha"))
	src := &fakeSource{files: map[string][]byte{"sub/goal.json": data}}

	got, err := BindGoal(src, sampleHandoff(), sourceRoot, "./sub/../sub/goal.json")
	if err != nil {
		t.Fatalf("BindGoal: %v", err)
	}
	if got.SourcePath != "sub/goal.json" {
		t.Errorf("SourcePath = %q, want the cleaned %q", got.SourcePath, "sub/goal.json")
	}
	want := []readCall{{root: sourceRoot, rel: "sub/goal.json", label: "goal source"}}
	if !reflect.DeepEqual(src.calls, want) {
		t.Errorf("source was asked %+v, want exactly %+v", src.calls, want)
	}
}

// What the source refuses is reported, with the operation named, and nothing is bound.
func TestBindGoalReportsWhatTheSourceRefusesWithoutAPartialBinding(t *testing.T) {
	refusal := errors.New(`goal source "goal.json" must not be a symlink`)
	got, err := BindGoal(&fakeSource{err: refusal}, sampleHandoff(), sourceRoot, "goal.json")
	assertRejectedWithoutPartialBinding(t, got, err, "a goal the source refused")
	if want := "bind goal: " + refusal.Error(); err.Error() != want {
		t.Errorf("BindGoal error = %q, want %q", err, want)
	}
	if !errors.Is(err, refusal) {
		t.Errorf("BindGoal error %v does not wrap the source's", err)
	}
}

func TestBindGoalRejectsInvalidGoalJSON(t *testing.T) {
	src := &fakeSource{files: map[string][]byte{"goal.json": []byte(`{"version":`)}}
	got, err := BindGoal(src, sampleHandoff(), sourceRoot, "goal.json")
	assertRejectedWithoutPartialBinding(t, got, err, "invalid Goal JSON")
}

func TestBindGoalRejectsGoalVersionOne(t *testing.T) {
	v1 := `{"version":1,"project_id":"standalone-shaper-handoff","objective":"o","scope":"s",` +
		`"constraints":[],"non_goals":[],"acceptance_criteria":["a"],` +
		`"memory_scope":"m","runtime_scope":"r","delivery_boundary":"d"}`
	src := &fakeSource{files: map[string][]byte{"goal.json": []byte(v1)}}

	got, err := BindGoal(src, sampleHandoff(), sourceRoot, "goal.json")
	assertRejectedWithoutPartialBinding(t, got, err, "a version 1 Goal")
}

func TestBindGoalRejectsProjectIDMismatch(t *testing.T) {
	src := &fakeSource{files: map[string][]byte{"goal.json": []byte(goalV2JSON("other-project", "goal-alpha"))}}
	got, err := BindGoal(src, sampleHandoff(), sourceRoot, "goal.json")
	assertRejectedWithoutPartialBinding(t, got, err, "a project_id mismatch")
	if !strings.Contains(err.Error(), "project_id") {
		t.Errorf("BindGoal error = %v, want it to name project_id", err)
	}
}

func TestBindGoalRejectsGoalIDMismatch(t *testing.T) {
	src := &fakeSource{files: map[string][]byte{"goal.json": []byte(goalV2JSON("standalone-shaper-handoff", "goal-beta"))}}
	got, err := BindGoal(src, sampleHandoff(), sourceRoot, "goal.json")
	assertRejectedWithoutPartialBinding(t, got, err, "a goal_id mismatch")
	if !strings.Contains(err.Error(), "goal_id") {
		t.Errorf("BindGoal error = %v, want it to name goal_id", err)
	}
}

func TestBindGoalExposesSHA256OfExactEvaluatedBytes(t *testing.T) {
	data := []byte(goalV2JSON("standalone-shaper-handoff", "goal-alpha"))
	src := &fakeSource{files: map[string][]byte{"goal.json": data}}

	got, err := BindGoal(src, sampleHandoff(), sourceRoot, "goal.json")
	if err != nil {
		t.Fatalf("BindGoal: %v", err)
	}
	sum := sha256.Sum256(data)
	if want := hex.EncodeToString(sum[:]); got.GoalSHA256 != want {
		t.Errorf("GoalSHA256 = %q, want %q", got.GoalSHA256, want)
	}
}

func TestBindGoalSHA256ChangesOnWhitespaceOnlyEdit(t *testing.T) {
	data := []byte(goalV2JSON("standalone-shaper-handoff", "goal-alpha"))
	before, err := BindGoal(&fakeSource{files: map[string][]byte{"goal.json": data}}, sampleHandoff(), sourceRoot, "goal.json")
	if err != nil {
		t.Fatalf("BindGoal before edit: %v", err)
	}

	edited := append(append([]byte{}, data...), '\n')
	after, err := BindGoal(&fakeSource{files: map[string][]byte{"goal.json": edited}}, sampleHandoff(), sourceRoot, "goal.json")
	if err != nil {
		t.Fatalf("BindGoal after edit: %v", err)
	}
	if before.Goal.Objective != after.Goal.Objective {
		t.Fatalf("whitespace-only edit changed the parsed Goal")
	}
	if before.GoalSHA256 == after.GoalSHA256 {
		t.Errorf("GoalSHA256 did not change on a whitespace-only edit: %q", before.GoalSHA256)
	}
}
