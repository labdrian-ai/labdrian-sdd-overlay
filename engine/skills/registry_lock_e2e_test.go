package skills

// The overlay lock against the BUILT engine binary: separate OS processes,
// started at the same moment, in fixtures under t.TempDir(). The in-process tests
// in the lock tests force each interleaving one at a time; these show that real
// processes cannot reach any of them. They also show the busy path as a user sees
// it (exit 2 after the bound, not a hang) and that a read-only verb leaves the real
// repository tree exactly as it found it.
//
// Every child process gets an explicit environment: a HOME and XDG_STATE_HOME of
// its own and a minimal PATH, so nothing here can reach ~/.claude, ~/.pi, ~/.codex,
// the workflow state home, or Engram. Nothing touches the repository's own skills,
// registry, or manifest except a read-only `validate`.
//
// SKILLS_E2E_ENGINE_BINARY names a prebuilt engine binary to test instead of the
// current source, which is how the numbers of an older build (before the lock) are
// measured with this same test. It is read only by these tests.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/filelock"
)

func lockE2EBinary(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("builds and runs the engine binary; skipped in -short mode")
	}
	if path := os.Getenv("SKILLS_E2E_ENGINE_BINARY"); path != "" {
		return path
	}
	return buildRetireEngineBinary(t)
}

type lockE2E struct {
	t                        *testing.T
	bin, dir, reg, man, root string
	home, state              string
	lockPath                 string
}

type e2eRun struct {
	code           int
	stdout, stderr string
	elapsed        time.Duration
}

// newLockE2E builds an overlay: a registry listing registered, the manifest, and a
// skill tree with every registered and every extra skill on disk and approved.
func newLockE2E(t *testing.T, bin string, registered, extra []string) lockE2E {
	t.Helper()
	dir := t.TempDir()
	reg, man, root := setupFixture(t, dir, minimalRegistry(registered...), minimalManifest(registered...), append(append([]string{}, registered...), extra...))
	w := lockE2E{t: t, bin: bin, dir: dir, reg: reg, man: man, root: root, home: t.TempDir(), state: t.TempDir(), lockPath: RegistryLockPath(reg)}
	return w
}

func (w lockE2E) flags() []string {
	return []string{"--registry", w.reg, "--manifest", w.man, "--source-root", w.root}
}

// engine runs the built binary as 'skills <args>' from dir and waits for it, at
// most a minute.
func (w lockE2E) engineIn(dir string, args ...string) e2eRun {
	w.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, w.bin, append([]string{"skills"}, args...)...)
	cmd.Dir = dir
	cmd.Env = []string{"HOME=" + w.home, "XDG_STATE_HOME=" + w.state, "PATH=/usr/bin:/bin"}
	var out, errBuf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errBuf
	start := time.Now()
	code := 0
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			w.t.Fatalf("run %s %v: %v", w.bin, args, err)
		}
		code = exitErr.ExitCode()
	}
	return e2eRun{code, out.String(), errBuf.String(), time.Since(start)}
}

func (w lockE2E) engine(args ...string) e2eRun { return w.engineIn(w.dir, args...) }

