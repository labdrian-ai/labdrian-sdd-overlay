package prespec

// ReadinessGate is the minimum readiness score required for a brief to be
// generated. Exported so the skill and tests can reference it without magic
// numbers (R-010, OQ-4).
const ReadinessGate = 0.6

// Score is the result of a readiness computation.
type Score struct {
	Value      float64
	ClearCount int
	Total      int
}

// Passes returns true when Value meets or exceeds ReadinessGate.
func (s Score) Passes() bool {
	return s.Value >= ReadinessGate
}

// Readiness computes (Clear + 0.5*Partial) / Total for the given cells.
// Total is always 10 (the fixed grid size). Partial is weighted 0.5 per ADR-5,
// which numerically guarantees Partial ≠ Clear and that an all-Partial grid
// (value=0.5) cannot pass the 0.6 gate (R-009, R-010).
func Readiness(cells []Cell) Score {
	const total = 10
	var clear, partial int
	for _, c := range cells {
		switch c.State {
		case Clear:
			clear++
		case Partial:
			partial++
		}
	}
	value := (float64(clear) + 0.5*float64(partial)) / float64(total)
	return Score{Value: value, ClearCount: clear, Total: total}
}
