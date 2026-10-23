package runtime_test

// The Pi adapter links every agent of the package (slice 4 of the role agents made it every agent
// and not GADU alone; slice 5 retired GADU): the role agents engine/agentgen writes for Pi reach
// ~/.pi/agent/agents through overlay-owned symlinks, proven by Readlink equality, and Install,
// Status and Uninstall treat each of them alike. The tests below drive the adapter through its
// public methods in a home of their own, with a fake CommandRunner: none of them reaches a live Pi.

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
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
// package holds the fixture agent (sweeper.md) of piFixtureOverlay besides them.
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

// Install links every agent file of the package, the fixture agent and the role agents alike, each to its own
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
	for _, file := range []string{fixtureAgent, "builder.md", "reviewer.md"} {
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
	for _, file := range []string{fixtureAgent, "reviewer.md"} {
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

// Status names each agent of the package it cannot prove linked, in one line of the agent's own name,
// and says nothing of the ones it can; with every agent linked it is supported.
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

	// sweeper.md is linked, builder.md is a file of the user, reviewer.md is not there.
	writeAgentLinkCurrent(t, home, destDir, fixtureAgent)
	mustWrite(t, agentLinkPathIn(home, "builder.md"), "an agent of the user\n")
	result := adapter.Status()
	if result.Status != core.CapabilityPartial {
		t.Fatalf("Status = %s, want partial", result)
	}
	assertMessageHasOnce(t, result.Message, "builder.md linked at ~/.pi/agent/agents/builder.md (conflict: a foreign file already exists there)")
	assertMessageHasOnce(t, result.Message, "reviewer.md linked at ~/.pi/agent/agents/reviewer.md (missing; run: labdrian-overlay apply --target pi)")
	if strings.Contains(result.Message, fixtureAgent+" linked at") {
		t.Errorf("Status names %s, which is linked:\n%s", fixtureAgent, result.Message)
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
	for _, file := range []string{fixtureAgent, "builder.md"} {
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

	for _, file := range []string{fixtureAgent, "reviewer.md"} {
		assertAgentNotLinked(t, home, file)
		assertMessageHasOnce(t, result.Message, file+" link removed.")
	}
	assertFileUnchanged(t, builderPath, []byte("an agent of the user\n"))
	assertFileUnchanged(t, otherPath, []byte("gentle-pi's own agent\n"))
	assertMessageHasOnce(t, result.Message, "builder link: "+builderPath+" is not the overlay-owned symlink; leaving it untouched.")
}

// Uninstall finds the overlay's links in the home, not only in the package: when the package's
// agents/ directory is gone (or a rebuild dropped an agent), every link that still points into it is
// the overlay's and is removed.
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
	for _, file := range []string{fixtureAgent, "builder.md", "reviewer.md"} {
		assertAgentNotLinked(t, home, file)
		assertMessageHasOnce(t, result.Message, file+" link removed.")
	}
	assertFileUnchanged(t, otherPath, []byte("gentle-pi's own agent\n"))
	if strings.Contains(result.Message, "cannot be read") {
		t.Errorf("Uninstall must not fail on the missing package agents: %s", result.Message)
	}
}

// A rebuild that dropped one agent leaves its link pointing into the package: Uninstall removes it
// with the links of the agents the package still has.
func TestPiAdapter_UninstallRemovesTheLinkOfAnAgentARebuildDropped(t *testing.T) {
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
	if err := os.Remove(filepath.Join(destDir, "agents", "reviewer.md")); err != nil {
		t.Fatal(err)
	}

	result := adapter.Uninstall()
	for _, file := range []string{fixtureAgent, "builder.md", "reviewer.md"} {
		assertAgentNotLinked(t, home, file)
		assertMessageHasOnce(t, result.Message, file+" link removed.")
	}
}

// A package agents directory that cannot be read for another reason than its absence is told, even
// though the links of the home are still found and removed.
func TestPiAdapter_UninstallTellsAnUnreadablePackageAgentsDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a directory whatever its mode")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	overlayRoot, registryPath := piFixtureOverlayWithAgents(t, map[string]string{
		"builder.md": piRoleAgentSource("builder"),
	})
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	buildPiPackage(t, overlayRoot, registryPath, destDir)
	adapter := newPiAdapterAt(t, overlayRoot, registryPath, destDir)
	if result := adapter.Install(); result.Status == core.CapabilityUnsupported {
		t.Fatalf("Install must not be unsupported, got: %s", result)
	}
	agentsDir := filepath.Join(destDir, "agents")
	if err := os.Chmod(agentsDir, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(agentsDir, 0755) })

	result := adapter.Uninstall()
	for _, file := range []string{fixtureAgent, "builder.md"} {
		assertAgentNotLinked(t, home, file)
		assertMessageHasOnce(t, result.Message, file+" link removed.")
	}
	assertMessageHasOnce(t, result.Message, "agents link: the package agents cannot be read at "+agentsDir)
}

