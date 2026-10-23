package agentgen_test

// The projection of the role agents (per-role design v3, sections 2 and 5): each role has one source,
// roles/<role>/body.md, and the generator writes three files from it, for Claude Code, Pi and Codex.
// The tables below are written out here, not read from the generator, so that the generator is
// checked against a table it does not own. They copy the Claude agent files that exist before this
// slice (their descriptions, models and tools byte for byte) and add what the slice decides: the
// Pi model and tool names, and the effort of each role.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/agentgen"
)

// engramLine is the second line of the core prefix: the one that sends a role to Engram for its
// artifacts. A role with no Engram tool is not sent there (slice 2, L3: the projection drops it).
const engramLine = "Read every artifact the prompt locates (file path, or Engram topic via mem_get_observation) before acting. A missing required locator is a blocker: report it, never substitute another store."

const (
	piSonnet = "pi-claude-cli/claude-sonnet-5"
	piOpus   = "pi-claude-cli/claude-opus-5"
	piHaiku  = "pi-claude-cli/claude-haiku-4-5"

	builderClaudeTools  = "Read, Edit, Write, Glob, Grep, Bash, mcp__engram__mem_search, mcp__plugin_engram_engram__mem_search, mcp__engram__mem_get_observation, mcp__plugin_engram_engram__mem_get_observation, mcp__engram__mem_save, mcp__plugin_engram_engram__mem_save, mcp__engram__mem_update, mcp__plugin_engram_engram__mem_update, mcp__codegraph__codegraph_explore"
	shaperClaudeTools   = "Read, Edit, Write, Grep, Glob, mcp__engram__mem_search, mcp__plugin_engram_engram__mem_search, mcp__engram__mem_get_observation, mcp__plugin_engram_engram__mem_get_observation, mcp__engram__mem_save, mcp__plugin_engram_engram__mem_save, mcp__codegraph__codegraph_explore"
	polisherClaudeTools = "Read, Glob, Grep, mcp__plugin_engram_engram__mem_search, mcp__plugin_engram_engram__mem_get_observation, mcp__codegraph__codegraph_explore"
	prototypeClaudeTool = "Read, Glob, Grep, Bash, Write, Edit, mcp__plugin_engram_engram__mem_search, mcp__plugin_engram_engram__mem_get_observation"
	sweeperClaudeTools  = "Read, Edit, Glob, Grep, Bash, mcp__engram__mem_search, mcp__plugin_engram_engram__mem_search, mcp__engram__mem_get_observation, mcp__plugin_engram_engram__mem_get_observation, mcp__codegraph__codegraph_explore"
	deliveryClaudeTools = "Read, Edit, Write, Glob, Bash, mcp__engram__mem_search, mcp__plugin_engram_engram__mem_search, mcp__engram__mem_get_observation, mcp__plugin_engram_engram__mem_get_observation, mcp__engram__mem_save, mcp__plugin_engram_engram__mem_save, mcp__codegraph__codegraph_explore"
)

// wantRole is what is expected of one role in the three files.
type wantRole struct {
	name, description string
	claudeModel       string
	claudeTools       string
	piModel           string
	piTools           string
	effort            string
	// engram says that the role has an Engram tool, so its body keeps the line that sends it to Engram.
	engram bool
}

