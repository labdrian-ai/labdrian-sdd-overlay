package register

import (
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

// TestTOMLSplice_LocatesTableSpan proves TOMLSplice locates the exact
// header-to-next-header (or EOF) span of an existing
// [mcp_servers.longterm-mem] table and replaces only that span: an
// unrelated top-level key and an unrelated [mcp_servers.other] section
// (which comes both before AND after the located table) stay byte-
// identical, and the result stays valid TOML (12a.1; D9; R-018 "Unrelated
// sections and ordering are preserved").
func TestTOMLSplice_LocatesTableSpan(t *testing.T) {
	raw := []byte(`theme = "dark"

[mcp_servers.other]
command = "/usr/bin/other"
args = ["--foo"]

[mcp_servers.longterm-mem]
command = "/old/path/longterm-mem"
args = ["mcp"]

[mcp_servers.third]
command = "/usr/bin/third"
`)
	newSection := []byte("[mcp_servers.longterm-mem]\ncommand = \"/new/path/longterm-mem\"\nargs = [\"mcp\"]\n")

	got, err := TOMLSplice(raw, "mcp_servers", "longterm-mem", newSection)
	if err != nil {
		t.Fatalf("TOMLSplice returned error: %v", err)
	}

	var doc map[string]interface{}
	if err := toml.Unmarshal(got, &doc); err != nil {
		t.Fatalf("TOMLSplice result is not valid TOML: %v\n%s", err, got)
	}

	// The unrelated top-level key and BOTH unrelated sections (one before,
	// one after the replaced table) must be byte-identical to the input.
	// Prove this by removing exactly the known old table text from raw and
	// the known new table text from got, and comparing what remains.
	oldTableText := `[mcp_servers.longterm-mem]
command = "/old/path/longterm-mem"
args = ["mcp"]
`
	if !strings.Contains(string(raw), oldTableText) {
		t.Fatalf("test fixture setup error: old table text not found verbatim in raw fixture")
	}
	rawWithoutTable := strings.Replace(string(raw), oldTableText, "", 1)

	if !strings.Contains(string(got), string(newSection)) {
		t.Fatalf("TOMLSplice result does not contain the new table verbatim:\n%s", got)
	}
	gotWithoutTable := strings.Replace(string(got), string(newSection), "", 1)

	if rawWithoutTable != gotWithoutTable {
		t.Fatalf("bytes outside the spliced table span changed\n--- raw (table removed) ---\n%s\n--- got (table removed) ---\n%s", rawWithoutTable, gotWithoutTable)
	}

	if !strings.Contains(string(got), `command = "/usr/bin/other"`) {
		t.Fatalf("unrelated section %q was altered: %s", "other", got)
	}
	if !strings.Contains(string(got), `command = "/usr/bin/third"`) {
		t.Fatalf("unrelated section %q was altered: %s", "third", got)
	}
	if !strings.Contains(string(got), `theme = "dark"`) {
		t.Fatalf("unrelated top-level key was altered: %s", got)
	}
}

// TestTOMLSplice_AppendsAtEOFWhenAbsent proves TOMLSplice appends a new
// table at EOF (with a blank-line separator) when no header matching
// tableKey.memberKey exists yet, and every existing byte of raw is
// preserved unchanged.
func TestTOMLSplice_AppendsAtEOFWhenAbsent(t *testing.T) {
	raw := []byte(`theme = "dark"

[mcp_servers.other]
command = "/usr/bin/other"
`)
	newSection := []byte("[mcp_servers.longterm-mem]\ncommand = \"/bin/longterm-mem\"\nargs = [\"mcp\"]\n")

	got, err := TOMLSplice(raw, "mcp_servers", "longterm-mem", newSection)
	if err != nil {
		t.Fatalf("TOMLSplice returned error: %v", err)
	}

	var doc map[string]interface{}
	if err := toml.Unmarshal(got, &doc); err != nil {
		t.Fatalf("TOMLSplice result is not valid TOML: %v\n%s", err, got)
	}

	want := string(raw) + "\n" + string(newSection)
	if string(got) != want {
		t.Fatalf("TOMLSplice (absent) =\n%s\nwant =\n%s", got, want)
	}
}

// TestTOMLSplice_RemovesFirstMiddleLastTable proves TOMLRemove's blank-line
// handling is genuinely position-dependent (12b.1, D9, R-019 "Removing a
// span is harder than replacing one"): removing the FIRST table must drop
// its trailing blank-line separator (nothing precedes it), removing the
// LAST table must drop its leading separator (nothing follows it), and
// removing a MIDDLE table must drop exactly one of the two adjacent
// separators — never both (which would jam two sections together), never
// neither (which would leave a double blank line). Each case asserts the
// result stays valid TOML and is byte-identical to a document that never
// had the removed table at all.
func TestTOMLSplice_RemovesFirstMiddleLastTable(t *testing.T) {
	for _, tc := range []struct {
		name      string
		raw       string
		wantAfter string
	}{
		{
			name: "first table",
			raw: "[mcp_servers.longterm-mem]\ncommand = \"/bin/longterm-mem\"\n\n" +
				"[mcp_servers.other]\ncommand = \"/usr/bin/other\"\n",
			wantAfter: "[mcp_servers.other]\ncommand = \"/usr/bin/other\"\n",
		},
		{
			name: "middle table",
			raw: "[mcp_servers.a]\ncommand = \"/usr/bin/a\"\n\n" +
				"[mcp_servers.longterm-mem]\ncommand = \"/bin/longterm-mem\"\n\n" +
				"[mcp_servers.b]\ncommand = \"/usr/bin/b\"\n",
			wantAfter: "[mcp_servers.a]\ncommand = \"/usr/bin/a\"\n\n" +
				"[mcp_servers.b]\ncommand = \"/usr/bin/b\"\n",
		},
		{
			name: "last table",
			raw: "theme = \"dark\"\n\n" +
				"[mcp_servers.other]\ncommand = \"/usr/bin/other\"\n\n" +
				"[mcp_servers.longterm-mem]\ncommand = \"/bin/longterm-mem\"\n",
			wantAfter: "theme = \"dark\"\n\n[mcp_servers.other]\ncommand = \"/usr/bin/other\"\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := TOMLRemove([]byte(tc.raw), "mcp_servers", "longterm-mem")
			if err != nil {
				t.Fatalf("TOMLRemove returned error: %v", err)
			}
			var doc map[string]interface{}
			if err := toml.Unmarshal(got, &doc); err != nil {
				t.Fatalf("TOMLRemove result is not valid TOML: %v\n%s", err, got)
			}
			if string(got) != tc.wantAfter {
				t.Fatalf("TOMLRemove(%s) =\n%s\nwant (byte-identical to a document that never had it) =\n%s", tc.name, got, tc.wantAfter)
			}
		})
	}
}