// Slice 5 of the role agents stopped shipping GADU, and every earlier deploy had linked its
// pi/agents/GADU.md at ~/.pi/agent/agents/GADU.md. A link of the home that reads back as the
// package's own agents/<file> is the overlay's (the proof every other removal here uses), so
// Install removes it once the package it built has no such file, and says so.
func TestPiAdapter_InstallRemovesTheLinkOfAnAgentThePackageNoLongerShips(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	overlayRoot, registryPath := piFixtureOverlayWithAgents(t, map[string]string{
		"builder.md": piRoleAgentSource("builder"),
	})
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	buildPiPackage(t, overlayRoot, registryPath, destDir)
	// The deploy before this one linked GADU.md; the package built now has no such file.
	writeAgentLinkCurrent(t, home, destDir, "GADU.md")

	result := newPiAdapterAt(t, overlayRoot, registryPath, destDir).Install()
	if result.Status == core.CapabilityUnsupported {
		t.Fatalf("Install must not be unsupported, got: %s", result)
	}

	assertAgentNotLinked(t, home, "GADU.md")
	assertMessageHasOnce(t, result.Message, "GADU.md link removed (the package no longer ships it).")
	for _, file := range []string{fixtureAgent, "builder.md"} {
		assertAgentLinked(t, home, destDir, file)
	}
}

// The same holds for an agent the overlay drops between two Installs: the second one builds the
// package without it, and the link the first one made does not outlive the file.
func TestPiAdapter_InstallRemovesTheLinkOfAnAgentTheOverlayDropped(t *testing.T) {
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
	assertAgentLinked(t, home, destDir, "reviewer.md")

	if err := os.Remove(filepath.Join(overlayRoot, "pi", "agents", "reviewer.md")); err != nil {
		t.Fatal(err)
	}
	result := adapter.Install()

	if _, err := os.Stat(filepath.Join(destDir, "agents", "reviewer.md")); !os.IsNotExist(err) {
		t.Fatalf("the rebuilt package must not hold reviewer.md, Stat err = %v", err)
	}
	assertAgentNotLinked(t, home, "reviewer.md")
	assertMessageHasOnce(t, result.Message, "reviewer.md link removed (the package no longer ships it).")
	for _, file := range []string{fixtureAgent, "builder.md"} {
		assertAgentLinked(t, home, destDir, file)
	}
}

