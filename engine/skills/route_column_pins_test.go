package skills

import (
	"reflect"
	"strings"
	"testing"
)

// The third column of overlay.manifest (S4, T21). The routing keeps the leniency of the script: a
// value it does not know is not refused, the row takes the skill route and is copied to every copy
// target. These pins stop a misspelt route where it is written, in CI, and keep the texts that say
// where a row goes in step with the routes.

// knownManifestRoutes is every value a third column may hold. RouteNonDeployable is not in it: it is an
// output of the routing, never a value to write.
var knownManifestRoutes = map[string]bool{
	"":                 true,
	RouteSkill:         true,
	RouteClaudeSkill:   true,
	RouteAgent:         true,
	RouteOpencodeAgent: true,
	RouteCodexAgent:    true,
	RouteOutputStyle:   true,
	RouteMCP:           true,
}

// unknownRouteRows is the rows whose third column is no known route, compared exactly.
func unknownRouteRows(rows []ManifestRow) []ManifestRow {
	var unknown []ManifestRow
	for _, row := range rows {
		if !knownManifestRoutes[row.Route] {
			unknown = append(unknown, row)
		}
	}
	return unknown
}

func TestKnownManifestRoutes_CoverEveryScopedRoute(t *testing.T) {
	for route := range agentRouteTargets {
		if !knownManifestRoutes[route] {
			t.Errorf("agentRouteTargets has route %q, which knownManifestRoutes lacks", route)
		}
	}
	for route := range skillRouteTargets {
		if !knownManifestRoutes[route] {
			t.Errorf("skillRouteTargets has route %q, which knownManifestRoutes lacks", route)
		}
	}
}

func TestOverlayManifest_EveryThirdColumnIsAKnownRoute(t *testing.T) {
	rows := ReadManifestRows([]byte(readRepoFile(t, skillsRepoRoot(t), "overlay.manifest")))
	for _, row := range unknownRouteRows(rows) {
		t.Errorf("overlay.manifest row %q has third column %q, which is no route: it would take the skill route and be copied to every copy target", row.Path, row.Route)
	}
}

func TestUnknownRouteRows_CatchesAMisspeltRoute(t *testing.T) {
	rows := []ManifestRow{
		{Path: "_shared/x/run.sh", Tag: "custom", Route: "claude-skills"},
		{Path: "_shared/x/b.sh", Tag: "custom", Route: "Claude-Skill"},
		{Path: "plain/SKILL.md", Tag: "custom", Route: ""},
		{Path: "_shared/x/c.sh", Tag: "custom", Route: "claude-skill"},
		{Path: "builder.md", Tag: "custom", Route: "agent"},
	}
	if got, want := unknownRouteRows(rows), rows[:2]; !reflect.DeepEqual(got, want) {
		t.Errorf("unknownRouteRows = %+v, want %+v", got, want)
	}
}

// linesBetween is the lines of text from the first line that satisfies start up to the first line, at or
// after it, that satisfies end; includeEnd says whether that last line is kept. ok is false when there is no
// start line or no end line.
func linesBetween(text string, start, end func(string) bool, includeEnd bool) (block string, ok bool) {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if !start(line) {
			continue
		}
		for j := i; j < len(lines); j++ {
			if end(lines[j]) {
				if includeEnd {
					j++
				}
				return strings.Join(lines[i:j], "\n"), true
			}
		}
		return "", false
	}
	return "", false
}

func TestRouteTexts_NameEveryReasonADestIsEmpty(t *testing.T) {
	root := skillsRepoRoot(t)
	routesGo := readRepoFile(t, root, "engine/skills/routes.go")
	routesSh := readRepoFile(t, root, "bin/lib/manifest_routes.sh")

	blockG, okG := linesBetween(routesGo,
		func(l string) bool { return strings.Contains(l, "// Dest is") },
		func(l string) bool { return strings.Contains(l, "Dest string") }, true)
	blockS, okS := linesBetween(routesSh,
		func(l string) bool { return strings.HasPrefix(l, "#   dest") },
		func(l string) bool { return l == "#" }, false)
	blockR, okR := linesBetween(routesGo,
		func(l string) bool { return strings.Contains(l, "// The skill route, claude-skill") },
		func(l string) bool { return strings.Contains(l, "placed.RepoRel = path") }, false)
	var lineK string
	for _, l := range strings.Split(routesGo, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "// RouteSkill copies") {
			lineK = l
			break
		}
	}

	wants := []string{"opencode", "mcp", "never deployed"}
	for route := range skillRouteTargets {
		wants = append(wants, route)
	}
	for _, block := range []struct {
		name, text string
		found      bool
	}{
		{"block G (the Dest comment of engine/skills/routes.go)", blockG, okG},
		{"block S (the dest column of bin/lib/manifest_routes.sh)", blockS, okS},
	} {
		if !block.found {
			t.Errorf("%s not found", block.name)
			continue
		}
		lower := strings.ToLower(block.text)
		for _, want := range wants {
			if !strings.Contains(lower, strings.ToLower(want)) {
				t.Errorf("%s does not say %q:\n%s", block.name, want, block.text)
			}
		}
	}
	// Block R is the comment above the skills-directory branch of resolveRoute: OpenCode gets no copy, and
	// it must not say "every target".
	if !okR {
		t.Error("block R (the skills-directory comment of resolveRoute) not found")
	} else {
		lower := strings.ToLower(blockR)
		rWants := []string{"opencode"}
		for route := range skillRouteTargets {
			rWants = append(rWants, route)
		}
		for _, want := range rWants {
			if !strings.Contains(lower, strings.ToLower(want)) {
				t.Errorf("block R (the skills-directory comment of resolveRoute) does not say %q:\n%s", want, blockR)
			}
		}
		if strings.Contains(lower, "every target") {
			t.Errorf("block R (the skills-directory comment of resolveRoute) says %q:\n%s", "every target", blockR)
		}
	}
	if lineK == "" {
		t.Error("line K (the RouteSkill comment of engine/skills/routes.go) not found")
	} else if !strings.Contains(lineK, "OpenCode") {
		t.Errorf("line K does not name OpenCode: %s", lineK)
	}
}
