package main

// The contracts the verbs 'propagate' and 'gate-task' manage: the table of them, where each one is
// read from, and how its block is delimited in the registry.

import (
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/assets"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/status"
)

// The names and default registry paths of the contracts the verbs manage. The minimalism
// contract is the one read from a file by default; anti-generic-design ships in the binary.
const (
	minimalismContract           = "minimalism-contract"
	defaultContractPath          = "skills/_shared/minimalism-contract.md"
	embeddedAntiGenericDesign    = "anti-generic-design"
	defaultAntiGenericDesignPath = "skills/_shared/anti-generic-design.md"
)

// managedContract is one contract the verbs manage, with what the registry keeps of it. The domain
// that writes the registry (engine/propagator) and the check of it (engine/status) name no
// contract; this table is the one place that says which there are and how each is delimited, and
// both are handed what they need from it.
type managedContract struct {
	// name is the label of the row of the contract in the registry, the first cell of it, and the
	// name its markers carry.
	name string
	// defaultPath is the Path cell of the row, and the bare injected line, when the caller does not
	// override --contract-path. It is where the overlay deploys the standalone copy of the contract.
	defaultPath string
	// content is the canonical text the binary carries, for a contract the engine owns. Nil for a
	// contract whose text is read from a file the caller names.
	content func() string
}

// scopeName is the name the markers of the contract carry.
func (c managedContract) scopeName() string { return c.name + "-scope" }

// beginMarker and endMarker delimit the block the contract owns. Each contract owns a DISTINCT
// pair, or propagating one would overwrite the other's block. The text is what registries in the
// field already hold.
func (c managedContract) beginMarker() string {
	return "<!-- BEGIN: " + c.scopeName() + " (auto-generated) -->"
}

func (c managedContract) endMarker() string { return "<!-- END: " + c.scopeName() + " -->" }

// config says how the contract is written into a registry, naming contractPath in its row.
func (c managedContract) config(contractPath string) propagator.Config {
	return propagator.Config{
		ContractPath: contractPath,
		BeginMarker:  c.beginMarker(),
		EndMarker:    c.endMarker(),
		RowLabel:     c.name,
	}
}

// minimalism is the contract read from a file; antiGenericDesign is the one the engine ships.
func minimalism() managedContract {
	return managedContract{name: minimalismContract, defaultPath: defaultContractPath}
}

func antiGenericDesign() managedContract {
	return managedContract{name: embeddedAntiGenericDesign, defaultPath: defaultAntiGenericDesignPath, content: assets.AntiGenericDesign}
}

// managedContracts lists them in the order the registry check of 'status' names a missing block.
// Adding a second contract the engine ships here is the supported extension point: the engine
// carries the canonical text, so the guard propagates on every install with no dependency on an
// external, regenerable skill file.
func managedContracts() []managedContract {
	return []managedContract{minimalism(), antiGenericDesign()}
}

// embeddedContract resolves a named engine-owned managed contract, one whose text the binary
// carries. Returns ok=false for any other name so callers can fail loud.
func embeddedContract(name string) (contract managedContract, ok bool) {
	for _, c := range managedContracts() {
		if c.name == name && c.content != nil {
			return c, true
		}
	}
	return managedContract{}, false
}

// scopedBlocks are the blocks 'status' expects a registry to hold: one for each managed
// contract, found by the marker that opens it and named in a report by the name the marker carries.
func scopedBlocks() []status.ScopedBlock {
	var blocks []status.ScopedBlock
	for _, c := range managedContracts() {
		blocks = append(blocks, status.ScopedBlock{Name: c.scopeName(), BeginMarker: c.beginMarker()})
	}
	return blocks
}
