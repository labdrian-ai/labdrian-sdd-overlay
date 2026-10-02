//go:build linux || darwin

package fsadapter

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/shaper"
)

// TestStoreRefusesAFIFORecord pins the regular-file check in the store's read path. A
// directory at the record path already fails at read time, so only a non-regular,
// non-directory record (a FIFO, opened non-blocking) proves that Get refuses anything but
// a regular file. It is refused in the store's words, and is not an absent clearance.
func TestStoreRefusesAFIFORecord(t *testing.T) {
	s, _ := newTestStore(t)
	key, data := storedRecord(t)
	path, err := s.Put(data)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove stored record: %v", err)
	}
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
	}
	got, err := s.Get(key[0], key[1], key[2])
	want := fmt.Sprintf("clearance store: record %q is not a regular file", path)
	if err == nil || err.Error() != want || errors.Is(err, shaper.ErrClearanceNotFound) {
		t.Fatalf("Get() of a FIFO at the record path = %q, %v, want a refusal %q", got, err, want)
	}
}