// wantRoles are the eight roles in the order of the table of per-role design v3, section 2.
var wantRoles = []wantRole{
	{"builder", "Implements the tasks a labdrian workflow names, or fixes confirmed ledger ids, and reports the evidence of each work unit.",
		"sonnet", builderClaudeTools, piSonnet, "read, edit, write, find, grep, bash, mem_search, mem_get_observation, mem_save, mem_update, codegraph", "high", true},
	{"shaper", "Shapes one planning artifact of a labdrian workflow (proposal, spec, design or tasks) and returns open product decisions as risks.",
		"opus", shaperClaudeTools, piOpus, "read, edit, write, grep, find, mem_search, mem_get_observation, mem_save, codegraph", "high", true},
	{"polisher", "Polisher role, the read-only blind judge of an immutable target (judgment-day launches it for both judges); returns one JSON findings object.",
		"sonnet", polisherClaudeTools, piSonnet, "read, find, grep, mem_search, mem_get_observation, codegraph", "high", true},
	{"prototyper", "Answers listed feasibility questions with throwaway spikes in a disposable worktree and returns kill, narrow or continue per question.",
		"sonnet", prototypeClaudeTool, piSonnet, "read, find, grep, bash, write, edit, mem_search, mem_get_observation", "medium", true},
	{"sweeper", "Removal-only cleanup of dead code and stale prose that a work unit left behind; commits one chore(sweep) commit or none.",
		"sonnet", sweeperClaudeTools, piSonnet, "read, edit, find, grep, bash, mem_search, mem_get_observation, codegraph", "medium", true},
	{"reviewer", "Reviewer role, the read-only reader of a committed diff and its detached worktree (send base, head, diff_sha256, the diff, the worktree path and the lens rows); returns one JSON object of findings.",
		"sonnet", "Read, Grep, Glob", piSonnet, "read, grep, find", "high", false},
	{"verifier", "Verifier role, runs the checks a prompt names and reproduces the findings it lists (send the finding ids, the exact checks and the worktree); returns reproduced, not-reproduced or not-run per id.",
		"sonnet", "Read, Grep, Glob, Bash, mcp__plugin_engram_engram__mem_get_observation", piSonnet, "read, grep, find, bash, mem_get_observation", "medium", true},
	{"delivery", "Archives a finished labdrian workflow by copying and moving its artifacts mechanically, recording unfinished work as unfinished.",
		"haiku", deliveryClaudeTools, piHaiku, "read, edit, write, find, bash, mem_search, mem_get_observation, mem_save, codegraph", "low", true},
}

// seededRoot is a root with the role sources of this repository under roles/ and nothing else: all
// that Generate and Check read.
func seededRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(repoRoot(t), "roles")
	err := filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(root, "roles", rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copying the role sources: %v", err)
	}
	return root
}

func readText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// wantBody is the body of a role in the three files: its source, without the line that sends it to
// Engram when it has no Engram tool.
func wantBody(t *testing.T, root string, r wantRole) string {
	t.Helper()
	source := readText(t, filepath.Join(root, "roles", r.name, "body.md"))
	if r.engram {
		return source
	}
	if strings.Count(source, engramLine+"\n") != 1 {
		t.Fatalf("roles/%s/body.md must carry the Engram line of the core prefix exactly once", r.name)
	}
	return strings.Replace(source, engramLine+"\n", "", 1)
}

// tableOf is the table of what the generator emits for the root of seededRoot.
func tableOf(t *testing.T) []agentgen.Artifact {
	t.Helper()
	table, err := agentgen.Artifacts(seededRoot(t))
	if err != nil {
		t.Fatalf("Artifacts: %v", err)
	}
	return table
}

// generated is a root with the role sources, after Generate has written everything from them.
func generated(t *testing.T) string {
	t.Helper()
	root := seededRoot(t)
	if err := agentgen.Generate(root); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return root
}

func TestGenerateWritesTheClaudePiAndCodexFileOfEveryRole(t *testing.T) {
	root := generated(t)
	for _, r := range wantRoles {
		for _, path := range []string{"agents/" + r.name + ".md", "pi/agents/" + r.name + ".md", "codex/agents/" + r.name + ".toml"} {
			if data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path))); err != nil || len(data) == 0 {
				t.Errorf("%s: (%d bytes, %v), want a file with something in it", path, len(data), err)
			}
		}
	}
}

// The Claude file of a role is its frontmatter (name, description, model, tools, in that order), a
// blank line and its body, nothing else: the files that exist before this slice, byte for byte.
func TestTheClaudeFileOfARoleIsItsFrontmatterAndItsBody(t *testing.T) {
	root := generated(t)
	for _, r := range wantRoles {
		t.Run(r.name, func(t *testing.T) {
			want := "---\nname: " + r.name + "\ndescription: " + r.description + "\nmodel: " + r.claudeModel + "\ntools: " + r.claudeTools + "\n---\n\n" + wantBody(t, root, r)
			if got := readText(t, filepath.Join(root, "agents", r.name+".md")); got != want {
				t.Errorf("agents/%s.md =\n%s\nwant\n%s", r.name, got, want)
			}
		})
	}
}

