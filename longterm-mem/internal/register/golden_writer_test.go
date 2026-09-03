package register

import (
	"bytes"
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
//	after-uninstall.<ext> the expected config after Unregister removes the
//	                      entry Register* installed against before.<ext> --
//	                      the fixture slice 12b (R-019) creates, per this
//	                      comment's own earlier promise (see
//	                      apply-progress.md Slice 11b).
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
	// unregister drives Unregister for this runtime's own target name
	// (12b, R-019) -- a thin closure rather than a fourth field naming the
	// target string again, since Unregister already takes target as a
	// plain argument and every goldenWriterCase value already knows its
	// own target.
	unregister func(configRoot, stateDir string) (UnregisterOutcome, error)
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

// testUninstallRemovesOwnedEntry (12b, R-019 "Removing a span is harder
// than replacing one"): GIVEN before.<ext> installed for real (so
// install-state genuinely owns this target, not merely a golden fixture on
// disk with nothing behind it), WHEN Unregister runs, THEN the result is
// byte-identical to the golden after-uninstall.<ext> fixture — for every
// runtime this slice covers, that fixture is byte-identical to before.<ext>
// itself, proving Remove/TOMLRemove are genuine inverses of Splice/
// TOMLSplice's own insert path, not merely "produces something valid" —
// and the unrelated snippet survives byte-for-byte throughout.
func (c goldenWriterCase) testUninstallRemovesOwnedEntry(t *testing.T) {
	dir := t.TempDir()
	stateDir := t.TempDir()
	configPath := c.seedConfig(t, dir, c.fixtureName("before"))

	if err := c.register(dir, stateDir, c.binary1); err != nil {
		t.Fatalf("%s: install returned error: %v", c.target, err)
	}

	outcome, err := c.unregister(dir, stateDir)
	if err != nil {
		t.Fatalf("%s: unregister returned error: %v", c.target, err)
	}
	if outcome != UnregisterRemoved {
		t.Fatalf("%s: unregister outcome = %v, want UnregisterRemoved", c.target, outcome)
	}

	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("%s: failed to read result config: %v", c.target, err)
	}
	want := c.readFixture(t, c.fixtureName("after-uninstall"))
	if string(got) != string(want) {
		t.Fatalf("%s: config after uninstall =\n%s\nwant (golden fixture) =\n%s", c.target, got, want)
	}
	c.assertUnrelatedSnippetPresent(t, "config after uninstall", got)
}

// testUninstallUntaggedEntryPreservedAndReported (12b.2, R-019 "Untagged
// entry is preserved and reported, not removed"): GIVEN an untagged
// memberKey-named entry install-state does not own, WHEN Unregister runs,
// THEN it reports UnregisterUnmanaged and — the load-bearing assertion,
// mirroring testUntaggedSameNamedEntryRefused's own — the config file is
// byte-identical to before the call. A silent no-op that merely returned
// nil without reporting anything would pass a weaker test than this one.
func (c goldenWriterCase) testUninstallUntaggedEntryPreservedAndReported(t *testing.T) {
	dir := t.TempDir()
	stateDir := t.TempDir()
	configPath := c.seedConfig(t, dir, c.fixtureName("untagged"))
	before, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("%s: failed to read seeded config: %v", c.target, err)
	}

	outcome, err := c.unregister(dir, stateDir)
	if err != nil {
		t.Fatalf("%s: unregister returned error: %v", c.target, err)
	}
	if outcome != UnregisterUnmanaged {
		t.Fatalf("%s: unregister outcome = %v, want UnregisterUnmanaged", c.target, outcome)
	}

	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("%s: failed to read config after unregister: %v", c.target, err)
	}
	if string(got) != string(before) {
		t.Fatalf("%s: config file was modified despite being unmanaged:\nbefore =\n%s\nafter =\n%s", c.target, before, got)
	}
}

