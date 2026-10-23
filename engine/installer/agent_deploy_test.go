package installer_test

// `apply`, `status` and `sync-check` over a sandbox overlay repository whose manifest has agent rows:
// where an agent and a skill land in each runtime's directories, that status and sync-check report
// the agent file, that an unrelated skill deploys byte for byte, and that the entry-contract bundle
// propagates and reports integrity drift. Integration tests: they skip under -short. The sandbox is
// in overlay_sandbox_test.go.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// TestApply_AgentsLandInNativeAgentDirs asserts that after overlay apply:
// - agents/GADU.md lands at $HOME/.claude/agents/GADU.md (not .claude/skills)
// - opencode/agents/GADU.md lands at $HOME/.config/opencode/agents/GADU.md
// - skills/test-skill/SKILL.md lands in all 3 skills dirs
// - no skill file is in .claude/agents except GADU.md (AC-3)
func TestApply_AgentsLandInNativeAgentDirs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	overlay := overlaytest.Script(t)
	home := t.TempDir()
	_, env := setupSandboxOverlay(t, home)

	out := runApplyAllTolerantOfPi(t, overlay, env)

	agentDest := filepath.Join(home, ".claude", "agents", "GADU.md")
	if _, err := os.Stat(agentDest); err != nil {
		t.Errorf("agent file not found at %s: %v\napply output:\n%s", agentDest, err, out)
	}
	opencodeAgentDest := filepath.Join(home, ".config", "opencode", "agents", "GADU.md")
	if _, err := os.Stat(opencodeAgentDest); err != nil {
		t.Errorf("opencode agent file not found at %s: %v\napply output:\n%s", opencodeAgentDest, err, out)
	}

	skillDests := []string{
		filepath.Join(home, ".claude", "skills", "test-skill", "SKILL.md"),
		filepath.Join(home, ".config", "opencode", "skills", "test-skill", "SKILL.md"),
		filepath.Join(home, ".codex", "skills", "test-skill", "SKILL.md"),
	}
	for _, p := range skillDests {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("skill not deployed at %s: %v", p, err)
		}
	}

	// No skill file should be in .claude/agents (only GADU.md)
	agentsDir := filepath.Join(home, ".claude", "agents")
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		t.Fatalf("readdir %s: %v", agentsDir, err)
	}
	for _, e := range entries {
		if e.Name() != "GADU.md" {
			t.Errorf("unexpected file in .claude/agents: %s", e.Name())
		}
	}
}

