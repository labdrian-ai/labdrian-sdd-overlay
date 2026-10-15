package fsstore_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/filelock"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/ownership"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/ownership/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/ownership/fsstore"
)

// The adapter answers the ports the use case owns.
var (
	_ app.Store    = (*fsstore.Store)(nil)
	_ app.Binaries = fsstore.Binaries{}
)

// quickLock is how a test waits for a lock another holder keeps: long enough to see it is held.
var quickLock = filelock.Options{Wait: 40 * time.Millisecond}

func newStore(t *testing.T) *fsstore.Store {
	t.Helper()
	store := fsstore.New(filepath.Join(t.TempDir(), "state"))
	store.LockOptions = quickLock
	return store
}

// seed writes the record with the content and the mode a person's editor leaves.
func seed(t *testing.T, store *fsstore.Store, content string) {
	t.Helper()
	if err := os.MkdirAll(store.Layout.RecordDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.Layout.Record(), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// onDisk is the record as the file system holds it.
func onDisk(t *testing.T, store *fsstore.Store) string {
	t.Helper()
	body, err := os.ReadFile(store.Layout.Record())
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// cannotReadMode0 says whether this user can read a file whose mode is 0, as root can.
func cannotReadMode0(t *testing.T) bool {
	t.Helper()
	probe := filepath.Join(t.TempDir(), "probe")
	if err := os.WriteFile(probe, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(probe, 0); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(probe)
	if err == nil {
		f.Close()
	}
	return err != nil
}

func TestLayoutKeepsTheRecordUnderTheLongtermMemDirectoryAndTheLockOutsideIt(t *testing.T) {
	layout := fsstore.Layout{StateDir: "/state"}
	if got := layout.Record(); got != "/state/longterm-mem/installed-targets" {
		t.Errorf("Record() = %q", got)
	}
	if got := layout.RecordDir(); got != "/state/longterm-mem" {
		t.Errorf("RecordDir() = %q", got)
	}
	// The lock is beside the directory of the record and not in it: a run that cannot write in that
	// directory must still be able to take the lock, and let it go, to say so.
	if got := layout.Lock(); got != "/state/.installed-targets.flock" {
		t.Errorf("Lock() = %q, want a file in the state directory, outside the directory of the record", got)
	}
	if got := (fsstore.Layout{StateDir: "/"}).Record(); got != "/longterm-mem/installed-targets" {
		t.Errorf("Record() of a state directory of / = %q, want the path cleaned", got)
	}
}

func TestExistsIsARegularFileAFollowedLinkIncluded(t *testing.T) {
	store := newStore(t)
	if store.Exists() {
		t.Error("Exists() with nothing there")
	}
	seed(t, store, "claude\n")
	if !store.Exists() {
		t.Error("Exists() with a regular file there is false")
	}
	if err := os.Remove(store.Layout.Record()); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(store.Layout.Record(), 0o755); err != nil {
		t.Fatal(err)
	}
	if store.Exists() {
		t.Error("Exists() with a directory in its place is true")
	}
	if err := os.Remove(store.Layout.Record()); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.WriteFile(target, []byte("claude\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, store.Layout.Record()); err != nil {
		t.Fatal(err)
	}
	if !store.Exists() {
		t.Error("Exists() with a link to a regular file is false")
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if store.Exists() {
		t.Error("Exists() with a link to nothing is true")
	}
}

func TestReadTellsAnAbsentRecordFromOneThatCannotBeRead(t *testing.T) {
	store := newStore(t)
	if state, content := store.Read(); state != ownership.Absent || content != nil {
		t.Errorf("Read() with nothing there = %v, %q", state, content)
	}
	seed(t, store, "claude\n\x00zed\nopenc")
	state, content := store.Read()
	if state != ownership.Readable || string(content) != "claude\n\x00zed\nopenc" {
		t.Errorf("Read() = %v, %q, want the bytes as they are: it is the rule's to judge them", state, content)
	}
	if err := os.Remove(store.Layout.Record()); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(store.Layout.Record(), 0o755); err != nil {
		t.Fatal(err)
	}
	if state, _ := store.Read(); state != ownership.Absent {
		t.Errorf("Read() with a directory in its place = %v, want Absent: it is not a record", state)
	}
}

func TestReadOfARecordNobodyCanReadIsUnreadableNeverEmpty(t *testing.T) {
	if !cannotReadMode0(t) {
		t.Skip("this user can read a file whose mode is 0")
	}
	store := newStore(t)
	seed(t, store, "claude\n")
	if err := os.Chmod(store.Layout.Record(), 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(store.Layout.Record(), 0o644)
	state, content := store.Read()
	if state != ownership.Unreadable || len(content) != 0 {
		t.Errorf("Read() = %v, %q, want Unreadable and nothing", state, content)
	}
}

func TestAppendAddsALineAfterWhateverTheRecordHeldAndTouchesNothingElse(t *testing.T) {
	store := newStore(t)
	if err := store.MakeRecordDir(); err != nil {
		t.Fatal(err)
	}
	if err := store.Append("claude"); err != nil {
		t.Fatal(err)
	}
	if got := onDisk(t, store); got != "claude\n" {
		t.Errorf("record = %q, want a record made with the line", got)
	}
	if err := os.WriteFile(store.Layout.Record(), []byte("\x00garbage\nopenc"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.Append("opencode"); err != nil {
		t.Fatal(err)
	}
	// The record is appended to and never rewritten: garbage stays, and a name cut short runs into
	// the name that follows it, as it always did.
	if got := onDisk(t, store); got != "\x00garbage\nopencopencode\n" {
		t.Errorf("record = %q, want the line added to the bytes as they were", got)
	}
}

func TestAppendMakesARecordWithTheModeTheUmaskLeaves(t *testing.T) {
	// The script's redirect made the file with the mode 0666 under the umask of the process, and so
	// does the store; the test sets a umask to see it.
	old := syscall.Umask(0o027)
	defer syscall.Umask(old)
	store := newStore(t)
	if err := store.MakeRecordDir(); err != nil {
		t.Fatal(err)
	}
	if err := store.Append("claude"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(store.Layout.Record())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Errorf("mode = %v, want 0640 (0666 under the umask 027)", info.Mode().Perm())
	}
}

func TestAppendFailsWithoutTheDirectoryOfTheRecord(t *testing.T) {
	store := newStore(t)
	if err := store.Append("claude"); err == nil {
		t.Error("Append with no directory of the record = nil, want the failure")
	}
}

func TestAppendsFromManyWritersAtOnceAreAllWholeLines(t *testing.T) {
	store := newStore(t)
	if err := store.MakeRecordDir(); err != nil {
		t.Fatal(err)
	}
	names := []string{"claude", "opencode", "codex", "pi", "a", "bb", "ccc", "dddd"}
	var wg sync.WaitGroup
	for _, name := range names {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			if err := store.Append(name); err != nil {
				t.Error(err)
			}
		}(name)
	}
	wg.Wait()
	got := ownership.Parse([]byte(onDisk(t, store))).Names()
	if len(got) != len(names) {
		t.Errorf("record holds %q, want all of %q: an append is one write", got, names)
	}
}

func TestReplacePutsTheNewRecordInThePlaceOfTheOldWithMode0600AndLeavesNoTemporaryFile(t *testing.T) {
	store := newStore(t)
	seed(t, store, "claude\nopencode\n")
	if err := store.Replace([]byte("opencode\n")); err != nil {
		t.Fatal(err)
	}
	if got := onDisk(t, store); got != "opencode\n" {
		t.Errorf("record = %q", got)
	}
	info, err := os.Stat(store.Layout.Record())
	if err != nil {
		t.Fatal(err)
	}
	// The rewrite goes through a temporary file the system makes at 0600, as the script's mktemp
	// made it, and the rename keeps that mode.
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
	entries, err := os.ReadDir(store.Layout.RecordDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("the directory of the record holds %d entries, want only the record", len(entries))
	}
	if err := store.Replace(nil); err != nil {
		t.Fatal(err)
	}
	if got := onDisk(t, store); got != "" {
		t.Errorf("record = %q, want an empty record that is there", got)
	}
}

func TestReplaceNeverShowsAReaderHalfARecord(t *testing.T) {
	store := newStore(t)
	old := strings.Repeat("claude\n", 20000)
	next := strings.Repeat("opencode\n", 20000)
	seed(t, store, old)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			body, err := os.ReadFile(store.Layout.Record())
			if err != nil || (string(body) != old && string(body) != next) {
				t.Errorf("a reader saw a record of %d bytes (err %v) that is neither the old nor the new", len(body), err)
				return
			}
		}
	}()
	for i := 0; i < 30; i++ {
		content := next
		if i%2 == 1 {
			content = old
		}
		if err := store.Replace([]byte(content)); err != nil {
			t.Error(err)
		}
	}
	close(stop)
	wg.Wait()
}

func TestReplaceNamesTheStageThatFailedAndLeavesTheRecordAsItWas(t *testing.T) {
	t.Run("no temporary file can be made", func(t *testing.T) {
		if !cannotReadMode0(t) {
			t.Skip("this user can write wherever it likes")
		}
		store := newStore(t)
		seed(t, store, "claude\n")
		if err := os.Chmod(store.Layout.RecordDir(), 0o555); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(store.Layout.RecordDir(), 0o755)
		err := store.Replace([]byte("opencode\n"))
		var replace *app.ReplaceError
		if !errors.As(err, &replace) || replace.Stage != app.StageCreate {
			t.Fatalf("Replace = %v, want a *ReplaceError at StageCreate", err)
		}
		if got := onDisk(t, store); got != "claude\n" {
			t.Errorf("record = %q, want it as it was", got)
		}
	})
	t.Run("the temporary file cannot take the place of the record", func(t *testing.T) {
		store := newStore(t)
		// A directory that is not empty where the record should be: a rename over it fails.
		if err := os.MkdirAll(filepath.Join(store.Layout.Record(), "in-the-way"), 0o755); err != nil {
			t.Fatal(err)
		}
		err := store.Replace([]byte("opencode\n"))
		var replace *app.ReplaceError
		if !errors.As(err, &replace) || replace.Stage != app.StagePublish {
			t.Fatalf("Replace = %v, want a *ReplaceError at StagePublish", err)
		}
		entries, rerr := os.ReadDir(store.Layout.RecordDir())
		if rerr != nil {
			t.Fatal(rerr)
		}
		if len(entries) != 1 {
			t.Errorf("the directory holds %d entries after a failed replace, want only what was there: the temporary file is removed", len(entries))
		}
	})
	t.Run("there is no directory", func(t *testing.T) {
		store := newStore(t)
		err := store.Replace([]byte("x\n"))
		var replace *app.ReplaceError
		if !errors.As(err, &replace) || replace.Stage != app.StageCreate {
			t.Fatalf("Replace = %v, want a *ReplaceError at StageCreate", err)
		}
	})
}

func TestDeleteRemovesTheRecordAndNotWhatIsNotOne(t *testing.T) {
	store := newStore(t)
	if err := store.Delete(); err != nil {
		t.Errorf("Delete() of a record that is not there = %v, want nil", err)
	}
	seed(t, store, "claude\n")
	if err := store.Delete(); err != nil || store.Exists() {
		t.Errorf("Delete() = %v, record there %v", err, store.Exists())
	}
	// A directory is no record: it is refused, as rm -f refuses it, and left where it is.
	if err := os.Mkdir(store.Layout.Record(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(); err == nil {
		t.Error("Delete() of a directory = nil, want the refusal")
	}
	if info, err := os.Stat(store.Layout.Record()); err != nil || !info.IsDir() {
		t.Errorf("the directory is gone after the refusal: %v", err)
	}
	// A link is removed, and not the file it points to.
	if err := os.Remove(store.Layout.Record()); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.WriteFile(target, []byte("claude\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, store.Layout.Record()); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("the file the link pointed to is gone: %v", err)
	}
}

func TestMakeRecordDirMakesTheDirectoryAndItsParents(t *testing.T) {
	store := newStore(t)
	if err := store.MakeRecordDir(); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(store.Layout.RecordDir()); err != nil || !info.IsDir() {
		t.Errorf("the directory of the record is not there: %v", err)
	}
	if err := store.MakeRecordDir(); err != nil {
		t.Errorf("a second MakeRecordDir = %v, want nil", err)
	}
	other := newStore(t)
	if err := os.MkdirAll(filepath.Dir(other.Layout.RecordDir()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other.Layout.RecordDir(), []byte("in the way\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := other.MakeRecordDir(); err == nil {
		t.Error("MakeRecordDir with a file where the directory should be = nil")
	}
}

func TestLockIsExclusiveBoundedAndLetGoOnRelease(t *testing.T) {
	store := newStore(t)
	release, err := store.Lock()
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(store.Layout.Lock()); err != nil || !info.Mode().IsRegular() {
		t.Errorf("the lock file is not a file in the state directory: %v", err)
	}
	other := fsstore.New(store.Layout.StateDir)
	other.LockOptions = quickLock
	start := time.Now()
	_, err = other.Lock()
	if !errors.Is(err, app.ErrBusy) {
		t.Fatalf("a second Lock while the first is held = %v, want app.ErrBusy", err)
	}
	if !strings.Contains(err.Error(), store.Layout.Lock()) {
		t.Errorf("the busy error %q does not name the lock file", err)
	}
	if waited := time.Since(start); waited < 30*time.Millisecond || waited > 5*time.Second {
		t.Errorf("gave up after %v, want about the 40ms bound", waited)
	}
	release()
	release() // twice is harmless
	again, err := other.Lock()
	if err != nil {
		t.Fatalf("Lock after the release = %v", err)
	}
	again()
}

func TestLockMakesTheStateDirectoryAndSaysWhenItCannot(t *testing.T) {
	store := newStore(t)
	release, err := store.Lock()
	if err != nil {
		t.Fatal(err)
	}
	release()
	if info, err := os.Stat(store.Layout.StateDir); err != nil || !info.IsDir() {
		t.Errorf("the state directory was not made: %v", err)
	}

	blocked := fsstore.New(filepath.Join(t.TempDir(), "state"))
	if err := os.WriteFile(blocked.Layout.StateDir, []byte("in the way\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = blocked.Lock()
	var stateDir *app.StateDirError
	if !errors.As(err, &stateDir) {
		t.Fatalf("Lock with a file where the state directory should be = %v, want a *StateDirError", err)
	}
}

func TestALockDirectoryTheOldScriptLeftBehindDoesNotBlockTheLock(t *testing.T) {
	// The script kept this lock as a directory called .installed-targets.lock; a run it killed left it
	// there. The engine's lock is another file, so that directory is garbage and not an obstacle.
	store := newStore(t)
	if err := os.MkdirAll(filepath.Join(store.Layout.StateDir, ".installed-targets.lock"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(store.Layout.StateDir, ".installed-targets.lock.break"), 0o755); err != nil {
		t.Fatal(err)
	}
	release, err := store.Lock()
	if err != nil {
		t.Fatalf("Lock with the old lock directory left behind = %v", err)
	}
	release()
}

func TestBinariesLookAtAFileAFollowedLinkIncludedAndRemoveOnlyFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "longterm-mem")
	b := fsstore.Binaries{}
	if b.Exists(path) {
		t.Error("Exists() of nothing")
	}
	if err := b.Remove(path); err != nil {
		t.Errorf("Remove() of nothing = %v, want nil: it is gone, which is what was asked", err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !b.Exists(path) {
		t.Error("Exists() of a file is false")
	}
	if err := b.Remove(path); err != nil || b.Exists(path) {
		t.Errorf("Remove() = %v, still there %v", err, b.Exists(path))
	}

	dangling := filepath.Join(dir, "dangling")
	if err := os.Symlink(filepath.Join(dir, "nowhere"), dangling); err != nil {
		t.Fatal(err)
	}
	if b.Exists(dangling) {
		t.Error("Exists() of a link to nothing is true")
	}

	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := b.Remove(filepath.Join(dir, "d")); err == nil {
		t.Error("Remove() of a directory = nil, want the refusal")
	}
	if _, err := os.Stat(filepath.Join(dir, "d")); err != nil {
		t.Errorf("the directory is gone after the refusal: %v", err)
	}
}

// slowStore widens the read-modify-write of every writer by a fixed time spent inside the critical
// section, because every writer reads there: a lost update is a timing bug, and a test of one that
// relies on luck passes on a fast machine while the bug is still there.
type slowStore struct {
	*fsstore.Store
	delay time.Duration
}

func (s slowStore) Read() (ownership.State, []byte) {
	state, content := s.Store.Read()
	time.Sleep(s.delay)
	return state, content
}

func knows(name string) bool {
	switch name {
	case "claude", "opencode", "codex", "pi":
		return true
	}
	return false
}

// raceService is the use case over the real store, with the reads slowed down. Its lock waits longer
// than the delay of every writer in the queue.
func raceService(t *testing.T) (*app.Service, *fsstore.Store) {
	t.Helper()
	store := newStore(t)
	store.LockOptions = filelock.Options{Wait: 10 * time.Second}
	return app.New(slowStore{Store: store, delay: 60 * time.Millisecond}, fsstore.Binaries{}, knows), store
}

func TestParallelRecordsKeepEveryTarget(t *testing.T) {
	svc, store := raceService(t)
	var wg sync.WaitGroup
	for _, name := range []string{"claude", "opencode", "codex", "pi"} {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			if err := svc.Record(name); err != nil {
				t.Error(err)
			}
		}(name)
	}
	wg.Wait()
	if got := ownership.Parse([]byte(onDisk(t, store))).Names(); len(got) != 4 {
		t.Errorf("record holds %q, want all four", got)
	}
}

func TestParallelReleasesLoseNoRemovalAndDropNoOtherTarget(t *testing.T) {
	svc, store := raceService(t)
	seed(t, store, "claude\nopencode\ncodex\n")
	var wg sync.WaitGroup
	for _, name := range []string{"claude", "opencode"} {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			if err := svc.Release(name); err != nil {
				t.Error(err)
			}
		}(name)
	}
	wg.Wait()
	if got := onDisk(t, store); got != "codex\n" {
		t.Errorf("record = %q, want only codex: neither removal is lost and the untouched target stays", got)
	}
}

func TestARecordRacingAReleaseLosesNeither(t *testing.T) {
	svc, store := raceService(t)
	seed(t, store, "claude\ncodex\n")
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if err := svc.Record("opencode"); err != nil {
			t.Error(err)
		}
	}()
	go func() {
		defer wg.Done()
		time.Sleep(20 * time.Millisecond)
		if err := svc.Release("claude"); err != nil {
			t.Error(err)
		}
	}()
	wg.Wait()
	if got := ownership.Parse([]byte(onDisk(t, store))); !got.Contains("opencode") || !got.Contains("codex") || got.Contains("claude") {
		t.Errorf("record holds %q, want opencode and codex and not claude", got.Names())
	}
}

func TestRetireRacingARecordNeverRemovesTheBinaryTheRecordIsRegisteringAgainst(t *testing.T) {
	svc, store := raceService(t)
	seed(t, store, "")
	binary := filepath.Join(t.TempDir(), "longterm-mem")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if err := svc.Record("opencode"); err != nil {
			t.Error(err)
		}
	}()
	go func() {
		defer wg.Done()
		time.Sleep(20 * time.Millisecond)
		if _, err := svc.Retire(app.RetireRequest{Binary: binary}); err != nil {
			t.Error(err)
		}
	}()
	wg.Wait()
	if _, err := os.Stat(binary); err != nil {
		t.Errorf("the shared binary was removed while a record of a target registered against it was being made: %v", err)
	}
	if got := onDisk(t, store); got != "opencode\n" {
		t.Errorf("record = %q, want the record the add made: it was not erased with the binary", got)
	}
}

// failingReplace is the store whose rewrite fails after the lock is taken and the record is read, as a
// full disk would make it.
type failingReplace struct{ *fsstore.Store }

func (failingReplace) Replace([]byte) error {
	return &app.ReplaceError{Stage: app.StageCreate, Err: errors.New("disk full")}
}

func TestAFailedCriticalSectionLetsTheLockGo(t *testing.T) {
	store := newStore(t)
	seed(t, store, "claude\nopencode\n")
	svc := app.New(failingReplace{Store: store}, fsstore.Binaries{}, knows)
	if err := svc.Release("claude"); err == nil {
		t.Fatal("Release = nil, want the failure of the rewrite")
	}
	release, err := store.Lock()
	if err != nil {
		t.Fatalf("the lock is still held after the failure: %v", err)
	}
	release()
	if got := onDisk(t, store); got != "claude\nopencode\n" {
		t.Errorf("record = %q, want it as it was", got)
	}
	if err := app.New(store, fsstore.Binaries{}, knows).Record("codex"); err != nil {
		t.Errorf("the next writer cannot record a target: %v", err)
	}
}
