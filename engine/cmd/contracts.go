package main

// The contracts the verbs 'propagate' and 'gate-task' manage: where the default one is read from, and the one that ships in the binary.

import (
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/assets"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator"
)

// The names and default registry paths of the contracts the verbs manage. The minimalism
// contract is the one read from a file by default; anti-generic-design ships in the binary.
const (
	defaultContractPath          = "skills/_shared/minimalism-contract.md"
	embeddedAntiGenericDesign    = "anti-generic-design"
	defaultAntiGenericDesignPath = "skills/_shared/anti-generic-design.md"
)

// embeddedContract resolves a named engine-owned managed contract to its content
// and (for propagate) the distinct marker pair + row label that scope its block.
// Returns ok=false for an unknown name so callers can fail loud.
//
// Adding a second managed contract here is the supported extension point: the
// engine ships the canonical text, so the guard propagates on every install with
// no dependency on an external, regenerable skill file.
func embeddedContract(name string) (spec embeddedContractSpec, ok bool) {
	switch name {
	case embeddedAntiGenericDesign:
		return embeddedContractSpec{
			content:     assets.AntiGenericDesign,
			beginMarker: propagator.AntiGenericDesignBeginMarker,
			endMarker:   propagator.AntiGenericDesignEndMarker,
			rowLabel:    embeddedAntiGenericDesign,
			// defaultPath is the registry-row Path cell / bare injected line when
			// the caller does not override --contract-path. It is where the
			// overlay deploys the standalone copy of this contract.
			defaultPath: defaultAntiGenericDesignPath,
		}, true
	default:
		return embeddedContractSpec{}, false
	}
}

// embeddedContractSpec bundles the resolved attributes of an engine-owned
// managed contract.
type embeddedContractSpec struct {
	content     string
	beginMarker string
	endMarker   string
	rowLabel    string
	defaultPath string
}
