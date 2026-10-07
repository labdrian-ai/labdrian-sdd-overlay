package skills

// Tests for the overlay lock: which verbs take it, in which
// mode, what a busy or failed lock does, and that the interleavings it exists to
// prevent cannot happen. Nothing here touches the file system outside t.TempDir():
// the locker is injected, so no real lock file is ever created.

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
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
