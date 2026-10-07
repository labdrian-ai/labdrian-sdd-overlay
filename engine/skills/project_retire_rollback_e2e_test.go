package skills

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// This file closes the acceptance gap recorded in the archived change
// 2026-09-21-procedural-memory-lifecycle: the retirement rollback-of-rollback
// diagnostic branch (ExecuteProjectRetirePlan's `error: rollback incomplete:
// <rel-path>`) was covered only by unit tests that
// inject failures through the fakeProjectFS fake
// (TestExecuteProjectRetireRollbackFailureReportsRelativePath), never through
// the real `engine skills project-retire` binary against a real filesystem.
// The tests below build that binary and drive it directly.
//
// Both tests use real directory permissions (chmod 0555, no write bit) as
// the fault, because the ordering of ExecuteProjectRetirePlan
// makes a decoupled "delete fails but its own
// restore later succeeds" fault impossible to construct with permissions
// alone in one synchronous CLI run:
//
//   - the lock's initial WriteTemp (staging, before any delete) and its
//     final Rename (commit, after every delete) write to the exact same
//     directory, so a static permission that blocks the commit also blocks
//     the initial stage -- no deletes are ever attempted (see
//     TestProjectRetireRealCLI_RollbackSucceedsWhenLockStagingIsDenied);
//   - rollback's restore() retries the exact
//     same (WriteTemp, Rename) pair, in the exact same directory, that the
//     forward delete just used. Whatever static condition made a target's
//     Remove fail will therefore also make that target's own restore fail
//     (see TestProjectRetireRealCLI_RollbackOfRollbackReportsIncompletePath).
//
// No env-var or hook seam exists in production for injecting a transient
// fault (the real CLI is wired to the real file system adapter), and
// adding one would be a production-only backdoor, so these two real-fs
// shapes are the complete, honest coverage available: a fault before any
// mutation (full rollback, nothing to restore, tree untouched) and a fault
// whose own restore attempt fails (rollback-of-rollback, reported and
// exited non-zero) alongside a sibling target that WAS deleted and IS
// correctly restored.
//
// A read-only bind mount would decouple delete-fails-but-restore-succeeds
// for a single target, but is not available in this environment.

// retireE2EBuildTimeout and retireE2ERunTimeout bound every child process
// these tests spawn, so a stalled build or a CLI that hangs on an induced
// filesystem fault fails the test instead of blocking the whole package run.
const (
	retireE2EBuildTimeout = 5 * time.Minute
	retireE2ERunTimeout   = time.Minute
)

// retireE2EBinary holds the engine binary shared by every test in this file:
// it is built at most once per package run (lazily, so a -run filter that
// skips these tests never compiles it) and removed by TestMain.
var retireE2EBinary struct {
	once sync.Once
	dir  string
	path string
	err  error
	out  []byte
}

// The shared engine binary is removed by the TestMain of this package, which
// also isolates HOME and the XDG state and config directories for the package.

// buildRetireEngineBinary compiles the real engine binary once per package
// run into an isolated temp directory, the same way
// the shell tests build it for their own e2e runs.
func buildRetireEngineBinary(t *testing.T) string {
	t.Helper()
	retireE2EBinary.once.Do(func() {
		engineRoot, err := filepath.Abs("..")
		if err != nil {
			retireE2EBinary.err = err
			return
		}
		dir, err := os.MkdirTemp("", "engine-retire-e2e-")
		if err != nil {
			retireE2EBinary.err = err
			return
		}
		retireE2EBinary.dir = dir
		binPath := filepath.Join(dir, "engine-retire-e2e")
		ctx, cancel := context.WithTimeout(context.Background(), retireE2EBuildTimeout)
		defer cancel()
		build := exec.CommandContext(ctx, "go", "build", "-o", binPath, "./cmd")
		build.Dir = engineRoot
		retireE2EBinary.out, retireE2EBinary.err = build.CombinedOutput()
		retireE2EBinary.path = binPath
	})
	if retireE2EBinary.err != nil {
		t.Fatalf("build engine binary: %v\n%s", retireE2EBinary.err, retireE2EBinary.out)
	}
	return retireE2EBinary.path
}

