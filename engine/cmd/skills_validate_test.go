package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// What validate does about locks and about a read that a first writer tears is the adapter's to
// show: the use case reads once and says what it found. These tests give it a locker that records
// and a registry reader that can let a writer in at the moment it reads.

// recordingLocker records the locks taken and let go of, can be told to fail, and answers Exists
// from the real file system unless it is told otherwise.
type recordingLocker struct {
	mu     sync.Mutex
	events []string
	fail   error
	exists func(path string) error
}

func (l *recordingLocker) Lock(path string, mode skills.LockMode) (func(), error) {
	return l.take("lock", path, mode)
}

func (l *recordingLocker) LockDir(dir string, mode skills.LockMode) (func(), error) {
	return l.take("lockdir", dir, mode)
}

func (l *recordingLocker) take(kind, path string, mode skills.LockMode) (func(), error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.fail != nil {
		l.events = append(l.events, "refused "+path)
		return nil, l.fail
	}
	name := "exclusive"
	if mode == skills.LockShared {
		name = "shared"
	}
	l.events = append(l.events, fmt.Sprintf("%s %s %s", kind, name, filepath.Base(path)))
	return func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.events = append(l.events, "unlock "+filepath.Base(path))
	}, nil
}

func (l *recordingLocker) Exists(path string) error {
	if l.exists != nil {
		return l.exists(path)
	}
	_, err := os.Stat(path)
	return err
}

func (l *recordingLocker) log() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.events, "|")
}

type busyFailure struct{}

func (busyFailure) Error() string { return "held by another process" }
func (busyFailure) Busy() bool    { return true }

// letWriterIn is a RegistryRepository that runs onFirstLoad the first time the registry is read,
// after it was read: the reader holds the registry as it was, as if a writer took its lock then.
type letWriterIn struct {
	skills.RegistryRepository
	once        sync.Once
	onFirstLoad func()
}

func (r *letWriterIn) Load(location string) (skills.Registry, error) {
	reg, err := r.RegistryRepository.Load(location)
	r.once.Do(r.onFirstLoad)
	return reg, err
}

// validateWorld is an overlay with one global skill, alpha, approved, whose registry, manifest and
// tree agree. It has no lock file.
type validateWorld struct {
	t    *testing.T
	dir  string
	args []string
}

func newValidateWorld(t *testing.T) *validateWorld {
	t.Helper()
	w := &validateWorld{t: t, dir: t.TempDir()}
	w.writeRegistryOf("alpha")
	w.addSkill("alpha")
	w.writeManifestOf("alpha")
	w.args = []string{"validate", "--registry", w.path("skills.registry.yaml"), "--manifest", w.path("overlay.manifest"), "--source-root", w.path("skills")}
	return w
}

func (w *validateWorld) path(rel string) string { return filepath.Join(w.dir, filepath.FromSlash(rel)) }

func (w *validateWorld) put(rel, content string) {
	w.t.Helper()
	p := w.path(rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		w.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		w.t.Fatal(err)
	}
}

func (w *validateWorld) writeRegistryOf(ids ...string) {
	w.put("skills.registry.yaml", registryOf(ids...))
}

func (w *validateWorld) writeManifestOf(ids ...string) {
	var b strings.Builder
	for _, id := range ids {
		b.WriteString(id + "/SKILL.md custom\n")
	}
	w.put("overlay.manifest", b.String())
}

func (w *validateWorld) addSkill(id string) {
	content := skillFile(id)
	w.put("skills/"+id+"/SKILL.md", content)
	data, err := skills.SerializeApprovalRecord(skills.ApprovalRecord{Skill: id, SHA256: skills.SkillDigest([]byte(content)), ApprovedAt: "2026-09-30T12:00:00Z", Approver: "reviewer"})
	if err != nil {
		w.t.Fatal(err)
	}
	w.put("skills/"+id+"/"+skills.ApprovalRecordName, string(data))
}