// TestCodexAgentsLandInTheAgentsDirectoryOfCodexAndNowhereElse asserts, for a manifest that has an
// agent of Claude Code and an agent of Codex (slice 4 of the role agents), that:
//   - apply --target codex copies codex/agents/builder.toml to $HOME/.codex/agents/builder.toml,
//     byte for byte, and puts nothing else there (the agent of Claude Code is not copied to Codex);
//   - apply --target claude copies the agent of Claude Code to $HOME/.claude/agents/ and not the one of
//     Codex;
//   - sync-check --target codex reports the deployed agent IN_SYNC, reports it OVERLAY_NOT_DEPLOYED
//     once removed, and a reapply restores it.
func TestCodexAgentsLandInTheAgentsDirectoryOfCodexAndNowhereElse(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	overlay := overlaytest.Script(t)
	home := t.TempDir()
	const codexAgent = "name = \"builder\"\ndescription = \"A test agent.\"\ndeveloper_instructions = \"\"\"\nBody.\n\"\"\"\nmodel_reasoning_effort = \"high\"\n"
	_, env := overlaytest.NewOverlay(t, home, map[string]string{
		"overlay.manifest": "test-skill/SKILL.md   managed\n" +
			"GADU.md   custom   agent\n" +
			"codex/agents/builder.toml   custom   codex-agent\n",
		"skills/test-skill/SKILL.md": "# overlay skill\n",
		"agents/GADU.md":             "---\nname: GADU\n---\n# overlay agent\n",
		"codex/agents/builder.toml":  codexAgent,
	})

	if out, err := overlaytest.Combined(t, overlay, env, "", "apply", "--target", "codex"); err != nil {
		t.Fatalf("overlay apply --target codex: %v\noutput:\n%s", err, out)
	}
	deployed := filepath.Join(home, ".codex", "agents", "builder.toml")
	got, err := os.ReadFile(deployed)
	if err != nil {
		t.Fatalf("the agent of Codex was not deployed: %v", err)
	}
	if string(got) != codexAgent {
		t.Errorf("deployed agent of Codex = %q, want the source byte for byte: %q", got, codexAgent)
	}
	assertOnlyEntries(t, filepath.Join(home, ".codex", "agents"), "builder.toml")
	if _, err := os.Stat(filepath.Join(home, ".claude", "agents")); err == nil {
		t.Error("apply --target codex wrote into the agents directory of Claude Code")
	}

	if out, err := overlaytest.Combined(t, overlay, env, "", "apply", "--target", "claude"); err != nil {
		t.Fatalf("overlay apply --target claude: %v\noutput:\n%s", err, out)
	}
	assertOnlyEntries(t, filepath.Join(home, ".claude", "agents"), "GADU.md")

	inSync, err := overlaytest.Combined(t, overlay, env, "", "sync-check", "--target", "codex")
	if err != nil {
		t.Fatalf("overlay sync-check --target codex: %v\noutput:\n%s", err, inSync)
	}
	if strings.Contains(inSync, "OVERLAY_NOT_DEPLOYED: ") || !strings.Contains(inSync, "IN_SYNC") {
		t.Errorf("sync-check of a deployed agent of Codex should say IN_SYNC and name none as not deployed\noutput:\n%s", inSync)
	}

	if err := os.Remove(deployed); err != nil {
		t.Fatal(err)
	}
	missing, _ := overlaytest.Combined(t, overlay, env, "", "sync-check", "--target", "codex")
	if !strings.Contains(missing, "OVERLAY_NOT_DEPLOYED: codex/agents/builder.toml") {
		t.Errorf("sync-check should name the removed agent of Codex as not deployed\noutput:\n%s", missing)
	}

	if out, err := overlaytest.Combined(t, overlay, env, "", "apply", "--target", "codex"); err != nil {
		t.Fatalf("overlay apply --target codex (reapply): %v\noutput:\n%s", err, out)
	}
	if restored, err := os.ReadFile(deployed); err != nil || string(restored) != codexAgent {
		t.Errorf("reapply did not restore the agent of Codex: %q, %v", restored, err)
	}
}

