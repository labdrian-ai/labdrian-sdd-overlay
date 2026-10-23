package skills_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// TestEveryManifestRowExistsInTheRepository resolves every row of the repository's
// overlay.manifest, whatever its tag or route, to the place the overlay keeps it and requires
// that place to exist. skills validate checks only the rows deployed as skills; the engine,
// root-level, agent and mcp rows are compared but never copied, so a deleted file left its row
// behind unseen until this test.
func TestEveryManifestRowExistsInTheRepository(t *testing.T) {
	root := repoRoot(t)
	manifest, err := os.ReadFile(filepath.Join(root, "overlay.manifest"))
	if err != nil {
		t.Fatalf("reading overlay.manifest: %v", err)
	}
	rows := skills.ReadManifestRows(manifest)
	if len(rows) == 0 {
		t.Fatal("overlay.manifest has no rows")
	}
	resolved, err := skills.ResolveRoutes(rows, false, root, skills.RouteDestinations{
		Target:     capability.TargetClaude,
		SkillsRoot: t.TempDir(),
		AgentsRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("resolving the manifest rows: %v", err)
	}
	for _, r := range resolved {
		// RepoSource is where a deployed row is copied from; a row with an unknown route keeps its
		// path as RepoRel but is copied from skills/, so the source decides. A row never deployed has
		// no source, and RepoRel is its place in the repository.
		place := r.RepoSource
		if place == "" {
			place = filepath.Join(root, r.RepoRel)
		}
		if _, err := os.Stat(place); err != nil {
			t.Errorf("manifest row %q (route %s) has no file at %s: %v", r.Path, r.Route, place, err)
		}
	}
}
