package gate_test

// The memory instructions of C23 M6 (docs/architecture/adr-c23-m6-plan-instructions.md, decision
// 2): while a workflow is bound the caller hands the gate the instructions block, and the same
// rewrite that applies the contract paths appends it to every sub-agent's prompt, once. The gate
// reads no binding: an empty block is a repository nothing is bound to, and the rewrite is then
// what it always was.

import (
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gate"
)

const instructionsBlock = "labdrian memory instructions (workflow wf-1, scope project):\n- Read memory only from: engram. A read from any other source is denied."

func withInstructions(cfg gate.Config) gate.Config {
	cfg.Instructions = instructionsBlock
	return cfg
}

func TestTheInstructionsAreAppendedToEverySubagentPrompt(t *testing.T) {
	for _, subagentType := range []string{"sdd-apply", "sdd-propose", "general-purpose", "Explore", ""} {
		got, changed := rewrite(withInstructions(gate.Config{}), subagentType, "Do the task.")
		if want := "Do the task.\n\n" + instructionsBlock + "\n"; !changed || got != want {
			t.Errorf("%q: Rewrite() = %q, %v, want %q, true", subagentType, got, changed, want)
		}
	}
}

// The contract paths and the block are one rewrite: the contract first, then the block.
func TestTheInstructionsJoinTheContractPathsInTheSameRewrite(t *testing.T) {
	got, changed := rewrite(withInstructions(singleContract(contractPath, contractContent)), "sdd-apply", "Do the task.")
	want := "Do the task.\n\n## Skills to load before work\n" + contractPath + "\n\n" + instructionsBlock + "\n"
	if !changed || got != want {
		t.Errorf("Rewrite() = %q, %v, want %q, true", got, changed, want)
	}
}

// A prompt that already carries the block is not given it twice, so the rewrite is idempotent.
func TestTheInstructionsAreNotAppendedTwice(t *testing.T) {
	cfg := withInstructions(singleContract(contractPath, contractContent))
	for _, prompt := range []string{"Do the task.", "Do the task.\n", "Do the task.\n\n"} {
		once, _ := rewrite(cfg, "sdd-apply", prompt)
		twice, changed := rewrite(cfg, "sdd-apply", once)
		if changed || twice != once || strings.Count(twice, instructionsBlock) != 1 {
			t.Errorf("%q: a second rewrite gave %q, %v, want %q unchanged", prompt, twice, changed, once)
		}
	}
}

// Only a block that ends the prompt counts as given: a prompt that quotes the block and goes on
// after it is still given the block at its end (C23 M6 review A, R1-001).
func TestAPromptThatQuotesTheBlockIsStillGivenItAtTheEnd(t *testing.T) {
	cfg := withInstructions(gate.Config{})
	prompt := "Compare this block with the plan:\n\n" + instructionsBlock + "\n\nThen report."
	got, changed := rewrite(cfg, "general-purpose", prompt)
	if want := prompt + "\n\n" + instructionsBlock + "\n"; !changed || got != want {
		t.Errorf("Rewrite() = %q, %v, want the block appended at the end %q", got, changed, want)
	}
}

// An empty prompt is left alone, block or not, as it always was.
func TestAnEmptyPromptIsNotGivenTheInstructions(t *testing.T) {
	if got, changed := rewrite(withInstructions(gate.Config{}), "sdd-apply", ""); changed || got != "" {
		t.Errorf("Rewrite() = %q, %v, want the empty prompt unchanged", got, changed)
	}
}

// Without a block (no workflow bound) the rewrite is what it was before M6.
func TestWithoutInstructionsTheRewriteIsUnchanged(t *testing.T) {
	cfg := singleContract(contractPath, contractContent)
	for _, tt := range []struct{ subagentType, prompt, want string }{
		{"sdd-apply", "Do the task.", "Do the task.\n\n## Skills to load before work\n" + contractPath + "\n"},
		{"general-purpose", "Do the task.", "Do the task."},
		{"", "Do the task.", "Do the task."},
	} {
		if got, _ := rewrite(cfg, tt.subagentType, tt.prompt); got != tt.want {
			t.Errorf("%q: Rewrite() = %q, want %q", tt.subagentType, got, tt.want)
		}
	}
}
