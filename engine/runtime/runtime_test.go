package runtime_test

import (
	"os"
	"strings"
	"testing"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/registryyaml"
)

// fileRegistries reads the registry files of a test from the file system, as the program does.
var fileRegistries = registryyaml.NewRepository(os.ReadFile)

// TestPiFromTheRegistryWithNoOverlayDirIsHonestlyUnsupported: a Pi adapter built from a Config
// that names no OVERLAY_DIR and a state dir with nothing built in it reports every action
// unsupported (R-001, R-008), never a fabricated success.
func TestPiFromTheRegistryWithNoOverlayDirIsHonestlyUnsupported(t *testing.T) {
	adapter, err := shippedRegistry(t).New(core.TargetPi, core.Config{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if adapter.Target() != core.TargetPi {
		t.Fatalf("PiAdapter.Target() = %q, want %q", adapter.Target(), core.TargetPi)
	}
	for _, result := range []core.LifecycleResult{
		adapter.Apply(), adapter.Install(), adapter.Status(), adapter.SyncCheck(),
		adapter.Update(), adapter.Rollback(), adapter.Uninstall(),
	} {
		if result.Status != core.CapabilityUnsupported {
			t.Fatalf("PiAdapter %s: status = %q, want unsupported (nothing built, no OVERLAY_DIR)", result.Action, result.Status)
		}
		if result.Target != core.TargetPi {
			t.Fatalf("PiAdapter %s: target = %q, want %q", result.Action, result.Target, core.TargetPi)
		}
	}
}

// TestPiAdapter_UnbuiltDefaultReportsConcreteReasons (supersedes the
// pre-pi-lifecycle stub-wording test): an unbuilt package now reports its
// own concrete reason instead of a "scheduled for a later slice" placeholder.
func TestPiAdapter_UnbuiltDefaultReportsConcreteReasons(t *testing.T) {
	adapter, err := shippedRegistry(t).New(core.TargetPi, core.Config{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}

	for _, result := range []core.LifecycleResult{adapter.Status(), adapter.Uninstall()} {
		if !strings.Contains(result.Message, "not built") {
			t.Fatalf("PiAdapter %s: message should say the package is not built, got %q", result.Action, result.Message)
		}
	}
	for _, result := range []core.LifecycleResult{adapter.Update(), adapter.Rollback()} {
		if !strings.Contains(result.Message, "OVERLAY_DIR") {
			t.Fatalf("PiAdapter %s: message should name the missing OVERLAY_DIR, got %q", result.Action, result.Message)
		}
	}
}

// TestClaudeAndCodexFromTheRegistryInAnEmptyHome: the adapters the registry builds from a Config
// whose home holds nothing report what the adapters of an empty machine always reported: Claude
// has no settings file, Codex has a root and no manifest.
func TestClaudeAndCodexFromTheRegistryInAnEmptyHome(t *testing.T) {
	registry := shippedRegistry(t)
	cfg := core.Config{Home: t.TempDir()}

	claude, err := registry.New(core.TargetClaude, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := claude.(engineRuntime.ClaudeAdapter); !ok {
		t.Fatalf("New(claude) = %T, want a ClaudeAdapter", claude)
	}
	if claude.Status().Status != core.CapabilityUnsupported {
		t.Fatalf("Claude status should be unsupported in an empty home, got %q", claude.Status().Status)
	}

	codex, err := registry.New(core.TargetCodex, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := codex.(engineRuntime.CodexAdapter); !ok {
		t.Fatalf("New(codex) = %T, want a CodexAdapter", codex)
	}
	if codex.Status().Status != core.CapabilityPartial {
		t.Fatalf("Codex status should be partial in an empty home, got %q", codex.Status().Status)
	}
}