func (w *validateWorld) lockPath() string {
	return skills.RegistryLockPath(w.path("skills.registry.yaml"))
}

func (w *validateWorld) deps(locker skills.Locker) skills.Deps {
	deps := skillsTestDeps()
	deps.Approvals = skillsApprovals()
	deps.Tree = skillsTree()
	deps.Locker = locker
	return deps
}

func TestSkillsValidateTakesTheSharedLockAndLetsGoOfIt(t *testing.T) {
	w := newValidateWorld(t)
	locker := &recordingLocker{}
	got := runSkillsVerb(skillsValidate, w.deps(locker), w.args...)
	if got.code() != 0 || got.stderr != "" || !strings.Contains(got.stdout, "registry and manifest aligned (1 skills)") {
		t.Fatalf("validate = %+v, want a pass", got)
	}
	if want := "lock shared .skills.registry.yaml.lock|unlock .skills.registry.yaml.lock"; locker.log() != want {
		t.Errorf("events = %q, want %q", locker.log(), want)
	}
	if _, err := os.Stat(w.lockPath()); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a shared lock made the lock file: %v", err)
	}
}

// The first writer ever creates the lock file and writes while a reader that holds nothing reads.
// The reader that sees the file appear reads again, under a lock that is real, and tells the
// settled state, not the pair of files it saw between two renames.
func TestSkillsValidateThatRacedTheFirstWriterReadsAgain(t *testing.T) {
	w := newValidateWorld(t)
	locker := &recordingLocker{}
	deps := w.deps(locker)
	deps.Registries = &letWriterIn{RegistryRepository: deps.Registries, onFirstLoad: func() {
		w.put(filepath.Base(w.lockPath()), "") // the writer takes its lock: the file now exists
		w.addSkill("beta")
		w.writeRegistryOf("alpha", "beta")
		w.writeManifestOf("alpha", "beta")
	}}
	got := runSkillsVerb(skillsValidate, deps, w.args...)
	if got.code() != 0 || got.stderr != "" || !strings.Contains(got.stdout, "aligned (2 skills)") {
		t.Fatalf("validate = %+v, want the settled registry of 2 skills", got)
	}
	want := "lock shared .skills.registry.yaml.lock|unlock .skills.registry.yaml.lock|lock shared .skills.registry.yaml.lock|unlock .skills.registry.yaml.lock"
	if locker.log() != want {
		t.Errorf("events = %q, want one read that held nothing and one under the lock that is real", locker.log())
	}
}

func TestSkillsValidateGivesUpWhenTheRegistryKeepsChanging(t *testing.T) {
	w := newValidateWorld(t)
	asked := 0
	locker := &recordingLocker{exists: func(string) error {
		asked++
		if asked%2 == 1 {
			return fs.ErrNotExist
		}
		return nil
	}}
	got := runSkillsVerb(skillsValidate, w.deps(locker), w.args...)
	want := "error: skills validate: the registry kept changing while it was being read (3 attempts); nothing was changed, retry in a moment\n"
	if got.code() != skills.ExitBusy || got.stdout != "" || got.stderr != want {
		t.Errorf("validate = %+v, want exit %d and %q", got, skills.ExitBusy, want)
	}
}

func TestSkillsValidateRefusesAReadItCannotTellWasTorn(t *testing.T) {
	w := newValidateWorld(t)
	asked := 0
	locker := &recordingLocker{exists: func(string) error {
		asked++
		if asked == 1 {
			return fs.ErrNotExist
		}
		return errors.New("permission denied")
	}}
	got := runSkillsVerb(skillsValidate, w.deps(locker), w.args...)
	want := "error: skills validate: cannot tell whether a writer began while it read: permission denied; nothing was changed\n"
	if got.code() != 1 || got.stdout != "" || got.stderr != want {
		t.Errorf("validate = %+v, want exit 1 and %q", got, want)
	}
}

