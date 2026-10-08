package skills

import (
	"fmt"
	"path/filepath"
)

// projectRetireStager carries a retirement out and holds the state needed to undo it. Delete
// backups use ProjectWrite.Backup for the old target bytes and Mode for its original permission
// bits; the lock uses the same representation but is restored only if its rename was attempted.
type projectRetireStager struct {
	fsys          ProjectFS
	root          string
	deletes       []ProjectWrite
	lock          ProjectWrite
	lockTemp      string
	lockAttempted bool
	attempted     []bool
}

func newProjectRetireStager(fsys ProjectFS, root string, p ProjectPlan) *projectRetireStager {
	return &projectRetireStager{
		fsys:      fsys,
		root:      root,
		deletes:   p.DeleteWrites,
		lock:      p.Lock,
		attempted: make([]bool, len(p.DeleteWrites)),
	}
}

// run stages the new lock, removes the targets, and commits the lock last, rolling back on any
// failure. The lock temp is staged before the first delete, so no deletion can become visible
// without a lock update that can either be committed or rolled back.
func (s *projectRetireStager) run() error {
	if err := s.stageLock(); err != nil {
		return err
	}
	if err := s.removeTargets(); err != nil {
		return err
	}
	return s.commitLock()
}

func (s *projectRetireStager) stageLock() error {
	tmp, err := writeProjectTemp(s.fsys, filepath.Dir(s.lock.Abs), s.lock.Data, s.lock.Mode)
	s.lockTemp = tmp
	if err != nil {
		return s.rollback(fmt.Errorf("%s: staging lock: %w", projectRetireVerb, err))
	}
	return nil
}

func (s *projectRetireStager) removeTargets() error {
	for i, w := range s.deletes {
		// A remove wrapper may delete the file and still report an error. Mark
		// the call before invoking it so rollback covers that check-then-act
		// window as well as ordinary successful removals.
		s.attempted[i] = true
		if err := s.fsys.Remove(w.Abs); err != nil {
			return s.rollback(fmt.Errorf("%s: removing %q: %w", projectRetireVerb, w.Rel, err))
		}
	}
	return nil
}

// commitLock renames the staged lock over the old one. The lock is the commit marker. The attempt
// is recorded before Rename because a filesystem wrapper can report an error after the rename
// landed.
func (s *projectRetireStager) commitLock() error {
	s.lockAttempted = true
	if err := s.fsys.Rename(s.lockTemp, s.lock.Abs); err != nil {
		return s.rollback(fmt.Errorf("%s: committing lock: %w", projectRetireVerb, err))
	}
	s.lockTemp = ""
	return nil
}

// rollback puts back what the retirement removed, and the lock if its rename was reached, and
// removes the staged lock; it returns cause, or a RollbackIncompleteError naming every path it
// could not restore.
func (s *projectRetireStager) rollback(cause error) error {
	bad := s.restoreTargets()
	bad = append(bad, s.restoreLock()...)
	if len(bad) != 0 {
		// Worded as it always was, for project-register, although this is a retirement.
		return &RollbackIncompleteError{Verb: projectRegisterVerb, Unrestored: bad, Cause: cause}
	}
	return cause
}

// restoreTargets restores target files in reverse order so a partially completed delete sequence
// is unwound from its newest mutation back toward its first one.
func (s *projectRetireStager) restoreTargets() (bad []string) {
	for i := len(s.deletes) - 1; i >= 0; i-- {
		if !s.attempted[i] {
			continue
		}
		w := s.deletes[i]
		if err := s.restore(w); err != nil {
			bad = append(bad, w.Rel)
		}
	}
	return bad
}

// restoreLock restores the old lock if its rename was reached, even when the injected rename
// failed before landing: rewriting the captured bytes is the safe choice because the wrapper may
// have landed the new lock already. The staged lock that was never renamed is removed.
func (s *projectRetireStager) restoreLock() (bad []string) {
	if s.lockAttempted {
		if err := s.restore(s.lock); err != nil {
			bad = append(bad, s.lock.Rel)
		}
	}
	if s.lockTemp != "" {
		if err := s.fsys.Remove(s.lockTemp); err != nil && !isAbsent(err) {
			bad = append(bad, s.lock.Rel)
		}
	}
	return bad
}

func (s *projectRetireStager) restore(w ProjectWrite) error {
	tmp, err := writeProjectTemp(s.fsys, filepath.Dir(w.Abs), w.Backup, w.Mode)
	if err != nil {
		return err
	}
	if err := s.fsys.Rename(tmp, w.Abs); err != nil {
		_ = s.fsys.Remove(tmp)
		return err
	}
	return nil
}
