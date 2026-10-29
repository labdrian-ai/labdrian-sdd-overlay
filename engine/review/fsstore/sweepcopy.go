package fsstore

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/filelock"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/statestore"
)

// copiesName is how the copies of the sweep name themselves in an error, and errCopiesUnsupported is what a platform
// without a no-follow open is refused with.
const copiesName = "review sweep copies"

var errCopiesUnsupported = errors.New(copiesName + ": unsupported platform")

// Copies is the adapter of the Copies port of review/app: the two copies a sweep runs its analyzers on and the lock that
// guards them (own-rdd T6b3, S6 b), kept at <state home>/labdrian/review-sweep/<repo key>/ with the directories base and
// head and the file lock. The repo key is the first 16 hex characters of the sha256 of the root of the main working
// tree, so the worktrees of one repository share one place. The paths are stable on purpose: a program that keys its cache
// by path (staticcheck) finds its cache again at the next run. The directories are made 0700 and refused when one is a
// link (statestore.EnsureDirs), outside every working tree. It is built over the state home and reads no variable of the
// environment.
type Copies struct{ home string }

// NewCopies is the store over the state home stateHome, which must be an absolute path, on a platform that has a
// no-follow open (linux and darwin).
func NewCopies(stateHome string) (Copies, error) {
	if err := statestore.RequireStore(copiesName, errCopiesUnsupported, stateHome); err != nil {
		return Copies{}, err
	}
	return Copies{home: stateHome}, nil
}

// directory is the state home followed by the components of the directory of the repository.
func (c Copies) directory(repo app.Repository) []string {
	sum := sha256.Sum256([]byte(repo.Main))
	return []string{c.home, "labdrian", "review-sweep", hex.EncodeToString(sum[:])[:16]}
}

// Lock takes the lock of the sweep of the repository without waiting: flock(2) on the file lock, which stays where it is
// after the release (filelock never removes it), and which the kernel gives back when the process ends, however it ends.
// A lock another sweep holds is app.ErrSweepBusy.
func (c Copies) Lock(repo app.Repository) (func(), error) {
	parts := c.directory(repo)
	if err := statestore.EnsureDirs(parts); err != nil {
		return nil, fmt.Errorf("%s: %w", copiesName, err)
	}
	unlock, err := filelock.Acquire(filepath.Join(append(parts, "lock")...), filelock.Options{NoWait: true, Perm: 0o600})
	switch {
	case errors.Is(err, filelock.ErrBusy):
		return nil, app.ErrSweepBusy
	case err != nil:
		return nil, fmt.Errorf("%s: %w", copiesName, err)
	}
	return unlock, nil
}

// Reset makes the copy called side, base or head, if it is not there, empties it, and returns its path. The directory
// itself stays, so its path and its inode are the same at every run; what was in it is removed.
func (c Copies) Reset(repo app.Repository, side string) (string, error) {
	if side != app.CopyBase && side != app.CopyHead {
		return "", fmt.Errorf("%s: %q is not %q or %q", copiesName, side, app.CopyBase, app.CopyHead)
	}
	parts := append(c.directory(repo), side)
	if err := statestore.EnsureDirs(parts); err != nil {
		return "", fmt.Errorf("%s: %w", copiesName, err)
	}
	dir := filepath.Join(parts...)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("%s: %w", copiesName, err)
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
			return "", fmt.Errorf("%s: empty %s: %w", copiesName, dir, err)
		}
	}
	return dir, nil
}
