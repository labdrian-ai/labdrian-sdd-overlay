package agentgen

import (
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
