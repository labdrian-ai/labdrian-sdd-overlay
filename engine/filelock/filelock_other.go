//go:build !linux && !darwin

package filelock

import "os"

// platformSupported is false here: Acquire fails closed with ErrUnsupported, so
// no lock file is ever created to be stranded, and a caller never runs unlocked
// on a platform that cannot lock. It is the choice engine/workflow and
// engine/projection make for their own stores.
const platformSupported = false

// openLockFile is unreachable: Acquire already refused this platform. It exists
// only so this build target compiles.
func openLockFile(path string, mode Mode) (*os.File, error) { return nil, ErrUnsupported }

// openDirLock is unreachable for the same reason as openLockFile.
func openDirLock(path string) (*os.File, error) { return nil, ErrUnsupported }

// tryLock is unreachable for the same reason as openLockFile.
func tryLock(f *os.File, mode Mode) (bool, error) { return false, ErrUnsupported }

// releaseLock is unreachable for the same reason as openLockFile.
func releaseLock(f *os.File) error { return ErrUnsupported }
