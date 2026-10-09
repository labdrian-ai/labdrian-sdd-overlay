package core

import (
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/contract"
)

// The prompt rules decide what a runtime does to the prompt of a phase: put the path of a
// contract under its injection header when the contract applies to the phase, take it out when
// the contract excludes the phase, and leave the prompt alone otherwise. They work on text and on
// a parsed contract, and read nothing.

// MutatePrompt applies the contract to the prompt of a phase: it injects the contract path under
// the injection header when the contract applies to the phase, strips it when the contract
// excludes the phase, and leaves the prompt as it is for a phase the contract does not name. The
// bool says whether the prompt changed.
func MutatePrompt(prompt, phase, contractPath string, c contract.Contract) (string, bool) {
	switch {
	case c.AppliesToPhase(phase):
		mutated := InjectPrompt(prompt, contractPath, c.Header())
		return mutated, mutated != prompt
	case c.ExcludesPhase(phase):
		mutated := StripPrompt(prompt, contractPath)
		return mutated, mutated != prompt
	default:
		return prompt, false
	}
}

// InjectPrompt puts the contract path under the injection header, adding the header at the end of
// the prompt when it has none, and does nothing when the path is already a line of the prompt.
func InjectPrompt(prompt, contractPath, injectionHeader string) string {
	entry := CanonicalEntry(contractPath)
	if HasExactEntry(prompt, contractPath) {
		return prompt
	}
	if HasExactHeader(prompt, injectionHeader) {
		lines := strings.Split(prompt, "\n")
		out := make([]string, 0, len(lines)+1)
		for _, line := range lines {
			out = append(out, line)
			if strings.TrimSpace(line) == injectionHeader {
				out = append(out, entry)
			}
		}
		return strings.Join(out, "\n")
	}
	sep := "\n"
	if !strings.HasSuffix(prompt, "\n") {
		sep = "\n\n"
	} else if !strings.HasSuffix(prompt, "\n\n") {
		sep = "\n"
	}
	return prompt + sep + injectionHeader + "\n" + entry + "\n"
}

// StripPrompt removes every line of the prompt that is the contract path.
func StripPrompt(prompt, contractPath string) string {
	if !HasExactEntry(prompt, contractPath) {
		return prompt
	}
	entry := CanonicalEntry(contractPath)
	lines := strings.Split(prompt, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == entry {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// CanonicalEntry is the line the contract is written as in a prompt: its path.
func CanonicalEntry(contractPath string) string {
	return contractPath
}

// HasExactEntry reports whether the contract path is a line of the prompt, ignoring surrounding
// space.
func HasExactEntry(prompt, contractPath string) bool {
	entry := CanonicalEntry(contractPath)
	for _, line := range strings.Split(prompt, "\n") {
		if strings.TrimSpace(line) == entry {
			return true
		}
	}
	return false
}

// HasExactHeader reports whether the injection header is a line of the prompt, ignoring
// surrounding space.
func HasExactHeader(prompt, injectionHeader string) bool {
	for _, line := range strings.Split(prompt, "\n") {
		if strings.TrimSpace(line) == injectionHeader {
			return true
		}
	}
	return false
}
