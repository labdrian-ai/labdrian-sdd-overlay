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
