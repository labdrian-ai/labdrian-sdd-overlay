//go:build linux

package evidencesh_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/evidencesh"
)

// (z) (T4z, `c9440b24b20f-6` F3): the adapter checks the mode of the script's path before it opens it, so a FIFO there is
// refused and never blocks the hook. An open of a FIFO for reading waits for a writer, so the call runs in a goroutine and
// the test waits 5 seconds for it. When it does not answer, the test frees the blocked open with an open for reading and
// writing (on Linux that open does not block and gives the blocked reader its end of file, fifo(7)), so even a build that
// opens first ends the package's tests, and then fails.
func TestClassified_RefusesAFIFOWithoutOpeningIt(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "evidence.sh")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("make a FIFO: %v", err)
	}
	c := &countingRunner{}
	answered := make(chan error, 1)
	go func() {
		_, err := evidencesh.New(c, evidencesh.Options{Script: path}).Classified("/r", []string{"f/0123456789ab-1"})
		answered <- err
	}()
	select {
	case err := <-answered:
		wantError(t, err, "the evidence script "+path+" is not a regular file")
		if c.calls != 0 {
			t.Errorf("the runner was called %d times, want none", c.calls)
		}
	case <-time.After(5 * time.Second):
		f, err := os.OpenFile(path, os.O_RDWR, 0)
		if err == nil {
			f.Close()
		}
		t.Fatalf("the call did not answer in 5 seconds: the script was opened before its mode was checked")
	}
}
