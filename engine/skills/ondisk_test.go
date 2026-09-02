package skills

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The manifest rows used across these tests mirror the real overlay.manifest
// shape: bare two-column skill rows, engine/* infra rows, a root-level row with
// no path separator, and three-column agent rows.
const routingManifest = `# comment line
engine/go.mod managed
engine/skills/sync.go managed
sdd-spec/SKILL.md managed
prespec-malandra/SKILL.md custom
prespec-malandra/references/coverage-taxonomy.md custom
_shared/minimalism-contract.md custom

skills.registry.yaml custom
GADU.md                  custom   agent
sdd-explore.md           managed  agent
opencode/agents/GADU.md custom   opencode-agent
`

func TestDeployableManifestPaths(t *testing.T) {
	got, err := DeployableManifestPaths(strings.NewReader(routingManifest))
	if err != nil {
		t.Fatalf("DeployableManifestPaths: unexpected error: %v", err)
	}

	// Only rows that route_resolve sends to skills/ belong here.
	want := []string{
		"sdd-spec/SKILL.md",
		"prespec-malandra/SKILL.md",
		"prespec-malandra/references/coverage-taxonomy.md",
		"_shared/minimalism-contract.md",
	}
	for _, w := range want {
		if _, ok := got[w]; !ok {
			t.Errorf("DeployableManifestPaths: missing deployable row %q", w)
		}
	}

	// engine/* is infra, a path with no separator is root-level, and the two
	// agent routes land outside skills/. None may be treated as deployable.
	excluded := []string{
		"engine/go.mod",
		"engine/skills/sync.go",
		"skills.registry.yaml",
		"GADU.md",
		"sdd-explore.md",
		"opencode/agents/GADU.md",
	}
	for _, e := range excluded {
		if _, ok := got[e]; ok {
			t.Errorf("DeployableManifestPaths: %q is not deployable to skills/ but was included", e)
		}
	}

	if len(got) != len(want) {
		t.Errorf("DeployableManifestPaths: got %d rows, want %d: %v", len(got), len(want), got)
	}
}

// TestDeployableManifestPaths_ExcludesMcpRoute pins that an mcp-routed row
// (D13: longterm-mem's install path, dispatched to build+copy+register rather
// than to any skills destination) is excluded from the on-disk deployable set,
// the same way agent/opencode-agent rows already are.
func TestDeployableManifestPaths_ExcludesMcpRoute(t *testing.T) {
	const manifest = `longterm-mem/go.mod  custom  mcp
sdd-spec/SKILL.md managed
`
	got, err := DeployableManifestPaths(strings.NewReader(manifest))
	if err != nil {
		t.Fatalf("DeployableManifestPaths: unexpected error: %v", err)
	}
	if _, ok := got["longterm-mem/go.mod"]; ok {
		t.Errorf("DeployableManifestPaths: mcp-routed row %q must be excluded, but was included: %v", "longterm-mem/go.mod", got)
	}
	if _, ok := got["sdd-spec/SKILL.md"]; !ok {
		t.Errorf("DeployableManifestPaths: skill row must remain deployable: %v", got)
	}
}

// TestDeployableManifestPaths_RejectsUnroutedLongtermMemRow pins CRITICAL-2's
// remediation: a row under longterm-mem/** with a missing third column must
// be rejected with an explicit error, not silently resolved to a
// skills-destination path (overlay-agent-route R-012, traces longterm-mem
// R-035). The bash half of this guard (route_reject_unrouted_longterm_mem)
// already existed; this pins the Go half that was missing.
func TestDeployableManifestPaths_RejectsUnroutedLongtermMemRow(t *testing.T) {
	const manifest = `longterm-mem/internal/foo.go managed
sdd-spec/SKILL.md managed
`
	got, err := DeployableManifestPaths(strings.NewReader(manifest))
	if err == nil {
		t.Fatalf("DeployableManifestPaths: expected an error for an unrouted longterm-mem/** row, got nil (paths=%v)", got)
	}
	if !strings.Contains(err.Error(), "longterm-mem/internal/foo.go") {
		t.Errorf("DeployableManifestPaths: error must name the rejected row, got: %v", err)
	}
	if _, ok := got["longterm-mem/internal/foo.go"]; ok {
		t.Errorf("DeployableManifestPaths: an unrouted longterm-mem row must never resolve to a skills-destination path, got: %v", got)
	}
}

