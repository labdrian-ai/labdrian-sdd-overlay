package shaper

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// goalV3JSONWithNonGoals is goalV2JSONWithNonGoals at Goal version 3, with
// memory_scope replaced by a memory directive that reads Engram only.
func goalV3JSONWithNonGoals(nonGoals string) string {
	return `{"version":3,"project_id":"standalone-shaper-handoff","goal_id":"goal-alpha",` +
		`"objective":"Bind the handoff to real intent.","scope":"One project.",` +
		`"constraints":[],"non_goals":` + nonGoals + `,"acceptance_criteria":["Binding succeeds."],` +
		`"memory_directive":{"version":1,"scope":"project","sources":["engram"],"write":"none"},` +
		`"runtime_scope":"Deferred.","delivery_boundary":"No delivery."}`
}

// completeV4Input is completeV3Input for a version 4 handoff and the version 3
// Goal it pairs with.
func completeV4Input(t *testing.T) ReadinessInput {
	t.Helper()
	return inputWith(t, []byte(validHandoffV4JSON), []byte(goalV3JSONWithNonGoals(`[]`)))
}

func viewValue(name, value string) string {
	return fmt.Sprintf("%s %d\n%s\n", name, len(value), value)
}

func TestEvaluateV4ReachesReadyWithVerifiedClearance(t *testing.T) {
	in := completeV4Input(t)
	draft := Evaluate(in, nil)
	if want := []Reason{ReasonClearanceMissing}; draft.State != StateDraft || !reflect.DeepEqual(blockerReasons(draft), want) {
		t.Fatalf("Evaluate(v4, nil) = %q with %v, want draft with %v", draft.State, blockerReasons(draft), want)
	}
	got := Evaluate(in, verifiedFor(t, in))
	if got.State != StateReady || len(got.Blockers) != 0 {
		t.Fatalf("Evaluate(v4, cleared) = %q with %v, want ready with no blocker", got.State, blockerReasons(got))
	}
}

// A version 4 handoff pairs with a version 3 Goal, and versions 1 to 3 with a version 2 Goal:
// each side of the pair carries the same kind of memory field.
func TestEvaluatePairsEachHandoffVersionWithItsGoalVersion(t *testing.T) {
	for _, tc := range []struct {
		name          string
		handoff, goal string
		detail        string
	}{
		{"v4 handoff with a v2 goal", validHandoffV4JSON, goalV2JSONWithNonGoals(`[]`), "goal has version 2, want 3"},
		{"v3 handoff with a v3 goal", validHandoffV3JSON, goalV3JSONWithNonGoals(`[]`), "goal has version 3, want 2"},
		{"v1 handoff with a v3 goal", validHandoffJSON, goalV3JSONWithNonGoals(`[]`), "goal has version 3, want 2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Evaluate(inputWith(t, []byte(tc.handoff), []byte(tc.goal)), nil)
			var detail string
			for _, b := range got.Blockers {
				if b.Reason == ReasonGoalInvalid {
					detail = b.Detail
				}
			}
			if detail != tc.detail {
				t.Errorf("goal_invalid detail = %q, want %q (blockers %v)", detail, tc.detail, got.Blockers)
			}
			if got.Subject != nil || got.View != nil {
				t.Errorf("a mismatched Goal version still produced a subject or view")
			}
		})
	}
}

