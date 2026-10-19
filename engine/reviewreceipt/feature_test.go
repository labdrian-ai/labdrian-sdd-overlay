package reviewreceipt_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewreceipt"
)

// The capture into an ODD feature (C23 M5d): an ODD review belongs to no SDD change, so its
// receipts are persisted beside the feature's task document, and the hook accepts a receipt
// persisted there as it accepts one persisted under a change.

const feature = "consolidation-phase9"

// CaptureFeature persists every approved receipt into the feature's folder, under the name its
// shape gives it, by the same selection Capture makes, and reports where each is.
func TestCaptureFeaturePersistsIntoTheFeatureFolder(t *testing.T) {
	w := newWorld()
	w.stores = []reviewreceipt.Store{"s"}
	w.docs["s"] = []reviewreceipt.Document{approvedReceiptDoc("review-a"), stateDoc("review-939c22285467c2b0", "approved"), stateDoc("review-busy", "reviewing")}

	captured, err := w.service().CaptureFeature(feature)
	if err != nil {
		t.Fatalf("CaptureFeature: %v", err)
	}
	wantWrites := []string{"feature:" + feature + "/review-a.json", "feature:" + feature + "/review-939c22285467c2b0.review-state.json"}
	if !reflect.DeepEqual(w.writes, wantWrites) {
		t.Errorf("writes = %v, want %v", w.writes, wantWrites)
	}
	want := w.Location(reviewreceipt.InFeature(feature), "review-939c22285467c2b0.review-state.json")
	if len(captured) != 2 || captured[1].LineageID != "review-939c22285467c2b0" || captured[1].Path != want {
		t.Errorf("captured = %+v, want the state at %q", captured, want)
	}
	if got := string(w.persisted["feature:"+feature+"/review-a.json"]); got != string(approvedReceiptDoc("review-a").Data) {
		t.Errorf("persisted %q, want the exact bytes of the document", got)
	}
	for _, call := range w.calls {
		if call == "Changes" || strings.HasPrefix(call, "HasArtifact") {
			t.Errorf("the capture into a feature asked the change catalog: %v", w.calls)
		}
	}
}

// What is already persisted in the feature, byte for byte, is left alone and still reported; a
// different file at the name is never replaced, exactly as for a change.
func TestCaptureFeatureIsIdempotentAndNeverReplacesADifferingFile(t *testing.T) {
	w := newWorld()
	w.stores = []reviewreceipt.Store{"s"}
	w.docs["s"] = []reviewreceipt.Document{approvedReceiptDoc("review-a")}

	if _, err := w.service().CaptureFeature(feature); err != nil {
		t.Fatalf("first CaptureFeature: %v", err)
	}
	captured, err := w.service().CaptureFeature(feature)
	if err != nil || len(captured) != 1 || len(w.writes) != 1 {
		t.Fatalf("second CaptureFeature = %+v, %v with writes %v, want the receipt reported and not written again", captured, err, w.writes)
	}

	w = newWorld()
	w.stores = []reviewreceipt.Store{"s"}
	w.docs["s"] = []reviewreceipt.Document{approvedReceiptDoc("review-a")}
	w.persisted["feature:"+feature+"/review-a.json"] = []byte(`{"different":true}`)
	_, err = w.service().CaptureFeature(feature)
	wantErr := "reviewreceipt: " + w.Location(reviewreceipt.InFeature(feature), "review-a.json") + " already exists with different content"
	if err == nil || err.Error() != wantErr {
		t.Fatalf("CaptureFeature error = %v, want %q", err, wantErr)
	}
	if len(w.writes) != 0 || string(w.persisted["feature:"+feature+"/review-a.json"]) != `{"different":true}` {
		t.Errorf("the differing file was replaced: writes %v", w.writes)
	}
}

