package skills

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// This file closes the acceptance gap recorded in the archived change
// 2026-09-21-procedural-memory-lifecycle: the retirement rollback-of-rollback
// diagnostic branch (ExecuteProjectRetirePlan's `error: rollback incomplete:
// <rel-path>`, project_register.go:855) was covered only by unit tests that
// inject failures through the fakeProjectFS fake (project_register_test.go,
// TestExecuteProjectRetireRollbackFailureReportsRelativePath), never through
// the real `engine skills project-retire` binary against a real filesystem.
// The tests below build that binary and drive it directly.
//
// Both tests use real directory permissions (chmod 0555, no write bit) as
// the fault, because the ordering of ExecuteProjectRetirePlan
// (project_register.go:752-808) makes a decoupled "delete fails but its own
// restore later succeeds" fault impossible to construct with permissions
// alone in one synchronous CLI run:
//
//   - the lock's initial WriteTemp (staging, before any delete) and its
//     final Rename (commit, after every delete) write to the exact same
//     directory, so a static permission that blocks the commit also blocks
//     the initial stage -- no deletes are ever attempted (see
//     TestProjectRetireRealCLI_RollbackSucceedsWhenLockStagingIsDenied);
//   - rollback's restore() (project_register.go:862-872) retries the exact
//     same (WriteTemp, Rename) pair, in the exact same directory, that the
//     forward delete just used. Whatever static condition made a target's
//     Remove fail will therefore also make that target's own restore fail
//     (see TestProjectRetireRealCLI_RollbackOfRollbackReportsIncompletePath).
//
// No env-var or hook seam exists in production for injecting a transient
// fault (skills/skills.go:42 hardcodes osProjectFS{} for the real CLI), and
// adding one would be a production-only backdoor, so these two real-fs
// shapes are the complete, honest coverage available: a fault before any
// mutation (full rollback, nothing to restore, tree untouched) and a fault
// whose own restore attempt fails (rollback-of-rollback, reported and
// exited non-zero) alongside a sibling target that WAS deleted and IS
// correctly restored.
//
// A read-only bind mount would decouple delete-fails-but-restore-succeeds
// for a single target, but is not available in this environment.

// buildRetireEngineBinary compiles the real engine binary once per test into
// an isolated temp directory, the same way
// shelltest/overlay_pi_package_build_test.go builds it for its own e2e runs.
func buildRetireEngineBinary(t *testing.T) string {
	t.Helper()
	engineRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve engine module root: %v", err)
	}
	binPath := filepath.Join(t.TempDir(), "engine-retire-e2e")
	build := exec.Command("go", "build", "-o", binPath, "./cmd")
	build.Dir = engineRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build engine binary: %v\n%s", err, out)
	}
	return binPath
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
	cmd := exec.Command(e.binPath, args...)
	cmd.Env = []string{
		"HOME=" + e.home,
		"STATE_DIR=" + filepath.Join(e.home, "state"),
		"PATH=" + os.Getenv("PATH"),
	}
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err := cmd.Run()
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
// very first write (staging the updated lock in .labdrian/, project_
// register.go:780-784, which always runs before any target delete is
// attempted) is denied by a real directory-permission fault. Rollback then
// has nothing attempted to undo (project_register.go:829-837), so it returns
// the original cause directly rather than ErrRollbackIncomplete, and the
// project tree is untouched -- the "rollback succeeds" shape of
// ExecuteProjectRetirePlan's contract, exercised on a real filesystem.
func TestProjectRetireRealCLI_RollbackSucceedsWhenLockStagingIsDenied(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission-based fault injection has no effect for root")
	}
	const id = "retire-rollback-ok"
	e := newRetireE2EEnv(t, id)
	e.registerRealSkill(t, id)

	lockDir := filepath.Join(e.root, ".labdrian")
	before := snapshotTree(t, e.root)

	if err := os.Chmod(lockDir, 0o555); err != nil {
		t.Fatalf("deny write on %q: %v", lockDir, err)
	}
	t.Cleanup(func() { _ = os.Chmod(lockDir, 0o755) })

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
// restored by rollback's restore() (project_register.go:862-872, real
// WriteTemp+Rename recreating the file from its captured backup); the
// .agents target's removal fails against the denied directory and its own
// restore attempt -- the exact same WriteTemp+Rename pair, in the exact same
// still-denied directory -- fails too, so rollback prints `error: rollback
// incomplete: <rel-path>` (project_register.go:855) naming it and the
// process exits 1. This is the diagnostic branch the archived change's
// verification left unexercised against a real binary and a real
// filesystem.
func TestProjectRetireRealCLI_RollbackOfRollbackReportsIncompletePath(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission-based fault injection has no effect for root")
	}
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

	if err := os.Chmod(agentsDir, 0o555); err != nil {
		t.Fatalf("deny write on %q: %v", agentsDir, err)
	}
	t.Cleanup(func() { _ = os.Chmod(agentsDir, 0o755) })

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
	// The dedicated "error: rollback incomplete: <rel>" line (project_
	// register.go:855) must name the repo-relative path. The wrapped cause
	// on the line after it legitimately carries the real OS error, which
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
