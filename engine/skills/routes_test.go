package skills

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// Tests of the routing of the rows of overlay.manifest (routes.go). What they say is what the
// overlay script did with a row before the engine took the routing over; the pins of the script
// (engine/installer/routing_pins_test.go) hold the same behavior from the outside.

// destinations is the destinations of a target, in a home that is no one's.
func destinations(target string) RouteDestinations {
	d := RouteDestinations{Target: target}
	switch target {
	case "claude":
		d.SkillsRoot, d.AgentsRoot = "/h/.claude/skills", "/h/.claude/agents"
	case "opencode":
		d.SkillsRoot, d.AgentsRoot = "/h/.config/opencode/skills", "/h/.config/opencode/agents"
	case "codex":
		d.SkillsRoot = "/h/.codex/skills"
	}
	return d
}

const testOverlay = "/o"

// --- reading the rows -------------------------------------------------------------------------

func TestReadManifestRowsReadsALineTheWayGrepAndAwkDid(t *testing.T) {
	for _, tc := range []struct {
		name     string
		manifest string
		want     []ManifestRow
	}{
		{"nothing", "", nil},
		{"blank lines and white space only", "\n  \n\t\n\v\f\r\n", nil},
		{"a comment", "# a comment\n", nil},
		{"a comment after white space", "  \t# a comment\nx/y managed\n", []ManifestRow{{"x/y", "managed", ""}}},
		{"a comment after a vertical tab", "\v# a comment\n", nil},
		{"a row of two columns", "a/SKILL.md managed\n", []ManifestRow{{"a/SKILL.md", "managed", ""}}},
		{"a row of three columns", "GADU.md custom agent\n", []ManifestRow{{"GADU.md", "custom", "agent"}}},
		{"a row with a path and no tag", "bare/SKILL.md\n", []ManifestRow{{Path: "bare/SKILL.md"}}},
		{"tabs and runs of spaces between columns", "a\tmanaged\t\tagent\nb   custom\n", []ManifestRow{{"a", "managed", "agent"}, {"b", "custom", ""}}},
		{"white space before and after a row", "   a managed  \t\n", []ManifestRow{{"a", "managed", ""}}},
		{"a carriage return stays on the last column", "a managed\r\nb custom agent\r\n", []ManifestRow{{"a", "managed\r", ""}, {"b", "custom", "agent\r"}}},
		{"a carriage return alone is a blank line", "a managed\n\r\nb custom\n", []ManifestRow{{"a", "managed", ""}, {"b", "custom", ""}}},
		{"a comment after a column is a column", "a managed # not a comment\n", []ManifestRow{{"a", "managed", "#"}}},
		{"a last line with no line end", "a managed\nb managed", []ManifestRow{{"a", "managed", ""}, {"b", "managed", ""}}},
		{"a row is not split at other white space", "a\vb managed\n", []ManifestRow{{"a\vb", "managed", ""}}},
		{"more than three columns", "a managed agent extra\n", []ManifestRow{{"a", "managed", "agent"}}},
		{"a path named twice is two rows", "a managed\na custom agent\n", []ManifestRow{{"a", "managed", ""}, {"a", "custom", "agent"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ReadManifestRows([]byte(tc.manifest)); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ReadManifestRows(%q) = %v, want %v", tc.manifest, got, tc.want)
			}
		})
	}
}

// --- the route of a row ------------------------------------------------------------------------

