package skills

// Tests of what the verbs ask of the ports they are given, with ports that answer from a map
// and not from the disk: the domain must take its answers from the port, which is the whole
// point of having one. What the file system adapter answers is tested in
// engine/skills/skillsfs, and what the program prints end to end is pinned by the golden files
// of engine/cmd.

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
)

// existsLocker is a Locker that takes every lock and answers Exists from a map: a path it has no
// answer for is absent.
type existsLocker struct {
	mu      sync.Mutex
	answers map[string][]error // the answers for a path, in the order they are asked; the last repeats
	asked   []string
	locks   int
}

func (l *existsLocker) Lock(string, LockMode) (func(), error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.locks++
	return func() {}, nil
}

func (l *existsLocker) LockDir(string, LockMode) (func(), error) { return func() {}, nil }

func (l *existsLocker) Exists(path string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.asked = append(l.asked, path)
	answers, ok := l.answers[path]
	if !ok {
		return fs.ErrNotExist
	}
	err := answers[0]
	if len(answers) > 1 {
		l.answers[path] = answers[1:]
	}
	return err
}

// --- isAbsent ---------------------------------------------------------------------------------

// A path that is not there is told by the words of the system and by the kind of error, however
// it was wrapped; a path that cannot be inspected is not absent. os.IsNotExist, which this
// replaces, did not see through a wrapped error; the system's own errors are what the verbs get.
func TestIsAbsentTellsAMissingPathFromOneThatCannotBeInspected(t *testing.T) {
	for name, tc := range map[string]struct {
		err  error
		want bool
	}{
		"no error":                         {nil, false},
		"the sentinel":                     {fs.ErrNotExist, true},
		"a path error that is not there":   {&fs.PathError{Op: "open", Path: "/x", Err: syscall.ENOENT}, true},
		"a wrapped error that is":          {fmt.Errorf("reading: %w", &fs.PathError{Op: "open", Path: "/x", Err: syscall.ENOENT}), true},
		"a path below a file":              {&fs.PathError{Op: "stat", Path: "/f/x", Err: syscall.ENOTDIR}, false},
		"a permission":                     {&fs.PathError{Op: "stat", Path: "/x", Err: syscall.EACCES}, false},
		"a loop of links":                  {&fs.PathError{Op: "stat", Path: "/x", Err: syscall.ELOOP}, false},
		"an error that says nothing of it": {errors.New("boom"), false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := isAbsent(tc.err); got != tc.want {
				t.Errorf("isAbsent(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// --- the locker answers whether a path is there -----------------------------------------------

func TestRereadsWhenTheLockFileAppearsTakesItsAnswersFromTheLocker(t *testing.T) {
	denied := &fs.PathError{Op: "stat", Path: "denied.lock", Err: syscall.EACCES}
	for _, tc := range []struct {
		name         string
		answers      map[string][]error
		paths        []string
		wantAppeared bool
		wantErr      error
	}{
		{"a file the locker says is there", map[string][]error{"a.lock": {nil}}, []string{"a.lock"}, true, nil},
		{"a file it says is not", map[string][]error{"a.lock": {fs.ErrNotExist}}, []string{"a.lock"}, false, nil},
		{"a file it has no answer for is absent", nil, []string{"a.lock"}, false, nil},
		{"a file it cannot answer for", map[string][]error{"denied.lock": {denied}}, []string{"denied.lock"}, false, denied},
		{"one there and one it cannot answer for", map[string][]error{"a.lock": {nil}, "denied.lock": {denied}}, []string{"a.lock", "denied.lock"}, true, denied},
	} {
		t.Run(tc.name, func(t *testing.T) {
			locker := &existsLocker{answers: tc.answers}
			appeared, err := rereadsWhenTheLockFileAppears(locker, tc.paths)
			if appeared != tc.wantAppeared || !errors.Is(err, tc.wantErr) {
				t.Errorf("rereadsWhenTheLockFileAppears = %v, %v, want %v, %v", appeared, err, tc.wantAppeared, tc.wantErr)
			}
			if strings.Join(locker.asked, ",") != strings.Join(tc.paths, ",") {
				t.Errorf("the locker was asked about %v, want %v: every path, in order", locker.asked, tc.paths)
			}
		})
	}
}

// --- the project verbs write through the project file system ----------------------------------

// A verb that reads or writes the files of a project and was given no file system refuses.
func TestAVerbThatNeedsTheProjectFileSystemRefusesWhenNoneIsWired(t *testing.T) {
	regPath := filepath.Join(t.TempDir(), "skills.registry.yaml")
	writeTestFile(t, regPath, minimalRegistry("existing"))
	for _, verb := range []string{"project-register", "project-revise", "project-status", "project-retire"} {
		t.Run(verb, func(t *testing.T) {
			var out, errBuf bytes.Buffer
			code := -1
			deps := testDeps(inDir(t.TempDir()), os.ReadFile, testRegistries(os.ReadFile), nil, noopLocker{})
			deps.Project = nil
			SkillsCoreAt(verb, []string{verb, "--registry", regPath}, deps, &out, &errBuf, func(c int) { code = c })
			want := "error: skills " + verb + ": no project file system is wired, so it cannot read or write files\n"
			if code != 1 || errBuf.String() != want || out.Len() != 0 {
				t.Errorf("%s = exit %d, stdout %q, stderr %q, want exit 1 and %q", verb, code, out.String(), errBuf.String(), want)
			}
		})
	}
}
