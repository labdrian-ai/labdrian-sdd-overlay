package skills

// Tests for the overlay lock: which verbs SkillsCoreAt takes it for, in which
// mode, what a busy or failed lock does, and that the interleavings it exists to
// prevent cannot happen. Nothing here touches the file system outside t.TempDir():
// the locker is injected, so no real lock file is ever created.

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---- fakes ------------------------------------------------------------------

// busyErr is a lock that stayed taken: the shape engine/filelock's BusyError has,
// which engine/skills recognizes without importing it.
type busyErr struct{ path string }

func (e busyErr) Error() string { return "lock " + e.path + " is held by another process" }
func (busyErr) Busy() bool      { return true }

// recordingLocker records every lock and unlock, in order, and can be told to
// fail.
type recordingLocker struct {
	osExists
	mu     sync.Mutex
	events []string
	fail   error            // every lock fails with it
	failOn map[string]error // the lock on this path or directory fails with it
}

func (l *recordingLocker) Lock(path string, mode LockMode) (func(), error) {
	return l.take("lock", path, mode)
}

func (l *recordingLocker) LockDir(dir string, mode LockMode) (func(), error) {
	return l.take("lockdir", dir, mode)
}

func (l *recordingLocker) take(kind, path string, mode LockMode) (func(), error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	err := l.fail
	if e := l.failOn[path]; e != nil {
		err = e
	}
	if err != nil {
		l.events = append(l.events, fmt.Sprintf("refused %s %s", modeName(mode), path))
		return nil, err
	}
	l.events = append(l.events, fmt.Sprintf("%s %s %s", kind, modeName(mode), path))
	return func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.events = append(l.events, fmt.Sprintf("unlock %s", path))
	}, nil
}

func (l *recordingLocker) log() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.events...)
}

func modeName(m LockMode) string {
	if m == LockShared {
		return "shared"
	}
	return "exclusive"
}

// noopLocker takes no lock at all. Tests whose subject is a verb's own behavior,
// not the lock, use it through skillsCoreUnlocked.
type noopLocker struct{ osExists }

func (noopLocker) Lock(string, LockMode) (func(), error)    { return func() {}, nil }
func (noopLocker) LockDir(string, LockMode) (func(), error) { return func() {}, nil }

// skillsCoreUnlocked is SkillsCore for tests that exercise a verb's own behavior:
// no clock, and a locker that never blocks.
func skillsCoreUnlocked(verb string, args []string, readFile readFileFn, stdout, stderr io.Writer, exit func(int)) {
	skillsCoreAt(verb, args, readFile, testRegistries(readFile), nil, noopLocker{}, stdout, stderr, exit)
}

// exclusionLocker is a real in-process lock: one exclusive holder or any number of
// shared ones, per path. blocked runs when a Lock call cannot be granted at once,
// which is how an interleaving test learns that the other side is waiting.
type exclusionLocker struct {
	osExists
	mu      sync.Mutex
	locks   map[string]*sync.RWMutex
	log     []string
	blocked func()
}

// record adds one event to the order in which locks were granted and released.
func (l *exclusionLocker) record(event string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.log = append(l.log, event)
}

// events returns the grants and releases so far, in order.
func (l *exclusionLocker) events() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.log...)
}

func (l *exclusionLocker) rw(path string) *sync.RWMutex {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.locks == nil {
		l.locks = map[string]*sync.RWMutex{}
	}
	if l.locks[path] == nil {
		l.locks[path] = &sync.RWMutex{}
	}
	return l.locks[path]
}

func (l *exclusionLocker) Lock(path string, mode LockMode) (func(), error) {
	return l.take(path, path, mode)
}

// LockDir locks a directory. A directory and a file that happen to share a path
// are different locks in the tests as little as they are in the kernel, where a
// file and a directory cannot share a path at all.
func (l *exclusionLocker) LockDir(dir string, mode LockMode) (func(), error) {
	return l.take("dir:"+dir, dir, mode)
}

