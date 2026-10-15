package fsstore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/ownership/app"
)

// The rewrite of the record syncs the temporary file before the rename puts it in the place of the
// record, so a crash cannot leave the rename standing over bytes that never reached the disk. The sync
// cannot be seen from outside, so the test sees the moment it happens through the seam of the store.
func TestReplaceSyncsTheTemporaryFileBeforeTheRenameAndNotAfter(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "state"))
	if err := os.MkdirAll(store.Layout.RecordDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.Layout.Record(), []byte("claude\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	calls := 0
	store.syncFile = func(f *os.File) error {
		calls++
		if body, err := os.ReadFile(store.Layout.Record()); err != nil || string(body) != "claude\n" {
			t.Errorf("at the sync the record is %q (%v), want the old one: the rename comes after", body, err)
		}
		if body, err := os.ReadFile(f.Name()); err != nil || string(body) != "opencode\n" {
			t.Errorf("at the sync the temporary file holds %q (%v), want the new record written", body, err)
		}
		return nil
	}
	if err := store.Replace([]byte("opencode\n")); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("the temporary file was synced %d times, want once", calls)
	}
}

func TestReplaceThatCannotSyncLeavesTheRecordAsItWasAndNoTemporaryFile(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "state"))
	if err := os.MkdirAll(store.Layout.RecordDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.Layout.Record(), []byte("claude\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store.syncFile = func(*os.File) error { return errors.New("disk on fire") }
	err := store.Replace([]byte("opencode\n"))
	var replace *app.ReplaceError
	if !errors.As(err, &replace) || replace.Stage != app.StageWrite {
		t.Fatalf("Replace = %v, want a *ReplaceError at StageWrite", err)
	}
	if body, rerr := os.ReadFile(store.Layout.Record()); rerr != nil || string(body) != "claude\n" {
		t.Errorf("record = %q (%v), want it as it was", body, rerr)
	}
	entries, rerr := os.ReadDir(store.Layout.RecordDir())
	if rerr != nil {
		t.Fatal(rerr)
	}
	if len(entries) != 1 {
		t.Errorf("the directory holds %d entries, want only the record: the temporary file is removed", len(entries))
	}
}
