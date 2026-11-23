package agentgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The body ClaudeInstructions reads out of the Claude file of every role is the body the generator wrote, the one
// CodexInstructions reads out of the Codex file of the same role. The foreman and each Claude-only role have no Codex
// file: their body is the source of roles/<name>/body.md as the role projects it, and a Claude-only role's file carries
// notice comment lines before name: (T3c-f, L51).
func TestClaudeInstructionsIsTheBodyOfTheClaudeFileOfEveryRole(t *testing.T) {
	artifacts, err := Artifacts(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, a := range artifacts {
		files[a.Path] = a.Content
	}
	for _, r := range roles {
		t.Run(r.name, func(t *testing.T) {
			want, err := CodexInstructions([]byte(files["codex/agents/"+r.name+".toml"]))
			if err != nil {
				t.Fatal(err)
			}
			got, err := ClaudeInstructions([]byte(files["agents/"+r.name+".md"]))
			if err != nil || got != want {
				t.Errorf("ClaudeInstructions = %q, %v; want %q", got, err, want)
			}
		})
	}
	type projected struct {
		name string
		body func(source string) (string, error)
	}
	withoutCodex := []projected{{foreman.name, foreman.body}}
	for _, r := range claudeOnlyRoles {
		withoutCodex = append(withoutCodex, projected{r.name, r.body})
	}
	for _, r := range withoutCodex {
		t.Run(r.name, func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join("..", "..", "roles", r.name, "body.md"))
			if err != nil {
				t.Fatal(err)
			}
			want, err := r.body(string(source))
			if err != nil {
				t.Fatal(err)
			}
			got, err := ClaudeInstructions([]byte(files["agents/"+r.name+".md"]))
			if err != nil || got != want {
				t.Errorf("ClaudeInstructions = %q, %v; want %q", got, err, want)
			}
		})
	}
}

// A file the generator did not write is refused, and the error says what is not its shape. Comment lines before name:
// belong only to the file of a Claude-only role with a notice, and only when the first of them is that role's notice
// header (T4s, L55; reviews 490a46c0b44b-3 F1 and -5 F3).
func TestClaudeInstructionsRefusesAFileTheGeneratorDidNotWrite(t *testing.T) {
	const keys = "name: reviewer\ndescription: d\nmodel: sonnet\ntools: Read, Grep, Glob\n"
	const foremanKeys = "name: foreman\ndescription: d\nmodel: sonnet\ntools: Read, Grep, Glob\n"
	const classifierKeys = "name: classifier\ndescription: d\nmodel: sonnet\ntools: Read, Grep, Glob\n"
	for _, c := range []struct{ name, file, want string }{
		{"a comment line in the file of a role with no notice", "---\n# x\n" + keys + "---\n\nbody\n", "carries no license notice"},
		{"a comment line in the file of the foreman", "---\n# x\n" + foremanKeys + "---\n\nbody\n", "carries no license notice"},
		{"comment lines that do not start with the notice header", "---\n# another header\n#\n" + classifierKeys + "---\n\nbody\n", "notice header"},
		{"an empty file", "", "opening"},
		{"no opening fence", keys + "---\n\nbody\n", "opening"},
		{"no closing fence", "---\n" + keys + "\nbody\n", "tools"},
		{"a missing key", "---\nname: reviewer\ndescription: d\ntools: Read\n---\n\nbody\n", "model"},
		{"keys out of order", "---\ndescription: d\nname: reviewer\nmodel: sonnet\ntools: Read\n---\n\nbody\n", "name"},
		{"a fifth key", "---\n" + keys + "thinking: high\n---\n\nbody\n", "closing"},
		{"no blank line after the fence", "---\n" + keys + "---\nbody\n", "blank"},
		{"a comment line with no space after the hash", "---\n#x\n" + keys + "---\n\nbody\n", "name"},
		{"a key line before name", "---\nfoo: bar\n" + keys + "---\n\nbody\n", "name"},
		{"a comment line between two keys", "---\nname: reviewer\n# c\ndescription: d\nmodel: sonnet\ntools: Read\n---\n\nbody\n", "description"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := ClaudeInstructions([]byte(c.file))
			if err == nil || !strings.Contains(err.Error(), c.want) || got != "" {
				t.Errorf("ClaudeInstructions = %q, %v; want an error naming %q", got, err, c.want)
			}
		})
	}
}
