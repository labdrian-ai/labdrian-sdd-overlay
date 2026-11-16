package register

import (
	"encoding/json"
	"errors"
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
	if bak, _ := os.ReadFile(backupPath(path)); string(bak) != string(c.readFixture(t, "legacy-1x.json")) {
		t.Fatalf(".bak = %s, want the original", bak)
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

// A hand-written 1.x mcp.longterm-mem is a stranger's entry, exactly as it was
// before the v2 shape: register refuses (ErrConflict) and writes nothing.
func TestOpencodeV2_ForeignLegacyEntryIsAConflict(t *testing.T) {
	c := opencodeGoldenCase()
	dir, stateDir := t.TempDir(), t.TempDir()
	path := filepath.Join(dir, c.configFileName)
	foreign := `{"type":"local","command":["/someone/elses/binary","mcp"],"enabled":true}`
	seed := strings.Replace(string(c.readFixture(t, "legacy-1x.json")), legacyOwnedEntry, foreign, 1)
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	err := c.register(dir, stateDir, c.binary1)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("register over a foreign 1.x entry = %v, want ErrConflict", err)
	}
	if got, _ := os.ReadFile(path); string(got) != seed {
		t.Fatalf("the refused register changed the config:\n%s", got)
	}
	if _, statErr := os.Stat(backupPath(path)); statErr == nil {
		t.Fatal("the refused register wrote a .bak")
	}
}

// A 1.x server that happens to be called "servers" is not the v2 container.
func TestOpencodeV2_A1xServerNamedServersIsAConflict(t *testing.T) {
	c := opencodeGoldenCase()
	dir, stateDir := t.TempDir(), t.TempDir()
	path := filepath.Join(dir, c.configFileName)
	seed := `{"mcp":{"servers":{"type":"local","command":["/usr/local/bin/servers"],"enabled":true}}}` + "\n"
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := c.register(dir, stateDir, c.binary1); !errors.Is(err, ErrConflict) {
		t.Fatalf("register = %v, want ErrConflict", err)
	}
	if got, _ := os.ReadFile(path); string(got) != seed {
		t.Fatalf("the refused register changed the config:\n%s", got)
	}
	outcome, err := c.unregister(dir, stateDir)
	if err != nil || outcome != UnregisterNoop {
		t.Fatalf("unregister = %v, %v; want noop", outcome, err)
	}
	if got, _ := os.ReadFile(path); string(got) != seed {
		t.Fatalf("unregister changed the config:\n%s", got)
	}
}

// One operation is one write: the .bak holds the file as the operation found it.
func TestOpencodeV2_BackupHoldsTheOriginalAfterAMigratingRegisterAndAfterUnregister(t *testing.T) {
	c := opencodeGoldenCase()
	dir, stateDir := t.TempDir(), t.TempDir()
	path := c.seedConfig(t, dir, "legacy-1x.json")
	original := c.readFixture(t, "legacy-1x.json")
	if err := c.register(dir, stateDir, c.binary1); err != nil {
		t.Fatalf("register: %v", err)
	}
	if bak, _ := os.ReadFile(backupPath(path)); string(bak) != string(original) {
		t.Fatalf(".bak after a migrating register =\n%s\nwant the original", bak)
	}
	migrated, _ := os.ReadFile(path)
	if _, err := c.unregister(dir, stateDir); err != nil {
		t.Fatalf("unregister: %v", err)
	}
	if bak, _ := os.ReadFile(backupPath(path)); string(bak) != string(migrated) {
		t.Fatalf(".bak after unregister =\n%s\nwant the file as unregister found it", bak)
	}

	// An owned 1.x entry and a v2 entry together go in one write too.
	dir2, state2 := t.TempDir(), t.TempDir()
	both := strings.Replace(string(c.readFixture(t, "after-install.json")), `"mcp": {`, `"mcp": {
    "longterm-mem": `+legacyOwnedEntry+`,`, 1)
	path2 := filepath.Join(dir2, c.configFileName)
	if err := os.WriteFile(path2, []byte(both), 0o600); err != nil {
		t.Fatal(err)
	}
	st := &InstallState{}
	st.Set(opencodeTarget, TargetRecord{Fingerprint: Fingerprint([]byte(legacyOwnedEntry))})
	if err := fileWriter().State.Save(state2, st); err != nil {
		t.Fatal(err)
	}
	outcome, err := c.unregister(dir2, state2)
	if err != nil || outcome != UnregisterRemoved {
		t.Fatalf("unregister = %v, %v; want removed", outcome, err)
	}
	if bak, _ := os.ReadFile(backupPath(path2)); string(bak) != both {
		t.Fatalf(".bak after unregister =\n%s\nwant the original", bak)
	}
	if got, _ := os.ReadFile(path2); strings.Contains(string(got), "longterm-mem") {
		t.Fatalf("an entry of ours is left:\n%s", got)
	}
}

// The proof of ownership of an older-binary 1.x entry is the record; a failed
// register must neither lose the record nor leave the config half migrated.
func TestOpencodeV2_FailedRegisterKeepsTheProofAndTheConfig(t *testing.T) {
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
	if err := (Writer{State: unsavableState{}}).RegisterOpencode(dir, stateDir, c.binary1); err == nil {
		t.Fatal("register with an unwritable install-state succeeded")
	}
	if got, _ := os.ReadFile(path); string(got) != seed {
		t.Fatalf("the failed register left the config changed:\n%s", got)
	}
	if err := c.register(dir, stateDir, c.binary1); err != nil {
		t.Fatalf("the retry: %v", err)
	}
	got, _ := os.ReadFile(path)
	if want := c.readFixture(t, "after-migrate.json"); string(got) != string(want) {
		t.Fatalf("config after the retry =\n%s\nwant =\n%s", got, want)
	}
}

// A new member lands at its real depth, also in an empty servers object.
func TestOpencodeV2_InsertsAtTheRealDepth(t *testing.T) {
	c := opencodeGoldenCase()
	dir, stateDir := t.TempDir(), t.TempDir()
	path := filepath.Join(dir, c.configFileName)
	seed := "{\n  \"mcp\": {\n    \"servers\": {}\n  }\n}\n"
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := c.register(dir, stateDir, c.binary1); err != nil {
		t.Fatalf("register: %v", err)
	}
	want := "{\n  \"mcp\": {\n    \"servers\": {\n      \"longterm-mem\": {\"type\":\"local\",\"command\":[\"/opt/labdrian-overlay/bin/longterm-mem\",\"mcp\"],\"codemode\":true}}\n  }\n}\n"
	if got, _ := os.ReadFile(path); string(got) != want {
		t.Fatalf("config =\n%s\nwant =\n%s", got, want)
	}
}

// Errors name the container the way opencode's config does.
func TestOpencodeV2_ErrorsNameTheContainerWithADot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opencode.json")
	if err := os.WriteFile(path, []byte(`{"mcp":{"servers":{}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	err := RemoveMember(path, opencodeContainerKey, "longterm-mem")
	if err == nil || !strings.Contains(err.Error(), "mcp.servers") || strings.Contains(err.Error(), "mcp/servers") {
		t.Fatalf("error = %v, want it to name mcp.servers", err)
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
	outcome, err := c.unregister(dir, stateDir)
	if err != nil || outcome != UnregisterUnmanaged {
		t.Fatalf("unregister = %v, %v; want unmanaged", outcome, err)
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
