package register

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteMember_AtomicReplaceWithBackup proves the file-level wrapper
// (11a.3): a .bak of the ORIGINAL bytes is written, the target file ends up
// exactly what Splice would have produced in memory, and no stray tmp file
// is left behind in the same directory (same-dir tmp+rename, D9).
func TestWriteMember_AtomicReplaceWithBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "claude.json")
	original := []byte(`{
  "mcpServers": {
    "other": {
      "type": "stdio",
      "command": "/usr/bin/other"
    }
  }
}
`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("failed to seed fixture file: %v", err)
	}

	newValue := json.RawMessage(`{"type":"stdio","command":"/bin/longterm-mem","args":["mcp"]}`)
	if err := WriteMember(path, "mcpServers", "longterm-mem", newValue); err != nil {
		t.Fatalf("WriteMember returned error: %v", err)
	}

	gotFile, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read result file: %v", err)
	}
	wantFile, err := Splice(original, "mcpServers", "longterm-mem", newValue)
	if err != nil {
		t.Fatalf("Splice (for comparison) returned error: %v", err)
	}
	if string(gotFile) != string(wantFile) {
		t.Fatalf("target file = %s, want (from Splice) %s", gotFile, wantFile)
	}

	bak, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatalf("failed to read .bak file: %v", err)
	}
	if string(bak) != string(original) {
		t.Fatalf(".bak = %s, want original untouched bytes %s", bak, original)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to list dir: %v", err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Fatalf("stray temp file left behind: %s", e.Name())
		}
	}
}

// TestWriteMember_InvalidResultLeavesOriginalUntouched proves validate
// gates the rename: a splice that would produce invalid JSON is rejected
// BEFORE any filesystem mutation — the target file's bytes are unchanged,
// and no .bak is created, since the guard fires before the backup step,
// not after it.
func TestWriteMember_InvalidResultLeavesOriginalUntouched(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "claude.json")
	original := []byte(`{
  "mcpServers": {
    "other": {
      "type": "stdio",
      "command": "/usr/bin/other"
    }
  }
}
`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("failed to seed fixture file: %v", err)
	}

	malformed := json.RawMessage(`{"type":"stdio", not valid json`)
	err := WriteMember(path, "mcpServers", "longterm-mem", malformed)
	if err == nil {
		t.Fatalf("WriteMember returned nil error for a malformed member value")
	}
	assertHelperErrorAttribution(t, err, path)

	gotFile, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read target file: %v", err)
	}
	if string(gotFile) != string(original) {
		t.Fatalf("target file was modified despite the invalid result:\n%s", gotFile)
	}

	if _, statErr := os.Stat(path + ".bak"); !os.IsNotExist(statErr) {
		t.Fatalf(".bak was created even though validation failed before any write (stat err: %v)", statErr)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to list dir: %v", err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Fatalf("stray temp file left behind after a rejected write: %s", e.Name())
		}
	}
}

// TestRemoveMember_AtomicRemoveWithBackup proves the file-level removal
// wrapper (12b.2, R-019): a .bak of the ORIGINAL bytes (with the member
// still present) is written, the target file ends up exactly what Remove
// would have produced in memory, and no stray tmp file is left behind.
func TestRemoveMember_AtomicRemoveWithBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "claude.json")
	original := []byte(`{
  "mcpServers": {
    "other": {
      "type": "stdio",
      "command": "/usr/bin/other"
    },
    "longterm-mem": {"type":"stdio","command":"/bin/longterm-mem","args":["mcp"]}
  }
}
`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("failed to seed fixture file: %v", err)
	}

	if err := RemoveMember(path, "mcpServers", "longterm-mem"); err != nil {
		t.Fatalf("RemoveMember returned error: %v", err)
	}

	gotFile, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read result file: %v", err)
	}
	wantFile, err := Remove(original, "mcpServers", "longterm-mem")
	if err != nil {
		t.Fatalf("Remove (for comparison) returned error: %v", err)
	}
	if string(gotFile) != string(wantFile) {
		t.Fatalf("target file = %s, want (from Remove) %s", gotFile, wantFile)
	}
	if strings.Contains(string(gotFile), "longterm-mem") {
		t.Fatalf("target file still mentions longterm-mem after RemoveMember: %s", gotFile)
	}

	bak, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatalf("failed to read .bak file: %v", err)
	}
	if string(bak) != string(original) {
		t.Fatalf(".bak = %s, want original untouched bytes %s", bak, original)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to list dir: %v", err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Fatalf("stray temp file left behind: %s", e.Name())
		}
	}
}

// TestRemoveMember_NotPresentLeavesOriginalUntouched proves RemoveMember's
// guard fires BEFORE any filesystem mutation when memberKey does not exist:
// the target file is unchanged and no .bak is created.
func TestRemoveMember_NotPresentLeavesOriginalUntouched(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "claude.json")
	original := []byte(`{"mcpServers":{"other":{"type":"stdio"}}}`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("failed to seed fixture file: %v", err)
	}

	err := RemoveMember(path, "mcpServers", "longterm-mem")
	if err == nil {
		t.Fatalf("RemoveMember returned nil error for a member that does not exist")
	}
	assertHelperErrorAttribution(t, err, path, "longterm-mem", "mcpServers")

	gotFile, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read target file: %v", err)
	}
	if string(gotFile) != string(original) {
		t.Fatalf("target file was modified despite the member not existing:\n%s", gotFile)
	}
	if _, statErr := os.Stat(path + ".bak"); !os.IsNotExist(statErr) {
		t.Fatalf(".bak was created even though nothing was removed (stat err: %v)", statErr)
	}
}
