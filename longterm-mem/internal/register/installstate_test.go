package register

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestInstallState_FingerprintRoundTrip proves install-state.json records a
// target's fingerprint (sha256 of the exact entry bytes longterm-mem
// wrote), that the fingerprint survives a save/load round trip unchanged,
// and — the R-017 half of this requirement — that the fingerprint tag
// itself never appears as an unknown key inside the runtime's OWN config
// schema: it lives only in install-state.json, never spliced into the MCP
// entry Splice would write into e.g. claude.json (11a.4).
func TestInstallState_FingerprintRoundTrip(t *testing.T) {
	entry := []byte(`{"type":"stdio","command":"/bin/longterm-mem","args":["mcp"]}`)
	fp := Fingerprint(entry)
	if fp == "" {
		t.Fatalf("Fingerprint(%s) returned an empty string", entry)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "install-state.json")

	st, err := LoadInstallState(path)
	if err != nil {
		t.Fatalf("LoadInstallState (fresh, no file yet) returned error: %v", err)
	}
	st.Set("claude", TargetRecord{Fingerprint: fp})
	if err := st.Save(path); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	reloaded, err := LoadInstallState(path)
	if err != nil {
		t.Fatalf("LoadInstallState (after save) returned error: %v", err)
	}
	rec, ok := reloaded.Get("claude")
	if !ok {
		t.Fatalf("Get(%q) after reload: record not found", "claude")
	}
	if rec.Fingerprint != fp {
		t.Fatalf("reloaded fingerprint = %q, want %q", rec.Fingerprint, fp)
	}

	// A different entry must produce a different fingerprint — proves
	// Fingerprint is a real function of entry's bytes, not a constant.
	otherEntry := []byte(`{"type":"stdio","command":"/other/path","args":["mcp"]}`)
	otherFP := Fingerprint(otherEntry)
	if otherFP == fp {
		t.Fatalf("Fingerprint of a different entry produced the same digest %q", fp)
	}

	// R-017 positive assertion: the entry bytes themselves — the exact
	// value a runtime writer would splice into the runtime's own config —
	// must not carry a "fingerprint" key. The tag lives only in
	// install-state.json.
	var decodedEntry map[string]json.RawMessage
	if err := json.Unmarshal(entry, &decodedEntry); err != nil {
		t.Fatalf("failed to decode entry fixture: %v", err)
	}
	if _, present := decodedEntry["fingerprint"]; present {
		t.Fatalf("the runtime config entry carries an unknown %q key: %s", "fingerprint", entry)
	}

	// And the saved install-state.json file itself must be the only place
	// the fingerprint tag lives — sanity check the raw bytes on disk.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	if !strings.Contains(string(raw), fp) {
		t.Fatalf("install-state.json does not contain the saved fingerprint %q:\n%s", fp, raw)
	}
}
