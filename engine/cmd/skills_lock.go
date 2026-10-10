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

// fileLocker is the skills.Locker of the production entry point.
type fileLocker struct {
	wait time.Duration
}

// newSkillsLocker returns the locker runSkillsCore hands to engine/skills: one that waits up to
// wait for a taken lock (zero is filelock.DefaultWait, 2 s, the bound the workflow binding store
// uses; deps.skillsLockWait).
func newSkillsLocker(wait time.Duration) skills.Locker { return fileLocker{wait: wait} }

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