func (l *exclusionLocker) take(key, path string, mode LockMode) (func(), error) {
	rw := l.rw(key)
	name := modeName(mode) + " " + filepath.Base(path)
	if mode == LockShared {
		if !rw.TryRLock() {
			l.wait()
			rw.RLock()
		}
		l.record("granted " + name)
		return func() { l.record("released " + name); rw.RUnlock() }, nil
	}
	if !rw.TryLock() {
		l.wait()
		rw.Lock()
	}
	l.record("granted " + name)
	return func() { l.record("released " + name); rw.Unlock() }, nil
}

func (l *exclusionLocker) wait() {
	if l.blocked != nil {
		l.blocked()
	}
}

// readGate forces the interleaving a missing lock allows. The first read of its
// path parks after the bytes are read, so the reader holds a state that is about
// to go stale, until either a second reader arrives (nothing serialized them) or
// the locker reports that somebody is waiting for the first one's lock (something
// did). It never sleeps on the passing path; the timeout only bounds a hung test.
type readGate struct {
	t        *testing.T
	path     string
	mu       sync.Mutex
	arrivals int
	once     sync.Once
	released chan struct{}
	arrived  chan struct{} // closed when the first reader has read and parked
}

func newReadGate(t *testing.T, path string) *readGate {
	return &readGate{t: t, path: filepath.Clean(path), released: make(chan struct{}), arrived: make(chan struct{})}
}

func (g *readGate) release() { g.once.Do(func() { close(g.released) }) }

func (g *readGate) readFile(name string) ([]byte, error) {
	data, err := os.ReadFile(name)
	if filepath.Clean(name) != g.path {
		return data, err
	}
	g.mu.Lock()
	g.arrivals++
	n := g.arrivals
	g.mu.Unlock()
	if n > 1 {
		g.release()
		return data, err
	}
	close(g.arrived)
	select {
	case <-g.released:
	case <-time.After(20 * time.Second):
		g.t.Error("the first reader was never released: the interleaving test hung")
	}
	return data, err
}

type coreRun struct {
	stdout, stderr string
	code           int
}

// runAt runs one verb through SkillsCoreAt and returns what it printed and its
// exit code. Some verbs (list, validate) return without calling exit when they
// succeed, as a process that falls off the end of main does, so that is 0.
func runAt(verb string, args []string, readFile readFileFn, now func() string, locker Locker) coreRun {
	return runAtIn(nil, verb, args, readFile, now, locker)
}

// runAtIn is runAt for a verb that installs into the directory cwd names. runAt wires none.
func runAtIn(cwd func() (string, error), verb string, args []string, readFile readFileFn, now func() string, locker Locker) coreRun {
	var out, errBuf bytes.Buffer
	code := 0
	skillsCoreAtIn(cwd, verb, append([]string{verb}, args...), readFile, testRegistries(readFile), now, locker, &out, &errBuf, func(c int) { code = c })
	return coreRun{out.String(), errBuf.String(), code}
}

// inDir is the working directory a test gives a verb: always dir.
func inDir(dir string) func() (string, error) {
	return func() (string, error) { return dir, nil }
}

// runConcurrently runs every job at once and returns their results in order.
func runConcurrently(jobs ...func() coreRun) []coreRun {
	results := make([]coreRun, len(jobs))
	var wg sync.WaitGroup
	for i, job := range jobs {
		wg.Add(1)
		go func(i int, job func() coreRun) {
			defer wg.Done()
			results[i] = job()
		}(i, job)
	}
	wg.Wait()
	return results
}

func registryIDs(t *testing.T, regPath string) []string {
	t.Helper()
	data, err := os.ReadFile(regPath)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := parseRegistry(data)
	if err != nil {
		t.Fatalf("registry does not parse: %v", err)
	}
	var ids []string
	for _, e := range reg.Skills {
		ids = append(ids, e.ID)
	}
	sort.Strings(ids)
	return ids
}

func manifestIDs(t *testing.T, mfPath string) []string {
	t.Helper()
	mv, err := loadManifestViewFile(mfPath)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for dir := range mv {
		ids = append(ids, dir)
	}
	sort.Strings(ids)
	return ids
}

// ---- the lock path ------------------------------------------------------------

