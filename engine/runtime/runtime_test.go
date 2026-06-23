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
	// R-101: all supported runtime targets and the aggregate target parse.
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
				t.Errorf("ParseTarget(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestParseTargetRejectsUnknownTarget(t *testing.T) {
	// R-101: unsupported targets are rejected with visible target context.
	_, err := engineRuntime.ParseTarget("future-cli")
	if err == nil {
		t.Fatal("ParseTarget should reject unknown targets")
	}
	if !strings.Contains(err.Error(), "future-cli") {
		t.Errorf("error should name rejected target, got %q", err.Error())
	}
}

func TestCapabilityStatusValuesAreStable(t *testing.T) {
	// R-101/R-104: lifecycle status values remain stable for user-visible reporting.
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
				t.Errorf("status = %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestLifecycleResultRendersTargetStatusAndMessage(t *testing.T) {
	// R-101: target-specific lifecycle results render action, status, and message independently.
	result := engineRuntime.LifecycleResult{
		Target:  engineRuntime.TargetClaude,
		Action:  engineRuntime.ActionStatus,
		Status:  engineRuntime.CapabilitySupported,
		Message: "Claude hooks are the deterministic baseline",
	}

	got := result.String()
	for _, want := range []string{"claude", "status", "supported", "Claude hooks"} {
		if !strings.Contains(got, want) {
			t.Errorf("LifecycleResult.String() should contain %q; got %q", want, got)
		}
	}
}

func TestLoadContractPhasesFromContent(t *testing.T) {
	// R-001: phase-scoped contract metadata drives injection/stripping decisions.
	phases, err := engineRuntime.LoadContractPhases(contractContent)
	if err != nil {
		t.Fatalf("LoadContractPhases: %v", err)
	}

	if phases.InjectionPoint != "## Skills to load before work" {
		t.Errorf("InjectionPoint = %q", phases.InjectionPoint)
	}
	if !phases.AppliesToPhase("sdd-tasks") || !phases.AppliesToPhase("sdd-apply") {
		t.Errorf("applies_to phases not recognized: %#v", phases.AppliesTo)
	}
	if !phases.ExcludesPhase("sdd-propose") || phases.ExcludesPhase("sdd-apply") {
		t.Errorf("excluded phases not recognized: %#v", phases.Excluded)
	}
}

func TestMutatePromptInjectsAndStripsByContractPhases(t *testing.T) {
	// R-001/R-102: scoped phases receive the contract while excluded phases remove it.
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
	// R-001: unknown phases are not mutated without explicit contract metadata.
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
		t.Errorf("unknown phase prompt changed: got %q", got)
	}
}

func TestPromptHelpersHandleExistingHeaderAndDefaultHeader(t *testing.T) {
	// R-001: prompt mutation handles both existing orchestrator headers and default headers.
	const contractPath = "skills/_shared/minimalism-contract.md"

	withHeader := "Do work.\n## Skills to load before work\n- existing"
	injected := engineRuntime.InjectPrompt(withHeader, contractPath, "## Skills to load before work")
	if !hasExactLine(injected, contractPath) {
		t.Fatalf("InjectPrompt should insert contract under an existing header, got:\n%s", injected)
	}
	if strings.Count(injected, contractPath) != 1 {
		t.Fatalf("InjectPrompt should not duplicate the contract path, got:\n%s", injected)
	}
	if reinjected := engineRuntime.InjectPrompt(injected, contractPath, "## Skills to load before work"); reinjected != injected {
		t.Fatalf("InjectPrompt should be idempotent when contract path already exists")
	}

	phases := engineRuntime.ContractPhases{AppliesTo: []string{"sdd-apply"}}
	mutated, changed := engineRuntime.MutatePrompt("Apply now.", "sdd-apply", contractPath, phases)
	if !changed || !strings.Contains(mutated, "## Skills to load before work") {
		t.Fatalf("MutatePrompt should use the default injection header when frontmatter omits one, got changed=%v prompt=%q", changed, mutated)
	}
}

func TestLoadContractPhasesRejectsBrokenFrontmatter(t *testing.T) {
	// R-001: invalid contract metadata must fail loudly instead of silently mutating prompts.
	if _, err := engineRuntime.LoadContractPhases("no frontmatter"); err == nil {
		t.Fatal("LoadContractPhases should reject content without frontmatter")
	}
}

func TestExpandTargetAndFoundationAdapters(t *testing.T) {
	// R-101/R-104: target expansion and foundation adapters expose honest per-target lifecycle status.
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

	claude := engineRuntime.NewFoundationAdapter(engineRuntime.TargetClaude)
	if claude.Target() != engineRuntime.TargetClaude || claude.Status().Status != engineRuntime.CapabilityPartial {
		t.Fatalf("Claude target lifecycle should not false-green legacy hook state, got target=%q status=%q", claude.Target(), claude.Status().Status)
	}
	if !strings.Contains(claude.Status().Message, "status-hooks") {
		t.Fatalf("Claude status should direct users to real legacy hook lifecycle, got %q", claude.Status().Message)
	}

	unknown := engineRuntime.NewFoundationAdapter(engineRuntime.Target("future"))
	for _, result := range []engineRuntime.LifecycleResult{
		unknown.Apply(), unknown.Install(), unknown.Status(), unknown.SyncCheck(), unknown.Update(), unknown.Rollback(), unknown.Uninstall(),
	} {
		if result.Target != engineRuntime.Target("future") || result.Status != engineRuntime.CapabilityUnsupported {
			t.Fatalf("unknown foundation result = %#v, want future unsupported", result)
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
