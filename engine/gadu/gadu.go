// Package gadu implements the GADU persona generator.
// It reads the canonical persona body (engine/gadu/persona/body.md) and emits
// four delivery artifacts:
//   - agents/GADU.md                  (Claude Code agent file)
//   - opencode/agents/GADU.md         (OpenCode agent file)
//   - pi/agents/GADU.md               (Pi native subagent file, compact body)
//   - skills/gadu-operator/SKILL.md   (portable overlay skill)
//
// agents/GADU.md, opencode/agents/GADU.md, and skills/gadu-operator/SKILL.md
// carry identical persona body content. pi/agents/GADU.md is the one
// exception: it carries a deliberately compact body (see PiAgentModel's doc
// comment for why) that defers the full persona to the gadu-operator skill.
// None of these artifacts are ever hand-edited. Use Generate to write them;
// use Check to assert they are not stale.
package gadu

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed persona/body.md
var personaBody string

// agentFrontmatter is the generator-owned template for agents/GADU.md (D7).
// Matching the existing hand-made Claude Code file so @GADU invocation works identically.
const agentFrontmatter = `---
name: GADU
description: High-judgment operator agent — invoke by name. Opinionated, red-teams the user's reasoning, recommends the highest-odds option, orchestrates sub-agents as lead, grounds claims in verified sources. Not a warm assistant.
model: opus
tools: '*'
---`

// opencodeAgentFrontmatter is the generator-owned template for
// opencode/agents/GADU.md. OpenCode requires provider-prefixed model IDs and
// rejects Claude Code's `tools: '*'` frontmatter, so this native file keeps the
// same persona body while using OpenCode's agent schema.
const opencodeAgentFrontmatter = `---
description: High-judgment operator agent — invoke by name. Opinionated, red-teams the user's reasoning, recommends the highest-odds option, orchestrates sub-agents as lead, grounds claims in verified sources. Not a warm assistant.
mode: all
model: openai/gpt-5.5
permission:
  task: allow
---`

// PiAgentModel is the model id the Pi native subagent variant (pi/agents/GADU.md)
// declares in its frontmatter.
//
// Verified live 2026-09-13 (Pi 0.85.1, gentle-pi 2.6.0, @saccolabs/pi-claude-cli
// 0.8.1): gentle-pi's native subagent_run only completes a child whose agent
// file declares a "pi-claude-cli/<model>" id. An "anthropic/*" (OAuth) or
// "openai-codex/*" model ends the child with "assistant reported an error",
// and an absent model: routes to "anthropic/claude-opus-4-8" (also fails).
// Separately, the pi-claude-cli bridge passes the agent's system prompt to a
// fresh Claude Code process via --append-system-prompt-file: a ~7 KB prompt
// (GADU's full persona body, or a same-size neutral filler) HANGS that child
// indefinitely, while a ~400-byte prompt completes normally. That bridge
// prompt-size limit is why pi/agents/GADU.md ships a compact body (see
// piCompactBody) instead of the full persona body used by the other two
// agent variants, deferring the full persona to the gadu-operator skill.
const PiAgentModel = "pi-claude-cli/claude-sonnet-5"

// piAgentFrontmatter is the generator-owned template for pi/agents/GADU.md.
// It declares PiAgentModel (the only model id verified to complete a native
// gentle-pi subagent_run on this stack) and keeps the same Claude
// Code-compatible `tools: '*'` inline scalar the pi-subagents-j0k3r frontmatter
// parser (and gentle-pi's own) already accept.
const piAgentFrontmatter = `---
name: GADU
description: High-judgment operator agent — invoke by name. Opinionated, red-teams the user's reasoning, recommends the highest-odds option, orchestrates sub-agents as lead, grounds claims in verified sources. Not a warm assistant.
model: ` + PiAgentModel + `
tools: '*'
---`