// A feature name that is blank, or not one path component, is refused by name before anything
// is asked: the feature's folder is joined from it.
func TestCaptureFeatureRefusesANameThatIsNotOnePathComponent(t *testing.T) {
	for _, name := range []string{"", "   "} {
		w := newWorld()
		captured, err := w.service().CaptureFeature(name)
		if err == nil || err.Error() != "reviewreceipt: feature name is required" || captured != nil {
			t.Errorf("CaptureFeature(%q) = %v, %v, want the refusal", name, captured, err)
		}
		w.calledNothing(t)
	}
	for _, name := range []string{"../x", "a/b", "..", ".", " padded", "padded ", `a\b`, "tab\there"} {
		w := newWorld()
		captured, err := w.service().CaptureFeature(name)
		var unsafe *reviewreceipt.UnsafeNameError
		if !errors.As(err, &unsafe) || unsafe.Kind != "feature name" || unsafe.Name != name || captured != nil {
			t.Errorf("CaptureFeature(%q) = %v, %v, want an *UnsafeNameError for the feature name", name, captured, err)
		}
		w.calledNothing(t)
	}
}

// ambiguous is a world with two active changes and two approved receipts, one of each shape.
func ambiguous() (*world, reviewreceipt.Document, reviewreceipt.Document) {
	w := newWorld()
	w.changes, w.artifacts = []string{"b", "a"}, map[string][]string{"a": {"tasks.md"}, "b": {"tasks.md"}}
	w.stores = []reviewreceipt.Store{"s"}
	r, st := approvedReceiptDoc("review-1"), stateDoc("review-2", "approved")
	w.docs["s"] = []reviewreceipt.Document{r, st}
	return w, r, st
}

// With several active changes, an acknowledgement passes once every surviving approved receipt
// is persisted byte for byte under one of the changes or in any feature folder.
func TestHookAllowsAnAmbiguousChangeOnceEveryReceiptIsPersistedInAFeature(t *testing.T) {
	for _, tc := range []struct {
		name     string
		features []string
		persist  func(w *world, r, st reviewreceipt.Document)
	}{
		{"both in one feature", []string{"other", feature}, func(w *world, r, st reviewreceipt.Document) {
			w.persisted["feature:"+feature+"/review-1.json"] = r.Data
			w.persisted["feature:"+feature+"/review-2.review-state.json"] = st.Data
		}},
		{"one in a change, one in a feature", []string{feature}, func(w *world, r, st reviewreceipt.Document) {
			w.persisted["a/review-1.json"] = r.Data
			w.persisted["feature:"+feature+"/review-2.review-state.json"] = st.Data
		}},
		{"in two features", []string{"f1", "f2"}, func(w *world, r, st reviewreceipt.Document) {
			w.persisted["feature:f1/review-1.json"] = r.Data
			w.persisted["feature:f2/review-2.review-state.json"] = st.Data
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, r, st := ambiguous()
			w.features = tc.features
			tc.persist(w, r, st)
			if v := w.service().CheckCommand(ackCommand); v.Deny || v.Reason != "" {
				t.Errorf("CheckCommand = %+v, want an allow", v)
			}
			if len(w.writes) != 0 {
				t.Errorf("the hook wrote %v", w.writes)
			}
		})
	}
}

