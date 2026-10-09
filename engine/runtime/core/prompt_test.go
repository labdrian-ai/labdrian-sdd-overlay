package core_test

import (
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/contract"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

const contractContent = `---
applies_to_phases: [sdd-tasks, sdd-apply]
excluded_phases: [sdd-propose, sdd-spec, sdd-design, sdd-verify, sdd-archive]
injection_point: "## Skills to load before work"
---
# Minimalism Contract
`

func TestMutatePromptInjectsAndStripsByContractPhases(t *testing.T) {
	phases, err := contract.Parse(contractContent)
	if err != nil {
		t.Fatalf("contract.Parse: %v", err)
	}
	const contractPath = "skills/_shared/minimalism-contract.md"

	injected, changed := core.MutatePrompt("Do apply.", "sdd-apply", contractPath, phases)
	if !changed {
		t.Fatal("sdd-apply should mutate the prompt")
	}
	if !hasExactLine(injected, contractPath) {
		t.Fatalf("injected prompt should contain contract path as exact line, got:\n%s", injected)
	}

	stripped, changed := core.MutatePrompt(injected, "sdd-verify", contractPath, phases)
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

	got, changed := core.MutatePrompt(prompt, "sdd-future", "skills/_shared/minimalism-contract.md", phases)
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
	injected := core.InjectPrompt(withHeader, contractPath, "## Skills to load before work")
	if !hasExactLine(injected, contractPath) {
		t.Fatalf("InjectPrompt should insert contract under an existing header, got:\n%s", injected)
	}
	if strings.Count(injected, contractPath) != 1 {
		t.Fatalf("InjectPrompt should be idempotent when contract path already exists, got:\n%s", injected)
	}
	if reinjected := core.InjectPrompt(injected, contractPath, "## Skills to load before work"); reinjected != injected {
		t.Fatalf("InjectPrompt should be idempotent when contract path already exists")
	}

	phases := contract.Contract{AppliesTo: []string{"sdd-apply"}}
	mutated, changed := core.MutatePrompt("Apply now.", "sdd-apply", contractPath, phases)
	if !changed || !strings.Contains(mutated, "## Skills to load before work") {
		t.Fatalf("MutatePrompt should use default injection header when absent, got changed=%v prompt=%q", changed, mutated)
	}
}

// With no header in the prompt, the header and the path are appended after a blank line, however
// the prompt ends: a prompt with no final newline gets one blank line, one that ends in a newline
// gets the one blank line the newline leaves, and one that already ends in a blank line is not
// trimmed (the separator is a single newline). The outputs are pinned as they are.
func TestInjectPromptAppendsTheHeaderAfterABlankLineWhateverThePromptEndsWith(t *testing.T) {
	const path, header = "skills/_shared/minimalism-contract.md", "## Skills to load before work"
	tail := header + "\n" + path + "\n"
	cases := []struct{ name, prompt, want string }{
		{"no final newline", "Do work.", "Do work.\n\n" + tail},
		{"one final newline", "Do work.\n", "Do work.\n\n" + tail},
		{"a final blank line", "Do work.\n\n", "Do work.\n\n\n" + tail},
		{"an empty prompt", "", "\n\n" + tail},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := core.InjectPrompt(tc.prompt, path, header); got != tc.want {
				t.Errorf("InjectPrompt(%q) = %q, want %q", tc.prompt, got, tc.want)
			}
		})
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