// TestTOMLSplice_RemoveNotFoundReturnsError proves TOMLRemove refuses
// (rather than silently no-op-ing) when no header matching
// tableKey.memberKey exists — a defensive guard, since callers only reach
// TOMLRemove after confirming the table is present.
func TestTOMLSplice_RemoveNotFoundReturnsError(t *testing.T) {
	raw := []byte("[mcp_servers.other]\ncommand = \"/usr/bin/other\"\n")

	_, err := TOMLRemove(raw, "mcp_servers", "longterm-mem")
	if err == nil {
		t.Fatalf("TOMLRemove returned nil error for a table that does not exist")
	}
	assertHelperErrorAttribution(t, err, "mcp_servers", "longterm-mem")
}

// TestTOMLSplice_RemoveRefusesWhenTheTableEndIsUnprovable proves TOMLRemove
// shares TOMLSplice's own unprovable-span refusal (assertSpanIsWholeTable):
// a located span the line-oriented scan cannot prove is a whole table must
// never be removed, on the theory that "cannot prove where it ends" is
// exactly as dangerous for deletion as for replacement — arguably more so,
// since a bad delete cannot be undone by re-running install.
func TestTOMLSplice_RemoveRefusesWhenTheTableEndIsUnprovable(t *testing.T) {
	raw := []byte("[mcp_servers.longterm-mem]\ncommand = \"/old\"\nmatrix = [\n  [\"a\"]\n]\n\n[mcp_servers.other]\ncommand = \"/other\"\n")

	got, err := TOMLRemove(raw, "mcp_servers", "longterm-mem")
	if err == nil {
		t.Fatalf("TOMLRemove returned nil error; it would have removed:\n%s", got)
	}
	if got != nil {
		t.Fatalf("TOMLRemove returned %d bytes alongside its error; a refusal must yield nothing to write", len(got))
	}
}

// TestTOMLSplice_RefusesWhenTheTableEndIsUnprovable proves TOMLSplice
// never reports success on a span it could not delimit. The scan is
// line-oriented by design (D9: it must never decode and re-encode the
// document), so a line whose first non-space byte is '[' is
// indistinguishable from the next table header without a full TOML
// lexer — an array element on its own line, and a header-looking line
// inside a multi-line string, both look exactly like one. Ending the span
// there and writing over it strands the rest of the value after the
// replacement. Both shapes must be refused with nothing to write, not
// half-overwritten and reported as success.
func TestTOMLSplice_RefusesWhenTheTableEndIsUnprovable(t *testing.T) {
	newSection := []byte("[mcp_servers.longterm-mem]\ncommand = \"/new\"\nargs = [\"mcp\"]\n")

	for _, tc := range []struct {
		name string
		raw  string
	}{
		{
			name: "an array element on its own line looks like a header",
			raw:  "[mcp_servers.longterm-mem]\ncommand = \"/old\"\nmatrix = [\n  [\"a\"]\n]\n\n[mcp_servers.other]\ncommand = \"/other\"\n",
		},
		{
			name: "a header-looking line inside a multi-line string",
			raw:  "[mcp_servers.longterm-mem]\ncommand = \"/old\"\nnote = \"\"\"\n[mcp_servers.decoy]\n\"\"\"\n\n[mcp_servers.other]\ncommand = \"/other\"\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := TOMLSplice([]byte(tc.raw), "mcp_servers", "longterm-mem", newSection)
			if err == nil {
				t.Fatalf("TOMLSplice returned nil error; it would have written:\n%s", got)
			}
			if got != nil {
				t.Fatalf("TOMLSplice returned %d bytes alongside its error; a refusal must yield nothing to write", len(got))
			}
		})
	}
}
