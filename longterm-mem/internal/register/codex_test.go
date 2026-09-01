package register

import "testing"

// codexGoldenCase pins codex.go's golden-fixture scenarios to
// testdata/codex/*.toml (12a.3-12a.4, R-018). It drives the exact same
// goldenWriterCase harness claude_test.go and opencode_test.go use
// (golden_writer_test.go), with fixtureExt set to ".toml" instead of
// ".json" and a TOML-syntax duplicateNeedle — see golden_writer_test.go's
// updated doc comment for why that is a field, not a second harness.
func codexGoldenCase() goldenWriterCase {
	return goldenWriterCase{
		target:         "codex",
		configFileName: codexConfigFileName,
		memberKey:      "longterm-mem",
		testdataDir:    "testdata/codex",
		fixtureExt:     ".toml",
		register:       RegisterCodex,
		unregister: func(configRoot, stateDir string) (UnregisterOutcome, error) {
			return Unregister(codexTarget, configRoot, stateDir)
		},
		binary1:          "/usr/local/bin/longterm-mem",
		binary2:          "/usr/local/bin/longterm-mem-v2",
		unrelatedSnippet: `other-project-server`,
		duplicateNeedle:  `[mcp_servers.longterm-mem]`,
	}
}

// TestCodex_UnrelatedSectionsAndOrderingPreserved (12a.3, R-018 scenario
// "Unrelated sections and ordering are preserved"): a golden fixture
// config.toml with an existing unrelated mcp_servers section and unrelated
// top-level keys; install adds only a new longterm-mem section, and every
// other section/key ordering stays unchanged.
func TestCodex_UnrelatedSectionsAndOrderingPreserved(t *testing.T) {
	codexGoldenCase().testUnrelatedEntriesPreserved(t)
}

// TestCodex_ReinstallIsIdempotent (12a.4, R-018 scenario "Reinstall is
// idempotent"): re-installing replaces the existing tagged section in
// place, never appending a duplicate.
func TestCodex_ReinstallIsIdempotent(t *testing.T) {
	codexGoldenCase().testReinstallIsIdempotent(t)
}

// TestCodex_UntaggedSameNamedEntryRefused mirrors claude/opencode's third
// scenario for codex, even though tasks.md names only the two R-018
// scenarios above for this slice: codex shares jsonInstall's sibling flow
// (tomlInstall) and the exact same Decide table, so the "an entry we don't
// own is refused, not overwritten" behavior is real for codex too and
// deserves the same proof the other two runtimes have, not an assumption
// that sharing the flow means sharing the coverage.
func TestCodex_UntaggedSameNamedEntryRefused(t *testing.T) {
	codexGoldenCase().testUntaggedSameNamedEntryRefused(t)
}
