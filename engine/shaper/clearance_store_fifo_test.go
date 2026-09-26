//go:build linux || darwin

package shaper

import (
	"os"
	"syscall"
	"testing"
)

// TestFileStoreRefusesFIFORecord pins the regular-file check in the store's
// read path. A directory at the record path already fails at read time, so
// only a non-regular, non-directory record (a FIFO, opened non-blocking)
// proves that Get refuses anything but a regular file.
func TestFileStoreRefusesFIFORecord(t *testing.T) {
	s, _ := isolatedStore(t)
	r, data := storedRecord(t)
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
	got, err := s.Get(r.Subject.ProjectID, r.Subject.GoalID, r.Subject.HandoffSHA256)
	if err == nil {
		t.Fatalf("Get accepted a FIFO at the record path, returned %q", got)
	}
}
