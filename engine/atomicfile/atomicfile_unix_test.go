//go:build linux || darwin

package atomicfile

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// linux and darwin have the no-follow open a backup is read through, so a backup
// is possible there; the platforms without it are pinned in atomicfile_other_test.go.
func TestThisPlatformHasANoFollowOpen(t *testing.T) {
	if realOps().openNoFollow == nil {
		t.Fatal("realOps has no no-follow opener on a platform that has the open")
	}
}

// A FIFO swapped in for the target would block a plain open until a writer
// appears, hanging the caller. The backup must refuse it as not a regular file.
func TestBackUpDoesNotBlockOnAFIFO(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Skipf("cannot make a FIFO here: %v", err)
	}
	staged, err := Stage(dir, []byte("new"), Options{Perm: 0o600, Backup: true})
	if err != nil {
		t.Fatal(err)
	}
	defer staged.Discard()

	done := make(chan error, 1)
	go func() { done <- staged.backUp(path) }()
	select {
	case err := <-done:
		if err == nil {
			t.Error("backUp of a FIFO = nil, want an error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("backUp blocked on a FIFO")
	}
	if _, lerr := os.Lstat(path + ".bak"); lerr == nil {
		t.Error("a backup was written from a FIFO")
	}
}