// Install removes only what it can prove is its own. At the places of the agents it retired, and
// beside them, the home holds what is not the overlay's -- a file of the user, a file or a link of
// gentle-pi, a link into another package, a link that is no agent file -- and none of it is
// removed, rewritten or told as removed.
func TestPiAdapter_InstallLeavesWhatIsNotTheOverlaysLinkAlone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	overlayRoot, registryPath := piFixtureOverlayWithAgents(t, map[string]string{
		"builder.md": piRoleAgentSource("builder"),
	})
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	buildPiPackage(t, overlayRoot, registryPath, destDir)

	gentlePiAgent := filepath.Join(t.TempDir(), "gentle-pi", "agents", "jd-judge-a.md")
	mustWrite(t, gentlePiAgent, "gentle-pi's own agent\n")
	otherPackage := filepath.Join(t.TempDir(), "another-package", "agents", "retired.md")
	mustWrite(t, otherPackage, "an agent of another package\n")
	regular := map[string]string{
		"GADU.md":                "a GADU.md of the user\n",
		"gentle-ai-reviewer.md":  "gentle-ai's own agent\n",
		"notes.txt":              "notes of the user\n",
		"review-requirements.md": "gentle-ai's own review agent\n",
		"jd-judge-b.md":          "gentle-ai's own judge\n",
		"sdd-apply.md":           "a file of the user at the place of a retired agent\n",
	}
	for file, content := range regular {
		mustWrite(t, agentLinkPathIn(home, file), content)
	}
	foreignLinks := map[string]string{
		"jd-judge-a.md": gentlePiAgent,
		"retired.md":    otherPackage,
	}
	for file, target := range foreignLinks {
		if err := os.Symlink(target, agentLinkPathIn(home, file)); err != nil {
			t.Fatal(err)
		}
	}
	// A link into the package that is no agent file: the agents of a package are its .md files.
	legacyTarget := filepath.Join(destDir, "agents", "legacy.txt")
	if err := os.Symlink(legacyTarget, agentLinkPathIn(home, "legacy.txt")); err != nil {
		t.Fatal(err)
	}

	result := newPiAdapterAt(t, overlayRoot, registryPath, destDir).Install()
	if result.Status == core.CapabilityUnsupported {
		t.Fatalf("Install must not be unsupported, got: %s", result)
	}

	for file, content := range regular {
		assertFileUnchanged(t, agentLinkPathIn(home, file), []byte(content))
	}
	for file, target := range foreignLinks {
		got, err := os.Readlink(agentLinkPathIn(home, file))
		if err != nil || got != target {
			t.Errorf("%s was linked to %q and is now %q (err %v)", file, target, got, err)
		}
	}
	if got, err := os.Readlink(agentLinkPathIn(home, "legacy.txt")); err != nil || got != legacyTarget {
		t.Errorf("legacy.txt is no agent file and must stay, readlink = %q (err %v)", got, err)
	}
	if strings.Contains(result.Message, "link removed") {
		t.Errorf("Install tells a link removed, and none was the overlay's:\n%s", result.Message)
	}
	for _, file := range []string{fixtureAgent, "builder.md"} {
		assertAgentLinked(t, home, destDir, file)
	}
}

// Retired means gone from the package, not unlinked this time: an agent the package still ships
// keeps its link when Install refuses its frontmatter, and the link of an agent the package no
// longer has goes whatever became of the others.
func TestPiAdapter_InstallRetiresOnlyTheAgentsThePackageNoLongerHas(t *testing.T) {
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
	// reviewer.md now has a frontmatter Pi would drop at dispatch; GADU.md is a retired agent.
	mustWrite(t, filepath.Join(overlayRoot, "pi", "agents", "reviewer.md"), "---\nname: reviewer\n---\n\nbody\n")
	writeAgentLinkCurrent(t, home, destDir, "GADU.md")

	result := adapter.Install()

	assertAgentNotLinked(t, home, "GADU.md")
	assertMessageHasOnce(t, result.Message, "GADU.md link removed (the package no longer ships it).")
	assertMessageHasOnce(t, result.Message, "reviewer frontmatter: missing required 'description'.")
	if strings.Contains(result.Message, "reviewer.md link removed") {
		t.Errorf("Install retires reviewer.md, which the package still ships:\n%s", result.Message)
	}
	// The earlier link of reviewer.md is the package's file as it was built, and the rebuild keeps
	// the shipped name: it stays in place for the next Install to refresh.
	if _, err := os.Lstat(agentLinkPathIn(home, "reviewer.md")); err != nil {
		t.Errorf("reviewer.md is shipped and must keep its link: %v", err)
	}
	assertAgentLinked(t, home, destDir, "builder.md")
}

