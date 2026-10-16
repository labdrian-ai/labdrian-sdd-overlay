package installer_test

// The commands that put longterm-mem into a home (`longterm-mem install`, and `apply` for the manifest
// row routed "mcp"), run through bin/labdrian-overlay with a sandbox HOME and STATE_DIR and a REAL
// `go build` of the real longterm-mem and engine modules (R-014/R-015): the binary lands at its fixed
// path, outlives the process that built it and keeps its path across inspections, and `apply` asks for
// one install. The helpers at the top (realOverlayDir, goToolchainEnv, copyTree) are used by
// longterm_uninstall_test.go too.

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// longterm-mem install/status/uninstall — Slice 10b (R-014 shell half, R-015)
// ---------------------------------------------------------------------------

// realOverlayDir returns the actual repository root (two levels up from
// engine/installer, same resolution overlayScript already uses). Unlike the
// synthetic sandboxes used elsewhere in this file, longterm-mem install must
// perform a REAL `go build` of the REAL longterm-mem and engine modules
// (R-014/R-015) — those only exist in the checked-out repo itself. Only
// HOME/STATE_DIR are sandboxed via t.TempDir(); these tests never call any
// git-mutating cmd_apply/cmd_capture branch against this directory, so its
// real git state is never touched.
func realOverlayDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	p, err := filepath.Abs(filepath.Join(wd, "..", ".."))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	if _, err := os.Stat(filepath.Join(p, "longterm-mem", "go.mod")); err != nil {
		t.Fatalf("longterm-mem module not found under %s: %v", p, err)
	}
	return p
}

// goToolchainEnv forwards this host's resolved GOCACHE/GOMODCACHE/GOPATH so
// a sandboxed subprocess's real `go build` calls reuse the warm build and
// module cache instead of a cold, network-dependent rebuild under a fresh
// sandboxed HOME (longterm-mem has real third-party dependencies; a cold,
// unforwarded module cache could otherwise require network access).
func goToolchainEnv(t *testing.T) []string {
	t.Helper()
	out, err := exec.Command("go", "env", "GOCACHE", "GOMODCACHE", "GOPATH").Output()
	if err != nil {
		t.Fatalf("go env: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 go env lines, got %d: %q", len(lines), out)
	}
	return []string{
		"GOCACHE=" + lines[0],
		"GOMODCACHE=" + lines[1],
		"GOPATH=" + lines[2],
	}
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

	overlay := overlayScript(t)
	overlayDir := realOverlayDir(t)
	home := t.TempDir()
	stateDir := t.TempDir()
	env := append([]string{
		"HOME=" + home,
		"OVERLAY_DIR=" + overlayDir,
		"STATE_DIR=" + stateDir,
		"PATH=" + os.Getenv("PATH"),
	}, goToolchainEnv(t)...)

	out, err := runOverlay(t, overlay, env, "longterm-mem", "install", "--target", "all")
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

	overlay := overlayScript(t)
	overlayDir := realOverlayDir(t)
	home := t.TempDir()
	stateDir := t.TempDir()
	env := append([]string{
		"HOME=" + home,
		"OVERLAY_DIR=" + overlayDir,
		"STATE_DIR=" + stateDir,
		"PATH=" + os.Getenv("PATH"),
	}, goToolchainEnv(t)...)

	out, err := runOverlay(t, overlay, env, "longterm-mem", "install", "--target", "all")
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

	overlay := overlayScript(t)
	overlayDir := realOverlayDir(t)
	home := t.TempDir()
	stateDir := t.TempDir()
	env := append([]string{
		"HOME=" + home,
		"OVERLAY_DIR=" + overlayDir,
		"STATE_DIR=" + stateDir,
		"PATH=" + os.Getenv("PATH"),
	}, goToolchainEnv(t)...)

	out, err := runOverlay(t, overlay, env, "longterm-mem", "install", "--target", "all")
	if err != nil {
		t.Fatalf("install failed: %v\noutput:\n%s", err, out)
	}

	wantPath := filepath.Join(stateDir, "bin", "longterm-mem")
	first, statErr := os.Stat(wantPath)
	if statErr != nil {
		t.Fatalf("stat after install: %v", statErr)
	}

	statusOut, statusErr := runOverlay(t, overlay, env, "longterm-mem", "status")
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

	repoRoot := realOverlayDir(t)
	home := t.TempDir()
	overlayDir := t.TempDir()

	const manifest = "test-skill/SKILL.md   managed\n" +
		"longterm-mem/go.mod   custom   mcp\n"

	files := map[string]string{
		"overlay.manifest":           manifest,
		"skills/test-skill/SKILL.md": "# overlay skill\n",
	}
	for rel, content := range files {
		p := filepath.Join(overlayDir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(p), err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	copyTree(t, filepath.Join(repoRoot, "engine"), filepath.Join(overlayDir, "engine"))
	copyTree(t, filepath.Join(repoRoot, "longterm-mem"), filepath.Join(overlayDir, "longterm-mem"))
	// longterm-mem requires identity through a local replace (../identity), so the build the hook
	// runs finds the module beside it as it does in the whole repository.
	copyTree(t, filepath.Join(repoRoot, "identity"), filepath.Join(overlayDir, "identity"))

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
	runGit(overlayDir, "init")
	runGit(overlayDir, "config", "user.email", "test@test.com")
	runGit(overlayDir, "config", "user.name", "test")
	runGit(overlayDir, "checkout", "-b", "upstream")
	runGit(overlayDir, "add", "overlay.manifest", "skills/")
	runGit(overlayDir, "commit", "-m", "upstream: baseline")
	runGit(overlayDir, "checkout", "-b", "main")

	stateDir := t.TempDir()
	env := append([]string{
		"HOME=" + home,
		"OVERLAY_DIR=" + overlayDir,
		"STATE_DIR=" + stateDir,
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@test.com",
		"PATH=" + os.Getenv("PATH"),
	}, goToolchainEnv(t)...)

	overlay := overlayScript(t)
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
		runGit(overlayDir, "checkout", "upstream")
		runGit(overlayDir, "add", "overlay.manifest")
		runGit(overlayDir, "commit", "-m", "upstream: drop the mcp row")
		runGit(overlayDir, "checkout", "main")

		out := runApplyAllTolerantOfPi(t, overlay, env)
		if n := strings.Count(out, "running install once"); n != 0 {
			t.Fatalf("the install hook fired %d time(s) for a manifest with no mcp row\noutput:\n%s", n, out)
		}
	})
}
