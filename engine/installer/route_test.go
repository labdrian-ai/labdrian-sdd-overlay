package installer_test

// TC-INSTALLER: validates the route_resolve bash helper and the five
// installer commands against the route contract from design D2/D3/D8.
//
// Structure:
//   - Unit tests (fast, always run): source route_resolve from the overlay
//     script, call it with fixture manifest rows, assert emitted record fields.
//   - Target-flag unit tests (fast): intersect emitted targets with a simulated
//     --target selection; pin the D2 applicability rule.
//   - Integration tests (skip on -short): run the full overlay apply/status/
//     sync-check commands against a git-repo sandbox with a sandbox HOME.

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// overlayScript returns the absolute path to bin/labdrian-overlay.
// Go tests run with cwd = engine/installer/, so two levels up → repo root.
func overlayScript(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	p, err := filepath.Abs(filepath.Join(wd, "..", "..", "bin", "labdrian-overlay"))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("overlay script not found at %s: %v", p, err)
	}
	return p
}

// resolveResult holds the parsed output of a single route_resolve call.
type resolveResult struct {
	Route   string            // "skill" or "agent"
	RepoSrc string            // absolute repo source path
	Targets map[string]string // target_name -> absolute dest path
}

// callRouteResolve sources route_resolve from the overlay script using eval+awk
// and calls it with manifestPath. manifestContent is written to a temp manifest.
// overlayDir is used as OVERLAY_DIR; home is used as HOME.
func callRouteResolve(t *testing.T, overlayPath, overlayDir, home, manifestContent, manifestPath string) (resolveResult, error) {
	t.Helper()

	manifestFile := filepath.Join(overlayDir, "overlay.manifest")
	if err := os.MkdirAll(overlayDir, 0755); err != nil {
		t.Fatalf("mkdir overlayDir: %v", err)
	}
	if err := os.WriteFile(manifestFile, []byte(manifestContent), 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	// minimal: forced — need to eval-extract route_resolve without running
	// the full overlay script dispatch (which exits 1 for empty command).
	// We define all globals route_resolve needs, then eval-load just the function.
	scriptFile := filepath.Join(t.TempDir(), "run_route_resolve.sh")
	script := fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail
OVERLAY_DIR=%q
MANIFEST=%q
HOME=%q
declare -A TARGET_PATHS=( [claude]=%q [opencode]=%q [codex]=%q )
declare -A AGENT_TARGET_PATHS=( [claude]=%q )
eval "$(awk '/^route_resolve\(\)/,/^}$/' %q)"
route_resolve %q
`,
		overlayDir,
		manifestFile,
		home,
		filepath.Join(home, ".claude", "skills"),
		filepath.Join(home, ".config", "opencode", "skills"),
		filepath.Join(home, ".codex", "skills"),
		filepath.Join(home, ".claude", "agents"),
		overlayPath,
		manifestPath,
	)
	if err := os.WriteFile(scriptFile, []byte(script), 0755); err != nil {
		t.Fatalf("write runner script: %v", err)
	}

	cmd := exec.Command("bash", scriptFile)
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return resolveResult{}, fmt.Errorf("route_resolve failed (exit %d): stderr: %s stdout: %s",
				ee.ExitCode(), ee.Stderr, out)
		}
		return resolveResult{}, fmt.Errorf("exec: %w", err)
	}

	line := strings.TrimRight(string(out), "\n")
	parts := strings.SplitN(line, "\t", 3)
	if len(parts) != 3 {
		return resolveResult{}, fmt.Errorf("expected 3 tab-separated fields, got %d: %q", len(parts), line)
	}

	targets := make(map[string]string)
	for _, pair := range strings.Fields(parts[2]) {
		idx := strings.Index(pair, ":")
		if idx < 0 {
			return resolveResult{}, fmt.Errorf("malformed target pair: %q", pair)
		}
		targets[pair[:idx]] = pair[idx+1:]
	}

	return resolveResult{
		Route:   parts[0],
		RepoSrc: parts[1],
		Targets: targets,
	}, nil
}

// fixtureManifest contains rows used by unit tests: a legacy skill row,
// the GADU skill row, and the GADU agent row.
const fixtureManifest = `sdd-spec/SKILL.md   managed
gadu-operator/SKILL.md   custom
GADU.md   custom   agent
`

// ---------------------------------------------------------------------------
// Unit tests — fast, always run
// ---------------------------------------------------------------------------

// TestRouteResolve_LegacySkillRow pins the back-compat invariant (D2):
// a bare two-column skill row resolves to route=skill, skills/ repo source,
// and exactly the three skills destination paths.
func TestRouteResolve_LegacySkillRow(t *testing.T) {
	overlay := overlayScript(t)
	overlayDir := t.TempDir()
	home := t.TempDir()

	got, err := callRouteResolve(t, overlay, overlayDir, home, fixtureManifest, "sdd-spec/SKILL.md")
	if err != nil {
		t.Fatalf("route_resolve: %v", err)
	}

	if got.Route != "skill" {
		t.Errorf("Route: got %q, want %q", got.Route, "skill")
	}
	if !strings.HasSuffix(got.RepoSrc, "skills/sdd-spec/SKILL.md") {
		t.Errorf("RepoSrc %q: want suffix skills/sdd-spec/SKILL.md", got.RepoSrc)
	}
	// Back-compat: exactly 3 skills targets, no agent targets
	if len(got.Targets) != 3 {
		t.Errorf("expected 3 targets, got %d: %v", len(got.Targets), got.Targets)
	}
	for _, tname := range []string{"claude", "opencode", "codex"} {
		dest, ok := got.Targets[tname]
		if !ok {
			t.Errorf("missing target %q", tname)
			continue
		}
		if !strings.HasSuffix(dest, "skills/sdd-spec/SKILL.md") {
			t.Errorf("target %q dest %q: want suffix skills/sdd-spec/SKILL.md", tname, dest)
		}
	}
}

// TestRouteResolve_GADUSkillRow verifies that the GADU skill row (no route column)
// resolves identically to a legacy skill row — 3 skills destinations.
func TestRouteResolve_GADUSkillRow(t *testing.T) {
	overlay := overlayScript(t)
	overlayDir := t.TempDir()
	home := t.TempDir()

	got, err := callRouteResolve(t, overlay, overlayDir, home, fixtureManifest, "gadu-operator/SKILL.md")
	if err != nil {
		t.Fatalf("route_resolve: %v", err)
	}

	if got.Route != "skill" {
		t.Errorf("Route: got %q, want %q", got.Route, "skill")
	}
	if !strings.HasSuffix(got.RepoSrc, "skills/gadu-operator/SKILL.md") {
		t.Errorf("RepoSrc %q: want suffix skills/gadu-operator/SKILL.md", got.RepoSrc)
	}
	if len(got.Targets) != 3 {
		t.Errorf("expected 3 targets, got %d: %v", len(got.Targets), got.Targets)
	}
	for _, tname := range []string{"claude", "opencode", "codex"} {
		dest, ok := got.Targets[tname]
		if !ok {
			t.Errorf("missing target %q", tname)
			continue
		}
		if !strings.HasSuffix(dest, "skills/gadu-operator/SKILL.md") {
			t.Errorf("target %q dest %q: want suffix skills/gadu-operator/SKILL.md", tname, dest)
		}
	}
}

// TestRouteResolve_GADUAgentRow verifies that the GADU agent row (route=agent)
// resolves to route=agent, agents/ repo source, and exactly ONE claude target.
func TestRouteResolve_GADUAgentRow(t *testing.T) {
	overlay := overlayScript(t)
	overlayDir := t.TempDir()
	home := t.TempDir()

	got, err := callRouteResolve(t, overlay, overlayDir, home, fixtureManifest, "GADU.md")
	if err != nil {
		t.Fatalf("route_resolve: %v", err)
	}

	if got.Route != "agent" {
		t.Errorf("Route: got %q, want %q", got.Route, "agent")
	}
	if !strings.HasSuffix(got.RepoSrc, "agents/GADU.md") {
		t.Errorf("RepoSrc %q: want suffix agents/GADU.md", got.RepoSrc)
	}
	// Exactly one target: claude only
	if len(got.Targets) != 1 {
		t.Errorf("expected exactly 1 target (claude), got %d: %v", len(got.Targets), got.Targets)
	}
	dest, ok := got.Targets["claude"]
	if !ok {
		t.Errorf("missing claude target; got: %v", got.Targets)
	} else if !strings.HasSuffix(dest, ".claude/agents/GADU.md") {
		t.Errorf("claude dest %q: want suffix .claude/agents/GADU.md", dest)
	}
	for _, tname := range []string{"opencode", "codex"} {
		if _, ok := got.Targets[tname]; ok {
			t.Errorf("unexpected target %q for agent row", tname)
		}
	}
}

// ---------------------------------------------------------------------------
// Target-flag unit tests — pin the D2 --target intersection rule
// ---------------------------------------------------------------------------

// intersectTargets returns the subset of route_resolve targets that match the
// requested --target selection, mirroring the call-site logic in cmd_apply/status/sync-check.
func intersectTargets(targets map[string]string, requested string) map[string]string {
	if requested == "all" {
		return targets
	}
	if dest, ok := targets[requested]; ok {
		return map[string]string{requested: dest}
	}
	return map[string]string{}
}

// TestRouteResolve_TargetFlag_AgentRowOpencode pins that an agent row + --target opencode
// yields ZERO applicable targets (agent route is claude-only, D2).
func TestRouteResolve_TargetFlag_AgentRowOpencode(t *testing.T) {
	overlay := overlayScript(t)
	overlayDir := t.TempDir()
	home := t.TempDir()

	got, err := callRouteResolve(t, overlay, overlayDir, home, fixtureManifest, "GADU.md")
	if err != nil {
		t.Fatalf("route_resolve: %v", err)
	}

	applicable := intersectTargets(got.Targets, "opencode")
	if len(applicable) != 0 {
		t.Errorf("expected 0 applicable targets (agent row + --target opencode), got: %v", applicable)
	}
}

// TestRouteResolve_TargetFlag_AgentRowClaude pins that agent row + --target claude
// yields exactly claude:~/.claude/agents/GADU.md.
func TestRouteResolve_TargetFlag_AgentRowClaude(t *testing.T) {
	overlay := overlayScript(t)
	overlayDir := t.TempDir()
	home := t.TempDir()

	got, err := callRouteResolve(t, overlay, overlayDir, home, fixtureManifest, "GADU.md")
	if err != nil {
		t.Fatalf("route_resolve: %v", err)
	}

	applicable := intersectTargets(got.Targets, "claude")
	if len(applicable) != 1 {
		t.Errorf("expected 1 applicable target (claude), got: %v", applicable)
	}
	dest := applicable["claude"]
	if !strings.HasSuffix(dest, ".claude/agents/GADU.md") {
		t.Errorf("claude dest %q: want suffix .claude/agents/GADU.md", dest)
	}
}

// TestRouteResolve_TargetFlag_SkillRowOpencode pins that a skill row + --target opencode
// yields exactly one opencode skills destination (D2 back-compat).
func TestRouteResolve_TargetFlag_SkillRowOpencode(t *testing.T) {
	overlay := overlayScript(t)
	overlayDir := t.TempDir()
	home := t.TempDir()

	got, err := callRouteResolve(t, overlay, overlayDir, home, fixtureManifest, "sdd-spec/SKILL.md")
	if err != nil {
		t.Fatalf("route_resolve: %v", err)
	}

	applicable := intersectTargets(got.Targets, "opencode")
	if len(applicable) != 1 {
		t.Errorf("expected 1 applicable target (opencode), got: %v", applicable)
	}
	dest := applicable["opencode"]
	if !strings.Contains(dest, "opencode") || !strings.HasSuffix(dest, "skills/sdd-spec/SKILL.md") {
		t.Errorf("opencode dest %q: want opencode skills path for sdd-spec/SKILL.md", dest)
	}
}

// ---------------------------------------------------------------------------
// Integration tests — skip under -short
// ---------------------------------------------------------------------------

// setupSandboxOverlay creates a minimal overlay git repo with upstream+main
// branches containing: agents/GADU.md, skills/test-skill/SKILL.md, overlay.manifest.
// The sandbox HOME is provided by the caller. Returns overlayDir and the
// environment slice for exec.Command (includes OVERLAY_DIR and HOME overrides).
func setupSandboxOverlay(t *testing.T, home string) (string, []string) {
	t.Helper()
	overlayDir := t.TempDir()

	const (
		fixtureManifestInteg = "test-skill/SKILL.md   managed\nGADU.md   custom   agent\n"
		fixtureSkillContent  = "# test skill\n"
		fixtureAgentContent  = "---\nname: GADU\ndescription: test agent\nmodel: opus\ntools: '*'\n---\n# GADU\n"
	)

	files := map[string]string{
		"overlay.manifest":           fixtureManifestInteg,
		"skills/test-skill/SKILL.md": fixtureSkillContent,
		"agents/GADU.md":             fixtureAgentContent,
	}
	for rel, content := range files {
		p := filepath.Join(overlayDir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(p), err)
		}
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

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
	runGit(overlayDir, "add", ".")
	runGit(overlayDir, "commit", "-m", "upstream: baseline")
	// main starts at the same commit as upstream — git merge upstream is a no-op
	runGit(overlayDir, "checkout", "-b", "main")

	env := []string{
		"HOME=" + home,
		"OVERLAY_DIR=" + overlayDir,
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@test.com",
		"PATH=" + os.Getenv("PATH"),
	}
	return overlayDir, env
}

// runOverlay executes the overlay script with the given args and env.
// It returns combined stdout+stderr and any error.
func runOverlay(t *testing.T, overlayPath string, env []string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("bash", append([]string{overlayPath}, args...)...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// TestApply_AgentLandsInClaudeAgents asserts that after overlay apply:
// - agents/GADU.md lands at $HOME/.claude/agents/GADU.md (not .claude/skills)
// - skills/test-skill/SKILL.md lands in all 3 skills dirs
// - no skill file is in .claude/agents except GADU.md (AC-3)
func TestApply_AgentLandsInClaudeAgents(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	overlay := overlayScript(t)
	home := t.TempDir()
	_, env := setupSandboxOverlay(t, home)

	out, err := runOverlay(t, overlay, env, "apply", "--target", "all")
	if err != nil {
		t.Fatalf("overlay apply: %v\noutput:\n%s", err, out)
	}

	agentDest := filepath.Join(home, ".claude", "agents", "GADU.md")
	if _, err := os.Stat(agentDest); err != nil {
		t.Errorf("agent file not found at %s: %v\napply output:\n%s", agentDest, err, out)
	}

	skillDests := []string{
		filepath.Join(home, ".claude", "skills", "test-skill", "SKILL.md"),
		filepath.Join(home, ".config", "opencode", "skills", "test-skill", "SKILL.md"),
		filepath.Join(home, ".codex", "skills", "test-skill", "SKILL.md"),
	}
	for _, p := range skillDests {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("skill not deployed at %s: %v", p, err)
		}
	}

	// No skill file should be in .claude/agents (only GADU.md)
	agentsDir := filepath.Join(home, ".claude", "agents")
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		t.Fatalf("readdir %s: %v", agentsDir, err)
	}
	for _, e := range entries {
		if e.Name() != "GADU.md" {
			t.Errorf("unexpected file in .claude/agents: %s", e.Name())
		}
	}
}

// TestStatus_ReportsAgentFile asserts that overlay status references the GADU
// agent row under agents/ not under skills/ (R-007, AC-4).
func TestStatus_ReportsAgentFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	overlay := overlayScript(t)
	home := t.TempDir()
	_, env := setupSandboxOverlay(t, home)

	if _, err := runOverlay(t, overlay, env, "apply", "--target", "all"); err != nil {
		t.Fatalf("overlay apply: %v", err)
	}

	out, err := runOverlay(t, overlay, env, "status", "--target", "all")
	if err != nil {
		t.Fatalf("overlay status: %v\noutput:\n%s", err, out)
	}

	// GADU row must NOT be reported under a skills path
	if strings.Contains(out, ".claude/skills/GADU.md") ||
		strings.Contains(out, "skills/GADU.md") {
		t.Errorf("status incorrectly references GADU under skills path\noutput:\n%s", out)
	}
}

// TestSyncCheck_DetectsMissingAgentFile pins the L511 fix: when an agent file IS
// deployed, sync-check reports IN_SYNC (not OVERLAY_NOT_DEPLOYED). After removal
// it reports OVERLAY_NOT_DEPLOYED. (R-007, AC-4, D2 L511 correctness-critical)
func TestSyncCheck_DetectsMissingAgentFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	overlay := overlayScript(t)
	home := t.TempDir()
	_, env := setupSandboxOverlay(t, home)

	// Deploy first
	if _, err := runOverlay(t, overlay, env, "apply", "--target", "all"); err != nil {
		t.Fatalf("overlay apply: %v", err)
	}

	// Deployed agent row must report IN_SYNC — pins L511 fix
	outSync, err := runOverlay(t, overlay, env, "sync-check", "--target", "claude")
	if err != nil {
		t.Fatalf("overlay sync-check (after deploy): %v\noutput:\n%s", err, outSync)
	}
	if strings.Contains(outSync, "OVERLAY_NOT_DEPLOYED") {
		t.Errorf("L511 regression: sync-check reports OVERLAY_NOT_DEPLOYED for deployed agent file\noutput:\n%s", outSync)
	}
	if !strings.Contains(outSync, "IN_SYNC") {
		t.Errorf("expected IN_SYNC in sync-check output after deploy\noutput:\n%s", outSync)
	}

	// Remove agent file and re-check — must report OVERLAY_NOT_DEPLOYED
	if err := os.Remove(filepath.Join(home, ".claude", "agents", "GADU.md")); err != nil {
		t.Fatalf("remove GADU.md: %v", err)
	}

	outMissing, _ := runOverlay(t, overlay, env, "sync-check", "--target", "claude")
	if !strings.Contains(outMissing, "OVERLAY_NOT_DEPLOYED") {
		t.Errorf("sync-check should report OVERLAY_NOT_DEPLOYED after agent file removed\noutput:\n%s", outMissing)
	}
}

// TestUnrelatedSkillUnchanged verifies that an unrelated skill deploys byte-identically
// to all three skills destinations (D2 backward-compat invariant).
func TestUnrelatedSkillUnchanged(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	overlay := overlayScript(t)
	home := t.TempDir()
	overlayDir, env := setupSandboxOverlay(t, home)

	if _, err := runOverlay(t, overlay, env, "apply", "--target", "all"); err != nil {
		t.Fatalf("overlay apply: %v", err)
	}

	srcContent, err := os.ReadFile(filepath.Join(overlayDir, "skills", "test-skill", "SKILL.md"))
	if err != nil {
		t.Fatalf("read source skill: %v", err)
	}

	dests := []string{
		filepath.Join(home, ".claude", "skills", "test-skill", "SKILL.md"),
		filepath.Join(home, ".config", "opencode", "skills", "test-skill", "SKILL.md"),
		filepath.Join(home, ".codex", "skills", "test-skill", "SKILL.md"),
	}
	for _, p := range dests {
		got, err := os.ReadFile(p)
		if err != nil {
			t.Errorf("read deployed skill at %s: %v", p, err)
			continue
		}
		if string(got) != string(srcContent) {
			t.Errorf("deployed skill content differs from source at %s", p)
		}
	}
}
