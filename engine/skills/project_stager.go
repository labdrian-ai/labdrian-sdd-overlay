package skills

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pathguard"
)

func newProjectStager(verb string, fsys ProjectFS, root string, order []ProjectWrite, deleting []bool) *projectStager {
	return &projectStager{
		verb:      verb,
		fsys:      fsys,
		root:      root,
		order:     order,
		deleting:  deleting,
		temps:     make([]string, len(order)),
		attempted: make([]bool, len(order)),
		preMode:   make([]fs.FileMode, len(order)),
	}
}

// isDelete reports whether order[i] removes its file instead of writing it.
func (s *projectStager) isDelete(i int) bool { return i < len(s.deleting) && s.deleting[i] }

// stageAndCommit stages every write and commits the whole order, rolling the tree
// back to its pre-run state on any failure. It prints nothing: what a failed rollback could not
// restore is in the error it returns.
func (s *projectStager) stageAndCommit() error {
	if err := s.stage(); err != nil {
		return err
	}
	return s.commit()
}

// stage creates each target directory, recording which ones this run created, and writes every
// SKILL.md and the new lock to a same-directory temp at mode 0644. A removal has nothing to stage.
func (s *projectStager) stage() error {
	for i, w := range s.order {
		if s.isDelete(i) {
			continue
		}
		dir := filepath.Dir(w.Abs)
		if err := s.mkdirAll(dir); err != nil {
			return s.rollback(fmt.Errorf("%s: creating %q: %w", s.verb, path.Dir(w.Rel), err))
		}
		tmp, err := writeProjectTemp(s.fsys, dir, w.Data, w.Mode)
		if err != nil {
			return s.rollback(fmt.Errorf("%s: staging %q: %w", s.verb, w.Rel, err))
		}
		s.temps[i] = tmp
	}
	return nil
}

// commit renames the SKILL.md temps in projectTargets order, the lock last. (An install also
// removes files, in its place in the order.)
func (s *projectStager) commit() error {
	for i, w := range s.order {
		if err := s.commitOne(i, w); err != nil {
			return s.rollback(err)
		}
	}
	return nil
}

// commitOne commits the write at order[i]: it records what rollback needs, and then renames or
// removes. A failure is returned worded for the verb; the caller rolls back.
func (s *projectStager) commitOne(i int, w ProjectWrite) error {
	// The destination's REAL mode, read immediately before it is renamed
	// over, is the only thing that can put it back the way it was. The
	// planned Mode cannot: it is always ProjectFileMode, so a lock the
	// project keeps at 0600 came back at 0644 after a failed run and the
	// headline guarantee — byte-identical, including file modes — was false
	// (review round 4, D1).
	if w.Backup != nil {
		mode, err := modeBeforeCommit(s.fsys, s.verb, w)
		if err != nil {
			return err
		}
		s.preMode[i] = mode
	}
	// Recorded BEFORE the call, not after it: a rename that reports an error
	// may still have landed, and rollback must sweep the destinations this
	// run reached for. It must equally leave alone the ones it never reached
	// — restoring identical bytes over an untouched file still replaces it,
	// with a new inode and the planned mode (review round 4, D1).
	s.attempted[i] = true
	if s.isDelete(i) {
		if err := s.fsys.Remove(w.Abs); err != nil {
			return fmt.Errorf("%s: removing %q: %w", s.verb, w.Rel, err)
		}
		return nil
	}
	if err := s.fsys.Rename(s.temps[i], w.Abs); err != nil {
		return fmt.Errorf("%s: committing %q: %w", s.verb, w.Rel, err)
	}
	s.temps[i] = ""
	return nil
}

