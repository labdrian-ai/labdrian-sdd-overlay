package agentgen_test

// The OpenCode agent files (S5, S7 and S18 of odd/tasks/labdrian-orchestrator-opencode.md): the eight roles
// and the foreman each get opencode/agents/<name>.md in the v2 agent schema measured by T4 (L10): the keys
// description, mode, model and permissions. The expectations are written out here, not read from the
// generator: the mode and the model of each agent, and the tools a deny removes.

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/agentgen"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// The OpenCode ids of the tier table of skills/role-chain/references/models.md, which holds each id once.
const (
	ocFrontier = "openai/gpt-6.1-sol#medium"
	ocMid      = "openai/gpt-6.1-terra#medium"
	ocLight    = "openai/gpt-6-luna#low"
)

// wantOpenCode is what is expected of the OpenCode file of one agent.
type wantOpenCode struct {
	name        string
	description string
	mode        string
	model       string
	// editShell are the tools of the first group of denies, in the order the file lists them: edit when the
	// Claude list of the agent holds neither Edit nor Write, shell when it holds no Bash.
	editShell []string
}

// wantOpenCodeAgents are the eight roles in the order of the table of per-role design v3, then the
// foreman, which is the parent and the one primary agent.
func wantOpenCodeAgents() []wantOpenCode {
	var out []wantOpenCode
	byName := map[string]wantRole{}
	for _, r := range wantRoles {
		byName[r.name] = r
	}
	for _, c := range []struct {
		name, model string
		editShell   []string
	}{
		{"builder", ocMid, nil},
		{"shaper", ocFrontier, []string{"shell"}},
		{"polisher", ocMid, []string{"edit", "shell"}},
		{"prototyper", ocMid, nil},
		{"sweeper", ocMid, nil},
		{"reviewer", ocFrontier, []string{"edit", "shell"}},
		{"verifier", ocFrontier, []string{"edit"}},
		{"delivery", ocLight, nil},
	} {
		out = append(out, wantOpenCode{c.name, byName[c.name].description, "subagent", c.model, c.editShell})
	}
	return append(out, wantOpenCode{"foreman", foremanDescription, "primary", ocFrontier, nil})
}

// permRule is one entry of the permissions list of an OpenCode agent file.
type permRule struct{ action, resource, effect string }

// openCodeFile is an OpenCode agent file taken apart: its frontmatter keys in order, the plain value of
// each, the rules of its permissions list and its body.
type openCodeFile struct {
	keys   []string
	values map[string]string
	rules  []permRule
	body   string
}

func parseOpenCode(t *testing.T, file string) openCodeFile {
	t.Helper()
	rest, ok := strings.CutPrefix(file, "---\n")
	if !ok {
		t.Fatalf("no opening --- line:\n%s", file)
	}
	front, body, ok := strings.Cut(rest, "\n---\n\n")
	if !ok {
		t.Fatalf("no closing --- line followed by a blank line:\n%s", file)
	}
	out := openCodeFile{values: map[string]string{}, body: body}
	for _, line := range strings.Split(front, "\n") {
		switch {
		case strings.HasPrefix(line, "  - action: "):
			out.rules = append(out.rules, permRule{action: strings.TrimPrefix(line, "  - action: ")})
		case strings.HasPrefix(line, "    resource: "):
			out.rules[len(out.rules)-1].resource = strings.TrimPrefix(line, "    resource: ")
		case strings.HasPrefix(line, "    effect: "):
			out.rules[len(out.rules)-1].effect = strings.TrimPrefix(line, "    effect: ")
		default:
			key, value, found := strings.Cut(line, ":")
			if !found || strings.HasPrefix(line, " ") {
				t.Fatalf("a frontmatter line that is neither a key nor a rule of the permissions: %q", line)
			}
			out.keys = append(out.keys, key)
			out.values[key] = strings.TrimPrefix(value, " ")
		}
	}
	return out
}

// denied are the actions the file denies, in the order it lists them; every rule must be a deny on `"*"`.
func (f openCodeFile) denied(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, r := range f.rules {
		if r.effect != "deny" || r.resource != `"*"` {
			t.Errorf("the rule %+v is not a deny on %q", r, `"*"`)
		}
		out = append(out, r.action)
	}
	return out
}

func openCodeBody(t *testing.T, root string, a wantOpenCode) string {
	t.Helper()
	if a.name == "foreman" {
		// The foreman holds mem_search, so its file keeps the whole source.
		return readText(t, filepath.Join(root, "roles", "foreman", "body.md"))
	}
	for _, r := range wantRoles {
		if r.name == a.name {
			return wantBody(t, root, r)
		}
	}
	t.Fatalf("%s is no role", a.name)
	return ""
}

