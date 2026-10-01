//go:build linux || darwin

package projection

import (
	"errors"
	"os"
	"syscall"
)

// platformSupported reports whether the store can run here: it needs a
// no-follow open and a file lock (engine/filelock). Only linux and darwin have
// both.
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
