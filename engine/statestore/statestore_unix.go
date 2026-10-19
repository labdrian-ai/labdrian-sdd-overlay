//go:build linux || darwin

package statestore

import (
	"errors"
	"os"
	"syscall"
)

// Supported reports whether the platform has the no-follow open the store needs:
// only linux and darwin do. A store built on this package refuses to start where
// it is false.
const Supported = true

// OpenNoFollow opens path read-only without following a final-component symlink
// and without blocking on a FIFO. A symlink is refused with the kernel's ELOOP
// (see IsSymlinkRefusal).
func OpenNoFollow(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
}

// OpenAppendNoFollow opens path for appending, creating it with perm when it does not exist,
// without following a final-component symlink and without blocking on a FIFO. A symlink is refused
// with the kernel's ELOOP (see IsSymlinkRefusal).
func OpenAppendNoFollow(path string, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, perm)
}

// IsSymlinkRefusal reports whether err is the kernel's refusal, under O_NOFOLLOW,
// of a final-component symlink.
func IsSymlinkRefusal(err error) bool {
	return errors.Is(err, syscall.ELOOP)
}
