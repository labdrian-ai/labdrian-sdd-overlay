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
	runPipkgCore(testDeps(), noGit(),
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
	runPipkgCore(testDeps(), noGit(),
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
	runPipkgCore(testDeps(), noGit(),
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
	runPipkgCore(testDeps(), noGit(), []string{}, &outBuf, &errBuf, func(code int) { exitCode = code })
	if exitCode != 1 {
		t.Fatalf("pipkg with no verb should exit 1, got %d", exitCode)
	}
}

// recordingSource is a SourceRepo that says nothing is a repository and remembers it was asked.
type recordingSource struct {
	pipkg.NoRepository
	asked   int
	commits int
}

func (r *recordingSource) HasCommit(dir, ref string) (bool, error) {
	r.commits++
	return r.NoRepository.HasCommit(dir, ref)
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
	runPipkgCore(testDeps(), source, []string{"build", "--overlay-root", overlayRoot, "--registry", registryPath, "--dest-dir", destDir}, &outBuf, &errBuf, func(int) {})
	if source.asked == 0 {
		t.Errorf("pipkg build never asked the source about the overlay; stderr=%q", errBuf.String())
	}
}

// The Pi runtime builds its package asking git through the source the command is given.
func TestRuntimeInstallPiAsksGitThroughTheSourceItIsGiven(t *testing.T) {
	piOverlayWorld(t)
	source := &recordingSource{}

	var out, errOut bytes.Buffer
	runRuntimeCore(testDeps(), &scriptedPiCommands{}, source, []string{"install", "--target", "pi"}, &out, &errOut, func(int) {})
	if source.asked == 0 {
		t.Errorf("the Pi package build never asked the source about the overlay; stdout=%q stderr=%q", out.String(), errOut.String())
	}
}

// The deploy ref the check compares against is the one the deps' environment gives, and the
// process's own environment is not looked at.
func TestRunPipkgCore_CheckReadsTheDeployRefFromTheDeps(t *testing.T) {
	overlayRoot, registryPath := pipkgFixtureOverlay(t)
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	var outBuf, errBuf bytes.Buffer
	runPipkgCore(testDeps(), noGit(), []string{"build", "--overlay-root", overlayRoot, "--registry", registryPath, "--dest-dir", destDir}, &outBuf, &errBuf, func(int) {})
	check := []string{"check", "--overlay-root", overlayRoot, "--registry", registryPath, "--dest-dir", destDir}

	t.Run("the ref of the deps is asked about first", func(t *testing.T) {
		d := testDeps()
		d.getenv = environmentOf(map[string]string{deployRefVariable: "a-ref-of-the-deps"})
		source := &refAskingSource{}
		runPipkgCore(d, source, check, &outBuf, &errBuf, func(int) {})
		if len(source.refs) == 0 || source.refs[0] != "a-ref-of-the-deps" {
			t.Errorf("the check asked git about the refs %q, want the deploy ref the deps gave first", source.refs)
		}
	})
	t.Run("the ref of the process is not asked about", func(t *testing.T) {
		t.Setenv(deployRefVariable, "a-ref-of-the-process")
		d := testDeps()
		d.getenv = environmentOf(nil)
		source := &refAskingSource{}
		runPipkgCore(d, source, check, &outBuf, &errBuf, func(int) {})
		if len(source.refs) == 0 || source.refs[0] != "main" {
			t.Errorf("the check asked git about the refs %q, want the usual ones (main first) since the deps gave none", source.refs)
		}
	})
}

// refAskingSource is a source tree under version control, clean, that holds no commit of any name,
// and records which refs it was asked about.
type refAskingSource struct {
	pipkg.NoRepository
	refs []string
}

func (r *refAskingSource) IsWorkTree(string) (bool, error)            { return true, nil }
func (r *refAskingSource) HasChanges(string, ...string) (bool, error) { return false, nil }
func (r *refAskingSource) HasCommit(_, ref string) (bool, error) {
	r.refs = append(r.refs, ref)
	return false, nil
}

// The build reads the deploy ref along with the rest of the options, but nothing in a build uses
// it: only the check compares against a ref. So a ref in the environment, even one the adapter
// would refuse, neither changes the build nor makes it ask git which commit the ref names.
func TestRunPipkgCore_BuildIgnoresTheDeployRef(t *testing.T) {
	overlayRoot, registryPath := pipkgFixtureOverlay(t)
	t.Setenv("LABDRIAN_PI_DEPLOY_REF", "--output=/nonexistent/never")
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	source := &recordingSource{}

	var outBuf, errBuf bytes.Buffer
	code := -1
	runPipkgCore(testDeps(), source, []string{"build", "--overlay-root", overlayRoot, "--registry", registryPath, "--dest-dir", destDir}, &outBuf, &errBuf, func(c int) { code = c })
	if code != 0 {
		t.Fatalf("pipkg build exited %d, stderr=%q", code, errBuf.String())
	}
	if source.commits != 0 {
		t.Errorf("the build asked git about %d ref(s); only the check compares against one", source.commits)
	}
}
