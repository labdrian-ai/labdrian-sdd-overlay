// Package gate implements the fail-safe policy of the gate-task subcommand for the
// deterministic-scoping engine.
//
// gate-task reads a Claude Code PreToolUse 'Agent' tool call from STDIN (engine/hookwire reads
// it, and writes the answer: this package knows no JSON), and the gate decides, from the type
// of the sub-agent and its prompt:
//   - INJECT the minimalism-contract path into the sub-agent prompt when the sub-agent type is
//     in the applies_to_phases set from contract frontmatter.
//   - STRIP the minimalism-contract path when the sub-agent type is in the excluded_phases set
//     and the path is present.
//   - LEAVE THE PROMPT UNCHANGED on any unknown type, empty prompt, or broken frontmatter.
//
// VERIFIED REALITY (Claude Code 2.1.185), which engine/hookwire encodes: the sub-agent spawn
// tool is named "Agent", NOT "Task"; a PreToolUse hook on "Agent" DOES fire and updatedInput
// DOES rewrite the sub-agent prompt, provided it echoes the FULL tool_input and carries
// hookEventName "PreToolUse" and permissionDecision "allow".
//
// The canonical injected entry is a BARE absolute path line. Exact trimmed-line matching
// prevents double-injection and makes strip work correctly.
//
// CRITICAL SAFETY: the gate MUST be fail-safe. On ANY error the command answers with a
// pass-through that leaves tool_input UNCHANGED and exits 0. It must NEVER block an Agent call,
// NEVER crash, NEVER deny.
package gate

import (
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/contract"
)

// workKindApplicationCode is the one kind of work a context-aware contract is injected
// for: the languages and activations of a contract describe application code.
const workKindApplicationCode = "application-code"

// Config holds the runtime configuration for the gate processor.
type Config struct {
	// Contracts are the managed guidance contracts, each evaluated independently, in
	// order, against the same prompt.
	Contracts []ContractConfig

	// WorkContext is trusted work metadata supplied by the runtime adapter. It is
	// the only source used for context-aware contracts; prompt text is ignored.
	WorkContext *WorkContext
}

// ContractConfig describes one managed guidance contract.
type ContractConfig struct {
	// Path is the path of the contract as it appears in skill prompts: the bare line
	// that is injected or stripped. Example:
	// "/home/user/.claude/skills/_shared/minimalism-contract.md"
	Path string
	// Content is the raw content of the contract document, whose frontmatter says which
	// phases it applies to and under which heading it is injected (engine/contract).
	Content string
}

// WorkContext is explicit metadata about the current unit of work.
type WorkContext struct {
	Trusted     bool     `json:"trusted"`
	Languages   []string `json:"languages"`
	Activations []string `json:"activations"`
	WorkKinds   []string `json:"work_kinds"`
}

// Call is the sub-agent spawn the gate decides about: the kind of sub-agent and the prompt it
// is given. Nothing else of the tool call matters to the gate; the adapter that reads the call
// keeps the rest, to echo it.
type Call struct {
	// SubagentType is the kind of sub-agent, such as sdd-apply.
	SubagentType string
	// Prompt is the prompt the sub-agent is given.
	Prompt string
}

// Rewrite applies the managed contracts of cfg to call and returns the prompt the sub-agent
// should be given, and whether it differs from call.Prompt. It never fails: a call it can do
// nothing with (no sub-agent type, no prompt), a type no contract names and a contract that
// cannot be read all leave the prompt as it is, and every contract that can be read is still
// applied.
func Rewrite(call Call, cfg Config) (string, bool) {
	// The sub-agent type must be non-empty, and the prompt present, to take any action.
	if call.SubagentType == "" || call.Prompt == "" {
		return call.Prompt, false
	}

	newPrompt := call.Prompt
	for _, managed := range cfg.Contracts {
		candidate, ok := evaluateContract(newPrompt, call.SubagentType, managed, cfg.WorkContext)
		if !ok {
			continue
		}
		newPrompt = candidate
	}
	return newPrompt, newPrompt != call.Prompt
}

// evaluateContract applies one managed contract to the prompt of a sub-agent of the given
// type: it strips the contract from a phase it excludes, injects it into a phase it applies
// to when the work matches what it asks for, and otherwise leaves the prompt alone. It
// reports false when the contract cannot be read, so that contract is skipped and every
// other is still applied.
func evaluateContract(prompt, subagentType string, managed ContractConfig, workContext *WorkContext) (string, bool) {
	c, needs, err := contract.ParseBoth(managed.Content)
	if err != nil {
		return prompt, false
	}
	if c.ExcludesPhase(subagentType) {
		return strip(prompt, managed.Path), true
	}
	if !c.AppliesToPhase(subagentType) {
		return prompt, true
	}
	if needs.ContextRequired() && !workContextMatches(workContext, needs) {
		return prompt, true
	}
	return inject(prompt, managed.Path, c.Header()), true
}

func workContextMatches(workContext *WorkContext, c contract.Context) bool {
	if workContext == nil || !workContext.Trusted {
		return false
	}
	if len(c.LanguageContext) > 0 && !intersects(c.LanguageContext, workContext.Languages) {
		return false
	}
	if len(c.ActivationContext) > 0 && !intersects(c.ActivationContext, workContext.Activations) {
		return false
	}
	if len(workContext.WorkKinds) == 0 {
		return false
	}
	if !containsFold(workContext.WorkKinds, workKindApplicationCode) {
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