// A link Install cannot remove is told, and does not turn the Install into a failure: the package
// is installed, only the stale link stays.
func TestPiAdapter_InstallTellsALinkItCannotRetire(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root removes a file whatever the mode of its directory")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	overlayRoot, registryPath := piFixtureOverlayWithAgents(t, map[string]string{
		"builder.md": piRoleAgentSource("builder"),
	})
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	buildPiPackage(t, overlayRoot, registryPath, destDir)
	adapter := newPiAdapterAt(t, overlayRoot, registryPath, destDir)
	if result := adapter.Install(); result.Status == core.CapabilityUnsupported {
		t.Fatalf("Install must not be unsupported, got: %s", result)
	}
	writeAgentLinkCurrent(t, home, destDir, "GADU.md")
	agentsHome := filepath.Dir(agentLinkPathIn(home, "GADU.md"))
	if err := os.Chmod(agentsHome, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(agentsHome, 0o755) })

	result := adapter.Install()

	if result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install = %s, want the status of an Install that went through (restart required)", result)
	}
	if !strings.Contains(result.Message, "gadu link: removing the link of an agent the package no longer ships") {
		t.Errorf("Install must tell the link it could not remove, got:\n%s", result.Message)
	}
	if _, err := os.Readlink(agentLinkPathIn(home, "GADU.md")); err != nil {
		t.Errorf("the link that could not be removed must still be there: %v", err)
	}
}

// droppedAgent is the agent the overlay of worldWithADroppedAgent drops: the GADU.md that every deploy
// before slice 5 of the role agents linked.
const droppedAgent = "GADU.md"

// worldWithADroppedAgent is the machine that deployed before the overlay dropped an agent: Install ran
// against an overlay whose pi/agents had a GADU.md, so ~/.pi/agent/agents/GADU.md is the overlay's
// link to the package's own file; then the overlay dropped that file, so the next build of the
// package does not have it. Beside the link the home holds what is not the overlay's -- a file of
// gentle-ai and a link into gentle-pi's own package -- which no step may touch. It returns the home,
// the overlay and the package.
func worldWithADroppedAgent(t *testing.T) (home, overlayRoot, registryPath, destDir string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	overlayRoot, registryPath = piFixtureOverlayWithAgents(t, map[string]string{
		"builder.md": piRoleAgentSource("builder"),
		droppedAgent: piRoleAgentSource("GADU"),
	})
	destDir = filepath.Join(t.TempDir(), "labdrian-pi")
	buildPiPackage(t, overlayRoot, registryPath, destDir)
	if result := newPiAdapterAt(t, overlayRoot, registryPath, destDir).Install(); result.Status == core.CapabilityUnsupported {
		t.Fatalf("Install must not be unsupported, got: %s", result)
	}
	assertAgentLinked(t, home, destDir, droppedAgent)

	mustWrite(t, agentLinkPathIn(home, "review-risk.md"), "an agent of gentle-ai\n")
	gentlePiAgent := filepath.Join(t.TempDir(), "gentle-pi", "agents", "jd-judge-a.md")
	mustWrite(t, gentlePiAgent, "gentle-pi's own agent\n")
	if err := os.Symlink(gentlePiAgent, agentLinkPathIn(home, "jd-judge-a.md")); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(filepath.Join(overlayRoot, "pi", "agents", droppedAgent)); err != nil {
		t.Fatal(err)
	}
	return home, overlayRoot, registryPath, destDir
}

// assertTheHomeKeepsWhatIsNotTheOverlays fails the test when the file of gentle-ai or the link of
// gentle-pi that worldWithADroppedAgent put in the home is not as it was.
func assertTheHomeKeepsWhatIsNotTheOverlays(t *testing.T, home string) {
	t.Helper()
	assertFileUnchanged(t, agentLinkPathIn(home, "review-risk.md"), []byte("an agent of gentle-ai\n"))
	target, err := os.Readlink(agentLinkPathIn(home, "jd-judge-a.md"))
	if err != nil || !strings.HasSuffix(target, filepath.Join("gentle-pi", "agents", "jd-judge-a.md")) {
		t.Errorf("the link of gentle-pi is not as it was: readlink = %q (err %v)", target, err)
	}
}

