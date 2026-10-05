package main

// The overlay lock's production locker: engine/skills defines what it needs (a
// Locker) and cannot implement it, because its import allowlist admits neither
// system calls nor time; this file builds one on engine/filelock. See
// engine/skills/lock.go for which verb takes which lock and why.

import (
	"os"
	"path/filepath"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/filelock"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// skillsLockWait is how long a skills verb waits for a taken lock before it gives
// up with exit 2. Zero means filelock.DefaultWait, 2 s, the bound the workflow
// binding store uses. It is a variable so that a test can shorten it.
var skillsLockWait time.Duration

// fileLocker is the skills.Locker of the production entry point.
type fileLocker struct {
	wait time.Duration
}

// newSkillsLocker returns the locker runSkillsCore hands to engine/skills.
func newSkillsLocker() skills.Locker { return fileLocker{wait: skillsLockWait} }

// Lock takes an advisory file lock. It returns filelock's errors as they are, so
// that a *filelock.BusyError still answers Busy() to engine/skills.
func (l fileLocker) Lock(path string, mode skills.LockMode) (func(), error) {
	return filelock.Acquire(path, l.options(mode))
}

// LockDir locks a directory itself, so that no file appears in it. The path is
// resolved through symlinks first: the lock is on the directory, whatever way the
// caller reached it (a working directory reached through a symlinked $PWD is the
// ordinary case), and filelock refuses a symlink as the final component. A path
// that cannot be resolved is an error, not a lock that is skipped.
func (l fileLocker) LockDir(dir string, mode skills.LockMode) (func(), error) {
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, err
	}
	return filelock.AcquireDir(real, l.options(mode))
}

// Exists says whether path is there: nil when it is, and otherwise the failure of the system
// with its own words (an error that is fs.ErrNotExist when the path is absent).
func (fileLocker) Exists(path string) error {
	_, err := os.Stat(path)
	return err
}

func (l fileLocker) options(mode skills.LockMode) filelock.Options {
	m := filelock.Exclusive
	if mode == skills.LockShared {
		m = filelock.Shared
	}
	return filelock.Options{Mode: m, Wait: l.wait}
}
