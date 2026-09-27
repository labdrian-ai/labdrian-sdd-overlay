//go:build linux || darwin

package roles

import (
	"errors"
	"os"
	"syscall"
)

// openNoFollow opens path read-only without following a final-component
// symlink and without blocking on a FIFO. It mirrors
// engine/shaper's identically named unexported helper; the two packages do
// not share this tiny primitive to keep the shaper clearance store isolated
// from the role-chain store.
func openNoFollow(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
}

// isSymlinkRefusal reports whether err is the kernel's O_NOFOLLOW refusal of
// a final-component symlink.
func isSymlinkRefusal(err error) bool {
	return errors.Is(err, syscall.ELOOP)
}