// The Pi file of a role has the frontmatter gentle-pi reads (name, description, a provider-prefixed
// model, thinking and a comma list of tools, which stays one inline scalar) and the same body.
func TestThePiFileOfARoleIsItsFrontmatterAndItsBody(t *testing.T) {
	root := generated(t)
	for _, r := range wantRoles {
		t.Run(r.name, func(t *testing.T) {
			want := "---\nname: " + r.name + "\ndescription: " + r.description + "\nmodel: " + r.piModel + "\nthinking: " + r.effort + "\ntools: " + r.piTools + "\n---\n\n" + wantBody(t, root, r)
			if got := readText(t, filepath.Join(root, "pi", "agents", r.name+".md")); got != want {
				t.Errorf("pi/agents/%s.md =\n%s\nwant\n%s", r.name, got, want)
			}
		})
	}
}

// The Codex file of a role has the four fields of the design and no model (the parent passes it at
// spawn, D14) and no tools (an agent file of Codex has no allowlist): its instructions are the body.
func TestTheCodexFileOfARoleHasTheFourFieldsOfTheDesignAndTheBody(t *testing.T) {
	root := generated(t)
	for _, r := range wantRoles {
		t.Run(r.name, func(t *testing.T) {
			keys, values := decodeCodex(t, readText(t, filepath.Join(root, "codex", "agents", r.name+".toml")))
			if want := []string{"name", "description", "developer_instructions", "model_reasoning_effort"}; strings.Join(keys, ",") != strings.Join(want, ",") {
				t.Fatalf("keys = %v, want %v", keys, want)
			}
			for key, want := range map[string]string{
				"name": r.name, "description": r.description, "model_reasoning_effort": r.effort,
				"developer_instructions": wantBody(t, root, r),
			} {
				if values[key] != want {
					t.Errorf("%s = %q, want %q", key, values[key], want)
				}
			}
		})
	}
}

// A role with no Engram tool is not sent to Engram by its instructions, in any of the three files; a
// role with one keeps the line. Only the reviewer has none.
func TestARoleWithNoEngramToolIsNotSentToEngramInAnyFile(t *testing.T) {
	root := generated(t)
	for _, r := range wantRoles {
		t.Run(r.name, func(t *testing.T) {
			for _, path := range []string{"agents/" + r.name + ".md", "pi/agents/" + r.name + ".md", "codex/agents/" + r.name + ".toml"} {
				has := strings.Contains(readText(t, filepath.Join(root, filepath.FromSlash(path))), "Engram topic via mem_get_observation")
				if has != r.engram {
					t.Errorf("%s carries the Engram line = %v, want %v", path, has, r.engram)
				}
			}
		})
	}
}

// The files of the roles are plain: no generated-file comment (the Claude files of the roles carry none
// today, and what Pi reads after the frontmatter is the prompt of the agent), only ASCII.
func TestTheFilesOfTheRolesAreAsciiWithNoGeneratedHeader(t *testing.T) {
	root := generated(t)
	for _, r := range wantRoles {
		for _, path := range []string{"agents/" + r.name + ".md", "pi/agents/" + r.name + ".md", "codex/agents/" + r.name + ".toml"} {
			text := readText(t, filepath.Join(root, filepath.FromSlash(path)))
			if strings.Contains(text, "<!--") || strings.Contains(strings.ToLower(text), "do not edit") {
				t.Errorf("%s carries a generated-file header", path)
			}
			for i := 0; i < len(text); i++ {
				if text[i] > 0x7e || (text[i] < 0x20 && text[i] != '\n') {
					t.Fatalf("%s has a byte that is not plain ASCII text at offset %d", path, i)
				}
			}
		}
	}
}

// The tool names of Pi are the ones the installed sources register (verified 2026-10-22 against Pi
// 1.3.0, gentle-pi 4.3.0 and gentle-engram 0.4.2): the built-in tools of Pi, the tools gentle-engram
// registers by the bare name of the Engram tool, and the one native tool gentle-pi names codegraph.
// gentle-pi drops a name it does not know without a word, so a name outside this set would take a
// tool from a role and say nothing.
func TestThePiToolsOfEveryRoleAreNamesTheInstalledSourcesRegister(t *testing.T) {
	known := map[string]bool{
		"read": true, "edit": true, "write": true, "bash": true, "grep": true, "find": true,
		"mem_search": true, "mem_get_observation": true, "mem_save": true, "mem_update": true, "codegraph": true,
	}
	for _, r := range wantRoles {
		for _, tool := range strings.Split(r.piTools, ", ") {
			if !known[tool] {
				t.Errorf("the Pi tools of %s name %q, which no installed source registers", r.name, tool)
			}
		}
	}
}