// denyWrites removes the write bit from dir for the rest of the test and
// proves the fault is real: if this process can still create a file there
// (root, or an equivalent capability, or a filesystem that ignores mode
// bits), the fault cannot be injected and the test is skipped with that
// reason instead of failing on a confusing exit-code mismatch.
func denyWrites(t *testing.T, dir string) {
	t.Helper()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("deny write on %q: %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	probe := filepath.Join(dir, ".write-probe")
	if f, err := os.Create(probe); err == nil {
		_ = f.Close()
		_ = os.Remove(probe)
		t.Skipf("chmod 0555 on %q is not enforced for this process (root or an equivalent capability); the permission fault cannot be injected", dir)
	}
}

// retireE2EEnv is one isolated project root plus everything
// project-register/project-retire need: a lint-clean draft outside the root,
// a readable registry that never matches the id under test, and an isolated
// HOME/state dir so nothing here can touch the real user's $HOME, ~/.claude,
// or ~/.pi (engine tests have previously run real `pi remove` against live
// state when this guard was missing).
type retireE2EEnv struct {
	binPath      string
	home         string
	root         string
	registryPath string
	draftPath    string
}

func newRetireE2EEnv(t *testing.T, id string) retireE2EEnv {
	t.Helper()
	binPath := buildRetireEngineBinary(t)
	base := t.TempDir()
	home := filepath.Join(base, "home")
	root := filepath.Join(base, "project")
	draftDir := filepath.Join(base, "drafts")
	for _, dir := range []string{home, root, draftDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %q: %v", dir, err)
		}
	}
	draftPath := filepath.Join(draftDir, "SKILL.md")
	if err := os.WriteFile(draftPath, validDraft(id), 0o644); err != nil {
		t.Fatalf("write draft: %v", err)
	}
	registryPath := filepath.Join(base, "skills.registry.yaml")
	if err := os.WriteFile(registryPath, []byte(projectCLIRegistry), 0o644); err != nil {
		t.Fatalf("write registry: %v", err)
	}
	return retireE2EEnv{binPath: binPath, home: home, root: root, registryPath: registryPath, draftPath: draftPath}
}

// run executes the real engine binary with an isolated HOME/STATE_DIR and
// returns its captured stdout, stderr and exit code. It never touches the
// invoking process's real environment.
func (e retireE2EEnv) run(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), retireE2ERunTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, e.binPath, args...)
	cmd.Env = []string{
		"HOME=" + e.home,
		"STATE_DIR=" + filepath.Join(e.home, "state"),
		"PATH=" + os.Getenv("PATH"),
	}
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("run %v: timed out after %s", args, retireE2ERunTimeout)
	}
	exitCode = 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("run %v: %v", args, err)
		}
	}
	return out.String(), errBuf.String(), exitCode
}

