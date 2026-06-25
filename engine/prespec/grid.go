// Package prespec implements the deterministic mechanics of the pre-specification
// Socratic interview engine: the 10-cell coverage grid, Impact×Uncertainty ranking,
// readiness scoring, and no-leading lint rules.
package prespec

import (
	"errors"
	"sort"
)

// CellState represents the coverage state of a discovery grid cell.
type CellState int

const (
	// Missing means the cell has not been probed.
	Missing CellState = iota
	// Partial means the cell has been probed but not fully answered.
	Partial
	// Clear means the cell has been fully answered.
	Clear
)

// Cell is one coverage dimension in the 10-cell discovery grid.
type Cell struct {
	Key         string
	Impact      int
	Uncertainty int
	State       CellState
}

// SetState transitions the cell to the given state.
// Returns an error (leaving state unchanged) if Clear→Partial or Clear→Missing
// is attempted, as those regressions are not permitted.
func (c *Cell) SetState(s CellState) error {
	if c.State == Clear && (s == Partial || s == Missing) {
		return errors.New("prespec: cannot regress a Clear cell")
	}
	c.State = s
	return nil
}

// DefaultCells returns the canonical 10-cell taxonomy in index order (R-005a).
func DefaultCells() []Cell {
	return []Cell{
		{Key: "jtbd-job", Impact: 5, Uncertainty: 5},
		{Key: "current-gap", Impact: 5, Uncertainty: 4},
		{Key: "why-now", Impact: 4, Uncertainty: 5},
		{Key: "user-segment", Impact: 4, Uncertainty: 3},
		{Key: "constraints", Impact: 3, Uncertainty: 4},
		{Key: "success-metric", Impact: 3, Uncertainty: 3},
		{Key: "alternatives", Impact: 3, Uncertainty: 3},
		{Key: "stakeholders", Impact: 2, Uncertainty: 3},
		{Key: "frequency", Impact: 2, Uncertainty: 2},
		{Key: "risk-unknowns", Impact: 2, Uncertainty: 4},
	}
}

// Grid holds the 10-cell coverage state for one interview session.
type Grid struct {
	Cells []Cell
}

// New returns a fresh Grid with all 10 cells in Missing state (R-005b).
func New() Grid {
	return Grid{Cells: DefaultCells()}
}

// RankUncovered returns all non-Clear cells sorted by Impact×Uncertainty
// descending, with ascending cell index as tie-break (R-005c).
func (g Grid) RankUncovered() []Cell {
	var uncovered []Cell
	for _, c := range g.Cells {
		if c.State != Clear {
			uncovered = append(uncovered, c)
		}
	}
	// sort.SliceStable preserves original index order for equal-score cells.
	sort.SliceStable(uncovered, func(i, j int) bool {
		si := uncovered[i].Impact * uncovered[i].Uncertainty
		sj := uncovered[j].Impact * uncovered[j].Uncertainty
		return si > sj
	})
	return uncovered
}

// CoverageCount returns the number of Clear, Partial, and Missing cells (R-016).
func CoverageCount(g Grid) (clear, partial, empty int) {
	for _, c := range g.Cells {
		switch c.State {
		case Clear:
			clear++
		case Partial:
			partial++
		default:
			empty++
		}
	}
	return
}

const standardBudget = 5

// BudgetRemaining returns how many questions remain in the current mode (R-006a).
// Unknown modes fall back to a safe default of 5.
func BudgetRemaining(asked int, mode string) int {
	budget := standardBudget // safe default covers unknown modes
	if mode == "standard" {
		budget = standardBudget
	}
	rem := budget - asked
	if rem < 0 {
		return 0
	}
	return rem
}

// StopReason is the named reason a session should stop.
type StopReason string

const (
	// StopBudget fires when the question budget is exhausted.
	StopBudget StopReason = "budget-exhausted"
	// StopConverged fires when enough cells are Clear to meet the convergence threshold.
	StopConverged StopReason = "coverage-threshold"
	// StopUserSignal fires when the user has explicitly requested to stop.
	StopUserSignal StopReason = "user-signal"
)

// convergenceThreshold is the number of Clear cells that triggers convergence.
const convergenceThreshold = 6

// ShouldStop reports whether the interview should end and why (R-019).
// Priority order: budget > convergence > user signal.
func ShouldStop(asked int, mode string, clear int, userSignal bool) (bool, StopReason) {
	if BudgetRemaining(asked, mode) == 0 {
		return true, StopBudget
	}
	if clear >= convergenceThreshold {
		return true, StopConverged
	}
	if userSignal {
		return true, StopUserSignal
	}
	return false, ""
}
