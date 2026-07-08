package assets_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/assets"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator"
)

// repoRoot returns the overlay repo root, two levels above engine/assets/.
func repoRoot(t *testing.T) string {
	t.Helper()
	// engine/assets/assets_test.go → engine/ → repo root
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Join(wd, "..", "..")
}

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

// TestAntiGenericDesignStandaloneCopyMatchesEmbeddedAsset is the Phase 5
// (drift guard) test for the two-copy design risk documented in design.md:
// the deployed standalone file skills/_shared/anti-generic-design.md MUST
// stay byte-identical to the compiled-in engine/assets/anti-generic-design.md
// asset. Any drift here means gate-task/propagate (embedded) and a user
// invoking the standalone skill by name would see DIFFERENT guidance — this
// guard fails loud instead of letting that happen silently.
func TestAntiGenericDesignStandaloneCopyMatchesEmbeddedAsset(t *testing.T) {
	standalonePath := filepath.Join(repoRoot(t), "skills", "_shared", "anti-generic-design.md")

	standaloneBytes, err := os.ReadFile(standalonePath)
	if err != nil {
		t.Fatalf("read standalone copy %s: %v", standalonePath, err)
	}

	if string(standaloneBytes) != assets.AntiGenericDesign {
		t.Errorf(
			"skills/_shared/anti-generic-design.md has drifted from the embedded engine/assets/anti-generic-design.md asset — they must be byte-identical.\nstandalone length=%d, embedded length=%d",
			len(standaloneBytes), len(assets.AntiGenericDesign),
		)
	}
}
