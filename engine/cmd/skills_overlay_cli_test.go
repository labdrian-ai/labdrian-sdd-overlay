package main

// Tests of the adapter of the verbs that write an overlay (skills_overlay_cli.go): which lock each
// takes and when, what a busy or a refused lock does, what is read under it, which ports must be
// wired, and the words each refusal of a use case is told in. What the use cases decide is tested
// in engine/skills/app, and what the program prints and leaves on disk by the golden files.

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

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/registryyaml"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/skillsfs"
)

const overlayApprovedAt = "2026-09-30T12:00:00Z"

func overlayClock() string { return overlayApprovedAt }

// overlaySkillMD is the smallest SKILL.md that passes the hard lint.
func overlaySkillMD(id string) string {
	return "---\nname: " + id + "\ndescription: A concise procedural skill for " + id + ".\nlicense: MIT\n" +
		"metadata:\n  author: tester\n  version: \"1.0\"\n---\n" +
		"## Activation Contract\nLoad this skill for its documented procedure.\n\n" +
		"## Hard Rules\n- Keep the procedure explicit.\n\n" +
		"## Execution Steps\n1. Follow the procedure.\n"
}

func overlayRegistryOf(ids ...string) string {
	var sb strings.Builder
	sb.WriteString("version: \"1\"\nskills:\n")
	for _, id := range ids {
		sb.WriteString("  - id: " + id + "\n    path: " + id + "\n    source:\n      type: custom\n")
		sb.WriteString("    install:\n      defaultScope: global\n      targets:\n        - claude\n")
		sb.WriteString("    lifecycle:\n      updateStrategy: overlay-only\n")
	}
	return sb.String()
}

func overlayManifestOf(ids ...string) string {
	var sb strings.Builder
	for _, id := range ids {
		sb.WriteString(id + "/SKILL.md custom\n")
	}
	return sb.String()
}

// overlayWorld is an overlay in a temporary directory: a registry and a manifest that agree on one
// skill, existing, the source of that skill, approved, and the source of newbie, which is not
// registered and is approved too, so that every verb that writes can run for real.
type overlayWorld struct {
	t                   *testing.T
	dir, reg, man, root string
	lockPath            string
}

func newOverlayWorld(t *testing.T) overlayWorld {
	t.Helper()
	dir := t.TempDir()
	w := overlayWorld{t: t, dir: dir, reg: filepath.Join(dir, "registry.yaml"), man: filepath.Join(dir, "overlay.manifest"), root: filepath.Join(dir, "skills")}
	w.lockPath = skills.RegistryLockPath(w.reg)
	writeTestFile(t, w.reg, overlayRegistryOf("existing"))
	writeTestFile(t, w.man, overlayManifestOf("existing"))
	for _, id := range []string{"existing", "newbie"} {
		w.skill(id, overlaySkillMD(id))
	}
	return w
}

// skill writes the SKILL.md of id and the approval of exactly those bytes.
func (w overlayWorld) skill(id, content string) {
	w.t.Helper()
	writeTestFile(w.t, filepath.Join(w.root, id, "SKILL.md"), content)
	record, err := skills.SerializeApprovalRecord(skills.ApprovalRecord{Skill: id, SHA256: skills.SkillDigest([]byte(content)), ApprovedAt: overlayApprovedAt, Approver: "fixture-reviewer"})
	if err != nil {
		w.t.Fatal(err)
	}
	writeTestFile(w.t, skills.ApprovalRecordPath(w.root, id), string(record))
}

func (w overlayWorld) flags() []string {
	return []string{"--registry", w.reg, "--manifest", w.man, "--source-root", w.root}
}

// readerApprovals is the approval store of a test: the SKILL.md and the record are read from the
// paths the domain names through read, so that a reader that gates or counts sees these too.
type readerApprovals struct{ read func(string) ([]byte, error) }

func (r readerApprovals) ReadSkill(sourceRoot, path string) ([]byte, error) {
	return r.read(skills.SkillMDPath(sourceRoot, path))
}

func (r readerApprovals) ReadRecord(sourceRoot, id string) ([]byte, error) {
	return r.read(skills.ApprovalRecordPath(sourceRoot, id))
}

