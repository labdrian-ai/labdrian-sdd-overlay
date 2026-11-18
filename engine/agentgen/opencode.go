package agentgen

// The OpenCode agent files (S5, S7 and S18 of odd/tasks/labdrian-orchestrator-opencode.md): one file per
// agent, opencode/agents/<name>.md, in the v2 agent schema that T4 of the feature measured (L10):
// the keys description, mode, model and permissions. The agent id is the name of the file, so there is
// no name key; steps is left to OpenCode's default.
//
// The model is the OpenCode column of the tier table (models.md of role-chain), `openai/<model>#<effort>`:
// the effort of the tier is the variant after the `#`, so no id and no effort is written here. A plain
// YAML scalar may hold a `#` that no space precedes, so the id stays unquoted, as T4 measured it.
//
// The permissions are written from the Claude tool list of the agent, the one list the roles are checked
// against: a tool the list does not hold becomes a deny on every resource of the OpenCode tool that does
// the same. OpenCode reads a rule as {action, resource, effect}; a deny on edit covers the patch tool and
// a deny on shell removes the shell tool, both measured in L10 T4. For each MCP server the rules are a deny on
// every tool of the server and an allow of each tool the list names (T39, L102 (2): the last match wins).

import (
	"encoding/json"
	"fmt"
	"strings"
)

// The modes of an OpenCode agent: the foreman is the one primary agent, the eight roles are subagents.
const (
	openCodeModePrimary  = "primary"
	openCodeModeSubagent = "subagent"
)

// openCodeDenyAll is the resource of every rule: a rule on a tool covers all of its uses.
const openCodeDenyAll = "*"

// The effects of a rule.
const (
	openCodeDeny  = "deny"
	openCodeAllow = "allow"
)

// openCodeRule is one entry of the permissions of an OpenCode agent file: an effect on every resource of
// the tool or the pattern its action names.
type openCodeRule struct {
	action string
	effect string
}

// openCodeMCPServers are the MCP servers of the labdrian flow, in the order of their rules: the servers the
// OpenCode adapter writes into opencode.json (engine/runtime/opencode_json.go), memorycard and codegraph, and
// longterm-mem, which `longterm-mem register` writes there. OpenCode names a tool of a server <server>_<tool>.
// A test of engine/runtime fails when opencode.json names a server this list lacks.
var openCodeMCPServers = []string{"memorycard", "longterm-mem", "codegraph"}

// openCodeRules are the rules of the agent, in the order of the file; the last rule that matches a tool wins
// (L102 (2)). First a deny on each tool of OpenCode the agent may not use: each is denied unless the Claude list
// of the agent holds a tool that does the same. The names are the v2 tool names L10 T2 listed; read, glob and
// grep are never denied, as every Claude list holds a reader. Then, for each MCP server, a deny on every tool of
// the server and an allow of each tool the Claude list names as mcp__<server>__<tool>, in that order.
func (r role) openCodeRules() []openCodeRule {
	var denies []string
	deny := func(tool string, unless bool) {
		if !unless {
			denies = append(denies, tool)
		}
	}
	// edit covers the patch tool: the Claude Edit and Write both write files. L10 T4 measured the deny on edit and
	// on shell; the others follow S7 and are declared as such until a run observes them (L16 risk 4).
	deny("edit", r.hasTool("Edit") || r.hasTool("Write"))
	deny("shell", r.hasTool("Bash"))
	deny("subagent", r.hasTool("Agent"))
	deny("question", r.hasTool("AskUserQuestion"))
	deny("skill", r.hasTool("Skill") || r.openCodeSkill)
	deny("webfetch", r.hasTool("WebFetch"))
	deny("websearch", r.hasTool("WebSearch"))
	// No Claude tool generates an image.
	deny("experimental_image_generate", false)
	// execute is the Code Mode wrapper around the MCP tools: a role with no MCP tool in its list has no use for it.
	deny("execute", r.hasMCPTool())
	rules := make([]openCodeRule, 0, len(denies))
	for _, tool := range denies {
		rules = append(rules, openCodeRule{tool, openCodeDeny})
	}
	for _, server := range openCodeMCPServers {
		rules = append(rules, openCodeRule{server + "_*", openCodeDeny})
		prefix := "mcp__" + server + "__"
		for _, tool := range r.tools {
			if name, ok := strings.CutPrefix(tool, prefix); ok {
				rules = append(rules, openCodeRule{server + "_" + name, openCodeAllow})
			}
		}
	}
	return rules
}

// hasMCPTool says whether the role holds a tool of any MCP server.
func (r role) hasMCPTool() bool {
	for _, tool := range r.tools {
		if strings.HasPrefix(tool, "mcp__") {
			return true
		}
	}
	return false
}

// openCodeMode is the mode of the OpenCode file of the agent.
func (r role) openCodeMode() string {
	if r.name == foreman.name {
		return openCodeModePrimary
	}
	return openCodeModeSubagent
}

// projectOpenCode makes the OpenCode file of the agent from its source: the frontmatter, a blank line and
// the same body as its Claude file.
func (r role) projectOpenCode(source string, models ModelTable) (Artifact, error) {
	model, err := models.ID(r.tier, runtimeOpenCode)
	if err != nil {
		return Artifact{}, err
	}
	body, err := r.body(source)
	if err != nil {
		return Artifact{}, err
	}
	description, err := yamlString(r.description)
	if err != nil {
		return Artifact{}, err
	}
	var b strings.Builder
	b.WriteString("---\ndescription: " + description + "\nmode: " + r.openCodeMode() + "\nmodel: " + model + "\n")
	if rules := r.openCodeRules(); len(rules) > 0 {
		b.WriteString("permissions:\n")
		for _, rule := range rules {
			b.WriteString("  - action: " + rule.action + "\n    resource: " + yamlStarString() + "\n    effect: " + rule.effect + "\n")
		}
	}
	b.WriteString("---\n\n" + body)
	return Artifact{Path: "opencode/agents/" + r.name + ".md", Content: b.String()}, nil
}

// yamlStarString is the resource that matches everything, quoted because a bare * starts a YAML alias.
func yamlStarString() string { return `"` + openCodeDenyAll + `"` }

// yamlString is text as a double-quoted YAML scalar: a description holds a colon followed by a space,
// which a plain scalar cannot. A JSON string is a YAML double-quoted scalar, and without HTML escaping
// it keeps `<`, `>` and `&` as they are.
func yamlString(text string) (string, error) {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(text); err != nil {
		return "", fmt.Errorf("the text %q cannot be a YAML string: %w", text, err)
	}
	return strings.TrimSuffix(b.String(), "\n"), nil
}
