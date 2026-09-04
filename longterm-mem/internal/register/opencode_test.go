package register

import "testing"

// opencodeGoldenCase pins opencode.go's golden-fixture scenarios to
// testdata/opencode/*.json (11b.5, R-017). See golden_writer_test.go for
// the shared three-scenario harness this drives — the same harness
// claude_test.go uses, pointed at opencode's own fixtures and Register
// entry point: adding this runtime meant adding a goldenWriterCase value
// and fixtures, not copying claude_test.go's test bodies.
func opencodeGoldenCase() goldenWriterCase {
	return goldenWriterCase{
		target:         "opencode",
		configFileName: opencodeConfigFileName,
		memberKey:      "longterm-mem",
		testdataDir:    "testdata/opencode",
		fixtureExt:     ".json",
		register:       RegisterOpencode,
		unregister: func(configRoot, stateDir string) (UnregisterOutcome, error) {
			return Unregister(opencodeTarget, configRoot, stateDir)
		},
		binary1:          "/opt/labdrian-overlay/bin/longterm-mem",
		binary2:          "/opt/labdrian-overlay/bin/longterm-mem-v2",
		unrelatedSnippet: `"other-tool"`,
		duplicateNeedle:  `"longterm-mem":`,
	}
}

// TestOpencode_UnrelatedEntriesPreserved (11b.5, R-017 scenario "Unrelated
// entries are preserved").
func TestOpencode_UnrelatedEntriesPreserved(t *testing.T) {
	opencodeGoldenCase().testUnrelatedEntriesPreserved(t)
}

// TestOpencode_ReinstallIsIdempotent (11b.5, R-017 scenario "Reinstall is
// idempotent").
func TestOpencode_ReinstallIsIdempotent(t *testing.T) {
	opencodeGoldenCase().testReinstallIsIdempotent(t)
}

// TestOpencode_UntaggedSameNamedEntryRefused (11b.5, R-017 scenario
// "Untagged same-named entry is refused, not overwritten").
func TestOpencode_UntaggedSameNamedEntryRefused(t *testing.T) {
	opencodeGoldenCase().testUntaggedSameNamedEntryRefused(t)
}

// TestOpencode_LostInstallStateIsAdopted (D9, "install-state lockout").
func TestOpencode_LostInstallStateIsAdopted(t *testing.T) {
	opencodeGoldenCase().testLostInstallStateIsAdopted(t)
}

// TestOpencode_LostInstallStateWithAForeignEntryIsStillRefused.
func TestOpencode_LostInstallStateWithAForeignEntryIsStillRefused(t *testing.T) {
	opencodeGoldenCase().testLostInstallStateWithAForeignEntryIsStillRefused(t)
}

// TestOpencode_MissingContainerIsSynthesized (A4): a fresh opencode
// install has no "mcp" key at all, which makes this the common case on a
// new machine rather than an exotic one.
func TestOpencode_MissingContainerIsSynthesized(t *testing.T) {
	opencodeGoldenCase().testMissingContainerIsSynthesized(t)
}
