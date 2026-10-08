package skills

import (
	"path/filepath"
)

// projectRetireStager holds the state needed to undo a retirement. Delete
// backups use ProjectWrite.Backup for the old target bytes and Mode for its
// original permission bits; the lock uses the same representation but is
// restored only if its rename was attempted.
type projectRetireStager struct {
	fsys          ProjectFS
	root          string
	deletes       []ProjectWrite
	lock          ProjectWrite
	lockTemp      string
	lockAttempted bool
	attempted     []bool
}

func (s *projectRetireStager) rollback(cause error) error {
	var bad []string

	// Restore target files in reverse order so a partially completed delete
	// sequence is unwound from its newest mutation back toward its first one.
	for i := len(s.deletes) - 1; i >= 0; i-- {
		if !s.attempted[i] {
			continue
		}
		w := s.deletes[i]
		if err := s.restore(w); err != nil {
			bad = append(bad, w.Rel)
		}
	}

	// If the lock rename was reached, restore the old lock even when the
	// injected rename failed before landing. Rewriting the captured bytes is
	// the safe choice because the wrapper may have landed the new lock already.
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

	if len(bad) != 0 {
		// Worded as it always was, for project-register, although this is a retirement.
		return &RollbackIncompleteError{Verb: projectRegisterVerb, Unrestored: bad, Cause: cause}
	}
	return cause
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
