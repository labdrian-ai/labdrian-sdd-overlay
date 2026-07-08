// Package gate implements the fail-safe gate-task subcommand for the
// deterministic-scoping engine.
//
// gate-task reads a Claude Code PreToolUse 'Agent' tool_input JSON from STDIN,
// inspects subagent_type, and:
//   - INJECTS the minimalism-contract path into the sub-agent prompt when
//     subagent_type is in the applies_to_phases set from contract frontmatter.
//   - STRIPS the minimalism-contract path when subagent_type is in the
//     excluded_phases set and the path is present.
//   - PASSES THROUGH unchanged on any error, unknown type, malformed input,
//     or broken frontmatter.
//
// VERIFIED REALITY (Claude Code 2.1.185):
//   - The sub-agent spawn tool is named "Agent", NOT "Task". Settings matcher must
//     be "Agent".
//   - A PreToolUse hook on "Agent" DOES fire and updatedInput DOES rewrite the
//     sub-agent prompt.
//   - tool_input fields: description, prompt, subagent_type, model (optional).
//   - updatedInput MUST echo FULL tool_input (description, prompt[mutated],
//     subagent_type, model-if-present). Returning just {prompt:...} fails schema
//     validation ("required parameter description is missing").
//   - hookSpecificOutput MUST include hookEventName:"PreToolUse" and
//     permissionDecision:"allow". Without them, updatedInput is ignored.
//   - The canonical injected entry is a BARE absolute path line. Exact trimmed-line
//     matching prevents double-injection and makes strip work correctly.
//
// CRITICAL SAFETY: gate-task MUST be fail-safe. On ANY error it outputs a
// pass-through response that leaves tool_input UNCHANGED and exits 0. It must
// NEVER block an Agent call, NEVER crash, NEVER deny.
package gate

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator"
)

// defaultInjectionHeader is the fallback header when contract frontmatter does
// not specify injection_point.
const defaultInjectionHeader = "## Skills to load before work"

// Config holds the runtime configuration for the gate processor.
type Config struct {
	// ContractPath is the ABSOLUTE path to the minimalism-contract file as it
	// appears in skill prompts. This is the bare path line injected/stripped.
	// Example: "/home/user/.claude/skills/_shared/minimalism-contract.md"
	ContractPath string

	// ContractContent is the raw content of the contract file, used to parse
	// the frontmatter for phase sets and injection_point.
	ContractContent string

	// Contracts is the multi-contract configuration. When empty, ContractPath and
	// ContractContent are adapted into a single contract for backward compatibility.
	Contracts []ContractConfig

	// WorkContext is trusted work metadata supplied by the runtime adapter. It is
	// the only source used for context-aware contracts; prompt text is ignored.
	WorkContext *WorkContext
}

// ContractConfig describes one managed guidance contract.
type ContractConfig struct {
	Path    string
	Content string
}

// WorkContext is explicit metadata about the current unit of work.
type WorkContext struct {
	Trusted     bool     `json:"trusted"`
	Languages   []string `json:"languages"`
	Activations []string `json:"activations"`
	WorkKinds   []string `json:"work_kinds"`
}

// agentToolInput represents the Agent tool's input fields.
// All fields are preserved for faithful echo in updatedInput.
type agentToolInput struct {
	Description  string  `json:"description"`
	Prompt       string  `json:"prompt"`
	SubagentType string  `json:"subagent_type"`
	Model        *string `json:"model,omitempty"`
}

// hookInput represents the Claude Code PreToolUse hook JSON input shape.
type hookInput struct {
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input"`
}

// hookResponse is the Claude Code PreToolUse hook response shape.
// hookSpecificOutput is included only when the input is modified.
type hookResponse struct {
	HookSpecificOutput *hookSpecificOutput `json:"hookSpecificOutput,omitempty"`
}

// hookSpecificOutput carries the mutation decision and the modified input.
// hookEventName and permissionDecision are REQUIRED by Claude Code — without
// them, updatedInput is silently ignored.
type hookSpecificOutput struct {
	HookEventName      string        `json:"hookEventName"`
	PermissionDecision string        `json:"permissionDecision"`
	UpdatedInput       *updatedInput `json:"updatedInput,omitempty"`
}

// updatedInput is the full echo of tool_input with only prompt mutated.
// ALL fields from the original tool_input must be present to pass schema
// validation (Claude Code rejects the response if description is missing).
type updatedInput struct {
	Description  string  `json:"description"`
	Prompt       string  `json:"prompt"`
	SubagentType string  `json:"subagent_type"`
	Model        *string `json:"model,omitempty"`
}

