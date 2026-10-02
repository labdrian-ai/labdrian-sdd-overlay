package receipttest_test

import (
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewreceipt"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewreceipt/receipttest"
)

// The documents the other packages' tests share are what the domain reads. A change to the
// wire format the domain accepts fails here, in one place, instead of leaving one package's
// copy of a document stale while it still compiles and its tests still pass.
func TestTheSharedDocumentsAreWhatTheDomainReads(t *testing.T) {
	for _, tc := range []struct {
		name  string
		doc   string
		shape reviewreceipt.Shape
	}{
		{"the legacy receipt", receipttest.ReceiptDocument("review-a", receipttest.ReceiptSchema, receipttest.Approved), reviewreceipt.ShapeReceipt},
		{"the lifecycle state", receipttest.StateDocument("review-a", receipttest.Approved), reviewreceipt.ShapeState},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := reviewreceipt.Parse("fixture", []byte(tc.doc))
			if err != nil {
				t.Fatalf("Parse of the shared fixture: %v (the fixture is stale, or the wire format changed)", err)
			}
			if got.Shape != tc.shape || got.Lineage != "review-a" || got.FinalCandidateTree == "" || got.BaseTree == "" || len(got.Lenses) == 0 || got.Risk == "" {
				t.Errorf("Parse = %+v, want an approved %v of lineage review-a with every field filled", got, tc.shape)
			}
		})
	}
}

// What the fixtures are not: a state that is not approved is read as not approved, and a
// receipt of another schema is not a receipt, so the tests that need those documents get
// them from the same place.
func TestTheSharedDocumentsCanBeMadeUnapproved(t *testing.T) {
	if _, err := reviewreceipt.Parse("state", []byte(receipttest.StateDocument("review-a", "reviewing"))); err == nil {
		t.Error("a state that is still being reviewed was read as approved")
	}
	if _, err := reviewreceipt.Parse("receipt", []byte(receipttest.ReceiptDocument("review-a", receipttest.ReceiptSchema, "declined"))); err == nil {
		t.Error("a declined receipt was read as approved")
	}
	if _, err := reviewreceipt.Parse("receipt", []byte(receipttest.ReceiptDocument("review-a", "gentle-ai.review-receipt/v1", receipttest.Approved))); err == nil {
		t.Error("a receipt of another schema was read as approved")
	}
}
