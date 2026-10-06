package skills

// Tests of how the verbs that write an overlay use the StagedWrites port they are given: which
// files they stage, at which mode, in which order they commit them, what they put back when a
// step fails, and the words of each failure. The port answers from the real file system over a
// temporary directory, with a failure injected at one call; what the real adapter does with a
// real disk is tested in engine/skills/skillsfs, and what the program leaves on disk is pinned
// by the golden files of engine/cmd.

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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

var errInjectedWrite = errors.New("injected failure")

func TestApproveStagesTheRecordWorldReadableAndPutsBackWhenItCannotCommit(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "foo", "SKILL.md"), lintCleanSkillMD("foo"))
	run := func(files StagedWrites) (int, string, string) {
		var out, errBuf bytes.Buffer
		code := -1
		RenderApproveCore([]string{"--id", "foo", "--approver", "reviewer", "--source-root", root},
			os.ReadFile, fileApprovals(os.ReadFile), fixedClock(approveFixedNow), files, &out, &errBuf, func(c int) { code = c })
		return code, out.String(), errBuf.String()
	}
	record := ApprovalRecordPath(root, "foo")

	failing := newStagedSpy(func(op, _ string, _ int) error {
		if op == "writetemp" {
			return errors.New("create temp: no room")
		}
		return nil
	})
	code, _, stderr := run(failing)
	want := fmt.Sprintf("error: skills approve: skill %q: writing approval record: writeFileAtomic: create temp: no room\n", "foo")
	if code != 1 || stderr != want {
		t.Errorf("approve with a file system that cannot stage = exit %d, stderr %q, want exit 1 and %q", code, stderr, want)
	}

	failing = newStagedSpy(func(op, _ string, _ int) error {
		if op == "rename" {
			return errInjectedWrite
		}
		return nil
	})
	code, _, stderr = run(failing)
	want = fmt.Sprintf("error: skills approve: skill %q: writing approval record: finalizing %q: injected failure\n", "foo", record)
	if code != 1 || stderr != want {
		t.Errorf("approve with a file system that cannot commit = exit %d, stderr %q, want exit 1 and %q", code, stderr, want)
	}
	if _, err := os.Stat(record); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a record is there after a failed commit: %v", err)
	}
	entries, _ := os.ReadDir(filepath.Join(root, "foo"))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), atomicTempPrefix) {
			t.Errorf("the temporary file %s was left behind", e.Name())
		}
	}

	working := newStagedSpy(nil)
	if code, _, stderr := run(working); code != 0 {
		t.Fatalf("approve = exit %d, stderr %q", code, stderr)
	}
	if len(working.perms) != 1 || working.perms[0] != 0o644 {
		t.Errorf("the record was staged at %v, want 0644: it is committed with the skill", working.perms)
	}
	if info, err := os.Stat(record); err != nil || info.Mode().Perm() != 0o644 {
		t.Errorf("the record is %v, %v, want a file at 0644", info, err)
	}
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
