package fsstore

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/ownership"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/ownership/app"
)

// seeded is a store whose record holds "claude\n".
func seeded(t *testing.T) *Store {
	t.Helper()
	store := New(filepath.Join(t.TempDir(), "state"))
	if err := os.MkdirAll(store.Layout.RecordDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.Layout.Record(), []byte("claude\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return store
}

// The rewrite of the record syncs the temporary file before the rename puts it in the place of the
// record, so a crash cannot leave the rename standing over bytes that never reached the disk. The sync
// cannot be seen from outside, so the test sees the moment it happens through the seam of the store.
func TestReplaceSyncsTheTemporaryFileBeforeTheRenameAndNotAfter(t *testing.T) {
	store := seeded(t)
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
	store := seeded(t)
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

// The directory of the record is synced after the rename, so that a crash cannot lose the rename and
// bring the old record back. Like the sync of the file it cannot be seen from outside, so the test sees
// the moment through the seam: it is the directory of the record, once, and the record is already the
// new one.
func TestReplaceSyncsTheDirectoryOfTheRecordAfterTheRename(t *testing.T) {
	store := seeded(t)
	var dirs []string
	store.syncDir = func(dir string) error {
		dirs = append(dirs, dir)
		if body, err := os.ReadFile(store.Layout.Record()); err != nil || string(body) != "opencode\n" {
			t.Errorf("at the sync of the directory the record is %q (%v), want the new one: the rename comes before", body, err)
		}
		return nil
	}
	if err := store.Replace([]byte("opencode\n")); err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 || dirs[0] != store.Layout.RecordDir() {
		t.Errorf("directories synced = %q, want the directory of the record once", dirs)
	}
}

// A directory that cannot be synced is a failure like the others of a rewrite, said at its own stage:
// the rename was made, so the record is the new one, no temporary file is left, and only the durability
// of the rename is in doubt.
func TestReplaceThatCannotSyncTheDirectoryIsAReplaceErrorAtItsOwnStageAndTheRecordIsTheNewOne(t *testing.T) {
	store := seeded(t)
	store.syncDir = func(string) error { return errors.New("disk on fire") }
	err := store.Replace([]byte("opencode\n"))
	var replace *app.ReplaceError
	if !errors.As(err, &replace) || replace.Stage != app.StageSync || replace.Err.Error() != "disk on fire" {
		t.Fatalf("Replace = %v, want a *ReplaceError at StageSync that carries the cause", err)
	}
	if body, rerr := os.ReadFile(store.Layout.Record()); rerr != nil || string(body) != "opencode\n" {
		t.Errorf("record = %q (%v), want the new one: the rename was made", body, rerr)
	}
	entries, rerr := os.ReadDir(store.Layout.RecordDir())
	if rerr != nil {
		t.Fatal(rerr)
	}
	if len(entries) != 1 {
		t.Errorf("the directory holds %d entries, want only the record", len(entries))
	}
}

func TestSyncDirectoryFlushesARealDirectoryAndFailsOnOneThatIsNotThere(t *testing.T) {
	dir := t.TempDir()
	if err := syncDirectory(dir); err != nil {
		t.Errorf("syncDirectory of a directory = %v, want nil", err)
	}
	if err := syncDirectory(filepath.Join(dir, "absent")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("syncDirectory of a directory that is not there = %v, want an absence", err)
	}
}

// A filesystem that answers that a directory cannot be synced has nothing more to give: that is not a
// failure. Any other answer is.
func TestIgnoreUnsupportedKeepsEveryErrorButTheOnesAFilesystemGivesForADirectory(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool // the error is kept
	}{
		{"nothing", nil, false},
		{"invalid argument", &fs.PathError{Op: "sync", Path: "/d", Err: syscall.EINVAL}, false},
		{"operation not supported", &fs.PathError{Op: "sync", Path: "/d", Err: syscall.ENOTSUP}, false},
		{"an input/output error", &fs.PathError{Op: "sync", Path: "/d", Err: syscall.EIO}, true},
		{"no space left", syscall.ENOSPC, true},
	} {
		if kept := ignoreUnsupported(tc.err) != nil; kept != tc.want {
			t.Errorf("%s: error kept = %v, want %v", tc.name, kept, tc.want)
		}
	}
}

// A record that is there and cannot be read is Unreadable, never Absent and never empty. A file whose
// mode is 0 does that for a user who cannot read anything he likes, and for no one else (root reads it),
// so the test makes the read fail through the seam, and holds the answer for every user.
func TestAReadThatFailsOnARecordThatIsThereIsUnreadable(t *testing.T) {
	store := seeded(t)
	store.readFile = func(path string) ([]byte, error) {
		if path != store.Layout.Record() {
			t.Errorf("read %s, want the record", path)
		}
		return nil, &fs.PathError{Op: "open", Path: path, Err: syscall.EACCES}
	}
	state, content := store.Read()
	if state != ownership.Unreadable || len(content) != 0 {
		t.Errorf("Read() = %v, %q, want Unreadable and nothing", state, content)
	}
	if !store.Exists() {
		t.Error("Exists() = false: a record that cannot be read is there")
	}
	// And a store that cannot read what is not there says that it is absent, without trying.
	store.readFile = func(string) ([]byte, error) {
		t.Error("the record that is not there was read")
		return nil, nil
	}
	if err := os.Remove(store.Layout.Record()); err != nil {
		t.Fatal(err)
	}
	if state, _ := store.Read(); state != ownership.Absent {
		t.Errorf("Read() of a record that is not there = %v, want Absent", state)
	}
}
