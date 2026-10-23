package agentgen

// The role agents of per-role design v3 (docs/architecture/role-design/per-role-design-v3.md, sections
// 2 and 5): one table of what differs between the runtimes, and the projection of the source of a
// role (roles/<role>/body.md) into the file of each of them.

import (
	"fmt"
	"strings"
)

// engramLine is the second line of the core prefix every role body starts with: it sends the role to
// Engram for the artifacts the prompt locates. A role with no Engram tool in its list is not sent
// there, so the projection drops the line for it (slice 2 of the design, L3).
const engramLine = "Read every artifact the prompt locates (file path, or Engram topic via mem_get_observation) before acting. A missing required locator is a blocker: report it, never substitute another store."

// modelFamily is a family of models named once for the runtimes that take the model from the agent
// file. Codex has no entry: its files omit the model, the parent passes it at spawn (D14).
type modelFamily struct {
	// claude is the alias Claude Code resolves to its current model of the family.
	claude string
	// pi is the provider-prefixed ref gentle-pi runs the family by.
	pi string
}

// piSonnetModel is the model id the Pi file of a sonnet role declares in its frontmatter.
//
// Verified live 2026-09-13 (Pi 0.85.1, gentle-pi 2.6.0, @saccolabs/pi-claude-cli 0.8.1): gentle-pi's
// native subagent_run only completes a child whose agent file declares a "pi-claude-cli/<model>" id.
// An "anthropic/*" (OAuth) or "openai-codex/*" model ends the child with "assistant reported an
// error", and an absent model: routes to "anthropic/claude-opus-4-8" (also fails). Separately, the
// pi-claude-cli bridge passes the agent's system prompt to a fresh Claude Code process via
// --append-system-prompt-file: a ~7 KB prompt (the full persona body of GADU, or a same-size neutral
// filler) HANGS that child indefinitely, while a ~400-byte prompt completes normally. That limit was
// measured with GADU's body (GADU is retired); it belongs to the bridge, not to GADU, so it can bound
// the Pi file of any role. The role files are well under it (the largest, the reviewer's, is about
// 2 KB in October 2026); a run of a ~2 KB prompt through the bridge has not been measured.
const piSonnetModel = "pi-claude-cli/claude-sonnet-5"

// The Pi refs are ids of the provider pi-claude-cli, which registers the Anthropic models of Pi
// (index.ts:200 of pi-claude-cli 0.10.0), and all three are in the model list of Pi 1.3.0 on this
// machine. Only sonnet's is the ref that was run: piSonnetModel completed a native subagent_run on
// 2026-09-13 (Pi 0.85.1, gentle-pi 2.6.0). The opus and haiku refs are the same provider's ids of the
// same generation, assumed to complete one too; the live check of the PR (PR 4 of the design, row
// "live check per runtime") is what proves them.
var (
	sonnet = modelFamily{claude: "sonnet", pi: piSonnetModel}
	opus   = modelFamily{claude: "opus", pi: "pi-claude-cli/claude-opus-5"}
	haiku  = modelFamily{claude: "haiku", pi: "pi-claude-cli/claude-haiku-4-5"}
)

// role is what the files of the runtimes hold for one role.
type role struct {
	name        string
	description string
	model       modelFamily
	// effort is how hard the role thinks, written once for the two runtimes that take it from the file:
	// the thinking of Pi and the model_reasoning_effort of Codex. It is one of low, medium, high and
	// xhigh, the values both vocabularies share (gentle-pi 4.3.0 knows off, minimal, low, medium, high,
	// xhigh and max; the model list of Codex 0.148.0 knows low, medium, high, xhigh, max and ultra).
	// The values are an assumption: the design names the field and not its values. They follow what
	// exists: every agent gentle-ai installs for Pi says high, and jd-judge-a says high on Claude.
	effort string
	// tools are the tools of the role in the names of Claude Code, in the order its agent file lists
	// them. The Claude files that existed before the generator are the oracle for these lists, byte
	// for byte; the tools of Pi are made from them (piTools).
	tools []string
}

