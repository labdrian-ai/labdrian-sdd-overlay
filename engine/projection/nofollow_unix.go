//go:build linux || darwin

package projection

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

// platformSupported reports whether the store can run here: it needs a
// no-follow open and a file lock (both below). Only linux and darwin have both.
const platformSupported = true

// openNoFollow opens path read-only without following a final-component
// symlink and without blocking on a FIFO. It mirrors the identically named
// helper in engine/workflow; each store package keeps its own copy of this
// tiny primitive rather than sharing it, so the stores stay independent of one
// another.
func openNoFollow(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
}

// isSymlinkRefusal reports whether err is the kernel's O_NOFOLLOW refusal of a
// final-component symlink.
func isSymlinkRefusal(err error) bool {
	return errors.Is(err, syscall.ELOOP)
}

// lockFile takes an exclusive OS advisory flock on the file at path, creating
// it with mode 0600 (never removed) and refusing a final symlink. It retries the
// non-blocking flock every 5 ms for up to lockWait, trying at least once, then
// fails with ErrBindingBusy. The returned function unlocks and closes the file.
func lockFile(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("projection store: acquire lock: %w", err)
	}
	deadline := time.Now().Add(lockWait)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		switch {
		case err == nil:
			return func() {
				_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
				f.Close()
			}, nil
		case !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN):
			f.Close()
			return nil, fmt.Errorf("projection store: acquire lock: %w", err)
		case !time.Now().Before(deadline):
			f.Close()
			return nil, ErrBindingBusy
		}
		time.Sleep(5 * time.Millisecond)
	}
}
