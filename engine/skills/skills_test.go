package skills

import (
	"bytes"
	"strings"
	"testing"
)

// skillsTestRegistryYAML is a minimal two-entry fixture for dispatcher tests.
const skillsTestRegistryYAML = `version: "1"
skills:
  - id: test-core
    path: test-core
    source:
      type: core
      upstream:
        owner: gentle-ai
    install:
      defaultScope: global
      targets:
        - claude
    lifecycle:
      updateStrategy: vendor-merge
  - id: test-custom
    path: test-custom
    source:
      type: custom
    install:
      defaultScope: global
      targets:
        - opencode
    lifecycle:
      updateStrategy: overlay-only
`

// skillsMockReadFile returns the two-entry fixture for any path.
func skillsMockReadFile(_ string) ([]byte, error) {
	return []byte(skillsTestRegistryYAML), nil
}

func TestSkillsCore(t *testing.T) {
	t.Run("unknown_verb", func(t *testing.T) {
		// Unknown verb "nuke" → exit 1, stderr contains the verb name.
		var out, errBuf bytes.Buffer
		exitCode := 0
		skillsCore("nuke", nil, skillsMockReadFile, testRegistries(skillsMockReadFile), &out, &errBuf, func(c int) { exitCode = c })
		if exitCode != 1 {
			t.Errorf("exit code = %d, want 1", exitCode)
		}
		if !strings.Contains(errBuf.String(), "nuke") {
			t.Errorf("stderr %q should contain verb name 'nuke'", errBuf.String())
		}
	})

	t.Run("empty_verb", func(t *testing.T) {
		// Empty verb → exit 1, stderr non-empty.
		var out, errBuf bytes.Buffer
		exitCode := 0
		skillsCore("", nil, skillsMockReadFile, testRegistries(skillsMockReadFile), &out, &errBuf, func(c int) { exitCode = c })
		if exitCode != 1 {
			t.Errorf("exit code = %d, want 1", exitCode)
		}
		if errBuf.Len() == 0 {
			t.Error("stderr must be non-empty on empty verb")
		}
	})

	t.Run("SC_24_unknown_verb_lists_install", func(t *testing.T) {
		// SC-24: unknown verb → exit 1, stderr contains "install" in the supported verb list.
		var out, errBuf bytes.Buffer
		exitCode := 0
		skillsCore("frobnicate", nil, skillsMockReadFile, testRegistries(skillsMockReadFile), &out, &errBuf, func(c int) { exitCode = c })
		if exitCode != 1 {
			t.Errorf("exit code = %d, want 1", exitCode)
		}
		if !strings.Contains(errBuf.String(), "install") {
			t.Errorf("stderr %q should contain 'install' in supported verb list", errBuf.String())
		}
	})
}

// ── T-08: CLI dispatch tests for add/remove ──────────────────────────────────

// TestSkillsCoreUnknownVerbMessage verifies SC-37: an unknown verb exits 1 and
// the error message lists both "add" and "remove" as supported verbs.
func TestSkillsCoreUnknownVerbMessage(t *testing.T) {
	var out, errBuf bytes.Buffer
	exitCode := 0
	skillsCore("bogus", nil, skillsMockReadFile, testRegistries(skillsMockReadFile), &out, &errBuf, func(c int) { exitCode = c })
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}
	if !strings.Contains(errBuf.String(), "add") {
		t.Errorf("stderr %q should contain 'add' in supported verb list", errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "remove") {
		t.Errorf("stderr %q should contain 'remove' in supported verb list", errBuf.String())
	}
}

// ── sync-manifest in the list of verbs (SC-52) ───────────────────────────────

// TestSkillsCoreDispatchSyncManifest verifies that the verbs listed for an unknown or empty verb
// include "sync-manifest" (SC-52). The verb itself runs behind its use case, whose adapter is
// tested in engine/cmd.
func TestSkillsCoreDispatchSyncManifest(t *testing.T) {
	t.Run("SC-52_unknown_verb_lists_sync_manifest", func(t *testing.T) {
		var out, errBuf bytes.Buffer
		exitCode := 0
		skillsCore("bogus-after-sync", nil, skillsMockReadFile, testRegistries(skillsMockReadFile), &out, &errBuf, func(c int) { exitCode = c })
		if exitCode != 1 {
			t.Errorf("exit code = %d, want 1", exitCode)
		}
		if !strings.Contains(errBuf.String(), "sync-manifest") {
			t.Errorf("stderr %q should contain 'sync-manifest' in supported verb list", errBuf.String())
		}
	})

	t.Run("SC-52_empty_verb_lists_sync_manifest", func(t *testing.T) {
		var out, errBuf bytes.Buffer
		exitCode := 0
		skillsCore("", nil, skillsMockReadFile, testRegistries(skillsMockReadFile), &out, &errBuf, func(c int) { exitCode = c })
		if exitCode != 1 {
			t.Errorf("exit code = %d, want 1", exitCode)
		}
		if !strings.Contains(errBuf.String(), "sync-manifest") {
			t.Errorf("stderr %q should contain 'sync-manifest' in supported verb list", errBuf.String())
		}
	})
}