// registerRealSkill drives the real `skills project-register` verb and fails
// the test loudly if it does not succeed -- every test below needs one real,
// agent-owned, lock-recorded registration to retire.
func (e retireE2EEnv) registerRealSkill(t *testing.T, id string) {
	t.Helper()
	// project-register takes the key of the procedural promotion candidate
	// that justified the draft; "repeated-success" is one of the candidate
	// categories the detection contract emits, and any well-formed key
	// works here because these tests only need a real, agent-owned
	// registration to retire.
	candidate := "procedural/candidates/repeated-success/" + id
	out, errOut, code := e.run(t, "skills", "project-register",
		"--project-root", e.root,
		"--candidate", candidate,
		"--registry", e.registryPath,
		e.draftPath)
	if code != 0 {
		t.Fatalf("real project-register failed: exit=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

// TestProjectRetireRealCLI_RollbackSucceedsWhenLockStagingIsDenied drives the
// real CLI end to end: a real registration, then a real retirement whose
// very first write (staging the updated lock in .labdrian/, which always runs before any target delete is
// attempted) is denied by a real directory-permission fault. Rollback then
// has nothing attempted to undo, so it returns
// the original cause directly rather than ErrRollbackIncomplete, and the
// project tree is untouched -- the "rollback succeeds" shape of
// ExecuteProjectRetirePlan's contract, exercised on a real filesystem.
func TestProjectRetireRealCLI_RollbackSucceedsWhenLockStagingIsDenied(t *testing.T) {
	const id = "retire-rollback-ok"
	e := newRetireE2EEnv(t, id)
	e.registerRealSkill(t, id)

	lockDir := filepath.Dir(filepath.Join(e.root, filepath.FromSlash(ProjectLockRelPath)))
	before := snapshotTree(t, e.root)

	denyWrites(t, lockDir)

	stdout, stderr, code := e.run(t, "skills", "project-retire",
		"--project-root", e.root,
		"--registry", e.registryPath,
		"--reason", "e2e-rollback-success",
		id)

	if err := os.Chmod(lockDir, 0o755); err != nil {
		t.Fatalf("restore write on %q: %v", lockDir, err)
	}

	if code != 1 {
		t.Fatalf("retirement blocked at lock staging must exit 1, got %d (stdout %q, stderr %q)", code, stdout, stderr)
	}
	if stdout != "" {
		t.Errorf("a blocked-before-any-delete retirement must print no removed lines, got %q", stdout)
	}
	if strings.Contains(stderr, "rollback incomplete") {
		t.Fatalf("nothing was attempted, so rollback must not report incomplete: %q", stderr)
	}
	if !strings.Contains(stderr, "staging lock") {
		t.Errorf("stderr %q must name the lock-staging failure", stderr)
	}

	after := snapshotTree(t, e.root)
	assertSameTree(t, before, after)
}

// TestProjectRetireRealCLI_RollbackOfRollbackReportsIncompletePath drives the
// real CLI end to end: a real registration (which lands one SKILL.md under
// .claude/skills/<id>/ AND one under .agents/skills/<id>/, two independent
// directories), then a real retirement where the .agents directory has its
// write bit denied. The .claude target is removed and then genuinely
// restored by rollback's restore() (real
// WriteTemp+Rename recreating the file from its captured backup); the
// .agents target's removal fails against the denied directory and its own
// restore attempt -- the exact same WriteTemp+Rename pair, in the exact same
// still-denied directory -- fails too, so rollback prints `error: rollback
// incomplete: <rel-path>` naming it and the
// process exits 1. This is the diagnostic branch the archived change's
// verification left unexercised against a real binary and a real
// filesystem.
//
// The scenario relies on ExecuteProjectRetirePlan processing the .claude
// target before the .agents target; the assertions below pin that order,
// since a reordering would report a different path or restore nothing.
func TestProjectRetireRealCLI_RollbackOfRollbackReportsIncompletePath(t *testing.T) {
	const id = "retire-rollback-incomplete"
	e := newRetireE2EEnv(t, id)
	e.registerRealSkill(t, id)

	claudeRel := ".claude/skills/" + id + "/SKILL.md"
	agentsRel := ".agents/skills/" + id + "/SKILL.md"
	claudeAbs := filepath.Join(e.root, filepath.FromSlash(claudeRel))
	agentsAbs := filepath.Join(e.root, filepath.FromSlash(agentsRel))
	agentsDir := filepath.Dir(agentsAbs)
	lockAbs := filepath.Join(e.root, filepath.FromSlash(ProjectLockRelPath))
	lockDir := filepath.Dir(lockAbs)

	claudeBefore, err := os.ReadFile(claudeAbs)
	if err != nil {
		t.Fatalf("read claude target before retirement: %v", err)
	}
	agentsBefore, err := os.ReadFile(agentsAbs)
	if err != nil {
		t.Fatalf("read agents target before retirement: %v", err)
	}
	lockBefore, err := os.ReadFile(lockAbs)
	if err != nil {
		t.Fatalf("read lock before retirement: %v", err)
	}

	denyWrites(t, agentsDir)

	stdout, stderr, code := e.run(t, "skills", "project-retire",
		"--project-root", e.root,
		"--registry", e.registryPath,
		"--reason", "e2e-rollback-of-rollback",
		id)

	if err := os.Chmod(agentsDir, 0o755); err != nil {
		t.Fatalf("restore write on %q: %v", agentsDir, err)
	}

	if code != 1 {
		t.Fatalf("a rollback-of-rollback failure must exit 1, got %d (stdout %q, stderr %q)", code, stdout, stderr)
	}
	if stdout != "" {
		t.Errorf("a failed retirement must print no removed lines, got %q", stdout)
	}
	// The dedicated "error: rollback incomplete: <rel>" line must name the repo-relative path. The
	// wrapped cause on the line after it legitimately carries the real OS error, which
	// includes the absolute path the kernel reported -- that line is not the
	// rollback-incomplete pointer under test.
	want := "error: rollback incomplete: " + agentsRel + "\n"
	if !strings.Contains(stderr, want) {
		t.Fatalf("stderr = %q, want it to contain %q", stderr, want)
	}

	// The .claude target was actually deleted and then genuinely restored:
	// prove the real bytes on disk, not just the process exit code.
	claudeAfter, err := os.ReadFile(claudeAbs)
	if err != nil {
		t.Fatalf("read claude target after rollback: %v", err)
	}
	if !bytes.Equal(claudeAfter, claudeBefore) {
		t.Errorf("claude target restored with wrong bytes: got %q, want %q", claudeAfter, claudeBefore)
	}

	// The .agents target's removal itself failed (permission denied), so its
	// content was never touched, independent of the restore attempt that
	// also failed against it.
	agentsAfter, err := os.ReadFile(agentsAbs)
	if err != nil {
		t.Fatalf("read agents target after rollback: %v", err)
	}
	if !bytes.Equal(agentsAfter, agentsBefore) {
		t.Errorf("agents target changed despite a denied directory: got %q, want %q", agentsAfter, agentsBefore)
	}

	// The lock commit was never reached (the delete loop failed first), so
	// the lock content must be exactly what it was before this retirement,
	// and no orphaned lock-staging temp file may be left behind in
	// .labdrian/.
	lockAfter, err := os.ReadFile(lockAbs)
	if err != nil {
		t.Fatalf("read lock after rollback: %v", err)
	}
	if !bytes.Equal(lockAfter, lockBefore) {
		t.Errorf("lock changed despite the retirement failing before its commit: got %q, want %q", lockAfter, lockBefore)
	}
	entries, err := os.ReadDir(lockDir)
	if err != nil {
		t.Fatalf("list lock dir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".tmp-skills-") {
			t.Errorf("rollback left an orphaned lock-staging temp file behind: %q", entry.Name())
		}
	}
}
