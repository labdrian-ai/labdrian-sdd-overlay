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
// a deny on shell removes the shell tool, both measured in L10 T4.

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

// openCodeDenyAll is the resource of every deny: a deny on a tool covers all of its uses.
const openCodeDenyAll = "*"

// The OpenCode tools a deny can name here, in the order a file lists them.
const (
	openCodeEdit  = "edit"
	openCodeShell = "shell"
)

// openCodeDenies are the tools of OpenCode the agent may not use, in the order of the file: each tool of
// OpenCode is denied unless the Claude list of the agent holds a tool that does the same.
func (r role) openCodeDenies() []string {
	var denies []string
	// edit covers the patch tool: the Claude Edit and Write both write files.
	if !r.hasTool("Edit") && !r.hasTool("Write") {
		denies = append(denies, openCodeEdit)
	}
	if !r.hasTool("Bash") {
		denies = append(denies, openCodeShell)
	}
	return denies
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
	if denies := r.openCodeDenies(); len(denies) > 0 {
		b.WriteString("permissions:\n")
		for _, tool := range denies {
			b.WriteString("  - action: " + tool + "\n    resource: " + yamlStarString() + "\n    effect: deny\n")
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
