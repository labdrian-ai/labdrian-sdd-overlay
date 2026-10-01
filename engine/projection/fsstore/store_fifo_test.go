//go:build linux || darwin

package fsstore_test

import (
	"errors"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
)

// TestAFIFOAtTheBindingPathIsUnavailableAndNeverBlocks pins the non-blocking,
// regular-file-only read. Opening a FIFO for reading blocks until a writer
// appears, so a store that read the path naively would hang the caller, which
// for a session hook would be the session itself. The store opens without
// blocking, sees that the file is not regular, and reports it unavailable.
func TestAFIFOAtTheBindingPathIsUnavailableAndNeverBlocks(t *testing.T) {
	s, root := isolatedStore(t)
	path := plant(t, root, hex64("a"), "placeholder")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
	}

	type outcome struct {
		loaded projection.Loaded
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		loaded, err := s.Load(hex64("a"))
		done <- outcome{loaded, err}
	}()
	select {
	case got := <-done:
		if got.err != nil || got.loaded.Classification != projection.ClassificationUnavailable {
			t.Fatalf("Load() = %+v, %v, want unavailable", got.loaded, got.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Load() blocked on a FIFO at the binding path")
	}

	if err := s.Bind(hex64("a"), "proj-1", "wf-1", t0, true); !errors.Is(err, projection.ErrBindingUnavailable) {
		t.Errorf("Bind() over a FIFO = %v, want ErrBindingUnavailable", err)
	}
}
