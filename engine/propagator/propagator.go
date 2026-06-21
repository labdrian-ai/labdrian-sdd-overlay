// Package propagator ensures a minimalism-contract scoped row is present in
// a consumer project's .atl/skill-registry.md using a marker-delimited block
// so the row survives registry regeneration.
//
// Regeneration-safety contract:
//
//	The row is wrapped in:
//	  <!-- BEGIN: minimalism-contract-scope (auto-generated) -->
//	  ...row...
//	  <!-- END: minimalism-contract-scope -->
//
// Regeneration tools that respect the BEGIN/END convention replace the block
// content in place rather than appending. Any tool that fully rewrites the
// registry must preserve or re-run this propagator.
package propagator

import (
	"errors"
	"fmt"
	"strings"
)

// Markers wrapping the scoped minimalism-contract row.
const (
	BeginMarker = "<!-- BEGIN: minimalism-contract-scope (auto-generated) -->"
	EndMarker   = "<!-- END: minimalism-contract-scope -->"
)

// ContractPhases holds the phase scope parsed from the contract frontmatter.
type ContractPhases struct {
	AppliesTo      []string
	Excluded       []string
	InjectionPoint string // from injection_point frontmatter key; may be empty
}

// Config holds the contract path used in the generated registry row.
type Config struct {
	// ContractPath is the relative path to the minimalism-contract markdown
	// file, as it should appear in the registry table (e.g.
	// "skills/_shared/minimalism-contract.md").
	ContractPath string
}

// ParseFrontmatter extracts phase scope from YAML-like frontmatter.
//
// Expected format:
//
//	---
//	applies_to_phases: [sdd-tasks, sdd-apply]
//	excluded_phases: [...]
//	...
//	---
//
// Returns an error if applies_to_phases is absent or empty — callers must not
// silently continue without knowing which phases the contract targets.
func ParseFrontmatter(content string) (ContractPhases, error) {
	// Locate the frontmatter block between the first two "---" delimiters.
	parts := strings.SplitN(content, "---", 3)
	if len(parts) < 3 {
		return ContractPhases{}, errors.New(
			"contract file has no YAML frontmatter (expected content between --- delimiters)")
	}
	fm := parts[1]

	var phases ContractPhases
	for _, line := range strings.Split(fm, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "applies_to_phases:") {
			phases.AppliesTo = parseInlineList(strings.TrimPrefix(line, "applies_to_phases:"))
		}
		if strings.HasPrefix(line, "excluded_phases:") {
			phases.Excluded = parseInlineList(strings.TrimPrefix(line, "excluded_phases:"))
		}
		if strings.HasPrefix(line, "injection_point:") {
			raw := strings.TrimSpace(strings.TrimPrefix(line, "injection_point:"))
			// Strip surrounding quotes if present.
			raw = strings.Trim(raw, `"'`)
			phases.InjectionPoint = raw
		}
	}

	if len(phases.AppliesTo) == 0 {
		return ContractPhases{}, errors.New(
			"contract frontmatter missing or empty applies_to_phases: " +
				"cannot derive scope without knowing which phases to inject into")
	}
	return phases, nil
}

// parseInlineList parses a YAML inline sequence like "[a, b, c]" into a slice.
func parseInlineList(raw string) []string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "[")
	raw = strings.TrimSuffix(raw, "]")
	var out []string
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

// BuildScopedRow returns the three lines (begin-marker, table-row, end-marker)
// that form the regeneration-safe block. The description is derived from
// phases.AppliesTo — never hardcoded. Exported for testing.
func BuildScopedRow(contractPath string, phases ContractPhases) string {
	injectList := strings.Join(phases.AppliesTo, " and ")
	// Build the "do not inject into" part from excluded phases when present.
	doNotInject := ""
	if len(phases.Excluded) > 0 {
		doNotInject = fmt.Sprintf(
			" Do NOT inject into %s.",
			strings.Join(phases.Excluded, "/"),
		)
	}
	desc := fmt.Sprintf(
		"Inject ONLY into %s sub-agent prompts under '## Skills to load before work'.%s",
		injectList,
		doNotInject,
	)
	row := fmt.Sprintf("| minimalism-contract | %s | %s |", contractPath, desc)
	return strings.Join([]string{BeginMarker, row, EndMarker}, "\n")
}

