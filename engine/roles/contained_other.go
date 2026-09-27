//go:build !linux && !darwin

package roles

import (
	"fmt"
	"os"
	"runtime"
)

// openNoFollow fails closed: this platform has no supported no-follow open,
// so no role-chain record read is attempted.
func openNoFollow(path string) (*os.File, error) {
	return nil, fmt.Errorf("contained read is unsupported on %s", runtime.GOOS)
}

// isSymlinkRefusal is never true here because openNoFollow never opens.
func isSymlinkRefusal(err error) bool { return false }