// roles are the eight roles in the order of the table of per-role design v3, section 2.
var roles = []role{
	{
		name:        "builder",
		description: "Implements the tasks a labdrian workflow names, or fixes confirmed ledger ids, and reports the evidence of each work unit.",
		model:       sonnet,
		effort:      "high",
		tools: []string{
			"Read", "Edit", "Write", "Glob", "Grep", "Bash",
			"mcp__engram__mem_search", "mcp__plugin_engram_engram__mem_search",
			"mcp__engram__mem_get_observation", "mcp__plugin_engram_engram__mem_get_observation",
			"mcp__engram__mem_save", "mcp__plugin_engram_engram__mem_save",
			"mcp__engram__mem_update", "mcp__plugin_engram_engram__mem_update",
			"mcp__codegraph__codegraph_explore",
		},
	},
	{
		name:        "shaper",
		description: "Shapes one planning artifact of a labdrian workflow (proposal, spec, design or tasks) and returns open product decisions as risks.",
		model:       opus,
		effort:      "high",
		tools: []string{
			"Read", "Edit", "Write", "Grep", "Glob",
			"mcp__engram__mem_search", "mcp__plugin_engram_engram__mem_search",
			"mcp__engram__mem_get_observation", "mcp__plugin_engram_engram__mem_get_observation",
			"mcp__engram__mem_save", "mcp__plugin_engram_engram__mem_save",
			"mcp__codegraph__codegraph_explore",
		},
	},
	{
		name:        "polisher",
		description: "Polisher role, the read-only blind judge of an immutable target (judgment-day launches it for both judges); returns one JSON findings object.",
		model:       sonnet,
		effort:      "high",
		tools: []string{
			"Read", "Glob", "Grep",
			"mcp__plugin_engram_engram__mem_search", "mcp__plugin_engram_engram__mem_get_observation",
			"mcp__codegraph__codegraph_explore",
		},
	},
	{
		name:        "prototyper",
		description: "Answers listed feasibility questions with throwaway spikes in a disposable worktree and returns kill, narrow or continue per question.",
		model:       sonnet,
		effort:      "medium",
		tools: []string{
			"Read", "Glob", "Grep", "Bash", "Write", "Edit",
			"mcp__plugin_engram_engram__mem_search", "mcp__plugin_engram_engram__mem_get_observation",
		},
	},
	{
		name:        "sweeper",
		description: "Removal-only cleanup of dead code and stale prose that a work unit left behind; commits one chore(sweep) commit or none.",
		model:       sonnet,
		effort:      "medium",
		tools: []string{
			"Read", "Edit", "Glob", "Grep", "Bash",
			"mcp__engram__mem_search", "mcp__plugin_engram_engram__mem_search",
			"mcp__engram__mem_get_observation", "mcp__plugin_engram_engram__mem_get_observation",
			"mcp__codegraph__codegraph_explore",
		},
	},
	{
		name:        "reviewer",
		description: "Reviewer role, the read-only reader of a committed diff and its detached worktree (send base, head, diff_sha256, the diff, the worktree path and the lens rows); returns one JSON object of findings.",
		model:       sonnet,
		effort:      "high",
		tools:       []string{"Read", "Grep", "Glob"},
	},
	{
		name:        "verifier",
		description: "Verifier role, runs the checks a prompt names and reproduces the findings it lists (send the finding ids, the exact checks and the worktree); returns reproduced, not-reproduced or not-run per id.",
		model:       sonnet,
		effort:      "medium",
		tools:       []string{"Read", "Grep", "Glob", "Bash", "mcp__plugin_engram_engram__mem_get_observation"},
	},
	{
		name:        "delivery",
		description: "Archives a finished labdrian workflow by copying and moving its artifacts mechanically, recording unfinished work as unfinished.",
		model:       haiku,
		effort:      "low",
		tools: []string{
			"Read", "Edit", "Write", "Glob", "Bash",
			"mcp__engram__mem_search", "mcp__plugin_engram_engram__mem_search",
			"mcp__engram__mem_get_observation", "mcp__plugin_engram_engram__mem_get_observation",
			"mcp__engram__mem_save", "mcp__plugin_engram_engram__mem_save",
			"mcp__codegraph__codegraph_explore",
		},
	},
}

