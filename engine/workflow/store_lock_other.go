//go:build !linux && !darwin

package workflow

import (
	"errors"
	"fmt"
	"os"
)

// acquireLock creates lockPath exclusively (O_EXCL), no reclaim: this
// platform lacks an advisory lock; a crashed holder's lock needs manual removal.
func acquireLock(lockPath string) (func(), error) {
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("%w: %s", ErrAppendConflict, lockPath)
		}
		return nil, fmt.Errorf("workflow store: acquire lock: %w", err)
	}
	return func() {
		f.Close()
		os.Remove(lockPath)
	}, nil
}
