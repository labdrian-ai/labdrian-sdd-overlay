// Package filelock is a bounded, advisory, cross-process file lock: the one flock
// idiom of the engine. engine/workflow (append), engine/projection (bind), the
// propagate registry lock of engine/cmd and the skills verbs all take their locks
// here, and so does any caller that cannot import system calls itself.
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
//     *BusyError. It never blocks without a bound. A caller whose contract is to
//     refuse a contended operation asks for Options.NoWait and gets the
//     *BusyError after the first attempt.
//   - Exclusive and Shared modes follow flock: any number of Shared holders, or one
//     Exclusive holder. A Shared acquire never creates the lock file; see Mode.
//   - A final-component symlink is refused, so a lock file cannot be aimed at
//     another file.
//   - AcquireDir locks a directory itself (open it read-only, flock the
//     descriptor) for a caller that may not put a file in the directory, such as a
//     project root in the user's repository. It creates nothing. A filesystem that
//     cannot lock a directory makes it fail with an error that says so, never
//     silently succeed and never fall back to a file.
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

// LockPath is the lock that stayed taken, as it was tried: for a directory lock,
// the path the caller passed to AcquireDir (a caller that resolves symlinks first
// passes the resolved one). It lets a caller that cannot import this package name,
// in its own message, the lock that was really contended.
func (e *BusyError) LockPath() string { return e.Path }

// Mode is how a lock is held.
type Mode int

const (
	// Exclusive is the writer's mode: one holder, no Shared holders. The lock
	// file is created (mode Options.Perm, 0644 by default, before the umask) if it
	// does not exist.
	Exclusive Mode = iota
	// Shared is the reader's mode: any number of holders, none while an Exclusive
	// one holds. A Shared acquire opens the lock file read-only and never creates
	// it. A missing file means no Exclusive lock has ever been taken there, so no
	// writer has been in the middle of anything, and Acquire returns a lock that
	// holds nothing; that is also what lets a reader run on a tree it cannot write
	// to.
	//
	// That lock cannot stop the first writer, who creates the file and writes while
	// the reader reads. A lock that holds nothing is therefore provisional: the
	// caller checks, after its read, that the lock file still does not exist (a
	// writer creates it before it writes anything, so a file that is still absent
	// proves that no write began during the read) and, when it does exist now,
	// reads again under a lock that is real. engine/skills does exactly that, for
	// its reads of the registry pair and of the skill tree.
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
// DefaultWait bound on the real clock, in a lock file created 0644.
type Options struct {
	// Mode is Exclusive (the zero value) or Shared.
	Mode Mode
	// Wait is the bound on waiting for a taken lock. Zero or negative means
	// DefaultWait. The lock is tried at least once whatever the bound.
	Wait time.Duration
	// NoWait tries the lock exactly once: a taken lock is a *BusyError at once,
	// with nothing waited for, whatever Wait says and without reading the clock.
	// It is for a caller whose contract is to refuse a contended operation, not to
	// queue behind it.
	NoWait bool
	// Perm is the mode, before the umask, of a lock file an Exclusive acquire
	// creates. Zero means 0644; a caller whose lock lives in a private directory
	// asks for 0600. A Shared acquire and AcquireDir create nothing and ignore it.
	// Any bit beyond the permission bits is an error.
	Perm os.FileMode
	// Clock is the time source; the zero value is the real clock.
	Clock Clock
}

// defaultPerm is the mode of a lock file when Options.Perm is zero: flock needs
// no write access, so every user of a shared checkout can take the lock and none
// can write into it.
const defaultPerm os.FileMode = 0o644

func (o Options) perm() (os.FileMode, error) {
	switch {
	case o.Perm == 0:
		return defaultPerm, nil
	case o.Perm&^os.ModePerm != 0:
		return 0, fmt.Errorf("filelock: Perm %v has bits beyond the permission bits", o.Perm)
	}
	return o.Perm, nil
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
	return locker{}.acquire(path, opts, false)
}

// AcquireDir takes the advisory lock on the directory at path itself: it opens
// the directory read-only and flocks that descriptor, so nothing is created in
// it, nothing needs git-ignoring, and a directory that cannot be written to locks
// like any other. It is what a caller wants when the directory belongs to someone
// else (a project root in the user's repository) and no file of ours may appear
// in it. Every other rule is Acquire's: the bound, the poll, *BusyError, a lock
// the kernel frees when the holder exits, ErrUnsupported off linux and darwin,
// and a final-component symlink refused (so a caller resolves the path first if
// it may be reached through one).
//
// Unlike a Shared file lock, a missing directory is an error in either mode, never
// a lock that holds nothing: there is no "nobody ever wrote here" reading of a
// project root that cannot be found. So is a path that is not a directory.
//
// Not every filesystem can lock a directory; some network and FUSE filesystems
// refuse. Then AcquireDir fails with an error that names the path, the kernel's
// reason, and says the filesystem may not support locking a directory and that
// nothing was locked. It never reports a lock it does not hold, never reports
// busy, and never falls back to a lock file in the directory.
func AcquireDir(path string, opts Options) (unlock func(), err error) {
	return locker{}.acquire(path, opts, true)
}

// acquire is Acquire and AcquireDir on one locker. The package-level functions use
// the zero locker, which locks through the operating system; a test uses a locker
// whose flock fails, and no other caller of the package sees it.
func (l locker) acquire(path string, opts Options, dir bool) (unlock func(), err error) {
	if !platformSupported {
		return nil, ErrUnsupported
	}
	perm, err := opts.perm()
	if err != nil {
		return nil, err
	}
	wait := opts.Wait
	if wait <= 0 {
		wait = DefaultWait
	}

	var f *os.File
	if dir {
		f, err = openDirLock(path)
	} else {
		f, err = openLockFile(path, opts.Mode, perm)
	}
	if err != nil {
		if !dir && opts.Mode == Shared && errors.Is(err, os.ErrNotExist) {
			return func() {}, nil
		}
		return nil, fmt.Errorf("filelock: open %s: %w", path, err)
	}

	var deadline time.Time
	if !opts.NoWait {
		deadline = opts.Clock.now().Add(wait)
	}
	for {
		held, err := l.tryLock(f, opts.Mode)
		if err != nil {
			f.Close()
			if dir {
				return nil, fmt.Errorf("filelock: lock %s: %w (this filesystem may not support locking a directory; nothing was locked)", path, err)
			}
			return nil, fmt.Errorf("filelock: lock %s: %w", path, err)
		}
		if held {
			return func() {
				// A closed *os.File reports descriptor -1, so a repeated call
				// fails harmlessly instead of touching a descriptor number that
				// something else has since been given.
				_ = l.releaseLock(f)
				f.Close()
			}, nil
		}
		if opts.NoWait {
			f.Close()
			return nil, &BusyError{Path: path}
		}
		if !opts.Clock.now().Before(deadline) {
			f.Close()
			return nil, &BusyError{Path: path, Waited: wait}
		}
		opts.Clock.sleep(pollInterval)
	}
}
