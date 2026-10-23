package runtime_test

// The Pi adapter links every agent of the package, not only GADU (slice 4 of the role agents): the
// role agents engine/agentgen writes for Pi reach ~/.pi/agent/agents through the same overlay-owned
// symlinks, proven by the same Readlink equality, and Install, Status and Uninstall treat each of
// them as they always treated GADU.md. The tests below drive the adapter through its public methods
// in a home of their own, with a fake CommandRunner: none of them reaches a live Pi.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

// newPiAdapterWithoutAHome is the Pi adapter of a process that cannot tell its home: the paths it
// is given name none, and it must not replace it with the home of the process.
func newPiAdapterWithoutAHome(overlayRoot, registryPath, destDir string) engineRuntime.PiAdapter {
	return engineRuntime.NewPiAdapter(
		engineRuntime.PiPorts{Commands: &fakeCommands{}, Packages: newPiPackages()},
		engineRuntime.PiPaths{OverlayRoot: overlayRoot, RegistryPath: registryPath, DestDir: destDir},
		engineRuntime.PiOptions{},
	)
}

// piRoleAgentSource is the agent file of a role as engine/agentgen writes it for Pi: name,
// description, provider-prefixed model, thinking and an inline tools line.
func piRoleAgentSource(name string) string {
	return "---\nname: " + name + "\ndescription: the " + name + " role\nmodel: pi-claude-cli/claude-sonnet-5\nthinking: high\ntools: read, grep, find\n---\n\nbody of " + name + "\n"
}

// piFixtureOverlayWithAgents is piFixtureOverlay plus the pi/agents files the test names. The
// package takes GADU.md from the generic agents/GADU.md of the fixture, as it does for an overlay
// whose pi/agents has no GADU.
func piFixtureOverlayWithAgents(t *testing.T, agents map[string]string) (overlayRoot, registryPath string) {
	t.Helper()
	overlayRoot, registryPath = piFixtureOverlay(t)
	for file, content := range agents {
		mustWrite(t, filepath.Join(overlayRoot, "pi", "agents", file), content)
	}
	return overlayRoot, registryPath
}

// agentLinkPathIn is where Pi looks for the agent file in the given home.
func agentLinkPathIn(home, file string) string {
	return filepath.Join(home, ".pi", "agent", "agents", file)
}

// writeAgentLinkCurrent symlinks the agent file of the package at its place in the home, as Install
// does, for the tests that need that link without running Install.
func writeAgentLinkCurrent(t *testing.T, home, destDir, file string) {
	t.Helper()
	linkPath := agentLinkPathIn(home, file)
	if err := os.MkdirAll(filepath.Dir(linkPath), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(linkPath), err)
	}
	if err := os.Symlink(filepath.Join(destDir, "agents", file), linkPath); err != nil {
		t.Fatalf("symlink %s: %v", linkPath, err)
	}
}

// assertAgentLinked fails the test unless the agent file of the home is the symlink Install makes to
// the file of the package.
func assertAgentLinked(t *testing.T, home, destDir, file string) {
	t.Helper()
	got, err := os.Readlink(agentLinkPathIn(home, file))
	if err != nil {
		t.Errorf("%s is not linked: %v", file, err)
		return
	}
	if want := filepath.Join(destDir, "agents", file); got != want {
		t.Errorf("%s is linked to %q, want %q", file, got, want)
	}
}

// assertAgentNotLinked fails the test when anything exists at the place of the agent file in the home.
func assertAgentNotLinked(t *testing.T, home, file string) {
	t.Helper()
	if _, err := os.Lstat(agentLinkPathIn(home, file)); !os.IsNotExist(err) {
		t.Errorf("%s must not exist in the home, Lstat err = %v", file, err)
	}
}

// assertMessageHasOnce fails the test unless the message holds the sentence exactly once.
func assertMessageHasOnce(t *testing.T, message, sentence string) {
	t.Helper()
	if n := strings.Count(message, sentence); n != 1 {
		t.Errorf("the message holds %q %d times, want once:\n%s", sentence, n, message)
	}
}

// Install links every agent file of the package, GADU.md and the role agents alike, each to its own
// file of the package, and tells it once.
func TestPiAdapter_InstallLinksEveryAgentOfThePackage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	overlayRoot, registryPath := piFixtureOverlayWithAgents(t, map[string]string{
		"builder.md":  piRoleAgentSource("builder"),
		"reviewer.md": piRoleAgentSource("reviewer"),
	})
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	buildPiPackage(t, overlayRoot, registryPath, destDir)

	result := newPiAdapterAt(t, overlayRoot, registryPath, destDir).Install()
	if result.Status == core.CapabilityUnsupported {
		t.Fatalf("Install must not be unsupported, got: %s", result)
	}
	for _, file := range []string{"GADU.md", "builder.md", "reviewer.md"} {
		assertAgentLinked(t, home, destDir, file)
		assertMessageHasOnce(t, result.Message, file+" linked at ~/.pi/agent/agents/"+file+".")
	}
}

