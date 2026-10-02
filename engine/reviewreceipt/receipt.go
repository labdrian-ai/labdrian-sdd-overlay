package reviewreceipt

import (
	"encoding/json"
	"fmt"
)

// receiptSchema is the only schema of the legacy receipt that is read.
const receiptSchema = "gentle-ai.review-receipt/v2"

// approvedState is the only terminal state that is persisted. A surviving receipt in the
// transaction store that is NOT approved (or not yet terminal) is never captured -- only
// an approved receipt is by definition un-acknowledged and worth preserving before the
// burn.
const approvedState = "approved"

// stateSuffix distinguishes a captured review-state.json from a captured legacy receipt
// sharing the same lineage id.
const stateSuffix = ".review-state.json"

// Shape is the form a review is kept in: gentle-ai changed it across versions, and both are
// read.
type Shape int

const (
	// ShapeReceipt is the legacy review-receipt.json (gentle-ai < 2.7.0), persisted as
	// <lineage>.json.
	ShapeReceipt Shape = iota + 1
	// ShapeState is the lifecycle review-state.json (gentle-ai 2.7.0+; an
	// approved-but-unacknowledged lineage holds only this file), persisted as
	// <lineage>.review-state.json.
	ShapeState
)

func (s Shape) String() string {
	switch s {
	case ShapeReceipt:
		return "legacy receipt"
	case ShapeState:
		return "lifecycle state"
	}
	return fmt.Sprintf("shape %d", int(s))
}

// ApprovedReceipt is what a document of either shape says about an approved review: the
// facts a caller needs to verify an approved_tree anchor.
type ApprovedReceipt struct {
	// Shape is the form the review was read from.
	Shape Shape
	// Lineage is the review transaction's id.
	Lineage string
	// FinalCandidateTree is the tree that was approved.
	FinalCandidateTree string
	// BaseTree is the tree it was reviewed against.
	BaseTree string
	// Lenses are the review lenses that ran.
	Lenses []string
	// Risk is the risk level the review was run at.
	Risk string
}

// FileName is the name the review is persisted under in a change's review-receipts folder:
// the lineage, with a suffix that says which shape it was, so both shapes of one lineage
// can sit side by side.
func (r ApprovedReceipt) FileName() string {
	if r.Shape == ShapeState {
		return r.Lineage + stateSuffix
	}
	return r.Lineage + ".json"
}

// receipt is the subset of gentle-ai.review-receipt/v2 fields that is read.
type receipt struct {
	Schema             string   `json:"schema"`
	LineageID          string   `json:"lineage_id"`
	TerminalState      string   `json:"terminal_state"`
	FinalCandidateTree string   `json:"final_candidate_tree"`
	BaseTree           string   `json:"base_tree"`
	SelectedLenses     []string `json:"selected_lenses"`
	RiskLevel          string   `json:"risk_level"`
}

func (r receipt) approved() ApprovedReceipt {
	return ApprovedReceipt{
		Shape: ShapeReceipt, Lineage: r.LineageID,
		FinalCandidateTree: r.FinalCandidateTree, BaseTree: r.BaseTree,
		Lenses: r.SelectedLenses, Risk: r.RiskLevel,
	}
}

// reviewState is the subset of review-state.json's nested "state" object that is read.
type reviewState struct {
	State struct {
		LineageID       string   `json:"lineage_id"`
		State           string   `json:"state"`
		RiskLevel       string   `json:"risk_level"`
		SelectedLenses  []string `json:"selected_lenses"`
		InitialSnapshot struct {
			BaseTree string `json:"base_tree"`
		} `json:"initial_snapshot"`
		CurrentSnapshot struct {
			CandidateTree string `json:"candidate_tree"`
		} `json:"current_snapshot"`
	} `json:"state"`
}

func (s reviewState) approved() ApprovedReceipt {
	return ApprovedReceipt{
		Shape: ShapeState, Lineage: s.State.LineageID,
		FinalCandidateTree: s.State.CurrentSnapshot.CandidateTree, BaseTree: s.State.InitialSnapshot.BaseTree,
		Lenses: s.State.SelectedLenses, Risk: s.State.RiskLevel,
	}
}

// Parse reads the approved review a document holds, in either shape. It is pure: the bytes
// are the caller's to read, and label only names them in an error (a path, typically). It
// fails when the document is neither shape or is not approved, and returns the zero value
// with the error.
func Parse(label string, data []byte) (ApprovedReceipt, error) {
	var r receipt
	if json.Unmarshal(data, &r) == nil && r.Schema == receiptSchema {
		if r.TerminalState != approvedState {
			return ApprovedReceipt{}, fmt.Errorf("reviewreceipt: %s is not approved (terminal_state=%q)", label, r.TerminalState)
		}
		return r.approved(), nil
	}

	var s reviewState
	if err := json.Unmarshal(data, &s); err != nil {
		return ApprovedReceipt{}, fmt.Errorf("reviewreceipt: %s is neither a recognized receipt nor review-state file: %w", label, err)
	}
	if s.State.LineageID == "" {
		return ApprovedReceipt{}, fmt.Errorf("reviewreceipt: %s is neither a recognized receipt nor review-state file", label)
	}
	if s.State.State != approvedState {
		return ApprovedReceipt{}, fmt.Errorf("reviewreceipt: %s is not approved (state=%q)", label, s.State.State)
	}
	return s.approved(), nil
}

// approvedIn reports the approved review a document of a known shape holds, for the survey
// of the transaction stores. It is stricter than Parse in one way: a review that names no
// lineage is not found, because it could not be persisted under a name. A document that is
// not of the shape, or not approved, is not an error: the stores hold many reviews, and
// only approved ones are of interest.
func approvedIn(shape Shape, data []byte) (ApprovedReceipt, bool) {
	switch shape {
	case ShapeReceipt:
		var r receipt
		if json.Unmarshal(data, &r) == nil && r.Schema == receiptSchema &&
			r.TerminalState == approvedState && r.LineageID != "" {
			return r.approved(), true
		}
	case ShapeState:
		var s reviewState
		if json.Unmarshal(data, &s) == nil && s.State.State == approvedState && s.State.LineageID != "" {
			return s.approved(), true
		}
	}
	return ApprovedReceipt{}, false
}