// piCompactBody is the deliberately compact GADU body shipped in
// pi/agents/GADU.md (target < 1500 bytes for the whole file, PiAgentModel's
// doc comment). It carries the identity line, the six defining traits as
// one-line headlines, a two-line voice summary, and an explicit instruction
// to load the gadu-operator skill for the full persona and protocols before
// non-trivial work -- the bridge prompt-size limit means the full persona
// body (as shipped in agents/GADU.md and opencode/agents/GADU.md) cannot be
// used here.
const piCompactBody = `# GADU

You are GADU, the user's high-judgment OPERATOR — not a warm assistant. Be useful and truthful, not liked.

- Judgment: be opinionated and decisive.
- Red-team the user's logic: steelman it, then find its flaws.
- No sycophancy, no condescension.
- Recommend the highest-probability path; don't dump a neutral menu.
- Autonomy: lead and orchestrate sub-agents for broad or adversarial work.
- Source-grounded: verify claims against real evidence before asserting.

Voice: direct, precise, economical, honest. Match the user's language.

Load the ` + "`gadu-operator`" + ` skill for the full persona, the adversarial-review and fan-out protocols, and the safety/memory baselines before non-trivial work.
`

// skillFrontmatter is the generator-owned template for skills/gadu-operator/SKILL.md (D7, R-005).
// The description carries the Trigger: line required by the overlay skill convention.
// The preamble instructs loaders to use the skill on demand; no auto-spawn (R-009).
const skillFrontmatter = `---
name: gadu-operator
description: "Trigger: GADU persona — load when invoking GADU as a portable operator in any runtime (Claude Code, opencode, codex). Provides the full GADU persona body for native sub-agent dispatch on demand."
license: Apache-2.0
metadata:
  author: labdrian-overlay
  version: "1.0"
---`

// skillPreamble is the load-into-native-subagent preamble (R-005, D7).
// It is placed between the do-not-edit header and the persona body in the skill file.
const skillPreamble = `## Activation Contract

Load this skill into a native sub-agent on demand using your runtime's dispatch mechanism.
Do NOT auto-spawn: invoke GADU explicitly by name or by loading this skill file into a
sub-agent prompt. Use the Agent tool (Claude Code), task dispatch (opencode), or
spawn_agent (codex) — never a runtime-specific workflow tool — so the behavior is portable.

`

// doNotEditHeader is the generated-file marker required by R-012 (D7).
// It appears immediately after the closing frontmatter "---" in every output file.
const doNotEditHeader = "<!-- GENERATED — DO NOT EDIT. Source: engine/gadu/persona/body.md. Run: gentle-ai-overlay gadu-generate -->"

// Generate writes all delivery artifacts under repoRoot:
//   - <repoRoot>/agents/GADU.md
//   - <repoRoot>/opencode/agents/GADU.md
//   - <repoRoot>/pi/agents/GADU.md
//   - <repoRoot>/skills/gadu-operator/SKILL.md
//
// Parent directories are created as needed. Generate is idempotent and
// overwrites any existing files.
func Generate(repoRoot string) error {
	agentContent := buildAgentFile()
	opencodeAgentContent := buildOpenCodeAgentFile()
	piAgentContent := buildPiAgentFile()
	skillContent := buildSkillFile()

	agentPath := filepath.Join(repoRoot, "agents", "GADU.md")
	opencodeAgentPath := filepath.Join(repoRoot, "opencode", "agents", "GADU.md")
	piAgentPath := filepath.Join(repoRoot, "pi", "agents", "GADU.md")
	skillPath := filepath.Join(repoRoot, "skills", "gadu-operator", "SKILL.md")

	if err := writeFile(agentPath, agentContent); err != nil {
		return fmt.Errorf("writing agents/GADU.md: %w", err)
	}
	if err := writeFile(opencodeAgentPath, opencodeAgentContent); err != nil {
		return fmt.Errorf("writing opencode/agents/GADU.md: %w", err)
	}
	if err := writeFile(piAgentPath, piAgentContent); err != nil {
		return fmt.Errorf("writing pi/agents/GADU.md: %w", err)
	}
	if err := writeFile(skillPath, skillContent); err != nil {
		return fmt.Errorf("writing skills/gadu-operator/SKILL.md: %w", err)
	}
	return nil
}

