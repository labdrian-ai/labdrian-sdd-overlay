// Package propagator ensures scoped contract rows are present in a consumer
// project's .atl/skill-registry.md using marker-delimited blocks so each row
// survives registry regeneration. Each managed contract owns a DISTINCT marker
// pair so multiple contracts can coexist without overwriting each other's block.
//
// The package names no contract. Which markers delimit the block of a contract
// and what its row is called are the Config the caller hands in (the composition
// root keeps the table of the contracts it manages), and a Config that leaves
// one out is refused (ErrInvalidConfig).
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

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/contract"
)

// ErrInvalidConfig is what Propagate returns, wrapped, for a Config that does not scope a block:
// a marker or the row label missing, or a marker that does not open (or close) a marker block.
var ErrInvalidConfig = errors.New("propagator: invalid config")

// The shapes of the lines that delimit a marker block in the registry, and the heading of
// the table a new row is appended to.
const (
	beginMarkerPrefix     = "<!-- BEGIN:"
	endMarkerPrefix       = "<!-- END:"
	sharedContractsHeader = "### Shared Contracts"
)

// Config says how the block of one contract is written into a registry: the
// path its row names, and the marker pair and row label that scope it. Every
// field but ContractPath is required.
type Config struct {
	// ContractPath is the relative path to the contract markdown file, as it
	// should appear in the registry table (e.g.
	// "skills/_shared/minimalism-contract.md").
	ContractPath string

	// BeginMarker / EndMarker delimit the regeneration-safe block this contract
	// owns: a line that starts with "<!-- BEGIN:" and one that starts with
	// "<!-- END:". Two contracts MUST have distinct pairs, or each would
	// overwrite the other's block.
	BeginMarker string
	EndMarker   string

	// RowLabel is the leading table-cell label for the generated row (the first
	// markdown cell), which is also how an unscoped row of the contract is found.
	RowLabel string
}

// validate says why cfg cannot scope a block, or returns nil.
func (c Config) validate() error {
	switch {
	case c.BeginMarker == "":
		return fmt.Errorf("%w: no begin marker", ErrInvalidConfig)
	case c.EndMarker == "":
		return fmt.Errorf("%w: no end marker", ErrInvalidConfig)
	case c.RowLabel == "":
		return fmt.Errorf("%w: no row label", ErrInvalidConfig)
	case !strings.HasPrefix(c.BeginMarker, beginMarkerPrefix):
		return fmt.Errorf("%w: the begin marker %q does not start with %q", ErrInvalidConfig, c.BeginMarker, beginMarkerPrefix)
	case !strings.HasPrefix(c.EndMarker, endMarkerPrefix):
		return fmt.Errorf("%w: the end marker %q does not start with %q", ErrInvalidConfig, c.EndMarker, endMarkerPrefix)
	}
	return nil
}

// buildScopedRow builds the marker-delimited block for cfg: the begin marker,
// the table row and the end marker. The description is derived from
// c.AppliesTo and c.Excluded, never hardcoded.
func buildScopedRow(cfg Config, c contract.Contract) string {
	injectList := strings.Join(c.AppliesTo, " and ")
	// Build the "do not inject into" part from excluded phases when present.
	doNotInject := ""
	if len(c.Excluded) > 0 {
		doNotInject = fmt.Sprintf(
			" Do NOT inject into %s.",
			strings.Join(c.Excluded, "/"),
		)
	}
	desc := fmt.Sprintf(
		"Inject ONLY into %s sub-agent prompts under '## Skills to load before work'.%s",
		injectList,
		doNotInject,
	)
	row := fmt.Sprintf("| %s | %s | %s |", cfg.RowLabel, cfg.ContractPath, desc)
	return strings.Join([]string{cfg.BeginMarker, row, cfg.EndMarker}, "\n")
}

// Propagate ensures registry contains the correctly scoped row of the contract
// cfg describes, inside its marker block. It returns:
//
//   - out: the (potentially modified) registry content
//   - changed: true when the content was modified
//   - err: non-nil only on hard failures, today a cfg that does not scope a
//     block (ErrInvalidConfig), with the registry returned as it came
//
// Idempotent: calling Propagate on already-correct output returns changed=false
// and an identical string.
func Propagate(registry string, cfg Config, c contract.Contract) (out string, changed bool, err error) {
	if err := cfg.validate(); err != nil {
		return registry, false, err
	}
	desiredBlock := buildScopedRow(cfg, c)
	begin := cfg.BeginMarker
	end := cfg.EndMarker
	label := cfg.RowLabel

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
	for scan := newLineScanner(strings.Split(registry, "\n")); scan.Next(); {
		if line := scan.Line(); !line.InBlock && isContractRow(line.Text, label) {
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

// replaceUnscopedRow replaces every unscoped table row for the given label
// (one that is NOT inside any marker block) with the new marker block, each
// by a whole block. Rows inside any marker block (foreign or ours) are left
// untouched.
func replaceUnscopedRow(registry, newBlock, label string) string {
	lines := strings.Split(registry, "\n")
	out := make([]string, 0, len(lines))
	for scan := newLineScanner(lines); scan.Next(); {
		line := scan.Line()
		if !line.InBlock && isContractRow(line.Text, label) {
			out = append(out, newBlock)
			continue
		}
		out = append(out, line.Text)
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
	lastTableRowIdx := -1

	for scan := newLineScanner(lines); scan.Next(); {
		line := scan.Line()

		if line.Closes {
			// Record the END marker line itself as the last known position so
			// we can insert after a closing marker that follows a section table.
			if inSharedContracts {
				lastTableRowIdx = line.Index
			}
			continue
		}

		if strings.HasPrefix(line.Trimmed, sharedContractsHeader) {
			inSharedContracts = true
		}
		if !inSharedContracts || line.InBlock {
			continue
		}
		if strings.HasPrefix(line.Trimmed, "|") {
			lastTableRowIdx = line.Index
		}
		// Stop at the next section heading (after we've entered Shared Contracts).
		if strings.HasPrefix(line.Trimmed, "#") && !strings.HasPrefix(line.Trimmed, sharedContractsHeader) {
			break
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
