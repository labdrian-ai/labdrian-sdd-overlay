package reviewreceipt_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewreceipt"
)

const legacyApproved = `{"schema":"gentle-ai.review-receipt/v2","lineage_id":"review-summary1",` +
	`"final_candidate_tree":"candidatetree1","base_tree":"basetree1",` +
	`"selected_lenses":["review-risk","review-readability"],"risk_level":"medium","terminal_state":"approved"}`

// Parse reads either shape of an approved review, and what it returns does not depend on
// which one the review was written in.
func TestParseReadsTheSameSummaryFromBothShapes(t *testing.T) {
	want := reviewreceipt.ApprovedReceipt{
		Lineage:            "review-summary1",
		FinalCandidateTree: "candidatetree1",
		BaseTree:           "basetree1",
		Lenses:             []string{"review-risk", "review-readability"},
		Risk:               "medium",
	}

	fromReceipt, err := reviewreceipt.Parse("receipt.json", []byte(legacyApproved))
	if err != nil {
		t.Fatalf("Parse(legacy receipt): %v", err)
	}
	fromState, err := reviewreceipt.Parse("state.json", stateJSON("review-summary1", "approved"))
	if err != nil {
		t.Fatalf("Parse(lifecycle state): %v", err)
	}

	// The shape is the one thing that differs: it says which file the review was kept in.
	if fromReceipt.Shape != reviewreceipt.ShapeReceipt || fromState.Shape != reviewreceipt.ShapeState {
		t.Errorf("shapes = %v and %v, want the legacy receipt and the lifecycle state", fromReceipt.Shape, fromState.Shape)
	}
	fromReceipt.Shape, fromState.Shape = 0, 0
	if !reflect.DeepEqual(fromReceipt, want) || !reflect.DeepEqual(fromState, want) {
		t.Errorf("Parse = %#v and %#v, want both %#v", fromReceipt, fromState, want)
	}
}

// The name a receipt is persisted under says which shape it was, so both shapes of one
// lineage can sit side by side in a change's review-receipts folder.
func TestAnApprovedReceiptNamesTheFileItIsPersistedUnder(t *testing.T) {
	for _, tc := range []struct {
		shape reviewreceipt.Shape
		want  string
	}{
		{reviewreceipt.ShapeReceipt, "review-a.json"},
		{reviewreceipt.ShapeState, "review-a.review-state.json"},
	} {
		if got := (reviewreceipt.ApprovedReceipt{Shape: tc.shape, Lineage: "review-a"}).FileName(); got != tc.want {
			t.Errorf("FileName of shape %v = %q, want %q", tc.shape, got, tc.want)
		}
	}
}

// Parse refuses what is not an approved review, in words that name the source it was given.
func TestParseRefusesWhatIsNotAnApprovedReview(t *testing.T) {
	for _, tc := range []struct {
		name, data, want string
		prefix           bool
	}{
		{"a legacy receipt that was declined", strings.Replace(legacyApproved, `"approved"`, `"declined"`, 1),
			`reviewreceipt: x.json is not approved (terminal_state="declined")`, false},
		{"a lifecycle state still in review", string(stateJSON("review-a", "reviewing")),
			`reviewreceipt: x.json is not approved (state="reviewing")`, false},
		{"text that is not JSON", "this is not json",
			"reviewreceipt: x.json is neither a recognized receipt nor review-state file: ", true},
		{"a document of another schema with no lifecycle state", `{"schema":"gentle-ai.review-receipt/v1","lineage_id":"review-a","terminal_state":"approved"}`,
			"reviewreceipt: x.json is neither a recognized receipt nor review-state file", false},
		{"an empty object", `{}`, "reviewreceipt: x.json is neither a recognized receipt nor review-state file", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := reviewreceipt.Parse("x.json", []byte(tc.data))
			switch {
			case err == nil:
				t.Fatalf("Parse accepted %s as %#v", tc.name, got)
			case tc.prefix && !strings.HasPrefix(err.Error(), tc.want), !tc.prefix && err.Error() != tc.want:
				t.Errorf("Parse = %q, want %q", err, tc.want)
			}
			if !reflect.DeepEqual(got, reviewreceipt.ApprovedReceipt{}) {
				t.Errorf("Parse returned %#v alongside its error, want the zero value", got)
			}
		})
	}
}

// A legacy receipt that is approved but names no lineage is still read: Capture refuses to
// persist it (a file needs a name), but the summary of an approved review is what its
// document says. This is how the reader has always behaved and the archive gate relies on
// it for its own checks, so it is kept and said here.
func TestParseReadsAnApprovedLegacyReceiptThatNamesNoLineage(t *testing.T) {
	data := strings.Replace(legacyApproved, `"lineage_id":"review-summary1"`, `"lineage_id":""`, 1)
	got, err := reviewreceipt.Parse("x.json", []byte(data))
	if err != nil || got.Lineage != "" || got.FinalCandidateTree != "candidatetree1" {
		t.Errorf("Parse = %#v, %v, want the summary with an empty lineage", got, err)
	}
}
