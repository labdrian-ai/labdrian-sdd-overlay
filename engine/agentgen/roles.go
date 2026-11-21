package agentgen

// The role agents of per-role design v3 (docs/architecture/role-design/per-role-design-v3.md, sections
// 2 and 5): one table of what differs between the runtimes, and the projection of the source of a
// role (roles/<role>/body.md) into the file of each of them.

import (
	"fmt"
	"strings"
)

// memoryLine is the second line of the core prefix every role body starts with: it sends the role to
// memorycard for the artifacts the prompt locates. A role with no memorycard tool in its list is not
// sent there, so the projection drops the line for it (slice 2 of the design, L3). memorycard replaced
// Engram as the memory store: Engram is frozen, read-only history (longterm-mem reads it), so no role
// lists an Engram tool.
const memoryLine = "Read every artifact the prompt locates (file path, or memorycard topic via mem_search, then mem_get) before acting. A missing required locator is a blocker: report it, never substitute another store."

// byIDLine takes the place of memoryLine in the files of a role that can read memorycard by id but
// cannot search it (the verifier, which per-role design v3 keeps from searching memory): the prompt
// gives it a memorycard id, never a topic.
const byIDLine = "Read every artifact the prompt locates (file path, or memorycard id via mem_get) before acting. A missing required locator is a blocker: report it, never substitute another store."

// The three tiers of the tier table (skills/role-chain/references/models.md), which holds the model id
// of each tier for each runtime. Codex has no id there: its files omit the model, the parent passes it
// at spawn (D14).
//
// What the Pi column holds, as verified when the ids were chosen. The Pi refs are ids of the provider
// pi-claude-cli, which registers the Anthropic models of Pi (index.ts:200 of pi-claude-cli 0.10.0), and
// all three are in the model list of Pi 1.3.0 on this machine. Only the mid one is the ref that was
// run: it completed a native subagent_run on 2026-09-13 (Pi 0.85.1, gentle-pi 2.6.0), while an
// "anthropic/*" (OAuth) or "openai-codex/*" model ends the child with "assistant reported an error",
// and an absent model routes to "anthropic/claude-opus-4-8" (also fails). The frontier and light refs
// are the same provider's ids of the same generation, assumed to complete one too; the live check of
// the PR (PR 4 of the design, row "live check per runtime") is what proves them. Separately, the
// pi-claude-cli bridge passes the agent's system prompt to a fresh Claude Code process via
// --append-system-prompt-file: a ~7 KB prompt HANGS that child indefinitely, while a ~400-byte prompt
// completes normally. That limit belongs to the bridge; the role files are well under it (the largest,
// the reviewer's, is about 2 KB in October 2026), and a run of a ~2 KB prompt has not been measured.
const (
	tierFrontier = "frontier"
	tierMid      = "mid"
	tierLight    = "light"
)

// role is what the files of the runtimes hold for one role.
type role struct {
	name        string
	description string
	// tier is the model tier of the role (models.md of skills/role-chain); the ids of each runtime come from
	// the tier table there, not from this file.
	tier string
	// effort is how hard the role thinks, written once for the two runtimes that take it from the file:
	// the thinking of Pi and the model_reasoning_effort of Codex. The owner set three tiers (per-role
	// design v3, D15): frontier and mid think at medium, light at low. Both vocabularies also know max
	// (gentle-pi 4.3.0: off, minimal, low, medium, high, xhigh, max; Codex 0.148.0: low, medium, high,
	// xhigh, max, ultra). Medium is the parent's effort, and the parent is not a row of this table.
	effort string
	// tools are the tools of the role in the names of Claude Code, in the order its agent file lists
	// them. The Claude files that existed before the generator are the oracle for these lists, byte
	// for byte; the tools of Pi are made from them (piTools).
	tools []string
	// openCodeSkill keeps the skill tool of OpenCode for a role whose Claude list names no Skill tool: only the
	// foreman, whose recorded OpenCode run loads a skill body in its session (S7, S13).
	openCodeSkill bool
}

