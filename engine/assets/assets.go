// Package assets embeds engine-owned managed content so the canonical text of
// generated guards lives in the engine binary rather than in an external,
// regenerable skill file.
//
// The skill-discovery-safety contract is the engine's authored compensating
// control for the fd/eza error-suppression footgun: REGISTRY-AUTHORITATIVE,
// FAIL-LOUD, and PORTABLE-DISCOVERY discipline injected into the phases that
// resolve skills or discover files.
package assets

import _ "embed"

// SkillDiscoverySafety is the embedded markdown for the skill-discovery-safety
// managed contract. It carries the frontmatter (applies_to_phases /
// excluded_phases / injection_point) the propagator and gate parse, plus the
// three fix-principle rules as agent-facing guidance.
//
//go:embed skill-discovery-safety.md
var SkillDiscoverySafety string
