package shelltest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/enginetest"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// pipkgWorld is the world the pipkg_* helpers of the script run in: a home that has the engine, a state
// directory in it, and runHelper, which sources the script and runs one of its helpers there from the
// root of the checkout. runHelper returns the combined output alongside the error the helper ended
// with. Callers assert on both explicitly -- these helpers now return honest non-zero exits for "not
// built" and "drift" (R4-silent-package-skip), so a blanket t.Fatalf on any non-zero exit would defeat
// the very thing the tests prove.
func pipkgWorld(t *testing.T) (home, stateDir string, runHelper func(script string) (string, error)) {
	t.Helper()
	overlay := overlayScript(t)
	overlayDir := overlaytest.RepoRoot(t)

	home = t.TempDir()
	enginetest.Provision(t, home)

	stateDir = filepath.Join(home, "state")
	runHelper = func(script string) (string, error) {
		t.Helper()
		env := append(os.Environ(), "HOME="+home, "STATE_DIR="+stateDir, "OVERLAY_DIR="+overlayDir,
			// these tests build directly from the checkout (no apply, so no main checkout); pin the deploy ref to HEAD
			"LABDRIAN_PI_DEPLOY_REF=HEAD")
		return overlaytest.Source(t, overlay, overlaytest.Shell{Env: env, Dir: overlayDir}, script)
	}
	return home, stateDir, runHelper
}

// TestPipkgHelpers_BuildStatusSyncCheck (R-002, R-003, R-010, pi-package-
// build slice 2.5): exercises the real pipkg_build_and_report /
// pipkg_status_and_report / pipkg_sync_check_and_report bash helpers
// end-to-end against a real built engine binary, without running cmd_apply
// itself (which mutates git state) -- these are the same functions
// cmd_apply/cmd_status/cmd_sync_check dispatch to for --target pi.
func TestPipkgHelpers_BuildStatusSyncCheck(t *testing.T) {
	_, stateDir, runHelper := pipkgWorld(t)

	// Before any build: status honestly reports "not built" and exits
	// non-zero; sync-check emits VERDICT:pi:NOT_BUILT and exits non-zero
	// too -- neither state is healthy, so neither may exit 0
	// (R4-silent-package-skip). The R-007 --no-extensions/--no-skills
	// disclosure is always present in status, independent of build state.
	statusBefore, statusBeforeErr := runHelper("pipkg_status_and_report")
	if statusBeforeErr == nil {
		t.Fatalf("status before build should exit non-zero (not built), got success: %s", statusBefore)
	}
	if !strings.Contains(statusBefore, "not built") {
		t.Fatalf("status before build should report 'not built', got: %s", statusBefore)
	}
	if !strings.Contains(statusBefore, "--no-extensions") || !strings.Contains(statusBefore, "--no-skills") {
		t.Fatalf("status should always disclose --no-extensions/--no-skills, got: %s", statusBefore)
	}
	if !strings.Contains(statusBefore, "(short aliases: -ne and -ns)") {
		t.Fatalf("disclosure must name the -ne/-ns aliases pi --help documents, got: %s", statusBefore)
	}
	syncBefore, syncBeforeErr := runHelper("pipkg_sync_check_and_report")
	if syncBeforeErr == nil {
		t.Fatalf("sync-check before build should exit non-zero (not built), got success: %s", syncBefore)
	}
	if !strings.Contains(syncBefore, "VERDICT:pi:NOT_BUILT") {
		t.Fatalf("sync-check before build should emit VERDICT:pi:NOT_BUILT, got: %s", syncBefore)
	}

	// Build: package.json must exist afterward, and the install hint is
	// printed; a successful build exits 0.
	buildOut, buildErr := runHelper("pipkg_build_and_report")
	if buildErr != nil {
		t.Fatalf("build should succeed: %v\n%s", buildErr, buildOut)
	}
	if !strings.Contains(buildOut, "install hint: pi install") {
		t.Fatalf("build should print the install hint, got: %s", buildOut)
	}
	destPkg := filepath.Join(stateDir, "pi", "labdrian-pi", "package.json")
	if _, statErr := os.Stat(destPkg); statErr != nil {
		t.Fatalf("expected package.json to be built at %s: %v", destPkg, statErr)
	}

	// Right after build (W-01): pipkg_status_and_report now delegates to
	// `engine runtime status --target pi`, which proves three owned
	// entries (built+in-sync, listed in ~/.pi/agent/settings.json,
	// longterm-mem MCP-registered) -- not just the bare drift check the
	// old bash-only implementation ran. Nothing was installed or
	// registered yet, so it stays honestly "partial" and exits non-zero,
	// naming the still-missing proof.
	statusAfter, statusAfterErr := runHelper("pipkg_status_and_report")
	if statusAfterErr == nil {
		t.Fatalf("status right after build (before install/register) should exit non-zero (partial), got success: %s", statusAfter)
	}
	if !strings.Contains(statusAfter, "partial") {
		t.Fatalf("status right after build should report partial, got: %s", statusAfter)
	}
	if !strings.Contains(statusAfter, "--no-extensions") || !strings.Contains(statusAfter, "--no-skills") {
		t.Fatalf("status right after build should still disclose --no-extensions/--no-skills (via the delegated adapter message), got: %s", statusAfter)
	}
	// sync-check still reports no drift and exits 0 (unchanged -- W-01
	// scoped the delegation to status only, not sync-check).
	syncAfter, syncAfterErr := runHelper("pipkg_sync_check_and_report")
	if syncAfterErr != nil {
		t.Fatalf("sync-check right after build should exit 0: %v\n%s", syncAfterErr, syncAfter)
	}
	if !strings.Contains(syncAfter, "SYNC_CHECK:pi: no drift") || !strings.Contains(syncAfter, "VERDICT:pi:IN_SYNC") {
		t.Fatalf("sync-check right after build should report no drift and VERDICT:pi:IN_SYNC, got: %q", syncAfter)
	}

	// Edit one built file directly: drift must now be reported (R-003),
	// with VERDICT:pi:DRIFT and a non-zero exit.
	if err := os.WriteFile(destPkg, []byte(`{"name":"tampered"}`), 0644); err != nil {
		t.Fatalf("tamper with built package.json: %v", err)
	}
	syncDrift, syncDriftErr := runHelper("pipkg_sync_check_and_report")
	if syncDriftErr == nil {
		t.Fatalf("sync-check after tampering should exit non-zero (drift), got success: %s", syncDrift)
	}
	if !strings.Contains(syncDrift, "SYNC_CHECK:pi: drift") || !strings.Contains(syncDrift, "package.json") || !strings.Contains(syncDrift, "VERDICT:pi:DRIFT") {
		t.Fatalf("sync-check after tampering must report drift naming package.json and VERDICT:pi:DRIFT, got: %s", syncDrift)
	}
	statusDrift, statusDriftErr := runHelper("pipkg_status_and_report")
	if statusDriftErr == nil {
		t.Fatalf("status after tampering should exit non-zero (drift), got success: %s", statusDrift)
	}
	if !strings.Contains(statusDrift, "package.json") {
		t.Fatalf("status after tampering should report drift detected, got: %s", statusDrift)
	}
}