// Each of the nine agents has an OpenCode file under opencode/agents, after the files of the other runtimes.
func TestTheOpenCodeFileOfEveryAgentIsWrittenAfterTheFilesOfTheOtherRuntimes(t *testing.T) {
	root := seededRoot(t)
	table, err := agentgen.Artifacts(root)
	if err != nil {
		t.Fatalf("Artifacts: %v", err)
	}
	var paths []string
	for _, a := range table {
		paths = append(paths, a.Path)
	}
	var want []string
	for _, a := range wantOpenCodeAgents() {
		want = append(want, "opencode/agents/"+a.name+".md")
	}
	if got := paths[len(paths)-len(want):]; !slices.Equal(got, want) {
		t.Errorf("the last artifacts are %v, want the OpenCode files %v", got, want)
	}
	if len(paths) != 34 {
		t.Errorf("%d artifacts, want 34: 24 files of the eight roles, the Claude file of the foreman and the nine OpenCode files", len(paths))
	}
}

// The frontmatter is description, mode, model and permissions, in that order, and the body is the one of
// the Claude file. The description is a quoted YAML string, because it holds a colon followed by a space.
func TestTheOpenCodeFileHoldsTheDescriptionTheModeTheModelAndTheBody(t *testing.T) {
	root := seededRoot(t)
	files := artifactContents(t, root)
	for _, a := range wantOpenCodeAgents() {
		t.Run(a.name, func(t *testing.T) {
			f := parseOpenCode(t, files["opencode/agents/"+a.name+".md"])
			// permissions is a key only when the agent has a deny: every role has one from the second
			// group on, but the keys the test pins first are the three every file has.
			if got := f.keys[:3]; !slices.Equal(got, []string{"description", "mode", "model"}) {
				t.Errorf("the first frontmatter keys are %v, want description, mode, model", got)
			}
			if want := strconv.Quote(a.description); f.values["description"] != want {
				t.Errorf("description = %s, want %s", f.values["description"], want)
			}
			if f.values["mode"] != a.mode {
				t.Errorf("mode = %q, want %q", f.values["mode"], a.mode)
			}
			if f.values["model"] != a.model {
				t.Errorf("model = %q, want %q", f.values["model"], a.model)
			}
			if want := openCodeBody(t, root, a); f.body != want {
				t.Errorf("the body differs from the source of the agent:\n%s\nwant\n%s", f.body, want)
			}
		})
	}
}

// The foreman is the one primary agent: the eight roles are subagents.
func TestOnlyTheForemanIsAPrimaryAgent(t *testing.T) {
	files := artifactContents(t, seededRoot(t))
	var primary []string
	for _, a := range wantOpenCodeAgents() {
		if parseOpenCode(t, files["opencode/agents/"+a.name+".md"]).values["mode"] == "primary" {
			primary = append(primary, a.name)
		}
	}
	if !slices.Equal(primary, []string{"foreman"}) {
		t.Errorf("the primary agents are %v, want only the foreman", primary)
	}
}

// The model and its variant are the OpenCode column of the tier table: changing a cell changes the files of
// that tier and leaves the Claude and Pi files and the other tiers alone.
func TestTheModelOfTheOpenCodeFilesIsTheOpenCodeColumnOfTheTierTable(t *testing.T) {
	root := seededRoot(t)
	table := readText(t, modelsPath(root))
	changed := strings.Replace(table, "`openai/gpt-6.1-terra#medium`", "`openai/table-mid#high`", 1)
	if changed == table {
		t.Fatal("the mid row of the tier table is not the shape the test edits")
	}
	if err := os.WriteFile(modelsPath(root), []byte(changed), 0o644); err != nil {
		t.Fatal(err)
	}
	files := artifactContents(t, root)
	for path, want := range map[string]string{
		"opencode/agents/builder.md":  "openai/table-mid#high",
		"opencode/agents/shaper.md":   ocFrontier,
		"opencode/agents/foreman.md":  ocFrontier,
		"opencode/agents/delivery.md": ocLight,
	} {
		if got := parseOpenCode(t, files[path]).values["model"]; got != want {
			t.Errorf("%s model = %q, want %q", path, got, want)
		}
	}
	for _, path := range []string{"agents/builder.md", "pi/agents/builder.md"} {
		if strings.Contains(files[path], "table-mid") {
			t.Errorf("%s carries the OpenCode id of the table", path)
		}
	}
}

