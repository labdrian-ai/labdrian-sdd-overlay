package prespec_test

import (
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/prespec"
)

// ---- T-01: cell types and state machine ------------------------------------

func TestCellStateTransitions(t *testing.T) {
	cases := []struct {
		name    string
		from    prespec.CellState
		to      prespec.CellState
		wantErr bool
	}{
		{"Missing→Partial", prespec.Missing, prespec.Partial, false},
		{"Missing→Clear", prespec.Missing, prespec.Clear, false},
		{"Partial→Clear", prespec.Partial, prespec.Clear, false},
		{"Clear→Partial rejected", prespec.Clear, prespec.Partial, true},
		{"Clear→Missing rejected", prespec.Clear, prespec.Missing, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := prespec.Cell{State: tc.from}
			err := c.SetState(tc.to)
			if (err != nil) != tc.wantErr {
				t.Fatalf("SetState(%v→%v) error=%v, wantErr=%v", tc.from, tc.to, err, tc.wantErr)
			}
			if tc.wantErr && c.State != tc.from {
				t.Errorf("rejected transition must leave state unchanged; got %v want %v", c.State, tc.from)
			}
			if !tc.wantErr && c.State != tc.to {
				t.Errorf("accepted transition must update state; got %v want %v", c.State, tc.to)
			}
		})
	}
}

func TestCellDefinitions(t *testing.T) {
	cells := prespec.DefaultCells()

	if len(cells) != 10 {
		t.Fatalf("DefaultCells() length = %d, want 10", len(cells))
	}

	// Expected taxonomy in index order (R-005a).
	want := []struct {
		Key         string
		Impact      int
		Uncertainty int
	}{
		{"jtbd-job", 5, 5},
		{"current-gap", 5, 4},
		{"why-now", 4, 5},
		{"user-segment", 4, 3},
		{"constraints", 3, 4},
		{"success-metric", 3, 3},
		{"alternatives", 3, 3},
		{"stakeholders", 2, 3},
		{"frequency", 2, 2},
		{"risk-unknowns", 2, 4},
	}

	for i, w := range want {
		c := cells[i]
		if c.Key != w.Key {
			t.Errorf("cells[%d].Key = %q, want %q", i, c.Key, w.Key)
		}
		if c.Impact != w.Impact {
			t.Errorf("cells[%d].Impact = %d, want %d", i, c.Impact, w.Impact)
		}
		if c.Uncertainty != w.Uncertainty {
			t.Errorf("cells[%d].Uncertainty = %d, want %d", i, c.Uncertainty, w.Uncertainty)
		}
		if c.State != prespec.Missing {
			t.Errorf("cells[%d].State = %v, want Missing", i, c.State)
		}
	}
}

// ---- T-02: Grid functions --------------------------------------------------

func TestGridNew(t *testing.T) {
	g := prespec.New()
	if len(g.Cells) != 10 {
		t.Fatalf("New() grid cells = %d, want 10", len(g.Cells))
	}
	for i, c := range g.Cells {
		if c.State != prespec.Missing {
			t.Errorf("New() cells[%d].State = %v, want Missing", i, c.State)
		}
	}
}