// TestDeployableManifestPaths_RejectsUnrecognizedRouteLongtermMemRow pins the
// same guard for a longterm-mem/** row whose third column holds a value
// outside {skill, agent, opencode-agent, mcp}.
func TestDeployableManifestPaths_RejectsUnrecognizedRouteLongtermMemRow(t *testing.T) {
	const manifest = `longterm-mem/internal/bar.go custom bogus-route
sdd-spec/SKILL.md managed
`
	got, err := DeployableManifestPaths(strings.NewReader(manifest))
	if err == nil {
		t.Fatalf("DeployableManifestPaths: expected an error for an unrecognized-route longterm-mem/** row, got nil (paths=%v)", got)
	}
	if !strings.Contains(err.Error(), "longterm-mem/internal/bar.go") {
		t.Errorf("DeployableManifestPaths: error must name the rejected row, got: %v", err)
	}
	if _, ok := got["longterm-mem/internal/bar.go"]; ok {
		t.Errorf("DeployableManifestPaths: an unrecognized-route longterm-mem row must never resolve to a skills-destination path, got: %v", got)
	}
}

// TestDeployableManifestPaths_LongtermMemRouteGuardAcceptsEveryValidRoute
// proves the guard is scoped exactly to the four-value route domain: every
// recognized route on a longterm-mem/** row parses without error (a
// skill-routed longterm-mem row is unusual but not itself invalid — R-012
// only forbids MISSING or UNRECOGNIZED routes).
func TestDeployableManifestPaths_LongtermMemRouteGuardAcceptsEveryValidRoute(t *testing.T) {
	for _, route := range []string{"skill", "agent", "opencode-agent", "mcp"} {
		manifest := "longterm-mem/valid.go custom " + route + "\n"
		if _, err := DeployableManifestPaths(strings.NewReader(manifest)); err != nil {
			t.Errorf("DeployableManifestPaths: route %q on a longterm-mem/** row must be accepted, got error: %v", route, err)
		}
	}
}

