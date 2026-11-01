package agentgen

import (
	"path/filepath"
	"strings"
	"testing"
)

// The body ClaudeInstructions reads out of the Claude file of every role is the body the generator wrote, the one
// CodexInstructions reads out of the Codex file of the same role.
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
}

// A file the generator did not write is refused, and the error says what is not its shape.
func TestClaudeInstructionsRefusesAFileTheGeneratorDidNotWrite(t *testing.T) {
	const keys = "name: reviewer\ndescription: d\nmodel: sonnet\ntools: Read, Grep, Glob\n"
	for _, c := range []struct{ name, file, want string }{
		{"an empty file", "", "opening"},
		{"no opening fence", keys + "---\n\nbody\n", "opening"},
		{"no closing fence", "---\n" + keys + "\nbody\n", "tools"},
		{"a missing key", "---\nname: reviewer\ndescription: d\ntools: Read\n---\n\nbody\n", "model"},
		{"keys out of order", "---\ndescription: d\nname: reviewer\nmodel: sonnet\ntools: Read\n---\n\nbody\n", "name"},
		{"a fifth key", "---\n" + keys + "thinking: high\n---\n\nbody\n", "closing"},
		{"no blank line after the fence", "---\n" + keys + "---\nbody\n", "blank"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := ClaudeInstructions([]byte(c.file))
			if err == nil || !strings.Contains(err.Error(), c.want) || got != "" {
				t.Errorf("ClaudeInstructions = %q, %v; want an error naming %q", got, err, c.want)
			}
		})
	}
}
