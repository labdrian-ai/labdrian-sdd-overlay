package shaper

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadContainedSourceReadsARegularFileInsideRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "h.json"), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	cleaned, data, err := ReadContainedSource(root, "./h.json")
	if err != nil {
		t.Fatalf("ReadContainedSource: %v", err)
	}
	if cleaned != "h.json" || string(data) != "not json" {
		t.Errorf("ReadContainedSource = (%q, %q)", cleaned, data)
	}
	for _, bad := range []string{"../h.json", "/etc/passwd", "missing.json"} {
		if _, _, err := ReadContainedSource(root, bad); err == nil {
			t.Errorf("ReadContainedSource(%q) accepted", bad)
		}
	}
}

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
	for _, want := range []string{"not a signature", "same OS user", "any installed Pi extension", "Phase 3", "no execution authority"} {
		if !strings.Contains(ForgeryDisclosure, want) {
			t.Errorf("ForgeryDisclosure does not state %q:\n%s", want, ForgeryDisclosure)
		}
	}
}
