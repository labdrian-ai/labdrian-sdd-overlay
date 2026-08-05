// Package propagator ensures scoped contract rows are present in a consumer
// project's .atl/skill-registry.md using marker-delimited blocks so each row
// survives registry regeneration. Each managed contract owns a DISTINCT marker
// pair so multiple contracts can coexist without overwriting each other's block.
//
// Regeneration-safety contract:
//
//	Each contract's row is wrapped in its own BEGIN/END pair, e.g.:
//	  <!-- BEGIN: minimalism-contract-scope (auto-generated) -->
//	  ...row...
//	  <!-- END: minimalism-contract-scope -->
//
// Regeneration tools that respect the BEGIN/END convention replace each block
// content in place rather than appending. Any tool that fully rewrites the
// registry must preserve or re-run the propagators for all managed contracts.
package propagator

import (
	"errors"
	"fmt"
	"strings"
)

// Markers wrapping the scoped minimalism-contract row. These remain the package
// defaults so existing callers and tests keep working; Config may override them
// when a second contract (e.g. skill-discovery-safety) needs its own block.
const (
	BeginMarker = "<!-- BEGIN: minimalism-contract-scope (auto-generated) -->"
	EndMarker   = "<!-- END: minimalism-contract-scope -->"
)

// Markers wrapping the scoped skill-discovery-safety row. A DISTINCT marker pair
// is mandatory: if the second contract reused minimalism-contract-scope, the two
// propagators would fight over the same BEGIN/END block in the registry.
const (
	DiscoverySafetyBeginMarker = "<!-- BEGIN: skill-discovery-safety-scope (auto-generated) -->"
	DiscoverySafetyEndMarker   = "<!-- END: skill-discovery-safety-scope -->"
)

// Markers wrapping the scoped anti-generic-design row. A DISTINCT marker pair
// is mandatory: reusing either the minimalism-contract or skill-discovery-safety
// pair would make Propagate overwrite that contract's block instead of the two
// coexisting (see package doc).
const (
	AntiGenericDesignBeginMarker = "<!-- BEGIN: anti-generic-design-scope (auto-generated) -->"
	AntiGenericDesignEndMarker   = "<!-- END: anti-generic-design-scope -->"
)

// Markers wrapping the scoped review-projection-contract row. A DISTINCT marker
// pair is mandatory for the same reason as the three pairs above: reusing any of
// them would make Propagate overwrite that contract's block instead of the four
// coexisting (see package doc).
const (
	ReviewProjectionBeginMarker = "<!-- BEGIN: review-projection-contract-scope (auto-generated) -->"
	ReviewProjectionEndMarker   = "<!-- END: review-projection-contract-scope -->"
)

// defaultRowLabel is the leading table-cell label used when Config.RowLabel is
// empty, preserving the original minimalism-contract behavior.
const defaultRowLabel = "minimalism-contract"

// ContractPhases holds the phase scope parsed from the contract frontmatter.
type ContractPhases struct {
	AppliesTo      []string
	Excluded       []string
	InjectionPoint string // from injection_point frontmatter key; may be empty
}

// Config holds the contract path used in the generated registry row, plus the
// marker pair and row label that scope the block. The marker/label fields are
// optional: when empty they default to the minimalism-contract values so the
// original single-contract behavior is unchanged.
type Config struct {
	// ContractPath is the relative path to the contract markdown file, as it
	// should appear in the registry table (e.g.
	// "skills/_shared/minimalism-contract.md").
	ContractPath string

	// BeginMarker / EndMarker delimit the regeneration-safe block this contract
	// owns. When empty they default to the package BeginMarker / EndMarker
	// (minimalism-contract-scope). A second contract MUST set a distinct pair
	// (e.g. DiscoverySafetyBeginMarker / DiscoverySafetyEndMarker) so the two
	// propagators do not overwrite each other's block.
	BeginMarker string
	EndMarker   string

	// RowLabel is the leading table-cell label for the generated row (the first
	// markdown cell). When empty it defaults to "minimalism-contract".
	RowLabel string
}

// begin returns the effective BEGIN marker, falling back to the package default.
func (c Config) begin() string {
	if c.BeginMarker != "" {
		return c.BeginMarker
	}
	return BeginMarker
}

