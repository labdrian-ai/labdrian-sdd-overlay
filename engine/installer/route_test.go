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
	"bytes"
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
	Route   string            // "skill", "agent", or "opencode-agent"
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
	// We define all globals route_resolve needs, then eval-load just the
	// function. route_resolve also calls route_reject_unrouted_longterm_mem
	// (defined immediately above it), so both ranges are extracted.
	scriptFile := filepath.Join(t.TempDir(), "run_route_resolve.sh")
	script := fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail
OVERLAY_DIR=%q
MANIFEST=%q
HOME=%q
declare -A TARGET_PATHS=( [claude]=%q [opencode]=%q [codex]=%q )
declare -A AGENT_TARGET_PATHS=( [claude]=%q [opencode]=%q )
eval "$(awk '/^route_reject_unrouted_longterm_mem\(\)/,/^}$/ { print } /^route_resolve\(\)/,/^}$/ { print }' %q)"
route_resolve %q
`,
		overlayDir,
		manifestFile,
		home,
		filepath.Join(home, ".claude", "skills"),
		filepath.Join(home, ".config", "opencode", "skills"),
		filepath.Join(home, ".codex", "skills"),
		filepath.Join(home, ".claude", "agents"),
		filepath.Join(home, ".config", "opencode", "agents"),
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

// createTarball writes the provided relative paths into a temporary directory and
// returns a tar.gz path containing them under a `files/` root.
func createTarball(t *testing.T, files map[string]string) string {
	t.Helper()

	tarRoot := t.TempDir()
	for rel, body := range files {
		p := filepath.Join(tarRoot, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(p), err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatalf("write tar fixture %s: %v", rel, err)
		}
	}

	// Tarball naming can be deterministic per test path; keep this stable but
	// unique by placing it in a temp dir.
	tarPath := filepath.Join(t.TempDir(), "snapshot.tar.gz")
	cmd := exec.Command("tar", "-czf", tarPath, "-C", tarRoot, "files")
	if _, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create tarball: %v", err)
	}

	return tarPath
}

// setupCaptureFromBackupSandbox creates an overlay git repo + environment pair
// suitable for testing `overlay capture --from-backup`.
func setupCaptureFromBackupSandbox(t *testing.T, home string) (string, []string) {
	t.Helper()

	overlayDir := t.TempDir()

	const manifest = "test-skill/SKILL.md   managed\n" +
		"GADU.md   managed   agent\n" +
		"opencode/agents/GADU.md   managed   opencode-agent\n"

	files := map[string]string{
		"overlay.manifest":           manifest,
		"skills/test-skill/SKILL.md": "# overlay skill\n",
		"agents/GADU.md":             "---\nname: GADU\n---\n# overlay agent\n",
		"opencode/agents/GADU.md":    "---\nname: GADU\n---\n# overlay open-agent\n",
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

// fixtureManifest contains rows used by unit tests: a legacy skill row,
// the GADU skill row, and the GADU agent row.
const fixtureManifest = `sdd-spec/SKILL.md   managed
gadu-operator/SKILL.md   custom
GADU.md   custom   agent
opencode/agents/GADU.md   custom   opencode-agent
`

// mcpFixtureManifest extends fixtureManifest with the longterm-mem mcp
// sentinel row (D13).
const mcpFixtureManifest = fixtureManifest + "longterm-mem/go.mod   custom   mcp\n"

// unroutedLongtermMemFixture has a longterm-mem/** row with a missing third
// column (route-domain guard, R-012 in overlay-agent-route).
const unroutedLongtermMemFixture = fixtureManifest + "longterm-mem/internal/foo.go   custom\n"

// unrecognizedRouteLongtermMemFixture has a longterm-mem/** row whose third
// column is not in the recognized route domain.
const unrecognizedRouteLongtermMemFixture = fixtureManifest + "longterm-mem/internal/bar.go   custom   bogus\n"

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

// TestRouteResolve_GADUAgentRow verifies that the Claude Code GADU agent row (route=agent)
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

// TestRouteResolve_GADUOpenCodeAgentRow verifies that the OpenCode GADU agent
// row resolves to the OpenCode-specific generated source and exactly one
// OpenCode agent target.
func TestRouteResolve_GADUOpenCodeAgentRow(t *testing.T) {
	overlay := overlayScript(t)
	overlayDir := t.TempDir()
	home := t.TempDir()

	got, err := callRouteResolve(t, overlay, overlayDir, home, fixtureManifest, "opencode/agents/GADU.md")
	if err != nil {
		t.Fatalf("route_resolve: %v", err)
	}

	if got.Route != "opencode-agent" {
		t.Errorf("Route: got %q, want %q", got.Route, "opencode-agent")
	}
	if !strings.HasSuffix(got.RepoSrc, "opencode/agents/GADU.md") {
		t.Errorf("RepoSrc %q: want suffix opencode/agents/GADU.md", got.RepoSrc)
	}
	if len(got.Targets) != 1 {
		t.Errorf("expected exactly 1 target (opencode), got %d: %v", len(got.Targets), got.Targets)
	}
	dest, ok := got.Targets["opencode"]
	if !ok {
		t.Errorf("missing opencode target; got: %v", got.Targets)
	} else if !strings.HasSuffix(dest, ".config/opencode/agents/GADU.md") {
		t.Errorf("opencode dest %q: want suffix .config/opencode/agents/GADU.md", dest)
	}
	for _, tname := range []string{"claude", "codex"} {
		if _, ok := got.Targets[tname]; ok {
			t.Errorf("unexpected target %q for opencode-agent row", tname)
		}
	}
}

// TestRouteResolve_NonDeployableRows pins that structurally non-deployable rows
// — engine/*.go overlay-source rows and root-level files like skills.registry.yaml
// — produce an empty target set from route_resolve so that the deploy and sync
// loops skip them silently (no WARNING, no OVERLAY_NOT_DEPLOYED noise). The
// back-compat invariant for real skill rows is preserved.
func TestRouteResolve_NonDeployableRows(t *testing.T) {
	overlay := overlayScript(t)
	overlayDir := t.TempDir()
	home := t.TempDir()

	// Manifest with infra rows that must be non-deployable plus one real skill row.
	const infraManifest = `sdd-spec/SKILL.md   managed
