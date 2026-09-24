package workflowprofile

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestBuiltinsSatisfyApprovedContract(t *testing.T) {
	wantNames := []string{"odd", "sdd", "standalone-minimal", "maintenance", "incident-recovery"}
	for _, name := range wantNames {
		t.Run(name, func(t *testing.T) {
			profile, err := Resolve(name)
			if err != nil {
				t.Fatalf("Resolve(%q): %v", name, err)
			}
			if profile.Name != name || len(profile.Stages) == 0 || len(profile.Roles) == 0 || len(profile.Checks) == 0 || profile.MemoryPolicy == "" || profile.ReviewPolicy == "" || profile.DeliveryPolicy == "" {
				t.Fatalf("profile does not define all seven contract fields: %+v", profile)
			}
			if err := Validate(profile); err != nil {
				t.Fatalf("Validate(%q): %v", name, err)
			}
		})
	}
}

func TestStageDependenciesAreOrderedAndKnown(t *testing.T) {
	profile, err := Resolve("odd")
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(profile); err != nil {
		t.Fatal(err)
	}

	profile.Stages[0].DependsOn = []string{"implement-task-by-task"}
	if err := Validate(profile); !errors.Is(err, ErrInvalidProfile) {
		t.Fatalf("forward dependency must fail closed, got %v", err)
	}
	profile, _ = Resolve("sdd")
	profile.Stages = profile.Stages[:4]
	if err := Validate(profile); !errors.Is(err, ErrInvalidProfile) {
		t.Fatalf("missing mandatory archive stage must fail closed, got %v", err)
	}
}

func TestRequiredWorkflowGatesArePresent(t *testing.T) {
	profile, _ := Resolve("sdd")
	if !testStageBefore(profile.Stages, "verify", "archive") {
		t.Fatal("SDD verify must execute before archive")
	}
	if !contains(profile.Checks, "record verify report before archive; findings do not automatically block archive") {
		t.Fatalf("SDD verify gate/report semantics missing: %#v", profile.Checks)
	}
	for name, required := range map[string][]string{
		"odd":                {"TDD only when configured", "applicable functional checks", "coordinator spot-check"},
		"standalone-minimal": {"relevant available checks", "preserve unavailable/unverified; never synthesize PASS"},
		"maintenance":        {"before/after evidence", "focused checks"},
		"incident-recovery":  {"capture initial state", "confirm final state", "stop when evidence is missing"},
	} {
		profile, err := Resolve(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, gate := range required {
			if !contains(profile.Checks, gate) {
				t.Errorf("%s missing required gate %q", name, gate)
			}
		}
	}
}

func TestSDDRequiresEveryApprovedCheck(t *testing.T) {
	requiredChecks := []string{
		"native dispatcher/dependencies",
		"configured TDD and apply checks",
		"record verify report before archive; findings do not automatically block archive",
	}
	for _, check := range requiredChecks {
		t.Run(check, func(t *testing.T) {
			profile, err := Resolve("sdd")
			if err != nil {
				t.Fatal(err)
			}
			profile.Checks = without(profile.Checks, check)
			if err := Validate(profile); !errors.Is(err, ErrInvalidProfile) {
				t.Fatalf("SDD profile without mandatory check %q must be rejected, got %v", check, err)
			}
		})
	}
}

func TestIncidentRecoveryRequiresAuditedRecoveryCheck(t *testing.T) {
	profile, err := Resolve("incident-recovery")
	if err != nil {
		t.Fatal(err)
	}
	profile.Checks = without(profile.Checks, "use only supported audited recovery")
	if err := Validate(profile); !errors.Is(err, ErrInvalidProfile) {
		t.Fatalf("incident-recovery profile without audited recovery check must be rejected, got %v", err)
	}
}

func TestResumeRequiresValidCallerStateAndKnownStage(t *testing.T) {
	profile, _ := Resolve("odd")
	state := ResumeState{ProfileName: "odd", CompletedStages: []string{"authorize", "explore", "resolve-uncertainty", "classify"}, CurrentStage: "track-if-substantial"}
	if err := ValidateResume(profile, state); err != nil {
		t.Fatalf("valid resume state rejected: %v", err)
	}
	invalid := []ResumeState{
		{},
		{ProfileName: "sdd", CurrentStage: "verify"},
		{ProfileName: "odd", CurrentStage: "unknown"},
		{ProfileName: "odd", CompletedStages: []string{"explore"}, CurrentStage: "authorize"},
		{ProfileName: "odd", CompletedStages: []string{"authorize", "explore", "classify"}, CurrentStage: "track-if-substantial"},
	}
	for _, state := range invalid {
		if err := ValidateResume(profile, state); !errors.Is(err, ErrInvalidResumeState) {
			t.Errorf("invalid resume state must fail closed: %+v: %v", state, err)
		}
	}
}

func TestUnknownAndUnsupportedProfileCompatibilityFailClosed(t *testing.T) {
	if _, err := Resolve("future-profile"); !errors.Is(err, ErrUnknownProfile) {
		t.Fatalf("unknown profile must fail closed, got %v", err)
	}
	for _, mode := range []string{"standalone", "gentle-compatible"} {
		if _, err := ResolveForCompatibility("odd", mode); !errors.Is(err, ErrUnsupportedCombination) {
			t.Fatalf("unmapped profile/compatibility pair %q must fail closed, got %v", mode, err)
		}
	}
	if _, err := ResolveForCompatibility("not-a-profile", "standalone"); !errors.Is(err, ErrUnknownProfile) {
		t.Fatalf("unknown profile must not be hidden by combination error, got %v", err)
	}
}

func TestProfileDataDoesNotGrantAuthority(t *testing.T) {
	profile, _ := Resolve("standalone-minimal")
	if profile.MemoryPolicy != "no persistence required; allow only a store explicitly configured by the caller" {
		t.Fatalf("unexpected standalone-minimal memory policy %q", profile.MemoryPolicy)
	}
	encoded, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 7 {
		t.Fatalf("profile must serialize exactly the seven declarative contract fields, got %v", fields)
	}
}

func testStageBefore(stages []Stage, first, second string) bool {
	firstIndex, secondIndex := -1, -1
	for i, stage := range stages {
		if stage.Name == first {
			firstIndex = i
		}
		if stage.Name == second {
			secondIndex = i
		}
	}
	return firstIndex >= 0 && secondIndex > firstIndex
}

func without(values []string, unwanted string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != unwanted {
			result = append(result, value)
		}
	}
	return result
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
