// Package assets embeds engine-owned managed content so the canonical text of
// generated guards lives in the engine binary rather than in an external,
// regenerable skill file.
//
// The anti-generic-design contract is the engine's compensating control for
// the model's default "Claude/SaaS look" design bias (Inter, violet-blue
// gradients, generic shadow cards, flat 3-column grids). The composition root
// hands it to the verbs that inject managed contracts (`engine propagate
// --embedded-contract anti-generic-design` and `engine gate-task`), and the
// OpenCode adapter reads it for the prompt it generates.
//
// The embedded text is handed out through functions and never as an exported
// variable, so no caller can assign over what every other caller reads.
package assets

import _ "embed"

//go:embed anti-generic-design.md
var antiGenericDesign string

// AntiGenericDesign is the embedded markdown for the anti-generic-design
// managed contract. It carries the frontmatter (applies_to_phases /
// excluded_phases / injection_point) that engine/contract parses, plus the
// forbidden-pattern list and self-critique checklist distilled from
// skills/anti-generic-design/SKILL.md as agent-facing guidance.
func AntiGenericDesign() string { return antiGenericDesign }