// TestApply_LeavesTheDeployedFileOfARowThatLeftTheOverlay pins what `apply` does with a file it
// deployed once and whose manifest row is gone, the case of the agents that slice 5 of the role
// agents retired (agents/sdd-*.md, GADU.md and the GADU of OpenCode): nothing. `apply` copies the
// rows the manifest has and has no step that removes a file, so the file stays where the earlier
// deploy put it, byte for byte, in the agents directory of Claude Code and in the one of OpenCode;
// and a file the overlay never deployed (the review-*, jd-* and gentle-ai-* agents of gentle-ai) is
// as it was. Only the Pi link of a retired agent is removed, by the Pi runtime and not by `apply`
// (engine/runtime, TestPiAdapter_InstallRemovesTheLinkOfAnAgentThePackageNoLongerShips).
//
// This is a CHARACTERIZATION test: it passed on the base, and a step of `apply` that removed a
// deployed file whose row is gone would fail it.
func TestApply_LeavesTheDeployedFileOfARowThatLeftTheOverlay(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	const (
		withTheRetired = "test-skill/SKILL.md   managed\n" +
			"builder.md   custom   agent\n" +
			"retired.md   custom   agent\n" +
			"opencode/agents/retired.md   custom   opencode-agent\n"
		withoutIt = "test-skill/SKILL.md   managed\n" +
			"builder.md   custom   agent\n"
		builder         = "---\nname: builder\n---\n# an agent that stays\n"
		retired         = "---\nname: retired\n---\n# an agent that leaves the overlay\n"
		retiredOpenCode = "---\ndescription: retired\nmode: all\n---\n# the same agent, for OpenCode\n"
		foreign         = "# an agent of gentle-ai, which the overlay never deployed\n"
	)
	overlay := overlaytest.Script(t)
	home := t.TempDir()
	dir, env := overlaytest.NewOverlay(t, home, map[string]string{
		"overlay.manifest":           withTheRetired,
		"skills/test-skill/SKILL.md": "# overlay skill\n",
		"agents/builder.md":          builder,
		"agents/retired.md":          retired,
		"opencode/agents/retired.md": retiredOpenCode,
	})
	claudeAgents := filepath.Join(home, ".claude", "agents")
	openCodeAgents := filepath.Join(home, ".config", "opencode", "agents")
	overlaytest.WriteFile(t, claudeAgents, "review-risk.md", foreign)
	overlaytest.WriteFile(t, openCodeAgents, "review-risk.md", foreign)

	apply := func(target string) {
		t.Helper()
		if out, err := overlaytest.Combined(t, overlay, env, "", "apply", "--target", target); err != nil {
			t.Fatalf("overlay apply --target %s: %v\noutput:\n%s", target, err, out)
		}
	}
	apply("claude")
	apply("opencode")
	assertOnlyEntries(t, claudeAgents, "builder.md", "retired.md", "review-risk.md")
	assertOnlyEntries(t, openCodeAgents, "retired.md", "review-risk.md")

	// The row and the file leave the overlay together, on main, as the retirement commits them.
	overlaytest.WriteFile(t, dir, "overlay.manifest", withoutIt)
	for _, gone := range []string{"agents/retired.md", "opencode/agents/retired.md"} {
		if err := os.Remove(filepath.Join(dir, gone)); err != nil {
			t.Fatalf("remove %s: %v", gone, err)
		}
	}
	overlaytest.Git(t, dir, env, "add", "-A")
	overlaytest.Git(t, dir, env, "commit", "-m", "retire the agent")

	apply("claude")
	apply("opencode")
	for path, want := range map[string]string{
		filepath.Join(claudeAgents, "retired.md"):       retired,
		filepath.Join(claudeAgents, "builder.md"):       builder,
		filepath.Join(claudeAgents, "review-risk.md"):   foreign,
		filepath.Join(openCodeAgents, "retired.md"):     retiredOpenCode,
		filepath.Join(openCodeAgents, "review-risk.md"): foreign,
	} {
		if got, err := os.ReadFile(path); err != nil || string(got) != want {
			t.Errorf("%s = %q, %v; want it as the deploy left it: %q", path, got, err, want)
		}
	}
	assertOnlyEntries(t, claudeAgents, "builder.md", "retired.md", "review-risk.md")
	assertOnlyEntries(t, openCodeAgents, "retired.md", "review-risk.md")
}

// assertOnlyEntries fails the test unless dir holds exactly the named entries.
func assertOnlyEntries(t *testing.T, dir string, want ...string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir %s: %v", dir, err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("%s holds %v, want exactly %v", dir, got, want)
	}
}

// TestStatus_ReportsAgentFile asserts that overlay status references the GADU
// agent row under agents/ not under skills/ (R-007, AC-4).
func TestStatus_ReportsAgentFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	overlay := overlaytest.Script(t)
	home := t.TempDir()
	_, env := setupSandboxOverlay(t, home)

	runApplyAllTolerantOfPi(t, overlay, env)

	out := runStatusAllTolerantOfPi(t, overlay, env)

	// GADU row must NOT be reported under a skills path
	if strings.Contains(out, ".claude/skills/GADU.md") ||
		strings.Contains(out, "skills/GADU.md") {
		t.Errorf("status incorrectly references GADU under skills path\noutput:\n%s", out)
	}
}

