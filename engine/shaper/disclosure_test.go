package shaper

import (
	"strings"
	"testing"
)

// The disclosures are the domain's statements of the limits of its own claim. Their exact
// text is pinned where it is printed (the golden files of the shaper verbs in engine/cmd,
// and the Pi gate's copy of ForgeryDisclosure); these tests pin what each statement must
// say and which one a handoff version owes.

func TestForgeryDisclosureNamesTheTrustLimits(t *testing.T) {
	for _, want := range []string{"not a signature", "same OS user", "any installed Pi extension", "no execution authority"} {
		if !strings.Contains(ForgeryDisclosure, want) {
			t.Errorf("ForgeryDisclosure does not state %q:\n%s", want, ForgeryDisclosure)
		}
	}
	// The unmet Phase 3 plan outcome is a version 2 fact, not a trust limit:
	// only the version 2 ready disclosure states it.
	if !strings.Contains(ReadyDisclosureFor(2), "Phase 3 plan outcome is not yet met") {
		t.Errorf("version 2 ready disclosure must state the unmet Phase 3 plan outcome:\n%s", ReadyDisclosureFor(2))
	}
}

func TestPlanIncompleteDisclosureNamesAbsentPhase3Fields(t *testing.T) {
	for _, want := range []string{"Phase 3", "not yet met", "roles", "tests", "risks", "estimates", "memory_scope", "delivery_limit"} {
		if !strings.Contains(ReadyDisclosure, want) {
			t.Errorf("ReadyDisclosure does not state %q:\n%s", want, ReadyDisclosure)
		}
	}
	if !strings.HasPrefix(ReadyDisclosure, ForgeryDisclosure) {
		t.Errorf("ReadyDisclosure must keep the forgery-limit disclosure first")
	}
}

func TestReadyDisclosureV3StatesCompletenessNotAbsence(t *testing.T) {
	for _, want := range []string{"roles", "tests", "risks", "estimates", "memory_scope", "delivery_limit", "no execution authority"} {
		if !strings.Contains(strings.ToLower(ReadyDisclosureV3), strings.ToLower(want)) {
			t.Errorf("ReadyDisclosureV3 does not state %q:\n%s", want, ReadyDisclosureV3)
		}
	}
	if strings.Contains(ReadyDisclosureV3, "not yet met") {
		t.Errorf("ReadyDisclosureV3 must not say the Phase 3 plan outcome is unmet:\n%s", ReadyDisclosureV3)
	}
	if strings.Contains(ReadyDisclosureV3, "are absent") {
		t.Errorf("ReadyDisclosureV3 must not claim missing plan fields:\n%s", ReadyDisclosureV3)
	}
	if !strings.HasPrefix(ReadyDisclosureV3, ForgeryDisclosure) {
		t.Errorf("ReadyDisclosureV3 must keep the forgery-limit disclosure first")
	}
}

// Version 3 is the only handoff version whose ready claim covers the full Phase 3 plan, so
// it is the only one that gets the completeness statement; every other version, the ones
// that exist and any that does not, gets the version 2 statement, which says what is
// absent. A version that is not 3 never gets the statement that claims the plan complete.
func TestReadyDisclosureForSelectsByVersion(t *testing.T) {
	for _, tc := range []struct {
		version int
		want    string
	}{
		{-1, ReadyDisclosure},
		{0, ReadyDisclosure},
		{1, ReadyDisclosure},
		{2, ReadyDisclosure},
		{3, ReadyDisclosureV3},
		{4, ReadyDisclosure},
	} {
		got := ReadyDisclosureFor(tc.version)
		if got != tc.want {
			t.Errorf("ReadyDisclosureFor(%d) = %q, want the %s statement", tc.version, got, map[bool]string{true: "version 3", false: "version 2"}[tc.want == ReadyDisclosureV3])
		}
		if !strings.HasPrefix(got, ForgeryDisclosure) {
			t.Errorf("ReadyDisclosureFor(%d) does not keep the forgery limit first", tc.version)
		}
	}
}
