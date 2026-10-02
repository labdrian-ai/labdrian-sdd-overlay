package shaper

import (
	"strings"
	"testing"
)

func TestCheckRecordBindingAcceptsBothDecisionsAndRefusesMismatch(t *testing.T) {
	a := freshAssessment(t, overlapInput(t))
	affirm := recordFor(t, a)
	decline := recordFor(t, a)
	decline.Decision = DecisionDecline
	for _, r := range []ClearanceRecord{affirm, decline} {
		got, err := CheckRecordBinding(marshalRecord(t, r), *a.Subject, a.Flags, a.View)
		if err != nil {
			t.Fatalf("CheckRecordBinding(%s): %v", r.Decision, err)
		}
		if got.Decision != r.Decision {
			t.Errorf("decision = %q, want %q", got.Decision, r.Decision)
		}
	}

	view := append(append([]byte{}, a.View...), ' ')
	if _, err := CheckRecordBinding(marshalRecord(t, decline), *a.Subject, a.Flags, view); err == nil || !strings.Contains(err.Error(), "view_sha256") {
		t.Errorf("view drift: err = %v, want a view_sha256 mismatch", err)
	}
	unresolved := decline
	unresolved.FlagResolutions = []FlagResolution{}
	if _, err := CheckRecordBinding(marshalRecord(t, unresolved), *a.Subject, a.Flags, a.View); err == nil || !strings.Contains(err.Error(), "unresolved") {
		t.Errorf("missing resolution: err = %v, want unresolved", err)
	}
}

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
