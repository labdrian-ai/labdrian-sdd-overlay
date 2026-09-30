package skills

import (
	"io"
	"path/filepath"
	"strings"
)

// ExecuteInstallPlan carries out a plan from PlanInstallOwnership, all or nothing.
//
// It re-proves, at the moment of writing, what the plan proved when it was built
// (checkProjectDestinations: containment after symlink resolution, nothing under the
// project's own skills/, every destination still as the plan found it), stages each
// new file next to its destination, then commits: the replaced and created files, the
// removals, and the project lock last, because the lock is the commit marker. Any
// failure puts every file back as it was, bytes and mode, and removes the directories
// this run created; if the undoing itself fails, the error is ErrRollbackIncomplete
// and names what is left.
//
// Only after the commit, the directories that a removal left empty inside an
// installed skill are pruned. They are not part of the rollback, so they are not
// touched until nothing can be rolled back any more.
//
// It prints nothing: what to say about each skill is the caller's.
func ExecuteInstallPlan(p InstallPlan, root string, fsys projectFS, stderr io.Writer) error {
	order := make([]ProjectWrite, 0, len(p.Writes)+len(p.Deletes)+1)
	deleting := make([]bool, 0, cap(order))
	for _, w := range p.Writes {
		order, deleting = append(order, w), append(deleting, false)
	}
	for _, w := range p.Deletes {
		order, deleting = append(order, w), append(deleting, true)
	}
	if p.Lock.Rel != "" {
		order, deleting = append(order, p.Lock), append(deleting, false)
	}
	if len(order) == 0 {
		return nil
	}

	root = filepath.Clean(root)
	if err := checkProjectDestinations(fsys, root, order); err != nil {
		return installFailure{err}
	}
	s := newProjectStager(fsys, root, order, deleting)
	if err := s.stageAndCommit(stderr); err != nil {
		return installFailure{err}
	}
	pruneEmptyDirs(fsys, p)
	return nil
}

// installFailure is an error from the shared executor, worded for install: the
// executor's messages are prefixed with the verb it was written for first, and this
// says "skills install" instead. It still unwraps to the original, so the rollback
// sentinel is still found.
type installFailure struct{ err error }

func (e installFailure) Error() string {
	return strings.ReplaceAll(e.err.Error(), "project-register:", "skills install:")
}

func (e installFailure) Unwrap() error { return e.err }

// pruneEmptyDirs removes, deepest first, the directories that removing the plan's
// files left empty inside an installed skill. It never removes a skill directory
// itself, and never a directory that still holds anything. It is best effort: a
// directory that cannot be removed is left, which is harmless.
func pruneEmptyDirs(fsys projectFS, p InstallPlan) {
	insideASkill := func(dir string) bool {
		for _, skillDir := range p.Dirs {
			if withinRoot(filepath.Clean(skillDir), filepath.Clean(dir)) {
				return true
			}
		}
		return false
	}
	for _, w := range p.Deletes {
		for dir := filepath.Dir(w.Abs); insideASkill(dir); dir = filepath.Dir(dir) {
			entries, err := fsys.ReadDir(dir)
			if err != nil || len(entries) > 0 {
				break
			}
			if err := fsys.Remove(dir); err != nil {
				break
			}
		}
	}
}
