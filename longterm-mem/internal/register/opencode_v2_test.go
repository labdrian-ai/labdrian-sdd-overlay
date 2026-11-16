package register

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// opencode v2 keeps MCP servers under mcp.servers.<name> with `codemode`
// (the 1.x shape was mcp.<name> with `enabled`). These tests pin the v2
// write, the removal, the migration of the 1.x entry this writer used to
// write, and that no foreign key is ever dropped.

const legacyOwnedEntry = `{"type":"local","command":["/opt/labdrian-overlay/bin/longterm-mem","mcp"],"enabled":true}`

func readV2Entry(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var doc struct {
		Mcp map[string]json.RawMessage `json:"mcp"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse config: %v\n%s", err, raw)
	}
	var servers map[string]map[string]any
	if err := json.Unmarshal(doc.Mcp["servers"], &servers); err != nil {
		t.Fatalf("mcp.servers is not an object: %v\n%s", err, raw)
	}
	return servers["longterm-mem"]
}

func TestOpencodeV2_RegisterWritesNestedServersShape(t *testing.T) {
	c := opencodeGoldenCase()
	dir, stateDir := t.TempDir(), t.TempDir()
	path := c.seedConfig(t, dir, "before.json")
	if err := c.register(dir, stateDir, c.binary1); err != nil {
		t.Fatalf("register: %v", err)
	}
	entry := readV2Entry(t, path)
	if entry["type"] != "local" || entry["codemode"] != true {
		t.Fatalf("v2 entry = %v, want type=local codemode=true", entry)
	}
	if _, has := entry["enabled"]; has {
		t.Fatalf("v2 entry carries the 1.x `enabled` field: %v", entry)
	}
	raw, _ := os.ReadFile(path)
	var top map[string]map[string]json.RawMessage
	_ = json.Unmarshal(raw, &top)
	if _, legacy := top["mcp"]["longterm-mem"]; legacy {
		t.Fatalf("1.x mcp.longterm-mem was written:\n%s", raw)
	}
}

func TestOpencodeV2_UnregisterRemovesOnlyTheV2Entry(t *testing.T) {
	c := opencodeGoldenCase()
	dir, stateDir := t.TempDir(), t.TempDir()
	path := c.seedConfig(t, dir, "before.json")
	if err := c.register(dir, stateDir, c.binary1); err != nil {
		t.Fatalf("register: %v", err)
	}
	outcome, err := c.unregister(dir, stateDir)
	if err != nil || outcome != UnregisterRemoved {
		t.Fatalf("unregister = %v, %v; want removed", outcome, err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(c.readFixture(t, "after-uninstall.json")) {
		t.Fatalf("config after unregister =\n%s", got)
	}
}

func TestOpencodeV2_LegacyEntryMigratedOnRegister(t *testing.T) {
	c := opencodeGoldenCase()
	dir, stateDir := t.TempDir(), t.TempDir()
	path := c.seedConfig(t, dir, "legacy-1x.json")
	if err := c.register(dir, stateDir, c.binary1); err != nil {
		t.Fatalf("register: %v", err)
	}
	got, _ := os.ReadFile(path)
	if want := c.readFixture(t, "after-migrate.json"); string(got) != string(want) {
		t.Fatalf("config after migration =\n%s\nwant =\n%s", got, want)
	}
}

func TestOpencodeV2_LegacyEntryOfAnOlderBinaryMigratedByRecord(t *testing.T) {
	c := opencodeGoldenCase()
	dir, stateDir := t.TempDir(), t.TempDir()
	path := filepath.Join(dir, c.configFileName)
	old := `{"type":"local","command":["/old/place/longterm-mem","mcp"],"enabled":true}`
	seed := strings.Replace(string(c.readFixture(t, "legacy-1x.json")), legacyOwnedEntry, old, 1)
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	st := &InstallState{}
	st.Set(opencodeTarget, TargetRecord{Fingerprint: Fingerprint([]byte(old))})
	if err := fileWriter().State.Save(stateDir, st); err != nil {
		t.Fatal(err)
	}
	if err := c.register(dir, stateDir, c.binary1); err != nil {
		t.Fatalf("register: %v", err)
	}
	got, _ := os.ReadFile(path)
	if want := c.readFixture(t, "after-migrate.json"); string(got) != string(want) {
		t.Fatalf("config after migration =\n%s\nwant =\n%s", got, want)
	}
}

func TestOpencodeV2_ForeignLegacyEntryIsNeverDropped(t *testing.T) {
	c := opencodeGoldenCase()
	dir, stateDir := t.TempDir(), t.TempDir()
	path := filepath.Join(dir, c.configFileName)
	foreign := `{"type":"local","command":["/someone/elses/binary","mcp"],"enabled":true}`
	seed := strings.Replace(string(c.readFixture(t, "legacy-1x.json")), legacyOwnedEntry, foreign, 1)
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := c.register(dir, stateDir, c.binary1); err != nil {
		t.Fatalf("register: %v", err)
	}
	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), foreign) {
		t.Fatalf("a foreign mcp.longterm-mem entry was dropped:\n%s", got)
	}
	if readV2Entry(t, path)["codemode"] != true {
		t.Fatalf("v2 entry not written:\n%s", got)
	}
}

func TestOpencodeV2_SecondRegisterIsANoOp(t *testing.T) {
	c := opencodeGoldenCase()
	dir, stateDir := t.TempDir(), t.TempDir()
	path := c.seedConfig(t, dir, "legacy-1x.json")
	for i := 0; i < 2; i++ {
		if err := c.register(dir, stateDir, c.binary1); err != nil {
			t.Fatalf("register #%d: %v", i+1, err)
		}
	}
	got, _ := os.ReadFile(path)
	if want := c.readFixture(t, "after-migrate.json"); string(got) != string(want) {
		t.Fatalf("config after second register =\n%s\nwant =\n%s", got, want)
	}
}

func TestOpencodeV2_UnregisterAlsoCleansAnOwnedLegacyEntry(t *testing.T) {
	c := opencodeGoldenCase()
	dir, stateDir := t.TempDir(), t.TempDir()
	path := c.seedConfig(t, dir, "legacy-1x.json")
	st := &InstallState{}
	st.Set(opencodeTarget, TargetRecord{Fingerprint: Fingerprint([]byte(legacyOwnedEntry))})
	if err := fileWriter().State.Save(stateDir, st); err != nil {
		t.Fatal(err)
	}
	outcome, err := c.unregister(dir, stateDir)
	if err != nil || outcome != UnregisterRemoved {
		t.Fatalf("unregister = %v, %v; want removed", outcome, err)
	}
	got, _ := os.ReadFile(path)
	want := `{
  "mcp": {
    "other-tool": {
      "type": "local",
      "command": ["/usr/local/bin/other-tool", "serve"],
      "enabled": true
    }
  },
  "theme": "dark"
}
`
	if string(got) != want {
		t.Fatalf("config after unregister =\n%s\nwant =\n%s", got, want)
	}
}

func TestOpencodeV2_UnregisterLeavesAForeignLegacyEntry(t *testing.T) {
	c := opencodeGoldenCase()
	dir, stateDir := t.TempDir(), t.TempDir()
	path := c.seedConfig(t, dir, "legacy-1x.json")
	before, _ := os.ReadFile(path)
	if _, err := c.unregister(dir, stateDir); err != nil {
		t.Fatalf("unregister: %v", err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatalf("unregister with no ownership record changed the config:\n%s", after)
	}
}

// The other runtimes' writers are untouched by the nested-container support:
// their own golden scenarios still hold.
func TestOpencodeV2_OtherTargetsWriteAsBefore(t *testing.T) {
	for _, c := range []goldenWriterCase{claudeGoldenCase(), piGoldenCase(), codexGoldenCase()} {
		t.Run(c.target, func(t *testing.T) {
			c.testUnrelatedEntriesPreserved(t)
			c.testReinstallIsIdempotent(t)
		})
	}
}
