package main

// Integration harness for `bin/labdrian-overlay capture`'s upstream-branch
// reconcile step. This is a Go-to-bash test, following the exact pattern
// established by selfupdate_backend_test.go (real backend script against
// hermetic scratch git repos) — it reuses that file's shared helpers
// (gitTestEnv, runGit, writeFile, currentBranch, headRev, realBackendBin,
// runBackendSubcommand) since both live in package main.
//
// Bug this covers: cmd_capture used to `git checkout upstream` and push
// unconditionally, with no fetch/reconcile step. If origin/upstream had
// advanced independently since the last local capture (another machine
// captured and pushed, or a manual reconcile merge landed upstream), the
// next local capture's `git commit` on 'upstream' would succeed, but the
// following `git push origin upstream` would be rejected non-fast-forward —
// and because the script runs under `set -euo pipefail`, that rejection
// crashed the whole capture command, even though the local capture itself
// had already succeeded. Observed for real on 2026-08-25 against this
// repo's own 'upstream' branch.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// newScratchRepoWithUpstream extends the self-update harness's scratch-repo
// shape with an 'upstream' branch on the bare origin, seeded with a minimal
// (comment-only) overlay.manifest so managed_files() resolves to zero
// entries — this test targets the reconcile step itself, not the
// managed-file copy loop, so an empty manifest keeps the scenario isolated
// and avoids depending on the real developer's $HOME/.claude/skills.
// Leaves 'feature-x' checked out as current, matching real usage (a user
// invokes capture from whatever branch they were already on).
func newScratchRepoWithUpstream(t *testing.T) (origin, clone string) {
	t.Helper()
	origin, clone = newScratchRepo(t)

	runGit(t, clone, "checkout", "-b", "upstream", "main")
	writeFile(t, filepath.Join(clone, "overlay.manifest"), "# empty test manifest\n")
	// cmd_capture unconditionally `git add skills/` after copying managed
	// files; git errors on a pathspec that matches nothing at all, so the
	// scratch 'upstream' branch needs a tracked skills/ directory to exist
	// even though the empty manifest means nothing new lands in it.
	if err := os.MkdirAll(filepath.Join(clone, "skills"), 0o755); err != nil {
		t.Fatalf("mkdir skills/: %v", err)
	}
	writeFile(t, filepath.Join(clone, "skills", ".gitkeep"), "")
	runGit(t, clone, "add", "overlay.manifest", "skills/.gitkeep")
	runGit(t, clone, "commit", "-m", "seed upstream manifest")
	runGit(t, clone, "push", "-u", "origin", "upstream")

	runGit(t, clone, "checkout", "feature-x")
	return origin, clone
}

// pushUpstreamBranchCommit lands one new commit directly on origin's
// 'upstream' branch via an ephemeral throwaway clone, mirroring
// pushUpstreamCommit's "advance main via a publisher" pattern but for
// 'upstream' — simulates another machine's capture landing on origin while
// this clone's local 'upstream' stays behind.
func pushUpstreamBranchCommit(t *testing.T, origin, filename, content, message string) {
	t.Helper()
	base := t.TempDir()
	pub := filepath.Join(base, "publisher")
	runGit(t, base, "clone", "--branch", "upstream", origin, pub)
	writeFile(t, filepath.Join(pub, filename), content)
	runGit(t, pub, "add", filename)
	runGit(t, pub, "commit", "-m", message)
	runGit(t, pub, "push", "origin", "upstream")
}

// runCapture runs `capture --target claude` with HOME pointed at an empty
// scratch directory, so $HOME/.claude/skills (TARGET_PATHS[claude]) never
// resolves to the real developer's skills — isolating the test from
// whatever happens to be on the machine running it. Combined with the empty
// manifest, managed_files() yields nothing, so this run's only possible
// effect on the 'upstream' branch is the reconcile step under test.
func runCapture(t *testing.T, overlayDir string) (string, int) {
	t.Helper()
	bin := realBackendBin(t)
	fakeHome := t.TempDir()
	cmd := exec.Command(bin, "capture", "--target", "claude")
	cmd.Dir = overlayDir
	cmd.Env = append(gitTestEnv(), "OVERLAY_DIR="+overlayDir, "HOME="+fakeHome)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), 0
	}
	ee, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("running %s capture: %v\n%s", bin, err, out)
	}
	return string(out), ee.ExitCode()
}

