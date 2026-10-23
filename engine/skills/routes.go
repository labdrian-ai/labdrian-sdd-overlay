package skills

import (
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability"
)

// The routing of the rows of overlay.manifest: for each row, which route it takes, where its file
// is in the overlay and where it is copied to for a target. This is the one place that decides
// it (Phase 9, B1): the overlay script used to hold the decision in route_resolve and read the
// manifest with grep and awk, and now asks the engine (`engine manifest routes`), which answers
// from here.
//
// The rules are the script's, kept as they were so that what it does with a manifest does not
// change: how a line is read (ReadManifestRows), which route a row takes and which rows are never
// deployed (ResolveRoutes), and what is refused (UnroutedRowError). Where the script was lenient
// the code says so and keeps the leniency; a change to it is a decision of the owner, not a side
// effect of moving the code.

// The routes of a row, the value of its third column. A row with no third column takes RouteSkill.
// A value outside these is not refused (except under longterm-mem/, see UnroutedRowError): the row
// takes the skill route and keeps the value as its name.
const (
	// RouteSkill copies a row from skills/ to the skills directory of every target.
	RouteSkill = "skill"
	// RouteAgent copies a row from agents/ to the agents directory of Claude Code.
	RouteAgent = "agent"
	// RouteOpencodeAgent copies a row from opencode/agents/ to the agents directory of OpenCode.
	RouteOpencodeAgent = "opencode-agent"
	// RouteCodexAgent copies a row from codex/agents/ to the agents directory of Codex.
	RouteCodexAgent = "codex-agent"
	// RouteMCP names a row of the longterm-mem install, which is dispatched on its own: the row
	// has a source in the overlay and is copied nowhere.
	RouteMCP = "mcp"
	// RouteNonDeployable names a row that no directory of the overlay holds: an engine source
	// file, a file at the root, an opencode-agent or codex-agent row outside its runtime's agents
	// directory of the overlay. The overlay tracks it for comparison and never copies it.
	RouteNonDeployable = "non-deployable"
)

// managedTag is the second column of a row whose files the overlay takes from upstream. The rows
// with any other tag (or none) are the overlay's own.
const managedTag = "managed"

// ManifestRow is one row of overlay.manifest: the columns the script reads, empty when the row
// has fewer. Path is the first column, as written.
type ManifestRow struct {
	Path  string
	Tag   string
	Route string
}

// ReadManifestRows reads the rows of a manifest, in the order they are written.
//
// It reads a line the way the script did with grep and awk, which is not how a person would read
// it, and the difference is kept: a line is skipped when nothing but white space (space, tab,
// vertical tab, form feed, carriage return) comes before its end or before a '#'; the columns of a
// row are separated by spaces and tabs and by nothing else, so a carriage return stays on the last
// column of a line that has one, and a comment that starts after a column is a column; a last line
// with no line end is a row. The other readers of the manifest in this package are more tolerant
// of white space and are not the ones to ask where a row goes.
func ReadManifestRows(manifest []byte) []ManifestRow {
	var rows []ManifestRow
	for _, line := range strings.Split(string(manifest), "\n") {
		if isSkippedManifestLine(line) {
			continue
		}
		columns := strings.FieldsFunc(line, func(r rune) bool { return r == ' ' || r == '\t' })
		row := ManifestRow{Path: columns[0]}
		if len(columns) > 1 {
			row.Tag = columns[1]
		}
		if len(columns) > 2 {
			row.Route = columns[2]
		}
		rows = append(rows, row)
	}
	return rows
}

// isSkippedManifestLine reports whether a line is blank or a comment: the first character of it
// that is not white space is none, or '#'. A line that is not skipped has a character that is not
// a space or a tab, so it has at least one column.
func isSkippedManifestLine(line string) bool {
	for i := 0; i < len(line); i++ {
		if c := line[i]; c != ' ' && (c < '\t' || c > '\r') {
			return c == '#'
		}
	}
	return true
}

// RouteDestinations says where the rows are copied to, for one target. The overlay script owns
// the directories of its targets and hands them over; the domain decides which row goes to which.
type RouteDestinations struct {
	// Target is the runtime the destinations are asked for.
	Target string
	// SkillsRoot is the directory that holds the skills of Target. A skill row is copied to
	// SkillsRoot and the row's path.
	SkillsRoot string
	// AgentsRoot is the directory that holds the native agents of Target, for a target that has
	// them (Claude Code, OpenCode and Codex). A row is refused when it needs one and there is none.
	AgentsRoot string
}

// ResolvedRoute is what the overlay does with one row for one target.
type ResolvedRoute struct {
	// Path is the first column of the row, as written.
	Path string
	// Route is the route the row takes: one of the Route constants, or the value of a third
	// column the domain does not know, which the row keeps as its name.
	Route string
	// RepoRel is where the row's file is in the overlay repository, relative to its root: the path
	// that git knows it by on the branches.
	RepoRel string
	// RepoSource is the absolute path of the file the overlay copies from; empty for a row that
	// is never deployed.
	RepoSource string
	// Dest is the absolute path the file is copied to for the target; empty when the row goes
	// nowhere for it (a native agent of another runtime, an mcp row, a row never deployed).
	Dest string
}

// UnroutedRowError is a row under longterm-mem/ whose third column is missing or outside the
// four routes: it is refused rather than let to take the skill route, which would copy the
// sources of the longterm-mem binary into the skills directories (R-012 in overlay-agent-route).
// The words are the ones the overlay script has always said it in, and the pins of the script
// hold them.
type UnroutedRowError struct {
	Path  string
	Route string
}