func TestRegistryLockPath_IsADotSiblingOfTheRegistry(t *testing.T) {
	sep := string(os.PathSeparator)
	for registry, want := range map[string]string{
		"skills.registry.yaml":                      ".skills.registry.yaml.lock",
		"." + sep + "skills.registry.yaml":          ".skills.registry.yaml.lock",
		sep + "repo" + sep + "skills.registry.yaml": sep + "repo" + sep + ".skills.registry.yaml.lock",
		"a" + sep + "b" + sep + "registry.yaml":     "a" + sep + "b" + sep + ".registry.yaml.lock",
	} {
		if got := RegistryLockPath(registry); got != want {
			t.Errorf("RegistryLockPath(%q) = %q, want %q", registry, got, want)
		}
	}
}

// The lock file is created beside the real registry by the first write, so the
// repository must ignore it or every checkout that ever ran `skills add` would
// show an untracked file (and `pipkg build`, which asks git whether the overlay is
// clean, would call it dirty).
func TestTheRegistryLockIsGitIgnored(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	want := "/" + RegistryLockPath("skills.registry.yaml")
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == want {
			return
		}
	}
	t.Errorf(".gitignore has no line %q for the registry lock file", want)
}

// The lock is not skill content: a scan of a tree that has it beside the skills
// does not report it as an unregistered file.
func TestTheRegistryLockIsNotSkillContent(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "one", "SKILL.md"), lintCleanSkillMD("one"))
	writeTestFile(t, filepath.Join(root, filepath.Base(RegistryLockPath("skills.registry.yaml"))), "")
	got, err := scanSkillFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"one/SKILL.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ScanSkillFiles = %v, want %v", got, want)
	}
}

// ---- which verbs lock, and how ---------------------------------------------------

// lockFixture is a registry with one skill, its manifest and source tree, all
// approved, so every locking verb can run for real. project is the temporary
// directory install is pointed at, so that no test of the lock installs into the
// directory the tests run in.
type lockFixture struct {
	dir, reg, man, root string
	lockPath            string
	project             string
}

func newLockFixture(t *testing.T) lockFixture {
	t.Helper()
	dir := t.TempDir()
	reg, man, root := setupFixture(t, dir, minimalRegistry("existing"), minimalManifest("existing"), []string{"existing"})
	return lockFixture{dir: dir, reg: reg, man: man, root: root, lockPath: RegistryLockPath(reg), project: t.TempDir()}
}

// runAt runs a verb that installs into the project of the fixture, never into the directory the
// tests run in.
func (f lockFixture) runAt(verb string, args []string, readFile readFileFn, now func() string, locker Locker) coreRun {
	return runAtIn(inDir(f.project), verb, args, readFile, now, locker)
}

func TestSkillsCoreAt_TakesNoLockForTheVerbsThatNeedNone(t *testing.T) {
	f := newLockFixture(t)
	for _, tc := range []struct {
		verb string
		args []string
	}{
		{"nuke", nil},
		{"", nil},
	} {
		locker := &recordingLocker{}
		f.runAt(tc.verb, tc.args, os.ReadFile, nil, locker)
		if got := locker.log(); len(got) != 0 {
			t.Errorf("verb %q took locks %v, want none", tc.verb, got)
		}
	}
}

// The project verbs lock the project they work on, and refuse to run when no locker
// is configured. The three that write would lose updates if two interleaved.
// project-status only reads, but it reads the project lock file and then the skill
// files it lists, which a revision's renames leave disagreeing for a moment, so it too
// refuses to run unserialized.
func TestSkillsCoreAt_WithoutALockerTheProjectVerbsFailClosed(t *testing.T) {
	for _, verb := range []string{"project-status", "project-register", "project-revise", "project-retire"} {
		t.Run(verb, func(t *testing.T) {
			root := t.TempDir()

			r := runAt(verb, []string{"--project-root", root}, os.ReadFile, nil, nil)

			if r.code != 1 || r.stdout != "" || !strings.Contains(r.stderr, "no lock is configured") {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 1 and a 'no lock is configured' refusal", r.code, r.stdout, r.stderr)
			}
			if !strings.Contains(r.stderr, "skills "+verb) {
				t.Errorf("stderr %q does not name the verb %s", r.stderr, verb)
			}
			if entries, _ := os.ReadDir(root); len(entries) != 0 {
				t.Errorf("a verb without a lock touched the project: %v", entries)
			}
		})
	}
}

// ---- the interleavings the lock exists to prevent ----------------------------------