// TestCaptureBackend_ReconcilesStaleUpstreamBeforePush is the RED case: a
// local-only commit sits unpushed on 'upstream' (a prior capture that
// committed but never pushed) while origin/upstream has independently
// advanced with a different commit. Before the fix, capture's push fails
// non-fast-forward and the whole command exits nonzero even though nothing
// was actually lost. After the fix, capture reconciles (merges) local
// 'upstream' with origin/upstream before its own push, so both commits'
// content survive and the command exits 0.
func TestCaptureBackend_ReconcilesStaleUpstreamBeforePush(t *testing.T) {
	origin, clone := newScratchRepoWithUpstream(t)

	// Simulate a prior capture that committed locally but was never pushed.
	runGit(t, clone, "checkout", "upstream")
	writeFile(t, filepath.Join(clone, "local-only.txt"), "local capture, never pushed\n")
	runGit(t, clone, "add", "local-only.txt")
	runGit(t, clone, "commit", "-m", "upstream: sync from gentle-ai (local, unpushed)")
	runGit(t, clone, "checkout", "feature-x")

	// Simulate another machine's capture landing on origin/upstream in the
	// meantime — this clone's local 'upstream' is now behind AND ahead.
	pushUpstreamBranchCommit(t, origin, "remote-only.txt", "another machine's capture\n", "upstream: sync from gentle-ai (remote)")

	out, code := runCapture(t, clone)
	if code != 0 {
		t.Fatalf("capture exit=%d, want 0 (reconcile must recover a stale-but-mergeable upstream)\noutput:\n%s", code, out)
	}

	if got := currentBranch(t, clone); got != "feature-x" {
		t.Errorf("current branch = %q, want restored to feature-x", got)
	}

	runGit(t, clone, "fetch", "origin", "upstream")
	localUpstream := headRev(t, clone, "upstream")
	remoteUpstream := headRev(t, clone, "origin/upstream")
	if localUpstream != remoteUpstream {
		t.Errorf("local upstream (%s) != origin/upstream (%s) after capture; push did not converge", localUpstream, remoteUpstream)
	}

	// Both the local-only and the remote-only content must survive the
	// reconcile — a real merge, not a discard of either side.
	lsTree := runGit(t, clone, "ls-tree", "-r", "--name-only", "origin/upstream")
	if !strings.Contains(lsTree, "local-only.txt") {
		t.Errorf("origin/upstream lost the local-only capture's content:\n%s", lsTree)
	}
	if !strings.Contains(lsTree, "remote-only.txt") {
		t.Errorf("origin/upstream lost the other machine's capture content:\n%s", lsTree)
	}
}

// TestCaptureBackend_FastForwardsCleanlyWhenOnlyOriginAdvanced covers the
// common case (no local-only commit at all): local 'upstream' is simply
// behind origin/upstream. The reconcile step must fast-forward silently and
// capture must still succeed, without fabricating a merge commit where a
// fast-forward would do.
func TestCaptureBackend_FastForwardsCleanlyWhenOnlyOriginAdvanced(t *testing.T) {
	origin, clone := newScratchRepoWithUpstream(t)

	pushUpstreamBranchCommit(t, origin, "remote-only.txt", "another machine's capture\n", "upstream: sync from gentle-ai (remote)")

	out, code := runCapture(t, clone)
	if code != 0 {
		t.Fatalf("capture exit=%d, want 0\noutput:\n%s", code, out)
	}

	runGit(t, clone, "fetch", "origin", "upstream")
	localUpstream := headRev(t, clone, "upstream")
	remoteUpstream := headRev(t, clone, "origin/upstream")
	if localUpstream != remoteUpstream {
		t.Errorf("local upstream (%s) != origin/upstream (%s) after capture", localUpstream, remoteUpstream)
	}
}