func TestSkillsValidateExitsBusyWhenTheLockStaysTaken(t *testing.T) {
	w := newValidateWorld(t)
	locker := &recordingLocker{fail: busyFailure{}}
	got := runSkillsVerb(skillsValidate, w.deps(locker), w.args...)
	if got.code() != skills.ExitBusy || got.stdout != "" || !strings.Contains(got.stderr, "another skills command is in progress for the registry "+w.path("skills.registry.yaml")) {
		t.Errorf("validate = %+v, want exit %d and the registry named", got, skills.ExitBusy)
	}
}

func TestSkillsValidateWillNotRunUnserialized(t *testing.T) {
	w := newValidateWorld(t)
	got := runSkillsVerb(skillsValidate, w.deps(nil), w.args...)
	want := "error: skills validate: no lock is configured, so it will not run unserialized with the other skills commands\n"
	if got.code() != 1 || got.stdout != "" || got.stderr != want {
		t.Errorf("validate = %+v, want exit 1 and %q", got, want)
	}
}

func TestSkillsValidateRefusesWhatItDoesNotKnowBeforeTakingAnyLock(t *testing.T) {
	w := newValidateWorld(t)
	locker := &recordingLocker{}
	got := runSkillsVerb(skillsValidate, w.deps(locker), append(w.args, "--frobnicate")...)
	if got.code() != 1 || got.stderr != "error: skills validate: unknown flag \"--frobnicate\"\n" || locker.log() != "" {
		t.Errorf("validate = %+v, locks %q, want a refusal and no lock", got, locker.log())
	}
}

func TestSkillsValidateTellsTheDivergencesBeforeTheFailureThatStopsIt(t *testing.T) {
	w := newValidateWorld(t)
	w.writeRegistryOf("alpha", "beta") // beta has no row in the manifest
	w.addSkill("beta")
	deps := w.deps(&recordingLocker{})
	deps.Tree = failingTree{err: errors.New("the tree is gone")}
	got := runSkillsVerb(skillsValidate, deps, w.args...)
	if got.code() != 1 || got.stdout != "" {
		t.Fatalf("validate = %+v, want exit 1", got)
	}
	lines := strings.Split(strings.TrimRight(got.stderr, "\n"), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "[") || !strings.Contains(lines[0], "beta") || !strings.HasPrefix(lines[1], "error: scanning skills directory ") {
		t.Errorf("stderr = %q, want the registry divergence, then the scan failure", got.stderr)
	}
}

type failingTree struct{ err error }

func (t failingTree) ScanSkillFiles(string) ([]string, error) { return nil, t.err }
func (failingTree) ReadSkillSource(string) ([]skills.SourceFile, error) {
	return nil, errors.New("not used")
}

func TestSkillsValidateRefusesAnOverlayWithNoTreeOrNoApprovalStoreWired(t *testing.T) {
	w := newValidateWorld(t)
	deps := w.deps(&recordingLocker{})
	deps.Tree = nil
	if got := runSkillsVerb(skillsValidate, deps, w.args...); got.code() != 1 || got.stderr != "error: skills validate: no skill tree is wired, so it cannot read the skills of the overlay\n" {
		t.Errorf("no tree: %+v", got)
	}
	deps = w.deps(&recordingLocker{})
	deps.Approvals = nil
	if got := runSkillsVerb(skillsValidate, deps, w.args...); got.code() != 1 || got.stderr != "error: skills validate: no approval record store is wired, so it cannot tell whether a skill is approved\n" {
		t.Errorf("no approvals: %+v", got)
	}
}

func TestSkillsValidateNeedsTheSourceRoot(t *testing.T) {
	w := newValidateWorld(t)
	got := runSkillsVerb(skillsValidate, w.deps(&recordingLocker{}), "validate", "--registry", w.path("skills.registry.yaml"), "--manifest", w.path("overlay.manifest"))
	if got.code() != 1 || got.stdout != "" || got.stderr != "error: skills validate requires --source-root <skills-dir>\n" {
		t.Errorf("validate = %+v", got)
	}
}
