//go:build linux || darwin

package atomicfile

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// noFollowOpener is the no-follow open of this platform.
func noFollowOpener() func(path string) (*os.File, error) { return openNoFollow }

// openNoFollow opens path read-only without following a final-component symlink
// and without blocking on a FIFO. A symlink at path is refused with ErrSymlink by
// the kernel (ELOOP under O_NOFOLLOW), so checking the name and opening it are one
// operation and no link can be swapped in between.
func openNoFollow(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if errors.Is(err, syscall.ELOOP) {
		return nil, fmt.Errorf("%w: %s", ErrSymlink, path)
	}
	return f, err
}
