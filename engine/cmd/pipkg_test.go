package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// pipkgFixtureOverlay writes a minimal overlay tree exercising the 'pipkg
// build|check' subcommand end to end.
func pipkgFixtureOverlay(t *testing.T) (overlayRoot, registryPath string) {
	t.Helper()
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "skills", "pi-skill", "SKILL.md"), "---\nname: pi-skill\n---\nbody\n")
	writeTestFile(t, filepath.Join(root, "agents", "GADU.md"), "---\nname: GADU\n---\nbody\n")
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
	runPipkgCore(
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
	runPipkgCore(
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
	runPipkgCore(
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
	runPipkgCore([]string{}, &outBuf, &errBuf, func(code int) { exitCode = code })
	if exitCode != 1 {
		t.Fatalf("pipkg with no verb should exit 1, got %d", exitCode)
	}
}
