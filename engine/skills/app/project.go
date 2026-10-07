package app

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// ProjectLockReadError is a project lock that could not be read, or that a verb needs and the
// project has none of. The verb refuses: a lock that exists and cannot be read must never be
// replaced by an empty one, which would disown everything recorded in it.
type ProjectLockReadError struct{ Err error }

func (e *ProjectLockReadError) Error() string {
	return fmt.Sprintf("reading project lock %q: %v", skills.ProjectLockRelPath, e.Err)
}
func (e *ProjectLockReadError) Unwrap() error { return e.Err }

// ExecutionError is a plan that could not be carried out: the files it staged and renamed were put
// back as far as they could be. Report is what the executor said while it did so, in its own words
// (the paths a rollback could not restore, one to a line), and is told before the error itself.
type ExecutionError struct {
	Err    error
	Report string
}

func (e *ExecutionError) Error() string { return e.Err.Error() }
func (e *ExecutionError) Unwrap() error { return e.Err }

// readOptionalProjectLock reads the lock of the project at root for a verb that creates it when
// the project has none: exists is false for a project with no lock yet, and any other failure is a
// *ProjectLockReadError.
func readOptionalProjectLock(locks skills.ProjectLockStore, root string) (data []byte, exists bool, err error) {
	data, err = locks.ReadLock(root)
	switch {
	case err == nil:
		return data, true, nil
	case errors.Is(err, fs.ErrNotExist):
		return nil, false, nil
	}
	return nil, false, &ProjectLockReadError{Err: err}
}
