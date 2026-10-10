package shaper

import (
	"errors"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/shaper/shapertest"
)

// LoadReadinessInput assembles what Evaluate is given. The cases here are the ones the command
// 'shaper assess' used to decide for itself: a source that exists but cannot be parsed or bound
// is handed on as the bytes it holds, so that Evaluate reports it as a blocker, and only a source
// that cannot be read at all is an error. The sources come from a fake ContainedSource and the
// worktree from a fake observer, so none of it touches a file or runs git.

// fakeObserver answers WorktreeObserver with a fixed provenance or error and records the roots
// it was asked about.
type fakeObserver struct {
	provenance WorktreeProvenance
	err        error
	asked      []string
}

func (f *fakeObserver) Observe(root string) (WorktreeProvenance, error) {
	f.asked = append(f.asked, root)
	return f.provenance, f.err
}

var observedWorktree = WorktreeProvenance{Toplevel: sourceRoot, GitDir: sourceRoot + "/.git", CommonDir: sourceRoot + "/.git"}

func inputSources(t *testing.T, handoff, goal []byte) *fakeSource {
	t.Helper()
	files := map[string][]byte{}
	if handoff != nil {
		files["handoff.json"] = handoff
	}
	if goal != nil {
		files["goal.json"] = goal
	}
	return &fakeSource{files: files}
}

func TestLoadReadinessInputGivesTheParsedSourcesAndTheObservedWorktree(t *testing.T) {
	handoff := documentWith(t, nil)
	goal := []byte(shapertest.GoalV2JSON("standalone-shaper-handoff", "goal-alpha"))
	observer := &fakeObserver{provenance: observedWorktree}

	in, notes, err := LoadReadinessInput(inputSources(t, handoff, goal), observer, sourceRoot, "handoff.json", "goal.json")

	if err != nil || len(notes) != 0 {
		t.Fatalf("LoadReadinessInput = notes %q, err %v; want neither", notes, err)
	}
	if in.WorktreeRoot != sourceRoot {
		t.Errorf("WorktreeRoot = %q, want %q", in.WorktreeRoot, sourceRoot)
	}
	if in.Handoff.Handoff.ProjectID != "standalone-shaper-handoff" || string(in.Handoff.Bytes) != string(handoff) {
		t.Errorf("Handoff = %+v, want the one parsed from the file", in.Handoff)
	}
	if in.Goal == nil || in.Goal.Goal.GoalID != "goal-alpha" || string(in.Goal.GoalBytes) != string(goal) {
		t.Errorf("Goal = %+v, want the one bound from the file", in.Goal)
	}
	if in.Provenance == nil || *in.Provenance != observedWorktree {
		t.Errorf("Provenance = %+v, want %+v", in.Provenance, observedWorktree)
	}
	if len(observer.asked) != 1 || observer.asked[0] != sourceRoot {
		t.Errorf("the worktree was observed as %q, want once, for %q", observer.asked, sourceRoot)
	}
}

func TestLoadReadinessInputHandsOnAHandoffItCannotParseAsItsBytesAndGoesNoFurther(t *testing.T) {
	raw := []byte("not a handoff")
	src := inputSources(t, raw, []byte(shapertest.GoalV2JSON("standalone-shaper-handoff", "goal-alpha")))
	observer := &fakeObserver{provenance: observedWorktree}

	in, notes, err := LoadReadinessInput(src, observer, sourceRoot, "./handoff.json", "goal.json")

	if err != nil || len(notes) != 0 {
		t.Fatalf("LoadReadinessInput = notes %q, err %v; want neither: Evaluate reports the blocker", notes, err)
	}
	if in.WorktreeRoot != sourceRoot || in.Handoff.SourcePath != "handoff.json" || string(in.Handoff.Bytes) != string(raw) || in.Handoff.SHA256 != SourceSHA256(raw) {
		t.Errorf("Handoff = %+v, want the cleaned path, the raw bytes and their digest", in.Handoff)
	}
	if in.Goal != nil || in.Provenance != nil {
		t.Errorf("Goal %+v and Provenance %+v, want neither: the Goal binds to a handoff that was not parsed", in.Goal, in.Provenance)
	}
	if len(observer.asked) != 0 {
		t.Errorf("the worktree was observed (%q) after a handoff that could not be parsed", observer.asked)
	}
}

