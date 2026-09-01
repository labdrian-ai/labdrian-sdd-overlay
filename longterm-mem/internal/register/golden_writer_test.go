package register

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// goldenWriterCase describes one JSON-backed runtime writer's golden-fixture
// scenarios (11b.8, D9; R-016/R-017): the three scenarios every JSON writer
// (claude.go, opencode.go) must satisfy, driven from
// testdata/<runtime>/*.json rather than each runtime's _test.go file
// re-deriving fixture plumbing. Adding a runtime means adding a
// goldenWriterCase value pointed at its own testdata directory and calling
// runGoldenWriterScenarios, not copying these three test bodies — that is
// the harness this REFACTOR step exists to guarantee.
//
// Fixture naming convention inside testdataDir (shared with the future
// codex/TOML and uninstall writers, 12a/12b):
//
//	before.json          the runtime config before any longterm-mem writer
//	                      call, with unrelated entries only.
//	after-install.json   the expected config after a fresh install with
//	                      binary1.
//	after-reinstall.json the expected config after a second install call
//	                      with binary2, replacing the entry in place.
//	untagged.<ext>         a config that already has a same-named entry
//	                      install-state does not own.
//
// after-uninstall.json is intentionally not part of this slice's fixture
// set: R-019 (Unregister) is implemented in slice 12b, which reuses this
// same naming convention once it exists (see apply-progress.md Slice 11b).
//
// codex (12a, R-018) drives this same harness with fixtureExt set to
// ".toml" instead of ".json" — see fixtureExt's own doc comment for why a
// field, rather than a second near-identical harness, is the honest fix:
// every byte comparison and fixture-plumbing method below is format-
// agnostic (it only ever moves raw bytes around), so the only thing that
// was ever JSON-specific was the four hardcoded ".json" literals and the
// JSON-syntax "not a duplicate" needle (duplicateNeedle generalizes that).
type goldenWriterCase struct {
	// target is install-state's target key ("claude", "opencode", "codex").
	target string
	// configFileName is the runtime config's file name inside configRoot
	// (".claude.json", "opencode.json", "config.toml").
	configFileName string
	// memberKey is the entry's key inside its container object
	// ("longterm-mem" for every runtime this slice writes).
	memberKey string
	// testdataDir holds this runtime's golden fixtures, relative to the
	// register package directory.
	testdataDir string
	// fixtureExt is the golden fixture file name suffix — ".json" for
	// claude/opencode, ".toml" for codex. It is the ONLY thing that
	// differs about which literal fixture file each scenario reads;
	// nothing else in this harness decodes or otherwise assumes JSON.
	fixtureExt string
	// register drives the runtime's own Register* entry point.
	register func(configRoot, stateDir, binary string) error
	// binary1/binary2 are the fixed binary paths baked into
	// after-install.<ext> / after-reinstall.<ext> respectively — binary2
	// simulates a reinstall picking up a different resolved binary path,
	// which forces Decide's replace branch (not noop) so the reinstall
	// scenario genuinely exercises an in-place rewrite.
	binary1, binary2 string
	// unrelatedSnippet is a literal byte sequence unique to one of
	// before.<ext>'s pre-existing entries. It is asserted present, byte-
	// for-byte, in every fixture this harness compares against — an extra,
	// fixture-independent check that "unrelated entries are preserved"
	// really means untouched bytes, not merely "the golden file matches".
	unrelatedSnippet string
	// duplicateNeedle is a literal byte sequence that must occur exactly
	// once in the config after a reinstall — the format-specific spelling
	// of "this is the memberKey entry/section, and there is only one of
	// it": `"longterm-mem":` for JSON, `[mcp_servers.longterm-mem]` for
	// TOML. Kept as an explicit field, not derived from memberKey, because
	// deriving it would smuggle a JSON-syntax assumption back into the
	// shared method below.
	duplicateNeedle string
}

// Each of a runtime's three Test<Runtime>_* functions (claude_test.go,
// opencode_test.go) calls exactly one of the test<Scenario> methods below
// on its own goldenWriterCase value — the harness's per-scenario methods,
// not a single "run everything" entry point, so `go test -run
// TestClaude_ReinstallIsIdempotent` (etc.) runs precisely the scenario its
// name promises, matching the Command lines in tasks.md.

