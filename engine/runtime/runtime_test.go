package runtime_test

import (
	"os"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/contract"
	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/registryyaml"
)

// fileRegistries reads the registry files of a test from the file system, as the program does.
var fileRegistries = registryyaml.NewRepository(os.ReadFile)

const contractContent = `---
applies_to_phases: [sdd-tasks, sdd-apply]
excluded_phases: [sdd-propose, sdd-spec, sdd-design, sdd-verify, sdd-archive]
injection_point: "## Skills to load before work"
---
# Minimalism Contract
`

func TestCapabilityStatusValuesAreStable(t *testing.T) {
	tests := []struct {
		name string
		got  engineRuntime.CapabilityStatus
		want string
	}{
		{name: "supported", got: engineRuntime.CapabilitySupported, want: "supported"},
		{name: "partial", got: engineRuntime.CapabilityPartial, want: "partial"},
		{name: "unsupported", got: engineRuntime.CapabilityUnsupported, want: "unsupported"},
		{name: "restart required", got: engineRuntime.CapabilityRestartRequired, want: "restart_required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.got) != tt.want {
				t.Fatalf("status = %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestLifecycleResultRendersTargetStatusAndMessage(t *testing.T) {
	result := engineRuntime.LifecycleResult{
		Target:  engineRuntime.TargetClaude,
		Action:  engineRuntime.ActionStatus,
		Status:  engineRuntime.CapabilitySupported,
		Message: "Claude hooks are the deterministic baseline",
	}

	got := result.String()
	for _, want := range []string{"claude", "status", "supported", "deterministic"} {
		if !strings.Contains(got, want) {
			t.Fatalf("LifecycleResult.String() should contain %q; got %q", want, got)
		}
	}
}

func TestMutatePromptInjectsAndStripsByContractPhases(t *testing.T) {
	phases, err := contract.Parse(contractContent)
	if err != nil {
		t.Fatalf("contract.Parse: %v", err)
	}
	const contractPath = "skills/_shared/minimalism-contract.md"

	injected, changed := engineRuntime.MutatePrompt("Do apply.", "sdd-apply", contractPath, phases)
	if !changed {
		t.Fatal("sdd-apply should mutate the prompt")
	}
	if !hasExactLine(injected, contractPath) {
		t.Fatalf("injected prompt should contain contract path as exact line, got:\n%s", injected)
	}

	stripped, changed := engineRuntime.MutatePrompt(injected, "sdd-verify", contractPath, phases)
	if !changed {
		t.Fatal("sdd-verify should strip an existing contract path")
	}
	if hasExactLine(stripped, contractPath) {
		t.Fatalf("stripped prompt should not contain contract path, got:\n%s", stripped)
	}
}

func TestMutatePromptLeavesUnknownPhaseUnchanged(t *testing.T) {
	phases, err := contract.Parse(contractContent)
	if err != nil {
		t.Fatalf("contract.Parse: %v", err)
	}
	const prompt = "Do future work."

	got, changed := engineRuntime.MutatePrompt(prompt, "sdd-future", "skills/_shared/minimalism-contract.md", phases)
	if changed {
		t.Fatal("unknown phases should not mutate prompt")
	}
	if got != prompt {
		t.Fatalf("unknown phase prompt changed: got %q", got)
	}
}

func TestPromptHelpersHandleExistingHeaderAndDefaultHeader(t *testing.T) {
	const contractPath = "skills/_shared/minimalism-contract.md"

	withHeader := "Do work.\n## Skills to load before work\n- existing"
	injected := engineRuntime.InjectPrompt(withHeader, contractPath, "## Skills to load before work")
	if !hasExactLine(injected, contractPath) {
		t.Fatalf("InjectPrompt should insert contract under an existing header, got:\n%s", injected)
	}
	if strings.Count(injected, contractPath) != 1 {
		t.Fatalf("InjectPrompt should be idempotent when contract path already exists, got:\n%s", injected)
	}
	if reinjected := engineRuntime.InjectPrompt(injected, contractPath, "## Skills to load before work"); reinjected != injected {
		t.Fatalf("InjectPrompt should be idempotent when contract path already exists")
	}

	phases := contract.Contract{AppliesTo: []string{"sdd-apply"}}
	mutated, changed := engineRuntime.MutatePrompt("Apply now.", "sdd-apply", contractPath, phases)
	if !changed || !strings.Contains(mutated, "## Skills to load before work") {
		t.Fatalf("MutatePrompt should use default injection header when absent, got changed=%v prompt=%q", changed, mutated)
	}
}

// TestPiFromTheRegistryWithNoOverlayDirIsHonestlyUnsupported: a Pi adapter built from a Config
// that names no OVERLAY_DIR and a state dir with nothing built in it reports every action
// unsupported (R-001, R-008), never a fabricated success.
func TestPiFromTheRegistryWithNoOverlayDirIsHonestlyUnsupported(t *testing.T) {
	adapter, err := shippedRegistry(t).New(engineRuntime.TargetPi, engineRuntime.Config{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if adapter.Target() != engineRuntime.TargetPi {
		t.Fatalf("PiAdapter.Target() = %q, want %q", adapter.Target(), engineRuntime.TargetPi)
	}
	for _, result := range []engineRuntime.LifecycleResult{
		adapter.Apply(), adapter.Install(), adapter.Status(), adapter.SyncCheck(),
		adapter.Update(), adapter.Rollback(), adapter.Uninstall(),
	} {
		if result.Status != engineRuntime.CapabilityUnsupported {
			t.Fatalf("PiAdapter %s: status = %q, want unsupported (nothing built, no OVERLAY_DIR)", result.Action, result.Status)
		}
		if result.Target != engineRuntime.TargetPi {
			t.Fatalf("PiAdapter %s: target = %q, want %q", result.Action, result.Target, engineRuntime.TargetPi)
		}
	}
}

// TestPiAdapter_UnbuiltDefaultReportsConcreteReasons (supersedes the
// pre-pi-lifecycle stub-wording test): an unbuilt package now reports its
// own concrete reason instead of a "scheduled for a later slice" placeholder.
func TestPiAdapter_UnbuiltDefaultReportsConcreteReasons(t *testing.T) {
	adapter, err := shippedRegistry(t).New(engineRuntime.TargetPi, engineRuntime.Config{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}

	for _, result := range []engineRuntime.LifecycleResult{adapter.Status(), adapter.Uninstall()} {
		if !strings.Contains(result.Message, "not built") {
			t.Fatalf("PiAdapter %s: message should say the package is not built, got %q", result.Action, result.Message)
		}
	}
	for _, result := range []engineRuntime.LifecycleResult{adapter.Update(), adapter.Rollback()} {
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
	cfg := engineRuntime.Config{Home: t.TempDir()}

	claude, err := registry.New(engineRuntime.TargetClaude, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := claude.(engineRuntime.ClaudeAdapter); !ok {
		t.Fatalf("New(claude) = %T, want a ClaudeAdapter", claude)
	}
	if claude.Status().Status != engineRuntime.CapabilityUnsupported {
		t.Fatalf("Claude status should be unsupported in an empty home, got %q", claude.Status().Status)
	}

	codex, err := registry.New(engineRuntime.TargetCodex, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := codex.(engineRuntime.CodexAdapter); !ok {
		t.Fatalf("New(codex) = %T, want a CodexAdapter", codex)
	}
	if codex.Status().Status != engineRuntime.CapabilityPartial {
		t.Fatalf("Codex status should be partial in an empty home, got %q", codex.Status().Status)
	}
}

func hasExactLine(text, line string) bool {
	for _, candidate := range strings.Split(text, "\n") {
		if strings.TrimSpace(candidate) == line {
			return true
		}
	}
	return false
}
