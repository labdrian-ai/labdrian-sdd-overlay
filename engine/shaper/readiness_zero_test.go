package shaper

import (
	"reflect"
	"testing"
)

// TestVerifiedClearanceZeroValueBindsNothing pins that the only
// VerifiedClearance a caller can build here, the zero value, carries no
// subject and no flag resolutions, so it can never stand in for a verified
// human clearance.
func TestVerifiedClearanceZeroValueBindsNothing(t *testing.T) {
	var c VerifiedClearance
	if !reflect.DeepEqual(c.subject, Subject{}) {
		t.Errorf("zero VerifiedClearance carries a subject: %+v", c.subject)
	}
	if c.resolutions != nil {
		t.Errorf("zero VerifiedClearance carries resolutions: %+v", c.resolutions)
	}
}
