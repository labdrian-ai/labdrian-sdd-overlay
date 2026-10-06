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
// fail. It also tracks which paths are held right now.
type recordingLocker struct {
	osExists
	mu     sync.Mutex
	events []string
	held   map[string]int
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
	if l.held == nil {
		l.held = map[string]int{}
	}
	l.held[path]++
	l.events = append(l.events, fmt.Sprintf("%s %s %s", kind, modeName(mode), path))
	return func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.held[path]--
		l.events = append(l.events, fmt.Sprintf("unlock %s", path))
	}, nil
}

func (l *recordingLocker) isHeld(path string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.held[path] > 0
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

// withSkillsFor is withSkills for the one verb that needs an unregistered skill
// on disk: every other verb's fixture must stay a clean validate target.
func (f lockFixture) withSkillsFor(t *testing.T, verb string, ids ...string) lockFixture {
	t.Helper()
	if verb == "add" {
		return f.withSkills(t, ids...)
	}
	return f
}

// withSkills puts approved, lint-clean skills on disk that are not registered, for
// the add tests. A fixture that has any of them is not a clean validate target.
func (f lockFixture) withSkills(t *testing.T, ids ...string) lockFixture {
	t.Helper()
	for _, id := range ids {
		writeTestFile(t, filepath.Join(f.root, id, "SKILL.md"), lintCleanSkillMD(id))
		writeValidApproval(t, f.root, id)
	}
	return f
}

func (f lockFixture) flags() []string {
	return []string{"--registry", f.reg, "--manifest", f.man, "--source-root", f.root}
}

func TestSkillsCoreAt_TakesTheRegistryLockByVerb(t *testing.T) {
	for _, tc := range []struct {
		verb  string
		extra []string
		mode  string
		code  int
	}{
		{"add", []string{"newbie"}, "exclusive", 0},
		{"remove", []string{"existing"}, "exclusive", 0},
		{"sync-manifest", nil, "exclusive", 0},
		{"approve", []string{"--id", "existing", "--approver", "reviewer"}, "exclusive", 0},
		{"validate", nil, "shared", 0},
		{"install", []string{"--project-id", "p"}, "shared", 0},
	} {
		t.Run(tc.verb, func(t *testing.T) {
			f := newLockFixture(t).withSkillsFor(t, tc.verb, "newbie")
			locker := &recordingLocker{}
			want := []string{"lock " + tc.mode + " " + f.lockPath, "unlock " + f.lockPath}
			if tc.verb == "install" {
				// install writes into the working directory, so it also takes the
				// project lock, second (see project_dirlock_test.go).
				want = []string{"lock shared " + f.lockPath, "lockdir exclusive " + f.project, "unlock " + f.project, "unlock " + f.lockPath}
			}
			r := f.runAt(tc.verb, append(append([]string{}, tc.extra...), f.flags()...), os.ReadFile, fixedClock(approveFixedNow), locker)
			if r.code != tc.code {
				t.Fatalf("exit %d, want %d; stderr=%q", r.code, tc.code, r.stderr)
			}
			if got := locker.log(); !reflect.DeepEqual(got, want) {
				t.Errorf("lock events = %v, want %v", got, want)
			}
		})
	}
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

func TestSkillsCoreAt_TheLockIsKeyedByTheRegistryTheVerbNames(t *testing.T) {
	f := newLockFixture(t)
	other := filepath.Join(f.dir, "other", "team.registry.yaml")
	// A writer locks the registry it names, and only a registry that is there: see
	// TestAWriterWithoutARegistryLocksNothing.
	registry, err := os.ReadFile(f.reg)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, other, string(registry))

	first := func(l *recordingLocker) string {
		if events := l.log(); len(events) > 0 {
			return events[0]
		}
		return "(no lock event)"
	}

	locker := &recordingLocker{}
	f.runAt("sync-manifest", []string{"--registry", other, "--manifest", f.man}, os.ReadFile, nil, locker)
	if got, want := first(locker), "lock exclusive "+filepath.Join(f.dir, "other", ".team.registry.yaml.lock"); got != want {
		t.Errorf("first event = %q, want %q", got, want)
	}

	// No --registry: the default the verbs share, relative to the working directory,
	// which here holds a registry.
	chdirToATempDir(t)
	writeTestFile(t, defaultRegistryPath, string(registry))
	locker = &recordingLocker{}
	f.runAt("sync-manifest", []string{"--manifest", f.man}, os.ReadFile, nil, locker)
	if got, want := first(locker), "lock exclusive .skills.registry.yaml.lock"; got != want {
		t.Errorf("first event without --registry = %q, want %q", got, want)
	}
}

// State is read after the lock is taken, never before: a read made before it can
// be stale by the time the lock is granted.
func TestSkillsCoreAt_EveryReadOfSharedStateHappensUnderTheLock(t *testing.T) {
	for _, tc := range []struct {
		verb  string
		extra []string
	}{
		{"add", []string{"newbie"}},
		{"remove", []string{"existing"}},
		{"sync-manifest", nil},
		{"approve", []string{"--id", "existing", "--approver", "reviewer"}},
		{"validate", nil},
		{"install", []string{"--project-id", "p"}},
	} {
		t.Run(tc.verb, func(t *testing.T) {
			f := newLockFixture(t).withSkillsFor(t, tc.verb, "newbie")
			locker := &recordingLocker{}
			var outside []string
			readFile := func(name string) ([]byte, error) {
				if !locker.isHeld(f.lockPath) {
					outside = append(outside, name)
				}
				return os.ReadFile(name)
			}
			r := f.runAt(tc.verb, append(append([]string{}, tc.extra...), f.flags()...), readFile, fixedClock(approveFixedNow), locker)
			if r.code != 0 {
				t.Fatalf("exit %d; stderr=%q", r.code, r.stderr)
			}
			if len(outside) != 0 {
				t.Errorf("read %v before the lock was taken or after it was released", outside)
			}
		})
	}
}

func TestSkillsCoreAt_TheLockIsReleasedWhenTheVerbRefuses(t *testing.T) {
	f := newLockFixture(t)
	locker := &recordingLocker{}
	r := f.runAt("remove", append([]string{"never-registered"}, f.flags()...), os.ReadFile, nil, locker)
	if r.code != 1 {
		t.Fatalf("exit %d, want 1; stderr=%q", r.code, r.stderr)
	}
	want := []string{"lock exclusive " + f.lockPath, "unlock " + f.lockPath}
	if got := locker.log(); !reflect.DeepEqual(got, want) {
		t.Errorf("lock events = %v, want %v", got, want)
	}
}

// ---- busy, failed, and missing locks -----------------------------------------------

// A lock that stays taken past the bound is reported with exit 2 and a retry
// message, and the verb does not run: the files are byte-for-byte as they were.
func TestSkillsCoreAt_ABusyLockExits2WithARetryMessageAndChangesNothing(t *testing.T) {
	for _, tc := range []struct {
		verb  string
		extra []string
	}{
		{"add", []string{"newbie"}},
		{"remove", []string{"existing"}},
		{"sync-manifest", nil},
		{"approve", []string{"--id", "existing", "--approver", "reviewer"}},
		{"validate", nil},
		{"install", []string{"--project-id", "p"}},
	} {
		t.Run(tc.verb, func(t *testing.T) {
			f := newLockFixture(t).withSkillsFor(t, tc.verb, "newbie")
			before := snapshotFiles(t, f.reg, f.man, ApprovalRecordPath(f.root, "existing"))
			locker := &recordingLocker{fail: busyErr{f.lockPath}}

			r := f.runAt(tc.verb, append(append([]string{}, tc.extra...), f.flags()...), os.ReadFile, fixedClock(approveFixedNow), locker)

			if r.code != ExitBusy || ExitBusy != 2 {
				t.Errorf("exit %d (ExitBusy %d), want 2", r.code, ExitBusy)
			}
			if r.stdout != "" {
				t.Errorf("stdout %q, want nothing on a busy lock", r.stdout)
			}
			for _, want := range []string{"skills " + tc.verb, "in progress", "retry", f.lockPath} {
				if !strings.Contains(r.stderr, want) {
					t.Errorf("stderr %q does not contain %q", r.stderr, want)
				}
			}
			if after := snapshotFiles(t, f.reg, f.man, ApprovalRecordPath(f.root, "existing")); !reflect.DeepEqual(before, after) {
				t.Errorf("a busy lock changed files:\nbefore %v\nafter  %v", before, after)
			}
			if entries, _ := filepath.Glob(filepath.Join(f.dir, ".tmp-skills-*")); len(entries) != 0 {
				t.Errorf("a busy lock left temp files %v", entries)
			}
		})
	}
}

// A lock that cannot be taken for any other reason is a refusal, exit 1: waiting
// would not help, so the message must not tell the caller to retry.
func TestSkillsCoreAt_ALockThatCannotBeTakenExits1AndDoesNotSayRetry(t *testing.T) {
	f := newLockFixture(t).withSkills(t, "newbie")
	before := snapshotFiles(t, f.reg, f.man)
	locker := &recordingLocker{fail: fmt.Errorf("open %s: permission denied", f.lockPath)}

	r := f.runAt("add", append([]string{"newbie"}, f.flags()...), os.ReadFile, nil, locker)

	if r.code != 1 || r.stdout != "" {
		t.Fatalf("exit %d, stdout %q, want exit 1 and nothing on stdout", r.code, r.stdout)
	}
	for _, want := range []string{"skills add", "cannot take the lock", "permission denied", f.lockPath} {
		if !strings.Contains(r.stderr, want) {
			t.Errorf("stderr %q does not contain %q", r.stderr, want)
		}
	}
	if strings.Contains(r.stderr, "retry") {
		t.Errorf("stderr %q tells the caller to retry a failure that will not clear", r.stderr)
	}
	if after := snapshotFiles(t, f.reg, f.man); !reflect.DeepEqual(before, after) {
		t.Error("a failed lock changed files")
	}
}

// A busy error that a caller wrapped is still a busy error.
func TestSkillsCoreAt_RecognizesAWrappedBusyError(t *testing.T) {
	f := newLockFixture(t)
	locker := &recordingLocker{fail: fmt.Errorf("acquire: %w", busyErr{f.lockPath})}
	if r := f.runAt("sync-manifest", f.flags(), os.ReadFile, nil, locker); r.code != ExitBusy {
		t.Errorf("exit %d for a wrapped busy error, want %d; stderr=%q", r.code, ExitBusy, r.stderr)
	}
}

// Without a locker the verbs that change shared state refuse rather than run
// unserialized.
func TestSkillsCoreAt_WithoutALockerTheLockingVerbsFailClosed(t *testing.T) {
	f := newLockFixture(t).withSkills(t, "newbie")
	before := snapshotFiles(t, f.reg, f.man)
	for _, verb := range []string{"add", "remove", "sync-manifest", "approve", "validate", "install", "adopt"} {
		r := f.runAt(verb, append([]string{"newbie"}, f.flags()...), os.ReadFile, fixedClock(approveFixedNow), nil)
		if r.code != 1 || r.stdout != "" || !strings.Contains(r.stderr, "no lock is configured") {
			t.Errorf("%s: exit %d, stdout %q, stderr %q, want exit 1 and a 'no lock is configured' refusal", verb, r.code, r.stdout, r.stderr)
		}
	}
	if after := snapshotFiles(t, f.reg, f.man); !reflect.DeepEqual(before, after) {
		t.Error("a verb ran without a lock")
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

// Two adds that both read the registry before either wrote used to leave only one
// of them registered although both exited 0. The gate parks the first reader after
// it has read, so the second cannot help reading the same registry unless the lock
// keeps it out.
func TestConcurrentAddsBothLandInTheRegistryAndTheManifest(t *testing.T) {
	f := newLockFixture(t).withSkills(t, "alpha", "beta")
	gate := newReadGate(t, f.reg)
	locker := &exclusionLocker{blocked: gate.release}
	add := func(id string) func() coreRun {
		return func() coreRun {
			return f.runAt("add", append([]string{id}, f.flags()...), gate.readFile, nil, locker)
		}
	}

	results := runConcurrently(add("alpha"), add("beta"))

	for i, r := range results {
		if r.code != 0 {
			t.Errorf("add #%d: exit %d, stderr=%q", i, r.code, r.stderr)
		}
	}
	want := []string{"alpha", "beta", "existing"}
	if got := registryIDs(t, f.reg); !reflect.DeepEqual(got, want) {
		t.Errorf("registry lists %v, want %v: an add that exited 0 was lost", got, want)
	}
	if got := manifestIDs(t, f.man); !reflect.DeepEqual(got, want) {
		t.Errorf("manifest lists %v, want %v", got, want)
	}
}

func TestConcurrentRemovesBothLeaveTheRegistryAndTheManifest(t *testing.T) {
	dir := t.TempDir()
	reg, man, root := setupFixture(t, dir, minimalRegistry("keep", "one", "two"), minimalManifest("keep", "one", "two"), []string{"keep", "one", "two"})
	gate := newReadGate(t, reg)
	locker := &exclusionLocker{blocked: gate.release}
	flags := []string{"--registry", reg, "--manifest", man, "--source-root", root}
	remove := func(id string) func() coreRun {
		return func() coreRun {
			return runAt("remove", append([]string{id}, flags...), gate.readFile, nil, locker)
		}
	}

	results := runConcurrently(remove("one"), remove("two"))

	for i, r := range results {
		if r.code != 0 {
			t.Errorf("remove #%d: exit %d, stderr=%q", i, r.code, r.stderr)
		}
	}
	want := []string{"keep"}
	if got := registryIDs(t, reg); !reflect.DeepEqual(got, want) {
		t.Errorf("registry lists %v, want %v: a remove that exited 0 was undone", got, want)
	}
	if got := manifestIDs(t, man); !reflect.DeepEqual(got, want) {
		t.Errorf("manifest lists %v, want %v", got, want)
	}
}

// sync-manifest regenerates the manifest from the registry it read; an add that
// lands between that read and its write must not be erased from the manifest.
//
// The sync parks after reading the registry. Then the add runs: without a lock it
// completes and the sync is released afterwards to write from the registry it
// read; with one, the add has to wait for the sync, which releases the sync.
func TestSyncManifestDoesNotEraseAConcurrentAdd(t *testing.T) {
	f := newLockFixture(t).withSkills(t, "alpha")
	gate := newReadGate(t, f.reg)
	locker := &exclusionLocker{blocked: gate.release}

	syncDone := make(chan coreRun, 1)
	go func() { syncDone <- f.runAt("sync-manifest", f.flags(), gate.readFile, nil, locker) }()
	<-gate.arrived
	added := f.runAt("add", append([]string{"alpha"}, f.flags()...), os.ReadFile, nil, locker)
	gate.release()
	synced := <-syncDone

	for name, r := range map[string]coreRun{"add": added, "sync-manifest": synced} {
		if r.code != 0 {
			t.Errorf("%s: exit %d, stderr=%q", name, r.code, r.stderr)
		}
	}
	reg, man := registryIDs(t, f.reg), manifestIDs(t, f.man)
	if !reflect.DeepEqual(reg, man) {
		t.Errorf("registry lists %v but the manifest lists %v", reg, man)
	}
	if want := []string{"alpha", "existing"}; !reflect.DeepEqual(reg, want) {
		t.Errorf("registry lists %v, want %v", reg, want)
	}
}

// Two approvals of the same bytes: the second must find the first one's record
// valid and leave it alone, so the original approver stays the record of who
// approved these bytes. Both used to read "no record" and both wrote one.
func TestConcurrentApprovalsOfTheSameBytesKeepTheFirstApprover(t *testing.T) {
	e := newApproveEnv(t, "my-skill")
	reg := filepath.Join(filepath.Dir(e.root), "skills.registry.yaml")
	gate := newReadGate(t, e.recordPath())
	locker := &exclusionLocker{blocked: gate.release}
	approve := func(approver string) func() coreRun {
		return func() coreRun {
			args := []string{"--id", e.id, "--approver", approver, "--source-root", e.root, "--registry", reg}
			return runAt("approve", args, gate.readFile, fixedClock(approveFixedNow), locker)
		}
	}

	results := runConcurrently(approve("alice"), approve("bob"))

	approvers := []string{"alice", "bob"}
	winner, unchanged := "", 0
	for i, r := range results {
		if r.code != 0 {
			t.Errorf("approve by %s: exit %d, stderr=%q", approvers[i], r.code, r.stderr)
		}
		switch {
		case strings.HasPrefix(r.stdout, "approved: "):
			if winner != "" {
				t.Errorf("both approvals reported 'approved:'; the second must be 'unchanged:' (stdouts %q)", []string{results[0].stdout, results[1].stdout})
			}
			winner = approvers[i]
		case strings.HasPrefix(r.stdout, "unchanged: "):
			unchanged++
		default:
			t.Errorf("approve by %s printed %q", approvers[i], r.stdout)
		}
	}
	if winner == "" || unchanged != 1 {
		t.Fatalf("outcomes: winner %q, unchanged %d, want exactly one of each", winner, unchanged)
	}
	if got := readRecord(t, e.recordPath()).Approver; got != winner {
		t.Errorf("the record names %q, want the approver whose approval was reported, %q", got, winner)
	}
}