// Check regenerates both artifacts into a temp directory and diffs them against
// the committed copies under repoRoot. Returns a non-nil error with a
// descriptive message if any file is missing, stale, or divergent. (R-003)
func Check(repoRoot string) error {
	tmpDir, err := os.MkdirTemp("", "gadu-check-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := Generate(tmpDir); err != nil {
		return fmt.Errorf("regenerating for check: %w", err)
	}

	pairs := []struct {
		label     string
		committed string
		generated string
	}{
		{
			label:     "agents/GADU.md",
			committed: filepath.Join(repoRoot, "agents", "GADU.md"),
			generated: filepath.Join(tmpDir, "agents", "GADU.md"),
		},
		{
			label:     "opencode/agents/GADU.md",
			committed: filepath.Join(repoRoot, "opencode", "agents", "GADU.md"),
			generated: filepath.Join(tmpDir, "opencode", "agents", "GADU.md"),
		},
		{
			label:     "pi/agents/GADU.md",
			committed: filepath.Join(repoRoot, "pi", "agents", "GADU.md"),
			generated: filepath.Join(tmpDir, "pi", "agents", "GADU.md"),
		},
		{
			label:     "skills/gadu-operator/SKILL.md",
			committed: filepath.Join(repoRoot, "skills", "gadu-operator", "SKILL.md"),
			generated: filepath.Join(tmpDir, "skills", "gadu-operator", "SKILL.md"),
		},
	}

	var errs []string
	for _, p := range pairs {
		committedBytes, err := os.ReadFile(p.committed)
		if err != nil {
			if os.IsNotExist(err) {
				errs = append(errs, fmt.Sprintf("%s: missing (not yet generated — run: gentle-ai-overlay gadu-generate)", p.label))
				continue
			}
			errs = append(errs, fmt.Sprintf("%s: cannot read committed file: %v", p.label, err))
			continue
		}
		generatedBytes, err := os.ReadFile(p.generated)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: cannot read regenerated file: %v", p.label, err))
			continue
		}
		if string(committedBytes) != string(generatedBytes) {
			errs = append(errs, fmt.Sprintf("%s: stale — committed content diverges from generator output (run: gentle-ai-overlay gadu-generate)", p.label))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("GADU artifacts are stale or missing:\n  %s", strings.Join(errs, "\n  "))
	}
	return nil
}

// buildAgentFile assembles the complete content for agents/GADU.md.
func buildAgentFile() string {
	return agentFrontmatter + "\n\n" + doNotEditHeader + "\n\n" + personaBody
}

// buildOpenCodeAgentFile assembles the complete content for
// opencode/agents/GADU.md.
func buildOpenCodeAgentFile() string {
	return opencodeAgentFrontmatter + "\n\n" + doNotEditHeader + "\n\n" + personaBody
}

// buildPiAgentFile assembles the complete content for pi/agents/GADU.md,
// using piCompactBody instead of the full canonical persona body (see
// PiAgentModel's doc comment for why).
func buildPiAgentFile() string {
	return piAgentFrontmatter + "\n\n" + doNotEditHeader + "\n\n" + piCompactBody
}

// buildSkillFile assembles the complete content for skills/gadu-operator/SKILL.md.
func buildSkillFile() string {
	return skillFrontmatter + "\n\n" + doNotEditHeader + "\n\n" + skillPreamble + personaBody
}

// PersonaBody returns the raw embedded persona body content (for testing).
func PersonaBody() string { return personaBody }

// writeFile writes content to path, creating parent directories as needed.
func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating parent dir for %s: %w", path, err)
	}
	return os.WriteFile(path, []byte(content), 0644)
}
