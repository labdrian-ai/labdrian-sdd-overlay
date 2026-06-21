// Package gate implements the fail-safe gate-task subcommand for the
// deterministic-scoping engine.
//
// gate-task reads a Claude Code PreToolUse 'Task' tool_input JSON from STDIN,
// inspects subagent_type, and:
//   - INJECTS the minimalism-contract path into the sub-agent prompt when
//     subagent_type is in the applies_to_phases set from contract frontmatter.
//   - STRIPS the minimalism-contract path when subagent_type is in the
//     excluded_phases set and the path is present.
//   - PASSES THROUGH unchanged on any error, unknown type, malformed input,
//     or broken frontmatter.
//
// CRITICAL SAFETY: gate-task MUST be fail-safe. On ANY error it outputs a
// pass-through response that leaves tool_input UNCHANGED and exits 0. It must
// NEVER block a Task, NEVER crash, NEVER deny.
package gate

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator"
)

// Config holds the runtime configuration for the gate processor.
type Config struct {
	// ContractPath is the path to the minimalism-contract file as it appears
	// in skill prompts (e.g. "skills/_shared/minimalism-contract.md").
	ContractPath string

	// ContractContent is the raw content of the contract file, used to parse
	// the frontmatter for phase sets.
	ContractContent string
}

// hookInput represents the Claude Code PreToolUse hook JSON input shape.
type hookInput struct {
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input"`
}

// taskToolInput represents the Task tool's input fields we care about.
type taskToolInput struct {
	SubagentType string `json:"subagent_type"`
	Prompt       string `json:"prompt"`
}

// hookResponse is the Claude Code PreToolUse hook response shape.
// hookSpecificOutput is included only when the input is modified.
type hookResponse struct {
	HookSpecificOutput *hookSpecificOutput `json:"hookSpecificOutput,omitempty"`
}

type hookSpecificOutput struct {
	UpdatedInput *updatedInput `json:"updatedInput,omitempty"`
}

type updatedInput struct {
	Prompt string `json:"prompt"`
}

// passThrough returns the benign allow response that leaves tool_input unchanged.
// This is the fail-safe output used on any error or unknown state.
func passThrough() (string, error) {
	resp := hookResponse{}
	b, err := json.Marshal(resp)
	if err != nil {
		// Absolute last resort: hand-craft a minimal valid JSON.
		return `{}`, nil
	}
	return string(b), nil
}

// Process applies gate logic to the raw JSON input string.
// It returns (responseJSON, nil) in all cases — the nil error contract is
// intentional: the gate is fail-safe and must never return a non-nil error
// that would cause the caller to exit non-zero. Errors are absorbed into
// pass-through responses.
func Process(rawInput string, cfg Config) (string, error) {
	// Parse the contract frontmatter to derive phase sets.
	// On broken frontmatter → fail-safe pass-through (gate is NOT like propagate
	// which may fail loud; see design asymmetry).
	phases, err := propagator.ParseFrontmatter(cfg.ContractContent)
	if err != nil {
		return passThrough()
	}

	applySet := toSet(phases.AppliesTo)
	excludeSet := toSet(phases.Excluded)

	// Parse the hook input JSON. On any parse failure → fail-safe pass-through.
	if strings.TrimSpace(rawInput) == "" {
		return passThrough()
	}

	var hi hookInput
	if err := json.Unmarshal([]byte(rawInput), &hi); err != nil {
		return passThrough()
	}

	// tool_input must be an object. If absent or not an object → pass-through.
	if len(hi.ToolInput) == 0 {
		return passThrough()
	}

	var ti taskToolInput
	if err := json.Unmarshal(hi.ToolInput, &ti); err != nil {
		return passThrough()
	}

	// subagent_type must be non-empty to take any action.
	if ti.SubagentType == "" {
		return passThrough()
	}

	// prompt must be present to inject or strip.
	// If missing → pass-through (nothing to modify).
	if ti.Prompt == "" {
		return passThrough()
	}

	switch {
	case applySet[ti.SubagentType]:
		// INJECT: ensure the contract path appears under the injection_point header.
		newPrompt := inject(ti.Prompt, cfg.ContractPath)
		if newPrompt == ti.Prompt {
			// Already present — no-op pass-through.
			return passThrough()
		}
		return buildResponse(newPrompt)

	case excludeSet[ti.SubagentType]:
		// STRIP: remove the contract path if present.
		newPrompt := strip(ti.Prompt, cfg.ContractPath)
		if newPrompt == ti.Prompt {
			// Was not present — no-op pass-through.
			return passThrough()
		}
		return buildResponse(newPrompt)

	default:
		// Unknown subagent_type → fail-safe pass-through.
		return passThrough()
	}
}

// toSet converts a string slice to a lookup map.
func toSet(items []string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, item := range items {
		m[item] = true
	}
	return m
}

// injectionHeader is the header under which the contract path is injected.
const injectionHeader = "## Skills to load before work"

// inject ensures that contractPath appears under the injectionHeader in prompt.
// If the header already exists, the path is appended under it.
// If the header does not exist, it is added at the end of the prompt.
func inject(prompt, contractPath string) string {
	// Already present → no-op.
	if strings.Contains(prompt, contractPath) {
		return prompt
	}

	entry := fmt.Sprintf("Read fully BEFORE work: %s", contractPath)

	if strings.Contains(prompt, injectionHeader) {
		// Insert the entry right after the header line.
		lines := strings.Split(prompt, "\n")
		var out []string
		for i, line := range lines {
			out = append(out, line)
			if strings.TrimSpace(line) == injectionHeader {
				// Check next line is not already our entry.
				if i+1 < len(lines) && strings.Contains(lines[i+1], contractPath) {
					continue // already there somehow
				}
				out = append(out, entry)
			}
		}
		return strings.Join(out, "\n")
	}

	// No header present — append header + entry.
	sep := "\n"
	if !strings.HasSuffix(prompt, "\n") {
		sep = "\n\n"
	} else if !strings.HasSuffix(prompt, "\n\n") {
		sep = "\n"
	}
	return prompt + sep + injectionHeader + "\n" + entry + "\n"
}

// strip removes the line containing contractPath from the prompt, along with
// any trailing blank line that would be left. The injectionHeader is NOT
// removed (other skills may also live under it).
func strip(prompt, contractPath string) string {
	if !strings.Contains(prompt, contractPath) {
		return prompt
	}
	lines := strings.Split(prompt, "\n")
	var out []string
	for _, line := range lines {
		if strings.Contains(line, contractPath) {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// buildResponse constructs the Claude Code PreToolUse hook response that
// carries the modified prompt.
func buildResponse(newPrompt string) (string, error) {
	resp := hookResponse{
		HookSpecificOutput: &hookSpecificOutput{
			UpdatedInput: &updatedInput{
				Prompt: newPrompt,
			},
		},
	}
	b, err := json.Marshal(resp)
	if err != nil {
		return passThrough()
	}
	return string(b), nil
}
