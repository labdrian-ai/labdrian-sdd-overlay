//go:build !linux && !darwin

package projection

import "os"

// platformSupported is false here: NewStore fails closed, so the store is
// never used on a platform without a no-follow open and a file lock.
const platformSupported = false

// openNoFollow is unreachable: NewStore already refused this platform. It
// exists only so this build target compiles.
func openNoFollow(path string) (*os.File, error) {
	return nil, ErrUnsupportedPlatform
}

// isSymlinkRefusal is never true here because openNoFollow never opens.
func isSymlinkRefusal(err error) bool { return false }
