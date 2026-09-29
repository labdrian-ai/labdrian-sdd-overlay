//go:build !linux && !darwin

package workflow

import "os"

// openNoFollow is unreachable: NewStore's checkPlatform already fails closed
// here. It exists only so this build target compiles.
func openNoFollow(path string) (*os.File, error) {
	return nil, ErrUnsupportedPlatform
}

// isSymlinkRefusal is never true here because openNoFollow never opens.
func isSymlinkRefusal(err error) bool { return false }
