package agentgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The two forms of TOML string the Codex files use, written out exactly. roles_test.go reads whole
// files back through a decoder; these say what each form looks like for the characters that need care.

func TestTomlStringEscapesTheBackslashAndTheQuotationMarkAndNothingElse(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{`plain`, `"plain"`},
		{`a\b"c`, `"a\\b\"c"`},
		{"a\tb", "\"a\tb\""},
		{`it's (a) {b}`, `"it's (a) {b}"`},
	} {
		got, err := tomlString(c.in)
		if err != nil || got != c.want {
			t.Errorf("tomlString(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
}

func TestTomlMultilineKeepsTheLinesAndEscapesWhatWouldCloseOrEscapeTheString(t *testing.T) {
	for _, c := range []struct{ name, in, want string }{
		{"lines", "one\ntwo\n", "\"\"\"\none\ntwo\n\"\"\""},
		{"a backslash", "a\\b\n", "\"\"\"\na\\\\b\n\"\"\""},
		{"one and two quotation marks", "a\" b\"\" c\n", "\"\"\"\na\" b\"\" c\n\"\"\""},
		{"a run of three", "x\"\"\"y\n", "\"\"\"\nx\"\"\\\"y\n\"\"\""},
		{"a run of four", "x\"\"\"\"y\n", "\"\"\"\nx\"\"\\\"\"y\n\"\"\""},
		{"a quotation mark at the end", "x\"", "\"\"\"\nx\"\"\"\""},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := tomlMultiline(c.in)
			if err != nil || got != c.want {
				t.Errorf("tomlMultiline(%q) = %q, %v; want %q", c.in, got, err, c.want)
			}
		})
	}
}

// A character TOML does not allow is refused by name and by place, whichever form it was to be
// written in; a newline is allowed in the multi-line form only.
func TestTheTomlFormsRefuseControlCharacters(t *testing.T) {
	for _, in := range []string{"a\x00b", "a\rb", "a\x1bb", "a\x7fb"} {
		for name, write := range map[string]func(string) (string, error){"string": tomlString, "multiline": tomlMultiline} {
			if _, err := write(in); err == nil || !strings.Contains(err.Error(), "control character") {
				t.Errorf("%s of %q: error = %v, want one that names a control character", name, in, err)
			}
		}
	}
	if _, err := tomlString("a\nb"); err == nil {
		t.Error("a newline in a string on one line was accepted")
	}
	if _, err := tomlMultiline("a\nb"); err != nil {
		t.Errorf("a newline in a multi-line string was refused: %v", err)
	}
}

// CodexInstructions is the exact inverse of codexFile: for every role, and for a body that holds a
// backslash, a run of three quotation marks and a run of four, it gives the body back.
func TestCodexInstructionsIsTheInverseOfTheCodexFile(t *testing.T) {
	bodies := map[string]string{
		"a backslash":        "one \\ two \\\\ three\n",
		"a run of three":     "x\"\"\"y\n",
		"a run of four":      "x\"\"\"\"y\n",
		"a quote at the end": "x\"",
		"two at the end":     "x\"\"",
		"no body line":       "",
	}
	for _, r := range roles {
		data, err := os.ReadFile(filepath.Join("..", "..", "roles", r.name, "body.md"))
		if err != nil {
			t.Fatal(err)
		}
		bodies["role "+r.name] = string(data)
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			file, err := roles[0].codexFile(body)
			if err != nil {
				t.Fatal(err)
			}
			got, err := CodexInstructions([]byte(file))
			if err != nil || got != body {
				t.Errorf("CodexInstructions = %q, %v; want %q", got, err, body)
			}
		})
	}
}

// A file the generator did not write is refused, and the error says what is not its shape.
func TestCodexInstructionsRefusesAFileTheGeneratorDidNotWrite(t *testing.T) {
	const tail = "\nmodel_reasoning_effort = \"high\"\n"
	head := "name = \"reviewer\"\ndescription = \"d\"\n"
	for _, c := range []struct{ name, file, want string }{
		{"no developer_instructions", head + "model_reasoning_effort = \"high\"\n", "developer_instructions"},
		{"an unterminated string", head + "developer_instructions = \"\"\"\nabc\n", "unterminated"},
		{"an unknown escape", head + "developer_instructions = \"\"\"\na\\nb\n\"\"\"" + tail, "escape"},
		{"a fifth key", head + "developer_instructions = \"\"\"\nabc\n\"\"\"" + tail + "model = \"x\"\n", "follows"},
		{"keys out of order", "description = \"d\"\nname = \"reviewer\"\ndeveloper_instructions = \"\"\"\nabc\n\"\"\"" + tail, "name"},
		{"an empty file", "", "name"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := CodexInstructions([]byte(c.file))
			if err == nil || !strings.Contains(err.Error(), c.want) || got != "" {
				t.Errorf("CodexInstructions = %q, %v; want an error naming %q", got, err, c.want)
			}
		})
	}
}
