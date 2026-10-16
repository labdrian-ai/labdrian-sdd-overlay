package installer_test

// The commands that put longterm-mem into a home (`longterm-mem install`, and `apply` for the manifest
// row routed "mcp"), run through bin/labdrian-overlay with a sandbox HOME and STATE_DIR and a REAL
// `go build` of the real longterm-mem and engine modules (R-014/R-015): the binary lands at its fixed
// path, outlives the process that built it and keeps its path across inspections, and `apply` asks for
// one install. Unlike the synthetic sandboxes of the other tests, these run against the repository
// itself (overlaytest.RepoRoot): a real build needs the real modules, which only exist in the checked
// out repo. Only HOME and STATE_DIR are sandboxed, and no test here calls a git-mutating branch of
// apply or capture against that directory, so its git state is never touched. The helpers at the top
// (buildEnv, copyTree) are used by longterm_uninstall_test.go too.

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// buildEnv is the environment of a run that builds longterm-mem for real: the home and the repository
// the script is told of, a state directory of the test's, and the caches of the go command, so that
// the build reuses the warm module cache and does not start a cold one under the sandboxed home.
func buildEnv(t *testing.T, home, overlayDir, stateDir string) []string {
	t.Helper()
	return overlaytest.Env(home, overlayDir, append([]string{"STATE_DIR=" + stateDir}, overlaytest.ToolchainEnv(t)...)...)
}

// copyTree copies the directory tree rooted at src into dst (created if
// needed), preserving each file's permission bits. Used to seed the real
// longterm-mem/engine module sources into a synthetic git sandbox so
// cmd_apply's mcp-install hook (10b.7) can perform a REAL build the same way
// it would in production — exercising the whole success path, not just a
// refusal branch (the exact gap 10a's review flagged: "a refusal test
// proves a refusal; it says nothing about what happens when the command is
// allowed to run").
func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return relErr
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
	if err != nil {
		t.Fatalf("copy tree %s -> %s: %v", src, dst, err)
	}
}

// TestInstall_BuildsCopiesThenReportsPerRuntimeStatus proves R-014's first
// scenario: 'longterm-mem install --target all' builds and copies the
// binary to the fixed path, then reports a per-runtime status for claude,
// opencode, and codex — the real success path, not merely exit 0 (10a's
// review flagged exactly this gap for the sibling --component surface: "the
// only test reaching it returned at [a] refusal ... the result line on
// stdout ... shipped with zero externally observable assertions").
func TestInstall_BuildsCopiesThenReportsPerRuntimeStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	overlay := overlaytest.Script(t)
	overlayDir := overlaytest.RepoRoot(t)
	home := t.TempDir()
	stateDir := t.TempDir()
	env := buildEnv(t, home, overlayDir, stateDir)

	out, err := overlaytest.Combined(t, overlay, env, "", "longterm-mem", "install", "--target", "all")
	if err != nil {
		t.Fatalf("longterm-mem install --target all failed: %v\noutput:\n%s", err, out)
	}

	binPath := filepath.Join(stateDir, "bin", "longterm-mem")
	info, statErr := os.Stat(binPath)
	if statErr != nil {
		t.Fatalf("expected binary at %s after install, got: %v\noutput:\n%s", binPath, statErr, out)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("binary at %s is not executable: mode=%v", binPath, info.Mode())
	}

	for _, want := range []string{"claude:", "opencode:", "codex:"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected a per-runtime status line for %q, got:\n%s", want, out)
		}
	}
}

// TestInstall_BinaryPersistsAfterProcessExits proves R-015's first scenario:
// after the installing process (runOverlay's subprocess) has fully exited,
// the binary still exists at the documented fixed path and is invocable
// from a DIFFERENT process than the one that installed it.
func TestInstall_BinaryPersistsAfterProcessExits(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	overlay := overlaytest.Script(t)
	overlayDir := overlaytest.RepoRoot(t)
	home := t.TempDir()
	stateDir := t.TempDir()
	env := buildEnv(t, home, overlayDir, stateDir)

	out, err := overlaytest.Combined(t, overlay, env, "", "longterm-mem", "install", "--target", "all")
	if err != nil {
		t.Fatalf("install failed: %v\noutput:\n%s", err, out)
	}

	binPath := filepath.Join(stateDir, "bin", "longterm-mem")
	if _, statErr := os.Stat(binPath); statErr != nil {
		t.Fatalf("binary missing after the installing process exited: %v", statErr)
	}

	invokeOut, invokeErr := exec.Command(binPath).CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(invokeErr, &exitErr) {
		t.Fatalf("expected the binary to be invocable (real exit via its own usage path), got: %v\noutput:\n%s", invokeErr, invokeOut)
	}
	if exitErr.ExitCode() != 2 {
		t.Fatalf("expected exit code 2 (usage) invoking the binary with no args, got %d\noutput:\n%s", exitErr.ExitCode(), invokeOut)
	}
	if !strings.Contains(string(invokeOut), "usage: longterm-mem") {
		t.Fatalf("expected usage output invoking the binary, got: %s", invokeOut)
	}
}

