package skills

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ExecuteProjectRetirePlan removes the planned target files and commits the
// lock update last. A failure after any target removal restores every target
// already reached from its captured bytes before returning the original cause;
// a failure during that rollback returns a *RollbackIncompleteError that names, repo-relative,
// every path it could not restore. The lock temp is staged
// before the first delete, so no deletion can become visible without a lock
// update that can either be committed or rolled back.
func ExecuteProjectRetirePlan(p ProjectPlan, fsys ProjectFS) error {
	if fsys == nil {
		return fmt.Errorf("project-retire: no filesystem was injected")
	}
	root, err := projectPlanRoot(p)
	if err != nil {
		return err
	}
	if len(p.DeleteWrites) == 0 {
		return fmt.Errorf("project-retire: the plan carries no target deletions")
	}
	if p.Lock.Backup == nil {
		return fmt.Errorf("project-retire: the plan carries no existing lock backup")
	}
	order := append([]ProjectWrite(nil), p.DeleteWrites...)
	order = append(order, p.Lock)
	// Worded as it always was, for project-register, although this is a retirement.
	if err := checkProjectDestinations(projectRegisterVerb, fsys, root, order); err != nil {
		return err
	}

	s := &projectRetireStager{
		fsys:      fsys,
		root:      root,
		deletes:   p.DeleteWrites,
		lock:      p.Lock,
		attempted: make([]bool, len(p.DeleteWrites)),
	}

	tmp, err := writeProjectTemp(fsys, filepath.Dir(p.Lock.Abs), p.Lock.Data, p.Lock.Mode)
	s.lockTemp = tmp
	if err != nil {
		return s.rollback(fmt.Errorf("project-retire: staging lock: %w", err))
	}

	for i, w := range p.DeleteWrites {
		// A remove wrapper may delete the file and still report an error. Mark
		// the call before invoking it so rollback covers that check-then-act
		// window as well as ordinary successful removals.
		s.attempted[i] = true
		if err := fsys.Remove(w.Abs); err != nil {
			return s.rollback(fmt.Errorf("project-retire: removing %q: %w", w.Rel, err))
		}
	}

	// The lock is the commit marker. Record the attempt before Rename because
	// a filesystem wrapper can report an error after the rename landed.
	s.lockAttempted = true
	if err := fsys.Rename(s.lockTemp, p.Lock.Abs); err != nil {
		return s.rollback(fmt.Errorf("project-retire: committing lock: %w", err))
	}
	s.lockTemp = ""
	return nil
}

// ExecuteProjectRevisePlan intentionally delegates to the registration
// executor. Revision writes carry backups for every target and the lock, so
// the existing temp -> ordered rename -> rollback implementation already
// provides the required mid-revision recovery without a second write path.
func ExecuteProjectRevisePlan(p ProjectPlan, fsys ProjectFS) error {
	return ExecuteProjectPlan(p, fsys)
}

// projectPlanRoot recovers the project root from the plan. design.md fixes
// ExecuteProjectPlan's signature and it carries no root, but every write pairs
// a repo-relative Rel with the absolute Abs it was resolved to, so the root is
// exactly Abs with that suffix removed. Deriving it from the LOCK write is
// deliberate: the lock is present in every plan PlanProjectRegister produces,
// so a zero or hand-built plan is refused here instead of being half-executed.
func projectPlanRoot(p ProjectPlan) (string, error) {
	if p.Lock.Rel == "" || p.Lock.Abs == "" {
		return "", fmt.Errorf("project-register: the plan carries no lock write, so it was never produced by PlanProjectRegister")
	}
	suffix := string(filepath.Separator) + filepath.FromSlash(p.Lock.Rel)
	abs := filepath.Clean(p.Lock.Abs)
	root := strings.TrimSuffix(abs, suffix)
	if root == abs || root == "" {
		return "", fmt.Errorf("project-register: lock write %q does not end in its repo-relative path %q", p.Lock.Abs, p.Lock.Rel)
	}
	return root, nil
}

// ExecuteProjectPlan performs one registration: it stages every planned file
// as a same-directory temp, then renames the SKILL.md temps in projectTargets
// order and the lock LAST. Any failure rolls the tree back to its pre-run
// state before returning.
//
// It performs no validation of the registration itself — PlanProjectRegister
// owns all eleven validate-before-write checks — but it DOES re-establish the
// containment proof, because the planner's proof is point-in-time and a path
// component can become a symlink between the plan and this write.
//
// Cross-directory atomicity is not available on POSIX (design.md,
// alternatives). The git commit is the real atomic unit; this function's job
// is to leave either the full planned set or the pre-run state behind.
func ExecuteProjectPlan(p ProjectPlan, fsys ProjectFS) error {
	// Retirement uses the same public executor seam as registration and
	// revision when called by a future CLI, but needs delete-specific rollback
	// state. Keep the dedicated implementation behind this dispatch so callers
	// that already know only ExecuteProjectPlan still receive atomic retirement
	// behavior.
	if len(p.DeleteWrites) > 0 || len(p.Deletes) > 0 {
		return ExecuteProjectRetirePlan(p, fsys)
	}

	root, err := projectPlanRoot(p)
	if err != nil {
		return err
	}
	order := projectCommitOrder(p)
	if err := checkProjectDestinations(projectRegisterVerb, fsys, root, order); err != nil {
		return err
	}

	s := newProjectStager(projectRegisterVerb, fsys, root, order, nil)
	return s.stageAndCommit()
}

// projectRegisterVerb is how the project verbs word a failure of the executor they
// share with `skills install` and `skills adopt`, which pass their own names.
// Retirement words its destination refusals the same way; that is how it always
// read, although it is not registration.
const projectRegisterVerb = "project-register"
