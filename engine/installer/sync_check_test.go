package installer_test

// TC-SYNC-CHECK: git-fixture integration tests for the REPO_BEHIND_ORIGIN
// verdict field in `bin/labdrian-overlay cmd_sync_check` (spec R-001..R-004).
//
// These tests reuse the setupSandboxOverlay/runOverlay pattern from
// route_test.go and extend it with a bare "origin" remote repo whose main
// branch can be advanced independently, so tests can assert both the
// cached-ref default path (no live fetch) and the --check-origin/--fetch
// live-fetch path.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// advanceOriginMain adds n empty commits to the bare origin repo's main
// branch by cloning it into a scratch directory, committing, and pushing
// back. Used both to seed the initial "ahead" state and to simulate origin
// advancing further after a cached fetch (proving default mode performs no
// live fetch).
func advanceOriginMain(t *testing.T, originPath string, n int) {
	t.Helper()
	if n <= 0 {
		return
	}

	scratch := t.TempDir()
	runGitIn := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	runGitIn(filepath.Dir(scratch), "clone", originPath, scratch)
	runGitIn(scratch, "checkout", "main")
	for i := 0; i < n; i++ {
		runGitIn(scratch, "commit", "--allow-empty", "-m", fmt.Sprintf("origin advance %d", i))
	}
	runGitIn(scratch, "push", "origin", "main")
}

// breakOriginRemote repoints the overlay repo's origin remote at a
// nonexistent path so `git fetch origin` fails deterministically.
func breakOriginRemote(t *testing.T, overlayDir, home string) {
	t.Helper()
	cmd := exec.Command("git", "remote", "set-url", "origin", filepath.Join(t.TempDir(), "does-not-exist.git"))
	cmd.Dir = overlayDir
	cmd.Env = append(os.Environ(), "HOME="+home)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git remote set-url: %v\n%s", err, out)
	}
}

// setupSandboxOverlayWithOrigin extends setupSandboxOverlay with a bare
// "origin" remote repo pointing at the same history.
//
// aheadCommits advances origin's main branch by that many additional empty
// commits BEFORE any fetch, so a cached fetch (fetchCached=true) caches
// refs/remotes/origin/main at exactly that offset from local main.
//
// fetchCached controls whether `git fetch origin` is run once during setup:
//   - true:  refs/remotes/origin/main is cached at aheadCommits (default
//     cached-ref mode should report aheadCommits without re-fetching).
//   - false: the origin remote is configured but never fetched (R-004's
//     "remote configured but ref never fetched" case).
//
// Returns the overlayDir, the env slice (same shape as setupSandboxOverlay),
// and the path to the bare origin repo so callers can advance it further via
// advanceOriginMain to prove no live fetch happens in default mode.
func setupSandboxOverlayWithOrigin(t *testing.T, home string, aheadCommits int, fetchCached bool) (overlayDir string, env []string, originPath string) {
	t.Helper()

	overlayDir, env = setupSandboxOverlay(t, home)

	originPath = filepath.Join(t.TempDir(), "origin.git")

	runGit := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
			"HOME="+home,
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	runGit(overlayDir, "clone", "--bare", overlayDir, originPath)

	advanceOriginMain(t, originPath, aheadCommits)

	runGit(overlayDir, "remote", "add", "origin", originPath)

	if fetchCached {
		runGit(overlayDir, "fetch", "origin")
	}

	return overlayDir, env, originPath
}

