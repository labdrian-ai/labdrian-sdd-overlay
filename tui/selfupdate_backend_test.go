package main

// Integration harness for `bin/labdrian-overlay self-update` (D1-D3, R-004
// through R-007). This is a Go-to-bash test: it spawns the REAL backend
// script against hermetic scratch git repos (a bare "origin" plus a working
// clone, both under t.TempDir()), so it exercises the actual `git`
// invocations the subcommand performs rather than a Go re-implementation of
// its logic. It lives in package main (the tui module) per design.md's File
// Changes table, even though it never touches Bubbletea.
//
// Every scratch repo is fully isolated from the developer's real git
// configuration (GIT_CONFIG_GLOBAL/SYSTEM=/dev/null, fixed author/committer
// identity) so the tests are hermetic and reproducible in CI.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// scratch-repo harness
// ---------------------------------------------------------------------------

func gitTestEnv() []string {
	return append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=selfupdate-test",
		"GIT_AUTHOR_EMAIL=selfupdate-test@example.invalid",
		"GIT_COMMITTER_NAME=selfupdate-test",
		"GIT_COMMITTER_EMAIL=selfupdate-test@example.invalid",
	)
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitTestEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s (dir=%s): %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return string(out)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func currentBranch(t *testing.T, dir string) string {
	t.Helper()
	return strings.TrimSpace(runGit(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
}

func headRev(t *testing.T, dir, ref string) string {
	t.Helper()
	return strings.TrimSpace(runGit(t, dir, "rev-parse", ref))
}

// newScratchRepoOnBranch creates a bare "origin" repo with one seed commit
// on main, clones it, and leaves the given branch checked out as current.
// Passing "main" leaves HEAD directly on main (used by the R-007 convergence
// check); any other name creates a local-only branch off main (mirroring
// R-004's "GIVEN a clean tracked tree on branch 'feature-x'" scenario).
func newScratchRepoOnBranch(t *testing.T, branch string) (origin, clone string) {
	t.Helper()
	base := t.TempDir()
	origin = filepath.Join(base, "origin.git")
	seed := filepath.Join(base, "seed")
	clone = filepath.Join(base, "clone")

	runGit(t, base, "init", "--bare", "-b", "main", origin)

	runGit(t, base, "clone", origin, seed)
	writeFile(t, filepath.Join(seed, "README.md"), "seed\n")
	runGit(t, seed, "add", "README.md")
	runGit(t, seed, "commit", "-m", "initial commit")
	runGit(t, seed, "push", "origin", "main")

	runGit(t, base, "clone", origin, clone)
	if branch != "main" {
		runGit(t, clone, "checkout", "-b", branch)
	}
	return origin, clone
}

// newScratchRepo is the common case: current branch is "feature-x".
func newScratchRepo(t *testing.T) (origin, clone string) {
	return newScratchRepoOnBranch(t, "feature-x")
}

// pushUpstreamCommit lands one new commit on origin's main via an ephemeral
// throwaway clone, so the caller's own scratch clone falls behind without
// this helper touching it directly.
func pushUpstreamCommit(t *testing.T, origin, filename, content, message string) {
	t.Helper()
	base := t.TempDir()
	pub := filepath.Join(base, "publisher")
	runGit(t, base, "clone", origin, pub)
	writeFile(t, filepath.Join(pub, filename), content)
	runGit(t, pub, "add", filename)
	runGit(t, pub, "commit", "-m", message)
	runGit(t, pub, "push", "origin", "main")
}

// realBackendBin locates the actual bin/labdrian-overlay script belonging to
// this checkout (never the scratch OVERLAY_DIR under test).
func realBackendBin(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root, ok := walkUpForBackend(wd)
	if !ok {
		t.Fatalf("could not locate bin/labdrian-overlay above %s", wd)
	}
	return filepath.Join(root, "bin", "labdrian-overlay")
}

// runBackendSubcommand runs the real backend script with OVERLAY_DIR pointed
// at the scratch clone, returning combined stdout+stderr and the exit code
// (0 when the process exits cleanly). HOME is pointed at a fresh scratch
// directory with .claude/skills pre-created, mirroring capture_backend_test.go's
// runCapture -- TARGET_PATHS[claude] ("$HOME/.claude/skills") must never
// resolve to whatever the machine running this test actually has. Unlike
// runCapture's intentionally-absent scratch HOME (capture never needs the
// directory to exist), sync-check's target-dir-missing guard skips its
// per-target block -- and never emits the VERDICT line an assertion checks --
// when the directory is absent, so it must be pre-created here (empty is
// enough; VERDICT prints regardless of what, if anything, is deployed
// there). Without this, a developer machine with real deployed skills masks
// the bug entirely while a CI runner with no ~/.claude/skills at all hits it
// every time: this exact test failed in CI (target dir not found) while
// passing locally, reproduced here by rerunning with HOME pointed at an
// empty directory before this fix existed.
func runBackendSubcommand(t *testing.T, overlayDir string, args ...string) (string, int) {
	t.Helper()
	bin := realBackendBin(t)
	fakeHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(fakeHome, ".claude", "skills"), 0o755); err != nil {
		t.Fatalf("creating scratch HOME/.claude/skills: %v", err)
	}
	cmd := exec.Command(bin, args...)
	cmd.Dir = overlayDir
	cmd.Env = append(gitTestEnv(), "OVERLAY_DIR="+overlayDir, "HOME="+fakeHome)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), 0
	}
	ee, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("running %s %s: %v\n%s", bin, strings.Join(args, " "), err, out)
	}
	return string(out), ee.ExitCode()
}