// testMissingContainerIsSynthesized (A4) is JSON-only: GIVEN a runtime
// config that exists but declares no MCP container object at all — a fresh
// opencode install has no "mcp" key, which makes this the ordinary case on
// a new machine, not an exotic one — WHEN longterm-mem installs, THEN it
// synthesizes the container at the document root, exactly as the TOML
// writer already appends a missing table at EOF, and every unrelated byte
// survives.
//
// codex does not drive this scenario: TOML has no container object to
// synthesize (tomlsplice.go's apply appends at EOF), so its equivalent has
// always worked and is covered by testUnrelatedEntriesPreserved.
func (c goldenWriterCase) testMissingContainerIsSynthesized(t *testing.T) {
	dir := t.TempDir()
	stateDir := t.TempDir()
	configPath := c.seedConfig(t, dir, c.fixtureName("before-no-container"))
	before := c.readFixture(t, c.fixtureName("before-no-container"))

	if err := c.register(dir, stateDir, c.binary1); err != nil {
		t.Fatalf("%s: install into a config with no MCP container returned error: %v", c.target, err)
	}

	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("%s: failed to read result config: %v", c.target, err)
	}
	want := c.readFixture(t, c.fixtureName("after-install-no-container"))
	if string(got) != string(want) {
		t.Fatalf("%s: config after install =\n%s\nwant (golden fixture) =\n%s", c.target, got, want)
	}
	// Stronger, and fixture-independent, than naming one unrelated snippet:
	// EVERY byte of the original document must still be there, in order,
	// split into a prefix and a suffix by the one inserted span (D9).
	if !isPureInsertion(before, got) {
		t.Fatalf("%s: synthesizing the container rewrote bytes outside the inserted span:\nbefore =\n%s\nafter =\n%s", c.target, before, got)
	}

	// The synthesis is a write like any other, so the backup safety net the
	// decision to synthesize rests on must have fired first.
	bak, err := os.ReadFile(configPath + ".bak")
	if err != nil {
		t.Fatalf("%s: no .bak beside a config whose container was synthesized: %v", c.target, err)
	}
	if string(bak) != string(before) {
		t.Fatalf("%s: .bak does not hold the pre-edit bytes:\ngot =\n%s\nwant =\n%s", c.target, bak, before)
	}
}

// isPureInsertion reports whether after differs from before by exactly one
// contiguous insertion: every byte of before survives, in order, as a
// prefix and a suffix of after. It is the byte-identity contract D9 states
// for unrelated content, checked without naming any particular fixture
// content.
func isPureInsertion(before, after []byte) bool {
	if len(after) < len(before) {
		return false
	}
	prefix := 0
	for prefix < len(before) && before[prefix] == after[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(before)-prefix && before[len(before)-1-suffix] == after[len(after)-1-suffix] {
		suffix++
	}
	return prefix+suffix == len(before)
}

// installStatePath is where this case's ownership record lives inside
// stateDir — the one file the two scenarios below delete on purpose.
func (c goldenWriterCase) installStatePath(stateDir string) string {
	return filepath.Join(stateDir, installStateFileName)
}

// testLostInstallStateIsAdopted (D9, the "install-state lockout" defect):
// GIVEN a runtime registered for real and install-state.json then lost
// (restored from a backup that predates it, a wiped state directory, a
// --state-dir typo), WHEN longterm-mem registers again with the same
// binary, THEN it re-derives ownership from the entry itself — the entry
// on disk is byte-identical to the one this call would write, so it can
// only be one longterm-mem wrote — restores the record, and leaves the
// runtime's own config byte-identical.
//
// Without that re-derivation, losing one small file in longterm-mem's own
// state directory locks the user out of every runtime at once: each entry
// reads as someone else's, register refuses (exit 6), unregister reports
// unmanaged, and the only recovery is hand-editing all three runtime
// configs.
func (c goldenWriterCase) testLostInstallStateIsAdopted(t *testing.T) {
	dir := t.TempDir()
	stateDir := t.TempDir()
	configPath := c.seedConfig(t, dir, c.fixtureName("before"))

	if err := c.register(dir, stateDir, c.binary1); err != nil {
		t.Fatalf("%s: first install returned error: %v", c.target, err)
	}
	installed, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("%s: failed to read config after the first install: %v", c.target, err)
	}

	if err := os.Remove(c.installStatePath(stateDir)); err != nil {
		t.Fatalf("%s: failed to remove install-state.json: %v", c.target, err)
	}

	if err := c.register(dir, stateDir, c.binary1); err != nil {
		t.Fatalf("%s: re-install after losing install-state.json returned error: %v — a lost ownership record must not lock the user out of their own entry", c.target, err)
	}

	state, err := LoadInstallState(c.installStatePath(stateDir))
	if err != nil {
		t.Fatalf("%s: failed to load the restored install-state: %v", c.target, err)
	}
	record, present := state.Get(c.target)
	if !present {
		t.Fatalf("%s: install-state has no record after re-install — the ownership record was not restored", c.target)
	}

	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("%s: failed to read config after the re-install: %v", c.target, err)
	}
	if string(got) != string(installed) {
		t.Fatalf("%s: config changed while adopting an entry identical to the one it would write:\nbefore =\n%s\nafter =\n%s", c.target, installed, got)
	}

	// The restored record must be the fingerprint of the entry actually on
	// disk, not merely any non-empty string: a record that does not match
	// what is installed would send the very next run down the "stale, rewrite
	// it" branch forever.
	if record.Fingerprint == "" {
		t.Fatalf("%s: restored record has an empty fingerprint: %+v", c.target, record)
	}
	outcome, err := c.unregister(dir, stateDir)
	if err != nil {
		t.Fatalf("%s: unregister after adoption returned error: %v", c.target, err)
	}
	if outcome != UnregisterRemoved {
		t.Fatalf("%s: unregister after adoption = %v, want UnregisterRemoved — the restored record must be the fingerprint of the entry actually on disk", c.target, outcome)
	}
}