// deps wires the real adapters over the files of the test, reading through read.
func (w overlayWorld) deps(locker skills.Locker, read func(string) ([]byte, error)) skills.Deps {
	return skills.Deps{
		ReadFile:   read,
		Registries: registryyaml.NewRepository(read),
		Approvals:  readerApprovals{read},
		Project:    skillsfs.Project{},
		Locker:     locker,
		Now:        overlayClock,
	}
}

func (w overlayWorld) run(verb func(skills.Deps, []string, io.Writer, io.Writer, func(int)), deps skills.Deps, args ...string) verbRun {
	w.t.Helper()
	return runSkillsVerb(verb, deps, args...)
}

// registryIDs and manifestIDs are what the files list, sorted.
func (w overlayWorld) registryIDs() []string {
	w.t.Helper()
	reg, err := skills.ReadRegistry(registryyaml.NewRepository(os.ReadFile), w.reg)
	if err != nil {
		w.t.Fatalf("the registry does not read: %v", err)
	}
	var ids []string
	for _, e := range reg.Skills {
		ids = append(ids, e.ID)
	}
	sort.Strings(ids)
	return ids
}

func (w overlayWorld) manifestIDs() []string {
	w.t.Helper()
	data, err := os.ReadFile(w.man)
	if err != nil {
		w.t.Fatal(err)
	}
	var ids []string
	for _, line := range strings.Split(string(data), "\n") {
		if fields := strings.Fields(line); len(fields) > 0 && strings.HasSuffix(fields[0], "/SKILL.md") {
			ids = append(ids, strings.TrimSuffix(fields[0], "/SKILL.md"))
		}
	}
	sort.Strings(ids)
	return ids
}

func (w overlayWorld) snapshot() map[string]string {
	w.t.Helper()
	out := map[string]string{}
	for _, path := range []string{w.reg, w.man, skills.ApprovalRecordPath(w.root, "existing"), skills.ApprovalRecordPath(w.root, "newbie")} {
		data, err := os.ReadFile(path)
		if err != nil {
			w.t.Fatal(err)
		}
		out[path] = string(data)
	}
	return out
}

// An overlay verb with the arguments that make it run for real in the world.
type overlayVerbCase struct {
	name  string
	verb  func(skills.Deps, []string, io.Writer, io.Writer, func(int))
	word  string
	extra []string
}

var overlayVerbCases = []overlayVerbCase{
	{"add", skillsAdd, "add", []string{"newbie"}},
	{"remove", skillsRemove, "remove", []string{"existing"}},
	{"sync-manifest", skillsSync, "sync-manifest", nil},
	{"approve", skillsApprove, "approve", []string{"--id", "existing", "--approver", "reviewer"}},
}

func (c overlayVerbCase) args(w overlayWorld) []string {
	return append(append([]string{c.word}, c.extra...), w.flags()...)
}

// ---- what each verb tells ---------------------------------------------------------------

func TestOverlayVerbsTellWhatTheyDid(t *testing.T) {
	w := newOverlayWorld(t)
	deps := w.deps(noopOverlayLocker{}, os.ReadFile)

	r := w.run(skillsAdd, deps, append([]string{"add", "newbie"}, w.flags()...)...)
	if r.stdout != "added: newbie\n" || r.stderr != "" || !reflect.DeepEqual(r.exits, []int{0}) {
		t.Errorf("add = %q, %q, %v", r.stdout, r.stderr, r.exits)
	}
	if got, want := w.registryIDs(), []string{"existing", "newbie"}; !reflect.DeepEqual(got, want) {
		t.Errorf("the registry lists %v, want %v", got, want)
	}
	if got, want := w.manifestIDs(), []string{"existing", "newbie"}; !reflect.DeepEqual(got, want) {
		t.Errorf("the manifest lists %v, want %v", got, want)
	}

	r = w.run(skillsRemove, deps, append([]string{"remove", "newbie"}, w.flags()...)...)
	if r.stdout != "removed: newbie\n" || r.stderr != "" || !reflect.DeepEqual(r.exits, []int{0}) {
		t.Errorf("remove = %q, %q, %v", r.stdout, r.stderr, r.exits)
	}

	r = w.run(skillsSync, deps, append([]string{"sync-manifest"}, w.flags()...)...)
	if r.stdout != "overlay.manifest already in sync\n" || r.stderr != "" || !reflect.DeepEqual(r.exits, []int{0}) {
		t.Errorf("sync-manifest of a manifest that is right = %q, %q, %v", r.stdout, r.stderr, r.exits)
	}
	writeTestFile(t, w.man, "ghost/SKILL.md custom\n")
	r = w.run(skillsSync, deps, append([]string{"sync-manifest"}, w.flags()...)...)
	if r.stdout != "sync-manifest: 1 added, 1 dropped, 0 retagged\n" || !reflect.DeepEqual(r.exits, []int{0}) {
		t.Errorf("sync-manifest = %q, %v, want the summary of the rows it changed", r.stdout, r.exits)
	}

	r = w.run(skillsApprove, deps, append([]string{"approve", "--id", "newbie", "--approver", "reviewer"}, w.flags()...)...)
	digest := skills.SkillDigest([]byte(overlaySkillMD("newbie")))
	want := "unchanged: newbie\nsha256: " + digest + "\nrecord: " + filepath.ToSlash(skills.ApprovalRecordPath(w.root, "newbie")) + "\n"
	if r.stdout != want || r.stderr != "" || !reflect.DeepEqual(r.exits, []int{0}) {
		t.Errorf("approve = %q, %q, %v, want %q", r.stdout, r.stderr, r.exits, want)
	}
}