// roles are the eight roles in the order of the table of per-role design v3, section 2.
var roles = []role{
	{
		name:        "builder",
		description: "Implements the tasks a labdrian workflow names, or fixes confirmed ledger ids, and reports the evidence of each work unit.",
		tier:        tierMid,
		effort:      "medium",
		tools: []string{
			"Read", "Edit", "Write", "Glob", "Grep", "Bash",
			"mcp__memorycard__mem_search", "mcp__memorycard__mem_get", "mcp__memorycard__mem_save", "mcp__memorycard__mem_update",
			"mcp__codegraph__codegraph_explore",
		},
	},
	{
		name:        "shaper",
		description: "Writes the Specs and Tasks of the feature document of a labdrian workflow and returns open product decisions as risks.",
		tier:        tierFrontier,
		effort:      "medium",
		tools: []string{
			"Read", "Edit", "Write", "Grep", "Glob",
			"mcp__memorycard__mem_search", "mcp__memorycard__mem_get", "mcp__memorycard__mem_save",
			"mcp__codegraph__codegraph_explore",
		},
	},
	{
		name:        "polisher",
		description: "Polisher role, the read-only blind judge of an immutable target (judgment-day launches it for both judges); returns one JSON findings object.",
		tier:        tierMid,
		effort:      "medium",
		tools: []string{
			"Read", "Glob", "Grep",
			"mcp__memorycard__mem_search", "mcp__memorycard__mem_get",
			"mcp__codegraph__codegraph_explore",
		},
	},
	{
		name:        "prototyper",
		description: "Answers listed feasibility questions with throwaway spikes in a disposable worktree and returns kill, narrow or continue per question.",
		tier:        tierMid,
		effort:      "medium",
		tools: []string{
			"Read", "Glob", "Grep", "Bash", "Write", "Edit",
			"mcp__memorycard__mem_search", "mcp__memorycard__mem_get",
		},
	},
	{
		name:        "sweeper",
		description: "Removal-only cleanup of dead code and stale prose that a work unit left behind; commits one chore(sweep) commit or none.",
		tier:        tierMid,
		effort:      "medium",
		tools: []string{
			"Read", "Edit", "Glob", "Grep", "Bash",
			"mcp__memorycard__mem_search", "mcp__memorycard__mem_get",
			"mcp__codegraph__codegraph_explore",
		},
	},
	{
		name:        "reviewer",
		description: "Reviewer role, the read-only reader of a committed diff and its detached worktree (send base, head, diff_sha256, the diff, the worktree path and the lens rows); returns one JSON object of findings.",
		tier:        tierFrontier,
		effort:      "medium",
		tools:       []string{"Read", "Grep", "Glob"},
	},
	{
		name:        "verifier",
		description: "Verifier role, runs the checks a prompt names and reproduces the findings it lists (send the finding ids, the exact checks and the worktree); returns reproduced, not-reproduced or not-run per id, or, with no finding id, the report of an exemption, each check as pass, fail or not-run.",
		tier:        tierFrontier,
		effort:      "medium",
		tools:       []string{"Read", "Grep", "Glob", "Bash", "mcp__memorycard__mem_get"},
	},
	{
		name:        "delivery",
		description: "Closes the feature document of a finished labdrian workflow in place with a final Log entry, recording unfinished work as unfinished.",
		tier:        tierLight,
		effort:      "low",
		tools: []string{
			"Read", "Edit", "Write", "Glob", "Bash",
			"mcp__memorycard__mem_search", "mcp__memorycard__mem_get", "mcp__memorycard__mem_save",
			"mcp__codegraph__codegraph_explore",
		},
	},
}

// foreman is the parent of the role chain (S2 of odd/tasks/labdrian-orchestrator-opencode.md): the agent a
// main session runs as, which talks with the owner and launches the eight roles. It is not one of them:
// the closed role vocabulary of engine/roles has no foreman, and it has two files here, the Claude one and the OpenCode one (S5).
// Its model is the frontier row of the tier table, as the parent's is (S18, models.md of role-chain). Its
// tools are the Claude names; the subagent tool is Agent (T1 of the feature), which the eight roles never
// hold. AskUserQuestion shows the owner a question menu; in OpenCode it keeps the question tool (T48, L134).
// It writes only the feature document: the Edit and Write it holds are a limit of its instructions,
// not of its tool list (S16 B8).
var foreman = role{
	name:        "foreman",
	description: "Talks with the owner and drives the labdrian workflow: entry, the role chain and closure; writes only the feature document.",
	tier:        tierFrontier,
	// Its OpenCode file keeps the skill tool (S7): the recorded run of S13 loads a skill body in its session.
	openCodeSkill: true,
	tools: []string{
		"Read", "Grep", "Glob", "Edit", "Write", "Bash", "Agent", "AskUserQuestion",
		"mcp__memorycard__mem_search", "mcp__memorycard__mem_get", "mcp__memorycard__mem_save", "mcp__memorycard__mem_update",
		"mcp__codegraph__codegraph_explore",
	},
}