// What the source of a role holds reaches the instructions of Codex whole, whatever characters it
// has: backslashes, runs of quotation marks and a quotation mark before the end.
func TestTheCodexInstructionsAreTheBodyWhateverCharactersItHolds(t *testing.T) {
	root := seededRoot(t)
	body := strings.Replace(readText(t, filepath.Join(root, "roles", "builder", "body.md")), "Artifacts are in English.", "A path C:\\work\\x, a run \"\"\"\"\"\" of quotes, one \"\" pair and a tab\there; the end is quoted: \"", 1)
	if err := os.WriteFile(filepath.Join(root, "roles", "builder", "body.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := agentgen.Generate(root); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	_, values := decodeCodex(t, readText(t, filepath.Join(root, "codex", "agents", "builder.toml")))

	if values["developer_instructions"] != body {
		t.Errorf("developer_instructions =\n%q\nwant\n%q", values["developer_instructions"], body)
	}
}

// A character that TOML does not allow in a string cannot be written, and the generator says so
// instead of writing a file that Codex would refuse.
func TestARoleSourceWithAControlCharacterIsRefusedBeforeAnythingIsWritten(t *testing.T) {
	for name, bad := range map[string]string{"a carriage return": "\r", "a bell": "\a", "an escape": "\x1b"} {
		t.Run(name, func(t *testing.T) {
			root := seededRoot(t)
			body := readText(t, filepath.Join(root, "roles", "builder", "body.md")) + "x" + bad + "y\n"
			if err := os.WriteFile(filepath.Join(root, "roles", "builder", "body.md"), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}

			err := agentgen.Generate(root)

			if err == nil || !strings.Contains(err.Error(), "builder") || !strings.Contains(err.Error(), "control character") {
				t.Errorf("Generate error = %v, want one that names the role and the control character", err)
			}
			if _, statErr := os.Stat(filepath.Join(root, "agents")); statErr == nil {
				t.Error("Generate wrote files although a source was refused")
			}
		})
	}
}

// decodeCodex reads a Codex agent file the way a TOML reader reads the subset the generator writes:
// a key, an equals sign and a basic string on one line, or a multi-line basic string that ends at its
// closing delimiter. The only escapes it knows are the two the generator writes; any other fails.
func decodeCodex(t *testing.T, text string) (keys []string, values map[string]string) {
	t.Helper()
	values = map[string]string{}
	for rest := text; rest != ""; {
		line, after, _ := strings.Cut(rest, "\n")
		key, value, ok := strings.Cut(line, " = ")
		if !ok {
			t.Fatalf("not a key = value line: %q", line)
		}
		keys = append(keys, key)
		if value == `"""` {
			values[key], rest = decodeMultiline(t, after)
			continue
		}
		if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
			t.Fatalf("%s is not a basic string: %q", key, value)
		}
		values[key], _ = unescape(t, value[1:len(value)-1], false)
		rest = after
	}
	return keys, values
}

// decodeMultiline reads the body of a multi-line basic string whose opening delimiter and its newline
// are already read, up to the closing delimiter, and returns what follows the line of the delimiter.
func decodeMultiline(t *testing.T, text string) (decoded, rest string) {
	t.Helper()
	decoded, tail := unescape(t, text, true)
	_, after, _ := strings.Cut(tail, "\n")
	return decoded, after
}

// unescape decodes the escapes \\ and \" of text. With multiline it stops at the closing delimiter (a
// run of three to five quotation marks, of which the last three close) and returns the rest.
func unescape(t *testing.T, text string, multiline bool) (decoded, rest string) {
	t.Helper()
	var b strings.Builder
	for i := 0; i < len(text); i++ {
		switch c := text[i]; {
		case c == '\\':
			i++
			if i >= len(text) || (text[i] != '\\' && text[i] != '"') {
				t.Fatalf("an escape this decoder does not know in %q", text)
			}
			b.WriteByte(text[i])
		case c == '"' && multiline:
			run := 0
			for i+run < len(text) && text[i+run] == '"' {
				run++
			}
			if run >= 3 {
				b.WriteString(strings.Repeat(`"`, run-3))
				return b.String(), text[i+run:]
			}
			b.WriteString(strings.Repeat(`"`, run))
			i += run - 1
		case c == '"':
			t.Fatalf("an unescaped quotation mark in a basic string: %q", text)
		default:
			b.WriteByte(c)
		}
	}
	if multiline {
		t.Fatalf("a multi-line string with no closing delimiter: %q", text)
	}
	return b.String(), ""
}
