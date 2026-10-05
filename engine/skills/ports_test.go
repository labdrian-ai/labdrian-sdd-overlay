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

func TestTheLockerIsAskedWhetherTheRegistryIsThereBeforeAWriterLocksIt(t *testing.T) {
	dir := t.TempDir()
	regPath, mfPath, skillsRoot := setupFixture(t, dir, minimalRegistry("existing"), minimalManifest("existing"), []string{"existing", "foo"})
	locker := &existsLocker{answers: map[string][]error{regPath: {errors.New("the locker cannot see it")}}}

	r := runAt("add", []string{"--registry", regPath, "--manifest", mfPath, "--source-root", skillsRoot, "foo"}, os.ReadFile, nil, locker)

	want := fmt.Sprintf("error: skills add: reading registry %q: the locker cannot see it; nothing was locked and nothing was changed\n", regPath)
	if r.code != 1 || r.stderr != want {
		t.Errorf("add = exit %d, stderr %q, want exit 1 and %q", r.code, r.stderr, want)
	}
	if locker.locks != 0 {
		t.Errorf("%d locks were taken, want none: the verb refuses before it asks for one", locker.locks)
	}
	if got, _ := os.ReadFile(regPath); string(got) != minimalRegistry("existing") {
		t.Errorf("the registry was changed to %q", got)
	}
}

// A shared lock on a lock file that is not there holds nothing, so validate reads again when the
// file appears during the read. Whether the file is there is the locker's to say.
func TestValidateReadsAgainWhenTheLockerSaysTheLockFileAppeared(t *testing.T) {
	dir := t.TempDir()
	regPath, mfPath, skillsRoot := setupFixture(t, dir, minimalRegistry("foo"), minimalManifest("foo"), []string{"foo"})
	lockFile := RegistryLockPath(regPath)
	// Absent when the lock is asked for, there once the read is done, there afterwards.
	locker := &existsLocker{answers: map[string][]error{
		regPath:  {nil},
		lockFile: {fs.ErrNotExist, nil, nil},
	}}

	r := runAt("validate", []string{"--registry", regPath, "--manifest", mfPath, "--source-root", skillsRoot}, os.ReadFile, nil, locker)

	if r.code != 0 {
		t.Fatalf("validate = exit %d, stderr %q", r.code, r.stderr)
	}
	if locker.locks != 2 {
		t.Errorf("%d locks taken, want 2: the verb read once, was told the lock file had appeared, and read again", locker.locks)
	}
	if strings.Count(r.stdout, "registry and manifest aligned") != 1 {
		t.Errorf("stdout %q, want the answer of the second read, printed once", r.stdout)
	}
}

func TestValidateRefusesWhenTheLockerCannotSayWhetherTheLockFileAppeared(t *testing.T) {
	dir := t.TempDir()
	regPath, mfPath, skillsRoot := setupFixture(t, dir, minimalRegistry("foo"), minimalManifest("foo"), []string{"foo"})
	lockFile := RegistryLockPath(regPath)
	locker := &existsLocker{answers: map[string][]error{
		regPath:  {nil},
		lockFile: {fs.ErrNotExist, &fs.PathError{Op: "stat", Path: lockFile, Err: syscall.EACCES}},
	}}

	r := runAt("validate", []string{"--registry", regPath, "--manifest", mfPath, "--source-root", skillsRoot}, os.ReadFile, nil, locker)

	want := fmt.Sprintf("error: skills validate: cannot tell whether a writer began while it read: stat %s: permission denied; nothing was changed\n", lockFile)
	if r.code != 1 || r.stderr != want || r.stdout != "" {
		t.Errorf("validate = exit %d, stdout %q, stderr %q, want exit 1, nothing on stdout and %q", r.code, r.stdout, r.stderr, want)
	}
	if locker.locks != 1 {
		t.Errorf("%d locks taken, want 1: a failure that reading again cannot clear is not read again", locker.locks)
	}
}

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

// validate lists the skills tree through the SkillTree it is given: a tree that is nowhere on
// disk, listed by a fake, is the tree the verb checks the manifest against.
func TestValidateListsTheSkillsTreeThroughTheTreeItIsGiven(t *testing.T) {
	dir := t.TempDir()
	regPath, mfPath, skillsRoot := setupFixture(t, dir, minimalRegistry("foo"), minimalManifest("foo"), []string{"foo"})
	tree := &fakeTree{scan: func(string) ([]string, error) { return []string{"foo/SKILL.md", "stray.md"}, nil }}

	var out, errBuf bytes.Buffer
	code := 0
	SkillsCoreAt("validate", []string{"validate", "--registry", regPath, "--manifest", mfPath, "--source-root", skillsRoot},
		Deps{ReadFile: os.ReadFile, Registries: testRegistries(os.ReadFile), Tree: tree, Locker: noopLocker{}},
		&out, &errBuf, func(c int) { code = c })

	if code != 1 || !strings.Contains(errBuf.String(), "[UNREGISTERED_ON_DISK] stray.md") {
		t.Errorf("validate = exit %d, stderr %q, want the file the tree listed reported as unregistered", code, errBuf.String())
	}
	if len(tree.asked) != 1 || tree.asked[0] != "scan "+skillsRoot {
		t.Errorf("the tree was asked %v, want one scan of the source root", tree.asked)
	}
}

func TestValidateSaysWhatTheTreeSaidWhenItCouldNotBeScanned(t *testing.T) {
	dir := t.TempDir()
	regPath, mfPath, skillsRoot := setupFixture(t, dir, minimalRegistry("foo"), minimalManifest("foo"), []string{"foo"})
	tree := &fakeTree{scan: func(string) ([]string, error) { return nil, errors.New("the tree is gone") }}

	var out, errBuf bytes.Buffer
	code := 0
	SkillsCoreAt("validate", []string{"validate", "--registry", regPath, "--manifest", mfPath, "--source-root", skillsRoot},
		Deps{ReadFile: os.ReadFile, Registries: testRegistries(os.ReadFile), Tree: tree, Locker: noopLocker{}},
		&out, &errBuf, func(c int) { code = c })

	want := fmt.Sprintf("error: scanning skills directory %q: the tree is gone\n", skillsRoot)
	if code != 1 || errBuf.String() != want {
		t.Errorf("validate = exit %d, stderr %q, want exit 1 and %q", code, errBuf.String(), want)
	}
}

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
		testRegistries(func(string) ([]byte, error) { return []byte(regYAML), nil }), tree, installCwdFn(project),
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
		testRegistries(func(string) ([]byte, error) { return []byte(regYAML), nil }), tree, installCwdFn(project),
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
	for _, verb := range []string{"validate", "install", "adopt"} {
		t.Run(verb, func(t *testing.T) {
			var out, errBuf bytes.Buffer
			code := -1
			SkillsCoreAt(verb, []string{verb}, Deps{ReadFile: os.ReadFile, Registries: testRegistries(os.ReadFile), Locker: noopLocker{}},
				&out, &errBuf, func(c int) { code = c })
			want := "error: skills " + verb + ": no skill tree is wired, so it cannot read the skills of the overlay\n"
			if code != 1 || errBuf.String() != want || out.Len() != 0 {
				t.Errorf("%s = exit %d, stdout %q, stderr %q, want exit 1 and %q", verb, code, out.String(), errBuf.String(), want)
			}
		})
	}
}
