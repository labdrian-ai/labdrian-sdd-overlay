package runtime_test

import (
	"strings"
	"testing"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
)

const contractContent = `---
applies_to_phases: [sdd-tasks, sdd-apply]
excluded_phases: [sdd-propose, sdd-spec, sdd-design, sdd-verify, sdd-archive]
injection_point: "## Skills to load before work"
---
# Minimalism Contract
`

func TestParseTargetAcceptsKnownTargetsAndAll(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want engineRuntime.Target
	}{
		{name: "claude", raw: "claude", want: engineRuntime.TargetClaude},
		{name: "opencode", raw: "opencode", want: engineRuntime.TargetOpenCode},
		{name: "codex", raw: "codex", want: engineRuntime.TargetCodex},
		{name: "all", raw: "all", want: engineRuntime.TargetAll},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := engineRuntime.ParseTarget(tt.raw)
			if err != nil {
				t.Fatalf("ParseTarget(%q): %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("ParseTarget(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestParseTargetRejectsUnknownTarget(t *testing.T) {
	_, err := engineRuntime.ParseTarget("future-cli")
	if err == nil {
		t.Fatal("ParseTarget should reject unknown targets")
	}
	if !strings.Contains(err.Error(), "future-cli") {
		t.Fatalf("error should name rejected target, got %q", err)
	}
}

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

func TestLoadContractPhasesFromContent(t *testing.T) {
	phases, err := engineRuntime.LoadContractPhases(contractContent)
	if err != nil {
		t.Fatalf("LoadContractPhases: %v", err)
	}
	if phases.InjectionPoint != "## Skills to load before work" {
		t.Fatalf("InjectionPoint = %q", phases.InjectionPoint)
	}
	if !phases.AppliesToPhase("sdd-tasks") || !phases.AppliesToPhase("sdd-apply") {
		t.Fatalf("applies-to phases not recognized: %#v", phases.AppliesTo)
	}
	if !phases.ExcludesPhase("sdd-propose") || phases.ExcludesPhase("sdd-apply") {
		t.Fatalf("excluded phases not recognized: %#v", phases.Excluded)
	}
}

func TestMutatePromptInjectsAndStripsByContractPhases(t *testing.T) {
	phases, err := engineRuntime.LoadContractPhases(contractContent)
	if err != nil {
		t.Fatalf("LoadContractPhases: %v", err)
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
	phases, err := engineRuntime.LoadContractPhases(contractContent)
	if err != nil {
		t.Fatalf("LoadContractPhases: %v", err)
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

	phases := engineRuntime.ContractPhases{AppliesTo: []string{"sdd-apply"}}
	mutated, changed := engineRuntime.MutatePrompt("Apply now.", "sdd-apply", contractPath, phases)
	if !changed || !strings.Contains(mutated, "## Skills to load before work") {
		t.Fatalf("MutatePrompt should use default injection header when absent, got changed=%v prompt=%q", changed, mutated)
	}
}

func TestExpandTargetAndFoundationAdapters(t *testing.T) {
	expanded := engineRuntime.ExpandTarget(engineRuntime.TargetAll)
	wantTargets := []engineRuntime.Target{engineRuntime.TargetClaude, engineRuntime.TargetOpenCode, engineRuntime.TargetCodex}
	if len(expanded) != len(wantTargets) {
		t.Fatalf("ExpandTarget(all) length = %d, want %d", len(expanded), len(wantTargets))
	}
	for i, want := range wantTargets {
		if expanded[i] != want {
			t.Fatalf("ExpandTarget(all)[%d] = %q, want %q", i, expanded[i], want)
		}
	}
	if single := engineRuntime.ExpandTarget(engineRuntime.TargetCodex); len(single) != 1 || single[0] != engineRuntime.TargetCodex {
		t.Fatalf("ExpandTarget(codex) = %#v", single)
	}

	t.Setenv("HOME", t.TempDir())
	claude := engineRuntime.NewFoundationAdapter(engineRuntime.TargetClaude)
	if _, ok := claude.(engineRuntime.ClaudeAdapter); !ok {
		t.Fatalf("NewFoundationAdapter(claude) should return ClaudeAdapter foundation")
	}
	if claude.Target() != engineRuntime.TargetClaude || claude.Status().Status != engineRuntime.CapabilityUnsupported {
		t.Fatalf("Claude foundation status should be unsupported in an empty HOME sandbox, got target=%q status=%q", claude.Target(), claude.Status().Status)
	}

	codex := engineRuntime.NewFoundationAdapter(engineRuntime.TargetCodex)
	if _, ok := codex.(engineRuntime.CodexAdapter); !ok {
		t.Fatalf("NewFoundationAdapter(codex) should return CodexAdapter foundation")
	}
	if codex.Target() != engineRuntime.TargetCodex || codex.Status().Status != engineRuntime.CapabilityUnsupported {
		t.Fatalf("Codex foundation status should be unsupported in the OpenCode-only salvage, got target=%q status=%q", codex.Target(), codex.Status().Status)
	}

	unknown := engineRuntime.NewFoundationAdapter(engineRuntime.Target("future"))
	for _, result := range []engineRuntime.LifecycleResult{
		unknown.Apply(), unknown.Install(), unknown.Status(), unknown.SyncCheck(), unknown.Update(), unknown.Rollback(), unknown.Uninstall(),
	} {
		if result.Target != engineRuntime.Target("future") || result.Status != engineRuntime.CapabilityUnsupported {
			t.Fatalf("unexpected fallback adapter result: %#v", result)
		}
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