// TestPipkgBuildAndReport_RetiresTheLinkOfAnAgentThePackageNoLongerShips (slice 5 of the role
// agents, S4): `apply --target pi` builds through pipkg_build_and_report before it asks whether a
// `pi` is on the PATH (pipkg_apply_and_report), so a machine without pi -- or one whose `pi install`
// then fails -- ends the run with this helper alone. The build swaps the package in whole: GADU.md,
// once the role agents replaced it, is no longer in it, and the link a deploy before made for it
// points at nothing. The helper's run removes that link and says so; the links of the agents the
// package ships and what is not the overlay's stay as they are.
func TestPipkgBuildAndReport_RetiresTheLinkOfAnAgentThePackageNoLongerShips(t *testing.T) {
	home, stateDir, runHelper := pipkgWorld(t)
	destDir := filepath.Join(stateDir, "pi", "labdrian-pi")
	agentsHome := filepath.Join(home, ".pi", "agent", "agents")

	if out, err := runHelper("pipkg_build_and_report"); err != nil {
		t.Fatalf("first build should succeed: %v\n%s", err, out)
	}
	shipped, err := os.ReadDir(filepath.Join(destDir, "agents"))
	if err != nil || len(shipped) == 0 {
		t.Fatalf("the package must ship agents to link: %v (%d)", err, len(shipped))
	}
	if _, statErr := os.Stat(filepath.Join(destDir, "agents", "GADU.md")); !os.IsNotExist(statErr) {
		t.Fatalf("this test needs a package that does not ship GADU.md: Stat err = %v", statErr)
	}
	// The deploy before slice 5 linked every agent of its package, GADU.md included.
	if err := os.MkdirAll(agentsHome, 0755); err != nil {
		t.Fatalf("mkdir the agents directory of the home: %v", err)
	}
	links := []string{"GADU.md"}
	for _, agent := range shipped {
		links = append(links, agent.Name())
	}
	for _, file := range links {
		if err := os.Symlink(filepath.Join(destDir, "agents", file), filepath.Join(agentsHome, file)); err != nil {
			t.Fatalf("symlink the link of %s: %v", file, err)
		}
	}
	foreign := filepath.Join(agentsHome, "review-risk.md")
	if err := os.WriteFile(foreign, []byte("an agent of gentle-ai\n"), 0644); err != nil {
		t.Fatalf("write the file of gentle-ai: %v", err)
	}

	out, buildErr := runHelper("pipkg_build_and_report")
	if buildErr != nil {
		t.Fatalf("build should succeed: %v\n%s", buildErr, out)
	}

	if !strings.Contains(out, "pipkg build: GADU.md link removed (the package no longer ships it).") {
		t.Errorf("build must say it removed the link of GADU.md, got: %s", out)
	}
	if !strings.Contains(out, "install hint: pi install") {
		t.Errorf("build must still print the install hint, got: %s", out)
	}
	if _, lstatErr := os.Lstat(filepath.Join(agentsHome, "GADU.md")); !os.IsNotExist(lstatErr) {
		t.Errorf("the link of GADU.md must be gone, Lstat err = %v", lstatErr)
	}
	for _, agent := range shipped {
		if _, statErr := os.Stat(filepath.Join(agentsHome, agent.Name())); statErr != nil {
			t.Errorf("the link of %s, an agent the package ships, must still resolve: %v", agent.Name(), statErr)
		}
		if strings.Contains(out, agent.Name()+" link removed") {
			t.Errorf("build says it removed the link of %s, which the package ships: %s", agent.Name(), out)
		}
	}
	if data, readErr := os.ReadFile(foreign); readErr != nil || string(data) != "an agent of gentle-ai\n" {
		t.Errorf("the file of gentle-ai must be as it was, got %q (%v)", data, readErr)
	}
}

