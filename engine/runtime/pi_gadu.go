package runtime

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// The GADU agent link: the overlay-owned symlink ~/.pi/agent/agents/GADU.md to the package's own
// agents/GADU.md, how its ownership is proven, and how it is made and removed without touching a
// file that is not the overlay's.

// gaduLinkPath returns the overlay-owned GADU agent link location.
func gaduLinkPath(home string) string {
	return filepath.Join(home, ".pi", "agent", "agents", "GADU.md")
}

// gaduSourcePath returns the package's own generated agents/GADU.md -- the
// STABLE symlink target across `swap` rebuilds (only the inode changes,
// design D10).
func gaduSourcePath(destDir string) string {
	return filepath.Join(destDir, "agents", "GADU.md")
}

// gaduLinkStatus is gaduLinkState's result, the state of the link and not the link: exactly one
// of missing/current/stale/conflict (D13 -- collapsing these into a single boolean was rejected as
// it hides which failure mode is present).
type gaduLinkStatus string

const (
	gaduLinkMissing  gaduLinkStatus = "missing"
	gaduLinkCurrent  gaduLinkStatus = "current"
	gaduLinkStale    gaduLinkStatus = "stale"
	gaduLinkConflict gaduLinkStatus = "conflict"
)

// gaduLinkState reports linkPath's ownership/state relative to
// expectedTarget. Ownership is proven ONLY by os.Readlink equality with
// expectedTarget, never by file contents or a side record: missing (no
// entry), current (our symlink, target resolves), stale (our symlink, but
// its target no longer exists -- e.g. destDir was rebuilt from scratch),
// or conflict (a pre-existing regular file, or a symlink pointing
// elsewhere) -- a conflict is reported and left untouched, never
// overwritten.
func gaduLinkState(linkPath, expectedTarget string) gaduLinkStatus {
	info, err := os.Lstat(linkPath)
	if err != nil {
		return gaduLinkMissing
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return gaduLinkConflict
	}
	target, err := os.Readlink(linkPath)
	if err != nil || filepath.Clean(target) != filepath.Clean(expectedTarget) {
		return gaduLinkConflict
	}
	if _, err := os.Stat(expectedTarget); err != nil {
		return gaduLinkStale
	}
	return gaduLinkCurrent
}

// validateGaduFrontmatter mirrors just enough of pi-subagents-j0k3r's own
// frontmatter parser (config.ts parseFrontmatterWithIssues) to catch the
// one failure mode that silently drops the subagent at dispatch time: a
// `tools` field declared BOTH as an inline scalar and a YAML list. `name`
// and `description` are required (the extension falls back to the
// filename/a generic description otherwise, which is not what GADU wants);
// `model` is optional and unconstrained (R-014). The file is read with
// skills.ReadFrontmatter, the reader the skills lint and the package build
// use, so the three agree on where the frontmatter is and which keys are
// at its top level.
func validateGaduFrontmatter(content string) error {
	fm, err := skills.ReadFrontmatter([]byte(content))
	if err != nil {
		// ReadFrontmatter fails only for a missing fence (a *FrontmatterError); anything it
		// might return later is not read as a frontmatter either.
		var fault *skills.FrontmatterError
		if errors.As(err, &fault) && fault.Fault == skills.NoClosingFence {
			return fmt.Errorf("gadu frontmatter: missing closing --- delimiter")
		}
		return fmt.Errorf("gadu frontmatter: missing opening --- delimiter")
	}
	var name, description string
	var toolsInline, toolsList bool
	for _, entry := range fm.Entries() {
		switch entry.Key {
		case "name":
			name = entry.Value
		case "description":
			description = entry.Value
		case "tools":
			toolsInline = toolsInline || entry.Value != ""
			toolsList = toolsList || entry.Items > 0
		}
	}
	if name == "" {
		return fmt.Errorf("gadu frontmatter: missing required 'name'")
	}
	if description == "" {
		return fmt.Errorf("gadu frontmatter: missing required 'description'")
	}
	if toolsInline && toolsList {
		return fmt.Errorf("gadu frontmatter: 'tools' declared both as an inline scalar and a YAML list; choose one")
	}
	return nil
}

// linkGaduAgent verifies the package's agents/GADU.md frontmatter (R-014),
// then symlinks it at ~/.pi/agent/agents/GADU.md (R-013). A pre-existing
// regular file or a symlink pointing elsewhere is a conflict, reported and
// left untouched -- never overwritten. A stale link (ours, but its target
// no longer exists) is recreated.
func linkGaduAgent(home, destDir string) error {
	target := gaduSourcePath(destDir)
	content, err := os.ReadFile(target)
	if err != nil {
		return fmt.Errorf("gadu link: package agents/GADU.md not found at %s: %w", target, err)
	}
	if err := validateGaduFrontmatter(string(content)); err != nil {
		return err
	}

	linkPath := gaduLinkPath(home)
	switch gaduLinkState(linkPath, target) {
	case gaduLinkCurrent:
		return nil
	case gaduLinkConflict:
		return fmt.Errorf("gadu link: %s exists and is not the overlay-owned symlink; leaving it untouched", linkPath)
	case gaduLinkStale:
		if err := os.Remove(linkPath); err != nil {
			return fmt.Errorf("gadu link: removing stale link: %w", err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(linkPath), 0755); err != nil {
		return fmt.Errorf("gadu link: creating agents dir: %w", err)
	}
	if err := os.Symlink(target, linkPath); err != nil {
		return fmt.Errorf("gadu link: creating symlink: %w", err)
	}
	return nil
}

// unlinkGaduAgent removes ~/.pi/agent/agents/GADU.md ONLY when it is the
// overlay-owned symlink (Readlink equality against destDir's agents/
// GADU.md). Never touches a conflicting entry or any other file in that
// directory, and never removes the Subagents extension package itself
// (R-016).
func unlinkGaduAgent(home, destDir string) error {
	linkPath := gaduLinkPath(home)
	target := gaduSourcePath(destDir)
	switch gaduLinkState(linkPath, target) {
	case gaduLinkMissing:
		return nil
	case gaduLinkConflict:
		return fmt.Errorf("gadu link: %s is not the overlay-owned symlink; leaving it untouched", linkPath)
	default: // current or stale -- still ours
		return os.Remove(linkPath)
	}
}
