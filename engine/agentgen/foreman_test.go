package agentgen_test

// The foreman (S2, S5 of odd/tasks/labdrian-orchestrator-opencode.md): the parent that talks with the
// owner and launches the eight roles. It has one source, roles/foreman/body.md, and one generated file,
// agents/foreman.md for Claude Code: no Pi file and no Codex file (S5), and it is not one of the eight
// roles of the chain, so none of the tables of roles_test.go holds it. Its model is the frontier row of
// the tier table (S18), never an id written in the generator.

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/agentgen"
)

const (
	foremanDescription = "Talks with the owner and drives the labdrian workflow: entry, the role chain and closure; writes only the feature document."
	// The subagent tool of Claude Code is named Agent (T1 of the feature, L10).
	foremanClaudeTools = "Read, Grep, Glob, Edit, Write, Bash, Agent, mcp__memorycard__mem_search, mcp__memorycard__mem_get, mcp__memorycard__mem_save, mcp__memorycard__mem_update, mcp__codegraph__codegraph_explore"
)

func artifactContents(t *testing.T, root string) map[string]string {
	t.Helper()
	table, err := agentgen.Artifacts(root)
	if err != nil {
		t.Fatalf("Artifacts: %v", err)
	}
	files := map[string]string{}
	for _, a := range table {
		files[a.Path] = a.Content
	}
	return files
}

// The table holds the Claude file of the foreman after the files of the eight roles, and no Pi or Codex
// file for it.
func TestTheForemanHasNoPiOrCodexFileAndItsClaudeFileFollowsTheRoles(t *testing.T) {
	root := seededRoot(t)
	table, err := agentgen.Artifacts(root)
	if err != nil {
		t.Fatalf("Artifacts: %v", err)
	}

	var paths []string
	for _, a := range table {
		paths = append(paths, a.Path)
	}
	// The OpenCode files of the nine agents come after it (opencode_test.go), so the Claude file of the foreman
	// is the tenth from the end.
	if at := paths[len(paths)-10]; at != "agents/foreman.md" {
		t.Errorf("the tenth artifact from the end is %q, want agents/foreman.md after the 24 files of the eight roles", at)
	}
	for _, p := range paths {
		if p == "pi/agents/foreman.md" || p == "codex/agents/foreman.toml" {
			t.Errorf("the table holds %s: the foreman has no Pi or Codex file", p)
		}
	}
	if len(paths) != 34 {
		t.Errorf("%d artifacts, want 34: the three files of each of the eight roles, the Claude file of the foreman and the nine OpenCode files", len(paths))
	}
}

// The Claude file of the foreman is its frontmatter (name, description, model, tools, in that order), a
// blank line and its source, which keeps the line that sends it to memorycard because it holds mem_search.
func TestTheClaudeFileOfTheForemanIsItsFrontmatterAndItsSource(t *testing.T) {
	root := seededRoot(t)
	source := readText(t, filepath.Join(root, "roles", "foreman", "body.md"))
	if strings.Count(source, memoryLine+"\n") != 1 {
		t.Fatal("roles/foreman/body.md must carry the memory line of the core prefix exactly once")
	}

	got := artifactContents(t, root)["agents/foreman.md"]

	want := "---\nname: foreman\ndescription: " + foremanDescription + "\nmodel: opus\ntools: " + foremanClaudeTools + "\n---\n\n" + source
	if got != want {
		t.Errorf("agents/foreman.md =\n%s\nwant\n%s", got, want)
	}
	// The Claude file has no effort field (Claude Code takes none from the file), and the foreman has no
	// Pi or Codex file that would take one.
	// Only the frontmatter is read: between the opening --- line and the next --- line, so body prose that
	// says "thinking: x" is no field. Both fences must be found, whatever the line ending, or the check fails.
	// A key counts with any indentation, quoting or space before its colon.
	if field, found := effortField(got); !found {
		t.Errorf("agents/foreman.md has no frontmatter between two --- lines:\n%s", got)
	} else if field != "" {
		t.Errorf("the frontmatter of agents/foreman.md holds an effort field: %q", field)
	}
}

