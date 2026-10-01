//go:build linux || darwin

package statestore

import (
	"errors"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A FIFO opened for reading blocks until a writer appears. ReadFile must refuse
// it as not a regular file instead of hanging a hook on it.
func TestReadFileDoesNotBlockOnAFIFO(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "pipe")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("cannot make a FIFO here: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := ReadFile(fifo, 0)
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, ErrNotRegular) {
			t.Errorf("ReadFile(FIFO) = %v, want ErrNotRegular", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ReadFile blocked on a FIFO")
	}
}