// ---- which lock, and when ------------------------------------------------------------

// noopOverlayLocker takes no lock at all, for the tests whose subject is not the lock.
type noopOverlayLocker struct{}

func (noopOverlayLocker) Lock(string, skills.LockMode) (func(), error)    { return func() {}, nil }
func (noopOverlayLocker) LockDir(string, skills.LockMode) (func(), error) { return func() {}, nil }
func (noopOverlayLocker) Exists(path string) error                        { _, err := os.Stat(path); return err }

// holdingLocker records every lock and unlock, in order, says which paths are held right now, and
// can be told to fail.
type holdingLocker struct {
	mu     sync.Mutex
	events []string
	held   map[string]int
	fail   error
	// unseen are the paths the locker says it cannot see, with what it says of each.
	unseen map[string]error
}

func (l *holdingLocker) Lock(path string, mode skills.LockMode) (func(), error) {
	return l.take("lock", path, mode)
}
func (l *holdingLocker) LockDir(dir string, mode skills.LockMode) (func(), error) {
	return l.take("lockdir", dir, mode)
}
func (l *holdingLocker) Exists(path string) error {
	if err := l.unseen[path]; err != nil {
		return err
	}
	_, err := os.Stat(path)
	return err
}

func lockModeName(m skills.LockMode) string {
	if m == skills.LockShared {
		return "shared"
	}
	return "exclusive"
}

func (l *holdingLocker) take(kind, path string, mode skills.LockMode) (func(), error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.fail != nil {
		l.events = append(l.events, fmt.Sprintf("refused %s %s", lockModeName(mode), path))
		return nil, l.fail
	}
	if l.held == nil {
		l.held = map[string]int{}
	}
	l.held[path]++
	l.events = append(l.events, fmt.Sprintf("%s %s %s", kind, lockModeName(mode), path))
	return func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.held[path]--
		l.events = append(l.events, "unlock "+path)
	}, nil
}

func (l *holdingLocker) isHeld(path string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.held[path] > 0
}

func (l *holdingLocker) log() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.events...)
}

func TestOverlayVerbsTakeTheExclusiveLockOfTheRegistryAndLetGoOfIt(t *testing.T) {
	for _, tc := range overlayVerbCases {
		t.Run(tc.name, func(t *testing.T) {
			w := newOverlayWorld(t)
			locker := &holdingLocker{}
			r := w.run(tc.verb, w.deps(locker, os.ReadFile), tc.args(w)...)
			if r.code() != 0 {
				t.Fatalf("exit %d; stderr=%q", r.code(), r.stderr)
			}
			if want := []string{"lock exclusive " + w.lockPath, "unlock " + w.lockPath}; !reflect.DeepEqual(locker.log(), want) {
				t.Errorf("lock events = %v, want %v", locker.log(), want)
			}
		})
	}
}

