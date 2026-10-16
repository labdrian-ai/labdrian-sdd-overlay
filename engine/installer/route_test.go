package installer_test

// TC-INSTALLER: validates the routing of the manifest rows (the engine's
// `manifest routes`, which the overlay script asks) and the five installer
// commands against the route contract from design D2/D3/D8.
//
// Structure:
//   - Unit tests (fast, always run): ask the engine about fixture manifest
//     rows, assert the emitted record fields.
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

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/enginetest"
)

// overlayScript returns the absolute path to bin/labdrian-overlay, for the tests that run it (the
// integration tests below, sync_check_test.go and the routing pins). The unit tests of the routing
// ask the engine (callManifestRoutes) and do not need the script.
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

// routedRow holds what the routing says of one manifest row.
type routedRow struct {
	Route   string            // "skill", "agent", or "opencode-agent"
	RepoSrc string            // absolute repo source path
	Targets map[string]string // target_name -> absolute dest path
}

// callManifestRoutes asks the engine (`manifest routes`) for the row of manifestPath, once for each
// target that copies files, and puts the answers together: the route and source are the row's, and
// a target is among the Targets when the row goes somewhere for it. manifestContent is written to
// the manifest of overlayDir. The skills and agents directories handed over are the ones the script
// holds for the targets in a home of the given name.
func callManifestRoutes(t *testing.T, engine, overlayDir, home, manifestContent, manifestPath string) (routedRow, error) {
	t.Helper()

	manifestFile := filepath.Join(overlayDir, "overlay.manifest")
	if err := os.MkdirAll(overlayDir, 0755); err != nil {
		t.Fatalf("mkdir overlayDir: %v", err)
	}
	if err := os.WriteFile(manifestFile, []byte(manifestContent), 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	skills := map[string]string{
		"claude":   filepath.Join(home, ".claude", "skills"),
		"opencode": filepath.Join(home, ".config", "opencode", "skills"),
		"codex":    filepath.Join(home, ".codex", "skills"),
	}
	agents := map[string]string{
		"claude":   filepath.Join(home, ".claude", "agents"),
		"opencode": filepath.Join(home, ".config", "opencode", "agents"),
	}
	result := routedRow{Targets: map[string]string{}}
	found := false
	for _, target := range []string{"claude", "opencode", "codex"} {
		args := []string{"manifest", "routes", "--overlay-root", overlayDir, "--manifest", manifestFile,
			"--target", target, "--skills-root", skills[target]}
		if root, ok := agents[target]; ok {
			args = append(args, "--agents-root", root)
		}
		cmd := exec.Command(engine, args...)
		out, err := cmd.Output()
		if err != nil {
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				return routedRow{}, fmt.Errorf("manifest routes failed (exit %d): stderr: %s stdout: %s",
					ee.ExitCode(), ee.Stderr, out)
			}
			return routedRow{}, fmt.Errorf("exec: %w", err)
		}
		for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
			columns := strings.Split(line, "\t")
			if len(columns) != 5 {
				return routedRow{}, fmt.Errorf("expected 5 tab-separated columns, got %d: %q", len(columns), line)
			}
			if columns[0] != manifestPath {
				continue
			}
			result.Route, result.RepoSrc, found = columns[1], columns[3], true
			if columns[4] != "" {
				result.Targets[target] = columns[4]
			}
			break
		}
	}
	if !found {
		return routedRow{}, fmt.Errorf("manifest routes told nothing of %q", manifestPath)
	}
	return result, nil
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
	enginetest.Provision(t, home)
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

