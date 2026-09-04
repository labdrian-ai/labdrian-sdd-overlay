package register

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// This file pins the ONE property the adoption rule (Decide's ActionAdopt)
// depends on and that a raw-byte comparison silently failed to deliver:
// the ownership question "is this entry one longterm-mem wrote?" must be
// answered the same way here as it is by the read-only status adapter that
// asks it about the very same file (engine/runtime's LongtermMemAdapter,
// ownedClaudeFingerprint and friends).
//
// That adapter canonicalizes before hashing -- it decodes the entry and
// re-marshals it, so object key order and every byte of insignificant
// whitespace drop out -- while adoption used to hash the raw bytes as
// they sat on disk. The two therefore disagreed on exactly the config the
// adoption rule exists for: an entry longterm-mem wrote that something
// then re-serialized (a config editor, a settings UI, a formatter, a
// user's own `jq . > config`). `doctor` called that entry ours; `register`
// called it a stranger's and refused with exit 6, which is the lockout
// adoption was added to close, still standing after its own fix.

// reorderedClaudeEntry is the claude entry RegisterClaude writes, re-
// serialized with its keys in a different order and spaced out -- the
// shape any generic JSON formatter produces from our own entry. It is
// semantically the identical entry: same type, same command, same args.
func reorderedClaudeEntry(binary string) []byte {
	return []byte(`{ "args": [ "mcp" ], "command": ` + mustJSONString(binary) + `, "type": "stdio" }`)
}

// reorderedOpencodeEntry is the opencode twin of reorderedClaudeEntry.
func reorderedOpencodeEntry(binary string) []byte {
	return []byte(`{"enabled":true,  "command": [` + mustJSONString(binary) + `, "mcp"],
      "type": "local"}`)
}

func mustJSONString(s string) string {
	encoded, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

// reformatInstalledEntry swaps the exact entry bytes longterm-mem wrote
// for a semantically identical re-serialization of them, leaving every
// other byte of the config alone -- the file a formatter would hand back.
func reformatInstalledEntry(t *testing.T, configPath string, written, reformatted []byte) []byte {
	t.Helper()
	installed, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config after install: %v", err)
	}
	rewritten := bytes.Replace(installed, written, reformatted, 1)
	if bytes.Equal(rewritten, installed) {
		t.Fatalf("the installed config does not contain the entry bytes to reformat:\nconfig =\n%s\nentry =\n%s", installed, written)
	}
	if err := os.WriteFile(configPath, rewritten, 0o600); err != nil {
		t.Fatalf("write reformatted config: %v", err)
	}
	return rewritten
}

// assertAdopted asserts the outcome every case below wants: registration
// succeeded, the ownership record came back, and not one byte of the
// runtime's own config moved.
func assertAdopted(t *testing.T, c goldenWriterCase, configRoot, stateDir, configPath string, want []byte, registerErr error) {
	t.Helper()
	if registerErr != nil {
		t.Fatalf("%s: register over a semantically identical but reformatted entry = %v, want adoption: the read-only status adapter already calls that entry ours, so refusing it here is the install-state lockout this rule was added to close", c.target, registerErr)
	}
	state, err := LoadInstallState(filepath.Join(stateDir, installStateFileName))
	if err != nil {
		t.Fatalf("%s: load restored install-state: %v", c.target, err)
	}
	if _, present := state.Get(c.target); !present {
		t.Fatalf("%s: install-state has no record after adoption -- the ownership record was not restored", c.target)
	}
	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("%s: read config after adoption: %v", c.target, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s: adoption rewrote the runtime's own config:\nbefore =\n%s\nafter =\n%s", c.target, want, got)
	}
	outcome, err := c.unregister(configRoot, stateDir)
	if err != nil {
		t.Fatalf("%s: unregister after adoption returned error: %v", c.target, err)
	}
	if outcome != UnregisterRemoved {
		t.Fatalf("%s: unregister after adoption = %v, want UnregisterRemoved -- an adopted entry must be removable, not stranded as unmanaged", c.target, outcome)
	}
}

