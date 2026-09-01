package register

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// uninstallCase drives one runtime's real install-then-uninstall path, so
// jsonUninstall/tomlUninstall are proved by what they do to a file on disk
// rather than by their doc comments. R-019's three named scenarios are
// proved separately against whole golden fixtures; what only exists here
// is the mapping from Decide's Action to an UnregisterOutcome, and the
// on-disk effect each mapping is allowed to have.
type uninstallCase struct {
	target     string
	configFile string
	// untagged carries a longterm-mem entry install-state does NOT own.
	untagged string
	// empty carries the container and unrelated entries, but nothing of
	// ours.
	empty   string
	install func(configRoot, stateDir, binary string) error
}

func uninstallCases() []uninstallCase {
	return []uninstallCase{{
		target:     claudeTarget,
		configFile: claudeConfigFileName,
		untagged:   `{"mcpServers":{"other-project-server":{"type":"stdio"},"longterm-mem":{"type":"stdio","command":"/someone/elses/binary"}}}`,
		empty:      `{"mcpServers":{"other-project-server":{"type":"stdio"}}}`,
		install:    RegisterClaude,
	}, {
		target:     codexTarget,
		configFile: codexConfigFileName,
		untagged:   "theme = \"dark\"\n\n[mcp_servers.longterm-mem]\ncommand = \"/someone/elses/binary\"\n",
		empty:      "theme = \"dark\"\n",
		install:    RegisterCodex,
	}}
}

// TestUnregister_OutcomeAndOnDiskEffect pins all three of Decide's
// reachable uninstall outcomes together, because the interesting property
// is the pairing: WHICH outcome is reported and WHAT it was allowed to do
// to the file. Reporting `unmanaged` while having deleted the entry, or
// `removed` while having left the ownership record behind, each passes a
// test that checks only one half.
func TestUnregister_OutcomeAndOnDiskEffect(t *testing.T) {
	for _, c := range uninstallCases() {
		for _, s := range []struct {
			name string
			// seed is the config written before the call.
			seed string
			// install performs a real registration first, so the removal
			// path runs against an entry longterm-mem genuinely owns.
			install bool
			want    UnregisterOutcome
		}{
			{name: "ours is removed, record and all", seed: c.empty, install: true, want: UnregisterRemoved},
			{name: "someone else's is left alone", seed: c.untagged, want: UnregisterUnmanaged},
			{name: "nothing installed is quiet", seed: c.empty, want: UnregisterNoop},
		} {
			t.Run(c.target+"/"+s.name, func(t *testing.T) {
				dir, stateDir := t.TempDir(), t.TempDir()
				configPath := filepath.Join(dir, c.configFile)
				if err := os.WriteFile(configPath, []byte(s.seed), 0o600); err != nil {
					t.Fatalf("seed %s: %v", configPath, err)
				}
				if s.install {
					if err := c.install(dir, stateDir, "/opt/bin/longterm-mem"); err != nil {
						t.Fatalf("install: %v", err)
					}
				}

				outcome, err := Unregister(c.target, dir, stateDir)
				if err != nil {
					t.Fatalf("Unregister: %v", err)
				}
				if outcome != s.want {
					t.Fatalf("outcome = %s, want %s", outcome, s.want)
				}

				got, err := os.ReadFile(configPath)
				if err != nil {
					t.Fatalf("read config: %v", err)
				}
				if s.want == UnregisterRemoved {
					// Both halves: the entry gone from the runtime's own
					// config AND the record gone from install-state. An
					// entry with no record reads as someone else's on the
					// next run; a record with no entry claims ownership of
					// nothing.
					if strings.Contains(string(got), "longterm-mem") {
						t.Fatalf("config still mentions longterm-mem after removal:\n%s", got)
					}
					state, err := LoadInstallState(filepath.Join(stateDir, installStateFileName))
					if err != nil {
						t.Fatalf("load install-state: %v", err)
					}
					if _, present := state.Get(c.target); present {
						t.Fatalf("install-state still records this target after removal")
					}
					return
				}
				// "Left it alone" is a claim about the file, not about the
				// return value: byte-identical, and no .bak, which would
				// mean something was written.
				if string(got) != s.seed {
					t.Fatalf("config was modified:\nbefore =\n%s\nafter =\n%s", s.seed, got)
				}
				if _, statErr := os.Stat(configPath + ".bak"); !os.IsNotExist(statErr) {
					t.Fatalf(".bak exists for a config nothing was written to (stat err: %v)", statErr)
				}
			})
		}
	}
}

