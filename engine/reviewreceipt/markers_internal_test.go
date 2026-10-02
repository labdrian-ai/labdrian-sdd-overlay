package reviewreceipt

import (
	"reflect"
	"testing"
)

// The list of artifacts that mark a change as active is handed out fresh on every call, so
// nothing a caller does to the list it got can change what the next call, or the next
// detection, sees. This is the one thing about it that cannot be seen from outside the
// package without reaching for the function itself.
func TestActiveChangeMarkersAreFreshOnEveryCall(t *testing.T) {
	// A copy: if the function shared one list, the first call's result is that list.
	want := append([]string(nil), activeChangeMarkers()...)
	if len(want) == 0 {
		t.Fatal("no marker at all: no change could ever be active")
	}

	got := activeChangeMarkers()
	got[0] = "tampered"
	got = append(got[:1], "extra")
	_ = got

	if again := activeChangeMarkers(); !reflect.DeepEqual(again, want) {
		t.Errorf("after the first list was changed, activeChangeMarkers() = %v, want it unchanged: %v", again, want)
	}
}