// TestClaude_LostInstallStateAdoptsAKeyReorderedEntry: GIVEN claude was
// registered, the entry was then re-serialized with its keys in another
// order, and install-state.json was lost, WHEN longterm-mem registers
// again for the same binary, THEN it adopts the entry rather than
// refusing it.
func TestClaude_LostInstallStateAdoptsAKeyReorderedEntry(t *testing.T) {
	c := claudeGoldenCase()
	dir, stateDir := t.TempDir(), t.TempDir()
	configPath := c.seedConfig(t, dir, c.fixtureName("before"))

	if err := c.register(dir, stateDir, c.binary1); err != nil {
		t.Fatalf("%s: first install returned error: %v", c.target, err)
	}
	written, err := json.Marshal(claudeEntry{Type: "stdio", Command: c.binary1, Args: []string{"mcp"}})
	if err != nil {
		t.Fatalf("marshal the entry register writes: %v", err)
	}
	want := reformatInstalledEntry(t, configPath, written, reorderedClaudeEntry(c.binary1))
	if err := os.Remove(filepath.Join(stateDir, installStateFileName)); err != nil {
		t.Fatalf("remove install-state.json: %v", err)
	}

	assertAdopted(t, c, dir, stateDir, configPath, want, c.register(dir, stateDir, c.binary1))
}

// TestOpencode_LostInstallStateAdoptsAKeyReorderedEntry is claude's twin:
// both JSON writers share jsonInstall, so a derivation aligned for one and
// not the other would be an accident, not a fix.
func TestOpencode_LostInstallStateAdoptsAKeyReorderedEntry(t *testing.T) {
	c := opencodeGoldenCase()
	dir, stateDir := t.TempDir(), t.TempDir()
	configPath := c.seedConfig(t, dir, c.fixtureName("before"))

	if err := c.register(dir, stateDir, c.binary1); err != nil {
		t.Fatalf("%s: first install returned error: %v", c.target, err)
	}
	written, err := json.Marshal(opencodeEntry{Type: "local", Command: []string{c.binary1, "mcp"}, Enabled: true})
	if err != nil {
		t.Fatalf("marshal the entry register writes: %v", err)
	}
	want := reformatInstalledEntry(t, configPath, written, reorderedOpencodeEntry(c.binary1))
	if err := os.Remove(filepath.Join(stateDir, installStateFileName)); err != nil {
		t.Fatalf("remove install-state.json: %v", err)
	}

	assertAdopted(t, c, dir, stateDir, configPath, want, c.register(dir, stateDir, c.binary1))
}

// TestCodex_LostInstallStateAdoptsASectionWithoutATrailingNewline is the
// TOML twin. codex's read-only fingerprint trims trailing newlines from
// both sides before hashing (engine/runtime's ownedCodexFingerprint and
// tomlSectionFingerprint), so a config whose final line carries no newline
// -- an editor setting, a here-doc, a hand edit -- reads as ours there.
// Adoption compared the located span verbatim against a section template
// that always ends in "\n", so the same file read as a stranger's here.
func TestCodex_LostInstallStateAdoptsASectionWithoutATrailingNewline(t *testing.T) {
	c := codexGoldenCase()
	dir, stateDir := t.TempDir(), t.TempDir()
	configPath := c.seedConfig(t, dir, c.fixtureName("before"))

	if err := c.register(dir, stateDir, c.binary1); err != nil {
		t.Fatalf("%s: first install returned error: %v", c.target, err)
	}
	installed, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config after install: %v", err)
	}
	want := bytes.TrimRight(installed, "\n")
	if bytes.Equal(want, installed) {
		t.Fatalf("codex fixture does not end in a newline, so this case proves nothing:\n%s", installed)
	}
	if err := os.WriteFile(configPath, want, 0o600); err != nil {
		t.Fatalf("write config without its trailing newline: %v", err)
	}
	if err := os.Remove(filepath.Join(stateDir, installStateFileName)); err != nil {
		t.Fatalf("remove install-state.json: %v", err)
	}

	assertAdopted(t, c, dir, stateDir, configPath, want, c.register(dir, stateDir, c.binary1))
}