// A role whose Claude list has no Edit or Write gets edit denied, one with no Bash gets shell denied; the
// foreman keeps both (S7). The rules are denies on "*", in the order edit, shell, and every deny of the
// file about edit and shell is the one the list calls for.
func TestTheOpenCodeFileDeniesEditAndShellExactlyWhereTheClaudeListLacksThem(t *testing.T) {
	files := artifactContents(t, seededRoot(t))
	for _, a := range wantOpenCodeAgents() {
		t.Run(a.name, func(t *testing.T) {
			f := parseOpenCode(t, files["opencode/agents/"+a.name+".md"])
			var got []string
			for _, d := range f.denied(t) {
				if d == "edit" || d == "shell" {
					got = append(got, d)
				}
			}
			if !slices.Equal(got, a.editShell) {
				t.Errorf("the denies on edit and shell are %v, want %v", got, a.editShell)
			}
		})
	}
}

// The files of the other runtimes do not carry the OpenCode keys: the eight roles' Claude, Pi and Codex files
// are what they were.
func TestTheOpenCodeFilesAddNoKeyToTheFilesOfTheOtherRuntimes(t *testing.T) {
	files := artifactContents(t, seededRoot(t))
	for _, a := range wantOpenCodeAgents() {
		for _, path := range []string{"agents/" + a.name + ".md", "pi/agents/" + a.name + ".md", "codex/agents/" + a.name + ".toml"} {
			text, ok := files[path]
			if !ok {
				continue
			}
			for _, banned := range []string{"\nmode:", "\npermissions:", "effect: deny"} {
				if strings.Contains(text, banned) {
					t.Errorf("%s holds %q, an OpenCode key", path, banned)
				}
			}
		}
	}
}

// The committed files of the repository are what the generator writes (CI runs the same check through the
// command).
func TestTheCommittedOpenCodeFilesAreWhatTheGeneratorWrites(t *testing.T) {
	root := repoRoot(t)
	files := artifactContents(t, root)
	for _, a := range wantOpenCodeAgents() {
		path := "opencode/agents/" + a.name + ".md"
		want, ok := files[path]
		if !ok {
			t.Errorf("the generator emits no %s", path)
			continue
		}
		if got := readText(t, filepath.Join(root, filepath.FromSlash(path))); got != want {
			t.Errorf("%s is stale: run gadu-generate", path)
		}
	}
}

// Each OpenCode file has a row of its own in overlay.manifest: the overlay's (custom, as the Claude and Codex
// agents), the route opencode-agent, copied to the agents directory of OpenCode and to no other runtime.
func TestEachOpenCodeAgentFileHasACustomOpencodeAgentRow(t *testing.T) {
	root := repoRoot(t)
	rows := skills.ReadManifestRows([]byte(readText(t, filepath.Join(root, "overlay.manifest"))))
	for _, target := range []string{"claude", "opencode", "codex"} {
		dest := skills.RouteDestinations{Target: target, SkillsRoot: "/h/" + target + "/skills", AgentsRoot: "/h/" + target + "/agents"}
		routes, err := skills.ResolveRoutes(rows, false, "/o", dest)
		if err != nil {
			t.Fatalf("ResolveRoutes for %s over the manifest of the repository: %v", target, err)
		}
		for _, a := range wantOpenCodeAgents() {
			path := "opencode/agents/" + a.name + ".md"
			var found []skills.ManifestRow
			for _, row := range rows {
				if row.Path == path {
					found = append(found, row)
				}
			}
			if want := []skills.ManifestRow{{Path: path, Tag: "custom", Route: "opencode-agent"}}; !slices.Equal(found, want) {
				t.Errorf("rows named %s = %+v, want exactly %+v", path, found, want)
				continue
			}
			var route skills.ResolvedRoute
			for _, r := range routes {
				if r.Path == path {
					route = r
				}
			}
			want := skills.ResolvedRoute{Path: path, Route: "opencode-agent", RepoRel: path, RepoSource: "/o/" + path}
			if target == "opencode" {
				want.Dest = "/h/opencode/agents/" + a.name + ".md"
			}
			if route != want {
				t.Errorf("%s resolves for %s to %+v, want %+v", path, target, route, want)
			}
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); err != nil {
				t.Errorf("the row %s names a file that is not in the repository: %v", path, err)
			}
		}
	}
}