// TestPipkgStatusAndReport_DelegatesHonestPartialThenSupported (W-01):
// pipkg_status_and_report must delegate to `engine runtime status --target
// pi` once the package is built, rather than the old bash-only "no
// drift = partial" heuristic. This proves the full honest transition with
// a freshly built engine binary: built-but-unregistered stays "partial"
// (naming the still-missing register step), and only reports "supported"
// once the package is also listed in ~/.pi/agent/settings.json and has a
// longterm-mem MCP entry in mcp.json (the same owned entries
// PiAdapter.Status() proves -- see TestPiAdapter_StatusTriangulatesAllOwnedEntries
// in engine/runtime/pi_test.go for the unit-level equivalent).
func TestPipkgStatusAndReport_DelegatesHonestPartialThenSupported(t *testing.T) {
	home, stateDir, runHelper := pipkgWorld(t)

	buildOut, buildErr := runHelper("pipkg_build_and_report")
	if buildErr != nil {
		t.Fatalf("build should succeed: %v\n%s", buildErr, buildOut)
	}
	destDir := filepath.Join(stateDir, "pi", "labdrian-pi")

	// Built but never registered: partial, naming the missing register step.
	statusPartial, statusPartialErr := runHelper("pipkg_status_and_report")
	if statusPartialErr == nil {
		t.Fatalf("status before register should exit non-zero (partial), got success: %s", statusPartial)
	}
	if !strings.Contains(statusPartial, "partial") {
		t.Fatalf("status before register should report partial, got: %s", statusPartial)
	}
	if !strings.Contains(statusPartial, "longterm-mem register --target pi") {
		t.Fatalf("status before register should name the missing register command, got: %s", statusPartial)
	}

	// Simulate `pi install <destDir>` (writes ~/.pi/agent/settings.json)
	// and `longterm-mem register --target pi` (writes destDir/mcp.json)
	// without depending on either real binary -- this test's own scope is
	// the bash delegation, already proven end-to-end at the unit level.
	settingsPath := filepath.Join(home, ".pi", "agent", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		t.Fatalf("mkdir settings.json dir: %v", err)
	}
	// Also simulate the Pi Subagents extension already being installed and
	// the links of the package's agents already existing (R-012/R-013 -- two
	// more owned entries PiAdapter.Status() now proves, see
	// TestPiAdapter_StatusTriangulatesAllOwnedEntries for the unit-level
	// equivalent).
	settingsJSON := `{"packages": ["` + destDir + `", "npm:pi-subagents-j0k3r"]}`
	if err := os.WriteFile(settingsPath, []byte(settingsJSON), 0644); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}
	mcpJSON := `{"mcpServers": {"longterm-mem": {"type": "stdio", "command": "/opt/labdrian-overlay/bin/longterm-mem", "args": ["mcp"]}}}`
	if err := os.WriteFile(filepath.Join(destDir, "mcp.json"), []byte(mcpJSON), 0644); err != nil {
		t.Fatalf("write mcp.json: %v", err)
	}
	// Every agent of the package is linked, as `apply` leaves them: the role agents.
	agents, err := os.ReadDir(filepath.Join(destDir, "agents"))
	if err != nil {
		t.Fatalf("read the agents of the package: %v", err)
	}
	if len(agents) == 0 {
		t.Fatal("the package has no agent to link")
	}
	for _, agent := range agents {
		linkPath := filepath.Join(home, ".pi", "agent", "agents", agent.Name())
		if err := os.MkdirAll(filepath.Dir(linkPath), 0755); err != nil {
			t.Fatalf("mkdir agent link dir: %v", err)
		}
		if err := os.Symlink(filepath.Join(destDir, "agents", agent.Name()), linkPath); err != nil {
			t.Fatalf("symlink the link of %s: %v", agent.Name(), err)
		}
	}

	// Now every owned entry is proven: supported.
	statusSupported, statusSupportedErr := runHelper("pipkg_status_and_report")
	if statusSupportedErr != nil {
		t.Fatalf("status after listing+registration should exit 0 (supported): %v\n%s", statusSupportedErr, statusSupported)
	}
	if !strings.Contains(statusSupported, "supported") {
		t.Fatalf("status after listing+registration should report supported, got: %s", statusSupported)
	}
}
