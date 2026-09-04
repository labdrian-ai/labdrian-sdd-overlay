package register

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// engine/runtime's read-only adapter and this package must agree about
// which codex sections exist; ownership.go and doc.go say so at length,
// and D9's whole adopt row rests on it. One disagreement survived that
// alignment: engine's tomlSectionFingerprint requires a `command =` line
// inside the section before it calls the section present ("a header with
// no command line is not a real entry -- nothing a register step would
// have written"), while locateTOMLSection called a bare header present.
//
// The consequence is the lockout shape in miniature. A stray
// `[mcp_servers.longterm-mem]` header with no body -- a half-finished hand
// edit, an interrupted writer, a merge that kept the header and dropped
// its lines -- reads as ABSENT to `doctor`/`status` and as a FOREIGN entry
// to `register`, which refuses with exit 6 and leaves it. The operator is
// then told, forever, both that longterm-mem is not installed and that it
// will not install, over a section carrying nothing.
//
// register's is the side that was wrong: longterm-mem never writes a
// section without a command line, so a section without one is provably not
// ours to preserve, and it is not a working codex entry for anyone else
// either -- codex needs the command to launch a server. The bytes are not
// discarded silently: replaceConfig backs the original file up to
// config.toml.bak before anything lands.
func TestCodex_BareSectionHeaderIsNotAForeignEntry(t *testing.T) {
	configRoot := t.TempDir()
	stateDir := t.TempDir()
	configPath := filepath.Join(configRoot, codexConfigFileName)
	original := "[some_other_table]\nkeep = true\n\n[mcp_servers.longterm-mem]\n"
	if err := os.WriteFile(configPath, []byte(original), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if err := RegisterCodex(configRoot, stateDir, "/usr/local/bin/longterm-mem"); err != nil {
		t.Fatalf("register over a bodyless section header failed: %v", err)
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config back: %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, "command = \"/usr/local/bin/longterm-mem\"") {
		t.Fatalf("register wrote no command line into the section:\n%s", got)
	}
	if strings.Count(got, "[mcp_servers.longterm-mem]") != 1 {
		t.Fatalf("register duplicated the section header instead of filling the empty one in place:\n%s", got)
	}
	if !strings.Contains(got, "[some_other_table]\nkeep = true\n") {
		t.Fatalf("an unrelated table was not preserved:\n%s", got)
	}
}

// The guard that keeps the change above from becoming a licence to
// overwrite: a section that DOES carry a command line, and is not ours, is
// still refused and still left byte-identical. "No command line" is the
// whole of the widening -- not "no fingerprint match".
func TestCodex_ForeignSectionWithACommandIsStillRefused(t *testing.T) {
	configRoot := t.TempDir()
	stateDir := t.TempDir()
	configPath := filepath.Join(configRoot, codexConfigFileName)
	original := "[mcp_servers.longterm-mem]\ncommand = \"/opt/somebody-elses/server\"\nargs = [\"serve\"]\n"
	if err := os.WriteFile(configPath, []byte(original), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	err := RegisterCodex(configRoot, stateDir, "/usr/local/bin/longterm-mem")
	if err == nil {
		t.Fatal("register overwrote a foreign codex section carrying its own command line")
	}
	raw, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("read config back: %v", readErr)
	}
	if string(raw) != original {
		t.Fatalf("a refused register still modified the file:\n%s", raw)
	}
}
