//go:build linux || darwin

package workflow

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// acquireLock holds an OS advisory flock on lockPath: a live holder is never
// mistaken for a crashed one, and the kernel frees a dead holder's lock.
func acquireLock(lockPath string) (func(), error) {
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("workflow store: acquire lock: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, fmt.Errorf("%w: %s", ErrAppendConflict, lockPath)
		}
		return nil, fmt.Errorf("workflow store: acquire lock: %w", err)
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}