// The policy of which verb takes which overlay lock is said once, in skills.OverlayLocks, and an
// adapter takes what it says: the locks an adapter asks of the locker are exactly the requests of
// the policy for the same verb and registry, so that a verb whose adapter drifted from the policy
// (approve names a registry it does not read, and takes no registry check for it) is caught.
func TestOverlayVerbsTakeExactlyTheLocksThePolicySaysForThem(t *testing.T) {
	for _, tc := range overlayVerbCases {
		t.Run(tc.name, func(t *testing.T) {
			w := newOverlayWorld(t)
			locker := &holdingLocker{}
			if r := w.run(tc.verb, w.deps(locker, os.ReadFile), tc.args(w)...); r.code() != 0 {
				t.Fatalf("exit %d; stderr=%q", r.code(), r.stderr)
			}
			var want []string
			for _, req := range skills.OverlayLocks(tc.name, w.reg) {
				if req.Dir {
					t.Fatalf("the policy asks a directory lock for %s", tc.name)
				}
				want = append(want, "lock "+lockModeName(req.Mode)+" "+req.Path)
			}
			var got []string
			for _, event := range locker.log() {
				if strings.HasPrefix(event, "lock") {
					got = append(got, event)
				}
			}
			if len(want) == 0 || !reflect.DeepEqual(got, want) {
				t.Errorf("the adapter took %v, the policy of OverlayLocks says %v", got, want)
			}
		})
	}
}

func TestOverlayVerbsLockTheRegistryTheyName(t *testing.T) {
	w := newOverlayWorld(t)
	other := filepath.Join(w.dir, "other", "team.registry.yaml")
	writeTestFile(t, other, overlayRegistryOf("existing"))
	locker := &holdingLocker{}
	w.run(skillsSync, w.deps(locker, os.ReadFile), "sync-manifest", "--registry", other, "--manifest", w.man)
	if got, want := locker.log()[0], "lock exclusive "+filepath.Join(w.dir, "other", ".team.registry.yaml.lock"); got != want {
		t.Errorf("first event = %q, want %q", got, want)
	}
}

// State is read after the lock is taken and before it is let go of, never outside it: a read made
// before it can be stale by the time the lock is granted.
func TestOverlayVerbsReadEverythingUnderTheLock(t *testing.T) {
	for _, tc := range overlayVerbCases {
		t.Run(tc.name, func(t *testing.T) {
			w := newOverlayWorld(t)
			locker := &holdingLocker{}
			var outside []string
			read := func(name string) ([]byte, error) {
				if !locker.isHeld(w.lockPath) {
					outside = append(outside, name)
				}
				return os.ReadFile(name)
			}
			if r := w.run(tc.verb, w.deps(locker, read), tc.args(w)...); r.code() != 0 {
				t.Fatalf("exit %d; stderr=%q", r.code(), r.stderr)
			}
			if len(outside) != 0 {
				t.Errorf("read %v before the lock was taken or after it was let go of", outside)
			}
		})
	}
}

func TestOverlayVerbsLetGoOfTheLockWhenTheyRefuse(t *testing.T) {
	w := newOverlayWorld(t)
	locker := &holdingLocker{}
	r := w.run(skillsRemove, w.deps(locker, os.ReadFile), append([]string{"remove", "never-registered"}, w.flags()...)...)
	if r.code() != 1 {
		t.Fatalf("exit %d, want 1; stderr=%q", r.code(), r.stderr)
	}
	if want := []string{"lock exclusive " + w.lockPath, "unlock " + w.lockPath}; !reflect.DeepEqual(locker.log(), want) {
		t.Errorf("lock events = %v, want %v", locker.log(), want)
	}
}

// busyLock is a lock that stayed taken: the shape engine/filelock's BusyError has.
type busyLock struct{ path string }

func (e busyLock) Error() string { return "lock " + e.path + " is held by another process" }
func (busyLock) Busy() bool      { return true }

// A lock that stays taken past the bound is exit 2 with a retry message, and the verb does not run.
func TestOverlayVerbsExit2WhenTheLockStaysTakenAndChangeNothing(t *testing.T) {
	for _, tc := range overlayVerbCases {
		t.Run(tc.name, func(t *testing.T) {
			w := newOverlayWorld(t)
			before := w.snapshot()
			locker := &holdingLocker{fail: busyLock{w.lockPath}}
			r := w.run(tc.verb, w.deps(locker, os.ReadFile), tc.args(w)...)
			if r.code() != skills.ExitBusy || skills.ExitBusy != 2 || r.stdout != "" {
				t.Errorf("exit %d, stdout %q, want exit 2 and nothing on stdout", r.code(), r.stdout)
			}
			for _, want := range []string{"skills " + tc.name, "in progress", "retry", w.lockPath} {
				if !strings.Contains(r.stderr, want) {
					t.Errorf("stderr %q does not contain %q", r.stderr, want)
				}
			}
			if !reflect.DeepEqual(before, w.snapshot()) {
				t.Error("a busy lock changed files")
			}
			if entries, _ := filepath.Glob(filepath.Join(w.dir, ".tmp-skills-*")); len(entries) != 0 {
				t.Errorf("a busy lock left temp files %v", entries)
			}
		})
	}
}