func TestResolveRoutesPlacesEachKindOfRow(t *testing.T) {
	for _, tc := range []struct {
		name   string
		row    ManifestRow
		target string
		want   ResolvedRoute
	}{
		{"a bare skill row goes to the skills of Claude", ManifestRow{"sdd-spec/SKILL.md", "managed", ""}, "claude",
			ResolvedRoute{"sdd-spec/SKILL.md", "skill", "skills/sdd-spec/SKILL.md", "/o/skills/sdd-spec/SKILL.md", "/h/.claude/skills/sdd-spec/SKILL.md"}},
		{"a bare skill row goes to the skills of OpenCode", ManifestRow{"sdd-spec/SKILL.md", "managed", ""}, "opencode",
			ResolvedRoute{"sdd-spec/SKILL.md", "skill", "skills/sdd-spec/SKILL.md", "/o/skills/sdd-spec/SKILL.md", "/h/.config/opencode/skills/sdd-spec/SKILL.md"}},
		{"a bare skill row goes to the skills of Codex", ManifestRow{"sdd-spec/SKILL.md", "managed", ""}, "codex",
			ResolvedRoute{"sdd-spec/SKILL.md", "skill", "skills/sdd-spec/SKILL.md", "/o/skills/sdd-spec/SKILL.md", "/h/.codex/skills/sdd-spec/SKILL.md"}},
		{"a row that says skill is the same", ManifestRow{"shared/helper.md", "managed", "skill"}, "claude",
			ResolvedRoute{"shared/helper.md", "skill", "skills/shared/helper.md", "/o/skills/shared/helper.md", "/h/.claude/skills/shared/helper.md"}},
		{"an agent goes to the agents of Claude", ManifestRow{"GADU.md", "custom", "agent"}, "claude",
			ResolvedRoute{"GADU.md", "agent", "agents/GADU.md", "/o/agents/GADU.md", "/h/.claude/agents/GADU.md"}},
		{"an agent goes nowhere for OpenCode", ManifestRow{"GADU.md", "custom", "agent"}, "opencode",
			ResolvedRoute{"GADU.md", "agent", "agents/GADU.md", "/o/agents/GADU.md", ""}},
		{"an agent goes nowhere for Codex", ManifestRow{"GADU.md", "custom", "agent"}, "codex",
			ResolvedRoute{"GADU.md", "agent", "agents/GADU.md", "/o/agents/GADU.md", ""}},
		{"a native agent of OpenCode goes to its agents by its name", ManifestRow{"opencode/agents/GADU.md", "custom", "opencode-agent"}, "opencode",
			ResolvedRoute{"opencode/agents/GADU.md", "opencode-agent", "opencode/agents/GADU.md", "/o/opencode/agents/GADU.md", "/h/.config/opencode/agents/GADU.md"}},
		{"a native agent of OpenCode goes nowhere for Claude", ManifestRow{"opencode/agents/GADU.md", "custom", "opencode-agent"}, "claude",
			ResolvedRoute{"opencode/agents/GADU.md", "opencode-agent", "opencode/agents/GADU.md", "/o/opencode/agents/GADU.md", ""}},
		{"a native agent of OpenCode keeps what is below its directory", ManifestRow{"opencode/agents/team/GADU.md", "custom", "opencode-agent"}, "opencode",
			ResolvedRoute{"opencode/agents/team/GADU.md", "opencode-agent", "opencode/agents/team/GADU.md", "/o/opencode/agents/team/GADU.md", "/h/.config/opencode/agents/team/GADU.md"}},
		{"an mcp row has a source and no destination", ManifestRow{"longterm-mem/go.mod", "custom", "mcp"}, "claude",
			ResolvedRoute{"longterm-mem/go.mod", "mcp", "longterm-mem/go.mod", "/o/longterm-mem/go.mod", ""}},
		{"a third column nobody knows is the skill route under its own name", ManifestRow{"weird/SKILL.md", "managed", "unknown-route"}, "claude",
			ResolvedRoute{"weird/SKILL.md", "unknown-route", "weird/SKILL.md", "/o/skills/weird/SKILL.md", "/h/.claude/skills/weird/SKILL.md"}},
		{"a third column nobody knows keeps a carriage return", ManifestRow{"x/y.md", "managed", "agent\r"}, "claude",
			ResolvedRoute{"x/y.md", "agent\r", "x/y.md", "/o/skills/x/y.md", "/h/.claude/skills/x/y.md"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveRoutes([]ManifestRow{tc.row}, false, testOverlay, destinations(tc.target))
			if err != nil {
				t.Fatalf("ResolveRoutes: %v", err)
			}
			if !reflect.DeepEqual(got, []ResolvedRoute{tc.want}) {
				t.Errorf("ResolveRoutes = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestResolveRoutesNeverDeploysWhatNoDirectoryOfTheOverlayHolds(t *testing.T) {
	for _, tc := range []struct {
		name string
		row  ManifestRow
	}{
		{"a file of the engine", ManifestRow{"engine/go.mod", "managed", ""}},
		{"a file of the engine with a route of its own", ManifestRow{"engine/go.mod", "managed", "skill"}},
		{"a file of the engine that says mcp", ManifestRow{"engine/go.mod", "managed", "mcp"}},
		{"a file of the engine that says opencode-agent", ManifestRow{"engine/x.md", "managed", "opencode-agent"}},
		{"a file at the root", ManifestRow{"skills.registry.yaml", "custom", ""}},
		{"a file at the root that says mcp", ManifestRow{"README.md", "custom", "mcp"}},
		{"a native agent of OpenCode outside its directory", ManifestRow{"other/stray.md", "custom", "opencode-agent"}},
		{"a native agent of OpenCode named by its directory alone", ManifestRow{"opencode/agents/", "custom", "opencode-agent"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, target := range []string{"claude", "opencode", "codex"} {
				got, err := ResolveRoutes([]ManifestRow{tc.row}, false, testOverlay, destinations(target))
				want := ResolvedRoute{Path: tc.row.Path, Route: "non-deployable", RepoRel: tc.row.Path}
				if err != nil || !reflect.DeepEqual(got, []ResolvedRoute{want}) {
					t.Errorf("%s: ResolveRoutes = %+v, %v, want %+v", target, got, err, want)
				}
			}
		})
	}
}

func TestResolveRoutesDeploysANativeAgentOfClaudeWhateverItsPath(t *testing.T) {
	// The check that keeps a bare file and the engine out of skills/ is for the other routes: a
	// native agent of Claude is named by a bare file, and the script placed one under engine/ too.
	for _, path := range []string{"GADU.md", "engine/agent.md"} {
		got, err := ResolveRoutes([]ManifestRow{{path, "custom", "agent"}}, false, testOverlay, destinations("claude"))
		want := ResolvedRoute{path, "agent", "agents/" + path, "/o/agents/" + path, "/h/.claude/agents/" + path}
		if err != nil || !reflect.DeepEqual(got, []ResolvedRoute{want}) {
			t.Errorf("%s: ResolveRoutes = %+v, %v, want %+v", path, got, err, want)
		}
	}
}

func TestResolveRoutesJoinsPathsByConcatenation(t *testing.T) {
	got, err := ResolveRoutes([]ManifestRow{{"a//b/../c.md", "managed", ""}}, false, "/o/", RouteDestinations{Target: "claude", SkillsRoot: "/s/"})
	if err != nil {
		t.Fatal(err)
	}
	want := ResolvedRoute{"a//b/../c.md", "skill", "skills/a//b/../c.md", "/o//skills/a//b/../c.md", "/s//a//b/../c.md"}
	if !reflect.DeepEqual(got, []ResolvedRoute{want}) {
		t.Errorf("ResolveRoutes = %+v, want %+v: a path is joined as written, not cleaned", got, want)
	}
}

// --- which rows, and the route of a path named twice ---------------------------------------------

func TestResolveRoutesKeepsTheRowsInTheOrderTheyAreWritten(t *testing.T) {
	rows := []ManifestRow{{"b/SKILL.md", "managed", ""}, {"a/SKILL.md", "custom", ""}, {"c/SKILL.md", "managed", ""}}
	all, err := ResolveRoutes(rows, false, testOverlay, destinations("claude"))
	if err != nil {
		t.Fatal(err)
	}
	managed, err := ResolveRoutes(rows, true, testOverlay, destinations("claude"))
	if err != nil {
		t.Fatal(err)
	}
	if got := pathsOf(all); !reflect.DeepEqual(got, []string{"b/SKILL.md", "a/SKILL.md", "c/SKILL.md"}) {
		t.Errorf("all rows = %v", got)
	}
	if got := pathsOf(managed); !reflect.DeepEqual(got, []string{"b/SKILL.md", "c/SKILL.md"}) {
		t.Errorf("managed rows = %v, want the rows whose tag is managed, in order", got)
	}
}

func TestResolveRoutesTakesTheRouteOfTheFirstRowThatNamesAPath(t *testing.T) {
	rows := []ManifestRow{{"dup/SKILL.md", "managed", ""}, {"dup/SKILL.md", "custom", "agent"}, {"GADU.md", "custom", "agent"}, {"GADU.md", "custom", "skill"}}
	got, err := ResolveRoutes(rows, false, testOverlay, destinations("claude"))
	if err != nil {
		t.Fatal(err)
	}
	var routes []string
	for _, r := range got {
		routes = append(routes, r.Route)
	}
	// Every row is placed, each by the route its path was first given.
	if want := []string{"skill", "skill", "agent", "agent"}; !reflect.DeepEqual(routes, want) {
		t.Errorf("routes = %v, want %v", routes, want)
	}
}

func TestResolveRoutesTakesTheRouteOfAPathFromARowThatIsNotKept(t *testing.T) {
	rows := []ManifestRow{{"GADU.md", "custom", "agent"}, {"GADU.md", "managed", ""}}
	got, err := ResolveRoutes(rows, true, testOverlay, destinations("claude"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Route != "agent" || got[0].Dest != "/h/.claude/agents/GADU.md" {
		t.Errorf("ResolveRoutes = %+v, want the managed row placed by the agent route of the custom row before it", got)
	}
}

func pathsOf(routes []ResolvedRoute) []string {
	var paths []string
	for _, r := range routes {
		paths = append(paths, r.Path)
	}
	return paths
}

// --- what is refused ---------------------------------------------------------------------------

func TestResolveRoutesRefusesALongtermMemRowWithoutAKnownRoute(t *testing.T) {
	for _, tc := range []struct {
		name string
		row  ManifestRow
		want string
	}{
		{"no route", ManifestRow{"longterm-mem/internal/foo.go", "custom", ""},
			"route_resolve: longterm-mem row 'longterm-mem/internal/foo.go' declares no route (missing third column); must be one of: skill, agent, opencode-agent, mcp"},
		{"no tag and no route", ManifestRow{Path: "longterm-mem/only-one-field"},
			"route_resolve: longterm-mem row 'longterm-mem/only-one-field' declares no route (missing third column); must be one of: skill, agent, opencode-agent, mcp"},
		{"a route nobody knows", ManifestRow{"longterm-mem/internal/bar.go", "custom", "bogus"},
			"route_resolve: longterm-mem row 'longterm-mem/internal/bar.go' declares an unrecognized route 'bogus'; must be one of: skill, agent, opencode-agent, mcp"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ResolveRoutes([]ManifestRow{tc.row}, false, testOverlay, destinations("claude"))
			var unrouted *UnroutedRowError
			if !errors.As(err, &unrouted) || err.Error() != tc.want {
				t.Errorf("ResolveRoutes error = %v, want an UnroutedRowError saying %q", err, tc.want)
			}
		})
	}
}

func TestResolveRoutesAcceptsEachRouteOfTheFourUnderLongtermMem(t *testing.T) {
	for _, route := range []string{"skill", "agent", "opencode-agent", "mcp"} {
		_, err := ResolveRoutes([]ManifestRow{{"longterm-mem/x/y.go", "custom", route}}, false, testOverlay, destinations("claude"))
		if err != nil {
			t.Errorf("route %q under longterm-mem/: %v, want it accepted", route, err)
		}
	}
}

func TestResolveRoutesRefusesOnlyTheRowsItWasAskedFor(t *testing.T) {
	rows := []ManifestRow{{"a/SKILL.md", "managed", ""}, {"longterm-mem/internal/foo.go", "custom", ""}}
	if got, err := ResolveRoutes(rows, true, testOverlay, destinations("claude")); err != nil || len(got) != 1 {
		t.Errorf("managed rows: %+v, %v, want the managed row and no refusal for the custom one", got, err)
	}
	if got, err := ResolveRoutes(rows, false, testOverlay, destinations("claude")); err == nil || got != nil {
		t.Errorf("all rows: %+v, %v, want the refusal and no row: a manifest with a row that cannot be placed is not half acted on", got, err)
	}
}

func TestResolveRoutesRefusesAnUnknownTarget(t *testing.T) {
	_, err := ResolveRoutes(nil, false, testOverlay, RouteDestinations{Target: "windsurf", SkillsRoot: "/s"})
	var unknown *UnknownTargetError
	if !errors.As(err, &unknown) || !strings.Contains(err.Error(), `unknown target "windsurf"; must be one of: claude`) {
		t.Errorf("ResolveRoutes error = %v, want an UnknownTargetError naming the target and the ones there are", err)
	}
}

func TestResolveRoutesRefusesARowThatGoesToADirectoryNobodyNamed(t *testing.T) {
	for _, tc := range []struct {
		name string
		row  ManifestRow
		dest RouteDestinations
		want string
	}{
		{"a skill with no skills directory", ManifestRow{"a/SKILL.md", "managed", ""}, RouteDestinations{Target: "claude"},
			"manifest row 'a/SKILL.md' is copied to the skills directory of claude, and none was given"},
		{"an agent with no agents directory", ManifestRow{"GADU.md", "custom", "agent"}, RouteDestinations{Target: "claude", SkillsRoot: "/s"},
			"manifest row 'GADU.md' is copied to the agents directory of claude, and none was given"},
		{"a native agent of OpenCode with no agents directory", ManifestRow{"opencode/agents/G.md", "custom", "opencode-agent"}, RouteDestinations{Target: "opencode", SkillsRoot: "/s"},
			"manifest row 'opencode/agents/G.md' is copied to the agents directory of opencode, and none was given"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ResolveRoutes([]ManifestRow{tc.row}, false, testOverlay, tc.dest)
			var missing *MissingRootError
			if !errors.As(err, &missing) || err.Error() != tc.want {
				t.Errorf("ResolveRoutes error = %v, want a MissingRootError saying %q", err, tc.want)
			}
		})
	}
}

func TestResolveRoutesDoesNotAskForAnAgentsDirectoryItWillNotUse(t *testing.T) {
	// Codex has no native agents, and an agent row goes nowhere for it: no directory is needed.
	got, err := ResolveRoutes([]ManifestRow{{"GADU.md", "custom", "agent"}}, false, testOverlay, RouteDestinations{Target: "codex", SkillsRoot: "/s"})
	if err != nil || len(got) != 1 || got[0].Dest != "" {
		t.Errorf("ResolveRoutes = %+v, %v, want the agent row placed for Codex with no destination", got, err)
	}
}
