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

// fakeTree is a SkillTree whose answers are given by the test, and that records what it was
// asked.
type fakeTree struct {
	scan   func(dir string) ([]string, error)
	source func(dir string) ([]SourceFile, error)
	mu     sync.Mutex
	asked  []string
}

func (f *fakeTree) ScanSkillFiles(dir string) ([]string, error) {
	f.mu.Lock()
	f.asked = append(f.asked, "scan "+dir)
	f.mu.Unlock()
	return f.scan(dir)
}

func (f *fakeTree) ReadSkillSource(dir string) ([]SourceFile, error) {
	f.mu.Lock()
	f.asked = append(f.asked, "source "+dir)
	f.mu.Unlock()
	return f.source(dir)
}

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

// --- the verbs read the tree through the port -------------------------------------------------

// install copies the files the SkillTree says a skill has, whatever the directory holds.
func TestInstallCopiesTheSourceTheTreeReads(t *testing.T) {
	overlay := t.TempDir()
	project := t.TempDir()
	makeSourceSkill(t, overlay, "skill-b", map[string]string{"SKILL.md": "on disk, which the tree does not read"})
	regYAML := `version: "1"
skills:
  - id: skill-b
    path: skill-b
    source:
      type: custom
    install:
      defaultScope: project
      targets:
        - claude
      allowedProjects:
        - target-repo
    lifecycle:
      updateStrategy: overlay-only
`
	tree := &fakeTree{source: func(string) ([]SourceFile, error) {
		return []SourceFile{{Rel: "SKILL.md", Data: []byte("what the tree read"), Mode: 0o644}}, nil
	}}
	var out, errBuf bytes.Buffer
	code := -1
	RenderInstallCore([]string{"--registry", "reg.yaml", "--source-root", overlay, "--project-id", "target-repo"},
		Deps{ReadFile: os.ReadFile, Registries: testRegistries(func(string) ([]byte, error) { return []byte(regYAML), nil }), Tree: tree, Project: testProjectFS(), Identity: testIdentity{}}, installCwdFn(project),
		&out, &errBuf, func(c int) { code = c })

	if code != 0 {
		t.Fatalf("install = exit %d, stderr %q", code, errBuf.String())
	}
	got, err := os.ReadFile(filepath.Join(project, ".claude", "skills", "skill-b", "SKILL.md"))
	if err != nil || string(got) != "what the tree read" {
		t.Errorf("installed SKILL.md = %q, %v, want the bytes the tree read", got, err)
	}
	if want := "source " + filepath.Join(overlay, "skill-b"); len(tree.asked) != 1 || tree.asked[0] != want {
		t.Errorf("the tree was asked %v, want [%q]", tree.asked, want)
	}
}

func TestInstallSaysWhatTheTreeSaidWhenASourceCouldNotBeRead(t *testing.T) {
	overlay := t.TempDir()
	project := t.TempDir()
	makeSourceSkill(t, overlay, "skill-b", map[string]string{"SKILL.md": "x"})
	regYAML := strings.Replace(minimalProjectRegistry, "SKILL", "skill-b", -1)
	tree := &fakeTree{source: func(string) ([]SourceFile, error) { return nil, errors.New("cannot read the source") }}
	var out, errBuf bytes.Buffer
	code := -1
	RenderInstallCore([]string{"--registry", "reg.yaml", "--source-root", overlay, "--project-id", "target-repo"},
		Deps{ReadFile: os.ReadFile, Registries: testRegistries(func(string) ([]byte, error) { return []byte(regYAML), nil }), Tree: tree, Project: testProjectFS(), Identity: testIdentity{}}, installCwdFn(project),
		&out, &errBuf, func(c int) { code = c })

	want := fmt.Sprintf("error: skill skill-b: reading its source %s: cannot read the source\n", filepath.Join(overlay, "skill-b"))
	if code != 1 || errBuf.String() != want {
		t.Errorf("install = exit %d, stderr %q, want exit 1 and %q", code, errBuf.String(), want)
	}
}

// minimalProjectRegistry admits the skill SKILL to the project target-repo.
const minimalProjectRegistry = `version: "1"
skills:
  - id: SKILL
    path: SKILL
    source:
      type: custom
    install:
      defaultScope: project
      targets:
        - claude
      allowedProjects:
        - target-repo
    lifecycle:
      updateStrategy: overlay-only
`

