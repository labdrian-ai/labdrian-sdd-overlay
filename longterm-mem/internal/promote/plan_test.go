package promote

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// vaultSnapshot records every file under root and its bytes, so a dry run
// that touches anything at all is visible -- not just one file a test
// thought to check.
func vaultSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	snap := map[string]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		snap[rel] = string(b)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot vault: %v", err)
	}
	return snap
}

func equalSnapshots(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// TestMergeFailures_DropsOnlyExactRepeats pins the claim mergeFailures'
// own doc comment makes. Deduping by observation ID alone passes the
// one-broken-page test just as well, and silently hides the second of two
// genuinely different failures for the same observation -- the same lie as
// the double count, pointing the other way. Nothing enforced that until
// this test: collapsing the key to the ID left the suite green.
func TestMergeFailures_DropsOnlyExactRepeats(t *testing.T) {
	sameCause := errors.New("check promoted state: unparseable")
	otherCause := errors.New("resolve status: edge unreadable")

	t.Run("an exact repeat is dropped", func(t *testing.T) {
		got := mergeFailures(
			[]SyncFailure{{ObservationID: 7, Err: sameCause}},
			[]SyncFailure{{ObservationID: 7, Err: sameCause}},
		)
		if len(got) != 1 {
			t.Fatalf("len = %d, want 1: %+v", len(got), got)
		}
	})

	t.Run("two different causes for one observation both survive", func(t *testing.T) {
		got := mergeFailures(
			[]SyncFailure{{ObservationID: 7, Err: sameCause}},
			[]SyncFailure{{ObservationID: 7, Err: otherCause}},
		)
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2: the patch pass's own distinct failure for observation 7 was dropped, hiding a real problem: %+v", len(got), got)
		}
	})

	t.Run("different observations always survive", func(t *testing.T) {
		got := mergeFailures(
			[]SyncFailure{{ObservationID: 7, Err: sameCause}},
			[]SyncFailure{{ObservationID: 8, Err: sameCause}},
		)
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2: %+v", len(got), got)
		}
	})
}
