package promote

import (
	"errors"
	"reflect"
	"testing"
)

// The two merges join what two passes found into a new list: they must not write into the first list's
// backing array, which is the caller's and may be longer than the list it holds. A slice with spare capacity
// shows it: append to the first list writes the second list's items into the caller's array past its length.

func TestMergeWithheldLeavesTheFirstListsBackingArrayAlone(t *testing.T) {
	first := make([]int64, 1, 4)
	first[0] = 1

	merged := mergeWithheld(first, []int64{2, 3})

	if want := []int64{1, 2, 3}; !reflect.DeepEqual(merged, want) {
		t.Fatalf("mergeWithheld = %v, want %v", merged, want)
	}
	if spare := first[:3]; spare[1] != 0 || spare[2] != 0 {
		t.Errorf("the first list's array past its length = %v, want it untouched: the merge appended into the caller's slice", spare)
	}
}

func TestMergeWithheldKeepsEachObservationOnceInTheOrderFirstMet(t *testing.T) {
	merged := mergeWithheld([]int64{4, 2}, []int64{2, 3, 4, 1})

	if want := []int64{4, 2, 3, 1}; !reflect.DeepEqual(merged, want) {
		t.Fatalf("mergeWithheld = %v, want %v", merged, want)
	}
}

func TestMergeFailuresLeavesTheFirstListsBackingArrayAlone(t *testing.T) {
	cause := errors.New("check promoted state: unparseable")
	first := make([]SyncFailure, 1, 4)
	first[0] = SyncFailure{ObservationID: 1, Err: cause}

	merged := mergeFailures(first, []SyncFailure{{ObservationID: 2, Err: cause}})

	if len(merged) != 2 {
		t.Fatalf("mergeFailures = %+v, want both failures", merged)
	}
	if spare := first[:2]; spare[1] != (SyncFailure{}) {
		t.Errorf("the first list's array past its length = %+v, want it untouched: the merge appended into the caller's slice", spare)
	}
}