// modeBeforeCommit is the REAL mode of the destination of w, read now, immediately before it is
// renamed over. Registration, revision, install and retirement all restore a destination at this
// mode and not at the one the plan recorded, which is as old as the plan.
func modeBeforeCommit(fsys ProjectFS, verb string, w ProjectWrite) (fs.FileMode, error) {
	info, err := fsys.Stat(w.Abs)
	if err != nil {
		return 0, fmt.Errorf("%s: inspecting %q before committing over it: %w", verb, w.Rel, err)
	}
	return info.Mode().Perm(), nil
}

// ErrRollbackIncomplete marks the one outcome that leaves the operator work to
// do: the run failed AND the rollback could not fully undo it. The CLI maps a
// non-nil return from ExecuteProjectPlan to exit 1; this sentinel is what lets
// it (and a test) tell an honest "nothing happened" from "look at these paths".
var ErrRollbackIncomplete = errors.New(projectRegisterVerb + ": rollback incomplete")

// RollbackIncompleteError is ErrRollbackIncomplete worded for the verb that ran the executor, and
// the typed result of an execution that failed and could not put everything back: Unrestored is
// every path the rollback could not restore, repo-relative and slash-separated (usable as a git
// pathspec), in the order the rollback found them, and Cause is the failure that made it roll
// back. errors.Is finds the sentinel in it; Cause is told, not unwrapped. The executor prints
// nothing: the caller says Unrestored where it likes.
type RollbackIncompleteError struct {
	Verb       string
	Unrestored []string
	Cause      error
}

func (e *RollbackIncompleteError) Error() string {
	return e.Verb + ": rollback incomplete: " + strings.Join(e.Unrestored, ", ") + " (after " + e.Cause.Error() + ")"
}

func (e *RollbackIncompleteError) Is(target error) bool { return target == ErrRollbackIncomplete }

// projectStager records what one execution has done so far, which is exactly
// what rollback needs: the directories this run created and the temps it
// staged. The planned writes themselves are already in order.
type projectStager struct {
	verb      string // how a failure is worded: the command that is executing the plan
	fsys      ProjectFS
	root      string
	order     []ProjectWrite
	deleting  []bool   // deleting[i]: order[i] removes its file (nil: nothing is removed)
	temps     []string // "" once renamed away or never staged
	created   []string // directories this run created
	attempted []bool   // a rename over order[i].Abs was reached for
	// preMode[i] is the destination's ACTUAL mode, read immediately before this
	// run renamed over it. It is meaningful only where order[i].Backup != nil,
	// because only those destinations are restored rather than removed.
	preMode []fs.FileMode
}

// mkdirAll creates every ancestor of dir strictly below the root that does not
// exist yet, ONE LEVEL AT A TIME, recording each level as soon as it is made.
//
// A single MkdirAll for the whole path cannot be made safe: os.MkdirAll creates
// the ancestors it can and only then fails on a deeper component, and it never
// reports which ones it created. Recording the whole list only after it returns
// nil therefore leaked every directory a partial failure left behind — rollback
// never learned they existed, so a failed run could leave a `.claude/skills/`
// in a project that had never had one (review round 4, D2). Creating one level
// at a time makes "it was created" and "it was recorded" the same event: each
// call has its parent already in place, so it creates exactly that level or
// nothing at all.
func (s *projectStager) mkdirAll(dir string) error {
	var missing []string
	for cur := filepath.Clean(dir); pathguard.WithinRoot(s.root, cur); cur = filepath.Dir(cur) {
		if _, err := s.fsys.Stat(cur); err == nil {
			break
		} else if !isAbsent(err) {
			return err
		}
		missing = append(missing, cur) // deepest first
	}
	// Shallowest first, so every call's parent already exists.
	for i := len(missing) - 1; i >= 0; i-- {
		if err := s.fsys.MkdirAll(missing[i], projectDirMode); err != nil {
			return err
		}
		s.created = append(s.created, missing[i])
	}
	return nil
}