// A lock that cannot be taken for another reason is a refusal, exit 1, and does not say to retry.
func TestOverlayVerbsExit1WithoutSayingRetryWhenTheLockCannotBeTaken(t *testing.T) {
	w := newOverlayWorld(t)
	before := w.snapshot()
	locker := &holdingLocker{fail: fmt.Errorf("open %s: permission denied", w.lockPath)}
	r := w.run(skillsAdd, w.deps(locker, os.ReadFile), append([]string{"add", "newbie"}, w.flags()...)...)
	if r.code() != 1 || r.stdout != "" {
		t.Fatalf("exit %d, stdout %q, want exit 1 and nothing on stdout", r.code(), r.stdout)
	}
	for _, want := range []string{"skills add", "cannot take the lock", "permission denied", w.lockPath} {
		if !strings.Contains(r.stderr, want) {
			t.Errorf("stderr %q does not contain %q", r.stderr, want)
		}
	}
	if strings.Contains(r.stderr, "retry") {
		t.Errorf("stderr %q tells the caller to retry a failure that will not clear", r.stderr)
	}
	if !reflect.DeepEqual(before, w.snapshot()) {
		t.Error("a failed lock changed files")
	}
}

func TestOverlayVerbsRecogniseABusyErrorThatWasWrapped(t *testing.T) {
	w := newOverlayWorld(t)
	locker := &holdingLocker{fail: fmt.Errorf("acquire: %w", busyLock{w.lockPath})}
	if r := w.run(skillsSync, w.deps(locker, os.ReadFile), append([]string{"sync-manifest"}, w.flags()...)...); r.code() != skills.ExitBusy {
		t.Errorf("exit %d for a wrapped busy error, want %d; stderr=%q", r.code(), skills.ExitBusy, r.stderr)
	}
}

// Without a locker the verbs refuse rather than run unserialized.
func TestOverlayVerbsFailClosedWithoutALocker(t *testing.T) {
	for _, tc := range overlayVerbCases {
		t.Run(tc.name, func(t *testing.T) {
			w := newOverlayWorld(t)
			before := w.snapshot()
			r := w.run(tc.verb, w.deps(nil, os.ReadFile), tc.args(w)...)
			if r.code() != 1 || r.stdout != "" || !strings.Contains(r.stderr, "no lock is configured") {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 1 and a 'no lock is configured' refusal", r.code(), r.stdout, r.stderr)
			}
			if !reflect.DeepEqual(before, w.snapshot()) {
				t.Error("a verb ran without a lock")
			}
		})
	}
}

// A command line that is refused is refused before any lock is asked for and before anything is
// read: a mistyped flag never waits for another command, and never reads the overlay.
func TestOverlayVerbsRefuseWhatTheyDoNotKnowBeforeTakingAnyLock(t *testing.T) {
	for _, tc := range overlayVerbCases {
		t.Run(tc.name, func(t *testing.T) {
			w := newOverlayWorld(t)
			locker := &holdingLocker{}
			var reads []string
			read := func(name string) ([]byte, error) { reads = append(reads, name); return os.ReadFile(name) }
			r := w.run(tc.verb, w.deps(locker, read), append(tc.args(w), "--frobnicate")...)
			if want := fmt.Sprintf("error: skills %s: unknown flag %q\n", tc.name, "--frobnicate"); r.stderr != want || r.code() != 1 || r.stdout != "" {
				t.Errorf("= %q, %q, exit %d, want %q and exit 1", r.stdout, r.stderr, r.code(), want)
			}
			if len(locker.log()) != 0 || len(reads) != 0 {
				t.Errorf("a refused command line took locks %v and read %v", locker.log(), reads)
			}
		})
	}
}

// ---- what must be wired -------------------------------------------------------------

