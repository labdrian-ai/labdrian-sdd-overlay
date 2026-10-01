//go:build linux || darwin

package filelock

import (
	"errors"
	"os"
	"syscall"
)

// locker takes locks through its flock. The zero locker is the real one.
type locker struct {
	// flock is the system call; nil means syscall.Flock. It is a field and not a
	// package variable so that a test can make it fail the way a filesystem that
	// cannot lock a directory does, without any other test or caller seeing it.
	flock func(fd, how int) error
}

func (l locker) flockCall() func(fd, how int) error {
	if l.flock != nil {
		return l.flock
	}
	return syscall.Flock
}

// platformSupported reports whether this platform has flock and a no-follow
// open: only linux and darwin do.
const platformSupported = true

// openLockFile opens the lock file for mode without following a final-component
// symlink (the kernel's O_NOFOLLOW refusal comes back as ELOOP). An Exclusive
// lock creates the file when it is missing, with mode perm before the umask (0644
// unless the caller asked otherwise): flock needs no write access, so every user
// of a checkout shared by several can take the lock (a Shared one only reads the
// file), and none of them can write into it. A Shared lock opens it read-only and
// never creates it.
func openLockFile(path string, mode Mode, perm os.FileMode) (*os.File, error) {
	if mode == Shared {
		return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	}
	return os.OpenFile(path, os.O_CREATE|os.O_RDONLY|syscall.O_NOFOLLOW, perm)
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
func (l locker) tryLock(f *os.File, mode Mode) (bool, error) {
	how := syscall.LOCK_EX
	if mode == Shared {
		how = syscall.LOCK_SH
	}
	flock := l.flockCall()
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
func (l locker) releaseLock(f *os.File) error {
	return l.flockCall()(int(f.Fd()), syscall.LOCK_UN)
}