// TestSyncCheck_ReportsRepoBehindOrigin_CachedRef pins R-001/R-002: HEAD is 3
// commits behind the CACHED origin/main ref, and the reported count reflects
// that cached value even after origin advances further — proving no live
// fetch occurs in default mode.
func TestSyncCheck_ReportsRepoBehindOrigin_CachedRef(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	overlay := overlayScript(t)
	home := t.TempDir()
	_, env, originPath := setupSandboxOverlayWithOrigin(t, home, 3, true)

	if _, err := runOverlay(t, overlay, env, "apply", "--target", "all"); err != nil {
		t.Fatalf("overlay apply: %v", err)
	}

	// Advance origin further AFTER the cached fetch. Default (cached-ref)
	// mode must NOT reflect this — it must still report the value cached at
	// setup time (3), proving no live `git fetch` was invoked.
	advanceOriginMain(t, originPath, 2)

	out, err := runOverlay(t, overlay, env, "sync-check", "--target", "claude")
	if err != nil {
		t.Fatalf("overlay sync-check: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "REPO_BEHIND_ORIGIN=3") {
		t.Errorf("expected REPO_BEHIND_ORIGIN=3 (cached), got:\n%s", out)
	}
	if strings.Contains(out, "REPO_BEHIND_ORIGIN=5") {
		t.Errorf("REPO_BEHIND_ORIGIN reflects live state (5) instead of the cached ref (3) — default mode must not fetch:\n%s", out)
	}
}

// TestSyncCheck_BehindOriginOnly_ActionHintsGitPull pins the ACTION-hint gap
// found by judgment-day on PR2: when a target is behind origin/main with no
// other drift (UPSTREAM_CHANGED=0, OVERLAY_NOT_DEPLOYED=0), the ACTION line
// must tell the user to git pull, not falsely claim "in sync (healthy)".
func TestSyncCheck_BehindOriginOnly_ActionHintsGitPull(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	overlay := overlayScript(t)
	home := t.TempDir()
	_, env, _ := setupSandboxOverlayWithOrigin(t, home, 3, true)

	if _, err := runOverlay(t, overlay, env, "apply", "--target", "all"); err != nil {
		t.Fatalf("overlay apply: %v", err)
	}

	out, err := runOverlay(t, overlay, env, "sync-check", "--target", "claude")
	if err != nil {
		t.Fatalf("overlay sync-check: %v\noutput:\n%s", err, out)
	}
	if strings.Contains(out, "in sync with gentle-ai (healthy)") {
		t.Errorf("ACTION falsely claims healthy while REPO_BEHIND_ORIGIN>0:\n%s", out)
	}
	if !strings.Contains(out, "git pull") {
		t.Errorf("expected ACTION to hint 'git pull' when behind origin with no other drift, got:\n%s", out)
	}
}

// TestSyncCheck_EvenWithOrigin_ReportsZero pins R-002: HEAD even with the
// cached origin/main ref reports REPO_BEHIND_ORIGIN=0.
func TestSyncCheck_EvenWithOrigin_ReportsZero(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	overlay := overlayScript(t)
	home := t.TempDir()
	_, env, _ := setupSandboxOverlayWithOrigin(t, home, 0, true)

	if _, err := runOverlay(t, overlay, env, "apply", "--target", "all"); err != nil {
		t.Fatalf("overlay apply: %v", err)
	}

	out, err := runOverlay(t, overlay, env, "sync-check", "--target", "claude")
	if err != nil {
		t.Fatalf("overlay sync-check: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "REPO_BEHIND_ORIGIN=0") {
		t.Errorf("expected REPO_BEHIND_ORIGIN=0, got:\n%s", out)
	}
}

// TestSyncCheck_NoOriginRemote_ReportsNA pins R-004: no origin remote
// configured degrades to the literal NA sentinel, and existing checks still
// complete.
func TestSyncCheck_NoOriginRemote_ReportsNA(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	overlay := overlayScript(t)
	home := t.TempDir()
	_, env := setupSandboxOverlay(t, home) // no origin remote configured

	if _, err := runOverlay(t, overlay, env, "apply", "--target", "all"); err != nil {
		t.Fatalf("overlay apply: %v", err)
	}

	out, err := runOverlay(t, overlay, env, "sync-check", "--target", "claude")
	if err != nil {
		t.Fatalf("overlay sync-check: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "REPO_BEHIND_ORIGIN=NA") {
		t.Errorf("expected literal REPO_BEHIND_ORIGIN=NA with no origin remote, got:\n%s", out)
	}
	if !strings.Contains(out, "VERDICT:claude:") {
		t.Errorf("expected existing VERDICT line to still be emitted, got:\n%s", out)
	}
	// Post-merge audit finding RES-002: this NA-producing branch must explain
	// itself on stderr like the fetch-failure branch does, instead of being
	// the only one of the four that stays silent.
	if !strings.Contains(out, "SYNC_CHECK:") || !strings.Contains(out, "no 'origin' remote configured") {
		t.Errorf("expected a SYNC_CHECK warning explaining the missing origin remote, got:\n%s", out)
	}
}

// TestSyncCheck_CheckOriginFlag_NoOriginRemote_ReportsNA pins R-003/R-004's
// scenario "with or without --check-origin" for the no-origin-remote case
// specifically: --check-origin must short-circuit to NA via the remote
// existence check BEFORE ever attempting a fetch (no fetch-related warning
// should appear), matching the code path's actual precedence.
func TestSyncCheck_CheckOriginFlag_NoOriginRemote_ReportsNA(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	overlay := overlayScript(t)
	home := t.TempDir()
	_, env := setupSandboxOverlay(t, home) // no origin remote configured

	if _, err := runOverlay(t, overlay, env, "apply", "--target", "all"); err != nil {
		t.Fatalf("overlay apply: %v", err)
	}

	out, err := runOverlay(t, overlay, env, "sync-check", "--target", "claude", "--check-origin")
	if err != nil {
		t.Fatalf("overlay sync-check --check-origin must not hard-fail with no origin remote: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "REPO_BEHIND_ORIGIN=NA") {
		t.Errorf("expected literal REPO_BEHIND_ORIGIN=NA with no origin remote (even with --check-origin), got:\n%s", out)
	}
	if !strings.Contains(out, "no 'origin' remote configured") {
		t.Errorf("expected the no-remote warning, got:\n%s", out)
	}
	if strings.Contains(out, "git fetch origin failed") {
		t.Errorf("must short-circuit before attempting a fetch when no origin remote exists, got a fetch-failure warning instead:\n%s", out)
	}
}

// TestSyncCheck_NoCachedRef_ReportsNA pins R-004: origin configured but
// refs/remotes/origin/main never fetched degrades to NA rather than erroring.
func TestSyncCheck_NoCachedRef_ReportsNA(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	overlay := overlayScript(t)
	home := t.TempDir()
	_, env, _ := setupSandboxOverlayWithOrigin(t, home, 3, false) // configured, never fetched

	if _, err := runOverlay(t, overlay, env, "apply", "--target", "all"); err != nil {
		t.Fatalf("overlay apply: %v", err)
	}

	out, err := runOverlay(t, overlay, env, "sync-check", "--target", "claude")
	if err != nil {
		t.Fatalf("overlay sync-check: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "REPO_BEHIND_ORIGIN=NA") {
		t.Errorf("expected literal REPO_BEHIND_ORIGIN=NA with no cached ref, got:\n%s", out)
	}
	// Post-merge audit finding RES-002: this NA-producing branch must explain
	// itself on stderr too.
	if !strings.Contains(out, "SYNC_CHECK:") || !strings.Contains(out, "no cached refs/remotes/origin/main") {
		t.Errorf("expected a SYNC_CHECK warning explaining the missing cached ref, got:\n%s", out)
	}
}

// TestSyncCheck_CheckOriginFlag_FetchesLive pins R-003: --check-origin fetches
// before comparing, so the reported count reflects freshly fetched state
// rather than a stale cached count.
func TestSyncCheck_CheckOriginFlag_FetchesLive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	overlay := overlayScript(t)
	home := t.TempDir()
	_, env, originPath := setupSandboxOverlayWithOrigin(t, home, 3, true)

	if _, err := runOverlay(t, overlay, env, "apply", "--target", "all"); err != nil {
		t.Fatalf("overlay apply: %v", err)
	}

	// Origin advances further after the initial cached fetch.
	advanceOriginMain(t, originPath, 2)

	out, err := runOverlay(t, overlay, env, "sync-check", "--target", "claude", "--check-origin")
	if err != nil {
		t.Fatalf("overlay sync-check --check-origin: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "REPO_BEHIND_ORIGIN=5") {
		t.Errorf("expected REPO_BEHIND_ORIGIN=5 (fresh fetch), got:\n%s", out)
	}
}

// TestSyncCheck_CheckOriginFlag_FetchFailure_DegradesToNA pins R-003: a failed
// `git fetch origin` under --check-origin degrades to NA with a scoped
// warning, and the existing offline checks still complete.
func TestSyncCheck_CheckOriginFlag_FetchFailure_DegradesToNA(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	overlay := overlayScript(t)
	home := t.TempDir()
	overlayDir, env, _ := setupSandboxOverlayWithOrigin(t, home, 0, true)

	if _, err := runOverlay(t, overlay, env, "apply", "--target", "all"); err != nil {
		t.Fatalf("overlay apply: %v", err)
	}

	breakOriginRemote(t, overlayDir, home)

	out, err := runOverlay(t, overlay, env, "sync-check", "--target", "claude", "--check-origin")
	if err != nil {
		t.Fatalf("overlay sync-check --check-origin must not hard-fail on fetch error: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "REPO_BEHIND_ORIGIN=NA") {
		t.Errorf("expected REPO_BEHIND_ORIGIN=NA on fetch failure, got:\n%s", out)
	}
	if !strings.Contains(out, "SYNC_CHECK:") {
		t.Errorf("expected a scoped SYNC_CHECK warning on fetch failure, got:\n%s", out)
	}
	if !strings.Contains(out, "VERDICT:claude:") {
		t.Errorf("expected VERDICT line still emitted despite fetch failure, got:\n%s", out)
	}
	// Post-merge audit finding RISK-001: the warning must surface git's own
	// diagnostic text, not just the generic "degrading to NA" message --
	// otherwise a security-relevant fetch failure (e.g. a host-key mismatch)
	// is indistinguishable from a benign offline failure.
	if !strings.Contains(out, "does not appear to be a git repository") {
		t.Errorf("expected the real git fetch error text surfaced in the warning, got:\n%s", out)
	}
	// Post-merge audit finding READ-002: the repo-scope warning prefix must
	// be a documented marker, not a bare "*" masquerading as a target name
	// (which breaks the SYNC_CHECK:$t: convention used everywhere else).
	if !strings.Contains(out, "SYNC_CHECK:(repo):") {
		t.Errorf("expected the SYNC_CHECK:(repo): repo-scope prefix, got:\n%s", out)
	}
}