// testLostInstallStateWithAForeignEntryIsStillRefused is the other half of
// the adoption rule, and the reason it is safe: adoption keys on the
// entry's exact bytes, never on its name. GIVEN install-state has no
// record and the same-named entry on disk is NOT the one longterm-mem
// would write, WHEN longterm-mem installs, THEN it still refuses with
// ErrConflict and still leaves the file byte-identical.
//
// The untagged.<ext> fixture proves this for an entry longterm-mem never
// touched; this proves it for the harder case — an entry longterm-mem DID
// write and a human then edited, which is the closest a foreign entry can
// get to ours without being ours.
func (c goldenWriterCase) testLostInstallStateWithAForeignEntryIsStillRefused(t *testing.T) {
	dir := t.TempDir()
	stateDir := t.TempDir()
	configPath := c.seedConfig(t, dir, c.fixtureName("before"))

	if err := c.register(dir, stateDir, c.binary1); err != nil {
		t.Fatalf("%s: first install returned error: %v", c.target, err)
	}
	if err := os.Remove(c.installStatePath(stateDir)); err != nil {
		t.Fatalf("%s: failed to remove install-state.json: %v", c.target, err)
	}

	// Hand-edit the installed entry so it is no longer byte-identical to
	// what register would write, without changing its name.
	installed, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("%s: failed to read config after the first install: %v", c.target, err)
	}
	edited := bytes.Replace(installed, []byte(c.binary1), []byte("/edited/by/hand"), 1)
	if bytes.Equal(edited, installed) {
		t.Fatalf("%s: hand-edit changed nothing — the fixture no longer contains %q", c.target, c.binary1)
	}
	if err := os.WriteFile(configPath, edited, 0o600); err != nil {
		t.Fatalf("%s: failed to write the hand-edited config: %v", c.target, err)
	}

	err = c.register(dir, stateDir, c.binary1)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("%s: install over a hand-edited same-named entry with no record = %v, want errors.Is(err, ErrConflict)", c.target, err)
	}

	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("%s: failed to read config after the refusal: %v", c.target, err)
	}
	if string(got) != string(edited) {
		t.Fatalf("%s: config file was modified despite refusal:\nbefore =\n%s\nafter =\n%s", c.target, edited, got)
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

// TestRegisterOpencode_ConfigWithNoMCPKeyIsInstalledInto (A4) is the
// end-to-end half of TestJSONSplice_SynthesizesAMissingContainer: it drives
// a real RegisterOpencode call (and so the whole jsonInstall flow —
// Decide, WriteMember's json.Valid gate, the .bak, install-state) against
// the config a fresh opencode install actually writes, which has no "mcp"
// key at all. Before A4 this exited 1 with "container key not found",
// making the commonest config on a new machine the one register could not
// handle.
func TestRegisterOpencode_ConfigWithNoMCPKeyIsInstalledInto(t *testing.T) {
	configRoot := t.TempDir()
	configPath := filepath.Join(configRoot, opencodeConfigFileName)
	original := []byte(`{"theme":"system"}`)
	if err := os.WriteFile(configPath, original, 0o600); err != nil {
		t.Fatalf("seed opencode.json: %v", err)
	}
	stateDir := t.TempDir()

	if err := RegisterOpencode(configRoot, stateDir, "/opt/bin/longterm-mem"); err != nil {
		t.Fatalf("RegisterOpencode into a config with no %q key returned error: %v", opencodeContainerKey, err)
	}

	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read result config: %v", err)
	}
	if !bytes.Contains(got, []byte(`"theme":"system"`)) {
		t.Fatalf("the unrelated theme key did not survive verbatim:\n%s", got)
	}
	if !isPureInsertion(original, got) {
		t.Fatalf("synthesizing %q rewrote bytes outside the inserted span:\nbefore =\n%s\nafter =\n%s", opencodeContainerKey, original, got)
	}

	entry, present, err := readMember(configPath, opencodeContainerKey, "longterm-mem")
	if err != nil {
		t.Fatalf("read back the installed entry: %v", err)
	}
	if !present {
		t.Fatalf("%s.longterm-mem is not present after install:\n%s", opencodeContainerKey, got)
	}
	if !bytes.Contains(entry, []byte("/opt/bin/longterm-mem")) {
		t.Fatalf("installed entry does not name the binary: %s", entry)
	}

	// The backup is the safety net the decision to synthesize rests on, so
	// it must hold the pre-edit bytes.
	bak, err := os.ReadFile(configPath + ".bak")
	if err != nil {
		t.Fatalf("no .bak beside a config whose container was synthesized: %v", err)
	}
	if string(bak) != string(original) {
		t.Fatalf(".bak does not hold the pre-edit bytes:\ngot = %s\nwant = %s", bak, original)
	}
}