// The effort check reads the frontmatter only, finds it whatever the line ending, refuses a file with no
// closing fence, and knows an effort key however it is written.
func TestTheEffortCheckReadsTheFrontmatterOnly(t *testing.T) {
	cases := []struct {
		name, file, field string
		found             bool
	}{
		{"none", "---\nname: x\n---\n\nbody\n", "", true},
		{"body prose", "---\nname: x\n---\n\nthinking: x\n", "", true},
		{"closing fence last, no newline", "---\nname: x\n---", "", true},
		{"crlf", "---\r\nname: x\r\n---\r\n\r\nthinking: x\r\n", "", true},
		{"crlf field", "---\r\neffort: high\r\n---\r\n", "effort: high", true},
		{"indented", "---\n  effort: high\n---\n", "  effort: high", true},
		{"quoted", "---\n\"thinking\": x\n---\n", "\"thinking\": x", true},
		{"space before colon", "---\nthinking : x\n---\n", "thinking : x", true},
		{"no opening fence", "name: x\n---\neffort: high\n", "", false},
		{"no closing fence", "---\nname: x\neffort: high\n", "", false},
	}
	for _, c := range cases {
		field, found := effortField(c.file)
		if field != c.field || found != c.found {
			t.Errorf("%s: effortField = %q, %v; want %q, %v", c.name, field, found, c.field, c.found)
		}
	}
}

// effortKey is a frontmatter line whose key is effort or thinking.
var effortKey = regexp.MustCompile(`^\s*["']?(effort|thinking)["']?\s*:`)

// effortField is the first effort or thinking line of the frontmatter of file, and whether the file has a
// frontmatter: a first line --- and a later line ---, either with or without a carriage return.
func effortField(file string) (string, bool) {
	lines := strings.Split(file, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], "\r")
	}
	if lines[0] != "---" {
		return "", false
	}
	end := slices.Index(lines[1:], "---")
	if end < 0 {
		return "", false
	}
	for _, line := range lines[1 : end+1] {
		if effortKey.MatchString(line) {
			return line, true
		}
	}
	return "", true
}

// The model of the foreman is the frontier row of the tier table of role-chain (S18): the parent takes the
// frontier model, as the shaper, the reviewer and the verifier do. Changing the cell changes the file of
// the foreman with theirs and leaves the file of a mid role alone.
func TestTheModelOfTheForemanIsTheFrontierRowOfTheTierTable(t *testing.T) {
	root := seededRoot(t)
	table := readText(t, modelsPath(root))
	changed := strings.Replace(table, "| frontier | `opus` |", "| frontier | `table-frontier` |", 1)
	if changed == table {
		t.Fatal("the frontier row of the tier table is not the shape the test edits")
	}
	if err := os.WriteFile(modelsPath(root), []byte(changed), 0o644); err != nil {
		t.Fatal(err)
	}

	files := artifactContents(t, root)

	for path, wantModel := range map[string]string{
		"agents/foreman.md": "table-frontier",
		"agents/shaper.md":  "table-frontier",
		"agents/builder.md": "sonnet",
	} {
		if !strings.Contains(files[path], "\nmodel: "+wantModel+"\n") {
			t.Errorf("%s does not carry the model %q of the table", path, wantModel)
		}
	}
}