// A composition root that forgot a port gets a refusal, not a crash, and only after the lock: the
// refusal is the verb's, which runs under it.
func TestOverlayVerbsRefuseWhenAPortIsNotWired(t *testing.T) {
	for _, tc := range overlayVerbCases {
		t.Run(tc.name+" with no project file system", func(t *testing.T) {
			w := newOverlayWorld(t)
			deps := w.deps(noopOverlayLocker{}, os.ReadFile)
			deps.Project = nil
			r := w.run(tc.verb, deps, tc.args(w)...)
			want := fmt.Sprintf("error: skills %s: no project file system is wired, so it cannot read or write files\n", tc.name)
			if r.stderr != want || r.code() != 1 {
				t.Errorf("stderr %q, exit %d, want %q and exit 1", r.stderr, r.code(), want)
			}
		})
	}
	for _, tc := range overlayVerbCases {
		if tc.name != "add" && tc.name != "approve" {
			continue
		}
		t.Run(tc.name+" with no approval record store", func(t *testing.T) {
			w := newOverlayWorld(t)
			deps := w.deps(noopOverlayLocker{}, os.ReadFile)
			deps.Approvals = nil
			r := w.run(tc.verb, deps, tc.args(w)...)
			want := fmt.Sprintf("error: skills %s: no approval record store is wired, so it cannot tell whether a skill is approved\n", tc.name)
			if r.stderr != want || r.code() != 1 {
				t.Errorf("stderr %q, exit %d, want %q and exit 1", r.stderr, r.code(), want)
			}
		})
	}
}

// ---- the words of a refusal --------------------------------------------------------------

func TestOverlayVerbsTellWhatTheReaderLeftOutBeforeTheyRefuse(t *testing.T) {
	w := newOverlayWorld(t)
	writeTestFile(t, w.reg, overlayRegistryOf("existing")+"unknownTopLevel: true\n")
	r := w.run(skillsAdd, w.deps(noopOverlayLocker{}, os.ReadFile), append([]string{"add", "newbie"}, w.flags()...)...)
	lines := strings.Split(strings.TrimSpace(r.stderr), "\n")
	if r.code() != 1 || len(lines) != 2 || !strings.HasPrefix(lines[0], "warning: ") || !strings.Contains(lines[1], "the registry has fields this program does not read") {
		t.Errorf("exit %d, stderr %q, want the warning and then the refusal", r.code(), r.stderr)
	}
}

func TestOverlayVerbsTellARegistryTheyCannotUseInTheWordsOfTheRegistry(t *testing.T) {
	w := newOverlayWorld(t)
	deps := w.deps(noopOverlayLocker{}, os.ReadFile)
	// A registry that is there and cannot be read: a directory stands for it.
	unreadable := filepath.Join(w.dir, "unreadable.yaml")
	if err := os.Mkdir(unreadable, 0o755); err != nil {
		t.Fatal(err)
	}
	r := w.run(skillsRemove, deps, "remove", "existing", "--registry", unreadable, "--manifest", w.man)
	if want := fmt.Sprintf("error: reading registry %q: ", unreadable); !strings.HasPrefix(r.stderr, want) || r.code() != 1 {
		t.Errorf("stderr %q, exit %d, want it to begin %q", r.stderr, r.code(), want)
	}
	writeTestFile(t, w.reg, "version: \"99\"\nskills: []\n")
	r = w.run(skillsRemove, deps, append([]string{"remove", "existing"}, w.flags()...)...)
	if !strings.HasPrefix(r.stderr, "error: parsing registry: ") || r.code() != 1 {
		t.Errorf("stderr %q, exit %d, want a registry that is not usable told as such", r.stderr, r.code())
	}
}

func TestAddTellsEachHardFindingOfTheLintOnItsOwnLineAndWritesNothing(t *testing.T) {
	w := newOverlayWorld(t)
	w.skill("newbie", strings.Replace(overlaySkillMD("newbie"), "  version: \"1.0\"\n", "", 1))
	before := w.snapshot()
	r := w.run(skillsAdd, w.deps(noopOverlayLocker{}, os.ReadFile), append([]string{"add", "newbie"}, w.flags()...)...)
	if r.code() != 1 || r.stdout != "" || !strings.HasPrefix(r.stderr, "[lint:required-fields]") || strings.Contains(r.stderr, "error:") {
		t.Errorf("exit %d, stdout %q, stderr %q, want the findings as the lint prints them", r.code(), r.stdout, r.stderr)
	}
	if !reflect.DeepEqual(before, w.snapshot()) {
		t.Error("a refused add changed files")
	}
}

