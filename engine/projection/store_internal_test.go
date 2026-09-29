package projection

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

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

// TestReadBindingFileStopsOneByteAfterTheCap pins that a large file is not read
// in full: one byte past MaxBindingBytes is all classifyBinding needs to see
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
	if len(data) != MaxBindingBytes+1 {
		t.Fatalf("readBindingFile() read %d bytes of a 1 MiB file, want %d", len(data), MaxBindingBytes+1)
	}
}

// TestBindAndUnbindReportBusyWhileTheRepositoryLockIsHeld holds the lock the way a
// concurrent Bind or Unbind would: neither may then act on the binding (an Unbind
// removing without it could destroy a fresh one), and both fail with ErrBindingBusy.
func TestBindAndUnbindReportBusyWhileTheRepositoryLockIsHeld(t *testing.T) {
	s, key, at := Store{stateHome: t.TempDir()}, strings.Repeat("a", 64), time.Unix(0, 0)
	if err := s.Bind(key, "proj-1", "wf-1", at, false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.stateHome, "labdrian", "bindings", key+".json")
	before, _ := os.ReadFile(path)
	wait := lockWait
	t.Cleanup(func() { lockWait = wait })
	lockWait = 20 * time.Millisecond

	unlock, err := s.lock(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Bind(key, "proj-1", "wf-2", at, true); !errors.Is(err, ErrBindingBusy) {
		t.Errorf("Bind() = %v, want ErrBindingBusy", err)
	}
	if removed, err := s.Unbind(key); removed || !errors.Is(err, ErrBindingBusy) {
		t.Errorf("Unbind() = %v, %v, want false and ErrBindingBusy", removed, err)
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