func runSelfUpdate(t *testing.T, overlayDir string) (string, int) {
	t.Helper()
	return runBackendSubcommand(t, overlayDir, "self-update")
}

// ---------------------------------------------------------------------------
// RED cases (task 1.2)
// ---------------------------------------------------------------------------

// R-004: a behind main fast-forwards to origin/main and the original branch
// is restored; only main's HEAD moves.
func TestSelfUpdateBackend_BehindFastForwardsAndReturns(t *testing.T) {
	origin, clone := newScratchRepo(t)
	pushUpstreamCommit(t, origin, "upstream.txt", "v2\n", "upstream advance")

	featureHeadBefore := headRev(t, clone, "feature-x")

	out, code := runSelfUpdate(t, clone)
	if code != 0 {
		t.Fatalf("self-update exit=%d, want 0\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "fast-forward") {
		t.Errorf("output does not mention a fast-forward:\n%s", out)
	}

	if got := currentBranch(t, clone); got != "feature-x" {
		t.Errorf("current branch = %q, want restored to feature-x", got)
	}

	mainHead := headRev(t, clone, "main")
	originMainHead := headRev(t, clone, "origin/main")
	if mainHead != originMainHead {
		t.Errorf("main HEAD = %s, want it to equal origin/main HEAD = %s", mainHead, originMainHead)
	}

	featureHeadAfter := headRev(t, clone, "feature-x")
	if featureHeadAfter != featureHeadBefore {
		t.Errorf("feature-x HEAD changed (%s -> %s); only main should move", featureHeadBefore, featureHeadAfter)
	}
}

// Already up to date: exit 0, and no checkout is ever attempted (step 6 is
// skipped entirely, proven via the HEAD reflog staying byte-identical).
func TestSelfUpdateBackend_UpToDateNoCheckout(t *testing.T) {
	_, clone := newScratchRepo(t)

	reflogBefore := runGit(t, clone, "reflog", "show", "--no-abbrev", "HEAD")

	out, code := runSelfUpdate(t, clone)
	if code != 0 {
		t.Fatalf("self-update exit=%d, want 0\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "already up to date") {
		t.Errorf("output does not report already-up-to-date:\n%s", out)
	}

	if got := currentBranch(t, clone); got != "feature-x" {
		t.Errorf("current branch = %q, want unchanged feature-x", got)
	}

	reflogAfter := runGit(t, clone, "reflog", "show", "--no-abbrev", "HEAD")
	if reflogBefore != reflogAfter {
		t.Errorf("HEAD reflog changed -- a checkout occurred when none was expected\nbefore:\n%s\nafter:\n%s", reflogBefore, reflogAfter)
	}
}

// R-005: a dirty tracked tree blocks the update before any checkout.
func TestSelfUpdateBackend_DirtyTrackedTreeBlocks(t *testing.T) {
	origin, clone := newScratchRepo(t)
	pushUpstreamCommit(t, origin, "upstream.txt", "v2\n", "upstream advance")

	readmePath := filepath.Join(clone, "README.md")
	writeFile(t, readmePath, "dirty tracked change\n")

	mainHeadBefore := headRev(t, clone, "main")

	out, code := runSelfUpdate(t, clone)
	if code == 0 {
		t.Fatalf("self-update exit=0, want nonzero for a dirty tracked tree\noutput:\n%s", out)
	}
	if !strings.Contains(out, "ERROR:") {
		t.Errorf("output does not carry an ERROR:-prefixed refusal:\n%s", out)
	}
	if !strings.Contains(out, "uncommitted tracked changes") {
		t.Errorf("output does not name the dirty-tree refusal:\n%s", out)
	}

	if got := currentBranch(t, clone); got != "feature-x" {
		t.Errorf("current branch = %q, want unchanged feature-x", got)
	}
	if got := headRev(t, clone, "main"); got != mainHeadBefore {
		t.Errorf("main HEAD changed (%s -> %s), want untouched", mainHeadBefore, got)
	}

	gotContent, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	if string(gotContent) != "dirty tracked change\n" {
		t.Errorf("dirty tracked change was lost: %q", gotContent)
	}
}

// Untracked-only changes must NOT block the update (capture/apply parity).
func TestSelfUpdateBackend_UntrackedOnlyProceeds(t *testing.T) {
	origin, clone := newScratchRepo(t)
	pushUpstreamCommit(t, origin, "upstream.txt", "v2\n", "upstream advance")

	scratchPath := filepath.Join(clone, "scratch.tmp")
	writeFile(t, scratchPath, "untracked scratch\n")

	out, code := runSelfUpdate(t, clone)
	if code != 0 {
		t.Fatalf("self-update exit=%d, want 0 (untracked-only must not block)\noutput:\n%s", code, out)
	}

	mainHead := headRev(t, clone, "main")
	originMainHead := headRev(t, clone, "origin/main")
	if mainHead != originMainHead {
		t.Errorf("main did not converge: main=%s origin/main=%s", mainHead, originMainHead)
	}

	gotContent, err := os.ReadFile(scratchPath)
	if err != nil {
		t.Fatalf("untracked file was removed: %v", err)
	}
	if string(gotContent) != "untracked scratch\n" {
		t.Errorf("untracked file content changed: %q", gotContent)
	}
}

// R-006: local-ahead main is a hard refusal, before any checkout.
func TestSelfUpdateBackend_LocalAheadBlocks(t *testing.T) {
	_, clone := newScratchRepo(t)

	runGit(t, clone, "checkout", "main")
	writeFile(t, filepath.Join(clone, "local-only.txt"), "ahead commit\n")
	runGit(t, clone, "add", "local-only.txt")
	runGit(t, clone, "commit", "-m", "local-only commit, never pushed")
	runGit(t, clone, "checkout", "feature-x")

	mainHeadBefore := headRev(t, clone, "main")

	out, code := runSelfUpdate(t, clone)
	if code == 0 {
		t.Fatalf("self-update exit=0, want nonzero for local-ahead main\noutput:\n%s", out)
	}
	if !strings.Contains(out, "ERROR:") {
		t.Errorf("output does not carry an ERROR:-prefixed refusal:\n%s", out)
	}
	if !strings.Contains(out, "ahead of origin/main") {
		t.Errorf("output does not name the local-ahead refusal:\n%s", out)
	}

	if got := currentBranch(t, clone); got != "feature-x" {
		t.Errorf("current branch = %q, want unchanged feature-x", got)
	}
	if got := headRev(t, clone, "main"); got != mainHeadBefore {
		t.Errorf("main HEAD changed (%s -> %s), want untouched", mainHeadBefore, got)
	}
}

// No 'origin' remote configured is a hard refusal.
func TestSelfUpdateBackend_NoOriginRemoteBlocks(t *testing.T) {
	_, clone := newScratchRepo(t)
	runGit(t, clone, "remote", "remove", "origin")

	out, code := runSelfUpdate(t, clone)
	if code == 0 {
		t.Fatalf("self-update exit=0, want nonzero with no origin remote\noutput:\n%s", out)
	}
	if !strings.Contains(out, "ERROR:") {
		t.Errorf("output does not carry an ERROR:-prefixed refusal:\n%s", out)
	}
	if !strings.Contains(out, "origin") {
		t.Errorf("output does not mention the missing origin remote:\n%s", out)
	}
}

// A checkout to main blocked mid-operation (untracked file collision)
// restores the original branch and never fast-forwards main.
func TestSelfUpdateBackend_BlockedCheckoutRestoresBranch(t *testing.T) {
	origin, clone := newScratchRepo(t)
	// origin/main gains a tracked "conflict.txt"; the clone never sees it as
	// tracked, but has an untracked file at the SAME path with different
	// content, so `git checkout main` refuses (would overwrite untracked file).
	pushUpstreamCommit(t, origin, "conflict.txt", "main-content\n", "add conflict file")
	writeFile(t, filepath.Join(clone, "conflict.txt"), "feature-content\n")

	featureHeadBefore := headRev(t, clone, "feature-x")
	mainHeadBefore := headRev(t, clone, "main")

	out, code := runSelfUpdate(t, clone)
	// This used to assert a NONZERO exit: the untracked collision blocked
	// `git checkout main`, which self-update needed in order to fast-forward.
	// It no longer checks main out at all -- off main it moves the ref alone
	// -- so the collision is simply irrelevant now, and the update succeeds.
	// The obstacle is kept in the fixture on purpose: it is the proof that
	// the working tree is genuinely untouched, not merely believed to be.
	if code != 0 {
		t.Fatalf("self-update exit=%d, want 0: an untracked collision on main is no longer in the way, because main is never checked out\noutput:\n%s", code, out)
	}

	if got := currentBranch(t, clone); got != "feature-x" {
		t.Errorf("current branch = %q, want never-left feature-x", got)
	}
	if got := headRev(t, clone, "feature-x"); got != featureHeadBefore {
		t.Errorf("feature-x HEAD changed (%s -> %s), want untouched", featureHeadBefore, got)
	}
	if got := headRev(t, clone, "main"); got == mainHeadBefore {
		t.Errorf("main HEAD did not move (%s); the whole point is that it converges while the operator's branch does not", got)
	}

	gotContent, err := os.ReadFile(filepath.Join(clone, "conflict.txt"))
	if err != nil {
		t.Fatalf("untracked conflict.txt was removed: %v", err)
	}
	if string(gotContent) != "feature-content\n" {
		t.Errorf("untracked conflict.txt content changed: %q", gotContent)
	}
}

// A held .git/index.lock is a concurrent git operation in progress. It used
// to block the update, because checking main out writes the index. Moving a
// ref does not, so the update now succeeds through it — and the operator's
// branch and HEAD are still untouched, which was always the real guarantee.
//
// The lock stays in the fixture deliberately: it is the cheapest available
// proof that this code path never writes the index.
func TestSelfUpdateBackend_HeldIndexLockDoesNotBlockTheUpdate(t *testing.T) {
	origin, clone := newScratchRepo(t)
	pushUpstreamCommit(t, origin, "upstream.txt", "v2\n", "upstream advance")

	lockPath := filepath.Join(clone, ".git", "index.lock")
	writeFile(t, lockPath, "")

	featureHeadBefore := headRev(t, clone, "feature-x")
	mainHeadBefore := headRev(t, clone, "main")

	out, code := runSelfUpdate(t, clone)
	if code != 0 {
		t.Fatalf("self-update exit=%d, want 0: a held index.lock cannot block an update that never writes the index\noutput:\n%s", code, out)
	}

	if got := currentBranch(t, clone); got != "feature-x" {
		t.Errorf("current branch = %q, want unchanged feature-x", got)
	}
	if got := headRev(t, clone, "feature-x"); got != featureHeadBefore {
		t.Errorf("feature-x HEAD changed (%s -> %s), want untouched", featureHeadBefore, got)
	}
	if got := headRev(t, clone, "main"); got == mainHeadBefore {
		t.Errorf("main HEAD did not move (%s); the update was supposed to converge it", got)
	}
	// The lock must survive: self-update has no business clearing another
	// process's lock, and removing it would be a far worse bug than failing.
	if _, err := os.Stat(lockPath); err != nil {
		t.Errorf("self-update removed a lock it did not create: %v", err)
	}
}

// ---------------------------------------------------------------------------
// R-007 (task 1.5): observable convergence via the existing sync-check.
// ---------------------------------------------------------------------------

// After a successful self-update, sync-check (read-only, unmodified
// detection logic) must report REPO_BEHIND_ORIGIN=0. This requires running
// sync-check from a branch directly comparable to origin/main, so the
// scratch repo here stays checked out on main throughout (self-update starts
// and ends on main when it was already the current branch).
func TestSelfUpdateBackend_SyncCheckConvergesToZero(t *testing.T) {
	origin, clone := newScratchRepoOnBranch(t, "main")
	pushUpstreamCommit(t, origin, "upstream.txt", "v2\n", "upstream advance")

	out, code := runSelfUpdate(t, clone)
	if code != 0 {
		t.Fatalf("self-update exit=%d, want 0\noutput:\n%s", code, out)
	}

	syncOut, syncCode := runBackendSubcommand(t, clone, "sync-check", "--target", "claude")
	if syncCode != 0 {
		t.Fatalf("sync-check exit=%d, want 0\noutput:\n%s", syncCode, syncOut)
	}
	if !strings.Contains(syncOut, "REPO_BEHIND_ORIGIN=0") {
		t.Errorf("sync-check does not report REPO_BEHIND_ORIGIN=0 after a successful self-update:\n%s", syncOut)
	}
}

// The TUI's "Actualizar repositorio" button (self-update) only ever converges
// local main -- it deliberately never touches the current branch (D1-D3) --
// so the observable-convergence guarantee must hold from ANY starting
// branch, not only when the repo happens to already be on main. The sibling
// test above sidesteps this entirely by forcing the scratch repo onto main
// throughout (see its own comment); this test exercises the realistic case
// self-update's actual caller hits in practice: a maintainer running the TUI
// from a feature branch. Before the fix, REPO_BEHIND_ORIGIN was computed as
// `HEAD..origin/main` (the checked-out branch), so it stayed nonzero here
// even after self-update fast-forwarded main -- the banner never converged.
func TestSelfUpdateBackend_SyncCheckConvergesToZeroFromNonMainBranch(t *testing.T) {
	origin, clone := newScratchRepo(t) // stays on "feature-x", never main
	pushUpstreamCommit(t, origin, "upstream.txt", "v2\n", "upstream advance")

	out, code := runSelfUpdate(t, clone)
	if code != 0 {
		t.Fatalf("self-update exit=%d, want 0\noutput:\n%s", code, out)
	}
	if got := currentBranch(t, clone); got != "feature-x" {
		t.Fatalf("self-update did not return to feature-x, got %q", got)
	}

	syncOut, syncCode := runBackendSubcommand(t, clone, "sync-check", "--target", "claude")
	if syncCode != 0 {
		t.Fatalf("sync-check exit=%d, want 0\noutput:\n%s", syncCode, syncOut)
	}
	if !strings.Contains(syncOut, "REPO_BEHIND_ORIGIN=0") {
		t.Errorf("sync-check does not report REPO_BEHIND_ORIGIN=0 after a successful self-update run from a non-main branch:\n%s", syncOut)
	}
}
