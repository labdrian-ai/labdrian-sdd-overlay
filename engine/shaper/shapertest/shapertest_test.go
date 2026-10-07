package shapertest_test

import (
	"errors"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/goal"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/shaper"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/shaper/shapertest"
)

// The fixtures are what the domain accepts. This is the one place that says so: the shaper's
// tests and the file-backed adapter's tests both read their documents from here, so when the
// shape of a handoff or a Goal changes this fails first, and names which fixture went stale.
func TestTheFixturesAreWhatTheDomainAccepts(t *testing.T) {
	h, err := shaper.Parse([]byte(shapertest.ValidHandoffJSON))
	if err != nil {
		t.Fatalf("shaper.Parse(ValidHandoffJSON) = %v, want a valid handoff", err)
	}
	g, err := goal.Parse([]byte(shapertest.GoalV2JSON(h.ProjectID, h.GoalID)))
	if err != nil {
		t.Fatalf("goal.Parse(GoalV2JSON(%q, %q)) = %v, want a valid Goal", h.ProjectID, h.GoalID, err)
	}
	if g.ProjectID != h.ProjectID || g.GoalID != h.GoalID {
		t.Errorf("Goal names (%q, %q), the handoff (%q, %q): the fixtures do not describe one goal", g.ProjectID, g.GoalID, h.ProjectID, h.GoalID)
	}
}

// DocumentWith(nil) is the valid document, byte for byte after the same encoding the
// variations go through, and a change replaces only the field it names.
func TestDocumentWithVariesOnlyWhatItIsAskedTo(t *testing.T) {
	base, err := shaper.Parse(shapertest.DocumentWith(t, nil))
	if err != nil {
		t.Fatalf("shaper.Parse(DocumentWith(nil)) = %v", err)
	}
	want, err := shaper.Parse([]byte(shapertest.ValidHandoffJSON))
	if err != nil {
		t.Fatal(err)
	}
	if base.ProjectID != want.ProjectID || base.Architecture != want.Architecture || len(base.Stages) != len(want.Stages) {
		t.Errorf("DocumentWith(nil) parses to %#v, want %#v", base, want)
	}

	changed, err := shaper.Parse(shapertest.DocumentWith(t, map[string]any{"architecture": "Other."}))
	if err != nil {
		t.Fatalf("shaper.Parse(DocumentWith(architecture)) = %v", err)
	}
	if changed.Architecture != "Other." || changed.ProjectID != want.ProjectID {
		t.Errorf("DocumentWith(architecture) parses to %#v, want only the architecture changed", changed)
	}
}

// DocumentWithout removes only the field it names, and GoalV2JSONWithObjective changes only
// the objective: with the default objective it is the document GoalV2JSON returns.
func TestDocumentWithoutAndTheObjectiveVariationChangeOnlyWhatTheyName(t *testing.T) {
	if _, err := shaper.Parse(shapertest.DocumentWithout(t, "architecture")); err == nil {
		t.Error("shaper.Parse(DocumentWithout(architecture)) accepted a handoff with no architecture")
	}
	if _, err := shaper.Parse(shapertest.DocumentWithout(t, "no_such_field")); err != nil {
		t.Errorf("shaper.Parse(DocumentWithout(no_such_field)) = %v, want the valid handoff", err)
	}
	if got, want := shapertest.GoalV2JSONWithObjective("p", "g", shapertest.DefaultObjective), shapertest.GoalV2JSON("p", "g"); got != want {
		t.Errorf("GoalV2JSONWithObjective(default) = %s, want GoalV2JSON %s", got, want)
	}
	g, err := goal.Parse([]byte(shapertest.GoalV2JSONWithObjective("p", "g", "Another objective.")))
	if err != nil || g.Objective != "Another objective." {
		t.Errorf("goal.Parse(GoalV2JSONWithObjective) = %q, %v, want the objective it was given", g.Objective, err)
	}
}

// recorder is a testing.TB that records a failure instead of ending the test, so a helper
// that is supposed to fail can be seen failing.
type recorder struct {
	testing.TB
	failed bool
	fatal  bool
}

func (r *recorder) Helper()                           {}
func (r *recorder) Errorf(format string, args ...any) { r.failed = true }
func (r *recorder) Fatalf(format string, args ...any) { r.failed, r.fatal = true, true }

func TestAssertRejectedWithoutPartialStateFailsOnAnAcceptanceAndOnLeakedState(t *testing.T) {
	type result struct{ N int }
	refusal := errors.New("refused")
	for _, tc := range []struct {
		name       string
		got        result
		err        error
		wantFailed bool
		wantFatal  bool
	}{
		{"a refusal with the zero value", result{}, refusal, false, false},
		{"an acceptance", result{N: 1}, nil, true, true},
		{"a refusal that leaks partial state", result{N: 1}, refusal, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &recorder{}
			shapertest.AssertRejectedWithoutPartialState(r, "Op", tc.got, tc.err, "some input")
			if r.failed != tc.wantFailed || r.fatal != tc.wantFatal {
				t.Errorf("failed=%v fatal=%v, want failed=%v fatal=%v", r.failed, r.fatal, tc.wantFailed, tc.wantFatal)
			}
		})
	}
}