// An agent Install cannot link does not keep the others from being linked: a file of the user at
// the place of one (never overwritten), an agent whose frontmatter Pi would drop at dispatch (never
// linked). Each is told by its own name, the rest are linked and told as always.
func TestPiAdapter_InstallGoesOnWhenOneAgentCannotBeLinked(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	overlayRoot, registryPath := piFixtureOverlayWithAgents(t, map[string]string{
		"builder.md":  piRoleAgentSource("builder"),
		"reviewer.md": piRoleAgentSource("reviewer"),
		"shaper.md":   "---\nname: shaper\n---\n\nbody of shaper\n", // no description
	})
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	buildPiPackage(t, overlayRoot, registryPath, destDir)
	builderPath := agentLinkPathIn(home, "builder.md")
	mustWrite(t, builderPath, "an agent of the user\n")

	result := newPiAdapterAt(t, overlayRoot, registryPath, destDir).Install()
	if result.Status == core.CapabilityUnsupported {
		t.Fatalf("Install must not be unsupported, got: %s", result)
	}

	assertFileUnchanged(t, builderPath, []byte("an agent of the user\n"))
	assertAgentNotLinked(t, home, "shaper.md")
	for _, file := range []string{"GADU.md", "reviewer.md"} {
		assertAgentLinked(t, home, destDir, file)
		assertMessageHasOnce(t, result.Message, file+" linked at ~/.pi/agent/agents/"+file+".")
	}
	assertMessageHasOnce(t, result.Message, "builder link: "+builderPath+" exists and is not the overlay-owned symlink; leaving it untouched.")
	assertMessageHasOnce(t, result.Message, "shaper frontmatter: missing required 'description'.")
	for _, file := range []string{"builder.md", "shaper.md"} {
		if strings.Contains(result.Message, file+" linked at") {
			t.Errorf("Install tells %s linked, and it is not:\n%s", file, result.Message)
		}
	}
}

// Status names each agent of the package it cannot prove linked, in the line it always used for
// GADU.md, and says nothing of the ones it can; with every agent linked it is supported.
func TestPiAdapter_StatusNamesEachAgentOfThePackageThatIsNotLinked(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	overlayRoot, registryPath := piFixtureOverlayWithAgents(t, map[string]string{
		"builder.md":  piRoleAgentSource("builder"),
		"reviewer.md": piRoleAgentSource("reviewer"),
	})
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	buildPiPackage(t, overlayRoot, registryPath, destDir)
	writePiSettingsPackages(t, home, []string{destDir, "npm:pi-subagents-j0k3r"})
	writePiMcpRegistration(t, destDir, true)
	adapter := newPiAdapterAt(t, overlayRoot, registryPath, destDir)

	// GADU.md is linked, builder.md is a file of the user, reviewer.md is not there.
	writeAgentLinkCurrent(t, home, destDir, "GADU.md")
	mustWrite(t, agentLinkPathIn(home, "builder.md"), "an agent of the user\n")
	result := adapter.Status()
	if result.Status != core.CapabilityPartial {
		t.Fatalf("Status = %s, want partial", result)
	}
	assertMessageHasOnce(t, result.Message, "builder.md linked at ~/.pi/agent/agents/builder.md (conflict: a foreign file already exists there)")
	assertMessageHasOnce(t, result.Message, "reviewer.md linked at ~/.pi/agent/agents/reviewer.md (missing; run: labdrian-overlay apply --target pi)")
	if strings.Contains(result.Message, "GADU.md linked at") {
		t.Errorf("Status names GADU.md, which is linked:\n%s", result.Message)
	}

	// Every agent linked.
	if err := os.Remove(agentLinkPathIn(home, "builder.md")); err != nil {
		t.Fatal(err)
	}
	writeAgentLinkCurrent(t, home, destDir, "builder.md")
	writeAgentLinkCurrent(t, home, destDir, "reviewer.md")
	result = adapter.Status()
	if result.Status != core.CapabilitySupported {
		t.Fatalf("Status with every agent linked = %s, want supported", result)
	}
	if !strings.Contains(result.Message, "the agents of the package are linked") {
		t.Errorf("Status must say the agents of the package are linked, got %q", result.Message)
	}
}

// Without a home to look in, Status cannot prove any link and says so of each agent of the package.
func TestPiAdapter_StatusWithoutAHomeNamesEveryAgent(t *testing.T) {
	overlayRoot, registryPath := piFixtureOverlayWithAgents(t, map[string]string{
		"builder.md": piRoleAgentSource("builder"),
	})
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	buildPiPackage(t, overlayRoot, registryPath, destDir)

	result := newPiAdapterWithoutAHome(overlayRoot, registryPath, destDir).Status()
	if result.Status != core.CapabilityPartial {
		t.Fatalf("Status = %s, want partial", result)
	}
	for _, file := range []string{"GADU.md", "builder.md"} {
		assertMessageHasOnce(t, result.Message, file+" linked at ~/.pi/agent/agents/"+file+" (cannot resolve home directory)")
	}
}