func TestRenderViewV4ShowsBothDirectives(t *testing.T) {
	view := string(freshAssessment(t, completeV4Input(t)).View)
	for _, want := range []string{
		"labdrian shaper clearance view 4\n",
		viewValue("memory_directive", "version=1 scope=goal sources=[engram,longterm-mem] write=none"),
		viewValue("goal_memory_directive", "version=1 scope=project sources=[engram] write=none"),
		viewValue("delivery_limit", "No delivery without human clearance."),
		"role 11\nimplementer\n",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("v4 view lacks %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "memory_scope ") {
		t.Errorf("v4 view still shows a memory_scope section:\n%s", view)
	}
	if i, j := strings.Index(view, "acceptance 2\n"), strings.Index(view, "memory_directive "); i < 0 || j < i {
		t.Errorf("the directives must come after the acceptance section:\n%s", view)
	}
}

func TestRenderViewShowsAnEmptySourceList(t *testing.T) {
	handoff := strings.Replace(validHandoffV4JSON, validDirectiveJSON, `{"version":1,"scope":"none","sources":[],"write":"none"}`, 1)
	view := string(freshAssessment(t, inputWith(t, []byte(handoff), []byte(goalV3JSONWithNonGoals(`[]`)))).View)
	if want := viewValue("memory_directive", "version=1 scope=none sources=[] write=none"); !strings.Contains(view, want) {
		t.Errorf("v4 view lacks %q:\n%s", want, view)
	}
}

func TestEvaluateV4RefusesAGoalNonGoalOverlap(t *testing.T) {
	got := Evaluate(inputWith(t, []byte(validHandoffV4JSON), []byte(goalV3JSONWithNonGoals(`["Extract jsonstrict."]`))), nil)
	if !hasReason(got, ReasonGoalNonGoalOverlap) || len(got.Flags) != 0 {
		t.Errorf("blockers %v flags %v, want the refused OD6 overlap and no flag", blockerReasons(got), got.Flags)
	}
}

func TestReadyDisclosureForV4NamesTheDirective(t *testing.T) {
	got := ReadyDisclosureFor(4)
	if got != ReadyDisclosureV4 {
		t.Fatalf("ReadyDisclosureFor(4) = %q, want ReadyDisclosureV4", got)
	}
	if !strings.HasPrefix(got, ForgeryDisclosure+" ") || !strings.Contains(got, "memory_directive") || strings.Contains(got, "memory_scope") {
		t.Errorf("ReadyDisclosureV4 = %q, want the forgery limit first and memory_directive in place of memory_scope", got)
	}
	if ReadyDisclosureFor(3) != ReadyDisclosureV3 || ReadyDisclosureFor(2) != ReadyDisclosure {
		t.Errorf("the disclosures of versions 2 and 3 changed")
	}
}

func TestBindGoalPairsAVersion4HandoffWithAVersion3Goal(t *testing.T) {
	h, err := Parse([]byte(validHandoffV4JSON))
	if err != nil {
		t.Fatalf("Parse(v4): %v", err)
	}
	goalV3 := []byte(goalV3JSONWithNonGoals(`[]`))
	got, err := BindGoal(&fakeSource{files: map[string][]byte{"goal.json": goalV3}}, h, sourceRoot, "goal.json")
	if err != nil {
		t.Fatalf("BindGoal(v4 handoff, v3 goal): %v", err)
	}
	if got.Goal.MemoryDirective == nil || got.Goal.MemoryDirective.Scope != "project" {
		t.Errorf("bound Goal directive = %#v, want the Goal's project-scope directive", got.Goal.MemoryDirective)
	}

	v2 := []byte(goalV2JSONWithNonGoals(`[]`))
	got, err = BindGoal(&fakeSource{files: map[string][]byte{"goal.json": v2}}, h, sourceRoot, "goal.json")
	assertRejectedWithoutPartialBinding(t, got, err, "a version 2 Goal for a version 4 handoff")
	if want := `bind goal: goal source "goal.json" has version 2, want 3`; err.Error() != want {
		t.Errorf("BindGoal error = %q, want %q", err, want)
	}

	v3Handoff, err := Parse([]byte(validHandoffV3JSON))
	if err != nil {
		t.Fatalf("Parse(v3): %v", err)
	}
	got, err = BindGoal(&fakeSource{files: map[string][]byte{"goal.json": goalV3}}, v3Handoff, sourceRoot, "goal.json")
	assertRejectedWithoutPartialBinding(t, got, err, "a version 3 Goal for a version 3 handoff")
	if want := `bind goal: goal source "goal.json" has version 3, want 2`; err.Error() != want {
		t.Errorf("BindGoal error = %q, want %q", err, want)
	}
}