// rollback returns the tree to its pre-run state and reports cause, or
// ErrRollbackIncomplete naming every path it could not restore.
//
// It walks the PLANNED writes rather than only the renames it saw succeed,
// because a rename that reports an error may still have landed (an NFS or
// wrapper reality). Restoring or removing a destination that was never touched
// is a no-op, so the wider sweep costs nothing and closes that window.
func (s *projectStager) rollback(cause error) error {
	bad := s.restoreReachedDestinations()
	bad = append(bad, s.removeLeftoverTemps()...)
	bad = append(bad, s.removeCreatedDirectories()...)
	if len(bad) > 0 {
		return &RollbackIncompleteError{Verb: s.verb, Unrestored: bad, Cause: cause}
	}
	return cause
}

// restoreReachedDestinations is the first step of a rollback: every destination this run reached
// the rename for, newest first: restore the bytes it captured at plan time AT THE MODE IT REALLY
// HAD, or remove it when it is a genuinely new file.
//
// It sweeps every write whose rename was ATTEMPTED rather than only those
// that succeeded, because a rename that reports an error may still have
// landed (an NFS or wrapper reality). It stops at the ones that were never
// attempted, because those were never touched: rewriting them restored
// nothing and changed two things it had no business changing, the inode and
// the mode (review round 4, D1).
func (s *projectStager) restoreReachedDestinations() (bad []string) {
	for i := len(s.order) - 1; i >= 0; i-- {
		if !s.attempted[i] {
			continue
		}
		w := s.order[i]
		if w.Backup == nil {
			if err := s.remove(w.Abs); err != nil {
				bad = append(bad, w.Rel)
			}
			continue
		}
		mode := s.preMode[i]
		if mode == 0 {
			// Unreachable while the commit loop records preMode before every
			// attempted rename of a backed-up write; kept so a future caller
			// that sets attempted without it degrades to the planned mode
			// instead of creating a file nobody can read.
			mode = w.Mode
		}
		tmp, err := writeProjectTemp(s.fsys, filepath.Dir(w.Abs), w.Backup, mode)
		if err != nil {
			bad = append(bad, w.Rel)
			continue
		}
		if err := s.fsys.Rename(tmp, w.Abs); err != nil {
			_ = s.remove(tmp)
			bad = append(bad, w.Rel)
		}
	}
	return bad
}

// removeLeftoverTemps is the second step: the temps that were staged and never renamed away.
func (s *projectStager) removeLeftoverTemps() (bad []string) {
	for _, tmp := range s.temps {
		if tmp == "" {
			continue
		}
		if err := s.remove(tmp); err != nil {
			bad = append(bad, s.rel(tmp))
		}
	}
	return bad
}

// removeCreatedDirectories is the third step: the directories this run created, deepest first and
// only if empty.
func (s *projectStager) removeCreatedDirectories() (bad []string) {
	for _, dir := range s.createdDeepestFirst() {
		entries, err := s.fsys.ReadDir(dir)
		if err != nil {
			if isAbsent(err) {
				continue
			}
			bad = append(bad, s.rel(dir))
			continue
		}
		if len(entries) > 0 {
			continue
		}
		if err := s.remove(dir); err != nil {
			bad = append(bad, s.rel(dir))
		}
	}
	return bad
}

// remove deletes p, treating "it is already gone" as success: rollback sweeps
// destinations it may never have created.
func (s *projectStager) remove(p string) error {
	if err := s.fsys.Remove(p); err != nil && !isAbsent(err) {
		return err
	}
	return nil
}

// createdDeepestFirst orders the created directories so a child is always
// removed before its parent. Sorting by descending path length is enough: a
// child is strictly longer than its parent.
func (s *projectStager) createdDeepestFirst() []string {
	dirs := append([]string(nil), s.created...)
	sort.SliceStable(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	return dirs
}

// rel turns an absolute path back into the repo-relative, slash-separated form
// every reported path uses, so a rollback pointer is usable as a git pathspec.
func (s *projectStager) rel(p string) string {
	return filepath.ToSlash(strings.TrimPrefix(p, s.root+string(filepath.Separator)))
}
