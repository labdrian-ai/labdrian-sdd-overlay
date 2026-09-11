package register

import "testing"

// piGoldenCase pins pi.go's golden-fixture scenarios to testdata/pi/*.json
// (Phase 4, R-005). See golden_writer_test.go for the shared harness this
// drives — before.json plays the role a freshly built mcp.json plays in
// practice (pipkg.Build ships an empty {"mcpServers":{}} skeleton, and this
// fixture layers one pre-existing unrelated server on top so the same test
// also covers "existing servers preserved").
func piGoldenCase() goldenWriterCase {
	return goldenWriterCase{
		target:         "pi",
		configFileName: piConfigFileName,
		memberKey:      "longterm-mem",
		testdataDir:    "testdata/pi",
		fixtureExt:     ".json",
		register:       RegisterPi,
		unregister: func(configRoot, stateDir string) (UnregisterOutcome, error) {
			return Unregister(piTarget, configRoot, stateDir)
		},
		binary1:          "/opt/labdrian-overlay/bin/longterm-mem",
		binary2:          "/opt/labdrian-overlay/bin/longterm-mem-v2",
		unrelatedSnippet: `"other-tool"`,
		duplicateNeedle:  `"longterm-mem":`,
	}
}

// TestRegisterPi_WritesMcpServersLongtermMem (4.1, R-005): a fresh mcp.json
// (unrelated server present, longterm-mem absent) gets exactly one
// ownership-tagged mcpServers.longterm-mem entry, unrelated content byte-
// identical.
func TestRegisterPi_WritesMcpServersLongtermMem(t *testing.T) {
	piGoldenCase().testUnrelatedEntriesPreserved(t)
}

// TestPi_ReinstallIsIdempotent: re-registering replaces the existing
// tagged entry in place, never appending a duplicate.
func TestPi_ReinstallIsIdempotent(t *testing.T) {
	piGoldenCase().testReinstallIsIdempotent(t)
}

// TestPi_UntaggedSameNamedEntryRefused: an untagged longterm-mem-named
// entry install-state does not own is refused (ErrConflict), file left
// byte-identical.
func TestPi_UntaggedSameNamedEntryRefused(t *testing.T) {
	piGoldenCase().testUntaggedSameNamedEntryRefused(t)
}

// TestPi_UninstallRemovesOwnedEntry: Unregister removes only the
// ownership-tagged entry it installed, leaving unrelated servers untouched.
func TestPi_UninstallRemovesOwnedEntry(t *testing.T) {
	piGoldenCase().testUninstallRemovesOwnedEntry(t)
}
