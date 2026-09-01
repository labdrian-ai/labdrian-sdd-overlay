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