// Uninstall removes the link of every agent that is the overlay's and nothing else: the file of the
// user at the place of one, and any other file of the directory, stay as they are and the file of
// the user is told.
func TestPiAdapter_UninstallRemovesEveryOwnedAgentLinkAndOnlyThose(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	overlayRoot, registryPath := piFixtureOverlayWithAgents(t, map[string]string{
		"builder.md":  piRoleAgentSource("builder"),
		"reviewer.md": piRoleAgentSource("reviewer"),
	})
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	buildPiPackage(t, overlayRoot, registryPath, destDir)
	adapter := newPiAdapterAt(t, overlayRoot, registryPath, destDir)
	if result := adapter.Install(); result.Status == core.CapabilityUnsupported {
		t.Fatalf("Install must not be unsupported, got: %s", result)
	}
	// The user replaced the link of builder.md by an agent of their own, and keeps another one.
	builderPath := agentLinkPathIn(home, "builder.md")
	if err := os.Remove(builderPath); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, builderPath, "an agent of the user\n")
	otherPath := agentLinkPathIn(home, "some-managed-agent.md")
	mustWrite(t, otherPath, "gentle-pi's own agent\n")

	result := adapter.Uninstall()
	if result.Status != core.CapabilitySupported {
		t.Fatalf("Uninstall = %s, want supported", result)
	}

	for _, file := range []string{"GADU.md", "reviewer.md"} {
		assertAgentNotLinked(t, home, file)
		assertMessageHasOnce(t, result.Message, file+" link removed.")
	}
	assertFileUnchanged(t, builderPath, []byte("an agent of the user\n"))
	assertFileUnchanged(t, otherPath, []byte("gentle-pi's own agent\n"))
	assertMessageHasOnce(t, result.Message, "builder link: "+builderPath+" is not the overlay-owned symlink; leaving it untouched.")
}

// Uninstall finds the overlay's links in the home, not only in the package: when the package's
// agents/ directory is gone (or a rebuild dropped an agent), every link that still points into it is
// the overlay's and is removed, as the fixed-name GADU.md link always was.
func TestPiAdapter_UninstallRemovesOwnedLinksWhenThePackageAgentsAreGone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	overlayRoot, registryPath := piFixtureOverlayWithAgents(t, map[string]string{
		"builder.md":  piRoleAgentSource("builder"),
		"reviewer.md": piRoleAgentSource("reviewer"),
	})
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	buildPiPackage(t, overlayRoot, registryPath, destDir)
	adapter := newPiAdapterAt(t, overlayRoot, registryPath, destDir)
	if result := adapter.Install(); result.Status == core.CapabilityUnsupported {
		t.Fatalf("Install must not be unsupported, got: %s", result)
	}
	if err := os.RemoveAll(filepath.Join(destDir, "agents")); err != nil {
		t.Fatal(err)
	}
	otherPath := agentLinkPathIn(home, "some-managed-agent.md")
	mustWrite(t, otherPath, "gentle-pi's own agent\n")

	result := adapter.Uninstall()
	if result.Status != core.CapabilitySupported {
		t.Fatalf("Uninstall = %s, want supported", result)
	}
	for _, file := range []string{"GADU.md", "builder.md", "reviewer.md"} {
		assertAgentNotLinked(t, home, file)
		assertMessageHasOnce(t, result.Message, file+" link removed.")
	}
	assertFileUnchanged(t, otherPath, []byte("gentle-pi's own agent\n"))
	if strings.Contains(result.Message, "cannot be read") {
		t.Errorf("Uninstall must not fail on the missing package agents: %s", result.Message)
	}
}

// The agents this overlay really ships pass what Install checks of them: every pi/agents file of
// the repository, GADU and the roles, becomes a link, and none is refused for its frontmatter.
func TestPiAdapter_InstallLinksTheAgentsOfThisOverlay(t *testing.T) {
	overlayRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(overlayRoot, "pi", "agents"))
	if err != nil {
		t.Fatalf("read the pi/agents of the overlay: %v", err)
	}
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			files = append(files, entry.Name())
		}
	}
	if len(files) < 2 {
		t.Fatalf("the overlay ships %v for Pi, want GADU.md and its role agents", files)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	registryPath := filepath.Join(overlayRoot, "skills.registry.yaml")
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	buildPiPackage(t, overlayRoot, registryPath, destDir)

	result := newPiAdapterAt(t, overlayRoot, registryPath, destDir).Install()
	if result.Status == core.CapabilityUnsupported {
		t.Fatalf("Install must not be unsupported, got: %s", result)
	}
	for _, file := range files {
		assertAgentLinked(t, home, destDir, file)
	}
	if strings.Contains(result.Message, "frontmatter") {
		t.Errorf("Install refused an agent of the overlay: %s", result.Message)
	}
}
