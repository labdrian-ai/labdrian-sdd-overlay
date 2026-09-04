package register

import "testing"

// claudeGoldenCase pins claude.go's golden-fixture scenarios to
// testdata/claude/*.json (11b.1-11b.3, R-016). See golden_writer_test.go
// for the shared three-scenario harness this drives.
func claudeGoldenCase() goldenWriterCase {
	return goldenWriterCase{
		target:         "claude",
		configFileName: claudeConfigFileName,
		memberKey:      "longterm-mem",
		testdataDir:    "testdata/claude",
		fixtureExt:     ".json",
		register:       RegisterClaude,
		unregister: func(configRoot, stateDir string) (UnregisterOutcome, error) {
			return Unregister(claudeTarget, configRoot, stateDir)
		},
		binary1:          "/usr/local/bin/longterm-mem",
		binary2:          "/usr/local/bin/longterm-mem-v2",
		unrelatedSnippet: `"other-project-server"`,
		duplicateNeedle:  `"longterm-mem":`,
	}
}

// TestClaude_UnrelatedEntriesPreserved (11b.1, R-016 scenario "Unrelated
// entries are preserved"): a golden fixture claude config with unrelated
// mcpServers entries; install adds only a new ownership-tagged
// longterm-mem entry, all pre-existing entries byte-identical.
func TestClaude_UnrelatedEntriesPreserved(t *testing.T) {
	claudeGoldenCase().testUnrelatedEntriesPreserved(t)
}

// TestClaude_ReinstallIsIdempotent (11b.2, R-016 scenario "Reinstall is
// idempotent"): re-installing replaces the existing tagged entry in place,
// never appending a duplicate.
func TestClaude_ReinstallIsIdempotent(t *testing.T) {
	claudeGoldenCase().testReinstallIsIdempotent(t)
}

// TestClaude_UntaggedSameNamedEntryRefused (11b.3, R-016 scenario
// "Untagged same-named entry is refused, not overwritten"): an untagged
// longterm-mem-named entry install-state does not own is refused (exit 6
// at the cmd layer), and the file is left byte-identical — nothing written.
func TestClaude_UntaggedSameNamedEntryRefused(t *testing.T) {
	claudeGoldenCase().testUntaggedSameNamedEntryRefused(t)
}

// TestClaude_LostInstallStateIsAdopted (D9, "install-state lockout"):
// losing install-state.json must not turn claude's own entry into someone
// else's.
func TestClaude_LostInstallStateIsAdopted(t *testing.T) {
	claudeGoldenCase().testLostInstallStateIsAdopted(t)
}

// TestClaude_LostInstallStateWithAForeignEntryIsStillRefused: adoption
// keys on the entry's bytes, never on its name.
func TestClaude_LostInstallStateWithAForeignEntryIsStillRefused(t *testing.T) {
	claudeGoldenCase().testLostInstallStateWithAForeignEntryIsStillRefused(t)
}

// TestClaude_MissingContainerIsSynthesized (A4): a ~/.claude.json with no
// mcpServers key at all is installed into, not hard-failed.
func TestClaude_MissingContainerIsSynthesized(t *testing.T) {
	claudeGoldenCase().testMissingContainerIsSynthesized(t)
}
