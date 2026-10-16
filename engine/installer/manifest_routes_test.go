package installer_test

// TC-INSTALLER: the routing of the manifest rows (the engine's `manifest routes`, which the overlay
// script asks) against the route contract from design D2/D3/D8. Unit tests, fast, always run: they
// ask the engine about fixture manifest rows and assert the emitted record fields; the target-flag
// tests intersect the emitted targets with a simulated --target selection and pin the D2
// applicability rule.

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/enginetest"
)

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