func TestAddTellsEachDivergenceTheWriteWouldLeaveAndWritesNothing(t *testing.T) {
	w := newOverlayWorld(t)
	writeTestFile(t, w.man, overlayManifestOf("existing")+"ghost/SKILL.md custom\n")
	before := w.snapshot()
	r := w.run(skillsAdd, w.deps(noopOverlayLocker{}, os.ReadFile), append([]string{"add", "newbie"}, w.flags()...)...)
	if r.code() != 1 || !strings.HasPrefix(r.stderr, "[MISSING_IN_REGISTRY] ghost: ") {
		t.Errorf("exit %d, stderr %q, want the divergence in the words of validate", r.code(), r.stderr)
	}
	if !reflect.DeepEqual(before, w.snapshot()) {
		t.Error("a refused add changed files")
	}
}

// ---- the interleavings the lock exists to prevent ------------------------------------------

// exclusionLocker is a real in-process lock: one exclusive holder or any number of shared ones, per
// path. blocked runs when a Lock call cannot be granted at once, which is how an interleaving test
// learns that the other side is waiting.
type exclusionLocker struct {
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

func (l *exclusionLocker) Exists(path string) error { _, err := os.Stat(path); return err }

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

func (l *exclusionLocker) Lock(path string, mode skills.LockMode) (func(), error) {
	return l.take(path, path, mode)
}

// LockDir locks a directory. A directory and a file that happen to share a path are different
// locks in the tests as little as they are in the kernel, where they cannot share a path at all.
func (l *exclusionLocker) LockDir(dir string, mode skills.LockMode) (func(), error) {
	return l.take("dir:"+dir, dir, mode)
}

func (l *exclusionLocker) take(key, path string, mode skills.LockMode) (func(), error) {
	rw := l.rw(key)
	name := lockModeName(mode) + " " + filepath.Base(path)
	if mode == skills.LockShared {
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

// readGate forces the interleaving a missing lock allows. The first read of its path parks after
// the bytes are read, so the reader holds a state that is about to go stale, until either a second
// reader arrives (nothing serialized them) or the locker reports that somebody is waiting for the
// first one's lock (something did). The timeout only bounds a hung test.
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

// concurrently runs every job at once and returns what each left, in order.
func concurrently(jobs ...func() verbRun) []verbRun {
	results := make([]verbRun, len(jobs))
	var wg sync.WaitGroup
	for i, job := range jobs {
		wg.Add(1)
		go func(i int, job func() verbRun) {
			defer wg.Done()
			results[i] = job()
		}(i, job)
	}
	wg.Wait()
	return results
}

// Two adds that both read the registry before either wrote used to leave only one of them
// registered although both exited 0. The gate parks the first reader after it has read, so the
// second cannot help reading the same registry unless the lock keeps it out.
func TestConcurrentAddsBothLandInTheRegistryAndTheManifest(t *testing.T) {
	w := newOverlayWorld(t)
	w.skill("alpha", overlaySkillMD("alpha"))
	w.skill("beta", overlaySkillMD("beta"))
	gate := newReadGate(t, w.reg)
	deps := w.deps(&exclusionLocker{blocked: gate.release}, gate.readFile)
	add := func(id string) func() verbRun {
		return func() verbRun { return runSkillsVerb(skillsAdd, deps, append([]string{"add", id}, w.flags()...)...) }
	}
	for i, r := range concurrently(add("alpha"), add("beta")) {
		if r.code() != 0 {
			t.Errorf("add #%d: exit %d, stderr=%q", i, r.code(), r.stderr)
		}
	}
	want := []string{"alpha", "beta", "existing"}
	if got := w.registryIDs(); !reflect.DeepEqual(got, want) {
		t.Errorf("registry lists %v, want %v: an add that exited 0 was lost", got, want)
	}
	if got := w.manifestIDs(); !reflect.DeepEqual(got, want) {
		t.Errorf("manifest lists %v, want %v", got, want)
	}
}

func TestConcurrentRemovesBothLeaveTheRegistryAndTheManifest(t *testing.T) {
	w := newOverlayWorld(t)
	writeTestFile(t, w.reg, overlayRegistryOf("keep", "one", "two"))
	writeTestFile(t, w.man, overlayManifestOf("keep", "one", "two"))
	gate := newReadGate(t, w.reg)
	deps := w.deps(&exclusionLocker{blocked: gate.release}, gate.readFile)
	remove := func(id string) func() verbRun {
		return func() verbRun {
			return runSkillsVerb(skillsRemove, deps, append([]string{"remove", id}, w.flags()...)...)
		}
	}
	for i, r := range concurrently(remove("one"), remove("two")) {
		if r.code() != 0 {
			t.Errorf("remove #%d: exit %d, stderr=%q", i, r.code(), r.stderr)
		}
	}
	want := []string{"keep"}
	if got := w.registryIDs(); !reflect.DeepEqual(got, want) {
		t.Errorf("registry lists %v, want %v: a remove that exited 0 was undone", got, want)
	}
	if got := w.manifestIDs(); !reflect.DeepEqual(got, want) {
		t.Errorf("manifest lists %v, want %v", got, want)
	}
}

// sync-manifest regenerates the manifest from the registry it read; an add that lands between that
// read and its write must not be erased from the manifest. The sync parks after reading the
// registry, and the add runs: without a lock it completes and the sync is released afterwards to
// write from the registry it read; with one, the add has to wait for the sync, which releases it.
func TestSyncManifestDoesNotEraseAConcurrentAdd(t *testing.T) {
	w := newOverlayWorld(t)
	w.skill("alpha", overlaySkillMD("alpha"))
	gate := newReadGate(t, w.reg)
	deps := w.deps(&exclusionLocker{blocked: gate.release}, gate.readFile)

	syncDone := make(chan verbRun, 1)
	go func() {
		syncDone <- runSkillsVerb(skillsSync, deps, append([]string{"sync-manifest"}, w.flags()...)...)
	}()
	<-gate.arrived
	added := runSkillsVerb(skillsAdd, w.deps(deps.Locker, os.ReadFile), append([]string{"add", "alpha"}, w.flags()...)...)
	gate.release()
	synced := <-syncDone

	for name, r := range map[string]verbRun{"add": added, "sync-manifest": synced} {
		if r.code() != 0 {
			t.Errorf("%s: exit %d, stderr=%q", name, r.code(), r.stderr)
		}
	}
	reg, man := w.registryIDs(), w.manifestIDs()
	if !reflect.DeepEqual(reg, man) {
		t.Errorf("registry lists %v but the manifest lists %v", reg, man)
	}
	if want := []string{"alpha", "existing"}; !reflect.DeepEqual(reg, want) {
		t.Errorf("registry lists %v, want %v", reg, want)
	}
}

// Two approvals of the same bytes: the second must find the first one's record valid and leave it
// alone, so the original approver stays the record of who approved these bytes.
func TestConcurrentApprovalsOfTheSameBytesKeepTheFirstApprover(t *testing.T) {
	w := newOverlayWorld(t)
	if err := os.Remove(skills.ApprovalRecordPath(w.root, "newbie")); err != nil {
		t.Fatal(err)
	}
	gate := newReadGate(t, skills.ApprovalRecordPath(w.root, "newbie"))
	deps := w.deps(&exclusionLocker{blocked: gate.release}, gate.readFile)
	approve := func(approver string) func() verbRun {
		return func() verbRun {
			return runSkillsVerb(skillsApprove, deps, append([]string{"approve", "--id", "newbie", "--approver", approver}, w.flags()...)...)
		}
	}
	results := concurrently(approve("alice"), approve("bob"))

	approvers := []string{"alice", "bob"}
	winner, unchanged := "", 0
	for i, r := range results {
		if r.code() != 0 {
			t.Errorf("approve by %s: exit %d, stderr=%q", approvers[i], r.code(), r.stderr)
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
	data, err := os.ReadFile(skills.ApprovalRecordPath(w.root, "newbie"))
	if err != nil {
		t.Fatal(err)
	}
	rec, err := skills.ParseApprovalRecord(data)
	if err != nil || rec.Approver != winner {
		t.Errorf("the record names %q (%v), want the approver whose approval was reported, %q", rec.Approver, err, winner)
	}
}

var _ = bytes.NewBuffer
