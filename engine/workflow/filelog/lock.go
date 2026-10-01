package filelog

import (
	"errors"
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/filelock"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// acquireLock serializes concurrent Append calls for one workflow, failing every
// other caller at once with workflow.ErrAppendConflict (the contract is to refuse
// a contended append, not to queue behind it); the returned release func must be
// called exactly once (via defer). It holds an OS advisory flock through
// engine/filelock: a live holder is never mistaken for a crashed one, and the
// kernel frees a dead holder's lock. The lock file is private (0600) like the rest
// of the store and is never removed.
//
// On a platform without flock it fails closed with ErrUnsupportedPlatform, so no
// lock file is ever created to be stranded; NewStore has refused the platform
// (statestore.RequirePlatform) long before this is reached.
func acquireLock(lockPath string) (func(), error) {
	unlock, err := filelock.Acquire(lockPath, filelock.Options{NoWait: true, Perm: 0o600})
	switch {
	case err == nil:
		return unlock, nil
	case errors.Is(err, filelock.ErrBusy):
		return nil, fmt.Errorf("%w: %s", workflow.ErrAppendConflict, lockPath)
	case errors.Is(err, filelock.ErrUnsupported):
		return nil, ErrUnsupportedPlatform
	default:
		return nil, fmt.Errorf("workflow store: acquire lock: %w", err)
	}
}
