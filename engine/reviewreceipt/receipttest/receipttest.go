// Package receipttest holds the review documents the tests of the review receipt capture
// share: the legacy review-receipt.json and the lifecycle review-state.json that gentle-ai
// leaves for a lineage, written as the wire text the program reads. It exists so that the
// domain's tests (engine/reviewreceipt), the file-backed adapter's tests
// (engine/reviewreceipt/fsstore) and the verbs' golden tests (engine/cmd) are exercised on
// one copy of what a review document looks like. A change to the wire format then fails in
// receipttest_test.go, in one place, instead of leaving one package's copy stale while both
// still compile.
//
// It is test support: only _test.go files import it, and it is not part of any binary. It
// imports no package of the review receipt on purpose, so that a test inside the domain
// package could use it without an import cycle: everything here is text, and
// receipttest_test.go proves that the domain reads it.
package receipttest

import "fmt"

// ReceiptSchema is the only schema of the legacy receipt that the domain reads.
const ReceiptSchema = "gentle-ai.review-receipt/v2"

// Approved is the terminal state of a review that is worth keeping.
const Approved = "approved"

// ReceiptDocument is the legacy review-receipt.json (gentle-ai before 2.7.0) of a lineage,
// with the given schema and terminal state. With ReceiptSchema and Approved it is a receipt
// the domain captures.
func ReceiptDocument(lineage, schema, terminalState string) string {
	return fmt.Sprintf(`{"schema":%q,"lineage_id":%q,"final_candidate_tree":"deadbeef","base_tree":"cafe","selected_lenses":["review-risk"],"risk_level":"high","terminal_state":%q}`,
		schema, lineage, terminalState)
}

// StateDocument is the lifecycle review-state.json (gentle-ai 2.7.0 and later) of a lineage
// in the given state. With Approved it is a review the domain captures.
func StateDocument(lineage, state string) string {
	return fmt.Sprintf(`{"schema":"gentle-ai.review-transaction/v2","revision":3,"state":{"schema":"gentle-ai.review-state/v2","lineage_id":%q,"generation":1,"state":%q,"risk_level":"medium","selected_lenses":["review-risk","review-readability"],"initial_snapshot":{"base_tree":"basetree1"},"current_snapshot":{"kind":"candidate","base_tree":"basetree1","candidate_tree":"candidatetree1"}}}`,
		lineage, state)
}