// passThrough returns the benign allow response that leaves tool_input unchanged.
// This is the fail-safe output used on any error or unknown state.
// An empty JSON object {} is sufficient — Claude Code treats it as "no change".
func passThrough() (string, error) {
	return `{}`, nil
}

// Process applies gate logic to the raw JSON input string.
// It returns (responseJSON, nil) in all cases — the nil error contract is
// intentional: the gate is fail-safe and must never return a non-nil error
// that would cause the caller to exit non-zero. Errors are absorbed into
// pass-through responses.
func Process(rawInput string, cfg Config) (string, error) {
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

	var ti agentToolInput
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

	newPrompt := ti.Prompt
	for _, contract := range cfg.contracts() {
		candidate, ok := evaluateContract(newPrompt, ti.SubagentType, contract, cfg.WorkContext)
		if !ok {
			continue
		}
		newPrompt = candidate
	}
	if newPrompt == ti.Prompt {
		return passThrough()
	}
	return buildResponse(newPrompt, &ti)
}

func (c Config) contracts() []ContractConfig {
	if len(c.Contracts) > 0 {
		return c.Contracts
	}
	if c.ContractPath == "" && c.ContractContent == "" {
		return nil
	}
	return []ContractConfig{{Path: c.ContractPath, Content: c.ContractContent}}
}

type contractMetadata struct {
	phases          propagator.ContractPhases
	languages       []string
	activations     []string
	contextRequired bool
}

func evaluateContract(prompt, subagentType string, contract ContractConfig, workContext *WorkContext) (string, bool) {
	metadata, err := parseContractMetadata(contract.Content)
	if err != nil {
		return prompt, false
	}
	injHeader := metadata.phases.InjectionPoint
	if injHeader == "" {
		injHeader = defaultInjectionHeader
	}
	applySet := toSet(metadata.phases.AppliesTo)
	excludeSet := toSet(metadata.phases.Excluded)
	if excludeSet[subagentType] {
		return strip(prompt, contract.Path), true
	}
	if !applySet[subagentType] {
		return prompt, true
	}
	if metadata.contextRequired && !workContextMatches(workContext, metadata) {
		return prompt, true
	}
	return inject(prompt, contract.Path, injHeader), true
}

func parseContractMetadata(content string) (contractMetadata, error) {
	phases, err := propagator.ParseFrontmatter(content)
	if err != nil {
		return contractMetadata{}, err
	}
	metadata := contractMetadata{phases: phases}
	parts := strings.SplitN(content, "---", 3)
	if len(parts) < 3 {
		return contractMetadata{}, err
	}
	for _, line := range strings.Split(parts[1], "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "language_context:"):
			items, ok := parseStrictInlineList(strings.TrimPrefix(line, "language_context:"))
			if !ok {
				return contractMetadata{}, errors.New("malformed language_context")
			}
			metadata.languages = items
		case strings.HasPrefix(line, "activation_context:"):
			items, ok := parseStrictInlineList(strings.TrimPrefix(line, "activation_context:"))
			if !ok {
				return contractMetadata{}, errors.New("malformed activation_context")
			}
			metadata.activations = items
		case strings.HasPrefix(line, "context_operator:"):
			return contractMetadata{}, errors.New("unsupported context_operator")
		}
	}
	metadata.contextRequired = len(metadata.languages) > 0 || len(metadata.activations) > 0
	return metadata, nil
}

func parseStrictInlineList(raw string) ([]string, bool) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "[") || !strings.HasSuffix(raw, "]") {
		return nil, false
	}
	raw = strings.TrimPrefix(strings.TrimSuffix(raw, "]"), "[")
	var out []string
	for _, item := range strings.Split(raw, ",") {
		item = strings.Trim(strings.TrimSpace(item), `"'`)
		if item != "" {
			out = append(out, strings.ToLower(item))
		}
	}
	return out, true
}

func workContextMatches(workContext *WorkContext, metadata contractMetadata) bool {
	if workContext == nil || !workContext.Trusted {
		return false
	}
	if len(metadata.languages) > 0 && !intersects(metadata.languages, workContext.Languages) {
		return false
	}
	if len(metadata.activations) > 0 && !intersects(metadata.activations, workContext.Activations) {
		return false
	}
	if len(workContext.WorkKinds) == 0 {
		return false
	}
	if !containsFold(workContext.WorkKinds, "application-code") {
		return false
	}
	return true
}

