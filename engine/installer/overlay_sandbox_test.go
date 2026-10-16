package installer_test

// The sandboxes the integration tests of this package run bin/labdrian-overlay in: a throwaway
// overlay repository with the branches the script merges and compares (overlaytest.NewOverlay), a
// home of its own, and the helpers that run the script over them. (The routing pins have a world of
// their own, routing_pins_world_test.go.)

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// createTarball writes the provided relative paths into a temporary directory and
// returns a tar.gz path containing them under a `files/` root.
func createTarball(t *testing.T, files map[string]string) string {
	t.Helper()

	tarRoot := t.TempDir()
	overlaytest.WriteFiles(t, tarRoot, files)

	// Tarball naming can be deterministic per test path; keep this stable but
	// unique by placing it in a temp dir.
	tarPath := filepath.Join(t.TempDir(), "snapshot.tar.gz")
	cmd := exec.Command("tar", "-czf", tarPath, "-C", tarRoot, "files")
	if _, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create tarball: %v", err)
	}

	return tarPath
}

// setupCaptureFromBackupSandbox creates an overlay git repo + environment pair
// suitable for testing `overlay capture --from-backup`.
func setupCaptureFromBackupSandbox(t *testing.T, home string) (string, []string) {
	t.Helper()

	const manifest = "test-skill/SKILL.md   managed\n" +
		"GADU.md   managed   agent\n" +
		"opencode/agents/GADU.md   managed   opencode-agent\n"

	return overlaytest.NewOverlay(t, home, map[string]string{
		"overlay.manifest":           manifest,
		"skills/test-skill/SKILL.md": "# overlay skill\n",
		"agents/GADU.md":             "---\nname: GADU\n---\n# overlay agent\n",
		"opencode/agents/GADU.md":    "---\nname: GADU\n---\n# overlay open-agent\n",
	})
}

// setupSandboxOverlay creates a minimal overlay git repo with upstream+main
// branches containing native GADU agent files, skills/test-skill/SKILL.md, overlay.manifest.
// The sandbox HOME is provided by the caller. Returns overlayDir and the
// environment slice for exec.Command (includes OVERLAY_DIR and HOME overrides).
func setupSandboxOverlay(t *testing.T, home string) (string, []string) {
	t.Helper()

	const (
		fixtureManifestInteg = "test-skill/SKILL.md   managed\n" +
			"inception-pipeline/SKILL.md   custom\n" +
			"_shared/pre-sdd-contracts.md   custom\n" +
			"_shared/entry-contract.schema.json   custom\n" +
			"_shared/actuals-record.schema.json   custom\n" +
			"GADU.md   custom   agent\n" +
			"opencode/agents/GADU.md   custom   opencode-agent\n"
		fixtureSkillContent         = "# test skill\n"
		fixtureInceptionContent     = "---\nname: inception-pipeline\n---\n# inception pipeline\n"
		fixtureContractsContent     = "# pre-SDD contracts\n"
		fixtureEntrySchemaContent   = "{\"title\":\"entry contract\"}\n"
		fixtureActualsSchemaContent = "{\"title\":\"actuals record\"}\n"
		fixtureAgentContent         = "---\nname: GADU\ndescription: test agent\nmodel: opus\ntools: '*'\n---\n# GADU\n"
		fixtureOpenCodeAgentContent = "---\ndescription: test agent\nmode: all\nmodel: openai/gpt-5.5\npermission:\n  task: allow\n---\n# GADU\n"
	)

	return overlaytest.NewOverlay(t, home, map[string]string{
		"overlay.manifest":                          fixtureManifestInteg,
		"skills/test-skill/SKILL.md":                fixtureSkillContent,
		"skills/inception-pipeline/SKILL.md":        fixtureInceptionContent,
		"skills/_shared/pre-sdd-contracts.md":       fixtureContractsContent,
		"skills/_shared/entry-contract.schema.json": fixtureEntrySchemaContent,
		"skills/_shared/actuals-record.schema.json": fixtureActualsSchemaContent,
		"agents/GADU.md":                            fixtureAgentContent,
		"opencode/agents/GADU.md":                   fixtureOpenCodeAgentContent,
	})
}

// runApplyAllTolerantOfPi runs `apply --target all` and tolerates ONLY the
// now-intentional non-zero exit caused by pi's own known incompleteness
// (R4-silent-package-skip, pi-target-plumbing / pi-package-build): apply's
// aggregate exit reflects pi honestly (build failed in this fixture
// environment, which has no built engine binary at ENGINE_BINARY, so pi
// never actually gets deployed, exit 1) so unattended automation cannot
// read a run that silently skipped pi as a clean success. These fixtures
// exercise the claude/opencode/codex deploy loop under --target all, not
// pi's own build, so a pi-only failure must not fail them -- but any OTHER
// failure still must.
func runApplyAllTolerantOfPi(t *testing.T, overlay string, env []string) string {
	t.Helper()
	out, err := overlaytest.Combined(t, overlay, env, "", "apply", "--target", "all")
	if err != nil && !strings.Contains(out, "pi: package build failed") {
		t.Fatalf("apply --target all failed for a reason other than pi's known incompleteness: %v\noutput:\n%s", err, out)
	}
	return out
}

// runStatusAllTolerantOfPi is runApplyAllTolerantOfPi's sibling for `status
// --target all`, which now fails the same way for the same reason
// (R4-silent-package-skip) -- the pi package is never built in these
// fixtures, so pipkg_status_and_report honestly reports "not built".
func runStatusAllTolerantOfPi(t *testing.T, overlay string, env []string) string {
	t.Helper()
	out, err := overlaytest.Combined(t, overlay, env, "", "status", "--target", "all")
	if err != nil && !strings.Contains(out, "pi: not built") {
		t.Fatalf("status --target all failed for a reason other than pi's known incompleteness: %v\noutput:\n%s", err, out)
	}
	return out
}