func TestLoadReadinessInputRefusesAHandoffItCannotRead(t *testing.T) {
	in, _, err := LoadReadinessInput(inputSources(t, nil, nil), &fakeObserver{}, sourceRoot, "handoff.json", "goal.json")

	if err == nil || !strings.HasPrefix(err.Error(), "load handoff: ") {
		t.Fatalf("err = %v, want the failure of LoadHandoff, the load handoff error of the read", err)
	}
	if in.WorktreeRoot != "" || in.Handoff.Bytes != nil || in.Goal != nil || in.Provenance != nil {
		t.Errorf("input = %+v, want none with an error", in)
	}
}

func TestLoadReadinessInputHandsOnAGoalItCannotBindAsItsBytes(t *testing.T) {
	raw := []byte("not a goal")
	observer := &fakeObserver{provenance: observedWorktree}

	in, notes, err := LoadReadinessInput(inputSources(t, documentWith(t, nil), raw), observer, sourceRoot, "handoff.json", "goal.json")

	if err != nil || len(notes) != 0 {
		t.Fatalf("LoadReadinessInput = notes %q, err %v; want neither", notes, err)
	}
	if in.Goal == nil || in.Goal.SourcePath != "goal.json" || string(in.Goal.GoalBytes) != string(raw) || in.Goal.GoalSHA256 != SourceSHA256(raw) {
		t.Errorf("Goal = %+v, want the cleaned path, the raw bytes and their digest", in.Goal)
	}
	if in.Provenance == nil {
		t.Error("the worktree was not observed after a goal that could not be bound; the handoff did parse")
	}
}

func TestLoadReadinessInputRefusesAGoalItCannotRead(t *testing.T) {
	in, _, err := LoadReadinessInput(inputSources(t, documentWith(t, nil), nil), &fakeObserver{provenance: observedWorktree}, sourceRoot, "handoff.json", "goal.json")

	if err == nil || !strings.HasPrefix(err.Error(), "bind goal: ") {
		t.Fatalf("err = %v, want the failure of BindGoal, the bind goal error of the read", err)
	}
	if in.WorktreeRoot != "" || in.Goal != nil || in.Provenance != nil {
		t.Errorf("input = %+v, want none with an error", in)
	}
}

func TestLoadReadinessInputSaysWhenTheWorktreeCannotBeObserved(t *testing.T) {
	observer := &fakeObserver{err: errors.New("not a repository")}

	in, notes, err := LoadReadinessInput(inputSources(t, documentWith(t, nil), []byte(shapertest.GoalV2JSON("standalone-shaper-handoff", "goal-alpha"))), observer, sourceRoot, "handoff.json", "goal.json")

	if err != nil {
		t.Fatalf("LoadReadinessInput: %v, want the sources used without a provenance", err)
	}
	if in.Provenance != nil {
		t.Errorf("Provenance = %+v, want none", in.Provenance)
	}
	if len(notes) != 1 || notes[0] != "worktree observation failed: not a repository" {
		t.Errorf("notes = %q, want the one that says why", notes)
	}
}

func TestLoadReadinessInputWithNoObserverLeavesTheWorktreeUnobserved(t *testing.T) {
	in, notes, err := LoadReadinessInput(inputSources(t, documentWith(t, nil), []byte(shapertest.GoalV2JSON("standalone-shaper-handoff", "goal-alpha"))), nil, sourceRoot, "handoff.json", "goal.json")

	if err != nil || len(notes) != 0 || in.Provenance != nil {
		t.Errorf("LoadReadinessInput = provenance %+v, notes %q, err %v; want an input with no provenance and nothing said", in.Provenance, notes, err)
	}
}