func (c goldenWriterCase) fixturePath(name string) string {
	return filepath.Join(c.testdataDir, name)
}

// fixtureName joins stem (e.g. "before", "after-install") with this case's
// fixtureExt, so every call site below names a fixture by its role, not by
// a hardcoded format-specific literal.
func (c goldenWriterCase) fixtureName(stem string) string {
	return stem + c.fixtureExt
}

func (c goldenWriterCase) readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(c.fixturePath(name))
	if err != nil {
		t.Fatalf("%s: failed to read fixture %s: %v", c.target, c.fixturePath(name), err)
	}
	return data
}

// seedConfig writes fixture name as the target's config file inside dir and
// returns the resulting file's path.
func (c goldenWriterCase) seedConfig(t *testing.T, dir, fixtureName string) string {
	t.Helper()
	path := filepath.Join(dir, c.configFileName)
	if err := os.WriteFile(path, c.readFixture(t, fixtureName), 0o600); err != nil {
		t.Fatalf("%s: failed to seed %s: %v", c.target, path, err)
	}
	return path
}

func (c goldenWriterCase) assertUnrelatedSnippetPresent(t *testing.T, label string, raw []byte) {
	t.Helper()
	if c.unrelatedSnippet == "" {
		t.Fatalf("%s: goldenWriterCase.unrelatedSnippet is empty — every case must name a real unrelated-entry snippet to assert", c.target)
	}
	if !strings.Contains(string(raw), c.unrelatedSnippet) {
		t.Fatalf("%s: %s does not contain the unrelated-entry snippet %q verbatim:\n%s", c.target, label, c.unrelatedSnippet, raw)
	}
}

// testUnrelatedEntriesPreserved: GIVEN before.<ext> already has unrelated
// MCP entries, WHEN longterm-mem installs, THEN the resulting file matches
// the pre-computed golden after-install.<ext> exactly (which by
// construction only differs from before.<ext> inside the newly inserted
// span), and the unrelated snippet survives byte-for-byte.
func (c goldenWriterCase) testUnrelatedEntriesPreserved(t *testing.T) {
	dir := t.TempDir()
	stateDir := t.TempDir()
	configPath := c.seedConfig(t, dir, c.fixtureName("before"))
	before := c.readFixture(t, c.fixtureName("before"))
	c.assertUnrelatedSnippetPresent(t, c.fixtureName("before")+" fixture", before)

	if err := c.register(dir, stateDir, c.binary1); err != nil {
		t.Fatalf("%s: install returned error: %v", c.target, err)
	}

	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("%s: failed to read result config: %v", c.target, err)
	}
	want := c.readFixture(t, c.fixtureName("after-install"))
	if string(got) != string(want) {
		t.Fatalf("%s: config after install =\n%s\nwant (golden fixture) =\n%s", c.target, got, want)
	}
	c.assertUnrelatedSnippetPresent(t, "config after install", got)
}

// testReinstallIsIdempotent: GIVEN an ownership-tagged entry already
// exists, WHEN longterm-mem re-installs (with a different resolved binary,
// forcing Decide's replace branch), THEN it replaces that entry in place —
// exactly one memberKey member/section exists afterward, its value is
// updated, and unrelated entries are still untouched.
func (c goldenWriterCase) testReinstallIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	stateDir := t.TempDir()
	configPath := c.seedConfig(t, dir, c.fixtureName("before"))

	if err := c.register(dir, stateDir, c.binary1); err != nil {
		t.Fatalf("%s: first install returned error: %v", c.target, err)
	}
	if err := c.register(dir, stateDir, c.binary2); err != nil {
		t.Fatalf("%s: second install (reinstall) returned error: %v", c.target, err)
	}

	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("%s: failed to read result config: %v", c.target, err)
	}
	want := c.readFixture(t, c.fixtureName("after-reinstall"))
	if string(got) != string(want) {
		t.Fatalf("%s: config after reinstall =\n%s\nwant (golden fixture) =\n%s", c.target, got, want)
	}
	c.assertUnrelatedSnippetPresent(t, "config after reinstall", got)

	// The "not a duplicate" half of idempotency: exactly one occurrence of
	// duplicateNeedle, checked on the raw bytes (not a decode, which would
	// silently hide a literal duplicate key/section by keeping only the
	// last one).
	if c.duplicateNeedle == "" {
		t.Fatalf("%s: goldenWriterCase.duplicateNeedle is empty — every case must name a real not-a-duplicate needle to assert", c.target)
	}
	needle := c.duplicateNeedle
	if count := strings.Count(string(got), needle); count != 1 {
		t.Fatalf("%s: config after reinstall has %d occurrences of %q, want exactly 1 (a duplicate entry, not a replace-in-place):\n%s", c.target, count, needle, got)
	}
}

