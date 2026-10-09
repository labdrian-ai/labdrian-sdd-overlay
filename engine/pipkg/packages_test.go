package pipkg_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pipkg"
)

// Packages is the object the Pi adapter holds as its package builder: the registry reader and
// the Options of the run, fixed once, behind the two verbs the adapter needs.

func TestPackagesBuildThenCheckReportsNoDrift(t *testing.T) {
	overlayRoot, registryPath := fixtureOverlay(t)
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	packages := packagesOf(fileRegistries)

	if err := packages.Build(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "package.json")); err != nil {
		t.Fatalf("Build must write the package: %v", err)
	}
	disclosure, err := packages.Check(overlayRoot, registryPath, destDir)
	if err != nil {
		t.Fatalf("Check right after Build: %v", err)
	}
	if !strings.Contains(disclosure, "worktree") {
		t.Errorf("the disclosure of a non-git overlay should say what it compared against, got %q", disclosure)
	}
}

func TestPackagesCheckReportsDriftAndStillDiscloses(t *testing.T) {
	overlayRoot, registryPath := fixtureOverlay(t)
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	packages := packagesOf(fileRegistries)
	if err := packages.Build(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("Build: %v", err)
	}
	writeFile(t, filepath.Join(overlayRoot, "skills", "pi-skill", "SKILL.md"), "---\nname: pi-skill\n---\nedited\n")

	disclosure, err := packages.Check(overlayRoot, registryPath, destDir)
	if err == nil || !strings.Contains(err.Error(), "SKILL.md") {
		t.Fatalf("Check must name the drifted file, got err=%v", err)
	}
	if disclosure == "" {
		t.Error("a Check that finds drift must still say what it compared against")
	}
}

// The Options of the value reach Check: the deploy ref it names is the one compared against.
func TestPackagesCheckUsesItsOptions(t *testing.T) {
	overlayRoot, registryPath, _ := gitFixtureOverlay(t)
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	runGit(t, overlayRoot, "checkout", "-q", "-b", "feature")
	if err := packagesOf(fileRegistries).Build(overlayRoot, registryPath, destDir); err != nil {
		t.Fatalf("Build: %v", err)
	}

	disclosure, _ := packagesOf(fileRegistries, pipkg.Options{DeployRef: "feature"}).Check(overlayRoot, registryPath, destDir)
	if !strings.Contains(disclosure, "compared against feature (") {
		t.Errorf("disclosure = %q, want it to name the feature ref the Options asked for", disclosure)
	}
	disclosure, _ = packagesOf(fileRegistries).Check(overlayRoot, registryPath, destDir)
	if strings.Contains(disclosure, "compared against feature (") {
		t.Errorf("disclosure = %q, want the default ref when the Options name none", disclosure)
	}
}
