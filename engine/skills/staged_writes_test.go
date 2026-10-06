package skills

// Tests of how the verbs that write an overlay use the StagedWrites port they are given: which
// files they stage, at which mode, in which order they commit them, what they put back when a
// step fails, and the words of each failure. The port answers from the real file system over a
// temporary directory, with a failure injected at one call; what the real adapter does with a
// real disk is tested in engine/skills/skillsfs, and what the program leaves on disk is pinned
// by the golden files of engine/cmd.

import (
	"errors"
	"io/fs"
	"path/filepath"
	"testing"
)

// stagedSpy records every call to the writes it wraps and can turn one into an error.
type stagedSpy struct {
	real  StagedWrites
	fail  func(op, path string, n int) error // n counts the calls of this op so far, from 1
	ops   []string
	perms []fs.FileMode
	n     map[string]int
}

func newStagedSpy(fail func(op, path string, n int) error) *stagedSpy {
	return &stagedSpy{real: testProjectFS(), fail: fail, n: map[string]int{}}
}

func (s *stagedSpy) check(op, path string) error {
	s.n[op]++
	s.ops = append(s.ops, op+" "+path)
	if s.fail != nil {
		return s.fail(op, path, s.n[op])
	}
	return nil
}

func (s *stagedSpy) WriteTemp(dir string, data []byte, perm fs.FileMode) (string, error) {
	s.perms = append(s.perms, perm)
	if err := s.check("writetemp", dir); err != nil {
		return "", err
	}
	return s.real.WriteTemp(dir, data, perm)
}

func (s *stagedSpy) Rename(oldPath, newPath string) error {
	if err := s.check("rename", newPath); err != nil {
		return err
	}
	return s.real.Rename(oldPath, newPath)
}

func (s *stagedSpy) Remove(name string) error {
	if err := s.check("remove", name); err != nil {
		return err
	}
	return s.real.Remove(name)
}

// A file is staged in the directory of its destination, and a destination that names no
// directory is staged in the working directory ("."), never in the directory of temporary files.
func TestWriteFileAtomicStagesInTheDirectoryOfThePath(t *testing.T) {
	for path, want := range map[string]string{
		"/overlay/skills.registry.yaml": "/overlay/",
		"overlay/overlay.manifest":      "overlay/",
		"overlay.manifest":              ".",
		"./overlay.manifest":            "./",
	} {
		t.Run(path, func(t *testing.T) {
			var got string
			spy := newStagedSpy(func(op, dir string, _ int) error {
				got = dir
				return errors.New("stop here")
			})
			if _, err := writeFileAtomic(spy, filepath.FromSlash(path), []byte("x"), 0o600); err == nil {
				t.Fatal("writeFileAtomic = nil, want the failure the port was told to return")
			}
			if got != filepath.FromSlash(want) {
				t.Errorf("staged in %q, want %q", got, filepath.FromSlash(want))
			}
		})
	}
}