// TestClaude_LostInstallStateStillRefusesASemanticallyDifferentEntry is
// the half that keeps the widened derivation honest: canonicalizing
// formatting must not canonicalize CONTENT. An entry whose command names
// another binary, or that carries a field ours never writes, is still a
// stranger's and is still refused, byte for byte untouched.
func TestClaude_LostInstallStateStillRefusesASemanticallyDifferentEntry(t *testing.T) {
	c := claudeGoldenCase()
	for _, tc := range []struct {
		name  string
		entry []byte
	}{
		{"different command", []byte(`{ "args": ["mcp"], "command": "/somewhere/else/longterm-mem", "type": "stdio" }`)},
		{"extra field", []byte(`{ "args": ["mcp"], "command": ` + mustJSONString("/usr/local/bin/longterm-mem") + `, "type": "stdio", "env": {"X": "1"} }`)},
		{"different args", []byte(`{ "args": ["mcp", "--verbose"], "command": ` + mustJSONString("/usr/local/bin/longterm-mem") + `, "type": "stdio" }`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, stateDir := t.TempDir(), t.TempDir()
			configPath := c.seedConfig(t, dir, c.fixtureName("before"))

			if err := c.register(dir, stateDir, c.binary1); err != nil {
				t.Fatalf("first install returned error: %v", err)
			}
			written, err := json.Marshal(claudeEntry{Type: "stdio", Command: c.binary1, Args: []string{"mcp"}})
			if err != nil {
				t.Fatalf("marshal the entry register writes: %v", err)
			}
			want := reformatInstalledEntry(t, configPath, written, tc.entry)
			if err := os.Remove(filepath.Join(stateDir, installStateFileName)); err != nil {
				t.Fatalf("remove install-state.json: %v", err)
			}

			if err := c.register(dir, stateDir, c.binary1); !errors.Is(err, ErrConflict) {
				t.Fatalf("register over a semantically DIFFERENT entry = %v, want errors.Is(err, ErrConflict)", err)
			}
			got, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatalf("read config after the refusal: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("config was modified despite refusal:\nbefore =\n%s\nafter =\n%s", want, got)
			}
		})
	}
}

// TestOwnershipFingerprint_JSONIsCanonical pins the derivation itself
// against the exact canonicalization the read-only adapter applies:
// decode, re-marshal (encoding/json sorts object keys and emits no
// insignificant whitespace), hash. Anything that is not valid JSON has no
// ownership fingerprint at all, so two unparseable blobs can never match
// each other into an adoption.
func TestOwnershipFingerprint_JSONIsCanonical(t *testing.T) {
	canonical := []byte(`{"args":["mcp"],"command":"/usr/local/bin/longterm-mem","type":"stdio"}`)
	structOrder := []byte(`{"type":"stdio","command":"/usr/local/bin/longterm-mem","args":["mcp"]}`)
	spacedOut := []byte("{\n  \"type\" : \"stdio\",\n  \"command\":\t\"/usr/local/bin/longterm-mem\",\n  \"args\": [ \"mcp\" ]\n}")

	want := ownershipFingerprintJSON(canonical)
	if want == "" {
		t.Fatalf("ownershipFingerprintJSON(%s) = \"\", want a fingerprint", canonical)
	}
	for _, equivalent := range [][]byte{structOrder, spacedOut} {
		if got := ownershipFingerprintJSON(equivalent); got != want {
			t.Fatalf("ownershipFingerprintJSON(%s) = %q, want %q -- key order and insignificant whitespace must not change ownership", equivalent, got, want)
		}
	}
	if got := ownershipFingerprintJSON([]byte(`{"type":"stdio","command":"/other","args":["mcp"]}`)); got == want {
		t.Fatal("ownershipFingerprintJSON collapsed two entries that name different binaries")
	}
	if got := ownershipFingerprintJSON([]byte("not json at all")); got != "" {
		t.Fatalf("ownershipFingerprintJSON(invalid) = %q, want \"\" so no two unparseable entries can match", got)
	}
	if ownershipFingerprintJSON(nil) != "" {
		t.Fatal("ownershipFingerprintJSON(nil) must have no fingerprint")
	}
}

// TestOwnershipFingerprint_TOMLTrimsTrailingNewlines mirrors the JSON case
// for codex, against engine/runtime's own hashed value: the section text
// with trailing newlines trimmed.
func TestOwnershipFingerprint_TOMLTrimsTrailingNewlines(t *testing.T) {
	section := []byte("[mcp_servers.longterm-mem]\ncommand = \"/usr/local/bin/longterm-mem\"\nargs = [\"mcp\"]\n")
	trimmed := bytes.TrimRight(section, "\n")

	want := ownershipFingerprintTOML(section)
	if want == "" {
		t.Fatal("ownershipFingerprintTOML returned no fingerprint for a real section")
	}
	if got := ownershipFingerprintTOML(trimmed); got != want {
		t.Fatalf("ownershipFingerprintTOML(no trailing newline) = %q, want %q", got, want)
	}
	if got := ownershipFingerprintTOML(append(append([]byte{}, section...), '\n')); got != want {
		t.Fatalf("ownershipFingerprintTOML(extra trailing newline) = %q, want %q", got, want)
	}
	other := []byte("[mcp_servers.longterm-mem]\ncommand = \"/other\"\nargs = [\"mcp\"]\n")
	if ownershipFingerprintTOML(other) == want {
		t.Fatal("ownershipFingerprintTOML collapsed two sections that name different binaries")
	}
}
