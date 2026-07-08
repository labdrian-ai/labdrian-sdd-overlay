package assets_test

import (
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/assets"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator"
)

// TestAntiGenericDesignFrontmatterParses asserts the embedded
// anti-generic-design asset carries valid, parseable frontmatter scoping it to
// exactly the sdd-tasks and sdd-apply phases (R-104). A parse error here means
// gate-task/propagate would fail loud (propagate exits 1; gate-task logs a
// stderr warning and passes through) instead of injecting the contract.
func TestAntiGenericDesignFrontmatterParses(t *testing.T) {
	phases, err := propagator.ParseFrontmatter(assets.AntiGenericDesign)
	if err != nil {
		t.Fatalf("ParseFrontmatter(assets.AntiGenericDesign): unexpected error: %v", err)
	}

	want := []string{"sdd-tasks", "sdd-apply"}
	if len(phases.AppliesTo) != len(want) {
		t.Fatalf("AppliesTo = %v, want %v", phases.AppliesTo, want)
	}
	for i, phase := range want {
		if phases.AppliesTo[i] != phase {
			t.Errorf("AppliesTo[%d] = %q, want %q", i, phases.AppliesTo[i], phase)
		}
	}
}