// testUntaggedSameNamedEntryRefused: GIVEN an untagged memberKey-named
// entry that install-state does not own, WHEN longterm-mem installs, THEN
// it refuses with ErrConflict and — the load-bearing assertion — the
// config file is byte-identical to before the call and no .bak was
// created. A refusal that still rewrote the file while reporting a
// conflict would pass a weaker test that only checked the error.
func (c goldenWriterCase) testUntaggedSameNamedEntryRefused(t *testing.T) {
	dir := t.TempDir()
	stateDir := t.TempDir()
	configPath := c.seedConfig(t, dir, c.fixtureName("untagged"))
	before, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("%s: failed to read seeded config: %v", c.target, err)
	}

	err = c.register(dir, stateDir, c.binary1)
	if err == nil {
		t.Fatalf("%s: install over an untagged same-named entry returned nil error, want ErrConflict", c.target)
	}
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("%s: install error = %v, want errors.Is(err, ErrConflict)", c.target, err)
	}

	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("%s: failed to read config after refusal: %v", c.target, err)
	}
	if string(got) != string(before) {
		t.Fatalf("%s: config file was modified despite refusal:\nbefore =\n%s\nafter =\n%s", c.target, before, got)
	}

	if _, statErr := os.Stat(configPath + ".bak"); !os.IsNotExist(statErr) {
		t.Fatalf("%s: .bak was created despite refusal (stat err: %v) — nothing must be written on refuse", c.target, statErr)
	}
}

// TestJSONInstall_UnsavableInstallStateWithdrawsTheConfigWrite belongs
// with the harness rather than with a runtime's own scenarios because it
// pins jsonInstall's own contract, identically for every runtime: the
// config write and the install-state write are two separate effects, and
// the window between them is the one failure re-running cannot recover
// from. A config carrying a longterm-mem entry install-state has no record
// of is exactly the shape jsonInstall treats as someone else's entry, so
// without a rollback every later run would refuse with ErrConflict over a
// member longterm-mem itself wrote.
func TestJSONInstall_UnsavableInstallStateWithdrawsTheConfigWrite(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	original := []byte("{\n  \"mcpServers\": {\n    \"other-project-server\": {\"type\": \"stdio\"}\n  }\n}\n")
	if err := os.WriteFile(configPath, original, 0o600); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	// The save is stubbed rather than broken through the filesystem: every
	// way to make the state directory unwritable also breaks the load that
	// precedes it, so jsonInstall would return before writing anything and
	// the test would pass without ever exercising the window it is about.
	// Overriding saveInstallState (the package's nowFunc convention) is
	// what puts the failure in the one place it matters.
	stateDir := t.TempDir()
	restoreSave := saveInstallState
	saveInstallState = func(*InstallState, string) error { return errors.New("install-state is unwritable") }
	t.Cleanup(func() { saveInstallState = restoreSave })

	err := jsonInstall("claude", configPath, stateDir, "mcpServers", "longterm-mem", json.RawMessage(`{"type":"stdio"}`))
	if err == nil {
		t.Fatal("jsonInstall with an unsavable install-state returned nil error, want a failure")
	}

	got, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("read config after the failed install: %v", readErr)
	}
	if string(got) != string(original) {
		t.Fatalf("the config kept a longterm-mem entry install-state has no record of — every later run would refuse it as someone else's:\nbefore =\n%s\nafter =\n%s", original, got)
	}
}
