package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pipkg"
)

// pipkgFixtureOverlay writes a minimal overlay tree exercising the 'pipkg
// build|check' subcommand end to end.
func pipkgFixtureOverlay(t *testing.T) (overlayRoot, registryPath string) {
	t.Helper()
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "skills", "pi-skill", "SKILL.md"), "---\nname: pi-skill\n---\nbody\n")
	writeTestFile(t, filepath.Join(root, "pi", "agents", "builder.md"), "---\nname: builder\ndescription: test agent\n---\nbody\n")
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

// pipkgArgs is the command line of a verb of pipkg over the paths of a test.
func pipkgArgs(verb, overlayRoot, registryPath, destDir string) []string {
	return []string{verb, "--overlay-root", overlayRoot, "--registry", registryPath, "--dest-dir", destDir}
}

// pipkgAgentLinks is a home in which the deploy before this one linked agents of the package at
// destDir, and the overlay has since dropped one of them. It returns the home and the directory Pi
// reads the agents from. builder.md is an agent the package still ships; GADU.md is one it does not
// (the pipkgFixtureOverlay package has no GADU.md), so its link points at nothing. Beside them the
// directory holds what is not the overlay's: a file of gentle-ai and a link into gentle-pi's package.
func pipkgAgentLinks(t *testing.T, destDir string) (home, agentsHome string) {
	t.Helper()
	home = t.TempDir()
	agentsHome = filepath.Join(home, ".pi", "agent", "agents")
	if err := os.MkdirAll(agentsHome, 0755); err != nil {
		t.Fatal(err)
	}
	// A link may be made before its target is: the target is the package's own file, which a build
	// writes or, for the dropped agent, no longer does.
	for _, file := range []string{"builder.md", "GADU.md"} {
		if err := os.Symlink(filepath.Join(destDir, "agents", file), filepath.Join(agentsHome, file)); err != nil {
			t.Fatal(err)
		}
	}
	writeTestFile(t, filepath.Join(agentsHome, "review-risk.md"), "an agent of gentle-ai\n")
	gentlePiAgent := filepath.Join(t.TempDir(), "gentle-pi", "agents", "jd-judge-a.md")
	writeTestFile(t, gentlePiAgent, "gentle-pi's own agent\n")
	if err := os.Symlink(gentlePiAgent, filepath.Join(agentsHome, "jd-judge-a.md")); err != nil {
		t.Fatal(err)
	}
	return home, agentsHome
}

// assertAgentLinksOfTheHome fails the test unless the home is as pipkgAgentLinks left it, except for
// the link of the dropped agent, which is there or not as dropped says.
func assertAgentLinksOfTheHome(t *testing.T, destDir, agentsHome string, dropped bool) {
	t.Helper()
	if got, err := os.Readlink(filepath.Join(agentsHome, "builder.md")); err != nil || got != filepath.Join(destDir, "agents", "builder.md") {
		t.Errorf("the link of builder.md, an agent the package ships, is %q (err %v), want the package's own file", got, err)
	}
	if data, err := os.ReadFile(filepath.Join(agentsHome, "review-risk.md")); err != nil || string(data) != "an agent of gentle-ai\n" {
		t.Errorf("review-risk.md, a file of gentle-ai, is %q (err %v), want it as it was", data, err)
	}
	if got, err := os.Readlink(filepath.Join(agentsHome, "jd-judge-a.md")); err != nil || !strings.HasSuffix(got, filepath.Join("gentle-pi", "agents", "jd-judge-a.md")) {
		t.Errorf("the link of gentle-pi is %q (err %v), want it as it was", got, err)
	}
	_, err := os.Lstat(filepath.Join(agentsHome, "GADU.md"))
	if dropped && !os.IsNotExist(err) {
		t.Errorf("the link of GADU.md, an agent the package no longer ships, is still there (Lstat err = %v)", err)
	}
	if !dropped && err != nil {
		t.Errorf("the link of GADU.md must be as it was: %v", err)
	}
}

