//go:build !linux && !darwin

package shaper

import (
	"fmt"
	"os"
	"runtime"
)

// openNoFollow fails closed: this platform has no supported no-follow open
// paired with a descriptor-path query, so no contained read is attempted.
func openNoFollow(path string) (*os.File, error) {
	return nil, fmt.Errorf("contained read is unsupported on %s", runtime.GOOS)
}

// isSymlinkRefusal is never true here because openNoFollow never opens.
func isSymlinkRefusal(err error) bool { return false }

// fdPath fails closed on unsupported platforms.
func fdPath(f *os.File) (string, error) {
	return "", fmt.Errorf("descriptor path is unsupported on %s", runtime.GOOS)
}
