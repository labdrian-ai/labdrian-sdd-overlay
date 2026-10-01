package projection

import (
	"errors"
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/filelock"
)

// lockFile takes an exclusive OS advisory flock on the file at path through
// engine/filelock, creating it with mode 0600 (never removed) and refusing a final
// symlink. It retries every 5 ms for up to lockWait, trying at least once, then
// fails with ErrBindingBusy. The returned function unlocks and closes the file.
//
// On a platform without flock it fails closed with ErrUnsupportedPlatform;
// NewStore's platformSupported check has refused the platform long before this is
// reached.
func lockFile(path string) (func(), error) {
	unlock, err := filelock.Acquire(path, filelock.Options{Perm: 0o600, Wait: lockWait})
	switch {
	case err == nil:
		return unlock, nil
	case errors.Is(err, filelock.ErrBusy):
		return nil, ErrBindingBusy
	case errors.Is(err, filelock.ErrUnsupported):
		return nil, ErrUnsupportedPlatform
	default:
		return nil, fmt.Errorf("projection store: acquire lock: %w", err)
	}
}
