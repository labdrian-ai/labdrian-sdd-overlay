package skills

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
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
	t.Run("verb_list", func(t *testing.T) {
		// "list" with valid registry → exit 0, output has entries.
		var out, errBuf bytes.Buffer
		exitCode := 0
		SkillsCore("list", nil, skillsMockReadFile, &out, &errBuf, func(c int) { exitCode = c })
		if exitCode != 0 {
			t.Errorf("exit code = %d, want 0; stderr=%q", exitCode, errBuf.String())
		}
		if out.Len() == 0 {
			t.Error("stdout must be non-empty for list")
		}
		if !strings.Contains(out.String(), "test-core") {
			t.Errorf("stdout %q missing test-core entry", out.String())
		}
	})

	t.Run("verb_status", func(t *testing.T) {
		// "status" with valid registry → exit 0, counts and OK in stdout.
		var out, errBuf bytes.Buffer
		exitCode := 0
		SkillsCore("status", nil, skillsMockReadFile, &out, &errBuf, func(c int) { exitCode = c })
		if exitCode != 0 {
			t.Errorf("exit code = %d, want 0; stderr=%q", exitCode, errBuf.String())
		}
		if !strings.Contains(out.String(), "OK") {
			t.Errorf("stdout %q missing OK", out.String())
		}
	})

	t.Run("verb_validate", func(t *testing.T) {
		// "validate" with aligned registry+manifest+skills dir → exit 0, "aligned" or "OK" in stdout.
		// The skills source root is a DEDICATED subdirectory, never the temp dir
		// holding registry.yaml/overlay.manifest: ScanSkillFiles would otherwise
		// pick those two files up as UNREGISTERED_ON_DISK too.
		dir := t.TempDir()
		regPath := filepath.Join(dir, "registry.yaml")
		mfPath := filepath.Join(dir, "overlay.manifest")
		skillsRoot := filepath.Join(dir, "skills")

		const alignedReg = `version: "1"
skills:
  - id: sdd-spec
    path: sdd-spec
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
`
		if err := os.WriteFile(regPath, []byte(alignedReg), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(mfPath, []byte("sdd-spec/SKILL.md managed\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(skillsRoot, "sdd-spec"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skillsRoot, "sdd-spec", "SKILL.md"), []byte("# sdd-spec\n"), 0644); err != nil {
			t.Fatal(err)
		}

		var out, errBuf bytes.Buffer
		exitCode := 0
		SkillsCore("validate",
			[]string{"--registry", regPath, "--manifest", mfPath, "--source-root", skillsRoot},
			os.ReadFile, &out, &errBuf, func(c int) { exitCode = c })
		if exitCode != 0 {
			t.Errorf("exit code = %d, want 0; stderr=%q", exitCode, errBuf.String())
		}
		outStr := out.String()
		if !strings.Contains(outStr, "aligned") && !strings.Contains(outStr, "OK") {
			t.Errorf("stdout %q should contain 'aligned' or 'OK'", outStr)
		}
	})

	t.Run("unknown_verb", func(t *testing.T) {
		// Unknown verb "nuke" → exit 1, stderr contains the verb name.
		var out, errBuf bytes.Buffer
		exitCode := 0
		SkillsCore("nuke", nil, skillsMockReadFile, &out, &errBuf, func(c int) { exitCode = c })
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
		SkillsCore("", nil, skillsMockReadFile, &out, &errBuf, func(c int) { exitCode = c })
		if exitCode != 1 {
			t.Errorf("exit code = %d, want 1", exitCode)
		}
		if errBuf.Len() == 0 {
			t.Error("stderr must be non-empty on empty verb")
		}
	})

	t.Run("SC_23_install_verb_routes_to_install_core", func(t *testing.T) {
		// SC-23: verb "install" routes to RenderInstallCore, not the default error branch.
		// We pass --project-id and --source-root so the core can run cleanly.
		// The mock registry has only global skills → empty plan → exit 0 with notice.
		var out, errBuf bytes.Buffer
		exitCode := -1
		SkillsCore(
			"install",
			[]string{"--registry", "reg.yaml", "--source-root", "/nonexistent", "--project-id", "test-project"},
			skillsMockReadFile,
			&out, &errBuf,
			func(c int) { exitCode = c },
		)
		// Must not produce "unknown skills verb" error.
		if strings.Contains(errBuf.String(), "unknown skills verb") {
			t.Errorf("stderr %q should not contain 'unknown skills verb'", errBuf.String())
		}
		// Empty plan (mock registry has global skills only) → exit 0.
		if exitCode != 0 {
			t.Errorf("exit code = %d, want 0; stderr: %q", exitCode, errBuf.String())
		}
		if !strings.Contains(out.String(), "no project-scoped skills") {
			t.Errorf("stdout %q should contain empty-plan notice", out.String())
		}
	})

	t.Run("SC_24_unknown_verb_lists_install", func(t *testing.T) {
		// SC-24: unknown verb → exit 1, stderr contains "install" in the supported verb list.
		var out, errBuf bytes.Buffer
		exitCode := 0
		SkillsCore("frobnicate", nil, skillsMockReadFile, &out, &errBuf, func(c int) { exitCode = c })
		if exitCode != 1 {
			t.Errorf("exit code = %d, want 1", exitCode)
		}
		if !strings.Contains(errBuf.String(), "install") {
			t.Errorf("stderr %q should contain 'install' in supported verb list", errBuf.String())
		}
	})
}

// stubScan returns a scanSkills-shaped function that ignores the directory it
// is given and always returns files, unconditionally. It lets on-disk gate
// core tests exercise RenderValidateCore's logic without walking a real
// skills tree (R-003).
func stubScan(files []string) func(string) ([]string, error) {
	return func(string) ([]string, error) { return files, nil }
}

// mustWriteValidateFixture writes a minimal registry + manifest pair to a
// fresh temp dir and returns their paths.
func mustWriteValidateFixture(t *testing.T, manifestContent string) (regPath, mfPath string) {
	t.Helper()
	const alignedReg = `version: "1"
skills:
  - id: sdd-spec
    path: sdd-spec
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
`
	dir := t.TempDir()
	regPath = filepath.Join(dir, "registry.yaml")
	mfPath = filepath.Join(dir, "overlay.manifest")
	if err := os.WriteFile(regPath, []byte(alignedReg), 0644); err != nil {
		t.Fatalf("write registry: %v", err)
	}
	if err := os.WriteFile(mfPath, []byte(manifestContent), 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return regPath, mfPath
}

// TestRenderValidateCoreOnDiskGate covers the on-disk cross-check wired into
// RenderValidateCore behind the required --source-root flag (R-002, R-005,
// R-006, R-007). Every subtest but the cwd-independence one uses a stubbed
// scanSkills so the core is exercised without ever walking a real tree
// (R-003); registry/manifest fixtures are test-owned temp files, matching the
// pre-existing verb_validate pattern.
func TestRenderValidateCoreOnDiskGate(t *testing.T) {
	t.Run("absent_source_root_fails_loud_naming_the_flag", func(t *testing.T) {
		regPath, mfPath := mustWriteValidateFixture(t, "sdd-spec/SKILL.md managed\n")
		neverScan := func(string) ([]string, error) {
			t.Fatal("scanSkills must not be called when --source-root is absent")
			return nil, nil
		}

		var out, errBuf bytes.Buffer
		exitCode := -1
		RenderValidateCore(
			[]string{"--registry", regPath, "--manifest", mfPath},
			os.ReadFile, neverScan, &out, &errBuf,
			func(c int) { exitCode = c },
		)
		if exitCode != 1 {
			t.Errorf("exit code = %d, want 1", exitCode)
		}
		if !strings.Contains(errBuf.String(), "--source-root") {
			t.Errorf("stderr %q must name the missing --source-root flag", errBuf.String())
		}
	})

	t.Run("unregistered_file_fails_then_row_added_passes", func(t *testing.T) {
		// A reference file, not a */SKILL.md row: LoadManifestView (registry
		// check) ignores it either way, so this isolates the on-disk check.
		regPath, mfPath := mustWriteValidateFixture(t, "sdd-spec/SKILL.md managed\n")
		scan := stubScan([]string{"sdd-spec/SKILL.md", "orphan/notes.md"})

		var out, errBuf bytes.Buffer
		exitCode := 0
		RenderValidateCore(
			[]string{"--registry", regPath, "--manifest", mfPath, "--source-root", "unused"},
			os.ReadFile, scan, &out, &errBuf,
			func(c int) { exitCode = c },
		)
		if exitCode != 1 {
			t.Errorf("exit code = %d, want 1; stderr=%q", exitCode, errBuf.String())
		}
		if !strings.Contains(errBuf.String(), "UNREGISTERED_ON_DISK") {
			t.Errorf("stderr %q should contain UNREGISTERED_ON_DISK", errBuf.String())
		}

		// Add the deploying row: a rerun must now exit 0 (no exit(0) call is
		// made on success — see RenderValidateCore convention — so exitCode
		// must start at 0, matching the other success-path tests in this file).
		if err := os.WriteFile(mfPath, []byte("sdd-spec/SKILL.md managed\norphan/notes.md custom\n"), 0644); err != nil {
			t.Fatalf("rewrite manifest: %v", err)
		}
		out.Reset()
		errBuf.Reset()
		exitCode = 0
		RenderValidateCore(
			[]string{"--registry", regPath, "--manifest", mfPath, "--source-root", "unused"},
			os.ReadFile, scan, &out, &errBuf,
			func(c int) { exitCode = c },
		)
		if exitCode != 0 {
			t.Errorf("exit code = %d, want 0 after adding the row; stderr=%q", exitCode, errBuf.String())
		}
	})

	t.Run("orphan_row_fails_loud", func(t *testing.T) {
		regPath, mfPath := mustWriteValidateFixture(t, "sdd-spec/SKILL.md managed\nghost/SKILL.md custom\n")
		scan := stubScan([]string{"sdd-spec/SKILL.md"})

		var out, errBuf bytes.Buffer
		exitCode := -1
		RenderValidateCore(
			[]string{"--registry", regPath, "--manifest", mfPath, "--source-root", "unused"},
			os.ReadFile, scan, &out, &errBuf,
			func(c int) { exitCode = c },
		)
		if exitCode != 1 {
			t.Errorf("exit code = %d, want 1; stderr=%q", exitCode, errBuf.String())
		}
		if !strings.Contains(errBuf.String(), "MISSING_ON_DISK") {
			t.Errorf("stderr %q should contain MISSING_ON_DISK", errBuf.String())
		}
	})

	t.Run("mixed_divergences_all_reported_in_one_run", func(t *testing.T) {
		const regWithExtra = `version: "1"
skills:
  - id: sdd-spec
    path: sdd-spec
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
  - id: extra-skill
    path: extra-skill
    source:
      type: custom
    install:
      defaultScope: global
      targets:
        - claude
    lifecycle:
      updateStrategy: overlay-only
`
		dir := t.TempDir()
		regPath := filepath.Join(dir, "registry.yaml")
		mfPath := filepath.Join(dir, "overlay.manifest")
		if err := os.WriteFile(regPath, []byte(regWithExtra), 0644); err != nil {
			t.Fatalf("write registry: %v", err)
		}
		// sdd-spec is aligned; ghost is an orphan row (no disk file, no registry
		// entry); extra-skill has no manifest row; unregistered/SKILL.md has no
		// manifest row either. That is 3+ divergences across registry/manifest
		// and on-disk classes in a single run.
		if err := os.WriteFile(mfPath, []byte("sdd-spec/SKILL.md managed\nghost/SKILL.md custom\n"), 0644); err != nil {
			t.Fatalf("write manifest: %v", err)
		}
		scan := stubScan([]string{"sdd-spec/SKILL.md", "unregistered/SKILL.md"})

		var out, errBuf bytes.Buffer
		exitCode := -1
		RenderValidateCore(
			[]string{"--registry", regPath, "--manifest", mfPath, "--source-root", "unused"},
			os.ReadFile, scan, &out, &errBuf,
			func(c int) { exitCode = c },
		)
		if exitCode != 1 {
			t.Errorf("exit code = %d, want 1; stderr=%q", exitCode, errBuf.String())
		}
		for _, class := range []string{"MISSING_IN_MANIFEST", "MISSING_ON_DISK", "UNREGISTERED_ON_DISK"} {
			if !strings.Contains(errBuf.String(), class) {
				t.Errorf("stderr %q should contain %q", errBuf.String(), class)
			}
		}
		lines := strings.Count(strings.TrimRight(errBuf.String(), "\n"), "\n") + 1
		if lines < 3 {
			t.Errorf("stderr should report at least 3 distinct divergence lines, got %d: %q", lines, errBuf.String())
		}
	})

	t.Run("cwd_independence_with_absolute_source_root", func(t *testing.T) {
		regPath, mfPath := mustWriteValidateFixture(t, "sdd-spec/SKILL.md managed\n")
		absRegPath, err := filepath.Abs(regPath)
		if err != nil {
			t.Fatalf("abs registry path: %v", err)
		}
		absMfPath, err := filepath.Abs(mfPath)
		if err != nil {
			t.Fatalf("abs manifest path: %v", err)
		}
		scan := stubScan([]string{"sdd-spec/SKILL.md"})

		oldWD, err := os.Getwd()
		if err != nil {
			t.Fatalf("getwd: %v", err)
		}
		t.Cleanup(func() { _ = os.Chdir(oldWD) })

		// One of the three cwds is engine/ itself: a real 28-file Go package
		// that must never be treated as an implicit skills directory (R-002).
		engineDir := filepath.Join(oldWD, "..")
		cwds := []string{t.TempDir(), engineDir, t.TempDir()}
		for i, cwd := range cwds {
			if err := os.Chdir(cwd); err != nil {
				t.Fatalf("chdir %d to %s: %v", i, cwd, err)
			}
			var out, errBuf bytes.Buffer
			exitCode := 0
			RenderValidateCore(
				[]string{"--registry", absRegPath, "--manifest", absMfPath, "--source-root", "/does-not-matter-because-stubbed"},
				os.ReadFile, scan, &out, &errBuf,
				func(c int) { exitCode = c },
			)
			if exitCode != 0 {
				t.Errorf("cwd %d (%s): exit code = %d, want 0; stderr=%q", i, cwd, exitCode, errBuf.String())
			}
		}
	})

	t.Run("registry_divergence_survives_scan_failure", func(t *testing.T) {
		// R4-001: registry divergences are computed by Validate before the three
		// on-disk stages run. A later fatal error in any of those stages must not
		// discard already-computed registry divergences (R-007 full-scan
		// guarantee). Here ghost/SKILL.md has no registry entry, so Validate
		// reports MISSING_IN_REGISTRY; scanSkills is then stubbed to fail
		// outright. Both diagnostics must reach stderr in the same run.
		regPath, mfPath := mustWriteValidateFixture(t, "sdd-spec/SKILL.md managed\nghost/SKILL.md custom\n")
		failScan := func(string) ([]string, error) {
			return nil, errors.New("scan boom: source root unreadable")
		}

		var out, errBuf bytes.Buffer
		exitCode := 0
		RenderValidateCore(
			[]string{"--registry", regPath, "--manifest", mfPath, "--source-root", "unused"},
			os.ReadFile, failScan, &out, &errBuf,
			func(c int) { exitCode = c },
		)
		if exitCode != 1 {
			t.Errorf("exit code = %d, want 1; stderr=%q", exitCode, errBuf.String())
		}
		if !strings.Contains(errBuf.String(), "MISSING_IN_REGISTRY") {
			t.Errorf("stderr %q must contain the registry divergence despite the later scan failure", errBuf.String())
		}
		if !strings.Contains(errBuf.String(), "scan boom") {
			t.Errorf("stderr %q must contain the scan error", errBuf.String())
		}
	})
}

// ── T-08: CLI dispatch tests for add/remove ──────────────────────────────────

// TestSkillsCoreDispatchAddRemove verifies SC-36: SkillsCore routes "add" and
// "remove" verbs without emitting "unknown skills verb" to stderr.
func TestSkillsCoreDispatchAddRemove(t *testing.T) {
	dir := t.TempDir()
	regPath := filepath.Join(dir, "registry.yaml")
	mfPath := filepath.Join(dir, "overlay.manifest")
	skillsRoot := filepath.Join(dir, "skills")

	const regYAML = `version: "1"
skills:
  - id: existing
    path: existing
    source:
      type: custom
    install:
      defaultScope: global
      targets:
        - claude
    lifecycle:
      updateStrategy: overlay-only
`
	if err := os.WriteFile(regPath, []byte(regYAML), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mfPath, []byte("existing/SKILL.md custom\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(skillsRoot, "new-skill"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsRoot, "new-skill", "SKILL.md"), []byte("# new-skill\n"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("add_routes_without_unknown_verb_error", func(t *testing.T) {
		var out, errBuf bytes.Buffer
		exitCode := -1
		SkillsCore("add",
			[]string{"--registry", regPath, "--manifest", mfPath, "--source-root", skillsRoot, "new-skill"},
			os.ReadFile, &out, &errBuf,
			func(c int) { exitCode = c },
		)
		if strings.Contains(errBuf.String(), "unknown skills verb") {
			t.Errorf("stderr %q should not contain 'unknown skills verb'", errBuf.String())
		}
		if exitCode != 0 {
			t.Errorf("exit code = %d, want 0; stderr=%q", exitCode, errBuf.String())
		}
	})

	t.Run("remove_routes_without_unknown_verb_error", func(t *testing.T) {
		// Registry now has "existing" + "new-skill" (added above); remove "new-skill".
		var out, errBuf bytes.Buffer
		exitCode := -1
		SkillsCore("remove",
			[]string{"--registry", regPath, "--manifest", mfPath, "--source-root", skillsRoot, "new-skill"},
			os.ReadFile, &out, &errBuf,
			func(c int) { exitCode = c },
		)
		if strings.Contains(errBuf.String(), "unknown skills verb") {
			t.Errorf("stderr %q should not contain 'unknown skills verb'", errBuf.String())
		}
		if exitCode != 0 {
			t.Errorf("exit code = %d, want 0; stderr=%q", exitCode, errBuf.String())
		}
	})
}

// TestSkillsCoreUnknownVerbMessage verifies SC-37: an unknown verb exits 1 and
// the error message lists both "add" and "remove" as supported verbs.
func TestSkillsCoreUnknownVerbMessage(t *testing.T) {
	var out, errBuf bytes.Buffer
	exitCode := 0
	SkillsCore("bogus", nil, skillsMockReadFile, &out, &errBuf, func(c int) { exitCode = c })
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

// ── T-05: sync-manifest dispatch tests (SC-51, SC-52) ────────────────────────

// TestSkillsCoreDispatchSyncManifest verifies that SkillsCore routes
// "sync-manifest" to SyncCore (SC-51) and that unknown verbs list
// "sync-manifest" in the error message (SC-52).
func TestSkillsCoreDispatchSyncManifest(t *testing.T) {
	const syncDispatchReg = `version: "1"
skills:
  - id: sc51-alpha
    path: sc51-alpha
    source:
      type: custom
    install:
      defaultScope: global
      targets:
        - claude
    lifecycle:
      updateStrategy: overlay-only
`
	t.Run("SC-51_routes_to_SyncCore", func(t *testing.T) {
		// Set up a t.TempDir with an aligned registry + manifest so SyncCore exits 0.
		dir := t.TempDir()
		regPath := filepath.Join(dir, "registry.yaml")
		mfPath := filepath.Join(dir, "overlay.manifest")

		if err := os.WriteFile(regPath, []byte(syncDispatchReg), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(mfPath, []byte("sc51-alpha/SKILL.md custom\n"), 0644); err != nil {
			t.Fatal(err)
		}

		var out, errBuf bytes.Buffer
		exitCode := -1
		SkillsCore(
			"sync-manifest",
			[]string{"--registry", regPath, "--manifest", mfPath},
			os.ReadFile, &out, &errBuf,
			func(c int) { exitCode = c },
		)

		if strings.Contains(errBuf.String(), "unknown skills verb") {
			t.Errorf("stderr %q should not contain 'unknown skills verb'", errBuf.String())
		}
		if exitCode != 0 {
			t.Errorf("exit code = %d, want 0; stderr=%q", exitCode, errBuf.String())
		}
		// stdout should contain the summary or "already in sync"
		outStr := out.String()
		if !strings.Contains(outStr, "sync-manifest") && !strings.Contains(outStr, "already in sync") {
			t.Errorf("stdout %q should contain sync-manifest summary or 'already in sync'", outStr)
		}
	})

	t.Run("SC-52_unknown_verb_lists_sync_manifest", func(t *testing.T) {
		var out, errBuf bytes.Buffer
		exitCode := 0
		SkillsCore("bogus-after-sync", nil, skillsMockReadFile, &out, &errBuf, func(c int) { exitCode = c })
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
		SkillsCore("", nil, skillsMockReadFile, &out, &errBuf, func(c int) { exitCode = c })
		if exitCode != 1 {
			t.Errorf("exit code = %d, want 1", exitCode)
		}
		if !strings.Contains(errBuf.String(), "sync-manifest") {
			t.Errorf("stderr %q should contain 'sync-manifest' in supported verb list", errBuf.String())
		}
	})
}