// piToolNames translates a tool of Claude Code into the tool of Pi that does the same.
//
// Verified on 2026-10-22 by reading the installed sources (Pi 1.3.0, gentle-pi 4.3.0), not by running
// Pi:
//   - read, edit, write, bash, grep and find are built-in tools of Pi (docs/cli.md of pi-coding-agent
//     lists them in --tools), and the agents gentle-ai installs for Pi name them
//     (~/.pi/agent/agents/gentle-ai-worker.md). Glob is find.
//   - the memorycard tools keep their MCP name, mcp__memorycard__<tool>. Verified on 2026-11-05 by
//     reading pi-coding-agent 1.4.0: Pi registers every tool of an MCP server as mcp__<server>__<tool>
//     (createMcpToolName in dist/extensions/mcp/tools.js), and memorycard is the server key
//     "memorycard" of its mcp.json (~/.pi/agent/mcp.json). gentle-pi treats a requested tool whose
//     name starts with mcp__ as an MCP tool it cannot check up front (extensions/gentle-agents.ts:338
//     of gentle-pi 4.4.0). gentle-engram, which registered the Engram
//     tools by their bare names (mem_search, ...), is no longer installed (2026-11-04), so the bare
//     names would reach no tool.
//   - gentle-pi has one native CodeGraph tool, codegraph, with the operations init, query and explore
//     (extensions/codegraph-tools.ts:345). It has no explore-only form: the tool a role gets on Pi
//     can also run init, which writes the .codegraph index of the workspace.
//
// Not verified: that the child process of a subagent loads gentle-pi and the memorycard server, so that
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

	"mcp__memorycard__mem_search":       "mcp__memorycard__mem_search",
	"mcp__memorycard__mem_get":          "mcp__memorycard__mem_get",
	"mcp__memorycard__mem_save":         "mcp__memorycard__mem_save",
	"mcp__memorycard__mem_update":       "mcp__memorycard__mem_update",
	"mcp__codegraph__codegraph_explore": "codegraph",
}

// sourcePath is where the source of the role is, as a slash-separated path under the repository root.
func (r role) sourcePath() string { return "roles/" + r.name + "/body.md" }

// hasMemoryTool says whether the role can reach memorycard: one of its tools is a tool of the
// memorycard server.
func (r role) hasMemoryTool() bool {
	for _, tool := range r.tools {
		if strings.HasPrefix(tool, "mcp__memorycard__") {
			return true
		}
	}
	return false
}

// hasTool says whether the role holds the tool of that name.
func (r role) hasTool(name string) bool {
	for _, tool := range r.tools {
		if tool == name {
			return true
		}
	}
	return false
}

// body is what the three files of the role carry after their frontmatter: its source, without the
// line that sends it to memorycard when it has no memorycard tool, and with byIDLine in its place when
// it can read memorycard by id but cannot search it (the verifier), so no role is told to call a tool
// it does not hold.
func (r role) body(source string) (string, error) {
	if r.hasMemoryTool() && r.hasTool("mcp__memorycard__mem_search") {
		return source, nil
	}
	if strings.Count(source, memoryLine+"\n") != 1 {
		return "", fmt.Errorf("the line of the core prefix that sends a role to memorycard is not there exactly once, and %s has no memorycard search to keep it for", r.name)
	}
	if r.hasTool("mcp__memorycard__mem_get") {
		return strings.Replace(source, memoryLine+"\n", byIDLine+"\n", 1), nil
	}
	return strings.Replace(source, memoryLine+"\n", "", 1), nil
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
func (r role) project(source string, models ModelTable) ([]Artifact, error) {
	claudeModel, err := models.ID(r.tier, runtimeClaude)
	if err != nil {
		return nil, err
	}
	piModel, err := models.ID(r.tier, runtimePi)
	if err != nil {
		return nil, err
	}
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
	claude := r.claudeFile(claudeModel, body)
	pi := "---\nname: " + r.name + "\ndescription: " + r.description + "\nmodel: " + piModel +
		"\nthinking: " + r.effort + "\ntools: " + piTools + "\n---\n\n" + body
	return []Artifact{
		{Path: "agents/" + r.name + ".md", Content: claude},
		{Path: "pi/agents/" + r.name + ".md", Content: pi},
		{Path: "codex/agents/" + r.name + ".toml", Content: codex},
	}, nil
}

// claudeFile is the Claude agent file: the four keys of claudeKeys, a blank line and the body.
func (r role) claudeFile(model, body string) string {
	return "---\nname: " + r.name + "\ndescription: " + r.description + "\nmodel: " + model +
		"\ntools: " + strings.Join(r.tools, ", ") + "\n---\n\n" + body
}

// projectClaude makes the Claude file of a role that has no Pi or Codex file (the foreman): its source, the
// model of its tier from the table, and no Pi or Codex file.
func (r role) projectClaude(source string, models ModelTable) (Artifact, error) {
	model, err := models.ID(r.tier, runtimeClaude)
	if err != nil {
		return Artifact{}, err
	}
	body, err := r.body(source)
	if err != nil {
		return Artifact{}, err
	}
	return Artifact{Path: "agents/" + r.name + ".md", Content: r.claudeFile(model, body)}, nil
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