// TestManifestRoutes_LegacySkillRow pins the back-compat invariant (D2):
// a bare two-column skill row resolves to route=skill, skills/ repo source,
// and exactly the three skills destination paths.
func TestManifestRoutes_LegacySkillRow(t *testing.T) {
	engine := enginetest.Binary(t)
	overlayDir := t.TempDir()
	home := t.TempDir()

	got, err := callManifestRoutes(t, engine, overlayDir, home, fixtureManifest, "sdd-spec/SKILL.md")
	if err != nil {
		t.Fatalf("manifest routes: %v", err)
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

// TestManifestRoutes_GADUSkillRow verifies that the GADU skill row (no route column)
// resolves identically to a legacy skill row — 3 skills destinations.
func TestManifestRoutes_GADUSkillRow(t *testing.T) {
	engine := enginetest.Binary(t)
	overlayDir := t.TempDir()
	home := t.TempDir()

	got, err := callManifestRoutes(t, engine, overlayDir, home, fixtureManifest, "gadu-operator/SKILL.md")
	if err != nil {
		t.Fatalf("manifest routes: %v", err)
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

// TestManifestRoutes_GADUAgentRow verifies that the Claude Code GADU agent row (route=agent)
// resolves to route=agent, agents/ repo source, and exactly ONE claude target.
func TestManifestRoutes_GADUAgentRow(t *testing.T) {
	engine := enginetest.Binary(t)
	overlayDir := t.TempDir()
	home := t.TempDir()

	got, err := callManifestRoutes(t, engine, overlayDir, home, fixtureManifest, "GADU.md")
	if err != nil {
		t.Fatalf("manifest routes: %v", err)
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

// TestManifestRoutes_GADUOpenCodeAgentRow verifies that the OpenCode GADU agent
// row resolves to the OpenCode-specific generated source and exactly one
// OpenCode agent target.
func TestManifestRoutes_GADUOpenCodeAgentRow(t *testing.T) {
	engine := enginetest.Binary(t)
	overlayDir := t.TempDir()
	home := t.TempDir()

	got, err := callManifestRoutes(t, engine, overlayDir, home, fixtureManifest, "opencode/agents/GADU.md")
	if err != nil {
		t.Fatalf("manifest routes: %v", err)
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

// TestManifestRoutes_NonDeployableRows pins that structurally non-deployable rows
// — engine/*.go overlay-source rows and root-level files like skills.registry.yaml
// — produce an empty target set from manifest routes so that the deploy and sync
// loops skip them silently (no WARNING, no OVERLAY_NOT_DEPLOYED noise). The
// back-compat invariant for real skill rows is preserved.
func TestManifestRoutes_NonDeployableRows(t *testing.T) {
	engine := enginetest.Binary(t)
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
			got, err := callManifestRoutes(t, engine, overlayDir, home, infraManifest, tc.manifestPath)
			if err != nil {
				t.Fatalf("manifest routes(%q): %v", tc.manifestPath, err)
			}
			if got.Route != tc.wantRoute {
				t.Errorf("manifest routes(%q): route=%q, want %q", tc.manifestPath, got.Route, tc.wantRoute)
			}
			if len(got.Targets) != tc.wantTargets {
				t.Errorf("manifest routes(%q): %d targets, want %d: %v",
					tc.manifestPath, len(got.Targets), tc.wantTargets, got.Targets)
			}
		})
	}
}

// TestManifestRoutes_McpRow verifies that a longterm-mem/** row routed "mcp"
// resolves to route=mcp, the repo source path as written (no skills/ or
// agents/ prefix rewriting), and zero copy targets — the longterm-mem
// install path (build, copy, register) is dispatched separately, not via the
// deploy loop. (overlay-agent-route R-006, traces longterm-mem R-013.)
func TestManifestRoutes_McpRow(t *testing.T) {
	engine := enginetest.Binary(t)
	overlayDir := t.TempDir()
	home := t.TempDir()

	got, err := callManifestRoutes(t, engine, overlayDir, home, mcpFixtureManifest, "longterm-mem/go.mod")
	if err != nil {
		t.Fatalf("manifest routes: %v", err)
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

// TestManifestRoutes_OpencodeAgentUnaffected is the slice 9 regression guard:
// an existing opencode-agent-routed row must resolve and deploy exactly as
// before this change, with no regression from the mcp route addition.
func TestManifestRoutes_OpencodeAgentUnaffected(t *testing.T) {
	engine := enginetest.Binary(t)
	overlayDir := t.TempDir()
	home := t.TempDir()

	got, err := callManifestRoutes(t, engine, overlayDir, home, mcpFixtureManifest, "opencode/agents/GADU.md")
	if err != nil {
		t.Fatalf("manifest routes: %v", err)
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

// TestManifestRoutes_UnroutedLongtermMemRowRejected pins that a longterm-mem/**
// row with a missing third column is rejected loudly (exit 1, explicit
// stderr naming the row) instead of silently falling through to route=skill.
// (overlay-agent-route R-012, traces longterm-mem R-035.)
func TestManifestRoutes_UnroutedLongtermMemRowRejected(t *testing.T) {
	engine := enginetest.Binary(t)
	overlayDir := t.TempDir()
	home := t.TempDir()

	_, err := callManifestRoutes(t, engine, overlayDir, home, unroutedLongtermMemFixture, "longterm-mem/internal/foo.go")
	if err == nil {
		t.Fatal("expected manifest routes to reject a longterm-mem row with a missing route column, got nil error")
	}
	if !strings.Contains(err.Error(), "exit 1") {
		t.Errorf("expected exit 1, got: %v", err)
	}
	if !strings.Contains(err.Error(), "longterm-mem/internal/foo.go") {
		t.Errorf("expected error to name the rejected row, got: %v", err)
	}
}

// TestManifestRoutes_UnrecognizedRouteLongtermMemRowRejected pins the same
// guard for a longterm-mem/** row whose third column holds a value outside
// {skill, agent, opencode-agent, mcp}.
func TestManifestRoutes_UnrecognizedRouteLongtermMemRowRejected(t *testing.T) {
	engine := enginetest.Binary(t)
	overlayDir := t.TempDir()
	home := t.TempDir()

	_, err := callManifestRoutes(t, engine, overlayDir, home, unrecognizedRouteLongtermMemFixture, "longterm-mem/internal/bar.go")
	if err == nil {
		t.Fatal("expected manifest routes to reject a longterm-mem row with an unrecognized route, got nil error")
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

// intersectTargets returns the subset of manifest routes targets that match the
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

// TestManifestRoutes_TargetFlag_AgentRowOpencode pins that the Claude Code agent
// row + --target opencode yields ZERO applicable targets.
func TestManifestRoutes_TargetFlag_AgentRowOpencode(t *testing.T) {
	engine := enginetest.Binary(t)
	overlayDir := t.TempDir()
	home := t.TempDir()

	got, err := callManifestRoutes(t, engine, overlayDir, home, fixtureManifest, "GADU.md")
	if err != nil {
		t.Fatalf("manifest routes: %v", err)
	}

	applicable := intersectTargets(got.Targets, "opencode")
	if len(applicable) != 0 {
		t.Errorf("expected 0 applicable targets (agent row + --target opencode), got: %v", applicable)
	}
}

// TestManifestRoutes_TargetFlag_OpenCodeAgentRowOpencode pins that the OpenCode
// native agent row + --target opencode yields the OpenCode agent destination.
func TestManifestRoutes_TargetFlag_OpenCodeAgentRowOpencode(t *testing.T) {
	engine := enginetest.Binary(t)
	overlayDir := t.TempDir()
	home := t.TempDir()

	got, err := callManifestRoutes(t, engine, overlayDir, home, fixtureManifest, "opencode/agents/GADU.md")
	if err != nil {
		t.Fatalf("manifest routes: %v", err)
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

// TestManifestRoutes_TargetFlag_AgentRowClaude pins that agent row + --target claude
// yields exactly claude:~/.claude/agents/GADU.md.
func TestManifestRoutes_TargetFlag_AgentRowClaude(t *testing.T) {
	engine := enginetest.Binary(t)
	overlayDir := t.TempDir()
	home := t.TempDir()

	got, err := callManifestRoutes(t, engine, overlayDir, home, fixtureManifest, "GADU.md")
	if err != nil {
		t.Fatalf("manifest routes: %v", err)
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

// TestManifestRoutes_TargetFlag_SkillRowOpencode pins that a skill row + --target opencode
// yields exactly one opencode skills destination (D2 back-compat).
func TestManifestRoutes_TargetFlag_SkillRowOpencode(t *testing.T) {
	engine := enginetest.Binary(t)
	overlayDir := t.TempDir()
	home := t.TempDir()

	got, err := callManifestRoutes(t, engine, overlayDir, home, fixtureManifest, "sdd-spec/SKILL.md")
	if err != nil {
		t.Fatalf("manifest routes: %v", err)
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
	enginetest.Provision(t, home)

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
	enginetest.Provision(t, home)
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

// runApplyAllTolerantOfPi runs `apply --target all` and tolerates ONLY the
// now-intentional non-zero exit caused by pi's own known incompleteness
// (R4-silent-package-skip, pi-target-plumbing / pi-package-build): apply's
// aggregate exit reflects pi honestly (build failed in this fixture
// environment, which has no built engine binary at ENGINE_BINARY, so pi
// never actually gets deployed, exit 1) so unattended automation cannot
// read a run that silently skipped pi as a clean success. These fixtures
// exercise the claude/opencode/codex deploy loop under --target all, not
// pi's own build, so a pi-only failure must not fail them -- but any OTHER
// failure still must.
func runApplyAllTolerantOfPi(t *testing.T, overlay string, env []string) string {
	t.Helper()
	out, err := runOverlay(t, overlay, env, "apply", "--target", "all")
	if err != nil && !strings.Contains(out, "pi: package build failed") {
		t.Fatalf("apply --target all failed for a reason other than pi's known incompleteness: %v\noutput:\n%s", err, out)
	}
	return out
}

// runStatusAllTolerantOfPi is runApplyAllTolerantOfPi's sibling for `status
// --target all`, which now fails the same way for the same reason
// (R4-silent-package-skip) -- the pi package is never built in these
// fixtures, so pipkg_status_and_report honestly reports "not built".
func runStatusAllTolerantOfPi(t *testing.T, overlay string, env []string) string {
	t.Helper()
	out, err := runOverlay(t, overlay, env, "status", "--target", "all")
	if err != nil && !strings.Contains(out, "pi: not built") {
		t.Fatalf("status --target all failed for a reason other than pi's known incompleteness: %v\noutput:\n%s", err, out)
	}
	return out
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

	out := runApplyAllTolerantOfPi(t, overlay, env)

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

	runApplyAllTolerantOfPi(t, overlay, env)

	out := runStatusAllTolerantOfPi(t, overlay, env)

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
	runApplyAllTolerantOfPi(t, overlay, env)

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
	runApplyAllTolerantOfPi(t, overlay, env)

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

	runApplyAllTolerantOfPi(t, overlay, env)

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

	runApplyAllTolerantOfPi(t, overlay, env)

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