// The deploy runs `pipkg build` first and `pi install` only when a `pi` is on the PATH, so the build
// is the one step every run of `apply --target pi` goes through. It swaps the package in whole: an
// agent the overlay dropped (GADU.md, once the role agents replaced it) is gone from it, and the link
// a deploy before made for it points at nothing. The build removes that link and tells it; the link
// of an agent the package still ships, a file of gentle-ai and a link of gentle-pi stay as they are.
func TestRunPipkgCore_BuildRetiresTheLinkOfAnAgentThePackageNoLongerShips(t *testing.T) {
	overlayRoot, registryPath := pipkgFixtureOverlay(t)
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	home, agentsHome := pipkgAgentLinks(t, destDir)
	d := testDeps()
	d.getenv = environmentOf(map[string]string{"HOME": home})

	var outBuf, errBuf bytes.Buffer
	exitCode := -1
	runPipkgCore(d, noGit(), pipkgArgs("build", overlayRoot, registryPath, destDir), &outBuf, &errBuf, func(code int) { exitCode = code })

	if exitCode != 0 {
		t.Fatalf("pipkg build exited %d, stderr=%q", exitCode, errBuf.String())
	}
	want := "pipkg build: labdrian-pi package written to " + destDir + "\n" +
		"pipkg build: GADU.md link removed (the package no longer ships it).\n"
	if outBuf.String() != want {
		t.Errorf("pipkg build said:\n%q\nwant:\n%q", outBuf.String(), want)
	}
	if errBuf.Len() != 0 {
		t.Errorf("pipkg build wrote to stderr: %q", errBuf.String())
	}
	if _, err := os.Stat(filepath.Join(destDir, "agents", "GADU.md")); !os.IsNotExist(err) {
		t.Fatalf("the built package must not hold GADU.md, Stat err = %v", err)
	}
	assertAgentLinksOfTheHome(t, destDir, agentsHome, true)
}

// A build that retires no link says what it always said and touches nothing of the home: the home
// holds the links of the agents the package ships and what is not the overlay's, and nothing else.
func TestRunPipkgCore_BuildSaysNothingMoreWhenNoLinkIsToRetire(t *testing.T) {
	overlayRoot, registryPath := pipkgFixtureOverlay(t)
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	home, agentsHome := pipkgAgentLinks(t, destDir)
	if err := os.Remove(filepath.Join(agentsHome, "GADU.md")); err != nil {
		t.Fatal(err)
	}
	d := testDeps()
	d.getenv = environmentOf(map[string]string{"HOME": home})

	var outBuf, errBuf bytes.Buffer
	exitCode := -1
	runPipkgCore(d, noGit(), pipkgArgs("build", overlayRoot, registryPath, destDir), &outBuf, &errBuf, func(code int) { exitCode = code })

	if exitCode != 0 {
		t.Fatalf("pipkg build exited %d, stderr=%q", exitCode, errBuf.String())
	}
	if want := "pipkg build: labdrian-pi package written to " + destDir + "\n"; outBuf.String() != want {
		t.Errorf("pipkg build said %q, want %q", outBuf.String(), want)
	}
	if errBuf.Len() != 0 {
		t.Errorf("pipkg build wrote to stderr: %q", errBuf.String())
	}
	assertAgentLinksOfTheHome(t, destDir, agentsHome, true)
}

// A machine whose home cannot be told has no link to retire: the build says what it always said.
func TestRunPipkgCore_BuildWithoutAHomeRetiresNothing(t *testing.T) {
	overlayRoot, registryPath := pipkgFixtureOverlay(t)
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")

	var outBuf, errBuf bytes.Buffer
	exitCode := -1
	runPipkgCore(deps{}, noGit(), pipkgArgs("build", overlayRoot, registryPath, destDir), &outBuf, &errBuf, func(code int) { exitCode = code })

	if exitCode != 0 {
		t.Fatalf("pipkg build exited %d, stderr=%q", exitCode, errBuf.String())
	}
	if want := "pipkg build: labdrian-pi package written to " + destDir + "\n"; outBuf.String() != want {
		t.Errorf("pipkg build said %q, want %q", outBuf.String(), want)
	}
}

// The check reads and reports: it removes no link, whatever the home holds, and says nothing of one.
func TestRunPipkgCore_CheckRetiresNothing(t *testing.T) {
	overlayRoot, registryPath := pipkgFixtureOverlay(t)
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	var buildOut, buildErr bytes.Buffer
	runPipkgCore(testDeps(), noGit(), pipkgArgs("build", overlayRoot, registryPath, destDir), &buildOut, &buildErr, func(int) {})
	home, agentsHome := pipkgAgentLinks(t, destDir)
	d := testDeps()
	d.getenv = environmentOf(map[string]string{"HOME": home})

	var outBuf, errBuf bytes.Buffer
	runPipkgCore(d, noGit(), pipkgArgs("check", overlayRoot, registryPath, destDir), &outBuf, &errBuf, func(int) {})

	if strings.Contains(outBuf.String()+errBuf.String(), "link removed") {
		t.Errorf("pipkg check tells a link removed:\nstdout=%q\nstderr=%q", outBuf.String(), errBuf.String())
	}
	assertAgentLinksOfTheHome(t, destDir, agentsHome, false)
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