// TestSyncCheck_DetectsMissingAgentFile pins the L511 fix: when an agent file IS
// deployed, sync-check reports IN_SYNC (not OVERLAY_NOT_DEPLOYED). After removal
// it reports OVERLAY_NOT_DEPLOYED, then a reapply restores the file and sync-check
// reports IN_SYNC again. (R-007, AC-4, D2 L511 correctness-critical)
//
// The restoration half (below the "Remove agent file" section) is the AC-5
// regression test referenced by docs/e1-durability-probe.md ("simulated sync →
// reapply → both artifacts present + managed"): it proves cmd_apply restores a
// drifted/missing agent artifact and cmd_sync_check reports it IN_SYNC afterward.
// This is a CHARACTERIZATION test, not a RED→GREEN cycle: the restoration
// behavior already exists (bin/labdrian-overlay copies unconditionally whenever
// the destination is absent or differs from the source — see cmd_apply's deploy
// loop — consulting no "already applied" ledger), so it is expected to, and did,
// PASS on its first run.
func TestSyncCheck_DetectsMissingAgentFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	overlay := overlaytest.Script(t)
	home := t.TempDir()
	_, env := setupSandboxOverlay(t, home)

	// Deploy first
	runApplyAllTolerantOfPi(t, overlay, env)

	// Deployed agent row must report IN_SYNC — pins L511 fix
	outSync, err := overlaytest.Combined(t, overlay, env, "", "sync-check", "--target", "claude")
	if err != nil {
		t.Fatalf("overlay sync-check (after deploy): %v\noutput:\n%s", err, outSync)
	}
	// Check for per-file "OVERLAY_NOT_DEPLOYED: <path>" status lines only.
	// The VERDICT line always contains "OVERLAY_NOT_DEPLOYED=N" (with =), not ": ".
	if strings.Contains(outSync, "OVERLAY_NOT_DEPLOYED: ") {
		t.Errorf("L511 regression: sync-check reports OVERLAY_NOT_DEPLOYED for deployed agent file\noutput:\n%s", outSync)
	}
	if !strings.Contains(outSync, "IN_SYNC") {
		t.Errorf("expected IN_SYNC in sync-check output after deploy\noutput:\n%s", outSync)
	}

	// Remove agent file and re-check — must report OVERLAY_NOT_DEPLOYED
	if err := os.Remove(filepath.Join(home, ".claude", "agents", "GADU.md")); err != nil {
		t.Fatalf("remove GADU.md: %v", err)
	}

	outMissing, _ := overlaytest.Combined(t, overlay, env, "", "sync-check", "--target", "claude")
	if !strings.Contains(outMissing, "OVERLAY_NOT_DEPLOYED: ") {
		t.Errorf("sync-check should report OVERLAY_NOT_DEPLOYED after agent file removed\noutput:\n%s", outMissing)
	}

	// Restoration half (AC-5): reapply must restore the missing agent file, and
	// sync-check must report IN_SYNC again afterward.
	runApplyAllTolerantOfPi(t, overlay, env)

	agentDest := filepath.Join(home, ".claude", "agents", "GADU.md")
	if _, err := os.Stat(agentDest); err != nil {
		t.Errorf("agent file not restored at %s after reapply: %v", agentDest, err)
	}

	outRestored, err := overlaytest.Combined(t, overlay, env, "", "sync-check", "--target", "claude")
	if err != nil {
		t.Fatalf("overlay sync-check (after reapply): %v\noutput:\n%s", err, outRestored)
	}
	if strings.Contains(outRestored, "OVERLAY_NOT_DEPLOYED: ") {
		t.Errorf("agent file still reported OVERLAY_NOT_DEPLOYED after reapply\noutput:\n%s", outRestored)
	}
	if !strings.Contains(outRestored, "IN_SYNC") {
		t.Errorf("expected IN_SYNC in sync-check output after reapply\noutput:\n%s", outRestored)
	}
}