// piToolNames translates a tool of Claude Code into the tool of Pi that does the same.
//
// Verified on 2026-10-22 by reading the installed sources (Pi 1.3.0, gentle-pi 4.3.0, gentle-engram
// 0.4.2), not by running Pi:
//   - read, edit, write, bash, grep and find are built-in tools of Pi (docs/cli.md of pi-coding-agent
//     lists them in --tools), and the agents gentle-ai installs for Pi name them
//     (~/.pi/agent/agents/gentle-ai-worker.md). Glob is find.
//   - gentle-engram registers each Engram tool by its bare name, with no mcp__ prefix
//     (index.ts:70-89 and :2337-2340), so both server names Claude Code lists map to one name.
//   - gentle-pi has one native CodeGraph tool, codegraph, with the operations init, query and explore
//     (extensions/codegraph-tools.ts:345). It has no explore-only form: the tool a role gets on Pi
//     can also run init, which writes the .codegraph index of the workspace.
//
// Not verified: that the child process of a subagent loads gentle-engram and gentle-pi, so that
// these tools exist there. gentle-pi reads the tools field as a comma list or a YAML list
// (lib/agents-config.ts:173-177), passes it to the child as --tools (lib/agents-runner.ts:283-329),
// and Pi drops a name it does not know without a word (the comment at lib/agents-runner.ts:241), which
// is why a test pins every name to this set.
var piToolNames = map[string]string{
	"Read":  "read",
	"Edit":  "edit",
	"Write": "write",
	"Glob":  "find",
	"Grep":  "grep",
	"Bash":  "bash",

	"mcp__engram__mem_search":                        "mem_search",
	"mcp__plugin_engram_engram__mem_search":          "mem_search",
	"mcp__engram__mem_get_observation":               "mem_get_observation",
	"mcp__plugin_engram_engram__mem_get_observation": "mem_get_observation",
	"mcp__engram__mem_save":                          "mem_save",
	"mcp__plugin_engram_engram__mem_save":            "mem_save",
	"mcp__engram__mem_update":                        "mem_update",
	"mcp__plugin_engram_engram__mem_update":          "mem_update",
	"mcp__codegraph__codegraph_explore":              "codegraph",
}

// sourcePath is where the source of the role is, as a slash-separated path under the repository root.
func (r role) sourcePath() string { return "roles/" + r.name + "/body.md" }

// hasEngramTool says whether the role can reach Engram: one of its tools is a tool of an Engram server,
// under either name the agents of Claude Code list it by.
func (r role) hasEngramTool() bool {
	for _, tool := range r.tools {
		if strings.HasPrefix(tool, "mcp__engram__") || strings.HasPrefix(tool, "mcp__plugin_engram_engram__") {
			return true
		}
	}
	return false
}

// body is what the three files of the role carry after their frontmatter: its source, without the
// line that sends it to Engram when it has no Engram tool.
func (r role) body(source string) (string, error) {
	if r.hasEngramTool() {
		return source, nil
	}
	if strings.Count(source, engramLine+"\n") != 1 {
		return "", fmt.Errorf("the line of the core prefix that sends a role to Engram is not there exactly once, and %s has no Engram tool to drop it for", r.name)
	}
	return strings.Replace(source, engramLine+"\n", "", 1), nil
}

// piTools is the tools field of the Pi file: the tools of the role in the names of Pi, in the order of
// the Claude list, each once.
func (r role) piTools() (string, error) {
	var names []string
	seen := map[string]bool{}
	for _, tool := range r.tools {
		name, ok := piToolNames[tool]
		if !ok {
			return "", fmt.Errorf("the tool %s of %s has no name on Pi", tool, r.name)
		}
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return strings.Join(names, ", "), nil
}

// project makes the three files of the role from its source: for Claude Code, Pi and Codex, in that
// order.
func (r role) project(source string) ([]Artifact, error) {
	body, err := r.body(source)
	if err != nil {
		return nil, err
	}
	piTools, err := r.piTools()
	if err != nil {
		return nil, err
	}
	codex, err := r.codexFile(body)
	if err != nil {
		return nil, err
	}
	claude := "---\nname: " + r.name + "\ndescription: " + r.description + "\nmodel: " + r.model.claude +
		"\ntools: " + strings.Join(r.tools, ", ") + "\n---\n\n" + body
	pi := "---\nname: " + r.name + "\ndescription: " + r.description + "\nmodel: " + r.model.pi +
		"\nthinking: " + r.effort + "\ntools: " + piTools + "\n---\n\n" + body
	return []Artifact{
		{Path: "agents/" + r.name + ".md", Content: claude},
		{Path: "pi/agents/" + r.name + ".md", Content: pi},
		{Path: "codex/agents/" + r.name + ".toml", Content: codex},
	}, nil
}

// codexFile is the Codex agent file: the four fields of the design in the order it lists them, and no
// model (the parent passes it at spawn, D14) and no tools (an agent file of Codex has no allowlist).
func (r role) codexFile(body string) (string, error) {
	name, err := tomlString(r.name)
	if err != nil {
		return "", err
	}
	description, err := tomlString(r.description)
	if err != nil {
		return "", err
	}
	instructions, err := tomlMultiline(body)
	if err != nil {
		return "", err
	}
	effort, err := tomlString(r.effort)
	if err != nil {
		return "", err
	}
	return "name = " + name + "\ndescription = " + description + "\ndeveloper_instructions = " + instructions +
		"\nmodel_reasoning_effort = " + effort + "\n", nil
}
