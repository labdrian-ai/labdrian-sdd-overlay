package fsstore

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
)

// shortWaitStore is a store over a fresh state home that gives up on a taken lock
// after 20 ms instead of two seconds, so a test that holds the lock does not wait.
func shortWaitStore(t *testing.T) Store {
	t.Helper()
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s.WithLockWait(20 * time.Millisecond)
}

// These tests reach two defensive paths the public API cannot: Bind classifies
// the file before it writes, so a failing publish or an oversized read can only
// be produced by calling the helpers directly. They write only under
// t.TempDir().

// TestWriteBindingRemovesItsTemporaryFileWhenThePublishFails makes the final
// rename fail (a file cannot be renamed over a non-empty directory) and checks
// that the temporary file written just before it does not stay behind.
func TestWriteBindingRemovesItsTemporaryFileWhenThePublishFails(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.json")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "keep"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := writeBinding(target, []byte("{}\n"))
	if err == nil || !strings.Contains(err.Error(), "publish") {
		t.Fatalf("writeBinding() = %v, want a publish error", err)
	}
	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		t.Fatal(readErr)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if !reflect.DeepEqual(names, []string{"target.json"}) {
		t.Fatalf("directory holds %v after the failed publish, want only the original target", names)
	}
}

// TestWriteBindingRefusesASymlinkAndWritesNothingThroughIt: Bind classifies the path
// before it writes, so a symlink there is refused as unavailable. One put at the
// name after that check is refused by the publish itself: it is never replaced (a
// rename would silently break the link) and nothing is written through it.
func TestWriteBindingRefusesASymlinkAndWritesNothingThroughIt(t *testing.T) {
	dir := t.TempDir()
	elsewhere := filepath.Join(dir, "elsewhere.json")
	if err := os.WriteFile(elsewhere, []byte("theirs\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "target.json")
	if err := os.Symlink(elsewhere, target); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	err := writeBinding(target, []byte("{}\n"))
	if want := `projection store: refusing symlinked binding file "` + target + `"`; err == nil || err.Error() != want {
		t.Fatalf("writeBinding() = %v, want %q", err, want)
	}
	if got, _ := os.ReadFile(elsewhere); string(got) != "theirs\n" {
		t.Errorf("the file behind the link holds %q, want it untouched", got)
	}
	if info, err := os.Lstat(target); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the link was replaced: %v, %v", info, err)
	}
}

// TestTheLockWaitIsAFieldOfTheStore: no package-level setting is shared between
// stores, and a wait that is not positive is the default.
func TestTheLockWaitIsAFieldOfTheStore(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	quick := s.WithLockWait(time.Millisecond)
	if s.lockWait != DefaultLockWait || quick.lockWait != time.Millisecond {
		t.Errorf("lock waits = %v and %v, want the default and 1ms: WithLockWait must change a copy", s.lockWait, quick.lockWait)
	}
	if got := quick.WithLockWait(0).lockWait; got != DefaultLockWait {
		t.Errorf("WithLockWait(0) = %v, want the default %v", got, DefaultLockWait)
	}
	if DefaultLockWait != 2*time.Second {
		t.Errorf("DefaultLockWait = %v, want 2s as it always was", DefaultLockWait)
	}
}

// TestReadBindingFileStopsOneByteAfterTheCap pins that a large file is not read
// in full: one byte past projection.MaxBindingBytes is all classifyBinding needs to see
// that the file is too large.
func TestReadBindingFileStopsOneByteAfterTheCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.json")
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), 1<<20), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := readBindingFile(path)
	if err != nil {
		t.Fatalf("readBindingFile() = %v, want nil", err)
	}
	if len(data) != projection.MaxBindingBytes+1 {
		t.Fatalf("readBindingFile() read %d bytes of a 1 MiB file, want %d", len(data), projection.MaxBindingBytes+1)
	}
}

