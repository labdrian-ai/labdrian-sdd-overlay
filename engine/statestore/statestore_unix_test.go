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

// A FIFO at an append path would take the bytes and keep none of them. OpenAppendNoFollow must
// refuse it, without blocking, as not a regular file (C23 M7 review B, R3-append-nonblock-regular).
func TestOpenAppendNoFollowRefusesAFIFO(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "pipe")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("cannot make a FIFO here: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		f, err := OpenAppendNoFollow(fifo, 0o600)
		if f != nil {
			f.Close()
		}
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, ErrNotRegular) {
			t.Errorf("OpenAppendNoFollow(FIFO) = %v, want ErrNotRegular", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("OpenAppendNoFollow blocked on a FIFO")
	}
}