// The hook still denies, naming both remedies, while a surviving approved receipt is persisted
// nowhere, or is persisted in a feature with other bytes; and it denies when the feature folders
// cannot be listed, since a receipt may be there.
func TestHookStillDeniesAnAmbiguousChangeWhileAReceiptIsNotPersisted(t *testing.T) {
	wantDeny := "reviewreceipt: multiple active changes (a, b); run `review-receipt capture --change <name>` or `review-receipt capture --feature <name>` before acknowledging"
	for _, tc := range []struct {
		name    string
		persist func(w *world, r, st reviewreceipt.Document)
	}{
		{"one is persisted nowhere", func(w *world, r, _ reviewreceipt.Document) {
			w.persisted["feature:"+feature+"/review-1.json"] = r.Data
		}},
		{"one is in the feature with other bytes", func(w *world, r, _ reviewreceipt.Document) {
			w.persisted["feature:"+feature+"/review-1.json"] = r.Data
			w.persisted["feature:"+feature+"/review-2.review-state.json"] = []byte("{}")
		}},
		{"one is in a folder that is not a feature the sink lists", func(w *world, r, st reviewreceipt.Document) {
			w.persisted["feature:"+feature+"/review-1.json"] = r.Data
			w.persisted["feature:unlisted/review-2.review-state.json"] = st.Data
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, r, st := ambiguous()
			w.features = []string{feature}
			tc.persist(w, r, st)
			if v := w.service().CheckCommand(ackCommand); !v.Deny || v.Reason != wantDeny {
				t.Errorf("CheckCommand = %+v, want a denial %q", v, wantDeny)
			}
			if len(w.writes) != 0 {
				t.Errorf("the hook guessed a destination and wrote %v", w.writes)
			}
		})
	}

	w, _, _ := ambiguous()
	w.featuresErr = errors.New("read /project/odd/review-receipts: permission denied")
	if v := w.service().CheckCommand(ackCommand); !v.Deny || v.Reason != "review-receipt: reviewreceipt: read /project/odd/review-receipts: permission denied" {
		t.Errorf("features cannot be listed: CheckCommand = %+v, want a denial naming the folder", v)
	}

	w, r, st := ambiguous()
	w.features = []string{"bad name "}
	w.persisted["a/review-1.json"], w.persisted["a/review-2.review-state.json"] = r.Data, st.Data
	if v := w.service().CheckCommand(ackCommand); v.Deny {
		t.Errorf("every receipt under a change: CheckCommand = %+v, want an allow before any feature is asked", v)
	}
	// A folder whose name capture would refuse is no feature: no receipt counts as persisted there,
	// and it does not stop the receipts in the features beside it from counting.
	w, r, st = ambiguous()
	w.features = []string{" bad", feature}
	w.persisted["a/review-1.json"] = r.Data
	w.persisted["feature:"+feature+"/review-2.review-state.json"] = st.Data
	if v := w.service().CheckCommand(ackCommand); v.Deny {
		t.Errorf("an unsafe folder beside the feature: CheckCommand = %+v, want an allow", v)
	}
	w, r, st = ambiguous()
	w.features = []string{"bad name "}
	w.persisted["a/review-1.json"] = r.Data
	w.persisted["feature:bad name /review-2.review-state.json"] = st.Data
	if v := w.service().CheckCommand(ackCommand); !v.Deny || v.Reason != wantDeny {
		t.Errorf("a receipt only in an unsafe folder: CheckCommand = %+v, want a denial %q", v, wantDeny)
	}
}

// The single-change branch and the zero-change branch are unchanged: one active change is
// captured into, and no feature is asked about; none passes through without asking.
func TestTheOtherBranchesOfTheHookNeverAskForFeatures(t *testing.T) {
	w := newWorld()
	w.changes, w.artifacts = []string{"only"}, map[string][]string{"only": {"tasks.md"}}
	w.stores = []reviewreceipt.Store{"s"}
	w.docs["s"] = []reviewreceipt.Document{approvedReceiptDoc("review-solo")}
	w.features = []string{feature}
	w.persisted["feature:"+feature+"/review-solo.json"] = approvedReceiptDoc("review-solo").Data
	if v := w.service().CheckCommand(ackCommand); v.Deny {
		t.Fatalf("one change: CheckCommand = %+v, want an allow", v)
	}
	if !reflect.DeepEqual(w.writes, []string{"only/review-solo.json"}) {
		t.Errorf("one change: writes = %v, want the capture into the change", w.writes)
	}

	z := newWorld()
	z.featuresErr = errors.New("must not be asked")
	if v := z.service().CheckCommand(ackCommand); v.Deny {
		t.Errorf("no change: CheckCommand = %+v, want an allow", v)
	}

	for _, calls := range [][]string{w.calls, z.calls} {
		for _, call := range calls {
			if call == "Features" {
				t.Errorf("the hook asked for the features: %v", calls)
			}
		}
	}
}
