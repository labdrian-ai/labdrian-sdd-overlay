package prespec_test

import (
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/prespec"
)

func TestReadinessCompute(t *testing.T) {
	// helper: build a []Cell with the given counts of Clear, Partial, Missing.
	// Returns DefaultCells() modified to match the requested counts.
	makeCells := func(nClear, nPartial int) []prespec.Cell {
		cells := prespec.DefaultCells()
		idx := 0
		for i := 0; i < nClear && idx < len(cells); i++ {
			_ = cells[idx].SetState(prespec.Clear)
			idx++
		}
		for i := 0; i < nPartial && idx < len(cells); i++ {
			_ = cells[idx].SetState(prespec.Partial)
			idx++
		}
		return cells
	}

	cases := []struct {
		name       string
		nClear     int
		nPartial   int
		wantValue  float64
		wantPasses bool
	}{
		{"all Clear", 10, 0, 1.0, true},
		{"all Missing", 0, 0, 0.0, false},
		{"5 Clear 3 Partial 2 Missing → 0.65", 5, 3, 0.65, true},
		{"10 Partial → 0.5 fails gate", 0, 10, 0.5, false},
		{"6 Clear 4 Missing → 0.6 boundary passes", 6, 0, 0.6, true},
		{"5 Clear 1 Partial 4 Missing → 0.55 fails gate", 5, 1, 0.55, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cells := makeCells(tc.nClear, tc.nPartial)
			score := prespec.Readiness(cells)
			if score.Value != tc.wantValue {
				t.Errorf("Readiness().Value = %v, want %v", score.Value, tc.wantValue)
			}
			if score.Passes() != tc.wantPasses {
				t.Errorf("Readiness().Passes() = %v, want %v", score.Passes(), tc.wantPasses)
			}
		})
	}
}

func TestReadinessGate(t *testing.T) {
	cases := []struct {
		value      float64
		wantPasses bool
	}{
		{0.6, true},
		{0.59, false},
		{0.0, false},
		{1.0, true},
		{0.5, false},
	}
	for _, tc := range cases {
		s := prespec.Score{Value: tc.value}
		got := s.Passes()
		if got != tc.wantPasses {
			t.Errorf("Score{%v}.Passes() = %v, want %v", tc.value, got, tc.wantPasses)
		}
	}
}

func TestReadinessGateConstantExported(t *testing.T) {
	if prespec.ReadinessGate != 0.6 {
		t.Errorf("ReadinessGate = %v, want 0.6", prespec.ReadinessGate)
	}
}
