package skills

import (
	"fmt"
	"io"
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
// back to its pre-run state on any failure. It prints nothing.
func (s *projectStager) stageAndCommit(stderr io.Writer) error {
	fsys, order := s.fsys, s.order

	// Stage: create each target directory, recording which ones this run
	// created, and write every SKILL.md and the new lock to a same-directory
	// temp at mode 0644. A removal has nothing to stage.
	for i, w := range order {
		if s.isDelete(i) {
			continue
		}
		dir := filepath.Dir(w.Abs)
		if err := s.mkdirAll(dir); err != nil {
			return s.rollback(stderr, fmt.Errorf("%s: creating %q: %w", s.verb, path.Dir(w.Rel), err))
		}
		tmp, err := writeProjectTemp(fsys, dir, w.Data, w.Mode)
		if err != nil {
			return s.rollback(stderr, fmt.Errorf("%s: staging %q: %w", s.verb, w.Rel, err))
		}
		s.temps[i] = tmp
	}

	// Commit: rename the SKILL.md temps in projectTargets order, the lock last.
	// (An install also removes files, in its place in the order.)
	for i, w := range order {
		// The destination's REAL mode, read immediately before it is renamed
		// over, is the only thing that can put it back the way it was. The
		// planned Mode cannot: it is always ProjectFileMode, so a lock the
		// project keeps at 0600 came back at 0644 after a failed run and the
		// headline guarantee — byte-identical, including file modes — was false
		// (review round 4, D1).
		if w.Backup != nil {
			info, err := fsys.Stat(w.Abs)
			if err != nil {
				return s.rollback(stderr, fmt.Errorf("%s: inspecting %q before committing over it: %w", s.verb, w.Rel, err))
			}
			s.preMode[i] = info.Mode().Perm()
		}
		// Recorded BEFORE the call, not after it: a rename that reports an error
		// may still have landed, and rollback must sweep the destinations this
		// run reached for. It must equally leave alone the ones it never reached
		// — restoring identical bytes over an untouched file still replaces it,
		// with a new inode and the planned mode (review round 4, D1).
		s.attempted[i] = true
		if s.isDelete(i) {
			if err := fsys.Remove(w.Abs); err != nil {
				return s.rollback(stderr, fmt.Errorf("%s: removing %q: %w", s.verb, w.Rel, err))
			}
			continue
		}
		if err := fsys.Rename(s.temps[i], w.Abs); err != nil {
			return s.rollback(stderr, fmt.Errorf("%s: committing %q: %w", s.verb, w.Rel, err))
		}
		s.temps[i] = ""
	}
	return nil
}

// ErrRollbackIncomplete marks the one outcome that leaves the operator work to
// do: the run failed AND the rollback could not fully undo it. The CLI maps a
// non-nil return from ExecuteProjectPlan to exit 1; this sentinel is what lets
// it (and a test) tell an honest "nothing happened" from "look at these paths".
var ErrRollbackIncomplete = fmt.Errorf("project-register: rollback incomplete")

// rollbackIncompleteError is ErrRollbackIncomplete worded for the verb that ran the
// executor, with the paths left and the cause as its detail. errors.Is finds the
// sentinel in it. For "project-register" the text is the sentinel's, unchanged.
type rollbackIncompleteError struct{ verb, detail string }

func (e rollbackIncompleteError) Error() string {
	return e.verb + ": rollback incomplete: " + e.detail
}

func (e rollbackIncompleteError) Is(target error) bool { return target == ErrRollbackIncomplete }

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
func (s *projectStager) rollback(stderr io.Writer, cause error) error {
	var bad []string

	// 1. Every destination this run reached the rename for, newest first:
	// restore the bytes it captured at plan time AT THE MODE IT REALLY HAD, or
	// remove it when it is a genuinely new file.
	//
	// It sweeps every write whose rename was ATTEMPTED rather than only those
	// that succeeded, because a rename that reports an error may still have
	// landed (an NFS or wrapper reality). It stops at the ones that were never
	// attempted, because those were never touched: rewriting them restored
	// nothing and changed two things it had no business changing, the inode and
	// the mode (review round 4, D1).
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

	// 2. Leftover temps.
	for _, tmp := range s.temps {
		if tmp == "" {
			continue
		}
		if err := s.remove(tmp); err != nil {
			bad = append(bad, s.rel(tmp))
		}
	}

	// 3. Directories this run created, deepest first and only if empty.
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

	if len(bad) > 0 {
		for _, rel := range bad {
			fmt.Fprintf(stderr, "error: rollback incomplete: %s\n", rel)
		}
		return rollbackIncompleteError{verb: s.verb, detail: fmt.Sprintf("%s (after %v)", strings.Join(bad, ", "), cause)}
	}
	return cause
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