func intersects(want, got []string) bool {
	for _, w := range want {
		if containsFold(got, w) {
			return true
		}
	}
	return false
}

func containsFold(items []string, want string) bool {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), want) {
			return true
		}
	}
	return false
}

// toSet converts a string slice to a lookup map.
func toSet(items []string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, item := range items {
		m[item] = true
	}
	return m
}

// canonicalEntry returns the exact line emitted and recognized by the gate for
// a given contract path. This is a BARE absolute path line — the contract path
// itself with no prefix. This matches the orchestrator's real format where skill
// paths are listed as bare lines under "## Skills to load before work".
//
// Exact trimmed-line matching (not substring) prevents collision with paths that
// contain the contract path as a substring (e.g. a backup path) and makes
// double-injection detection and strip both reliable.
func canonicalEntry(contractPath string) string {
	return contractPath
}

// hasExactEntry reports whether prompt contains the canonical entry line for
// contractPath as a line unto itself (trimmed match, not substring of a longer path).
func hasExactEntry(prompt, contractPath string) bool {
	entry := canonicalEntry(contractPath)
	for _, line := range strings.Split(prompt, "\n") {
		if strings.TrimSpace(line) == entry {
			return true
		}
	}
	return false
}

// hasExactHeader reports whether prompt contains the injection header as an
// exact line (trimmed). A header variant like '## Skills to load before work
// (extra context)' does NOT count as the exact header.
func hasExactHeader(prompt, injectionHeader string) bool {
	for _, line := range strings.Split(prompt, "\n") {
		if strings.TrimSpace(line) == injectionHeader {
			return true
		}
	}
	return false
}

// inject ensures that contractPath appears under the injectionHeader in prompt
// as a bare path line (the canonical entry format).
// If the exact header already exists, the entry is appended under it.
// If the exact header does not exist, it is added at the end of the prompt.
// Detection of "already present" uses exact line matching.
func inject(prompt, contractPath, injectionHeader string) string {
	entry := canonicalEntry(contractPath)

	// Already present (exact match) → no-op.
	if hasExactEntry(prompt, contractPath) {
		return prompt
	}

	if hasExactHeader(prompt, injectionHeader) {
		// Insert the entry right after the exact header line.
		lines := strings.Split(prompt, "\n")
		var out []string
		for _, line := range lines {
			out = append(out, line)
			if strings.TrimSpace(line) == injectionHeader {
				out = append(out, entry)
			}
		}
		return strings.Join(out, "\n")
	}

	// No exact header present — append header + entry.
	sep := "\n"
	if !strings.HasSuffix(prompt, "\n") {
		sep = "\n\n"
	} else if !strings.HasSuffix(prompt, "\n\n") {
		sep = "\n"
	}
	return prompt + sep + injectionHeader + "\n" + entry + "\n"
}

// strip removes the canonical entry line for contractPath from the prompt.
// Only the exact canonical entry (trimmed line match) is removed — lines that
// merely contain the contract path as a substring are left untouched. The
// injectionHeader is NOT removed (other skills may also live under it).
func strip(prompt, contractPath string) string {
	if !hasExactEntry(prompt, contractPath) {
		return prompt
	}
	entry := canonicalEntry(contractPath)
	lines := strings.Split(prompt, "\n")
	var out []string
	for _, line := range lines {
		if strings.TrimSpace(line) == entry {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// buildResponse constructs the Claude Code PreToolUse hook response that
// carries the modified prompt and echoes the FULL tool_input.
//
// CRITICAL: hookSpecificOutput must include hookEventName:"PreToolUse" and
// permissionDecision:"allow" — without these, Claude Code ignores updatedInput.
// updatedInput must echo ALL original tool_input fields (description,
// subagent_type, model-if-present) — Claude Code rejects the response if
// required parameters like description are missing.
func buildResponse(newPrompt string, ti *agentToolInput) (string, error) {
	ui := &updatedInput{
		Description:  ti.Description,
		Prompt:       newPrompt,
		SubagentType: ti.SubagentType,
		Model:        ti.Model,
	}
	resp := hookResponse{
		HookSpecificOutput: &hookSpecificOutput{
			HookEventName:      "PreToolUse",
			PermissionDecision: "allow",
			UpdatedInput:       ui,
		},
	}
	b, err := json.Marshal(resp)
	if err != nil {
		return passThrough()
	}
	return string(b), nil
}
