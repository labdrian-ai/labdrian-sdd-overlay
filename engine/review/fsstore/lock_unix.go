//go:build unix

package fsstore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
)

const (
	// defaultLockWait is how long Lock waits for another holder before it gives up: the held section lists one
	// directory and stores one file, so a holder that keeps the lock longer is stuck, and a caller that waits
	// for ever would be stuck with it.
	defaultLockWait = 30 * time.Second
	// lockPoll is how often Lock asks again while another holder has the lock.
	lockPoll = 50 * time.Millisecond
)

// take takes an exclusive flock(2) on the directory dir, which is at path, asking again every lockPoll while
// another holder has it, for at most the store's wait. The lock is given back by closing dir. The kernel ties
// the lock to what was opened, so every call, of this process or of another, waits for the one before; the
// end of the process that held it gives it back too. Only callers of Lock wait for each other: Create needs
// no lock, since it never writes over a file. A file system that cannot lock (ENOLCK, EOPNOTSUPP) is an error,
// so a verdict that blocks is not stored where the round cannot be guarded. Waiters ask again in no order, so
// they are not served first come first served; one that keeps losing stops at the wait with ErrLockHeld, and
// nothing is stored, which is safe for a section this short.
func (s Store) take(dir *os.File, path string) error {
	wait := s.lockWait
	if wait == 0 {
		wait = defaultLockWait
	}
	flock := s.flock
	if flock == nil {
		flock = syscall.Flock
	}
	deadline := time.Now().Add(wait)
	for {
		err := flock(int(dir.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		switch {
		case err == nil:
			return nil
		case errors.Is(err, syscall.EINTR):
			continue
		case !errors.Is(err, syscall.EWOULDBLOCK):
			return fmt.Errorf("lock %s: %w", path, err)
		case !time.Now().Before(deadline):
			return fmt.Errorf("%w: %s was not given back within %s", ErrLockHeld, path, wait)
		}
		time.Sleep(lockPoll)
	}
}

// lockByPath is Lock where the store works by path: it walks to the directory of the feature, looking at each
// directory without following it, opens the last without following a link, and requires what it opened to be
// the directory it looked at before it takes the lock.
func (s Store) lockByPath(root, feature string) (func(), error) {
	loc, found, err := s.walk(root, feature, false)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("%w: %s", app.ErrNotFound, filepath.Join(root, review.ResultsDir, feature))
	}
	s.at(pointWalked, loc.dir)
	fd, err := syscall.Open(loc.dir, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", loc.dir, err)
	}
	dir := os.NewFile(uintptr(fd), loc.dir)
	if opened, err := dir.Stat(); err != nil || !os.SameFile(loc.seen[len(loc.seen)-1].info, opened) {
		dir.Close()
		return nil, fmt.Errorf("%w: %s is not the directory that was looked at", ErrChanged, loc.dir)
	}
	if err := s.take(dir, loc.dir); err != nil {
		dir.Close()
		return nil, err
	}
	return func() { dir.Close() }, nil
}