// A foreman source that is not there is named after the sources of the eight roles, and nothing is
// written: a table with a source missing is not written half.
func TestAForemanSourceThatIsMissingIsNamedAndNothingIsWritten(t *testing.T) {
	root := seededRoot(t)
	if err := os.Remove(filepath.Join(root, "roles", "foreman", "body.md")); err != nil {
		t.Fatal(err)
	}

	checkErr := agentgen.Check(root)
	generateErr := agentgen.Generate(root)

	const missing = "  roles/foreman/body.md: missing (the source of the role is not in the repository)"
	if checkErr == nil || !strings.HasSuffix(checkErr.Error(), "\n"+missing) || strings.Contains(checkErr.Error(), "/foreman.") {
		t.Errorf("Check error = %v\nwant it to end with\n%s and to name no file of the foreman", checkErr, missing)
	}
	if generateErr == nil || !strings.HasPrefix(generateErr.Error(), "reading roles/foreman/body.md: ") {
		t.Errorf("Generate error = %v, want one that starts %q", generateErr, "reading roles/foreman/body.md: ")
	}
	if _, err := os.Stat(filepath.Join(root, "agents")); err == nil {
		t.Error("Generate wrote files although a source is missing")
	}
}

// The foreman file of the repository is what the generator writes (CI runs the same check through the
// command), and it names the subagent tool Agent so a main session can launch the roles.
func TestTheCommittedFileOfTheForemanIsWhatTheGeneratorWrites(t *testing.T) {
	root := repoRoot(t)
	want := artifactContents(t, root)["agents/foreman.md"]
	if want == "" {
		t.Fatal("the generator emits no agents/foreman.md for the repository")
	}
	if got := readText(t, filepath.Join(root, "agents", "foreman.md")); got != want {
		t.Errorf("agents/foreman.md is stale: run gadu-generate")
	}
	tools := strings.Split(strings.SplitN(strings.SplitN(want, "\ntools: ", 2)[1], "\n", 2)[0], ", ")
	hasAgent := false
	for _, tool := range tools {
		hasAgent = hasAgent || tool == "Agent"
	}
	if !hasAgent {
		t.Errorf("the tools of the foreman are %v, want the subagent tool Agent among them", tools)
	}
}

// The source of the foreman says what S2, S3 and S16 ask of it, and carries nothing the owner's draft
// (L9, with the Tier cell of L29) leaves out.
func TestTheForemanBodyCarriesTheDisciplineTheStagesAndTheTierCellOfL29(t *testing.T) {
	body := readText(t, filepath.Join(repoRoot(t), "roles", "foreman", "body.md"))

	for _, want := range []string{
		"You are the foreman of the labdrian workflow: you talk with the owner, explore read-only, and launch the labdrian role agents that do the work. You never write source, tests or another role's artifact.\n" + memoryLine + "\n",
		"Report only commands you ran and their exact results; an unrun check is \"not run\", never a pass.\n",
		"## Discipline\n",
		"## Stages\n",
		"## Never\n",
		"- End every reply that reports work with four lines: Decision, Outcome, Evidence, Next.\n",
		// L29: the Tier row's stop cell names the checklist rule it means.
		"| the tier is ambiguous (rule 4 of the `inception-pipeline` tier checklist: default up) and one question remains |",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("roles/foreman/body.md lacks %q", want)
		}
	}
	// S2: the body carries no gentle-ai review lifecycle, provider-defect handoff, Engram instruction or persona.
	for _, banned := range []string{"review lifecycle", "provider-defect", "engram", "persona"} {
		if strings.Contains(strings.ToLower(body), banned) {
			t.Errorf("roles/foreman/body.md holds %q, which S2 says it carries none of", banned)
		}
	}
	// S2: the document format is sent to odd-feature-doc.
	if !strings.Contains(body, "`odd-feature-doc` skill") {
		t.Error("roles/foreman/body.md does not send the feature document to the `odd-feature-doc` skill")
	}
	// S2 says "about 45 lines": the test takes that as a band of 35 to 55 lines.
	if n := strings.Count(body, "\n"); n < 35 || n > 55 {
		t.Errorf("roles/foreman/body.md has %d lines, want 35 to 55 (S2: about 45 lines)", n)
	}
	// The stage order has the ten stages of L9, one row each.
	for _, stage := range []string{"Authorize", "Tier", "Explore", "Shape", "Open", "Build", "Sweep, polish", "Review", "Deliver", "Close"} {
		if !strings.Contains(body, "\n| "+stage+" |") {
			t.Errorf("the Stages table has no row for %q", stage)
		}
	}
}
