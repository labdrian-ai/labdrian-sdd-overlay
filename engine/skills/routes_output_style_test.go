package skills

import (
	"errors"
	"reflect"
	"testing"
)

// The output-style route (T27): a row named by a bare file is copied from output-styles/ to the
// output styles directory of Claude Code, which sits beside its agents directory, and goes nowhere
// for any other target. It follows the agent route: the bare file name is the row, the directory of
// the overlay it comes from is the route's.

func TestResolveRoutesPlacesAnOutputStyleForClaudeOnly(t *testing.T) {
	row := ManifestRow{"labdrian.md", "custom", "output-style"}
	for _, tc := range []struct {
		target string
		want   ResolvedRoute
	}{
		{"claude", ResolvedRoute{"labdrian.md", "output-style", "output-styles/labdrian.md", "/o/output-styles/labdrian.md", "/h/.claude/output-styles/labdrian.md"}},
		{"opencode", ResolvedRoute{"labdrian.md", "output-style", "output-styles/labdrian.md", "/o/output-styles/labdrian.md", ""}},
		{"codex", ResolvedRoute{"labdrian.md", "output-style", "output-styles/labdrian.md", "/o/output-styles/labdrian.md", ""}},
	} {
		t.Run(tc.target, func(t *testing.T) {
			got, err := ResolveRoutes([]ManifestRow{row}, false, testOverlay, destinations(tc.target))
			if err != nil {
				t.Fatalf("ResolveRoutes: %v", err)
			}
			if !reflect.DeepEqual(got, []ResolvedRoute{tc.want}) {
				t.Errorf("ResolveRoutes = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// A target that does not take the row needs no directory for it, as an agent row of another runtime
// needs none; Claude Code does need the directory its output styles sit beside.
func TestResolveRoutesAsksForTheClaudeDirectoryOfAnOutputStyleOnlyWhenItWillUseIt(t *testing.T) {
	row := []ManifestRow{{"labdrian.md", "custom", "output-style"}}
	for _, target := range []string{"opencode", "codex"} {
		got, err := ResolveRoutes(row, false, testOverlay, RouteDestinations{Target: target, SkillsRoot: "/s"})
		if err != nil || len(got) != 1 || got[0].Dest != "" {
			t.Errorf("%s: ResolveRoutes = %+v, %v, want the row placed with no destination", target, got, err)
		}
	}
	_, err := ResolveRoutes(row, false, testOverlay, RouteDestinations{Target: "claude", SkillsRoot: "/s"})
	var missing *MissingRootError
	if !errors.As(err, &missing) || missing.Path != "labdrian.md" || missing.Directory != "output-styles" || missing.Target != "claude" {
		t.Errorf("ResolveRoutes error = %#v, want a MissingRootError for labdrian.md, output-styles, claude", err)
	}
}

// The route is not a route of longterm-mem/: the domain that guard refuses outside of is unchanged,
// and so are its words.
func TestResolveRoutesStillRefusesAnOutputStyleRouteUnderLongtermMem(t *testing.T) {
	_, err := ResolveRoutes([]ManifestRow{{"longterm-mem/x.md", "custom", "output-style"}}, false, testOverlay, destinations("claude"))
	var unrouted *UnroutedRowError
	if !errors.As(err, &unrouted) {
		t.Fatalf("ResolveRoutes error = %v, want an UnroutedRowError", err)
	}
	const want = "route_resolve: longterm-mem row 'longterm-mem/x.md' declares an unrecognized route 'output-style'; must be one of: skill, agent, opencode-agent, codex-agent, mcp"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

// A bare file that names no route is never deployed, whatever it is called: the new route does not
// make a bare name a style.
func TestResolveRoutesStillNeverDeploysABareFileWithoutARoute(t *testing.T) {
	got, err := ResolveRoutes([]ManifestRow{{"labdrian.md", "custom", ""}}, false, testOverlay, destinations("claude"))
	want := []ResolvedRoute{{"labdrian.md", "non-deployable", "labdrian.md", "", ""}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("ResolveRoutes = %+v, %v, want %+v", got, err, want)
	}
}

// A trailing slash on the agents root names the same directory: the style goes beside it all the same.
func TestResolveRoutesCleansTheAgentsRootBeforePlacingAnOutputStyle(t *testing.T) {
	for _, root := range []string{"/h/.claude/agents/", "/h/.claude//agents//", "/h/./.claude/agents"} {
		dest := RouteDestinations{Target: "claude", SkillsRoot: "/s", AgentsRoot: root}
		got, err := ResolveRoutes([]ManifestRow{{"labdrian.md", "custom", "output-style"}}, false, testOverlay, dest)
		if err != nil || len(got) != 1 || got[0].Dest != "/h/.claude/output-styles/labdrian.md" {
			t.Errorf("agents root %q: ResolveRoutes = %+v, %v, want /h/.claude/output-styles/labdrian.md", root, got, err)
		}
	}
}

// An agents root with no parent directory (a bare name, the root of the file system) has nothing to
// place output-styles beside: the row is refused, saying so, instead of landing in the working directory.
func TestResolveRoutesRefusesAnOutputStyleWhenTheAgentsRootHasNoParent(t *testing.T) {
	for _, root := range []string{"agents", "agents/", ".", "/"} {
		dest := RouteDestinations{Target: "claude", SkillsRoot: "/s", AgentsRoot: root}
		got, err := ResolveRoutes([]ManifestRow{{"labdrian.md", "custom", "output-style"}}, false, testOverlay, dest)
		var refused *OutputStylesRootError
		if !errors.As(err, &refused) || got != nil {
			t.Fatalf("agents root %q: ResolveRoutes = %+v, %v, want an OutputStylesRootError and no row", root, got, err)
		}
		want := "manifest row 'labdrian.md' is an output style of claude, and the agents directory " + `"` + root + `"` + " has no parent directory to place output-styles beside"
		if err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
	}
}