// TestUnregister_SelectiveRemovalAcrossAllThreeRuntimes (12b.1, R-019
// scenario "Selective removal across all three runtimes"): claude,
// opencode and codex are each installed for real (through the real
// Register* writers, sharing ONE install-state.json the way a real
// machine's three targets do) with their own unrelated entries plus a
// longterm-mem entry. Unregistering ONE runtime (opencode) removes only
// that runtime's entry — the OTHER two runtimes' files, and every
// unrelated entry in the unregistered runtime's own file, are
// byte-identical to what Register* itself produced.
func TestUnregister_SelectiveRemovalAcrossAllThreeRuntimes(t *testing.T) {
	claude, opencode, codex := claudeGoldenCase(), opencodeGoldenCase(), codexGoldenCase()
	stateDir := t.TempDir() // shared install-state.json, mirroring a real machine's one file for all targets

	claudeDir, opencodeDir, codexDir := t.TempDir(), t.TempDir(), t.TempDir()
	claudePath := claude.seedConfig(t, claudeDir, claude.fixtureName("before"))
	opencodePath := opencode.seedConfig(t, opencodeDir, opencode.fixtureName("before"))
	codexPath := codex.seedConfig(t, codexDir, codex.fixtureName("before"))

	for _, tc := range []struct {
		dir string
		gc  goldenWriterCase
	}{
		{claudeDir, claude},
		{opencodeDir, opencode},
		{codexDir, codex},
	} {
		if err := tc.gc.register(tc.dir, stateDir, tc.gc.binary1); err != nil {
			t.Fatalf("%s: install returned error: %v", tc.gc.target, err)
		}
	}

	outcome, err := Unregister("opencode", opencodeDir, stateDir)
	if err != nil {
		t.Fatalf("Unregister(opencode) returned error: %v", err)
	}
	if outcome != UnregisterRemoved {
		t.Fatalf("Unregister(opencode) outcome = %v, want UnregisterRemoved", outcome)
	}

	// The unregistered runtime: back to its own before-install bytes,
	// unrelated entries included, proven byte-for-byte against the golden
	// after-uninstall fixture.
	opencodeGot, err := os.ReadFile(opencodePath)
	if err != nil {
		t.Fatalf("read opencode config: %v", err)
	}
	opencodeWant := opencode.readFixture(t, opencode.fixtureName("after-uninstall"))
	if string(opencodeGot) != string(opencodeWant) {
		t.Fatalf("opencode config after unregister =\n%s\nwant =\n%s", opencodeGot, opencodeWant)
	}

	// The OTHER two runtimes: untouched by an opencode-only unregister,
	// still carrying their own installed entry and their own unrelated
	// entries — proven byte-for-byte against the golden after-install
	// fixture each already had to satisfy for Register* itself.
	claudeGot, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("read claude config: %v", err)
	}
	claudeWant := claude.readFixture(t, claude.fixtureName("after-install"))
	if string(claudeGot) != string(claudeWant) {
		t.Fatalf("claude config changed by an opencode-only unregister:\ngot =\n%s\nwant =\n%s", claudeGot, claudeWant)
	}
	codexGot, err := os.ReadFile(codexPath)
	if err != nil {
		t.Fatalf("read codex config: %v", err)
	}
	codexWant := codex.readFixture(t, codex.fixtureName("after-install"))
	if string(codexGot) != string(codexWant) {
		t.Fatalf("codex config changed by an opencode-only unregister:\ngot =\n%s\nwant =\n%s", codexGot, codexWant)
	}
}

// TestUnregister_UntaggedEntryPreservedAndReported (12b.2, R-019 scenario
// "Untagged entry is preserved and reported, not removed"): run for all
// three runtimes, since the untagged conflict is real and reachable for
// every one of them (the exact same reasoning 12a's own third scenario,
// TestCodex_UntaggedSameNamedEntryRefused, gave for the install side).
func TestUnregister_UntaggedEntryPreservedAndReported(t *testing.T) {
	t.Run("claude", func(t *testing.T) { claudeGoldenCase().testUninstallUntaggedEntryPreservedAndReported(t) })
	t.Run("opencode", func(t *testing.T) { opencodeGoldenCase().testUninstallUntaggedEntryPreservedAndReported(t) })
	t.Run("codex", func(t *testing.T) { codexGoldenCase().testUninstallUntaggedEntryPreservedAndReported(t) })
}