// TestUnrelatedSkillUnchanged verifies that an unrelated skill deploys byte-identically
// to all three skills destinations (D2 backward-compat invariant).
func TestUnrelatedSkillUnchanged(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	overlay := overlaytest.Script(t)
	home := t.TempDir()
	overlayDir, env := setupSandboxOverlay(t, home)

	runApplyAllTolerantOfPi(t, overlay, env)

	srcContent, err := os.ReadFile(filepath.Join(overlayDir, "skills", "test-skill", "SKILL.md"))
	if err != nil {
		t.Fatalf("read source skill: %v", err)
	}

	dests := []string{
		filepath.Join(home, ".claude", "skills", "test-skill", "SKILL.md"),
		filepath.Join(home, ".config", "opencode", "skills", "test-skill", "SKILL.md"),
		filepath.Join(home, ".codex", "skills", "test-skill", "SKILL.md"),
	}
	for _, p := range dests {
		got, err := os.ReadFile(p)
		if err != nil {
			t.Errorf("read deployed skill at %s: %v", p, err)
			continue
		}
		if string(got) != string(srcContent) {
			t.Errorf("deployed skill content differs from source at %s", p)
		}
	}
}

func TestEntryContractBundlePropagatesAndReportsIntegrityDrift(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	overlay := overlaytest.Script(t)
	home := t.TempDir()
	overlayDir, env := setupSandboxOverlay(t, home)

	runApplyAllTolerantOfPi(t, overlay, env)

	assets := []string{
		"inception-pipeline/SKILL.md",
		"_shared/pre-sdd-contracts.md",
		"_shared/entry-contract.schema.json",
		"_shared/actuals-record.schema.json",
	}
	targetRoots := []string{
		filepath.Join(home, ".claude", "skills"),
		filepath.Join(home, ".config", "opencode", "skills"),
		filepath.Join(home, ".codex", "skills"),
	}
	for _, rel := range assets {
		want, err := os.ReadFile(filepath.Join(overlayDir, "skills", rel))
		if err != nil {
			t.Fatalf("read source asset %s: %v", rel, err)
		}
		for _, targetRoot := range targetRoots {
			got, err := os.ReadFile(filepath.Join(targetRoot, rel))
			if err != nil {
				t.Errorf("read deployed asset %s under %s: %v", rel, targetRoot, err)
				continue
			}
			if !bytes.Equal(got, want) {
				t.Errorf("deployed asset %s differs under %s", rel, targetRoot)
			}
		}
	}

	missingRel := "_shared/entry-contract.schema.json"
	driftedRel := "_shared/pre-sdd-contracts.md"
	opencodeRoot := filepath.Join(home, ".config", "opencode", "skills")
	if err := os.Remove(filepath.Join(opencodeRoot, missingRel)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(opencodeRoot, driftedRel), []byte("drifted\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	statusOut, err := overlaytest.Combined(t, overlay, env, "", "status", "--target", "opencode")
	if err != nil {
		t.Fatalf("overlay status: %v\noutput:\n%s", err, statusOut)
	}
	if !strings.Contains(statusOut, "MISSING in target: "+missingRel) {
		t.Errorf("status did not report missing shared asset\noutput:\n%s", statusOut)
	}
	if !strings.Contains(statusOut, "DIFFERS  : "+driftedRel) {
		t.Errorf("status did not report drifted shared asset\noutput:\n%s", statusOut)
	}

	syncOut, err := overlaytest.Combined(t, overlay, env, "", "sync-check", "--target", "opencode")
	if err != nil {
		t.Fatalf("overlay sync-check: %v\noutput:\n%s", err, syncOut)
	}
	if !strings.Contains(syncOut, "OVERLAY_NOT_DEPLOYED: skills/"+missingRel) {
		t.Errorf("sync-check did not report %s as not deployed\noutput:\n%s", missingRel, syncOut)
	}
	if !strings.Contains(syncOut, "UPSTREAM_CHANGED: skills/"+driftedRel) {
		t.Errorf("sync-check did not report %s as drifted\noutput:\n%s", driftedRel, syncOut)
	}
}
