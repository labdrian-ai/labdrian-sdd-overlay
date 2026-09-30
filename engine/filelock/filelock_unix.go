//go:build linux || darwin

package filelock

import (
	"errors"
	"os"
	"syscall"
)

// flock is the system call; a variable only so that a test can make it fail the way
// a filesystem that cannot lock a directory does.
var flock = syscall.Flock

// platformSupported reports whether this platform has flock and a no-follow
// open: only linux and darwin do.
const platformSupported = true

// openLockFile opens the lock file for mode without following a final-component
// symlink (the kernel's O_NOFOLLOW refusal comes back as ELOOP). An Exclusive
// lock creates the file when it is missing, with mode 0666 before the umask, so
// that a checkout shared by several users can be locked by all of them; a Shared
// lock opens it read-only and never creates it. flock needs no write access, so
// a read-only file locks like any other.
func openLockFile(path string, mode Mode) (*os.File, error) {
	if mode == Shared {
		return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	}
	return os.OpenFile(path, os.O_CREATE|os.O_RDONLY|syscall.O_NOFOLLOW, 0o666)
}

// openDirLock opens a directory read-only to lock it. It creates nothing, never
// follows a final-component symlink, and fails for anything that is not a
// directory (O_DIRECTORY).
func openDirLock(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
}

// tryLock makes one non-blocking attempt. It reports false with a nil error when
// the lock is taken by someone else, and an error only for a failure that waiting
// would not cure.
func tryLock(f *os.File, mode Mode) (bool, error) {
	how := syscall.LOCK_EX
	if mode == Shared {
		how = syscall.LOCK_SH
	}
	for {
		err := flock(int(f.Fd()), how|syscall.LOCK_NB)
		switch {
		case err == nil:
			return true, nil
		case errors.Is(err, syscall.EINTR):
			continue
		case errors.Is(err, syscall.EWOULDBLOCK), errors.Is(err, syscall.EAGAIN):
			return false, nil
		default:
			return false, err
		}
	}
}

// releaseLock drops the lock. Closing the file would do it too; unlocking first
// makes it explicit and lets the caller close afterwards.
func releaseLock(f *os.File) error {
	return flock(int(f.Fd()), syscall.LOCK_UN)
}
