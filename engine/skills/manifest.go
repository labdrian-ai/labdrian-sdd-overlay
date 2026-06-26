package skills

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// infraPrefixes lists path prefixes that are never skill directories.
// Rows whose first path segment starts with any of these are excluded from ManifestView.
var infraPrefixes = []string{"engine/", "_shared/"}

// ManifestEntry records the skill directory name and its raw manifest tag.
type ManifestEntry struct {
	Dir         string // first path component of the SKILL.md row (e.g. "sdd-spec")
	Tag         string // raw manifest tag: "managed" or "custom"
	HasConflict bool   // true when the same dir has SKILL.md rows with different tags
}

// ManifestView is a map from skill directory name to ManifestEntry.
// It is derived from overlay.manifest by collapsing per-file rows to their
// top-level skill directory, excluding infra prefixes, and keeping only
// directories identified by a SKILL.md row.
type ManifestView map[string]ManifestEntry

// LoadManifestView reads the file at path and returns a ManifestView.
//
// Rules:
//   - Blank lines and lines starting with '#' are skipped.
//   - Only rows containing '<path> <tag>' are processed.
//   - A skill directory is identified by a row whose path ends with '/SKILL.md'.
//     The first path component (before the first '/') is the directory name.
//   - Rows whose first path component matches an infra prefix are excluded.
//   - Non-SKILL.md rows are ignored for directory discovery; they do not produce entries.
//   - If multiple SKILL.md rows share the same directory, the first one wins.
func LoadManifestView(path string) (ManifestView, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("manifest: open %s: %w", path, err)
	}
	defer f.Close()
	return loadManifestViewReader(f)
}

// loadManifestViewReader parses a ManifestView from r. It is the pure, reader-based
// core of LoadManifestView, extracted to allow in-memory callers (ADR-9 step 7).
func loadManifestViewReader(r io.Reader) (ManifestView, error) {
	mv := make(ManifestView)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		rowPath, tag := fields[0], fields[1]

		// Only SKILL.md rows create directory entries.
		if !strings.HasSuffix(rowPath, "/SKILL.md") {
			continue
		}

		// Extract the first path component as the skill directory.
		slashIdx := strings.IndexByte(rowPath, '/')
		if slashIdx < 0 {
			continue
		}
		dirName := rowPath[:slashIdx]

		// Exclude infra prefixes.
		if isInfraDir(dirName) {
			continue
		}

		if existing, exists := mv[dirName]; !exists {
			// First SKILL.md row for this dir.
			mv[dirName] = ManifestEntry{Dir: dirName, Tag: tag}
		} else if tag != existing.Tag {
			// A later SKILL.md row for the same dir carries a different tag: record conflict.
			me := mv[dirName]
			me.HasConflict = true
			mv[dirName] = me
		}
		// Same tag seen again: no-op (no conflict, first row already recorded).
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("manifest: read error: %w", err)
	}
	return mv, nil
}

// isInfraDir reports whether dirName matches an infra prefix.
func isInfraDir(dirName string) bool {
	for _, prefix := range infraPrefixes {
		// prefix is like "engine/"; dirName is like "engine"
		// check if dirName+"/" == prefix
		if dirName+"/" == prefix {
			return true
		}
	}
	return false
}
