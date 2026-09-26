package shaper

import (
	"reflect"
	"strings"
	"testing"
)

// completeV3Input returns readiness evidence in which every piece a caller
// can supply is present and consistent for a version 3 handoff. Only
// clearance is left for Evaluate to find missing.
func completeV3Input(t *testing.T) ReadinessInput {
	t.Helper()
	return inputWith(t, []byte(validHandoffV3JSON), []byte(goalV2JSONWithNonGoals(`[]`)))
}

func TestReasonGoalNonGoalOverlapIsRefusedAndListed(t *testing.T) {
	if !ReasonGoalNonGoalOverlap.Valid() {
		t.Fatalf("ReasonGoalNonGoalOverlap is not in the closed vocabulary")
	}
	if k := ReasonGoalNonGoalOverlap.Kind(); k != ReasonKindRefused {
		t.Errorf("Kind = %q, want refused", k)
	}
	found := false
	for _, r := range Reasons() {
		if r == ReasonGoalNonGoalOverlap {
			found = true
		}
	}
	if !found {
		t.Errorf("Reasons() does not list ReasonGoalNonGoalOverlap")
	}
}

func TestEvaluateV3DropsStraightToDraftWithOnlyClearanceMissing(t *testing.T) {
	got := Evaluate(completeV3Input(t), nil)
	if got.State != StateDraft {
		t.Fatalf("State = %q, want draft (blockers %v)", got.State, blockerReasons(got))
	}
	if want := []Reason{ReasonClearanceMissing}; !reflect.DeepEqual(blockerReasons(got), want) {
		t.Errorf("blockers = %v, want %v", blockerReasons(got), want)
	}
	if hasReason(got, ReasonAcceptanceVerificationUnrepresentable) {
		t.Errorf("v3 handoff kept the v1 acceptance cap: %v", blockerReasons(got))
	}
}

func TestEvaluateV3ReachesReadyWithVerifiedClearance(t *testing.T) {
	in := completeV3Input(t)
	c := verifiedFor(t, in)
	got := Evaluate(in, c)
	if got.State != StateReady {
		t.Fatalf("State = %q, want ready (blockers %v)", got.State, blockerReasons(got))
	}
	if len(got.Blockers) != 0 {
		t.Errorf("blockers = %v, want none", blockerReasons(got))
	}
	if got.Subject == nil || len(got.View) == 0 {
		t.Errorf("ready assessment lacks its subject or view")
	}
}

// TestEvaluateV3OverlapWithGoalNonGoalIsRefusedNotFlagged proves OD6: a v3
// stage, acceptance criterion, test, or role responsibility byte-identical to
// a Goal non_goals item is a refused rejection, never a human-review flag.
func TestEvaluateV3OverlapWithGoalNonGoalIsRefusedNotFlagged(t *testing.T) {
	for _, tc := range []struct {
		name    string
		nonGoal string
	}{
		{name: "stage", nonGoal: "Extract jsonstrict."},
		{name: "acceptance criterion", nonGoal: "go test ./... passes."},
		{name: "test", nonGoal: "staticcheck ./... is clean."},
		{name: "role responsibility", nonGoal: "Writes the parsing and validation code."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			goalBytes := []byte(goalV2JSONWithNonGoals(`["` + tc.nonGoal + `"]`))
			got := Evaluate(inputWith(t, []byte(validHandoffV3JSON), goalBytes), nil)
			if got.State != StateDraft {
				t.Fatalf("State = %q, want draft", got.State)
			}
			if !hasReason(got, ReasonGoalNonGoalOverlap) {
				t.Errorf("blockers = %v, want to include %q", blockerReasons(got), ReasonGoalNonGoalOverlap)
			}
			if len(got.Flags) != 0 {
				t.Errorf("Flags = %v, want none: v3 overlap is refused, not flagged", got.Flags)
			}
			if hasReason(got, ReasonFlagsUnresolved) {
				t.Errorf("blockers = %v, want no %q for a refused v3 overlap", blockerReasons(got), ReasonFlagsUnresolved)
			}
		})
	}
}

func TestEvaluateV1V2NonGoalOverlapStillFlagsNotRefuses(t *testing.T) {
	got := Evaluate(inputWith(t, []byte(validHandoffJSON), []byte(goalV2JSONWithNonGoals(`["Extract jsonstrict."]`))), nil)
	if hasReason(got, ReasonGoalNonGoalOverlap) {
		t.Errorf("v1 handoff got the v3 refused overlap reason: %v", blockerReasons(got))
	}
	if len(got.Flags) != 1 {
		t.Errorf("v1 handoff overlap did not raise the human-review flag: %v", got.Flags)
	}
}

// --- presented view ---

func TestRenderViewV3ShowsSixFieldsAndGoalPairing(t *testing.T) {
	a := freshAssessment(t, completeV3Input(t))
	view := string(a.View)
	for _, want := range []string{
		"labdrian shaper clearance view 3\n",
		"memory_scope 15\nProject-scoped.\n",
		"goal_memory_scope 15\nProject-scoped.\n",
		"delivery_limit 36\nNo delivery without human clearance.\n",
		"goal_delivery_boundary 12\nNo delivery.\n",
		"role 11\nimplementer\n",
		"responsibility 39\nWrites the parsing and validation code.\n",
		"role 8\nreviewer\n",
		"item 27\nstaticcheck ./... is clean.\n",
		"risk 25\nNested key casing drifts.\n",
		"mitigation 30\nReuse checkAcceptanceV2Fields.\n",
		"stage 19\nExtract jsonstrict.\n",
		"low_minutes 10\n",
		"high_minutes 20\n",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("v3 view lacks %q:\n%s", want, view)
		}
	}
	if i, j := strings.Index(view, "acceptance 2\n"), strings.Index(view, "memory_scope "); i < 0 || j < i {
		t.Errorf("v3 fields must come after the acceptance section:\n%s", view)
	}
	if i, j := strings.Index(view, "memory_scope "), strings.Index(view, "\nflags "); i < 0 || j < i {
		t.Errorf("v3 fields must come before the flags section:\n%s", view)
	}
}

