// Package filelock is a bounded, advisory, cross-process file lock: the flock
// idiom engine/workflow (append) and engine/projection (bind) already use, in one
// place for the callers that cannot import system calls themselves.
//
// It exists as its own package because engine/skills' import allowlist
// (zero_fetch_test.go) admits neither "syscall" nor "time", on purpose: that
// package is the one held to "no process execution, no network", and a system
// call wrapper does not belong in it. The package is small enough to read in one
// sitting and imports only errors, fmt, os, syscall, and time, which a test pins.
//
// The contract, all of it tested:
//
//   - The lock is an OS advisory flock on a file the caller names. The kernel
//     frees it when the holder exits, however it exits, so a crashed process never
//     leaves a stale lock, and a live holder is never mistaken for a dead one: no
//     PID file, no age check, nothing to reclaim.
//   - The lock file is never removed, by this package or by a caller that follows
//     it. Removing it while another process holds or is about to take it would give
//     two processes two different locks (the unlink race). It is created empty
//     when an exclusive lock first needs it and stays.
//   - Acquire tries the lock at least once, then retries every pollInterval until
//     the bound (Options.Wait, DefaultWait by default), then fails with a
//     *BusyError. It never blocks without a bound.
//   - Exclusive and Shared modes follow flock: any number of Shared holders, or one
//     Exclusive holder. A Shared acquire never creates the lock file; see Mode.
//   - A final-component symlink is refused, so a lock file cannot be aimed at
//     another file.
//   - Only linux and darwin have flock. Elsewhere Acquire fails closed with
//     ErrUnsupported, the same choice engine/workflow and engine/projection make.
//
// The lock does not protect anything by itself: it is advisory, so it excludes
// only callers that take it, and it says nothing about what is read or written
// under it. A caller reads its state again after acquiring, never trusting a read
// made before.
package filelock

import (
	"errors"
	"fmt"
	"os"
	"time"
)

// DefaultWait is how long Acquire waits for a taken lock before it gives up with
// a *BusyError: the bound engine/projection's binding store uses.
const DefaultWait = 2 * time.Second

// pollInterval is how often a taken lock is tried again while waiting: the
// interval engine/projection's binding store uses.
const pollInterval = 5 * time.Millisecond

// Sentinel errors. Wrap-aware callers use errors.Is; a caller that must not import
// this package's errors can ask a returned error for its Busy() bool method.
var (
	// ErrBusy is what a *BusyError matches: the lock stayed taken for the whole
	// bound. Nothing was changed, and the call can be retried.
	ErrBusy = errors.New("filelock: the lock is held by another process")
	// ErrUnsupported is returned on a platform without advisory file locks.
	ErrUnsupported = errors.New("filelock: unsupported platform (advisory file locks need linux or darwin)")
)

// BusyError is returned by Acquire when the lock stayed taken past the bound.
type BusyError struct {
	// Path is the lock file that is taken.
	Path string
	// Waited is the bound that was spent waiting for it.
	Waited time.Duration
}

func (e *BusyError) Error() string {
	return fmt.Sprintf("filelock: the lock %s is held by another process (waited %v)", e.Path, e.Waited)
}

// Is makes errors.Is(err, ErrBusy) true for a *BusyError.
func (e *BusyError) Is(target error) bool { return target == ErrBusy }

// Busy reports true. It lets a caller tell a busy lock from any other failure
// with a plain interface assertion, without importing this package.
func (e *BusyError) Busy() bool { return true }

// Mode is how a lock is held.
type Mode int

const (
	// Exclusive is the writer's mode: one holder, no Shared holders. The lock
	// file is created (mode 0666 before the umask) if it does not exist.
	Exclusive Mode = iota
	// Shared is the reader's mode: any number of holders, none while an Exclusive
	// one holds. A Shared acquire opens the lock file read-only and never creates
	// it. A missing file means no Exclusive lock has ever been taken there, so no
	// writer can be in the middle of anything, and Acquire returns a lock that
	// holds nothing; that is also what lets a reader run on a tree it cannot write
	// to. A writer that creates the file at that very moment is the one case this
	// misses, and it can only be the first write ever made there.
	Shared
)

// Clock is the time source of the wait. The zero Clock is the real one; a test
// injects its own so that a two second bound costs no real time.
type Clock struct {
	// Now returns the current time. Nil means time.Now.
	Now func() time.Time
	// Sleep pauses for d. Nil means time.Sleep.
	Sleep func(d time.Duration)
}

func (c Clock) now() time.Time {
	if c.Now == nil {
		return time.Now()
	}
	return c.Now()
}

func (c Clock) sleep(d time.Duration) {
	if c.Sleep == nil {
		time.Sleep(d)
		return
	}
	c.Sleep(d)
}

// Options tunes one Acquire. The zero value is an Exclusive lock with the
// DefaultWait bound on the real clock.
type Options struct {
	// Mode is Exclusive (the zero value) or Shared.
	Mode Mode
	// Wait is the bound on waiting for a taken lock. Zero or negative means
	// DefaultWait. The lock is tried at least once whatever the bound.
	Wait time.Duration
	// Clock is the time source; the zero value is the real clock.
	Clock Clock
}

// Acquire takes the advisory lock on the file at path and returns the function
// that releases it. The returned function may be called more than once: a call
// after the first releases nothing. Nothing else needs to be cleaned up, and the
// lock file is left in place. An unlock that is never called is not a leak the
// process can outlive: the kernel frees the lock when the process exits.
//
// Failures: a lock that stayed taken past the bound is a *BusyError (errors.Is
// ErrBusy); a lock file that cannot be opened (a missing directory, a permission
// error, a symlink) is an ordinary error naming the path, because waiting would
// not help; a platform without flock is ErrUnsupported.
func Acquire(path string, opts Options) (unlock func(), err error) {
	if !platformSupported {
		return nil, ErrUnsupported
	}
	wait := opts.Wait
	if wait <= 0 {
		wait = DefaultWait
	}

	f, err := openLockFile(path, opts.Mode)
	if err != nil {
		if opts.Mode == Shared && errors.Is(err, os.ErrNotExist) {
			return func() {}, nil
		}
		return nil, fmt.Errorf("filelock: open %s: %w", path, err)
	}

	deadline := opts.Clock.now().Add(wait)
	for {
		held, err := tryLock(f, opts.Mode)
		if err != nil {
			f.Close()
			return nil, fmt.Errorf("filelock: lock %s: %w", path, err)
		}
		if held {
			return func() {
				// A closed *os.File reports descriptor -1, so a repeated call
				// fails harmlessly instead of touching a descriptor number that
				// something else has since been given.
				_ = releaseLock(f)
				f.Close()
			}, nil
		}
		if !opts.Clock.now().Before(deadline) {
			f.Close()
			return nil, &BusyError{Path: path, Waited: wait}
		}
		opts.Clock.sleep(pollInterval)
	}
}