// TestUnregister_PartialUninstallKeepsSharedBinary (12b.3, R-019 scenario
// "Partial uninstall does not remove the shared binary"): the longterm-mem
// binary itself is bin/labdrian-overlay's own concern
// (longtermmem_maybe_remove_binary, 10b/12b.6) — this package has no
// notion of a binary path to remove at all. What THIS layer owns, and what
// that bash-level guard depends on being correct, is that unregistering
// one target never disturbs another target's install-state record: with
// all three targets recorded in ONE install-state.json (mirroring a real
// machine), unregistering "claude" alone must leave "opencode" and "codex"
// fully intact — both their config entries AND their install-state
// records — proving the Go-level half of "a still-installed target keeps
// claiming the binary" (LoadInstallState's Targets map is the very
// question a future doctor/status/uninstall-all run would ask).
func TestUnregister_PartialUninstallKeepsSharedBinary(t *testing.T) {
	claude, opencode, codex := claudeGoldenCase(), opencodeGoldenCase(), codexGoldenCase()
	stateDir := t.TempDir()

	claudeDir, opencodeDir, codexDir := t.TempDir(), t.TempDir(), t.TempDir()
	claude.seedConfig(t, claudeDir, claude.fixtureName("before"))
	opencodePath := opencode.seedConfig(t, opencodeDir, opencode.fixtureName("before"))
	codexPath := codex.seedConfig(t, codexDir, codex.fixtureName("before"))

	for _, tc := range []struct {
		dir string
		gc  goldenWriterCase
	}{
		{claudeDir, claude},
		{opencodeDir, opencode},
		{codexDir, codex},
	} {
		if err := tc.gc.register(tc.dir, stateDir, tc.gc.binary1); err != nil {
			t.Fatalf("%s: install returned error: %v", tc.gc.target, err)
		}
	}

	outcome, err := Unregister("claude", claudeDir, stateDir)
	if err != nil {
		t.Fatalf("Unregister(claude) returned error: %v", err)
	}
	if outcome != UnregisterRemoved {
		t.Fatalf("Unregister(claude) outcome = %v, want UnregisterRemoved", outcome)
	}

	statePath := filepath.Join(stateDir, installStateFileName)
	state, err := LoadInstallState(statePath)
	if err != nil {
		t.Fatalf("LoadInstallState after partial unregister: %v", err)
	}
	if _, ok := state.Get("claude"); ok {
		t.Fatalf("claude's install-state record still present after Unregister(claude)")
	}
	if _, ok := state.Get("opencode"); !ok {
		t.Fatalf("opencode's install-state record was removed by an unrelated claude-only unregister — the shared binary would be removed out from under a still-installed target")
	}
	if _, ok := state.Get("codex"); !ok {
		t.Fatalf("codex's install-state record was removed by an unrelated claude-only unregister — the shared binary would be removed out from under a still-installed target")
	}

	// The config-file half of the same guarantee: opencode's and codex's
	// own files still carry their installed entry, untouched by an
	// unrelated target's unregister.
	opencodeGot, err := os.ReadFile(opencodePath)
	if err != nil {
		t.Fatalf("read opencode config: %v", err)
	}
	opencodeWant := opencode.readFixture(t, opencode.fixtureName("after-install"))
	if string(opencodeGot) != string(opencodeWant) {
		t.Fatalf("opencode config changed by a claude-only unregister:\ngot =\n%s\nwant =\n%s", opencodeGot, opencodeWant)
	}
	codexGot, err := os.ReadFile(codexPath)
	if err != nil {
		t.Fatalf("read codex config: %v", err)
	}
	codexWant := codex.readFixture(t, codex.fixtureName("after-install"))
	if string(codexGot) != string(codexWant) {
		t.Fatalf("codex config changed by a claude-only unregister:\ngot =\n%s\nwant =\n%s", codexGot, codexWant)
	}
}

// TestUnregister_RemovalIsTheInverseOfInstall wires the harness's
// install-then-uninstall round trip for ALL THREE runtimes. Without this
// fan-out the scenario is an unexported method Go never rejects and
// `go test` never runs, and two of the three after-uninstall fixtures are
// files nothing reads — so R-019's "Remove/TOMLRemove are genuine
// inverses of the insert path" would be asserted for opencode alone, and
// not at all for the whole TOML removal path.
func TestUnregister_RemovalIsTheInverseOfInstall(t *testing.T) {
	t.Run("claude", func(t *testing.T) { claudeGoldenCase().testUninstallRemovesOwnedEntry(t) })
	t.Run("opencode", func(t *testing.T) { opencodeGoldenCase().testUninstallRemovesOwnedEntry(t) })
	t.Run("codex", func(t *testing.T) { codexGoldenCase().testUninstallRemovesOwnedEntry(t) })
}