// A verb that needs the tree and was given none refuses; it does not crash.
func TestAVerbThatNeedsTheTreeRefusesWhenNoneIsWired(t *testing.T) {
	for _, verb := range []string{"install", "adopt"} {
		t.Run(verb, func(t *testing.T) {
			var out, errBuf bytes.Buffer
			code := -1
			deps := testDeps(inDir(t.TempDir()), os.ReadFile, testRegistries(os.ReadFile), nil, noopLocker{})
			deps.Tree = nil
			SkillsCoreAt(verb, []string{verb}, deps, &out, &errBuf, func(c int) { code = c })
			want := "error: skills " + verb + ": no skill tree is wired, so it cannot read the skills of the overlay\n"
			if code != 1 || errBuf.String() != want || out.Len() != 0 {
				t.Errorf("%s = exit %d, stdout %q, stderr %q, want exit 1 and %q", verb, code, out.String(), errBuf.String(), want)
			}
		})
	}
}

// A verb that reads or writes the files of a project and was given no file system refuses too.
func TestAVerbThatNeedsTheProjectFileSystemRefusesWhenNoneIsWired(t *testing.T) {
	regPath := filepath.Join(t.TempDir(), "skills.registry.yaml")
	writeTestFile(t, regPath, minimalRegistry("existing"))
	for _, verb := range []string{"approve", "install", "adopt", "project-register", "project-revise", "project-status", "project-retire"} {
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

// install and adopt install into the directory the Deps name, and a root that named none has
// wired nothing to install into.
func TestInstallAndAdoptRefuseWhenNoWorkingDirectoryIsWired(t *testing.T) {
	for verb, did := range map[string]string{"install": "installed", "adopt": "adopted"} {
		t.Run(verb, func(t *testing.T) {
			var out, errBuf bytes.Buffer
			code := -1
			deps := testDeps(nil, os.ReadFile, testRegistries(os.ReadFile), nil, noopLocker{})
			deps.Cwd = nil
			SkillsCoreAt(verb, []string{verb}, deps, &out, &errBuf, func(c int) { code = c })
			want := "error: skills " + verb + ": cannot resolve the project directory it works in (no working directory is wired); nothing was locked and nothing was " + did + "\n"
			if code != 1 || errBuf.String() != want {
				t.Errorf("%s = exit %d, stderr %q, want exit 1 and %q", verb, code, errBuf.String(), want)
			}
		})
	}
}

// install writes through the ProjectFS it is given and words a failure to stage a file as it
// always has: the verb's words, then 'writeProjectTemp: ', then the step the port says and the cause.
func TestInstallWritesThroughTheProjectFileSystemAndWordsItsFailure(t *testing.T) {
	overlay := t.TempDir()
	project := t.TempDir()
	makeSourceSkill(t, overlay, "skill-b", map[string]string{"SKILL.md": "B"})
	regYAML := strings.Replace(minimalProjectRegistry, "SKILL", "skill-b", -1)
	run := func(fsys ProjectFS) (string, int) {
		var out, errBuf bytes.Buffer
		code := -1
		deps := testDeps(nil, os.ReadFile, testRegistries(func(string) ([]byte, error) { return []byte(regYAML), nil }), nil, nil)
		deps.Project = fsys
		RenderInstallCore([]string{"--registry", "reg.yaml", "--source-root", overlay, "--project-id", "target-repo"},
			deps, installCwdFn(project), &out, &errBuf, func(c int) { code = c })
		return errBuf.String(), code
	}

	dest := filepath.Join(project, ".claude", "skills", "skill-b")
	failing := newFakeProjectFS(failAt("writetemp", dest, errors.New("create temp: no room")))
	stderr, code := run(failing)
	want := "error: skills install: staging \".claude/skills/skill-b/SKILL.md\": writeProjectTemp: create temp: no room\n"
	if code != 1 || stderr != want {
		t.Errorf("install with a file system that fails = exit %d, stderr %q, want exit 1 and %q", code, stderr, want)
	}
	if _, err := os.Stat(dest); err == nil {
		t.Error("something was installed although staging failed")
	}

	working := newFakeProjectFS(nil)
	if stderr, code := run(working); code != 0 {
		t.Fatalf("install with a file system that works = exit %d, stderr %q", code, stderr)
	}
	if working.n["writetemp"] == 0 || working.n["rename"] == 0 || working.n["mkdirall"] == 0 {
		t.Errorf("install did not write through the file system it was given: %v", working.n)
	}
}
