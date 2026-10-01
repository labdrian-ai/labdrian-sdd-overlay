//go:build !linux && !darwin

package statestore

import "os"

// Supported is false here: the platform has no no-follow open, so a store built on
// this package must fail closed instead of reading a record it cannot vouch for.
const Supported = false

// OpenNoFollow is unavailable on this platform and always fails with
// ErrUnsupported. It exists so that this build target compiles.
func OpenNoFollow(path string) (*os.File, error) { return nil, ErrUnsupported }

// IsSymlinkRefusal is never true here, because OpenNoFollow never opens.
func IsSymlinkRefusal(err error) bool { return false }