// end returns the effective END marker, falling back to the package default.
func (c Config) end() string {
	if c.EndMarker != "" {
		return c.EndMarker
	}
	return EndMarker
}

// rowLabel returns the effective row label, falling back to the package default.
func (c Config) rowLabel() string {
	if c.RowLabel != "" {
		return c.RowLabel
	}
	return defaultRowLabel
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
// that form the regeneration-safe block for the default minimalism-contract
// markers and label. The description is derived from phases.AppliesTo — never
// hardcoded. Exported for testing; backward-compatible signature.
func BuildScopedRow(contractPath string, phases ContractPhases) string {
	return buildScopedRowWith(Config{ContractPath: contractPath}, phases)
}

// buildScopedRowWith builds the marker-delimited block using the marker pair and
// row label resolved from cfg (with package defaults when unset).
func buildScopedRowWith(cfg Config, phases ContractPhases) string {
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
	row := fmt.Sprintf("| %s | %s | %s |", cfg.rowLabel(), cfg.ContractPath, desc)
	return strings.Join([]string{cfg.begin(), row, cfg.end()}, "\n")
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
	desiredBlock := buildScopedRowWith(cfg, phases)
	begin := cfg.begin()
	end := cfg.end()
	label := cfg.rowLabel()

	// Case 1: the registry already contains the exact desired block → no-op.
	if strings.Contains(registry, desiredBlock) {
		return registry, false, nil
	}

	// Case 2: the registry has a stale/incorrect marker block (THIS contract's
	// own marker pair) → replace it. Foreign marker pairs are left untouched.
	if strings.Contains(registry, begin) {
		updated := replaceBlock(registry, desiredBlock, begin, end)
		return updated, updated != registry, nil
	}

	// Case 3: no marker block exists yet.
	// Sub-case 3a: an unscoped row for this contract's label exists → replace it.
	if hasUnscopedRow(registry, label) {
		updated := replaceUnscopedRow(registry, desiredBlock, label)
		return updated, true, nil
	}

	// Sub-case 3b: no row at all → append inside the Shared Contracts table.
	updated := appendToSharedContracts(registry, desiredBlock)
	return updated, true, nil
}

// replaceBlock replaces everything between the given BEGIN and END markers with
// the new block content. The marker pair is passed in so a second contract can
// operate on its own distinct block without disturbing other blocks.
func replaceBlock(registry, newBlock, beginMarker, endMarker string) string {
	start := strings.Index(registry, beginMarker)
	end := strings.Index(registry, endMarker)
	if start == -1 || end == -1 || end < start {
		return registry
	}
	end += len(endMarker)
	return registry[:start] + newBlock + registry[end:]
}

// hasUnscopedRow reports whether the registry has a table row referencing the
// given contract label that is NOT inside any marker block (neither ours nor a
// foreign one).
func hasUnscopedRow(registry, label string) bool {
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
		if !inAnyBlock && isContractRow(line, label) {
			return true
		}
	}
	return false
}

// isContractRow returns true when line is a markdown table row whose FIRST data
// cell matches label exactly. Anchoring to the first cell prevents a false
// positive when label appears only in a description or path cell.
func isContractRow(line, label string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "|") || strings.HasPrefix(trimmed, "<!--") {
		return false
	}
	// Split on "|" and inspect the first non-empty segment (the first data cell).
	// A standard markdown table row looks like: | cell1 | cell2 | cell3 |
	// strings.Split("|a|b|", "|") → ["", "a", "b", ""]
	cells := strings.Split(trimmed, "|")
	for _, cell := range cells {
		cell = strings.TrimSpace(cell)
		if cell == "" {
			continue
		}
		// The first non-empty cell is the label cell.
		return cell == label
	}
	return false
}

// replaceUnscopedRow replaces the first unscoped table row for the given label
// (one that is NOT inside any marker block) with the new marker block.
// Rows inside any marker block (foreign or ours) are left untouched.
func replaceUnscopedRow(registry, newBlock, label string) string {
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
		if !inAnyBlock && isContractRow(line, label) {
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
