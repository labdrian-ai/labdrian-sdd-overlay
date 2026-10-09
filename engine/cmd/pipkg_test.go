package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pipkg"
)

// pipkgFixtureOverlay writes a minimal overlay tree exercising the 'pipkg
// build|check' subcommand end to end.
func pipkgFixtureOverlay(t *testing.T) (overlayRoot, registryPath string) {
	t.Helper()
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "skills", "pi-skill", "SKILL.md"), "---\nname: pi-skill\n---\nbody\n")
	writeTestFile(t, filepath.Join(root, "agents", "GADU.md"), "---\nname: GADU\n---\nbody\n")
	writeTestFile(t, filepath.Join(root, "skills", "_shared", "minimalism-contract.md"),
		"---\napplies_to_phases: [sdd-tasks, sdd-apply]\nexcluded_phases: [sdd-verify]\ninjection_point: \"## Skills to load before work\"\n---\nbody\n")
	writeTestFile(t, filepath.Join(root, "skills", "_shared", "anti-generic-design.md"),
		"---\napplies_to_phases: [sdd-tasks, sdd-apply]\nexcluded_phases: [sdd-verify]\ninjection_point: \"## Skills to load before work\"\n---\nbody\n")
	registry := `version: "1"
skills:
  - id: pi-skill
    path: pi-skill
    source:
      type: custom
    install:
      defaultScope: global
      targets:
        - pi
    lifecycle:
      updateStrategy: overlay-only
`
	regPath := filepath.Join(root, "skills.registry.yaml")
	writeTestFile(t, regPath, registry)
	return root, regPath
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestRunPipkgCore_BuildThenCheck(t *testing.T) {
	overlayRoot, registryPath := pipkgFixtureOverlay(t)
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")

	var outBuf, errBuf bytes.Buffer
	exitCode := -1
	runPipkgCore(noGit(),
		[]string{"build", "--overlay-root", overlayRoot, "--registry", registryPath, "--dest-dir", destDir},
		&outBuf, &errBuf, func(code int) { exitCode = code },
	)
	if exitCode != 0 {
		t.Fatalf("pipkg build should exit 0, got %d, stderr=%q", exitCode, errBuf.String())
	}
	if _, err := os.Stat(filepath.Join(destDir, "package.json")); err != nil {
		t.Fatalf("pipkg build must write package.json: %v", err)
	}

	outBuf.Reset()
	errBuf.Reset()
	exitCode = -1
	runPipkgCore(noGit(),
		[]string{"check", "--overlay-root", overlayRoot, "--registry", registryPath, "--dest-dir", destDir},
		&outBuf, &errBuf, func(code int) { exitCode = code },
	)
	if exitCode != 0 {
		t.Fatalf("pipkg check should exit 0 right after build, got %d, stderr=%q", exitCode, errBuf.String())
	}
}

func TestRunPipkgCore_CheckReportsDriftBeforeBuild(t *testing.T) {
	overlayRoot, registryPath := pipkgFixtureOverlay(t)
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")

	var outBuf, errBuf bytes.Buffer
	exitCode := -1
	runPipkgCore(noGit(),
		[]string{"check", "--overlay-root", overlayRoot, "--registry", registryPath, "--dest-dir", destDir},
		&outBuf, &errBuf, func(code int) { exitCode = code },
	)
	if exitCode != 1 {
		t.Fatalf("pipkg check on an unbuilt package should exit 1, got %d", exitCode)
	}
	if errBuf.Len() == 0 {
		t.Fatal("pipkg check on an unbuilt package should report the drift/missing reason on stderr")
	}
}

func TestRunPipkgCore_MissingVerbFailsLoud(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	exitCode := -1
	runPipkgCore(noGit(), []string{}, &outBuf, &errBuf, func(code int) { exitCode = code })
	if exitCode != 1 {
		t.Fatalf("pipkg with no verb should exit 1, got %d", exitCode)
	}
}

// recordingSource is a SourceRepo that says nothing is a repository and remembers it was asked.
type recordingSource struct {
	pipkg.NoRepository
	asked int
}

func (r *recordingSource) HasChanges(dir string, paths ...string) (bool, error) {
	r.asked++
	return r.NoRepository.HasChanges(dir, paths...)
}

// The builder asks git through the source the command is given, for the build and for the check.
func TestRunPipkgCore_AsksGitThroughTheSourceItIsGiven(t *testing.T) {
	overlayRoot, registryPath := pipkgFixtureOverlay(t)
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	source := &recordingSource{}

	var outBuf, errBuf bytes.Buffer
	runPipkgCore(source, []string{"build", "--overlay-root", overlayRoot, "--registry", registryPath, "--dest-dir", destDir}, &outBuf, &errBuf, func(int) {})
	if source.asked == 0 {
		t.Errorf("pipkg build never asked the source about the overlay; stderr=%q", errBuf.String())
	}
}

// The Pi runtime builds its package asking git through the source the command is given.
func TestRuntimeInstallPiAsksGitThroughTheSourceItIsGiven(t *testing.T) {
	piOverlayWorld(t)
	source := &recordingSource{}

	var out, errOut bytes.Buffer
	runRuntimeCore(&scriptedPiCommands{}, source, []string{"install", "--target", "pi"}, &out, &errOut, func(int) {})
	if source.asked == 0 {
		t.Errorf("the Pi package build never asked the source about the overlay; stdout=%q stderr=%q", out.String(), errOut.String())
	}
}
