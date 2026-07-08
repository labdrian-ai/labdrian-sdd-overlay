// Package assets embeds engine-owned managed content so the canonical text of
// generated guards lives in the engine binary rather than in an external,
// regenerable skill file.
//
// The skill-discovery-safety contract is the engine's authored compensating
// control for the fd/eza error-suppression footgun: REGISTRY-AUTHORITATIVE,
// FAIL-LOUD, and PORTABLE-DISCOVERY discipline injected into the phases that
// resolve skills or discover files.
//
// The anti-generic-design contract is the engine's compensating control for
// the model's default "Claude/SaaS look" design bias (Inter, violet-blue
// gradients, generic shadow cards, flat 3-column grids). It will be
// auto-injected into the phases that generate or refine UI/design output
// once wired by embeddedContract() (see PR-2 of the
// anti-generic-design-runtime-wiring chain).
package assets

import _ "embed"

// SkillDiscoverySafety is the embedded markdown for the skill-discovery-safety
// managed contract. It carries the frontmatter (applies_to_phases /
// excluded_phases / injection_point) the propagator and gate parse, plus the
// three fix-principle rules as agent-facing guidance.
//
//go:embed skill-discovery-safety.md
var SkillDiscoverySafety string

// AntiGenericDesign is the embedded markdown for the anti-generic-design
// managed contract. It carries the frontmatter (applies_to_phases /
// excluded_phases / injection_point) the propagator and gate parse, plus the
// forbidden-pattern list and self-critique checklist distilled from
// skills/anti-generic-design/SKILL.md as agent-facing guidance.
//
//go:embed anti-generic-design.md
var AntiGenericDesign string
