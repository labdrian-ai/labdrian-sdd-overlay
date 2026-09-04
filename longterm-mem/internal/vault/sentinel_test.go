package vault

import (
	"os"
	"path/filepath"
	"testing"
)

// TestWriteProvisionedSentinel_SurvivesAStaleTempPath: the sentinel's whole
// job is to survive a crash, so its own write has to. Writing through a
// FIXED "<sentinel>.tmp" name means any leftover at that exact path — a
// directory, a read-only file, a second process's in-flight temp — blocks
// provisioning permanently, and the vault can never be marked provisioned
// again without manual cleanup. A unique temp name per write has no such
// shared state.
//
// A directory is used as the obstruction because it is the one leftover no
// permission change can make writable: os.WriteFile onto it is EISDIR
// whatever the mode.
func TestWriteProvisionedSentinel_SurvivesAStaleTempPath(t *testing.T) {
	root := t.TempDir()
	sentinelPath := filepath.Join(root, provisionedSentinel)
	if err := os.MkdirAll(filepath.Dir(sentinelPath), 0o755); err != nil {
		t.Fatalf("mkdir sentinel parent: %v", err)
	}
	if err := os.Mkdir(sentinelPath+".tmp", 0o755); err != nil {
		t.Fatalf("seed stale temp path: %v", err)
	}

	if err := writeProvisionedSentinel(root); err != nil {
		t.Fatalf("writeProvisionedSentinel with a stale <sentinel>.tmp leftover: %v", err)
	}

	if _, err := os.Stat(sentinelPath); err != nil {
		t.Fatalf("sentinel was not written: %v", err)
	}
}

// TestWriteProvisionedSentinel_LeavesNoTempFileBehind: a temp file that
// outlives its write is litter inside the user's vault, and inside
// .vault-meta/ it sits next to files the retrieval scripts enumerate.
func TestWriteProvisionedSentinel_LeavesNoTempFileBehind(t *testing.T) {
	root := t.TempDir()
	if err := writeProvisionedSentinel(root); err != nil {
		t.Fatalf("writeProvisionedSentinel: %v", err)
	}

	metaDir := filepath.Dir(filepath.Join(root, provisionedSentinel))
	entries, err := os.ReadDir(metaDir)
	if err != nil {
		t.Fatalf("read %s: %v", metaDir, err)
	}
	for _, e := range entries {
		if e.Name() == filepath.Base(provisionedSentinel) {
			continue
		}
		t.Fatalf("writeProvisionedSentinel left %s behind in %s", e.Name(), metaDir)
	}
}
