package skills

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Divergence classes for the on-disk cross-check. They are deliberately distinct
// from the registry/manifest classes in validate.go: Diff compares two
// REGISTRATION artifacts against each other, so a skill absent from both reads as
// aligned. DiffOnDisk compares the filesystem against the manifest, which is the
// only way that case becomes visible.
const (
	// DivUnregisteredOnDisk reports a file under skills/ with no deploying
	// manifest row. Such a file is never copied to any runtime target.
	DivUnregisteredOnDisk DivergenceClass = "UNREGISTERED_ON_DISK"
	// DivMissingOnDisk reports a deploying manifest row whose source file is
	// absent from skills/. The deploy step cannot satisfy such a row.
	DivMissingOnDisk DivergenceClass = "MISSING_ON_DISK"
)

// nonSkillRoutes lists the third-column route values that send a manifest row
// somewhere other than skills/. Mirrors route_resolve in bin/labdrian-overlay,
// where any other third-column value falls through to the default skill route.
// The full route domain is {skill, agent, opencode-agent, mcp} (D13); "skill"
// is not listed here because it is the default route and IS the skills
// destination, so it must stay out of this exclusion set.
var nonSkillRoutes = map[string]bool{
	"agent":          true,
	"opencode-agent": true,
	"mcp":            true,
}

// validLongtermMemRoutes is the full four-value route domain (D13), used only
// to validate rows under longterm-mem/**. Unlike nonSkillRoutes, "skill" is
// included here: a longterm-mem/** row explicitly routed "skill" is not
// itself invalid (R-012 forbids a MISSING or UNRECOGNIZED route, not the
// skill route specifically), so this set intentionally is NOT nonSkillRoutes
// plus one entry — it validates a different question (is this route
// recognized at all?) than nonSkillRoutes answers (does this route leave
// skills/?).
var validLongtermMemRoutes = map[string]bool{
	"skill":          true,
	"agent":          true,
	"opencode-agent": true,
	"mcp":            true,
}

// DeployableManifestPaths parses a manifest and returns the set of row paths that
// route to skills/, keyed by the row path exactly as written.
//
// It mirrors route_resolve in bin/labdrian-overlay, which is the single source of
// truth for row → routing:
//   - Blank lines and lines starting with '#' are skipped.
//   - A row needs at least two fields; the first is the path. This applies to
//     every path prefix except longterm-mem/** — see below.
//   - A third column of "agent", "opencode-agent", or "mcp" routes outside
//     skills/. Any other third-column value falls through to the skill route.
//   - On the skill route, a path with no '/' is a root-level bookkeeping row and
//     a path whose first segment is "engine" is overlay infra. Both are tracked
//     for diff purposes only and are never deployed.
//
// One case is REJECTED rather than falling through, and is checked BEFORE the
// "at least two fields" rule above so it also catches a one-column row (path
// only, no tag): a row whose path is under longterm-mem/** with a missing or
// unrecognized third column (R-012 in overlay-agent-route, traces
// longterm-mem R-035). Mirrors route_reject_unrouted_longterm_mem in
// bin/labdrian-overlay — that guard is the single source of truth this one is
// kept in sync with (see
// TestRouteDomain_MatchesBashAndGo).
func DeployableManifestPaths(r io.Reader) (map[string]struct{}, error) {
	paths := make(map[string]struct{})
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		rowPath := fields[0]

		routeRaw := ""
		if len(fields) >= 3 {
			routeRaw = fields[2]
		}

		// The longterm-mem/** route-domain guard must see every row with this
		// prefix, including a one-column row (path only, no tag). Mirrors
		// bash: all_tracked_files (awk '{print $1}') still emits such a row,
		// and route_resolve's awk lookup for the third field still resolves
		// to an empty (undefaulted) string for it, so
		// route_reject_unrouted_longterm_mem still fires. This check must
		// therefore run BEFORE the generic "fewer than two fields" skip
		// below, which otherwise applies identically to every path prefix
		// and must not change for anything other than longterm-mem/**.
		if strings.HasPrefix(rowPath, "longterm-mem/") && !validLongtermMemRoutes[routeRaw] {
			if routeRaw == "" {
				return nil, fmt.Errorf("ondisk: manifest row %q under longterm-mem/** declares no route (missing third column); must be one of: skill, agent, opencode-agent, mcp", rowPath)
			}
			return nil, fmt.Errorf("ondisk: manifest row %q under longterm-mem/** declares an unrecognized route %q; must be one of: skill, agent, opencode-agent, mcp", rowPath, routeRaw)
		}

		if len(fields) < 2 {
			continue
		}

		if nonSkillRoutes[routeRaw] {
			continue
		}

		slashIdx := strings.IndexByte(rowPath, '/')
		if slashIdx < 0 {
			continue
		}
		if rowPath[:slashIdx] == "engine" {
			continue
		}

		paths[rowPath] = struct{}{}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("ondisk: read manifest: %w", err)
	}
	return paths, nil
}

// ScanSkillFiles walks skillsDir and returns every regular file it contains as a
// slash-separated path relative to skillsDir, sorted.
//
// Entries whose name begins with '.' are skipped, files and directories alike.
// Nothing the overlay deploys is dot-prefixed, so this keeps editor scratch files
// and VCS metadata from being reported as unregistered content without weakening
// the guard for anything real.
func ScanSkillFiles(skillsDir string) ([]string, error) {
	info, err := os.Stat(skillsDir)
	if err != nil {
		return nil, fmt.Errorf("ondisk: stat skills dir %s: %w", skillsDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("ondisk: %s is not a directory", skillsDir)
	}

	var out []string
	err = filepath.WalkDir(skillsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == skillsDir {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, relErr := filepath.Rel(skillsDir, path)
		if relErr != nil {
			return relErr
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("ondisk: walk %s: %w", skillsDir, err)
	}

	sort.Strings(out)
	return out, nil
}

// DiffOnDisk cross-checks the files actually present under skills/ against the
// manifest rows that deploy from it, in both directions. It performs a full scan
// and never stops at the first divergence. Results are ordered: unregistered
// files first in disk order, then missing sources in sorted row order.
func DiffOnDisk(diskPaths []string, manifestPaths map[string]struct{}) []Divergence {
	var divs []Divergence

	onDisk := make(map[string]struct{}, len(diskPaths))
	for _, p := range diskPaths {
		onDisk[p] = struct{}{}
		if _, ok := manifestPaths[p]; !ok {
			divs = append(divs, Divergence{
				Class: DivUnregisteredOnDisk,
				Path:  p,
				Detail: fmt.Sprintf(
					"skills/%s exists on disk but no overlay.manifest row deploys it; add a row for %q to overlay.manifest to register it",
					p, p,
				),
			})
		}
	}

	var missing []string
	for p := range manifestPaths {
		if _, ok := onDisk[p]; !ok {
			missing = append(missing, p)
		}
	}
	sort.Strings(missing)
	for _, p := range missing {
		divs = append(divs, Divergence{
			Class: DivMissingOnDisk,
			Path:  p,
			Detail: fmt.Sprintf(
				"overlay.manifest deploys %q but skills/%s does not exist; remove the %q row from overlay.manifest, or restore the missing file",
				p, p, p,
			),
		})
	}

	return divs
}
