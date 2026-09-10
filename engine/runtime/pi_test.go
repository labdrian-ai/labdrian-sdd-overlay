package runtime_test

import (
	"os"
	"path/filepath"
	"testing"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
)

// piFixtureOverlay writes a minimal overlay tree (one pi-targeted skill,
// agents/GADU.md, a matching registry) and returns its root and registry
// path, for exercising PiAdapter's pipkg wiring (task 2.4).
func piFixtureOverlay(t *testing.T) (overlayRoot, registryPath string) {
	t.Helper()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "skills", "pi-skill", "SKILL.md"), "---\nname: pi-skill\n---\nbody\n")
	mustWrite(t, filepath.Join(root, "agents", "GADU.md"), "---\nname: GADU\n---\nbody\n")
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
	mustWrite(t, regPath, registry)
	return root, regPath
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestPiAdapter_ApplyInstallSyncCheck_WiredToPipkg pins task 2.4: with real
// overlay/registry/dest paths, Apply/Install build the package via pipkg and
// SyncCheck reports it as drift-free right after.
func TestPiAdapter_ApplyInstallSyncCheck_WiredToPipkg(t *testing.T) {
	overlayRoot, registryPath := piFixtureOverlay(t)
	destDir := filepath.Join(t.TempDir(), "labdrian-pi")

	adapter := engineRuntime.NewPiAdapterWithPaths(overlayRoot, registryPath, destDir)

	applyResult := adapter.Apply()
	if applyResult.Status == engineRuntime.CapabilityUnsupported {
		t.Fatalf("Apply with real paths must not be unsupported, got: %s", applyResult)
	}
	if _, err := os.Stat(filepath.Join(destDir, "package.json")); err != nil {
		t.Errorf("Apply must build the package: %v", err)
	}

	installResult := adapter.Install()
	if installResult.Status == engineRuntime.CapabilityUnsupported {
		t.Fatalf("Install with real paths must not be unsupported, got: %s", installResult)
	}

	syncResult := adapter.SyncCheck()
	if syncResult.Status != engineRuntime.CapabilitySupported {
		t.Fatalf("SyncCheck right after a build must report supported (no drift), got: %s", syncResult)
	}
}

// TestPiAdapter_ApplyWithoutOverlayRoot_StaysHonestlyUnsupported guards the
// zero-arg NewPiAdapter() path (used by NewFoundationAdapter(TargetPi) and
// exercised by TestExpandTarget_Pi): with OVERLAY_DIR unset, wiring the
// pipkg calls must not fabricate success.
func TestPiAdapter_ApplyWithoutOverlayRoot_StaysHonestlyUnsupported(t *testing.T) {
	adapter := engineRuntime.NewPiAdapterWithPaths("", "", t.TempDir())
	for _, result := range []engineRuntime.LifecycleResult{adapter.Apply(), adapter.Install(), adapter.SyncCheck()} {
		if result.Status != engineRuntime.CapabilityUnsupported {
			t.Fatalf("%s with empty overlayRoot must stay unsupported, got: %s", result.Action, result)
		}
	}
}