// A rebuild of the package is what leaves the link of a dropped agent pointing at nothing, and it
// happens before Install knows whether `pi install` will run. So the link goes as soon as the package
// is built, and does not wait for a `pi` that is not on the PATH: the package is rebuilt without
// GADU.md, the hint to run `pi install` stays the message, and the link is gone, so no overlay-owned
// link points at a file the package no longer ships. What is not the overlay's is as it was.
func TestPiAdapter_InstallRetiresTheLinkOfADroppedAgentWhenPiIsNotOnPath(t *testing.T) {
	home, overlayRoot, registryPath, destDir := worldWithADroppedAgent(t)
	commands := &fakeCommands{missing: true}

	result := newPiAdapterWith(t, commands, engineRuntime.PiOptions{}, overlayRoot, registryPath, destDir).Install()

	if result.Status != core.CapabilityPartial {
		t.Fatalf("Install without pi = %s, want partial", result)
	}
	if want := "run: pi install " + destDir; !strings.Contains(result.Message, want) {
		t.Errorf("message = %q, want it to keep the hint %q", result.Message, want)
	}
	if _, err := os.Stat(filepath.Join(destDir, "agents", droppedAgent)); !os.IsNotExist(err) {
		t.Fatalf("the rebuilt package must not hold %s, Stat err = %v", droppedAgent, err)
	}
	assertAgentNotLinked(t, home, droppedAgent)
	assertMessageHasOnce(t, result.Message, "GADU.md link removed (the package no longer ships it).")
	assertTheHomeKeepsWhatIsNotTheOverlays(t, home)
	if len(commands.calls) != 0 {
		t.Errorf("a command ran although pi was not found: %v", commands.invocations())
	}
}

// The same holds when `pi install` fails: the package is rebuilt all the same, so the link of the
// dropped agent goes, and the failure of the install is still what the message tells.
func TestPiAdapter_InstallRetiresTheLinkOfADroppedAgentWhenPiInstallFails(t *testing.T) {
	home, overlayRoot, registryPath, destDir := worldWithADroppedAgent(t)
	commands := &fakeCommands{fail: func(args []string) error { return errors.New("exit status 1") }}

	result := newPiAdapterWith(t, commands, engineRuntime.PiOptions{}, overlayRoot, registryPath, destDir).Install()

	if result.Status != core.CapabilityPartial {
		t.Fatalf("Install with a failing pi = %s, want partial", result)
	}
	if want := "but `pi install` failed: exit status 1"; !strings.Contains(result.Message, want) {
		t.Errorf("message = %q, want it to tell %q", result.Message, want)
	}
	assertAgentNotLinked(t, home, droppedAgent)
	assertMessageHasOnce(t, result.Message, "GADU.md link removed (the package no longer ships it).")
	assertTheHomeKeepsWhatIsNotTheOverlays(t, home)
	if len(commands.calls) != 1 {
		t.Errorf("Install went on after the package install failed: %v", commands.invocations())
	}
}

// Update and Rollback rebuild the package too, and say to run `pi install` after: the link of a
// dropped agent does not outlive the file there either.
func TestPiAdapter_UpdateAndRollbackRetireTheLinkOfADroppedAgent(t *testing.T) {
	for name, step := range map[string]func(engineRuntime.PiAdapter) core.LifecycleResult{
		"Update":   engineRuntime.PiAdapter.Update,
		"Rollback": engineRuntime.PiAdapter.Rollback,
	} {
		t.Run(name, func(t *testing.T) {
			home, overlayRoot, registryPath, destDir := worldWithADroppedAgent(t)
			commands := &fakeCommands{}

			result := step(newPiAdapterWith(t, commands, engineRuntime.PiOptions{}, overlayRoot, registryPath, destDir))

			if result.Status != core.CapabilityPartial {
				t.Fatalf("%s = %s, want partial", name, result)
			}
			if want := "run: pi install " + destDir; !strings.Contains(result.Message, want) {
				t.Errorf("message = %q, want it to keep the hint %q", result.Message, want)
			}
			assertAgentNotLinked(t, home, droppedAgent)
			assertMessageHasOnce(t, result.Message, "GADU.md link removed (the package no longer ships it).")
			assertTheHomeKeepsWhatIsNotTheOverlays(t, home)
			if len(commands.calls) != 0 || len(commands.lookups) != 0 {
				t.Errorf("%s reached the CLI: lookups=%v calls=%v", name, commands.lookups, commands.calls)
			}
		})
	}
}

