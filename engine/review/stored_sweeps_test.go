package review_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
)

// No record stored before a change of the sweep record becomes unreadable (own-rdd T12b, S15 f): every sweep the
// repository stores under odd/review-results decodes with DecodeSweep.
func TestEveryStoredSweepOfTheRepositoryStillDecodes(t *testing.T) {
	files, err := filepath.Glob("../../odd/review-results/*/*.sweep.json")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	for _, name := range files {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if _, err := review.DecodeSweep(data); err != nil {
			t.Errorf("%s: DecodeSweep() error = %v, want nil", name, err)
		}
	}
	t.Logf("read %d stored sweeps", len(files))
}