engine/go.mod   managed
skills.registry.yaml   custom
`
	cases := []struct {
		name         string
		manifestPath string
		wantRoute    string
		wantTargets  int // expected number of deploy targets
	}{
		{
			name:         "engine_go_mod_non_deployable",
			manifestPath: "engine/go.mod",
			wantRoute:    "non-deployable",
			wantTargets:  0,
		},
		{
			name:         "skills_registry_yaml_non_deployable",
			manifestPath: "skills.registry.yaml",
			wantRoute:    "non-deployable",
			wantTargets:  0,
		},
		{
			// Back-compat: a real skill row must still resolve normally.
			name:         "sdd_spec_skill_back_compat",
			manifestPath: "sdd-spec/SKILL.md",
			wantRoute:    "skill",
			wantTargets:  3,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := callRouteResolve(t, overlay, overlayDir, home, infraManifest, tc.manifestPath)
			if err != nil {
				t.Fatalf("route_resolve(%q): %v", tc.manifestPath, err)
			}
			if got.Route != tc.wantRoute {
				t.Errorf("route_resolve(%q): route=%q, want %q", tc.manifestPath, got.Route, tc.wantRoute)
			}
			if len(got.Targets) != tc.wantTargets {
				t.Errorf("route_resolve(%q): %d targets, want %d: %v",
					tc.manifestPath, len(got.Targets), tc.wantTargets, got.Targets)
			}
		})
	}
}

// TestRouteResolve_McpRow verifies that a longterm-mem/** row routed "mcp"
// resolves to route=mcp, the repo source path as written (no skills/ or
// agents/ prefix rewriting), and zero copy targets — the longterm-mem
// install path (build, copy, register) is dispatched separately, not via the
// deploy loop. (overlay-agent-route R-006, traces longterm-mem R-013.)
func TestRouteResolve_McpRow(t *testing.T) {
	overlay := overlayScript(t)
	overlayDir := t.TempDir()
	home := t.TempDir()

	got, err := callRouteResolve(t, overlay, overlayDir, home, mcpFixtureManifest, "longterm-mem/go.mod")
	if err != nil {
		t.Fatalf("route_resolve: %v", err)
	}

	if got.Route != "mcp" {
		t.Errorf("Route: got %q, want %q", got.Route, "mcp")
	}
	if !strings.HasSuffix(got.RepoSrc, "longterm-mem/go.mod") {
		t.Errorf("RepoSrc %q: want suffix longterm-mem/go.mod", got.RepoSrc)
	}
	if len(got.Targets) != 0 {
		t.Errorf("expected 0 copy targets for mcp route, got %d: %v", len(got.Targets), got.Targets)
	}
}

// TestRouteResolve_OpencodeAgentUnaffected is the slice 9 regression guard:
// an existing opencode-agent-routed row must resolve and deploy exactly as
// before this change, with no regression from the mcp route addition.
func TestRouteResolve_OpencodeAgentUnaffected(t *testing.T) {
	overlay := overlayScript(t)
	overlayDir := t.TempDir()
	home := t.TempDir()

	got, err := callRouteResolve(t, overlay, overlayDir, home, mcpFixtureManifest, "opencode/agents/GADU.md")
	if err != nil {
		t.Fatalf("route_resolve: %v", err)
	}

	if got.Route != "opencode-agent" {
		t.Errorf("Route: got %q, want %q", got.Route, "opencode-agent")
	}
	if !strings.HasSuffix(got.RepoSrc, "opencode/agents/GADU.md") {
		t.Errorf("RepoSrc %q: want suffix opencode/agents/GADU.md", got.RepoSrc)
	}
	if len(got.Targets) != 1 {
		t.Errorf("expected exactly 1 target (opencode), got %d: %v", len(got.Targets), got.Targets)
	}
	dest, ok := got.Targets["opencode"]
	if !ok {
		t.Errorf("missing opencode target; got: %v", got.Targets)
	} else if !strings.HasSuffix(dest, ".config/opencode/agents/GADU.md") {
		t.Errorf("opencode dest %q: want suffix .config/opencode/agents/GADU.md", dest)
	}
}

// TestRouteResolve_UnroutedLongtermMemRowRejected pins that a longterm-mem/**
// row with a missing third column is rejected loudly (exit 1, explicit
// stderr naming the row) instead of silently falling through to route=skill.
// (overlay-agent-route R-012, traces longterm-mem R-035.)
func TestRouteResolve_UnroutedLongtermMemRowRejected(t *testing.T) {
	overlay := overlayScript(t)
	overlayDir := t.TempDir()
	home := t.TempDir()

	_, err := callRouteResolve(t, overlay, overlayDir, home, unroutedLongtermMemFixture, "longterm-mem/internal/foo.go")
	if err == nil {
		t.Fatal("expected route_resolve to reject a longterm-mem row with a missing route column, got nil error")
	}
	if !strings.Contains(err.Error(), "exit 1") {
		t.Errorf("expected exit 1, got: %v", err)
	}
	if !strings.Contains(err.Error(), "longterm-mem/internal/foo.go") {
		t.Errorf("expected error to name the rejected row, got: %v", err)
	}
}

// TestRouteResolve_UnrecognizedRouteLongtermMemRowRejected pins the same
// guard for a longterm-mem/** row whose third column holds a value outside
// {skill, agent, opencode-agent, mcp}.
func TestRouteResolve_UnrecognizedRouteLongtermMemRowRejected(t *testing.T) {
	overlay := overlayScript(t)
	overlayDir := t.TempDir()
	home := t.TempDir()

	_, err := callRouteResolve(t, overlay, overlayDir, home, unrecognizedRouteLongtermMemFixture, "longterm-mem/internal/bar.go")
	if err == nil {
		t.Fatal("expected route_resolve to reject a longterm-mem row with an unrecognized route, got nil error")
	}
	if !strings.Contains(err.Error(), "exit 1") {
		t.Errorf("expected exit 1, got: %v", err)
	}
	if !strings.Contains(err.Error(), "longterm-mem/internal/bar.go") {
		t.Errorf("expected error to name the rejected row, got: %v", err)
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

// TestRouteResolve_TargetFlag_AgentRowOpencode pins that the Claude Code agent
// row + --target opencode yields ZERO applicable targets.
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

// TestRouteResolve_TargetFlag_OpenCodeAgentRowOpencode pins that the OpenCode
// native agent row + --target opencode yields the OpenCode agent destination.
func TestRouteResolve_TargetFlag_OpenCodeAgentRowOpencode(t *testing.T) {
	overlay := overlayScript(t)
	overlayDir := t.TempDir()
	home := t.TempDir()

	got, err := callRouteResolve(t, overlay, overlayDir, home, fixtureManifest, "opencode/agents/GADU.md")
	if err != nil {
		t.Fatalf("route_resolve: %v", err)
	}

	applicable := intersectTargets(got.Targets, "opencode")
	if len(applicable) != 1 {
		t.Errorf("expected 1 applicable target (opencode), got: %v", applicable)
	}
	dest := applicable["opencode"]
	if !strings.HasSuffix(dest, ".config/opencode/agents/GADU.md") {
		t.Errorf("opencode dest %q: want suffix .config/opencode/agents/GADU.md", dest)
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

// TestCaptureFromBackup_UsesRouteAwareTarPaths asserts that managed rows are
// restored from the correct route-specific paths inside the snapshot.
func TestCaptureFromBackup_UsesRouteAwareTarPaths(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	overlay := overlayScript(t)
	home := t.TempDir()
	overlayDir, env := setupCaptureFromBackupSandbox(t, home)

	createdBackup := createTarball(t, map[string]string{
		"files/home/labdrian/.claude/skills/test-skill/SKILL.md": "# backup skill\n",
		"files/home/labdrian/.claude/agents/GADU.md":             "---\nname: GADU\n---\n# backup claude agent\n",
		"files/home/labdrian/.config/opencode/agents/GADU.md":    "---\ndescription: test agent\nmodel: openai/gpt-5.5\n---\n# backup opencode agent\n",
	})
	backup := filepath.Join(home, ".gentle-ai", "backups", "upgrade-20260615T175529Z", "snapshot.tar.gz")
	if err := os.MkdirAll(filepath.Dir(backup), 0755); err != nil {
		t.Fatalf("mkdir backup parent: %v", err)
	}
	backupBytes, err := os.ReadFile(createdBackup)
	if err != nil {
		t.Fatalf("read created backup: %v", err)
	}
	if err := os.WriteFile(backup, backupBytes, 0644); err != nil {
		t.Fatalf("write backup: %v", err)
	}

	out, err := runOverlay(t, overlay, env, "capture", "--from-backup", backup)
	if err != nil {
		t.Fatalf("overlay capture --from-backup: %v\noutput:\n%s", err, out)
	}

	assertFileEquals := func(relPath, want string) {
		cmd := exec.Command("git", "show", "upstream:"+relPath)
		cmd.Dir = overlayDir
		got, err := cmd.Output()
		if err != nil {
			t.Fatalf("git show upstream:%s: %v", relPath, err)
		}
		if string(got) != want {
			t.Fatalf("upstream:%s content mismatch:\n  got:  %q\n  want: %q\noutput:\n%s", relPath, string(got), want, out)
		}
	}

	assertFileEquals("skills/test-skill/SKILL.md", "# backup skill\n")
	assertFileEquals("agents/GADU.md", "---\nname: GADU\n---\n# backup claude agent\n")
	assertFileEquals("opencode/agents/GADU.md", "---\ndescription: test agent\nmodel: openai/gpt-5.5\n---\n# backup opencode agent\n")
}

// TestBootstrap_UsesRouteAwareTarPaths ensures bootstrap reads route-specific
// backup locations (including opencode-agent under ~/.config/opencode).
func TestBootstrap_UsesRouteAwareTarPaths(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	overlay := overlayScript(t)
	home := t.TempDir()
	overlayDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(overlayDir, "overlay.manifest"), []byte("test-skill/SKILL.md   managed\n"+
		"GADU.md   managed   agent\n"+
		"opencode/agents/GADU.md   managed   opencode-agent\n"), 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	scriptDst := filepath.Join(overlayDir, "bin", "overlay")
	if err := os.MkdirAll(filepath.Dir(scriptDst), 0755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	scriptBody, err := os.ReadFile(overlay)
	if err != nil {
		t.Fatalf("read overlay script: %v", err)
	}
	if err := os.WriteFile(scriptDst, scriptBody, 0755); err != nil {
		t.Fatalf("write overlay script: %v", err)
	}

	createdBackup := createTarball(t, map[string]string{
		"files/home/labdrian/.claude/skills/test-skill/SKILL.md": "# backup skill\n",
		"files/home/labdrian/.claude/agents/GADU.md":             "---\nname: GADU\n---\n# backup claude agent\n",
		"files/home/labdrian/.config/opencode/agents/GADU.md":    "---\ndescription: test agent\nmodel: openai/gpt-5.5\n---\n# backup opencode agent\n",
	})
	backup := filepath.Join(home, ".gentle-ai", "backups", "upgrade-20260615T175529Z", "snapshot.tar.gz")
	if err := os.MkdirAll(filepath.Dir(backup), 0755); err != nil {
		t.Fatalf("mkdir backup parent: %v", err)
	}
	backupBytes, err := os.ReadFile(createdBackup)
	if err != nil {
		t.Fatalf("read created backup: %v", err)
	}
	if err := os.WriteFile(backup, backupBytes, 0644); err != nil {
		t.Fatalf("write backup to default path: %v", err)
	}

	env := []string{
		"HOME=" + home,
		"OVERLAY_DIR=" + overlayDir,
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@test.com",
		"PATH=" + os.Getenv("PATH"),
	}

	out, err := runOverlay(t, overlay, env, "bootstrap")
	if err != nil {
		t.Fatalf("overlay bootstrap: %v\noutput:\n%s", err, out)
	}

	assertBranchFileEquals := func(branch, relPath, want string) {
		cmd := exec.Command("git", "show", branch+":"+relPath)
		cmd.Dir = overlayDir
		got, err := cmd.Output()
		if err != nil {
			t.Fatalf("git show %s:%s: %v", branch, relPath, err)
		}
		if string(got) != want {
			t.Fatalf("%s:%s content mismatch:\n  got:  %q\n  want: %q\noutput:\n%s", branch, relPath, string(got), want, out)
		}
	}

	assertBranchFileEquals("upstream", "skills/test-skill/SKILL.md", "# backup skill\n")
	assertBranchFileEquals("upstream", "agents/GADU.md", "---\nname: GADU\n---\n# backup claude agent\n")
	assertBranchFileEquals("upstream", "opencode/agents/GADU.md", "---\ndescription: test agent\nmodel: openai/gpt-5.5\n---\n# backup opencode agent\n")
	assertBranchFileEquals("main", "opencode/agents/GADU.md", "---\ndescription: test agent\nmodel: openai/gpt-5.5\n---\n# backup opencode agent\n")
}

// ---------------------------------------------------------------------------
// Integration tests — skip under -short
// ---------------------------------------------------------------------------

// setupSandboxOverlay creates a minimal overlay git repo with upstream+main
// branches containing native GADU agent files, skills/test-skill/SKILL.md, overlay.manifest.
// The sandbox HOME is provided by the caller. Returns overlayDir and the
// environment slice for exec.Command (includes OVERLAY_DIR and HOME overrides).
func setupSandboxOverlay(t *testing.T, home string) (string, []string) {
	t.Helper()
	overlayDir := t.TempDir()

	const (
		fixtureManifestInteg = "test-skill/SKILL.md   managed\n" +
			"inception-pipeline/SKILL.md   custom\n" +
			"_shared/pre-sdd-contracts.md   custom\n" +
			"_shared/entry-contract.schema.json   custom\n" +
			"_shared/actuals-record.schema.json   custom\n" +
			"GADU.md   custom   agent\n" +
			"opencode/agents/GADU.md   custom   opencode-agent\n"
		fixtureSkillContent         = "# test skill\n"
		fixtureInceptionContent     = "---\nname: inception-pipeline\n---\n# inception pipeline\n"
		fixtureContractsContent     = "# pre-SDD contracts\n"
		fixtureEntrySchemaContent   = "{\"title\":\"entry contract\"}\n"
		fixtureActualsSchemaContent = "{\"title\":\"actuals record\"}\n"
		fixtureAgentContent         = "---\nname: GADU\ndescription: test agent\nmodel: opus\ntools: '*'\n---\n# GADU\n"
		fixtureOpenCodeAgentContent = "---\ndescription: test agent\nmode: all\nmodel: openai/gpt-5.5\npermission:\n  task: allow\n---\n# GADU\n"
	)

	files := map[string]string{
		"overlay.manifest":                          fixtureManifestInteg,
		"skills/test-skill/SKILL.md":                fixtureSkillContent,
		"skills/inception-pipeline/SKILL.md":        fixtureInceptionContent,
		"skills/_shared/pre-sdd-contracts.md":       fixtureContractsContent,
		"skills/_shared/entry-contract.schema.json": fixtureEntrySchemaContent,
		"skills/_shared/actuals-record.schema.json": fixtureActualsSchemaContent,
		"agents/GADU.md":                            fixtureAgentContent,
		"opencode/agents/GADU.md":                   fixtureOpenCodeAgentContent,
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

// TestApply_AgentsLandInNativeAgentDirs asserts that after overlay apply:
// - agents/GADU.md lands at $HOME/.claude/agents/GADU.md (not .claude/skills)
// - opencode/agents/GADU.md lands at $HOME/.config/opencode/agents/GADU.md
// - skills/test-skill/SKILL.md lands in all 3 skills dirs
// - no skill file is in .claude/agents except GADU.md (AC-3)
func TestApply_AgentsLandInNativeAgentDirs(t *testing.T) {
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
	opencodeAgentDest := filepath.Join(home, ".config", "opencode", "agents", "GADU.md")
	if _, err := os.Stat(opencodeAgentDest); err != nil {
		t.Errorf("opencode agent file not found at %s: %v\napply output:\n%s", opencodeAgentDest, err, out)
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
// it reports OVERLAY_NOT_DEPLOYED, then a reapply restores the file and sync-check
// reports IN_SYNC again. (R-007, AC-4, D2 L511 correctness-critical)
//
// The restoration half (below the "Remove agent file" section) is the AC-5
// regression test referenced by docs/e1-durability-probe.md ("simulated sync →
// reapply → both artifacts present + managed"): it proves cmd_apply restores a
// drifted/missing agent artifact and cmd_sync_check reports it IN_SYNC afterward.
// This is a CHARACTERIZATION test, not a RED→GREEN cycle: the restoration
// behavior already exists (bin/labdrian-overlay copies unconditionally whenever
// the destination is absent or differs from the source — see cmd_apply's deploy
// loop — consulting no "already applied" ledger), so it is expected to, and did,
// PASS on its first run.
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
	// Check for per-file "OVERLAY_NOT_DEPLOYED: <path>" status lines only.
	// The VERDICT line always contains "OVERLAY_NOT_DEPLOYED=N" (with =), not ": ".
	if strings.Contains(outSync, "OVERLAY_NOT_DEPLOYED: ") {
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
	if !strings.Contains(outMissing, "OVERLAY_NOT_DEPLOYED: ") {
		t.Errorf("sync-check should report OVERLAY_NOT_DEPLOYED after agent file removed\noutput:\n%s", outMissing)
	}

	// Restoration half (AC-5): reapply must restore the missing agent file, and
	// sync-check must report IN_SYNC again afterward.
	if _, err := runOverlay(t, overlay, env, "apply", "--target", "all"); err != nil {
		t.Fatalf("overlay apply (reapply after removal): %v", err)
	}

	agentDest := filepath.Join(home, ".claude", "agents", "GADU.md")
	if _, err := os.Stat(agentDest); err != nil {
		t.Errorf("agent file not restored at %s after reapply: %v", agentDest, err)
	}

	outRestored, err := runOverlay(t, overlay, env, "sync-check", "--target", "claude")
	if err != nil {
		t.Fatalf("overlay sync-check (after reapply): %v\noutput:\n%s", err, outRestored)
	}
	if strings.Contains(outRestored, "OVERLAY_NOT_DEPLOYED: ") {
		t.Errorf("agent file still reported OVERLAY_NOT_DEPLOYED after reapply\noutput:\n%s", outRestored)
	}
	if !strings.Contains(outRestored, "IN_SYNC") {
		t.Errorf("expected IN_SYNC in sync-check output after reapply\noutput:\n%s", outRestored)
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

func TestEntryContractBundlePropagatesAndReportsIntegrityDrift(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	overlay := overlayScript(t)
	home := t.TempDir()
	overlayDir, env := setupSandboxOverlay(t, home)

	if out, err := runOverlay(t, overlay, env, "apply", "--target", "all"); err != nil {
		t.Fatalf("overlay apply: %v\noutput:\n%s", err, out)
	}

	assets := []string{
		"inception-pipeline/SKILL.md",
		"_shared/pre-sdd-contracts.md",
		"_shared/entry-contract.schema.json",
		"_shared/actuals-record.schema.json",
	}
	targetRoots := []string{
		filepath.Join(home, ".claude", "skills"),
		filepath.Join(home, ".config", "opencode", "skills"),
		filepath.Join(home, ".codex", "skills"),
	}
	for _, rel := range assets {
		want, err := os.ReadFile(filepath.Join(overlayDir, "skills", rel))
		if err != nil {
			t.Fatalf("read source asset %s: %v", rel, err)
		}
		for _, targetRoot := range targetRoots {
			got, err := os.ReadFile(filepath.Join(targetRoot, rel))
			if err != nil {
				t.Errorf("read deployed asset %s under %s: %v", rel, targetRoot, err)
				continue
			}
			if !bytes.Equal(got, want) {
				t.Errorf("deployed asset %s differs under %s", rel, targetRoot)
			}
		}
	}

	missingRel := "_shared/entry-contract.schema.json"
	driftedRel := "_shared/pre-sdd-contracts.md"
	opencodeRoot := filepath.Join(home, ".config", "opencode", "skills")
	if err := os.Remove(filepath.Join(opencodeRoot, missingRel)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(opencodeRoot, driftedRel), []byte("drifted\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	statusOut, err := runOverlay(t, overlay, env, "status", "--target", "opencode")
	if err != nil {
		t.Fatalf("overlay status: %v\noutput:\n%s", err, statusOut)
	}
	if !strings.Contains(statusOut, "MISSING in target: "+missingRel) {
		t.Errorf("status did not report missing shared asset\noutput:\n%s", statusOut)
	}
	if !strings.Contains(statusOut, "DIFFERS  : "+driftedRel) {
		t.Errorf("status did not report drifted shared asset\noutput:\n%s", statusOut)
	}

	syncOut, err := runOverlay(t, overlay, env, "sync-check", "--target", "opencode")
	if err != nil {
		t.Fatalf("overlay sync-check: %v\noutput:\n%s", err, syncOut)
	}
	if !strings.Contains(syncOut, "OVERLAY_NOT_DEPLOYED: skills/"+missingRel) {
		t.Errorf("sync-check did not report %s as not deployed\noutput:\n%s", missingRel, syncOut)
	}
	if !strings.Contains(syncOut, "UPSTREAM_CHANGED: skills/"+driftedRel) {
		t.Errorf("sync-check did not report %s as drifted\noutput:\n%s", driftedRel, syncOut)
	}
}

// TestGaduGenerate_ForwardsThroughWrapper validates that wrapper dispatch forwards
// gadu-generate to the engine binary with an explicit OVERLAY_DIR and preserves
// args (including --check).
func TestGaduGenerate_ForwardsThroughWrapper(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	// Missing engine must fail loudly first.
	overlay := overlayScript(t)
	home := t.TempDir()
	overlayDir := t.TempDir()
	env := []string{
		"HOME=" + home,
		"OVERLAY_DIR=" + overlayDir,
		"PATH=" + os.Getenv("PATH"),
	}

	if out, err := runOverlay(t, overlay, env, "gadu-generate", "--check"); err == nil {
		t.Fatalf("expected gadu-generate without binary to fail, but exit was 0\noutput:\n%s", out)
	} else if !strings.Contains(out, "engine binary not found") {
		t.Fatalf("expected missing-engine guidance, got: %s", out)
	}

	// Fake binary logs invocation details.
	fakeBinaryDir := filepath.Join(home, ".claude", "bin")
	fakeBinaryPath := filepath.Join(fakeBinaryDir, "gentle-ai-overlay")
	logPath := filepath.Join(home, "gadu-wrapper.log")
	if err := os.MkdirAll(fakeBinaryDir, 0755); err != nil {
		t.Fatalf("mkdir fake binary dir: %v", err)
	}
	fakeScript := "#!/usr/bin/env bash\n" +
		"set -euo pipefail\n" +
		"printf 'OVERLAY_DIR=%s\\n' \"$OVERLAY_DIR\" >" + logPath + "\n" +
		"printf 'ARGC:%s\\n' \"$#\" >>" + logPath + "\n" +
		"for arg in \"$@\"; do printf 'ARG:%s\\n' \"$arg\" >>" + logPath + "; done\n"
	if err := os.WriteFile(fakeBinaryPath, []byte(fakeScript), 0755); err != nil {
		t.Fatalf("write fake engine: %v", err)
	}

	env = append(env, "HOME="+home)
	out, err := runOverlay(t, overlay, env, "gadu-generate", "--check", "--future", "value with spaces")
	if err != nil {
		t.Fatalf("gadu-generate with fake engine failed: %v\noutput:\n%s", err, out)
	}

	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake engine log: %v", err)
	}
	gotLog := strings.TrimSpace(string(logged))
	wantPrefix := "OVERLAY_DIR=" + overlayDir
	if !strings.Contains(gotLog, wantPrefix) {
		t.Errorf("expected logged OVERLAY_DIR %q, got %q", wantPrefix, gotLog)
	}
	for _, want := range []string{
		"ARGC:4",
		"ARG:gadu-generate",
		"ARG:--check",
		"ARG:--future",
		"ARG:value with spaces",
	} {
		if !strings.Contains(gotLog, want) {
			t.Errorf("expected fake engine log to contain %q, got %q", want, gotLog)
		}
	}
}

// TestStatusHooks_IsReadOnlyAndFailLoudOnMissingBinary validates that status-hooks
// no longer builds the engine and still exits non-zero when missing.
func TestStatusHooks_IsReadOnlyAndFailLoudOnMissingBinary(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	overlay := overlayScript(t)
	home := t.TempDir()
	overlayDir := t.TempDir()
	env := []string{
		"HOME=" + home,
		"OVERLAY_DIR=" + overlayDir,
		"PATH=" + os.Getenv("PATH"),
	}

	out, err := runOverlay(t, overlay, env, "status-hooks")
	if err == nil {
		t.Fatalf("expected status-hooks without binary to fail, but exit was 0\noutput:\n%s", out)
	}
	if !strings.Contains(out, "run 'overlay install-hooks'") {
		t.Fatalf("expected install-hooks guidance, got: %s", out)
	}

	if _, err := os.Stat(filepath.Join(home, ".claude", "bin", "gentle-ai-overlay")); err == nil {
		t.Fatalf("status-hooks should not write or build binary at %s", filepath.Join(home, ".claude", "bin", "gentle-ai-overlay"))
	}
}

// TestOverlay_HelpContainsGaduGenerateAndStatusHooksGuidance asserts the updated
// inline usage text exposes the new command and read-only status-hooks contract.
func TestOverlay_HelpContainsGaduGenerateAndStatusHooksGuidance(t *testing.T) {
	overlay := overlayScript(t)
	home := t.TempDir()
	output, err := runOverlay(t, overlay, []string{"HOME=" + home, "PATH=" + os.Getenv("PATH")}, "--help")
	if err != nil {
		t.Fatalf("--help should exit 0: %v\noutput:\n%s", err, output)
	}

	if !strings.Contains(output, "gadu-generate [--check]") {
		t.Fatalf("expected gadu-generate command in help output, got: %s", output)
	}
	if !strings.Contains(output, "status-hooks") {
		t.Fatalf("expected status-hooks line in help output, got: %s", output)
	}
	if !strings.Contains(output, "read-only") {
		t.Fatalf("expected read-only status-hooks guidance in help output, got: %s", output)
	}
}

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

// TestStatusUninstall_SkipBuildStep proves R-014's second scenario
// positively: status and uninstall never invoke the longterm-mem build step
// (a status command that silently rebuilt would still exit 0 and pass a
// weaker "command succeeded" test, so this asserts byte-for-byte content
// and mtime invariance of a pre-placed binary instead). It also covers the
// binary-removal guard (10b.5, D4) end to end, including its dangerous
// negative case explicitly: uninstalling one target while another remains
// installed must leave the binary in place.
func TestStatusUninstall_SkipBuildStep(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	overlay := overlayScript(t)
	overlayDir := realOverlayDir(t)
	home := t.TempDir()
	goEnv := goToolchainEnv(t)

	newEnv := func(stateDir string) []string {
		return append([]string{
			"HOME=" + home,
			"OVERLAY_DIR=" + overlayDir,
			"STATE_DIR=" + stateDir,
			"PATH=" + os.Getenv("PATH"),
		}, goEnv...)
	}

	const dummyContent = "#!/bin/sh\necho dummy\n"

	placeDummyBinary := func(t *testing.T, stateDir string) (string, os.FileInfo) {
		t.Helper()
		binPath := filepath.Join(stateDir, "bin", "longterm-mem")
		if err := os.MkdirAll(filepath.Dir(binPath), 0o755); err != nil {
			t.Fatalf("mkdir bin dir: %v", err)
		}
		if err := os.WriteFile(binPath, []byte(dummyContent), 0o755); err != nil {
			t.Fatalf("write dummy binary: %v", err)
		}
		info, err := os.Stat(binPath)
		if err != nil {
			t.Fatalf("stat dummy binary: %v", err)
		}
		return binPath, info
	}

	seedInstalledTargets := func(t *testing.T, stateDir, content string) string {
		t.Helper()
		trackFile := filepath.Join(stateDir, "longterm-mem", "installed-targets")
		if err := os.MkdirAll(filepath.Dir(trackFile), 0o755); err != nil {
			t.Fatalf("mkdir track dir: %v", err)
		}
		if err := os.WriteFile(trackFile, []byte(content), 0o644); err != nil {
			t.Fatalf("seed track file: %v", err)
		}
		return trackFile
	}

	assertUnchanged := func(t *testing.T, binPath string, before os.FileInfo) {
		t.Helper()
		after, statErr := os.Stat(binPath)
		if statErr != nil {
			t.Fatalf("binary vanished: %v", statErr)
		}
		if !after.ModTime().Equal(before.ModTime()) || after.Size() != before.Size() {
			t.Fatalf("binary was rebuilt: mtime/size changed (before=%v/%d after=%v/%d)", before.ModTime(), before.Size(), after.ModTime(), after.Size())
		}
		content, err := os.ReadFile(binPath)
		if err != nil {
			t.Fatalf("read binary: %v", err)
		}
		if string(content) != dummyContent {
			t.Fatalf("binary content changed — it was rebuilt")
		}
	}

	t.Run("StatusNeverBuilds", func(t *testing.T) {
		stateDir := t.TempDir()
		binPath, before := placeDummyBinary(t, stateDir)

		out, err := runOverlay(t, overlay, newEnv(stateDir), "longterm-mem", "status")
		if err != nil {
			t.Fatalf("longterm-mem status failed: %v\noutput:\n%s", err, out)
		}

		assertUnchanged(t, binPath, before)
	})

	t.Run("UninstallSingleTargetSkipsBuildAndLeavesBinaryInPlace", func(t *testing.T) {
		stateDir := t.TempDir()
		binPath, before := placeDummyBinary(t, stateDir)
		// This is the dangerous case: opencode and codex are still tracked
		// installed. Uninstalling only claude must never remove a binary
		// those still-installed targets depend on.
		trackFile := seedInstalledTargets(t, stateDir, "claude\nopencode\ncodex\n")

		out, err := runOverlay(t, overlay, newEnv(stateDir), "longterm-mem", "uninstall", "--target", "claude")
		if err != nil {
			t.Fatalf("longterm-mem uninstall --target claude failed: %v\noutput:\n%s", err, out)
		}

		assertUnchanged(t, binPath, before)

		remaining, err := os.ReadFile(trackFile)
		if err != nil {
			t.Fatalf("read track file after uninstall: %v", err)
		}
		if strings.Contains(string(remaining), "claude") {
			t.Errorf("expected 'claude' removed from tracked installed targets, got: %q", remaining)
		}
		for _, want := range []string{"opencode", "codex"} {
			if !strings.Contains(string(remaining), want) {
				t.Errorf("expected %q to remain tracked installed, got: %q", want, remaining)
			}
		}
	})

	t.Run("UninstallLastTargetRemovesBinary", func(t *testing.T) {
		stateDir := t.TempDir()
		binPath, _ := placeDummyBinary(t, stateDir)
		// Only codex remains tracked — uninstalling it leaves ZERO
		// install-state targets, which must remove the binary (D4/10b.5).
		seedInstalledTargets(t, stateDir, "codex\n")

		out, err := runOverlay(t, overlay, newEnv(stateDir), "longterm-mem", "uninstall", "--target", "codex")
		if err != nil {
			t.Fatalf("longterm-mem uninstall --target codex failed: %v\noutput:\n%s", err, out)
		}

		if _, statErr := os.Stat(binPath); statErr == nil {
			t.Fatalf("expected binary removed once the last install-state target is gone, but it is still present\noutput:\n%s", out)
		} else if !os.IsNotExist(statErr) {
			t.Fatalf("unexpected stat error: %v", statErr)
		}
	})

	// An absent tracking file must FAIL CLOSED: it means "this entrypoint
	// does not know what is installed", never "nothing is installed", and
	// treating empty as safe-to-wipe destroys exactly what the guard
	// protects (review finding R3-uninstall-guard-fails-open).
	t.Run("UninstallWithNoTrackingFileLeavesBinaryInPlace", func(t *testing.T) {
		stateDir := t.TempDir()
		binPath, _ := placeDummyBinary(t, stateDir)
		// Deliberately no seedInstalledTargets: the file does not exist.
		out, err := runOverlay(t, overlay, newEnv(stateDir), "longterm-mem", "uninstall", "--target", "claude")
		if err != nil {
			t.Fatalf("longterm-mem uninstall --target claude failed: %v\noutput:\n%s", err, out)
		}

		if _, statErr := os.Stat(binPath); statErr != nil {
			t.Fatalf("the shared binary was removed although no tracking file existed to prove nothing else depends on it (stat err=%v)\noutput:\n%s", statErr, out)
		}
		if !strings.Contains(out, "--purge") {
			t.Errorf("the refusal should tell the operator that --purge is the explicit way to force removal; got:\n%s", out)
		}
	})
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

// TestInstall_UninstallRoundTripRemovesTheMcpEntry proves CRITICAL-1's fix
// end to end using the REAL longterm-mem binary, not the two-line dummy
// stub TestStatusUninstall_SkipBuildStep substitutes (that stub exits 0 for
// every invocation, which is exactly why a state-dir mismatch between the
// shell's register and unregister call sites was invisible to the suite).
// Install writes an ownership-tagged MCP entry into Claude Code's own
// configuration; uninstall — invoked through the exact bin/labdrian-overlay
// call site CRITICAL-1 found broken — must actually remove it, leaving the
// unrelated pre-existing entry untouched throughout.
func TestInstall_UninstallRoundTripRemovesTheMcpEntry(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	overlay := overlayScript(t)
	overlayDir := realOverlayDir(t)
	home := t.TempDir()
	// STATE_DIR is deliberately a directory OUTSIDE $HOME/.labdrian-overlay
	// — the exact condition that exposed CRITICAL-1: install and uninstall
	// must agree on where install-state.json lives (the module's own
	// default, ~/.labdrian-overlay/longterm-mem, D9) regardless of where
	// STATE_DIR (the shared binary's own location) happens to point.
	stateDir := t.TempDir()
	env := append([]string{
		"HOME=" + home,
		"OVERLAY_DIR=" + overlayDir,
		"STATE_DIR=" + stateDir,
		"PATH=" + os.Getenv("PATH"),
	}, goToolchainEnv(t)...)

	claudeConfig := filepath.Join(home, ".claude.json")
	const seedContent = `{"mcpServers":{"unrelated":{"type":"stdio","command":"/bin/true","args":[]}}}`
	if err := os.WriteFile(claudeConfig, []byte(seedContent), 0o600); err != nil {
		t.Fatalf("seed .claude.json: %v", err)
	}

	installOut, err := runOverlay(t, overlay, env, "longterm-mem", "install", "--target", "claude")
	if err != nil {
		t.Fatalf("longterm-mem install --target claude failed: %v\noutput:\n%s", err, installOut)
	}

	afterInstall, err := os.ReadFile(claudeConfig)
	if err != nil {
		t.Fatalf("read .claude.json after install: %v", err)
	}
	if !strings.Contains(string(afterInstall), `"longterm-mem"`) {
		t.Fatalf("expected an ownership-tagged longterm-mem MCP entry after install, got:\n%s", afterInstall)
	}
	if !strings.Contains(string(afterInstall), `"unrelated"`) {
		t.Fatalf("install must not disturb the unrelated pre-existing entry, got:\n%s", afterInstall)
	}

	uninstallOut, err := runOverlay(t, overlay, env, "longterm-mem", "uninstall", "--target", "claude")
	if err != nil {
		t.Fatalf("longterm-mem uninstall --target claude failed: %v\noutput:\n%s", err, uninstallOut)
	}

	afterUninstall, err := os.ReadFile(claudeConfig)
	if err != nil {
		t.Fatalf("read .claude.json after uninstall: %v", err)
	}
	if strings.Contains(string(afterUninstall), `"longterm-mem"`) {
		t.Fatalf("CRITICAL-1 regression: the longterm-mem MCP entry is still present after uninstall (register/unregister disagreed on install-state.json's location), got:\n%s", afterUninstall)
	}
	if !strings.Contains(string(afterUninstall), `"unrelated"`) {
		t.Fatalf("uninstall must not remove the unrelated pre-existing entry, got:\n%s", afterUninstall)
	}
}

// TestUninstall_HardFailureKeepsTrackingAndSharedBinary proves the other
// half of CRITICAL-1's fix: bin/labdrian-overlay inspects unregister's real
// exit code rather than swallowing it with `|| warn ... continuing`. Only
// exit 0 (removed) and exit 6 (unmanaged — an entry longterm-mem does not
// own) may clear this target's bash-level tracking; any other exit status
// must keep the target tracked and leave the shared binary in place, so the
// binary-removal guard can never fire for a target whose entry may still be
// present in its runtime config.
func TestUninstall_HardFailureKeepsTrackingAndSharedBinary(t *testing.T) {
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

	claudeConfig := filepath.Join(home, ".claude.json")
	if err := os.WriteFile(claudeConfig, []byte(`{"mcpServers":{}}`), 0o600); err != nil {
		t.Fatalf("seed .claude.json: %v", err)
	}

	installOut, err := runOverlay(t, overlay, env, "longterm-mem", "install", "--target", "claude")
	if err != nil {
		t.Fatalf("install failed: %v\noutput:\n%s", err, installOut)
	}

	// install-state.json lives at the module's own default state dir
	// (~/.labdrian-overlay/longterm-mem, D9), independent of $STATE_DIR.
	// Corrupt it so the next unregister call hits a REAL hard failure
	// (LoadInstallState's JSON parse error, exit 1), not merely "no
	// record" (exit 6, which is the recoverable, tracking-clearing case
	// this test must NOT trigger).
	installStatePath := filepath.Join(home, ".labdrian-overlay", "longterm-mem", "install-state.json")
	if _, statErr := os.Stat(installStatePath); statErr != nil {
		t.Fatalf("expected install-state.json at %s after install: %v", installStatePath, statErr)
	}
	if err := os.WriteFile(installStatePath, []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("corrupt install-state.json: %v", err)
	}

	binPath := filepath.Join(stateDir, "bin", "longterm-mem")
	binBefore, statErr := os.Stat(binPath)
	if statErr != nil {
		t.Fatalf("expected binary at %s after install: %v", binPath, statErr)
	}

	uninstallOut, err := runOverlay(t, overlay, env, "longterm-mem", "uninstall", "--target", "claude")
	if err == nil {
		t.Fatalf("expected uninstall to report failure (corrupted install-state.json forces unregister exit 1), got success\noutput:\n%s", uninstallOut)
	}

	binAfter, statErr := os.Stat(binPath)
	if statErr != nil {
		t.Fatalf("shared binary removed after a failed uninstall: %v", statErr)
	}
	if !binAfter.ModTime().Equal(binBefore.ModTime()) {
		t.Errorf("shared binary was rebuilt/replaced after a failed uninstall")
	}

	trackFile := filepath.Join(stateDir, "longterm-mem", "installed-targets")
	tracked, err := os.ReadFile(trackFile)
	if err != nil {
		t.Fatalf("read tracking file: %v", err)
	}
	if !strings.Contains(string(tracked), "claude") {
		t.Errorf("expected claude to remain tracked as installed after a failed uninstall, got: %q", tracked)
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
	out, err := runOverlay(t, overlay, env, "apply", "--target", "all")
	if err != nil {
		t.Fatalf("apply failed: %v\noutput:\n%s", err, out)
	}

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

		out, err := runOverlay(t, overlay, env, "apply", "--target", "all")
		if err != nil {
			t.Fatalf("apply failed: %v\noutput:\n%s", err, out)
		}
		if n := strings.Count(out, "running install once"); n != 0 {
			t.Fatalf("the install hook fired %d time(s) for a manifest with no mcp row\noutput:\n%s", n, out)
		}
	})
}

// TestUninstall_MissingBinaryStillConverges: an uninstall that cannot run
// the binary at all must still finish. Keeping the target tracked would
// wedge the run permanently -- every retry reproduces the same state, and
// the only escape (--purge) is the one that destroys the shared binary
// while leaving the entries it could have removed. A state no operator
// action can resolve must not be treated as a recoverable failure.
func TestUninstall_MissingBinaryStillConverges(t *testing.T) {
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

	claudeConfig := filepath.Join(home, ".claude.json")
	if err := os.WriteFile(claudeConfig, []byte(`{"mcpServers":{}}`), 0o600); err != nil {
		t.Fatalf("seed .claude.json: %v", err)
	}

	installOut, err := runOverlay(t, overlay, env, "longterm-mem", "install", "--target", "claude")
	if err != nil {
		t.Fatalf("install failed: %v\noutput:\n%s", err, installOut)
	}

	// The binary is gone by the time uninstall runs -- a wiped bin
	// directory, a partially restored backup, an interrupted upgrade.
	binPath := filepath.Join(stateDir, "bin", "longterm-mem")
	if err := os.Remove(binPath); err != nil {
		t.Fatalf("remove binary: %v", err)
	}

	uninstallOut, err := runOverlay(t, overlay, env, "longterm-mem", "uninstall", "--target", "claude")
	if err != nil {
		t.Fatalf("uninstall did not converge with the binary missing: %v\noutput:\n%s", err, uninstallOut)
	}
	if !strings.Contains(uninstallOut, "by hand") {
		t.Errorf("uninstall did not tell the operator the MCP entries were left behind:\n%s", uninstallOut)
	}

	trackFile := filepath.Join(stateDir, "longterm-mem", "installed-targets")
	tracked, err := os.ReadFile(trackFile)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read tracking file: %v", err)
	}
	if strings.Contains(string(tracked), "claude") {
		t.Errorf("claude stayed tracked after a converging uninstall, so the run can never finish; got: %q", tracked)
	}
}