func TestScanSkillFiles(t *testing.T) {
	root := t.TempDir()
	files := []string{
		"alpha/SKILL.md",
		"alpha/references/one.md",
		"_shared/contract.md",
		"beta/SKILL.md",
	}
	for _, f := range files {
		p := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", f, err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
	}

	got, err := ScanSkillFiles(root)
	if err != nil {
		t.Fatalf("ScanSkillFiles: unexpected error: %v", err)
	}

	// Sorted, slash-separated, relative to root — directories are not entries.
	want := []string{
		"_shared/contract.md",
		"alpha/SKILL.md",
		"alpha/references/one.md",
		"beta/SKILL.md",
	}
	if len(got) != len(want) {
		t.Fatalf("ScanSkillFiles: got %d entries %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ScanSkillFiles[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestScanSkillFilesMissingDir(t *testing.T) {
	if _, err := ScanSkillFiles(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("ScanSkillFiles: expected an error for a missing directory, got nil")
	}
}

// TestDiffOnDiskCatchesUnregisteredSkill is the regression test for the class of
// bug that shipped anti-generic-design undeployed: a skill present on disk and in
// Git but absent from overlay.manifest. Registry-vs-manifest cross-checking
// cannot see it, because a skill missing from BOTH files reads as aligned.
func TestDiffOnDiskCatchesUnregisteredSkill(t *testing.T) {
	disk := []string{
		"registered/SKILL.md",
		"unregistered/SKILL.md",
		"unregistered/references/detail.md",
	}
	manifest := map[string]struct{}{
		"registered/SKILL.md": {},
	}

	divs := DiffOnDisk(disk, manifest)
	if len(divs) != 2 {
		t.Fatalf("DiffOnDisk: got %d divergences %v, want 2", len(divs), divs)
	}
	for _, d := range divs {
		if d.Class != DivUnregisteredOnDisk {
			t.Errorf("DiffOnDisk: class = %q, want %q", d.Class, DivUnregisteredOnDisk)
		}
		if !strings.HasPrefix(d.Path, "unregistered/") {
			t.Errorf("DiffOnDisk: unexpected path %q flagged", d.Path)
		}
	}
}

// TestDiffOnDiskCatchesManifestRowWithNoFile covers the symmetric failure: a
// manifest row whose source file does not exist makes the deploy step fail.
func TestDiffOnDiskCatchesManifestRowWithNoFile(t *testing.T) {
	disk := []string{"present/SKILL.md"}
	manifest := map[string]struct{}{
		"present/SKILL.md": {},
		"ghost/SKILL.md":   {},
	}

	divs := DiffOnDisk(disk, manifest)
	if len(divs) != 1 {
		t.Fatalf("DiffOnDisk: got %d divergences %v, want 1", len(divs), divs)
	}
	if divs[0].Class != DivMissingOnDisk {
		t.Errorf("DiffOnDisk: class = %q, want %q", divs[0].Class, DivMissingOnDisk)
	}
	if divs[0].Path != "ghost/SKILL.md" {
		t.Errorf("DiffOnDisk: path = %q, want %q", divs[0].Path, "ghost/SKILL.md")
	}
}

func TestDiffOnDiskClean(t *testing.T) {
	disk := []string{"a/SKILL.md", "a/references/r.md"}
	manifest := map[string]struct{}{
		"a/SKILL.md":        {},
		"a/references/r.md": {},
	}
	if divs := DiffOnDisk(disk, manifest); len(divs) != 0 {
		t.Fatalf("DiffOnDisk: got %d divergences %v, want none", len(divs), divs)
	}
}

// TestRepositorySkillsAreFullyRegistered is the live invariant over this
// repository: every file under skills/ must have a deploying overlay.manifest
// row, and every deploying row must have a file. This is the guard that would
// have failed while anti-generic-design sat on disk unregistered, at a time when
// apply, sync-check and skills validate all reported green.
func TestRepositorySkillsAreFullyRegistered(t *testing.T) {
	root := skillsRepoRoot(t)

	manifestFile, err := os.Open(filepath.Join(root, "overlay.manifest"))
	if err != nil {
		t.Fatalf("open overlay.manifest: %v", err)
	}
	defer manifestFile.Close()

	rows, err := DeployableManifestPaths(manifestFile)
	if err != nil {
		t.Fatalf("DeployableManifestPaths: %v", err)
	}

	disk, err := ScanSkillFiles(filepath.Join(root, "skills"))
	if err != nil {
		t.Fatalf("ScanSkillFiles: %v", err)
	}

	if divs := DiffOnDisk(disk, rows); len(divs) != 0 {
		var b strings.Builder
		for _, d := range divs {
			b.WriteString("\n  [" + string(d.Class) + "] " + d.Path + ": " + d.Detail)
		}
		t.Fatalf("skills/ and overlay.manifest disagree — %d divergence(s):%s", len(divs), b.String())
	}
}

// skillsRepoRoot resolves the repository root from this package's test working
// directory: engine/skills/ → engine/ → repo root.
func skillsRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Join(wd, "..", "..")
}

// bashAcceptsLongtermMemRoute runs the real bin/labdrian-overlay
// route_resolve (plus its route_reject_unrouted_longterm_mem guard) against a
// longterm-mem/** row whose third column is set to route, and reports whether
// bash accepted the row (exit 0) or rejected it (exit 1, per R-012 in
// overlay-agent-route). This drives the parity check from the real script
// rather than a second hardcoded list.
func bashAcceptsLongtermMemRoute(t *testing.T, overlayPath, route string) bool {
	t.Helper()

	overlayDir := t.TempDir()
	home := t.TempDir()
	rowPath := "longterm-mem/parity-fixture.txt"
	manifest := rowPath + "   custom   " + route + "\n"
	manifestFile := filepath.Join(overlayDir, "overlay.manifest")
	if err := os.WriteFile(manifestFile, []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	scriptFile := filepath.Join(t.TempDir(), "run_route_resolve.sh")
	script := "#!/usr/bin/env bash\n" +
		"set -euo pipefail\n" +
		"OVERLAY_DIR=" + shellQuote(overlayDir) + "\n" +
		"MANIFEST=" + shellQuote(manifestFile) + "\n" +
		"HOME=" + shellQuote(home) + "\n" +
		"declare -A TARGET_PATHS=( [claude]=" + shellQuote(filepath.Join(home, ".claude", "skills")) +
		" [opencode]=" + shellQuote(filepath.Join(home, ".config", "opencode", "skills")) +
		" [codex]=" + shellQuote(filepath.Join(home, ".codex", "skills")) + " )\n" +
		"declare -A AGENT_TARGET_PATHS=( [claude]=" + shellQuote(filepath.Join(home, ".claude", "agents")) +
		" [opencode]=" + shellQuote(filepath.Join(home, ".config", "opencode", "agents")) + " )\n" +
		`eval "$(awk '/^route_reject_unrouted_longterm_mem\(\)/,/^}$/ { print } /^route_resolve\(\)/,/^}$/ { print }' ` + shellQuote(overlayPath) + `)"` + "\n" +
		"route_resolve " + shellQuote(rowPath) + " >/dev/null\n"
	if err := os.WriteFile(scriptFile, []byte(script), 0o755); err != nil {
		t.Fatalf("write runner script: %v", err)
	}

	cmd := exec.Command("bash", scriptFile)
	err := cmd.Run()
	return err == nil
}

// shellQuote wraps s in single quotes for embedding into a generated bash
// script, escaping any embedded single quote.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// TestRouteDomain_MatchesBashAndGo pins that the bash route_resolve dispatch
// (bin/labdrian-overlay) and the Go nonSkillRoutes exclusion set recognize
// the identical four-value route domain {skill, agent, opencode-agent, mcp}
// (overlay-agent-route R-006). The Go side of the domain is derived from the
// real nonSkillRoutes map (plus the implicit "skill" default) rather than
// restated as a second hardcoded list, so the two parsers cannot silently
// drift without failing this test.
func TestRouteDomain_MatchesBashAndGo(t *testing.T) {
	goDomain := map[string]bool{"skill": true}
	for route := range nonSkillRoutes {
		goDomain[route] = true
	}
	if len(goDomain) != 4 {
		t.Fatalf("Go route domain (nonSkillRoutes + implicit skill default) has %d values, want 4: %v", len(goDomain), goDomain)
	}

	overlay := filepath.Join(skillsRepoRoot(t), "bin", "labdrian-overlay")
	if _, err := os.Stat(overlay); err != nil {
		t.Fatalf("overlay script not found at %s: %v", overlay, err)
	}

	candidates := []string{"skill", "agent", "opencode-agent", "mcp", "bogus-route"}
	for _, route := range candidates {
		bashAccepts := bashAcceptsLongtermMemRoute(t, overlay, route)
		goAccepts := goDomain[route]
		if bashAccepts != goAccepts {
			t.Errorf("route %q: bash accepted=%v, Go nonSkillRoutes-derived domain accepted=%v — parsers have drifted", route, bashAccepts, goAccepts)
		}
	}
}

// TestInfraExclusionRulesArePinnedIndependently pins R-004: the on-disk
// check's deployable-path rule (DeployableManifestPaths) and manifest
// loading's infra-exclusion rule (loadManifestViewReader / infraPrefixes)
// look similar but MUST stay independent. `_shared/*` rows are deployable
// for the on-disk check, even though manifest loading excludes `_shared` as
// infra when collapsing rows to skill directories. Unifying either rule set
// to match the other must fail one of the two subtests below.
func TestInfraExclusionRulesArePinnedIndependently(t *testing.T) {
	const fixture = `_shared/x.md managed
_shared/pin/SKILL.md managed
engine/y.go managed
real-skill/SKILL.md managed
`

	t.Run("ondisk direction: _shared rows are deployable, engine rows are not", func(t *testing.T) {
		got, err := DeployableManifestPaths(strings.NewReader(fixture))
		if err != nil {
			t.Fatalf("DeployableManifestPaths: unexpected error: %v", err)
		}
		for _, want := range []string{"_shared/x.md", "_shared/pin/SKILL.md"} {
			if _, ok := got[want]; !ok {
				t.Errorf("DeployableManifestPaths: missing deployable row %q — excluding _shared here would break the on-disk check", want)
			}
		}
		if _, ok := got["engine/y.go"]; ok {
			t.Error("DeployableManifestPaths: engine/y.go must stay excluded as infra")
		}
	})

	t.Run("manifest direction: _shared is excluded as infra, real skills are not", func(t *testing.T) {
		mv, err := loadManifestViewReader(strings.NewReader(fixture))
		if err != nil {
			t.Fatalf("loadManifestViewReader: unexpected error: %v", err)
		}
		if _, ok := mv["real-skill"]; !ok {
			t.Error("LoadManifestView: missing real-skill directory")
		}
		if _, ok := mv["_shared"]; ok {
			t.Error("LoadManifestView: _shared must stay excluded as infra — the explicit _shared/pin/SKILL.md row is what makes this direction meaningful")
		}
	})
}

// TestOnDiskDiagnosticsNameManifestRowEdit pins R-009: both on-disk
// divergence classes must name the manifest-row edit that resolves them and
// must never cite `sync-manifest`, which cannot clear an UNREGISTERED_ON_DISK
// or MISSING_ON_DISK divergence for a reference or `_shared/*` file (isSkillRow
// only regenerates `*/SKILL.md` rows).
func TestOnDiskDiagnosticsNameManifestRowEdit(t *testing.T) {
	tests := []struct {
		name          string
		disk          []string
		manifest      map[string]struct{}
		wantClass     DivergenceClass
		wantPath      string
		wantRemediate string
	}{
		{
			name:          "unregistered file names the row to add",
			disk:          []string{"orphan/SKILL.md"},
			manifest:      map[string]struct{}{},
			wantClass:     DivUnregisteredOnDisk,
			wantPath:      "orphan/SKILL.md",
			wantRemediate: "add a row for",
		},
		{
			name:          "orphan row names the row to remove",
			disk:          []string{},
			manifest:      map[string]struct{}{"ghost/SKILL.md": {}},
			wantClass:     DivMissingOnDisk,
			wantPath:      "ghost/SKILL.md",
			wantRemediate: "remove the",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			divs := DiffOnDisk(tt.disk, tt.manifest)
			if len(divs) != 1 {
				t.Fatalf("DiffOnDisk: got %d divergences %v, want 1", len(divs), divs)
			}
			d := divs[0]
			if d.Class != tt.wantClass {
				t.Errorf("DiffOnDisk: class = %q, want %q", d.Class, tt.wantClass)
			}
			if d.Path != tt.wantPath {
				t.Errorf("DiffOnDisk: path = %q, want %q", d.Path, tt.wantPath)
			}
			if !strings.Contains(d.Detail, tt.wantRemediate) {
				t.Errorf("DiffOnDisk: Detail %q must name the remediation %q", d.Detail, tt.wantRemediate)
			}
			if !strings.Contains(d.Detail, "overlay.manifest") {
				t.Errorf("DiffOnDisk: Detail %q must name overlay.manifest", d.Detail)
			}
			if strings.Contains(d.Detail, "sync-manifest") {
				t.Errorf("DiffOnDisk: Detail %q must not cite sync-manifest, which cannot clear this divergence", d.Detail)
			}
		})
	}
}
