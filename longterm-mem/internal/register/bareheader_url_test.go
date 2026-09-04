package register

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The twin of TestCodex_ForeignSectionWithACommandIsStillRefused, and the
// shape that made the "no command line means absent" rule destructive. A
// codex entry may be url-based: type/url/bearer_token and no `command` at
// all. That is a real, working third-party entry, so it must be refused
// exactly like the command-carrying one, and the file must come back
// byte-identical -- the secret it holds is not something a register step
// gets to move into a .bak.
func TestCodex_ForeignURLSectionWithoutACommandIsStillRefused(t *testing.T) {
	configRoot := t.TempDir()
	stateDir := t.TempDir()
	configPath := filepath.Join(configRoot, codexConfigFileName)
	original := "[mcp_servers.longterm-mem]\ntype = \"http\"\nurl = \"https://mcp.example.com/sse\"\nbearer_token = \"sk-not-yours\"\n"
	if err := os.WriteFile(configPath, []byte(original), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	err := RegisterCodex(configRoot, stateDir, "/usr/local/bin/longterm-mem")
	if err == nil {
		t.Fatal("register overwrote a foreign url-based codex section that carries no command line")
	}
	raw, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("read config back: %v", readErr)
	}
	if string(raw) != original {
		t.Fatalf("a refused register still modified the file:\n%s", raw)
	}
}

// The bodyless header stays absent even when it is only visually empty:
// comments and blank lines are not a body, so this is still the
// half-finished hand edit the narrow rule is for, not somebody's entry.
func TestCodex_CommentOnlySectionIsStillNotAForeignEntry(t *testing.T) {
	configRoot := t.TempDir()
	stateDir := t.TempDir()
	configPath := filepath.Join(configRoot, codexConfigFileName)
	original := "[mcp_servers.longterm-mem]\n# TODO: fill this in\n\n"
	if err := os.WriteFile(configPath, []byte(original), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if err := RegisterCodex(configRoot, stateDir, "/usr/local/bin/longterm-mem"); err != nil {
		t.Fatalf("register over a comment-only section header failed: %v", err)
	}
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config back: %v", err)
	}
	if !strings.Contains(string(raw), "command = \"/usr/local/bin/longterm-mem\"") {
		t.Fatalf("register wrote no command line into the section:\n%s", raw)
	}
}
