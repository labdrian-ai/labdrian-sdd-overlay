//go:build !linux && !darwin

package workflow

// acquireLock is unreachable: NewStore's checkPlatform already fails closed
// here, so no lock file is ever created to be stranded.
func acquireLock(lockPath string) (func(), error) {
	return nil, ErrUnsupportedPlatform
}