func TestRenderViewV1AndV2AreUnchangedByV3(t *testing.T) {
	v1 := freshAssessment(t, completeInput(t))
	if strings.Contains(string(v1.View), "\nmemory_scope ") || strings.Contains(string(v1.View), "\nroles ") {
		t.Errorf("v1 view gained a v3 section:\n%s", v1.View)
	}
	v2 := freshAssessment(t, completeV2Input(t))
	if !strings.HasPrefix(string(v2.View), "labdrian shaper clearance view 2\n") {
		t.Errorf("v2 view header changed:\n%s", v2.View)
	}
	if strings.Contains(string(v2.View), "\nmemory_scope ") || strings.Contains(string(v2.View), "\nroles ") {
		t.Errorf("v2 view gained a v3 section:\n%s", v2.View)
	}
}

func TestCheckPresentableCoversEveryV3Section(t *testing.T) {
	base := PresentedView{
		GoalBytes:  []byte("g"),
		PlanBytes:  []byte("p"),
		GoalScope:  "s",
		Acceptance: []AcceptanceItem{{Criterion: "c", Verification: AcceptanceVerification{Check: "go test"}}},
		V3: &PresentedViewV3{
			Roles:                []Role{{Role: "r", Responsibility: "resp"}},
			Tests:                []string{"t"},
			Risks:                []Risk{{Risk: "risk", Mitigation: "mit"}},
			Estimates:            []Estimate{{Stage: "s", LowMinutes: 1, HighMinutes: 2}},
			MemoryScope:          "mem",
			GoalMemoryScope:      "gmem",
			DeliveryLimit:        "lim",
			GoalDeliveryBoundary: "gbound",
		},
	}
	if err := checkPresentable(base); err != nil {
		t.Fatalf("checkPresentable(clean v3) = %v, want nil", err)
	}
	bad := "\x1b[8m"
	for _, mutate := range []func(*PresentedView){
		func(v *PresentedView) { v.V3.MemoryScope = bad },
		func(v *PresentedView) { v.V3.GoalMemoryScope = bad },
		func(v *PresentedView) { v.V3.DeliveryLimit = bad },
		func(v *PresentedView) { v.V3.GoalDeliveryBoundary = bad },
		func(v *PresentedView) { v.V3.Roles[0].Role = bad },
		func(v *PresentedView) { v.V3.Roles[0].Responsibility = bad },
		func(v *PresentedView) { v.V3.Tests[0] = bad },
		func(v *PresentedView) { v.V3.Risks[0].Risk = bad },
		func(v *PresentedView) { v.V3.Risks[0].Mitigation = bad },
		func(v *PresentedView) { v.V3.Estimates[0].Stage = bad },
	} {
		v := base
		v3 := *base.V3
		v3.Roles = append([]Role(nil), base.V3.Roles...)
		v3.Tests = append([]string(nil), base.V3.Tests...)
		v3.Risks = append([]Risk(nil), base.V3.Risks...)
		v3.Estimates = append([]Estimate(nil), base.V3.Estimates...)
		v.V3 = &v3
		mutate(&v)
		if err := checkPresentable(v); err == nil {
			t.Errorf("checkPresentable accepted a control rune in a v3 section: %#v", v.V3)
		}
	}
}

func TestEvaluateV3RefusesUnpresentableV3Section(t *testing.T) {
	data := strings.Replace(validHandoffV3JSON, `"Project-scoped."`, `"Project\u001b[8m-scoped."`, 1)
	got := Evaluate(inputWith(t, []byte(data), []byte(goalV2JSONWithNonGoals(`[]`))), nil)
	if !hasReason(got, ReasonViewUnpresentable) {
		t.Fatalf("blockers = %v, want %q", blockerReasons(got), ReasonViewUnpresentable)
	}
	if got.Subject != nil || got.View != nil {
		t.Errorf("unpresentable v3 view produced a subject or view")
	}
}

// --- disclosures ---

func TestReadyDisclosureV3StatesCompletenessNotAbsence(t *testing.T) {
	for _, want := range []string{"roles", "tests", "risks", "estimates", "memory_scope", "delivery_limit", "no execution authority"} {
		if !strings.Contains(strings.ToLower(ReadyDisclosureV3), strings.ToLower(want)) {
			t.Errorf("ReadyDisclosureV3 does not state %q:\n%s", want, ReadyDisclosureV3)
		}
	}
	if strings.Contains(ReadyDisclosureV3, "are absent") {
		t.Errorf("ReadyDisclosureV3 must not claim missing plan fields:\n%s", ReadyDisclosureV3)
	}
	if !strings.HasPrefix(ReadyDisclosureV3, ForgeryDisclosure) {
		t.Errorf("ReadyDisclosureV3 must keep the forgery-limit disclosure first")
	}
}

func TestReadyDisclosureForSelectsByVersion(t *testing.T) {
	if got := ReadyDisclosureFor(1); got != ReadyDisclosure {
		t.Errorf("ReadyDisclosureFor(1) = %q, want ReadyDisclosure", got)
	}
	if got := ReadyDisclosureFor(2); got != ReadyDisclosure {
		t.Errorf("ReadyDisclosureFor(2) = %q, want ReadyDisclosure", got)
	}
	if got := ReadyDisclosureFor(3); got != ReadyDisclosureV3 {
		t.Errorf("ReadyDisclosureFor(3) = %q, want ReadyDisclosureV3", got)
	}
}
