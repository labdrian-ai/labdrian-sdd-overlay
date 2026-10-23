package skills

import (
	"os"
	"path/filepath"
	"reflect"
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
sdd-apply.md             managed  agent
opencode/agents/GADU.md custom   opencode-agent
codex/agents/builder.toml custom codex-agent
`

func TestDeployableManifestPaths(t *testing.T) {
	got, err := DeployableManifestPaths(strings.NewReader(routingManifest))
	if err != nil {
		t.Fatalf("DeployableManifestPaths: unexpected error: %v", err)
	}

	// Only rows that the routing (ResolveRoutes) sends to skills/ belong here.
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

	// engine/* is infra, a path with no separator is root-level, and the three
	// agent routes land outside skills/. None may be treated as deployable.
	excluded := []string{
		"engine/go.mod",
		"engine/skills/sync.go",
		"skills.registry.yaml",
		"GADU.md",
		"sdd-apply.md",
		"opencode/agents/GADU.md",
		"codex/agents/builder.toml",
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
// R-035). The routing (ResolveRoutes) refuses the same row; this pins the
// on-disk check's half of that guard.
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
// outside {skill, agent, opencode-agent, codex-agent, mcp}.
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

// TestDeployableManifestPaths_RejectsOneColumnLongtermMemRow pins WARNING-2
// from the verify report: a longterm-mem/** row with only a path and no
// other field (no tag, no route) must be rejected the same way the routing
// rejects it (ResolveRoutes), not silently skipped by the generic "fewer than
// two fields" rule that applies to every other path prefix. The routing still
// reads such a row, with an empty (undefaulted) third column, and refuses it
// naming the row — the on-disk check must reach the same outcome.
func TestDeployableManifestPaths_RejectsOneColumnLongtermMemRow(t *testing.T) {
	const manifest = `longterm-mem/only-one-field
sdd-spec/SKILL.md managed
`
	got, err := DeployableManifestPaths(strings.NewReader(manifest))
	if err == nil {
		t.Fatalf("DeployableManifestPaths: expected an error for a one-column longterm-mem/** row, got nil (paths=%v)", got)
	}
	if !strings.Contains(err.Error(), "longterm-mem/only-one-field") {
		t.Errorf("DeployableManifestPaths: error must name the rejected row, got: %v", err)
	}
	if _, ok := got["longterm-mem/only-one-field"]; ok {
		t.Errorf("DeployableManifestPaths: a one-column longterm-mem row must never resolve to a skills-destination path, got: %v", got)
	}
}

// TestDeployableManifestPaths_OneColumnRowStillSkippedForOtherPrefixes proves
// the fix above is scoped to longterm-mem/** only: a one-column row under any
// other path prefix is still silently skipped by the pre-existing
// whole-manifest "fewer than two fields" rule, exactly as before.
func TestDeployableManifestPaths_OneColumnRowStillSkippedForOtherPrefixes(t *testing.T) {
	const manifest = `sdd-spec/only-one-field
sdd-spec/SKILL.md managed
`
	got, err := DeployableManifestPaths(strings.NewReader(manifest))
	if err != nil {
		t.Fatalf("DeployableManifestPaths: unexpected error: %v", err)
	}
	if _, ok := got["sdd-spec/only-one-field"]; ok {
		t.Errorf("DeployableManifestPaths: a one-column row outside longterm-mem/** must still be silently skipped, got: %v", got)
	}
	if _, ok := got["sdd-spec/SKILL.md"]; !ok {
		t.Errorf("DeployableManifestPaths: the following well-formed row must still parse, got: %v", got)
	}
}

// TestDeployableManifestPaths_LongtermMemRouteGuardAcceptsEveryValidRoute
// proves the guard is scoped exactly to the five-value route domain: every
// recognized route on a longterm-mem/** row parses without error (a
// skill-routed longterm-mem row is unusual but not itself invalid — R-012
// only forbids MISSING or UNRECOGNIZED routes).
func TestDeployableManifestPaths_LongtermMemRouteGuardAcceptsEveryValidRoute(t *testing.T) {
	for _, route := range []string{"skill", "agent", "opencode-agent", "codex-agent", "mcp"} {
		manifest := "longterm-mem/valid.go custom " + route + "\n"
		if _, err := DeployableManifestPaths(strings.NewReader(manifest)); err != nil {
			t.Errorf("DeployableManifestPaths: route %q on a longterm-mem/** row must be accepted, got error: %v", route, err)
		}
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

	disk, err := scanSkillFiles(filepath.Join(root, "skills"))
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

// routingAcceptsLongtermMemRoute reports whether the routing (ResolveRoutes) accepts a
// longterm-mem/** row whose third column is route. The on-disk check is asked the same question by
// TestRouteDomain_OnDiskCheckAndRoutingAgree, so that the two readers of the manifest in this
// package cannot recognize different route domains without failing a test.
func routingAcceptsLongtermMemRoute(route string) bool {
	rows := []ManifestRow{{Path: "longterm-mem/parity-fixture.txt", Tag: "custom", Route: route}}
	_, err := ResolveRoutes(rows, false, "/o", RouteDestinations{Target: "claude", SkillsRoot: "/s", AgentsRoot: "/a"})
	return err == nil
}

// TestRouteDomain_OnDiskCheckAndRoutingAgree pins that the on-disk check (DeployableManifestPaths,
// which uses the route domain to refuse a longterm-mem/** row) and the routing (ResolveRoutes,
// which the overlay script asks) recognize the identical five-value route domain {skill, agent,
// opencode-agent, codex-agent, mcp} (overlay-agent-route R-006; codex-agent joined the four of D13
// in slice 4 of the role agents). The domain of the on-disk check is derived from
// the real nonSkillRoutes map (plus the implicit "skill" default) rather than restated as a second
// list, so the two cannot silently drift without failing this test. Before the routing moved into
// the engine the other side of this comparison was the bash route_resolve.
func TestRouteDomain_OnDiskCheckAndRoutingAgree(t *testing.T) {
	onDiskDomain := map[string]bool{"skill": true}
	for route := range nonSkillRoutes {
		onDiskDomain[route] = true
	}
	if len(onDiskDomain) != 5 {
		t.Fatalf("on-disk route domain (nonSkillRoutes + implicit skill default) has %d values, want 5: %v", len(onDiskDomain), onDiskDomain)
	}

	for _, route := range []string{"skill", "agent", "opencode-agent", "codex-agent", "mcp", "bogus-route"} {
		_, onDiskErr := DeployableManifestPaths(strings.NewReader("longterm-mem/parity-fixture.txt   custom   " + route + "\n"))
		if onDiskAccepts, routingAccepts := onDiskErr == nil, routingAcceptsLongtermMemRoute(route); onDiskAccepts != routingAccepts || onDiskAccepts != onDiskDomain[route] {
			t.Errorf("route %q: the on-disk check accepted=%v, the routing accepted=%v, the route domain has it=%v — the readers have drifted", route, onDiskAccepts, routingAccepts, onDiskDomain[route])
		}
	}
}

// TestDeployableManifestPaths_AgreesWithTheRoutingOnWhatGoesToSkills pins that a row the on-disk
// check expects under skills/ is a row the routing copies into the skills directories, and the
// other way round: the rows with a tag of routingManifest, which holds a row of each kind.
func TestDeployableManifestPaths_AgreesWithTheRoutingOnWhatGoesToSkills(t *testing.T) {
	onDisk, err := DeployableManifestPaths(strings.NewReader(routingManifest + "weird/SKILL.md managed unknown-route\n"))
	if err != nil {
		t.Fatalf("DeployableManifestPaths: %v", err)
	}
	var tagged []ManifestRow
	for _, row := range ReadManifestRows([]byte(routingManifest + "weird/SKILL.md managed unknown-route\n")) {
		if row.Tag != "" {
			tagged = append(tagged, row)
		}
	}
	routed, err := ResolveRoutes(tagged, false, "/o", RouteDestinations{Target: "claude", SkillsRoot: "/s", AgentsRoot: "/a"})
	if err != nil {
		t.Fatalf("ResolveRoutes: %v", err)
	}
	toSkills := map[string]struct{}{}
	for _, r := range routed {
		if strings.HasPrefix(r.Dest, "/s/") {
			toSkills[r.Path] = struct{}{}
		}
	}
	if len(toSkills) == 0 || !reflect.DeepEqual(onDisk, toSkills) {
		t.Errorf("the on-disk check expects %v under skills/, the routing copies %v into the skills directories", onDisk, toSkills)
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