// TestInstall_BinaryPathStableAcrossInspections proves R-015's second
// scenario: with no install/uninstall in progress, inspecting the binary at
// two points in time (across an intervening read-only 'status' call) shows
// the same file identity and the same mtime — the path never moves.
func TestInstall_BinaryPathStableAcrossInspections(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	overlay := overlaytest.Script(t)
	overlayDir := overlaytest.RepoRoot(t)
	home := t.TempDir()
	stateDir := t.TempDir()
	env := buildEnv(t, home, overlayDir, stateDir)

	out, err := overlaytest.Combined(t, overlay, env, "", "longterm-mem", "install", "--target", "all")
	if err != nil {
		t.Fatalf("install failed: %v\noutput:\n%s", err, out)
	}

	wantPath := filepath.Join(stateDir, "bin", "longterm-mem")
	first, statErr := os.Stat(wantPath)
	if statErr != nil {
		t.Fatalf("stat after install: %v", statErr)
	}

	statusOut, statusErr := overlaytest.Combined(t, overlay, env, "", "longterm-mem", "status")
	if statusErr != nil {
		t.Fatalf("status failed: %v\noutput:\n%s", statusErr, statusOut)
	}

	second, statErr := os.Stat(wantPath)
	if statErr != nil {
		t.Fatalf("stat after status: %v", statErr)
	}
	if !os.SameFile(first, second) {
		t.Fatalf("binary identity changed across inspections (path must be stable)")
	}
	if !second.ModTime().Equal(first.ModTime()) {
		t.Fatalf("binary mtime changed across inspections at %s: before=%v after=%v", wantPath, first.ModTime(), second.ModTime())
	}
}

// TestApply_InvokesLongtermMemInstallOnceForMcpRow proves cmd_apply's D13
// hook (10b.7): a manifest row routed "mcp" makes 'apply' invoke the
// longterm-mem install path exactly once — never once per deploy target —
// and exercises its REAL success path (binary built, per-runtime status
// reported), not merely a refusal branch.
func TestApply_InvokesLongtermMemInstallOnceForMcpRow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	repoRoot := overlaytest.RepoRoot(t)
	home := t.TempDir()
	overlayDir := t.TempDir()

	const manifest = "test-skill/SKILL.md   managed\n" +
		"longterm-mem/go.mod   custom   mcp\n"

	overlaytest.WriteFiles(t, overlayDir, map[string]string{
		"overlay.manifest":           manifest,
		"skills/test-skill/SKILL.md": "# overlay skill\n",
	})

	copyTree(t, filepath.Join(repoRoot, "engine"), filepath.Join(overlayDir, "engine"))
	copyTree(t, filepath.Join(repoRoot, "longterm-mem"), filepath.Join(overlayDir, "longterm-mem"))
	// longterm-mem requires identity through a local replace (../identity), so the build the hook
	// runs finds the module beside it as it does in the whole repository.
	copyTree(t, filepath.Join(repoRoot, "identity"), filepath.Join(overlayDir, "identity"))

	stateDir := t.TempDir()
	env := buildEnv(t, home, overlayDir, stateDir)
	// The engine is not provisioned in the home: the build the hook runs makes it from the sources copied above.
	git := func(args ...string) {
		t.Helper()
		overlaytest.Git(t, overlayDir, env, args...)
	}
	overlaytest.InitOverlay(git, "overlay.manifest", "skills/")

	overlay := overlaytest.Script(t)
	out := runApplyAllTolerantOfPi(t, overlay, env)

	binPath := filepath.Join(stateDir, "bin", "longterm-mem")
	if _, statErr := os.Stat(binPath); statErr != nil {
		t.Fatalf("expected longterm-mem binary built by apply's mcp hook: %v\noutput:\n%s", statErr, out)
	}
	for _, want := range []string{"claude:", "opencode:", "codex:"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected a per-runtime status line for %q in apply output, got:\n%s", want, out)
		}
	}

	// The hook's own marker line must appear exactly once per apply
	// invocation, regardless of how many deploy targets --target all
	// expands to in the OUTER per-target deploy loop (three, here) — a
	// wrongly-placed hook (inside that loop) would print it three times.
	count := strings.Count(out, "running install once")
	if count != 1 {
		t.Fatalf("expected the longterm-mem install hook to fire exactly once, got %d\noutput:\n%s", count, out)
	}

	// The guard's other direction, on the same fixture: with the mcp row
	// removed the hook must fire ZERO times. Without this, a regression
	// setting the flag unconditionally would still satisfy the "exactly
	// once" assertion above while paying for a Go build on every apply
	// (review finding R3-negative-case-unproved).
	t.Run("NoMcpRowSkipsTheInstallHook", func(t *testing.T) {
		manifestPath := filepath.Join(overlayDir, "overlay.manifest")
		if err := os.WriteFile(manifestPath, []byte("test-skill/SKILL.md   managed\n"), 0o644); err != nil {
			t.Fatalf("rewrite manifest without the mcp row: %v", err)
		}
		git("checkout", "upstream")
		git("add", "overlay.manifest")
		git("commit", "-m", "upstream: drop the mcp row")
		git("checkout", "main")

		out := runApplyAllTolerantOfPi(t, overlay, env)
		if n := strings.Count(out, "running install once"); n != 0 {
			t.Fatalf("the install hook fired %d time(s) for a manifest with no mcp row\noutput:\n%s", n, out)
		}
	})
}