// Status proves the links the way it always did, and also names an overlay-owned link that no agent of
// the package backs any more -- the one a removal that failed, or a build by a program that did not
// retire it, leaves behind -- so a link that points at nothing is never reported as fine. A file of
// the user and a link of gentle-pi are not the overlay's and are not named.
func TestPiAdapter_StatusNamesTheOwnedLinkOfAnAgentThePackageNoLongerShips(t *testing.T) {
	home, overlayRoot, registryPath, destDir := worldWithADroppedAgent(t)
	buildPiPackage(t, overlayRoot, registryPath, destDir) // the package built without GADU.md, nothing else touched
	writePiSettingsPackages(t, home, []string{destDir, "npm:pi-subagents-j0k3r"})
	writePiMcpRegistration(t, destDir, true)
	adapter := newPiAdapterAt(t, overlayRoot, registryPath, destDir)
	assertAgentLinked(t, home, destDir, "builder.md")
	assertAgentLinked(t, home, destDir, fixtureAgent)

	result := adapter.Status()
	if result.Status != core.CapabilityPartial {
		t.Fatalf("Status with a link no agent of the package backs = %s, want partial", result)
	}
	assertMessageHasOnce(t, result.Message, "GADU.md linked at ~/.pi/agent/agents/GADU.md (stale: the package no longer ships it; run: labdrian-overlay apply --target pi)")
	for _, notTheOverlays := range []string{"review-risk.md linked at", "jd-judge-a.md linked at"} {
		if strings.Contains(result.Message, notTheOverlays) {
			t.Errorf("Status names an entry that is not the overlay's (%s):\n%s", notTheOverlays, result.Message)
		}
	}

	if err := os.Remove(agentLinkPathIn(home, droppedAgent)); err != nil {
		t.Fatal(err)
	}
	if result := adapter.Status(); result.Status != core.CapabilitySupported {
		t.Fatalf("Status once the link is gone = %s, want supported", result)
	}
}

// A home whose agent links cannot be listed leaves Status unable to tell whether a link of the
// overlay outlives the file it points at, and it says so rather than reporting the agents as linked.
func TestPiAdapter_StatusTellsAnAgentLinksDirectoryItCannotList(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root lists a directory whatever its mode")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	overlayRoot, registryPath := piFixtureOverlayWithAgents(t, map[string]string{
		"builder.md": piRoleAgentSource("builder"),
	})
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")
	buildPiPackage(t, overlayRoot, registryPath, destDir)
	writePiSettingsPackages(t, home, []string{destDir, "npm:pi-subagents-j0k3r"})
	writePiMcpRegistration(t, destDir, true)
	for _, file := range []string{fixtureAgent, "builder.md"} {
		writeAgentLinkCurrent(t, home, destDir, file)
	}
	adapter := newPiAdapterAt(t, overlayRoot, registryPath, destDir)
	if result := adapter.Status(); result.Status != core.CapabilitySupported {
		t.Fatalf("Status with every agent linked = %s, want supported", result)
	}
	agentsHome := filepath.Dir(agentLinkPathIn(home, "builder.md"))
	// Searchable but not readable: each link can still be proven by its name, none can be listed.
	if err := os.Chmod(agentsHome, 0o311); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(agentsHome, 0o755) })

	result := adapter.Status()

	if result.Status != core.CapabilityPartial {
		t.Fatalf("Status with links it cannot list = %s, want partial", result)
	}
	assertMessageHasOnce(t, result.Message, "agents linked at ~/.pi/agent/agents (the links cannot be listed: ")
}

// The agents this overlay really ships pass what Install checks of them: every pi/agents file of
// the repository, the role agents, becomes a link, and none is refused for its frontmatter.
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
	for _, role := range []string{"builder.md", "reviewer.md"} {
		if !slices.Contains(files, role) {
			t.Fatalf("the overlay ships %v for Pi, want its role agents (%s is missing)", files, role)
		}
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
