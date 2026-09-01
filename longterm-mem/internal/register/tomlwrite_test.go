package register

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteTOMLSection_AtomicReplaceWithBackup proves the file-level
// wrapper (12a.2): a .bak of the ORIGINAL bytes is written, the target
// file ends up exactly what TOMLSplice would have produced in memory, and
// no stray tmp file is left behind in the same directory (same-dir
// tmp+rename, D9).
func TestWriteTOMLSection_AtomicReplaceWithBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	original := []byte("theme = \"dark\"\n\n[mcp_servers.other]\ncommand = \"/usr/bin/other\"\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("failed to seed fixture file: %v", err)
	}

	newSection := []byte("[mcp_servers.longterm-mem]\ncommand = \"/bin/longterm-mem\"\nargs = [\"mcp\"]\n")
	if err := WriteTOMLSection(path, "mcp_servers", "longterm-mem", "/bin/longterm-mem", newSection); err != nil {
		t.Fatalf("WriteTOMLSection returned error: %v", err)
	}

	gotFile, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read result file: %v", err)
	}
	wantFile, err := TOMLSplice(original, "mcp_servers", "longterm-mem", newSection)
	if err != nil {
		t.Fatalf("TOMLSplice returned error: %v", err)
	}
	if string(gotFile) != string(wantFile) {
		t.Fatalf("target file = %s, want (from TOMLSplice) %s", gotFile, wantFile)
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

// TestWriteTOMLSection_InvalidTOMLLeavesOriginalUntouched proves the
// go-toml/v2 parse gate fires BEFORE any filesystem mutation: a splice
// that would produce malformed TOML is rejected with the target file
// unchanged and no .bak created.
func TestWriteTOMLSection_InvalidTOMLLeavesOriginalUntouched(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	original := []byte("theme = \"dark\"\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("failed to seed fixture file: %v", err)
	}

	malformed := []byte("[mcp_servers.longterm-mem\ncommand = \"/bin/longterm-mem\"\n") // missing closing ']'
	err := WriteTOMLSection(path, "mcp_servers", "longterm-mem", "/bin/longterm-mem", malformed)
	if err == nil {
		t.Fatalf("WriteTOMLSection returned nil error for a malformed table")
	}
	if !strings.Contains(err.Error(), "register:") {
		t.Fatalf("error %q is not prefixed with the package name", err.Error())
	}

	assertUntouched(t, path, original)
}

// TestWriteTOMLSection_CommandMismatchLeavesOriginalUntouched is the
// load-bearing proof that the post-write mcp_servers.longterm-mem.command
// == binary assertion (task 12a.2) is not decoration: a newSection whose
// command does not match the binary argument is refused before the
// rename, exactly as a splice bug that wrote the wrong path (or dropped
// the table into the wrong table name) would be. Without this check,
// WriteTOMLSection would happily commit a config that talks to the wrong
// binary — this test fails the moment that check is removed (see
// apply-progress.md Slice 12a for the deliberate-mutation proof).
func TestWriteTOMLSection_CommandMismatchLeavesOriginalUntouched(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	original := []byte("theme = \"dark\"\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("failed to seed fixture file: %v", err)
	}

	mismatched := []byte("[mcp_servers.longterm-mem]\ncommand = \"/wrong/path\"\nargs = [\"mcp\"]\n")
	err := WriteTOMLSection(path, "mcp_servers", "longterm-mem", "/bin/longterm-mem", mismatched)
	if err == nil {
		t.Fatalf("WriteTOMLSection returned nil error for a command mismatch (newSection command != binary argument)")
	}
	if !strings.Contains(err.Error(), "register:") {
		t.Fatalf("error %q is not prefixed with the package name", err.Error())
	}

	assertUntouched(t, path, original)
}

func assertUntouched(t *testing.T, path string, original []byte) {
	t.Helper()
	gotFile, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read target file: %v", err)
	}
	if string(gotFile) != string(original) {
		t.Fatalf("target file was modified despite the rejected write:\n%s", gotFile)
	}
	if _, statErr := os.Stat(path + ".bak"); !os.IsNotExist(statErr) {
		t.Fatalf(".bak was created even though validation failed before any write (stat err: %v)", statErr)
	}
	dir := filepath.Dir(path)
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