func (e *UnroutedRowError) Error() string {
	const domain = "must be one of: skill, agent, opencode-agent, codex-agent, mcp"
	if e.Route == "" {
		return fmt.Sprintf("route_resolve: longterm-mem row '%s' declares no route (missing third column); %s", e.Path, domain)
	}
	return fmt.Sprintf("route_resolve: longterm-mem row '%s' declares an unrecognized route '%s'; %s", e.Path, e.Route, domain)
}

// UnknownTargetError is a target the engine has no runtime by.
type UnknownTargetError struct{ Target string }

func (e *UnknownTargetError) Error() string {
	return fmt.Sprintf("unknown target %q; must be one of: %s", e.Target, strings.Join(capability.Targets(), ", "))
}

// MissingRootError is a row that goes to a directory the destinations do not name.
type MissingRootError struct {
	Path string
	// Directory says which directory is missing: "skills" or "agents".
	Directory string
	Target    string
}

func (e *MissingRootError) Error() string {
	return fmt.Sprintf("manifest row '%s' is copied to the %s directory of %s, and none was given", e.Path, e.Directory, e.Target)
}

// agentRouteTargets names the runtime that holds the native agents of a route.
var agentRouteTargets = map[string]string{
	RouteAgent:         capability.TargetClaude,
	RouteOpencodeAgent: capability.TargetOpenCode,
	RouteCodexAgent:    capability.TargetCodex,
}

// ResolveRoutes decides, for each row of the manifest, the route it takes and where its file is
// and goes to. overlayRoot is the directory of the overlay, joined to the paths of the rows by
// concatenation and not cleaned, as the script did, so that what a person sees in a path is what
// they wrote. managedOnly keeps the rows whose tag is "managed", which is what the script's
// commands that move files from upstream read.
//
// A row's route is the third column of the first row that names the same path: a path named
// twice takes the route of the first, whatever the second says. Only the rows kept are resolved
// and only they can be refused, and the first refusal stops the resolution: the caller gets the
// error and no row, so that a manifest with a row that cannot be placed is not half acted on.
func ResolveRoutes(rows []ManifestRow, managedOnly bool, overlayRoot string, dest RouteDestinations) ([]ResolvedRoute, error) {
	if !capability.IsTarget(dest.Target) {
		return nil, &UnknownTargetError{Target: dest.Target}
	}
	declared := make(map[string]string, len(rows))
	for _, row := range rows {
		if _, seen := declared[row.Path]; !seen {
			declared[row.Path] = row.Route
		}
	}
	var resolved []ResolvedRoute
	for _, row := range rows {
		if managedOnly && row.Tag != managedTag {
			continue
		}
		route, err := resolveRoute(row.Path, declared[row.Path], overlayRoot, dest)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, route)
	}
	return resolved, nil
}

// resolveRoute places one row, given the third column its path is known by.
func resolveRoute(path, declared, overlayRoot string, dest RouteDestinations) (ResolvedRoute, error) {
	if strings.HasPrefix(path, "longterm-mem/") && !validLongtermMemRoutes[declared] {
		return ResolvedRoute{}, &UnroutedRowError{Path: path, Route: declared}
	}
	route := declared
	if route == "" {
		route = RouteSkill
	}
	never := ResolvedRoute{Path: path, Route: RouteNonDeployable, RepoRel: path}

	// A path with no directory, or under engine/, is not a skill: the overlay tracks it and never
	// copies it. The native agents of Claude Code are the exception, they are named by a bare file.
	if route != RouteAgent {
		first, _, hasDir := strings.Cut(path, "/")
		if !hasDir || first == "engine" {
			return never, nil
		}
	}

	placed := ResolvedRoute{Path: path, Route: route}
	switch route {
	case RouteAgent:
		placed.RepoRel = "agents/" + path
		placed.RepoSource = overlayRoot + "/agents/" + path
		return withAgentDest(placed, path, dest)
	case RouteOpencodeAgent:
		name := strings.TrimPrefix(path, "opencode/agents/")
		if name == path || name == "" {
			return never, nil
		}
		placed.RepoRel = path
		placed.RepoSource = overlayRoot + "/" + path
		return withAgentDest(placed, name, dest)
	case RouteCodexAgent:
		name := strings.TrimPrefix(path, "codex/agents/")
		if name == path || name == "" {
			return never, nil
		}
		placed.RepoRel = path
		placed.RepoSource = overlayRoot + "/" + path
		return withAgentDest(placed, name, dest)
	case RouteMCP:
		placed.RepoRel = path
		placed.RepoSource = overlayRoot + "/" + path
		return placed, nil
	}
	// The skill route, and the route of any other third column: the skills directory of every
	// target. Only the first names the file by its place in skills/ on the branches.
	placed.RepoRel = path
	if route == RouteSkill {
		placed.RepoRel = "skills/" + path
	}
	placed.RepoSource = overlayRoot + "/skills/" + path
	if dest.SkillsRoot == "" {
		return ResolvedRoute{}, &MissingRootError{Path: path, Directory: "skills", Target: dest.Target}
	}
	placed.Dest = dest.SkillsRoot + "/" + path
	return placed, nil
}

// withAgentDest says where a native agent goes for the target: its name under the agents directory
// when the target is the runtime the route belongs to, and nowhere when it is another.
func withAgentDest(placed ResolvedRoute, name string, dest RouteDestinations) (ResolvedRoute, error) {
	if agentRouteTargets[placed.Route] != dest.Target {
		return placed, nil
	}
	if dest.AgentsRoot == "" {
		return ResolvedRoute{}, &MissingRootError{Path: placed.Path, Directory: "agents", Target: dest.Target}
	}
	placed.Dest = dest.AgentsRoot + "/" + name
	return placed, nil
}
