package register

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestJSONSplice_LocatesAndReplacesMemberSpan proves Splice locates the
// exact byte span of an existing member via json.Decoder.InputOffset() and
// replaces only that span: every other byte of the document, including an
// unrelated sibling member before and after it, stays byte-identical
// (11a.1; D9; R-016/R-017 "Unrelated entries are preserved").
func TestJSONSplice_LocatesAndReplacesMemberSpan(t *testing.T) {
	raw := []byte(`{
  "mcpServers": {
    "other": {
      "type": "stdio",
      "command": "/usr/bin/other"
    },
    "longterm-mem": {
      "type": "stdio",
      "command": "/old/path/longterm-mem",
      "args": ["mcp"]
    },
    "third": {
      "type": "stdio",
      "command": "/usr/bin/third"
    }
  }
}
`)
	newValue := json.RawMessage(`{"type":"stdio","command":"/new/path/longterm-mem","args":["mcp"]}`)

	got, err := Splice(raw, "mcpServers", "longterm-mem", newValue)
	if err != nil {
		t.Fatalf("Splice returned error: %v", err)
	}

	if !json.Valid(got) {
		t.Fatalf("Splice result is not valid JSON:\n%s", got)
	}

	// The unrelated "other" and "third" entries, and everything around
	// them, must be byte-identical to the input. Prove this by removing
	// exactly the known old member span from raw and the known new member
	// span from got, and comparing what remains.
	oldMemberText := `"longterm-mem": {
      "type": "stdio",
      "command": "/old/path/longterm-mem",
      "args": ["mcp"]
    }`
	if !strings.Contains(string(raw), oldMemberText) {
		t.Fatalf("test fixture setup error: old member text not found verbatim in raw fixture")
	}
	rawWithoutMember := strings.Replace(string(raw), oldMemberText, "", 1)

	newMemberText := `"longterm-mem": {"type":"stdio","command":"/new/path/longterm-mem","args":["mcp"]}`
	if !strings.Contains(string(got), newMemberText) {
		t.Fatalf("Splice result does not contain the new member verbatim:\n%s", got)
	}
	gotWithoutMember := strings.Replace(string(got), newMemberText, "", 1)

	if rawWithoutMember != gotWithoutMember {
		t.Fatalf("bytes outside the spliced member span changed\n--- raw (member removed) ---\n%s\n--- got (member removed) ---\n%s", rawWithoutMember, gotWithoutMember)
	}

	// Decode and check the actual replaced value took effect and the
	// unrelated entries are semantically untouched.
	var doc map[string]map[string]json.RawMessage
	if err := json.Unmarshal(got, &doc); err != nil {
		t.Fatalf("failed to decode spliced result: %v", err)
	}
	servers := doc["mcpServers"]
	if string(servers["longterm-mem"]) != string(newValue) {
		t.Fatalf("longterm-mem entry = %s, want %s", servers["longterm-mem"], newValue)
	}
	if !strings.Contains(string(servers["other"]), "/usr/bin/other") {
		t.Fatalf("unrelated entry %q was altered: %s", "other", servers["other"])
	}
	if !strings.Contains(string(servers["third"]), "/usr/bin/third") {
		t.Fatalf("unrelated entry %q was altered: %s", "third", servers["third"])
	}
}

// TestJSONSplice_InsertsWhenAbsent proves Splice inserts a new member when
// none exists yet under the container, and the result is valid JSON
// (11a.2). Unrelated sibling entries must survive untouched too.
func TestJSONSplice_InsertsWhenAbsent(t *testing.T) {
	raw := []byte(`{
  "mcpServers": {
    "other": {
      "type": "stdio",
      "command": "/usr/bin/other"
    }
  }
}
`)
	newValue := json.RawMessage(`{"type":"stdio","command":"/bin/longterm-mem","args":["mcp"]}`)

	got, err := Splice(raw, "mcpServers", "longterm-mem", newValue)
	if err != nil {
		t.Fatalf("Splice returned error: %v", err)
	}

	if !json.Valid(got) {
		t.Fatalf("Splice result is not valid JSON:\n%s", got)
	}

	var doc map[string]map[string]json.RawMessage
	if err := json.Unmarshal(got, &doc); err != nil {
		t.Fatalf("failed to decode spliced result: %v", err)
	}
	servers := doc["mcpServers"]
	if len(servers) != 2 {
		t.Fatalf("mcpServers has %d members, want 2 (other + longterm-mem): %v", len(servers), servers)
	}
	if string(servers["longterm-mem"]) != string(newValue) {
		t.Fatalf("longterm-mem entry = %s, want %s", servers["longterm-mem"], newValue)
	}
	if !strings.Contains(string(servers["other"]), "/usr/bin/other") {
		t.Fatalf("unrelated entry %q was altered: %s", "other", servers["other"])
	}

	// Removing exactly the newly inserted member text from got must leave
	// the original bytes untouched: proves insertion only ADDS bytes and
	// never rewrites anything else in the document.
	insertedText := `,
    "longterm-mem": {"type":"stdio","command":"/bin/longterm-mem","args":["mcp"]}`
	if !strings.Contains(string(got), insertedText) {
		t.Fatalf("Splice result does not contain the expected inserted text verbatim:\n%s", got)
	}
	gotWithoutInsertion := strings.Replace(string(got), insertedText, "", 1)
	if gotWithoutInsertion != string(raw) {
		t.Fatalf("bytes outside the inserted member changed\n--- raw ---\n%s\n--- got (insertion removed) ---\n%s", raw, gotWithoutInsertion)
	}
}

// TestJSONSplice_ContainerKeyNotFound proves locate's error path for a
// document whose root has no member named containerKey at all — a
// deliberately out-of-scope case for 11a (see apply-progress.md), asserted
// so the guard clause is real, not dead code.
func TestJSONSplice_ContainerKeyNotFound(t *testing.T) {
	raw := []byte(`{"unrelated": {"foo": "bar"}}`)

	_, err := Splice(raw, "mcpServers", "longterm-mem", json.RawMessage(`{}`))
	if err == nil {
		t.Fatalf("Splice returned nil error for a document with no %q container", "mcpServers")
	}
	if !strings.Contains(err.Error(), "register:") {
		t.Fatalf("error %q is not prefixed with the package name", err.Error())
	}
}
