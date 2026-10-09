package runtime_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

// The overlay the OpenCode plugin's contracts are read from is a choice the composition root
// hands the adapter (OpenCodeOptions.OverlayDir, from $LABDRIAN_OVERLAY_DIR). The adapter reads no
// environment, so the variable has no say here.

// shippedOverlay is the repository root, which holds the real contracts.
func shippedOverlay(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestOpenCodeReadsItsContractsFromTheOverlayItWasGiven(t *testing.T) {
	// The environment names a directory with no contracts; the option names the real overlay.
	t.Setenv("LABDRIAN_OVERLAY_DIR", t.TempDir())
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{OverlayDir: shippedOverlay(t)})

	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v, want the overlay of the option to be the one read", result)
	}
}

func TestOpenCodeIgnoresTheEnvironmentWhenItWasGivenAnOverlay(t *testing.T) {
	// The environment names the real overlay; the option names a directory with no contracts.
	t.Setenv("LABDRIAN_OVERLAY_DIR", shippedOverlay(t))
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{OverlayDir: t.TempDir()})

	result := adapter.Install()
	if result.Status != core.CapabilityPartial || !strings.Contains(result.Message, "prompt config could not be derived") {
		t.Fatalf("Install() = %#v, want the empty overlay of the option to fail it: the environment must not be read", result)
	}
}

func TestOpenCodeRefusesARelativeOverlayAndNamesIt(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{OverlayDir: "relative/overlay"})

	result := adapter.Install()
	want := `LABDRIAN_OVERLAY_DIR must be absolute, got "relative/overlay"`
	if result.Status != core.CapabilityPartial || !strings.Contains(result.Message, want) {
		t.Fatalf("Install() = %#v, want a refusal that says %q", result, want)
	}
	if _, err := os.Stat(filepath.Join(root, "plugins")); !os.IsNotExist(err) {
		t.Errorf("a refused overlay still wrote under the config root (stat err: %v)", err)
	}
}

// A value that is only space is no value: the adapter looks for the overlay above the working
// directory, which for this test is inside the repository.
func TestOpenCodeTreatsABlankOverlayAsNoneAndLooksAboveTheWorkingDirectory(t *testing.T) {
	adapter := engineRuntime.NewOpenCodeAdapter(t.TempDir(), engineRuntime.OpenCodeOptions{OverlayDir: "  \t "})

	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v, want the overlay above the working directory to be found", result)
	}
}

// The registry hands the adapter the overlay the Config names, so the composition root's one read
// of the variable is the one the adapter sees.
func TestTheRegistryHandsOpenCodeTheOverlayOfTheConfig(t *testing.T) {
	adapter, err := shippedRegistry(t).New(core.TargetOpenCode, core.Config{ConfigRoot: t.TempDir(), LabdrianOverlayDir: "relative/overlay"})
	if err != nil {
		t.Fatal(err)
	}
	result := adapter.Install()
	if want := `LABDRIAN_OVERLAY_DIR must be absolute, got "relative/overlay"`; result.Status != core.CapabilityPartial || !strings.Contains(result.Message, want) {
		t.Fatalf("Install() = %#v, want the overlay of the Config to reach the adapter (%q)", result, want)
	}
}

// Space around an absolute path is not part of it.
func TestOpenCodeTrimsTheSpaceAroundTheOverlay(t *testing.T) {
	adapter := engineRuntime.NewOpenCodeAdapter(t.TempDir(), engineRuntime.OpenCodeOptions{OverlayDir: "  " + shippedOverlay(t) + "\t"})

	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v, want the padded path to be read as the overlay it names", result)
	}
}

// Nothing above the working directory and nothing configured: the refusal says what to set.
func TestOpenCodeWithoutAnyOverlayAsksForOne(t *testing.T) {
	elsewhere := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(elsewhere); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	result := engineRuntime.NewOpenCodeAdapter(t.TempDir(), engineRuntime.OpenCodeOptions{}).Install()
	want := "could not locate skills/_shared/minimalism-contract.md; set LABDRIAN_OVERLAY_DIR"
	if result.Status != core.CapabilityPartial || !strings.Contains(result.Message, want) {
		t.Fatalf("Install() = %#v, want a refusal that says %q", result, want)
	}
}
