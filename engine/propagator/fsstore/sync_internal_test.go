package fsstore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// The data reaches the disk before the rename makes it the registry: after a crash the rename can
// survive only if the bytes did, so no empty or truncated registry is left behind. The test sees
// the moment of the sync: the temporary file already holds the new content and the registry still
// holds the old one.
func TestWriteSyncsTheTemporaryFileBeforeTheRename(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "skill-registry.md")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	synced := 0
	store := Registry{syncFile: func(f *os.File) error {
		synced++
		if got, err := os.ReadFile(f.Name()); err != nil || string(got) != "new" {
			t.Errorf("at the sync the temporary file holds %q, %v, want the new content", got, err)
		}
		if got, err := os.ReadFile(path); err != nil || string(got) != "old" {
			t.Errorf("at the sync the registry holds %q, %v, want the old one (the rename comes after)", got, err)
		}
		return nil
	}}
	if err := store.Write(path, []byte("new")); err != nil {
		t.Fatal(err)
	}
	if synced != 1 {
		t.Errorf("%d syncs, want 1", synced)
	}
	if got, _ := os.ReadFile(path); string(got) != "new" {
		t.Errorf("registry = %q, want new", got)
	}
}

// A sync that fails is a write that failed: the registry keeps what it held and no temporary file
// is left.
func TestWriteThatCannotSyncLeavesTheRegistryAsItWas(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "skill-registry.md")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	disk := errors.New("input/output error")
	err := Registry{syncFile: func(*os.File) error { return disk }}.Write(path, []byte("new"))
	if !errors.Is(err, disk) {
		t.Fatalf("Write = %v, want the sync's error", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "old" {
		t.Errorf("registry = %q, want the old content", got)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("the directory holds %v, want only the registry", entries)
	}
}