// registryIDs are the ids of the registry at regPath, sorted, as the built engine reads them: it
// asks `skills list`, which decodes the file with the registry reader of the program. This package
// cannot import that reader (the adapter imports this package), and a scan of the lines of the file
// would answer wrongly, or nothing, for a registry written with another indentation, quoted ids or
// comments.
func registryIDs(t *testing.T, bin, regPath string) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "skills", "list", "--registry", regPath)
	cmd.Dir = filepath.Dir(regPath)
	cmd.Env = []string{"HOME=" + t.TempDir(), "XDG_STATE_HOME=" + t.TempDir(), "PATH=/usr/bin:/bin"}
	var out, errBuf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errBuf
	if err := cmd.Run(); err != nil {
		t.Fatalf("skills list --registry %s: %v\n%s", regPath, err, errBuf.String())
	}
	var ids []string
	for _, line := range strings.Split(strings.TrimRight(out.String(), "\n"), "\n") {
		if id, _, ok := strings.Cut(line, "\t"); ok {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// runAll starts every job at the same moment and waits for all of them.
func runAll(jobs []func() e2eRun) []e2eRun {
	results := make([]e2eRun, len(jobs))
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i, job := range jobs {
		wg.Add(1)
		go func(i int, job func() e2eRun) {
			defer wg.Done()
			<-start
			results[i] = job()
		}(i, job)
	}
	close(start)
	wg.Wait()
	return results
}

// tornDiagnostics counts the diagnostics that say the registry and the manifest
// disagree, which a run only prints if it saw the pair between its two renames.
func tornDiagnostics(stderr string) int {
	return strings.Count(stderr, "MISSING_IN_MANIFEST") + strings.Count(stderr, "MISSING_IN_REGISTRY")
}

// Many processes at once, every verb that changes the registry among them: no add
// or remove that exited 0 is lost, the registry and the manifest agree after every
// round, and no validate ever saw the pair torn. Exit 2 (busy) is allowed and
// counted: it is a refusal that changed nothing, not a loss.
func TestRegistryLockE2E_ConcurrentVerbsNeverLoseAnUpdate(t *testing.T) {
	bin := lockE2EBinary(t)
	const rounds = 15

	var lostAdds, undoneRemoves, disagreements, torn, busy, other, addsOK, removesOK int
	var samples []string
	for round := 0; round < rounds; round++ {
		newIDs := []string{"new-0", "new-1", "new-2", "new-3", "new-4", "new-5", "new-6", "new-7"}
		oldIDs := []string{"old-0", "old-1", "old-2"}
		w := newLockE2E(t, bin, append([]string{"base"}, oldIDs...), newIDs)

		type job struct {
			kind, id string
			run      func() e2eRun
		}
		var jobs []job
		add := func(kind, id string, args ...string) {
			jobs = append(jobs, job{kind, id, func() e2eRun { return w.engine(append(args, w.flags()...)...) }})
		}
		for _, id := range newIDs {
			add("add", id, "add", id)
		}
		for _, id := range oldIDs {
			add("remove", id, "remove", id)
		}
		for i := 0; i < 2; i++ {
			add("sync", "", "sync-manifest")
			add("approve", newIDs[i], "approve", "--id", newIDs[i], "--approver", "stress")
		}
		for i := 0; i < 4; i++ {
			add("validate", "", "validate")
		}
		runs := make([]func() e2eRun, len(jobs))
		for i, j := range jobs {
			runs[i] = j.run
		}
		results := runAll(runs)

		regIDs, manIDs := registryIDs(t, w.bin, w.reg), manifestIDs(t, w.man)
		if !reflect.DeepEqual(regIDs, manIDs) {
			disagreements++
		}
		present := map[string]bool{}
		for _, id := range regIDs {
			present[id] = true
		}
		for i, r := range results {
			j := jobs[i]
			torn += tornDiagnostics(r.stderr)
			switch {
			case r.code == ExitBusy:
				busy++
			case r.code == 0 && j.kind == "add":
				addsOK++
				if !present[j.id] {
					lostAdds++
				}
			case r.code == 0 && j.kind == "remove":
				removesOK++
				if present[j.id] {
					undoneRemoves++
				}
			case r.code != 0 && j.kind == "validate":
				// Red for a skill on disk that is not registered yet: expected while
				// adds are in flight. Only a torn pair (counted above) is a defect.
			case r.code != 0:
				other++
				if len(samples) < 4 {
					samples = append(samples, j.kind+": "+strings.TrimSpace(r.stderr))
				}
			}
		}
	}

	summary := fmt.Sprintf("%d rounds of 19 processes (8 add, 3 remove, 2 sync-manifest, 2 approve, 4 validate): adds ok %d, removes ok %d, busy %d; "+
		"ok adds missing from the registry %d, ok removes still registered %d, rounds with registry != manifest %d, validate runs that saw a torn pair %d, other failures %d",
		rounds, addsOK, removesOK, busy, lostAdds, undoneRemoves, disagreements, torn, other)
	t.Log(summary)
	if lostAdds+undoneRemoves+disagreements+torn+other != 0 {
		t.Errorf("concurrent skills verbs were not serialized: %s\nfailure samples: %v", summary, samples)
	}
	if addsOK+removesOK == 0 {
		t.Errorf("no add or remove succeeded at all: %s", summary)
	}
}

// Many approvals of the same bytes at once: exactly one writes the record and
// reports approved; every other finds it valid and reports unchanged, and the
// record names the approver whose approval was reported.
func TestRegistryLockE2E_ConcurrentApprovalsKeepTheFirstApprover(t *testing.T) {
	bin := lockE2EBinary(t)
	for round := 0; round < 6; round++ {
		w := newLockE2E(t, bin, []string{"base"}, nil)
		writeTestFile(t, filepath.Join(w.root, "solo", "SKILL.md"), lintCleanSkillMD("solo"))

		approvers := []string{"a0", "a1", "a2", "a3", "a4", "a5"}
		// The closure takes the approver as a parameter: this module builds with
		// go 1.21 semantics, where a loop variable is shared by every iteration.
		approve := func(who string) func() e2eRun {
			return func() e2eRun {
				return w.engine(append([]string{"approve", "--id", "solo", "--approver", who}, w.flags()...)...)
			}
		}
		jobs := make([]func() e2eRun, len(approvers))
		for i := range approvers {
			jobs[i] = approve(approvers[i])
		}
		results := runAll(jobs)

		winner, unchanged := "", 0
		for i, r := range results {
			switch {
			case r.code != 0:
				t.Errorf("round %d: approve by %s: exit %d, stderr %q", round, approvers[i], r.code, r.stderr)
			case strings.HasPrefix(r.stdout, "approved: "):
				if winner != "" {
					t.Errorf("round %d: %s and %s both reported approved", round, winner, approvers[i])
				}
				winner = approvers[i]
			case strings.HasPrefix(r.stdout, "unchanged: "):
				unchanged++
			default:
				t.Errorf("round %d: approve by %s printed %q", round, approvers[i], r.stdout)
			}
		}
		if winner == "" || unchanged != len(approvers)-1 {
			t.Errorf("round %d: winner %q, unchanged %d, want one approved and %d unchanged", round, winner, unchanged, len(approvers)-1)
			continue
		}
		if got := recordApprover(t, ApprovalRecordPath(w.root, "solo")); got != winner {
			t.Errorf("round %d: the record names %q, want %q, whose approval was reported", round, got, winner)
		}
	}
}

// A lock held by another process is reported, not waited on forever: exit 2 after
// about the bound, a retry message, and nothing changed. A reader is refused only
// by a writer.
func TestRegistryLockE2E_ABusyLockIsReportedAsExit2AfterTheBound(t *testing.T) {
	bin := lockE2EBinary(t)
	w := newLockE2E(t, bin, []string{"base"}, nil)
	before := snapshotFiles(t, w.reg, w.man)

	hold, err := filelock.Acquire(w.lockPath, filelock.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"sync-manifest"}, {"validate"}} {
		r := w.engine(append(append([]string{}, args...), w.flags()...)...)
		if r.code != ExitBusy || r.stdout != "" {
			t.Errorf("%v while the lock is held: exit %d, stdout %q, want exit 2 and no stdout; stderr %q", args, r.code, r.stdout, r.stderr)
		}
		for _, want := range []string{"in progress", "retry", filepath.Base(w.lockPath)} {
			if !strings.Contains(r.stderr, want) {
				t.Errorf("%v: stderr %q does not contain %q", args, r.stderr, want)
			}
		}
		if r.elapsed < filelock.DefaultWait-200*time.Millisecond || r.elapsed > 20*time.Second {
			t.Errorf("%v took %v, want about the %v bound: neither an instant refusal nor a hang", args, r.elapsed, filelock.DefaultWait)
		}
	}
	if after := snapshotFiles(t, w.reg, w.man); !reflect.DeepEqual(before, after) {
		t.Errorf("a busy verb changed files:\nbefore %v\nafter  %v", before, after)
	}
	hold()

	// Released, the same verb runs; and a reader is not refused by another reader.
	reader, err := filelock.Acquire(w.lockPath, filelock.Options{Mode: filelock.Shared})
	if err != nil {
		t.Fatal(err)
	}
	defer reader()
	if r := w.engine(append([]string{"validate"}, w.flags()...)...); r.code != 0 {
		t.Errorf("validate while only a reader holds the lock: exit %d, stderr %q", r.code, r.stderr)
	}
}

// install reads the overlay and writes the project: it takes the shared lock, so
// it works on an overlay it cannot write to and creates no file in it.
func TestRegistryLockE2E_InstallWorksOnAReadOnlyOverlayAndCreatesNoFileThere(t *testing.T) {
	bin := lockE2EBinary(t)
	f := newInstallFixture(t)
	w := lockE2E{t: t, bin: bin, dir: f.dir, reg: f.reg, man: f.man, root: f.root, home: t.TempDir(), state: t.TempDir(), lockPath: RegistryLockPath(f.reg)}
	if err := os.RemoveAll(w.lockPath); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{f.dir} {
		if err := os.Chmod(dir, 0o555); err != nil {
			t.Fatal(err)
		}
		d := dir
		t.Cleanup(func() { _ = os.Chmod(d, 0o755) })
	}
	if probe, err := os.Create(filepath.Join(f.dir, ".probe")); err == nil {
		probe.Close()
		t.Skip("directory permissions are not enforced for this process (root or an equivalent); the read-only overlay cannot be simulated")
	}

	r := w.engineIn(f.project, "install", "--registry", f.reg, "--source-root", f.root, "--project-id", "p")
	if r.code != 0 || !strings.Contains(r.stdout, "installed: proj") {
		t.Fatalf("install on a read-only overlay: exit %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}
	if _, err := os.Stat(filepath.Join(f.project, ".claude", "skills", "proj", "SKILL.md")); err != nil {
		t.Errorf("the skill was not installed: %v", err)
	}
	if _, err := os.Lstat(w.lockPath); err == nil {
		t.Errorf("install created %s", w.lockPath)
	}
}

// The repository's own tree: a read-only verb leaves it exactly as it found it.
// validate takes the shared lock, which never creates the lock file, so running it
// against the real registry, manifest, and skills does not add an untracked file
// (or anything else) to a checkout.
func TestRegistryLockE2E_ValidateLeavesTheRealTreeUntouched(t *testing.T) {
	bin := lockE2EBinary(t)
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	reg, man, root := filepath.Join(repo, "skills.registry.yaml"), filepath.Join(repo, "overlay.manifest"), filepath.Join(repo, "skills")
	for _, p := range []string{reg, man, root} {
		if _, err := os.Stat(p); err != nil {
			t.Skipf("the real tree is not here (%v)", err)
		}
	}
	dirs := []string{repo, root}
	before := treeListing(t, dirs...)

	w := lockE2E{t: t, bin: bin, dir: t.TempDir(), home: t.TempDir(), state: t.TempDir()}
	r := w.engine("validate", "--registry", reg, "--manifest", man, "--source-root", root)
	if r.code != 0 {
		t.Fatalf("validate on the real tree: exit %d, stderr %q", r.code, r.stderr)
	}
	if after := treeListing(t, dirs...); !reflect.DeepEqual(before, after) {
		t.Errorf("validate changed the real tree:\nbefore %v\nafter  %v", before, after)
	}
}

// treeListing describes each directory's entries (name, size, modification time),
// so that a created, removed, or rewritten entry shows.
func treeListing(t *testing.T, dirs ...string) []string {
	t.Helper()
	var out []string
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			info, err := e.Info()
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, fmt.Sprintf("%s/%s %d %d", dir, e.Name(), info.Size(), info.ModTime().UnixNano()))
		}
	}
	sort.Strings(out)
	return out
}

// recordApprover is who the approval record at path names.
func recordApprover(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read record %q: %v", path, err)
	}
	rec, err := ParseApprovalRecord(data)
	if err != nil {
		t.Fatalf("record on disk does not parse: %v\n%s", err, data)
	}
	return rec.Approver
}