// Propagate ensures registry contains a correctly scoped minimalism-contract
// row inside a marker block. It returns:
//
//   - out: the (potentially modified) registry content
//   - changed: true when the content was modified
//   - err: non-nil only on hard failures
//
// Idempotent: calling Propagate on already-correct output returns changed=false
// and an identical string.
func Propagate(registry string, cfg Config, phases ContractPhases) (out string, changed bool, err error) {
	desiredBlock := BuildScopedRow(cfg.ContractPath, phases)

	// Case 1: the registry already contains the exact desired block → no-op.
	if strings.Contains(registry, desiredBlock) {
		return registry, false, nil
	}

	// Case 2: the registry has a stale/incorrect marker block → replace it.
	if strings.Contains(registry, BeginMarker) {
		updated := replaceBlock(registry, desiredBlock)
		return updated, updated != registry, nil
	}

	// Case 3: no marker block exists yet.
	// Sub-case 3a: an unscoped row for minimalism-contract exists → replace it.
	if hasUnscopedRow(registry) {
		updated := replaceUnscopedRow(registry, desiredBlock)
		return updated, true, nil
	}

	// Sub-case 3b: no row at all → append inside the Shared Contracts table.
	updated := appendToSharedContracts(registry, desiredBlock)
	return updated, true, nil
}

// replaceBlock replaces everything between the BEGIN and END markers with the
// new block content.
func replaceBlock(registry, newBlock string) string {
	start := strings.Index(registry, BeginMarker)
	end := strings.Index(registry, EndMarker)
	if start == -1 || end == -1 || end < start {
		return registry
	}
	end += len(EndMarker)
	return registry[:start] + newBlock + registry[end:]
}

// hasUnscopedRow reports whether the registry has a minimalism-contract table
// row that is NOT inside any marker block (neither ours nor a foreign one).
func hasUnscopedRow(registry string) bool {
	lines := strings.Split(registry, "\n")
	inAnyBlock := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "<!-- BEGIN:") {
			inAnyBlock = true
		}
		if strings.HasPrefix(trimmed, "<!-- END:") {
			inAnyBlock = false
		}
		if !inAnyBlock && isMinimalismContractRow(line) {
			return true
		}
	}
	return false
}

// isMinimalismContractRow returns true when line is a markdown table row
// that references minimalism-contract (but is not a marker comment).
func isMinimalismContractRow(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "|") &&
		strings.Contains(trimmed, "minimalism-contract") &&
		!strings.HasPrefix(trimmed, "<!--")
}

// replaceUnscopedRow replaces the first unscoped minimalism-contract table row
// (one that is NOT inside any marker block) with the new marker block.
// Rows inside any marker block (foreign or ours) are left untouched.
func replaceUnscopedRow(registry, newBlock string) string {
	lines := strings.Split(registry, "\n")
	inAnyBlock := false
	var out []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "<!-- BEGIN:") {
			inAnyBlock = true
		}
		if strings.HasPrefix(trimmed, "<!-- END:") {
			inAnyBlock = false
			out = append(out, line)
			continue
		}
		if !inAnyBlock && isMinimalismContractRow(line) {
			out = append(out, newBlock)
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// appendToSharedContracts appends the marker block inside the
// "### Shared Contracts" section's table. If no such section exists, it
// appends the block at the end of the file.
//
// Lines inside any foreign marker block (BEGIN: ... / END: ...) are skipped
// so that inserting after the last bare table row never splits a foreign block.
func appendToSharedContracts(registry, newBlock string) string {
	// Find the end of the Shared Contracts table by looking for the last
	// bare table row in that section (skipping rows inside any marker block).
	lines := strings.Split(registry, "\n")
	inSharedContracts := false
	inAnyMarkerBlock := false
	lastTableRowIdx := -1

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Track entry/exit of ANY marker block (foreign or ours).
		if strings.HasPrefix(trimmed, "<!-- BEGIN:") {
			inAnyMarkerBlock = true
		}
		if strings.HasPrefix(trimmed, "<!-- END:") {
			inAnyMarkerBlock = false
			// Record the END marker line itself as the last known position so
			// we can insert after a closing marker that follows a section table.
			if inSharedContracts {
				lastTableRowIdx = i
			}
			continue
		}

		if strings.HasPrefix(trimmed, "### Shared Contracts") {
			inSharedContracts = true
		}
		if inSharedContracts && !inAnyMarkerBlock {
			if strings.HasPrefix(trimmed, "|") {
				lastTableRowIdx = i
			}
			// Stop at the next section heading (after we've entered Shared Contracts).
			if i > 0 && strings.HasPrefix(trimmed, "#") &&
				!strings.HasPrefix(trimmed, "### Shared Contracts") {
				break
			}
		}
	}

	if lastTableRowIdx >= 0 {
		// Insert after the last table row (or END marker).
		result := make([]string, 0, len(lines)+4)
		result = append(result, lines[:lastTableRowIdx+1]...)
		result = append(result, newBlock)
		result = append(result, lines[lastTableRowIdx+1:]...)
		return strings.Join(result, "\n")
	}

	// Fallback: append at end.
	return registry + "\n" + newBlock + "\n"
}
