package register

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An empty configuration document is a real state on a real machine: a
// runtime (or an operator, or a crashed writer, or `: > ~/.claude.json`)
// leaves a zero-byte file where a config belongs. The TOML writer has
// always handled it -- a zero-byte config.toml simply has no sections, and
// the section is appended -- and the spec's "An empty configuration
// document is installed into" scenario claims the JSON writers do too.
//
// They did not: encoding/json rejects an empty document with "unexpected
// end of JSON input", so register failed and left the file untouched, on
// the one shape a brand-new machine most plausibly presents. `{}` worked;
// zero bytes did not. That is not a boundary anyone chose, and no test
// covered either side of it.
//
// The whitespace-only twin is here for the same reason the zero-byte case
// is: "empty" is not a byte count. A file holding "\n" or a couple of
// spaces carries exactly as much configuration as one holding nothing,
// and a fix that keys on len(raw)==0 leaves the identical failure standing
// one keystroke away.
func TestRegister_EmptyJSONConfigIsInstalledInto(t *testing.T) {
	cases := []struct {
		name         string
		body         string
		fileName     string
		containerKey string
		register     func(configRoot, stateDir, binary string) error
	}{
		{"claude zero-byte", "", claudeConfigFileName, claudeContainerKey, RegisterClaude},
		{"claude whitespace-only", "\n  \t\n", claudeConfigFileName, claudeContainerKey, RegisterClaude},
		{"opencode zero-byte", "", opencodeConfigFileName, opencodeContainerKey, RegisterOpencode},
		{"opencode whitespace-only", " \n", opencodeConfigFileName, opencodeContainerKey, RegisterOpencode},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			configRoot := t.TempDir()
			stateDir := t.TempDir()
			configPath := filepath.Join(configRoot, tc.fileName)
			if err := os.WriteFile(configPath, []byte(tc.body), 0o644); err != nil {
				t.Fatalf("write empty config: %v", err)
			}

			if err := tc.register(configRoot, stateDir, "/usr/local/bin/longterm-mem"); err != nil {
				t.Fatalf("register into an empty config failed: %v", err)
			}

			raw, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatalf("read config back: %v", err)
			}
			var doc map[string]map[string]json.RawMessage
			if err := json.Unmarshal(raw, &doc); err != nil {
				t.Fatalf("register produced a config that is not a JSON object: %v\n%s", err, raw)
			}
			if _, ok := doc[tc.containerKey]["longterm-mem"]; !ok {
				t.Fatalf("register reported success but wrote no %s.longterm-mem entry:\n%s", tc.containerKey, raw)
			}
			if !strings.Contains(string(raw), "/usr/local/bin/longterm-mem") {
				t.Fatalf("the written entry does not carry the binary path:\n%s", raw)
			}
		})
	}
}

// The TOML twin of the case above, pinned rather than assumed: the finding
// that opened this hole rested on codex already succeeding on a zero-byte
// config, and an unpinned "already works" is exactly what regresses while
// the JSON side is being repaired.
func TestRegister_EmptyTOMLConfigIsInstalledInto(t *testing.T) {
	configRoot := t.TempDir()
	stateDir := t.TempDir()
	configPath := filepath.Join(configRoot, codexConfigFileName)
	if err := os.WriteFile(configPath, nil, 0o644); err != nil {
		t.Fatalf("write empty config: %v", err)
	}

	if err := RegisterCodex(configRoot, stateDir, "/usr/local/bin/longterm-mem"); err != nil {
		t.Fatalf("register into an empty config.toml failed: %v", err)
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config back: %v", err)
	}
	if !strings.Contains(string(raw), "[mcp_servers.longterm-mem]") {
		t.Fatalf("register wrote no codex section:\n%s", raw)
	}
}