// TestBindAndUnbindReportBusyWhileTheRepositoryLockIsHeld holds the lock the way a
// concurrent Bind or Unbind would: neither may then act on the binding (an Unbind
// removing without it could destroy a fresh one), and both fail with projection.ErrBindingBusy.
func TestBindAndUnbindReportBusyWhileTheRepositoryLockIsHeld(t *testing.T) {
	s, key, at := shortWaitStore(t), strings.Repeat("a", 64), time.Unix(0, 0)
	if err := s.Bind(key, "proj-1", "wf-1", at, false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.stateHome, "labdrian", "bindings", key+".json")
	before, _ := os.ReadFile(path)
	unlock, err := s.lock(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Bind(key, "proj-1", "wf-2", at, true); !errors.Is(err, projection.ErrBindingBusy) {
		t.Errorf("Bind() = %v, want projection.ErrBindingBusy", err)
	}
	if removed, err := s.Unbind(key); removed || !errors.Is(err, projection.ErrBindingBusy) {
		t.Errorf("Unbind() = %v, %v, want false and projection.ErrBindingBusy", removed, err)
	}
	if after, _ := os.ReadFile(path); len(before) == 0 || !bytes.Equal(after, before) {
		t.Errorf("a busy Bind or Unbind changed the binding: %q, was %q", after, before)
	}

	unlock()
	if err := s.Bind(key, "proj-1", "wf-2", at, true); err != nil {
		t.Errorf("Bind() after the release = %v, want nil", err)
	}
	if removed, err := s.Unbind(key); !removed || err != nil {
		t.Errorf("Unbind() after the release = %v, %v, want true, nil", removed, err)
	}
}

// TestBindIfUnchangedReportsBusyWhileTheRepositoryLockIsHeld: a compare-and-swap
// that cannot get the lock has compared nothing, so it must neither write nor
// claim the binding changed. It fails with projection.ErrBindingBusy, and the same call
// succeeds once the lock is released.
func TestBindIfUnchangedReportsBusyWhileTheRepositoryLockIsHeld(t *testing.T) {
	s, key, at := shortWaitStore(t), strings.Repeat("a", 64), time.Unix(0, 0)
	if err := s.Bind(key, "proj-1", "wf-1", at, false); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.Load(key)
	if err != nil || loaded.Classification != projection.ClassificationOwned {
		t.Fatalf("Load() = %+v, %v, want an owned binding", loaded, err)
	}
	path := filepath.Join(s.stateHome, "labdrian", "bindings", key+".json")
	before, _ := os.ReadFile(path)
	unlock, err := s.lock(path)
	if err != nil {
		t.Fatal(err)
	}
	err = s.BindIfUnchanged(key, "proj-1", "wf-2", at, loaded.Binding)
	if !errors.Is(err, projection.ErrBindingBusy) || errors.Is(err, projection.ErrBindingChanged) {
		t.Errorf("BindIfUnchanged() = %v, want projection.ErrBindingBusy and not projection.ErrBindingChanged", err)
	}
	if after, _ := os.ReadFile(path); len(before) == 0 || !bytes.Equal(after, before) {
		t.Errorf("a busy BindIfUnchanged changed the binding: %q, was %q", after, before)
	}

	unlock()
	if err := s.BindIfUnchanged(key, "proj-1", "wf-2", at, loaded.Binding); err != nil {
		t.Errorf("BindIfUnchanged() after the release = %v, want nil", err)
	}
}

// TestUnbindIfUnchangedReportsBusyWhileTheRepositoryLockIsHeld: a removal that
// cannot get the lock has compared nothing, so it neither removes the binding
// nor claims it changed.
func TestUnbindIfUnchangedReportsBusyWhileTheRepositoryLockIsHeld(t *testing.T) {
	s, key, at := shortWaitStore(t), strings.Repeat("a", 64), time.Unix(0, 0)
	if err := s.Bind(key, "proj-1", "wf-1", at, false); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.Load(key)
	if err != nil || loaded.Classification != projection.ClassificationOwned {
		t.Fatalf("Load() = %+v, %v, want an owned binding", loaded, err)
	}
	path := filepath.Join(s.stateHome, "labdrian", "bindings", key+".json")
	before, _ := os.ReadFile(path)
	unlock, err := s.lock(path)
	if err != nil {
		t.Fatal(err)
	}
	removed, err := s.UnbindIfUnchanged(key, loaded.Binding)
	if removed || !errors.Is(err, projection.ErrBindingBusy) || errors.Is(err, projection.ErrBindingChanged) {
		t.Errorf("UnbindIfUnchanged() = %v, %v, want false and projection.ErrBindingBusy, not projection.ErrBindingChanged", removed, err)
	}
	if after, _ := os.ReadFile(path); len(before) == 0 || !bytes.Equal(after, before) {
		t.Errorf("a busy UnbindIfUnchanged changed the binding: %q, was %q", after, before)
	}

	unlock()
	if removed, err := s.UnbindIfUnchanged(key, loaded.Binding); !removed || err != nil {
		t.Errorf("UnbindIfUnchanged() after the release = %v, %v, want true, nil", removed, err)
	}
}

// TestAFileThatVanishedBeforeItWasOpenedIsAbsentNotUnavailable pins how Load
// classifies a failure to read a file it has just seen exist. Another process
// may remove the binding between Load's check and its open; the state is then
// "no binding", and reporting it as unavailable would make a hook that reads on
// every prompt warn about a binding that was simply unbound.
func TestAFileThatVanishedBeforeItWasOpenedIsAbsentNotUnavailable(t *testing.T) {
	gone := &os.PathError{Op: "open", Path: "binding.json", Err: os.ErrNotExist}
	if got := readFailure(gone); got.Classification != projection.ClassificationAbsent || got.Detail != "" {
		t.Errorf("readFailure(file not found) = %+v, want absent with no detail", got)
	}
	denied := &os.PathError{Op: "open", Path: "binding.json", Err: os.ErrPermission}
	if got := readFailure(denied); got.Classification != projection.ClassificationUnavailable || !strings.Contains(got.Detail, "permission denied") {
		t.Errorf("readFailure(permission denied) = %+v, want unavailable saying why", got)
	}
}
