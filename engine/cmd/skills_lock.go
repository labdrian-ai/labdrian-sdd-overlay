package main

// The overlay lock's production locker: engine/skills defines what it needs (a
// Locker) and cannot implement it, because its import allowlist admits neither
// system calls nor time; this file builds one on engine/filelock. See
// engine/skills/lock.go for which verb takes which lock and why.

import (
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
	m := filelock.Exclusive
	if mode == skills.LockShared {
		m = filelock.Shared
	}
	return filelock.Acquire(path, filelock.Options{Mode: m, Wait: l.wait})
}