func TestRankUncovered(t *testing.T) {
	t.Run("all-missing: sorted by Impact×Uncertainty desc then index asc", func(t *testing.T) {
		g := prespec.New()
		ranked := g.RankUncovered()
		if len(ranked) != 10 {
			t.Fatalf("all-missing: want 10 ranked cells, got %d", len(ranked))
		}
		// jtbd-job = 5×5=25 must be first
		if ranked[0].Key != "jtbd-job" {
			t.Errorf("ranked[0] = %q, want jtbd-job", ranked[0].Key)
		}
		// current-gap (20, idx=1) before why-now (20, idx=2)
		if ranked[1].Key != "current-gap" {
			t.Errorf("ranked[1] = %q, want current-gap", ranked[1].Key)
		}
		if ranked[2].Key != "why-now" {
			t.Errorf("ranked[2] = %q, want why-now", ranked[2].Key)
		}
	})

	t.Run("clear cells excluded", func(t *testing.T) {
		g := prespec.New()
		_ = g.Cells[0].SetState(prespec.Clear) // jtbd-job → Clear
		ranked := g.RankUncovered()
		for _, c := range ranked {
			if c.Key == "jtbd-job" {
				t.Error("Clear cell jtbd-job must be excluded from RankUncovered")
			}
		}
		if len(ranked) != 9 {
			t.Errorf("want 9 uncovered cells, got %d", len(ranked))
		}
	})

	t.Run("fully-cleared grid returns empty slice", func(t *testing.T) {
		g := prespec.New()
		for i := range g.Cells {
			_ = g.Cells[i].SetState(prespec.Clear)
		}
		ranked := g.RankUncovered()
		if len(ranked) != 0 {
			t.Errorf("fully cleared: want empty slice, got %d cells", len(ranked))
		}
	})
}

func TestCoverageCount(t *testing.T) {
	t.Run("fresh grid: 0 clear, 0 partial, 10 empty", func(t *testing.T) {
		g := prespec.New()
		clear, partial, empty := prespec.CoverageCount(g)
		if clear != 0 || partial != 0 || empty != 10 {
			t.Errorf("fresh grid: clear=%d partial=%d empty=%d, want 0,0,10", clear, partial, empty)
		}
	})

	t.Run("mixed 3 clear, 2 partial, 5 empty", func(t *testing.T) {
		g := prespec.New()
		for i := 0; i < 3; i++ {
			_ = g.Cells[i].SetState(prespec.Clear)
		}
		for i := 3; i < 5; i++ {
			_ = g.Cells[i].SetState(prespec.Partial)
		}
		clear, partial, empty := prespec.CoverageCount(g)
		if clear != 3 || partial != 2 || empty != 5 {
			t.Errorf("mixed: clear=%d partial=%d empty=%d, want 3,2,5", clear, partial, empty)
		}
	})
}

func TestBudgetRemaining(t *testing.T) {
	cases := []struct {
		name    string
		asked   int
		mode    string
		wantRem int
	}{
		{"standard asked=0", 0, "standard", 5},
		{"standard asked=5", 5, "standard", 0},
		{"asked over budget clamps to zero", 6, "standard", 0},
		{"unknown mode safe default", 0, "unknown", 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rem := prespec.BudgetRemaining(tc.asked, tc.mode)
			if rem != tc.wantRem {
				t.Errorf("BudgetRemaining(%d, %q) = %d, want %d", tc.asked, tc.mode, rem, tc.wantRem)
			}
		})
	}
}

func TestShouldStop(t *testing.T) {
	cases := []struct {
		name       string
		asked      int
		mode       string
		clear      int
		userSignal bool
		wantStop   bool
		wantReason prespec.StopReason
	}{
		// Budget fires first (asked=5, clear=3, signal=false) → budget-exhausted
		{"budget fires first", 5, "standard", 3, false, true, prespec.StopBudget},
		// Convergence fires (asked=3, clear=6, signal=false) → coverage-threshold
		{"convergence fires", 3, "standard", 6, false, true, prespec.StopConverged},
		// User signal (asked=2, clear=4, signal=true) → user-signal
		{"user signal fires", 2, "standard", 4, true, true, prespec.StopUserSignal},
		// No condition (asked=2, clear=4, signal=false) → false, ""
		{"no condition", 2, "standard", 4, false, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stop, reason := prespec.ShouldStop(tc.asked, tc.mode, tc.clear, tc.userSignal)
			if stop != tc.wantStop {
				t.Errorf("ShouldStop stop=%v, want %v", stop, tc.wantStop)
			}
			if reason != tc.wantReason {
				t.Errorf("ShouldStop reason=%q, want %q", reason, tc.wantReason)
			}
		})
	}
}
