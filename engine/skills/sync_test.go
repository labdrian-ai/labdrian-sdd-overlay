package skills

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// mkEntry builds a minimal valid Entry for use in sync tests.
func mkEntry(id, path, sourceType string) Entry {
	e := Entry{
		ID:   id,
		Path: path,
		Source: Source{
			Type: sourceType,
		},
		Install: Install{
			DefaultScope: "global",
			Targets:      []string{"claude"},
		},
		Lifecycle: Lifecycle{
			UpdateStrategy: "overlay-only",
		},
	}
	if sourceType == "core" {
		e.Source.Upstream = &Upstream{Owner: "gentle-ai"}
		e.Lifecycle.UpdateStrategy = "vendor-merge"
	}
	return e
}

// mkReg builds a Registry from the given entries.
func mkReg(entries ...Entry) Registry {
	return Registry{Version: "1", Skills: entries}
}

// ── T-01: TestSyncManifest (pure function, table-driven) ─────────────────────

func TestSyncManifest(t *testing.T) {
	alpha := mkEntry("alpha", "alpha", "custom")
	beta := mkEntry("beta", "beta", "core")

	tests := []struct {
		name        string
		reg         Registry
		manifest    []byte
		wantOut     []byte
		wantAdded   []string
		wantDropped []string
		wantRetagged []string
		wantErr     bool
	}{
		// SC-40: aligned manifest — byte-identical output, empty ChangeReport
		{
			name:     "SC-40_aligned_no_op",
			reg:      mkReg(alpha, beta),
			manifest: []byte("alpha/SKILL.md custom\nbeta/SKILL.md managed\n"),
			wantOut:  []byte("alpha/SKILL.md custom\nbeta/SKILL.md managed\n"),
		},
		// SC-41: TAG_MISMATCH — wrong tag corrected, Retagged event
		{
			name:         "SC-41_tag_mismatch",
			reg:          mkReg(alpha),
			manifest:     []byte("alpha/SKILL.md managed\n"),
			wantOut:      []byte("alpha/SKILL.md custom\n"),
			wantRetagged: []string{"alpha"},
		},
		// SC-42: MIXED_TAG — two rows for same dir, collapsed to one correct row
		{
			name:         "SC-42_mixed_tag",
			reg:          mkReg(beta),
			manifest:     []byte("beta/SKILL.md managed\nbeta/SKILL.md custom\n"),
			wantOut:      []byte("beta/SKILL.md managed\n"),
			wantRetagged: []string{"beta"},
		},
		// SC-43: orphan drop — row not in registry is removed
		{
			name:        "SC-43_orphan_drop",
			reg:         mkReg(alpha),
			manifest:    []byte("alpha/SKILL.md custom\norphan/SKILL.md custom\n"),
			wantOut:     []byte("alpha/SKILL.md custom\n"),
			wantDropped: []string{"orphan"},
		},
		// SC-44: missing entry — registry entry absent from manifest is added at anchor
		{
			name:      "SC-44_missing_entry",
			reg:       mkReg(alpha, beta),
			manifest:  []byte("alpha/SKILL.md custom\n"),
			wantOut:   []byte("alpha/SKILL.md custom\nbeta/SKILL.md managed\n"),
			wantAdded: []string{"beta"},
		},
		// SC-45: non-skill rows preserved verbatim (engine/*, _shared/*, registry row)
		{
			name: "SC-45_non_skill_rows_verbatim",
			reg:  mkReg(alpha),
			manifest: []byte(
				"engine/install.sh managed\n" +
					"alpha/SKILL.md custom\n" +
					"_shared/common.sh managed\n" +
					"skills.registry.yaml custom\n",
			),
			wantOut: []byte(
				"engine/install.sh managed\n" +
					"alpha/SKILL.md custom\n" +
					"_shared/common.sh managed\n" +
					"skills.registry.yaml custom\n",
			),
		},
		// SC-46: interspersed non-skill rows — anchor model; non-skill rows after first skill
		// move to after the skill block
		{
			name: "SC-46_interspersed_non_skill",
			reg:  mkReg(alpha, beta),
			manifest: []byte(
				"engine/a.sh managed\n" +
					"alpha/SKILL.md custom\n" +
					"engine/b.sh managed\n" +
					"beta/SKILL.md custom\n" +
					"skills.registry.yaml custom\n",
			),
			wantOut: []byte(
				"engine/a.sh managed\n" +
					"alpha/SKILL.md custom\n" +
					"beta/SKILL.md managed\n" +
					"engine/b.sh managed\n" +
					"skills.registry.yaml custom\n",
			),
			wantRetagged: []string{"beta"},
		},
		// SC-47: no existing skill rows — skill block appended at EOF
		{
			name: "SC-47_no_anchor_append_at_eof",
			reg:  mkReg(alpha),
			manifest: []byte(
				"engine/install.sh managed\n" +
					"skills.registry.yaml custom\n",
			),
			wantOut: []byte(
				"engine/install.sh managed\n" +
					"skills.registry.yaml custom\n" +
					"alpha/SKILL.md custom\n",
			),
			wantAdded: []string{"alpha"},
		},
		// SC-XX: non-SKILL.md agent row (e.g. GADU.md custom agent) is preserved
		// verbatim. isSkillRow must return false for a row whose path does not end
		// with /SKILL.md, so the row is treated as a non-skill line and kept intact.
		{
			name: "non_skill_md_agent_row_preserved",
			reg:  mkReg(alpha),
			manifest: []byte(
				"GADU.md   custom   agent\n" +
					"alpha/SKILL.md custom\n",
			),
			wantOut: []byte(
				"GADU.md   custom   agent\n" +
					"alpha/SKILL.md custom\n",
			),
		},
		// SC-XX+1: GADU.md agent row AFTER the last skill row — the real overlay.manifest
		// layout. SyncManifest must place GADU.md in preservedAfter and emit it verbatim
		// after the skill block (the anchor-replacement "preservedAfter" path).
		{
			name: "non_skill_md_agent_row_after_skill_preserved",
			reg:  mkReg(alpha),
			manifest: []byte(
				"alpha/SKILL.md custom\n" +
					"GADU.md   custom   agent\n",
			),
			wantOut: []byte(
				"alpha/SKILL.md custom\n" +
					"GADU.md   custom   agent\n",
			),
		},
		// SC-48: post-condition invariant — Diff must be empty for every output
		// (tested inline via the post-condition self-check inside SyncManifest)
		// The table entries above all implicitly cover this; here we test an
		// explicit drifted case to confirm Diff is empty on the returned bytes.
		{
			name: "SC-48_post_condition_invariant",
			reg:  mkReg(alpha, beta),
			manifest: []byte(
				"alpha/SKILL.md managed\n" + // wrong tag
					"orphan/SKILL.md custom\n", // not in registry
			),
			wantOut: []byte("alpha/SKILL.md custom\nbeta/SKILL.md managed\n"),
			wantRetagged: []string{"alpha"},
			wantDropped:  []string{"orphan"},
			wantAdded:    []string{"beta"},
		},
		// SC-53 idempotency (base inputs — full loop tested in TestSyncManifestIdempotency)
		{
			name:     "SC-53_idempotency_aligned",
			reg:      mkReg(alpha, beta),
			manifest: []byte("alpha/SKILL.md custom\nbeta/SKILL.md managed\n"),
			wantOut:  []byte("alpha/SKILL.md custom\nbeta/SKILL.md managed\n"),
		},
		// SC-54: tag mapping — core→managed, custom→custom
		{
			name:         "SC-54_tag_core_to_managed",
			reg:          mkReg(beta), // core
			manifest:     []byte("beta/SKILL.md custom\n"), // wrong tag
			wantOut:      []byte("beta/SKILL.md managed\n"),
			wantRetagged: []string{"beta"},
		},
		{
			name:         "SC-54_tag_custom_to_custom",
			reg:          mkReg(alpha), // custom
			manifest:     []byte("alpha/SKILL.md managed\n"), // wrong tag
			wantOut:      []byte("alpha/SKILL.md custom\n"),
			wantRetagged: []string{"alpha"},
		},
		// Trailing newline preservation
		{
			name:     "trailing_newline_no_trailing_in_input",
			reg:      mkReg(alpha),
			manifest: []byte("alpha/SKILL.md custom"), // no trailing newline
			wantOut:  []byte("alpha/SKILL.md custom\n"),
		},
		// Empty manifest (no lines at all)
		{
			name:      "empty_manifest",
			reg:       mkReg(alpha),
			manifest:  []byte(""),
			wantOut:   []byte("alpha/SKILL.md custom\n"),
			wantAdded: []string{"alpha"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, report, err := SyncManifest(tt.reg, tt.manifest)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Output bytes must match.
			if !bytes.Equal(out, tt.wantOut) {
				t.Errorf("output mismatch:\n  got  %q\n  want %q", out, tt.wantOut)
			}

			// Output must end with exactly one newline (when non-empty).
			if len(out) > 0 && out[len(out)-1] != '\n' {
				t.Errorf("output does not end with newline: %q", out)
			}

			// ChangeReport slices (nil and empty are treated as equivalent here).
			assertStringSlice(t, "Added", report.Added, tt.wantAdded)
			assertStringSlice(t, "Dropped", report.Dropped, tt.wantDropped)
			assertStringSlice(t, "Retagged", report.Retagged, tt.wantRetagged)

			// SC-48 post-condition: Diff(reg, view(out)) must be empty.
			mv, mvErr := loadManifestViewReader(bytes.NewReader(out))
			if mvErr != nil {
				t.Fatalf("loadManifestViewReader on output: %v", mvErr)
			}
			if divs := Diff(tt.reg, mv); len(divs) > 0 {
				t.Errorf("post-condition Diff not empty: %v", divs)
			}
		})
	}
}

// TestSyncManifestIdempotency verifies SC-53: a second call with the same
// registry and the first call's output returns byte-identical bytes and
// an empty ChangeReport.
func TestSyncManifestIdempotency(t *testing.T) {
	alpha := mkEntry("alpha", "alpha", "custom")
	beta := mkEntry("beta", "beta", "core")

	inputs := []struct {
		name     string
		reg      Registry
		manifest []byte
	}{
		{
			name:     "aligned",
			reg:      mkReg(alpha, beta),
			manifest: []byte("alpha/SKILL.md custom\nbeta/SKILL.md managed\n"),
		},
		{
			name:     "tag_mismatch",
			reg:      mkReg(alpha),
			manifest: []byte("alpha/SKILL.md managed\n"),
		},
		{
			name:     "orphan",
			reg:      mkReg(alpha),
			manifest: []byte("alpha/SKILL.md custom\norphan/SKILL.md custom\n"),
		},
		{
			name:     "missing_entry",
			reg:      mkReg(alpha, beta),
			manifest: []byte("alpha/SKILL.md custom\n"),
		},
		{
			name:     "no_anchor",
			reg:      mkReg(alpha),
			manifest: []byte("engine/x.sh managed\n"),
		},
	}

	for _, tt := range inputs {
		t.Run(tt.name, func(t *testing.T) {
			out1, _, err := SyncManifest(tt.reg, tt.manifest)
			if err != nil {
				t.Fatalf("first SyncManifest: %v", err)
			}
			out2, report2, err := SyncManifest(tt.reg, out1)
			if err != nil {
				t.Fatalf("second SyncManifest: %v", err)
			}
			if !bytes.Equal(out1, out2) {
				t.Errorf("idempotency violation:\n  out1=%q\n  out2=%q", out1, out2)
			}
			if len(report2.Added)+len(report2.Dropped)+len(report2.Retagged) != 0 {
				t.Errorf("second sync produced non-empty ChangeReport: %+v", report2)
			}
		})
	}
}

// assertStringSlice checks that got and want contain the same set of strings
// (order-independent, treating nil and empty as equivalent).
func assertStringSlice(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if len(got) != len(want) {
		t.Errorf("%s: got %v (len=%d), want %v (len=%d)", label, got, len(got), want, len(want))
		return
	}
	// Build set from want and check each got element.
	wantSet := make(map[string]int, len(want))
	for _, s := range want {
		wantSet[s]++
	}
	for _, s := range got {
		wantSet[s]--
	}
	for k, v := range wantSet {
		if v != 0 {
			t.Errorf("%s: element %q count mismatch (diff=%d); got=%v want=%v", label, k, v, got, want)
		}
	}
}

// TestIsSkillRow_NonSkillMdAgentRow pins that isSkillRow returns false for a
// row whose path does not end with /SKILL.md (e.g. the GADU agent row
// "GADU.md   custom   agent"). Such rows must be preserved verbatim by
// SyncManifest and never treated as skill directories.
func TestIsSkillRow_NonSkillMdAgentRow(t *testing.T) {
	cases := []struct {
		line    string
		wantOk  bool
		wantDir string
	}{
		// Standard skill rows (must be classified as skill rows).
		{"alpha/SKILL.md custom", true, "alpha"},
		{"beta/SKILL.md managed", true, "beta"},
		// Non-SKILL.md agent row: three-column GADU.md entry.
		{"GADU.md   custom   agent", false, ""},
		// Root-level file row without /SKILL.md suffix.
		{"skills.registry.yaml custom", false, ""},
		// Infra row (engine/ prefix, filtered by isInfraDir).
		{"engine/go.mod managed", false, ""},
	}

	for _, tc := range cases {
		dir, ok := isSkillRow(tc.line)
		if ok != tc.wantOk {
			t.Errorf("isSkillRow(%q): ok=%v, want %v", tc.line, ok, tc.wantOk)
		}
		if dir != tc.wantDir {
			t.Errorf("isSkillRow(%q): dir=%q, want %q", tc.line, dir, tc.wantDir)
		}
	}
}

// TestSyncManifestExternalEntry verifies SC-64: SyncManifest with an external
// entry must emit "<path>/SKILL.md custom" and report Added (ADR-13, R-125).
func TestSyncManifestExternalEntry(t *testing.T) {
	ext := Entry{
		ID:        "my-ext-skill",
		Path:      "my-ext-skill",
		Source:    Source{Type: "external", Repo: "https://github.com/example/skills", Ref: "a1b2c3d"},
		Install:   Install{DefaultScope: "global", Targets: []string{"claude"}},
		Lifecycle: Lifecycle{UpdateStrategy: "overlay-only"},
	}
	reg := mkReg(ext)
	out, report, err := SyncManifest(reg, []byte(""))
	if err != nil {
		t.Fatalf("SyncManifest with external entry: unexpected error: %v", err)
	}
	outStr := string(out)
	if !strings.Contains(outStr, "my-ext-skill/SKILL.md custom") {
		t.Errorf("output should contain %q; got %q", "my-ext-skill/SKILL.md custom", outStr)
	}
	if strings.Contains(outStr, "my-ext-skill/SKILL.md managed") {
		t.Errorf("output must NOT contain managed tag for external; got %q", outStr)
	}
	if len(report.Added) != 1 || report.Added[0] != "my-ext-skill" {
		t.Errorf("report.Added = %v, want [my-ext-skill]", report.Added)
	}
}

// ── T-03: TestSyncCore (I/O shell) ───────────────────────────────────────────

// syncCoreTestReg is a two-entry registry fixture for SyncCore tests.
const syncCoreTestReg = `version: "1"
skills:
  - id: alpha
    path: alpha
    source:
      type: custom
    install:
      defaultScope: global
      targets:
        - claude
    lifecycle:
      updateStrategy: overlay-only
  - id: beta
    path: beta
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

func TestSyncCore(t *testing.T) {
	// drifted: alpha present, beta missing
	const driftedManifest = "alpha/SKILL.md custom\n"
	// aligned: alpha + beta with correct tags
	const alignedManifest = "alpha/SKILL.md custom\nbeta/SKILL.md managed\n"

	t.Run("drifted_manifest_validate_passes_after_sync", func(t *testing.T) {
		dir := t.TempDir()
		regPath := filepath.Join(dir, "registry.yaml")
		mfPath := filepath.Join(dir, "overlay.manifest")

		if err := os.WriteFile(regPath, []byte(syncCoreTestReg), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(mfPath, []byte(driftedManifest), 0644); err != nil {
			t.Fatal(err)
		}

		var stdout, stderr bytes.Buffer
		exitCode := -1
		SyncCore(
			[]string{"--registry", regPath, "--manifest", mfPath},
			os.ReadFile, &stdout, &stderr,
			func(c int) { exitCode = c },
		)
		if exitCode != 0 {
			t.Fatalf("exit code %d; stderr=%q", exitCode, stderr.String())
		}

		regData, err := os.ReadFile(regPath)
		if err != nil {
			t.Fatal(err)
		}
		reg, err := ParseRegistry(bytes.NewReader(regData))
		if err != nil {
			t.Fatal(err)
		}
		divs, vErr := Validate(reg, mfPath)
		if vErr != nil || len(divs) > 0 {
			t.Errorf("validate failed after sync: err=%v divs=%v", vErr, divs)
		}
	})

	t.Run("atomic_write_failure_leaves_manifest_unchanged", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping filesystem chmod test in short mode")
		}
		dir := t.TempDir()
		regPath := filepath.Join(dir, "registry.yaml")

		// Use a sub-directory for the manifest so we can chmod it separately.
		manDir := filepath.Join(dir, "man")
		if err := os.MkdirAll(manDir, 0755); err != nil {
			t.Fatal(err)
		}
		mfPath := filepath.Join(manDir, "overlay.manifest")

		const origManifest = "alpha/SKILL.md custom\n" // beta missing → sync would write

		if err := os.WriteFile(regPath, []byte(syncCoreTestReg), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(mfPath, []byte(origManifest), 0644); err != nil {
			t.Fatal(err)
		}

		// Make the manifest directory read-only: writeFileAtomic's CreateTemp will fail.
		if err := os.Chmod(manDir, 0555); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Chmod(manDir, 0755) }() // restore for t.TempDir cleanup

		var stdout, stderr bytes.Buffer
		exitCode := -1
		SyncCore(
			[]string{"--registry", regPath, "--manifest", mfPath},
			os.ReadFile, &stdout, &stderr,
			func(c int) { exitCode = c },
		)

		if exitCode != 1 {
			t.Errorf("expected exit 1, got %d", exitCode)
		}
		if stderr.Len() == 0 {
			t.Error("expected non-empty stderr on write failure")
		}

		// Restore write perms to read the manifest.
		_ = os.Chmod(manDir, 0755)
		got, err := os.ReadFile(mfPath)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, []byte(origManifest)) {
			t.Errorf("manifest changed on write failure; got %q, want %q", got, origManifest)
		}
	})

	t.Run("report_printed_on_drifted_manifest", func(t *testing.T) {
		dir := t.TempDir()
		regPath := filepath.Join(dir, "registry.yaml")
		mfPath := filepath.Join(dir, "overlay.manifest")

		if err := os.WriteFile(regPath, []byte(syncCoreTestReg), 0644); err != nil {
			t.Fatal(err)
		}
		// beta missing → 1 added
		if err := os.WriteFile(mfPath, []byte(driftedManifest), 0644); err != nil {
			t.Fatal(err)
		}

		var stdout, stderr bytes.Buffer
		exitCode := -1
		SyncCore(
			[]string{"--registry", regPath, "--manifest", mfPath},
			os.ReadFile, &stdout, &stderr,
			func(c int) { exitCode = c },
		)
		if exitCode != 0 {
			t.Fatalf("exit code %d; stderr=%q", exitCode, stderr.String())
		}
		outStr := stdout.String()
		if !strings.Contains(outStr, "added") {
			t.Errorf("stdout %q should contain 'added'", outStr)
		}
		if !strings.Contains(outStr, "dropped") {
			t.Errorf("stdout %q should contain 'dropped'", outStr)
		}
		if !strings.Contains(outStr, "retagged") {
			t.Errorf("stdout %q should contain 'retagged'", outStr)
		}
	})

	t.Run("already_in_sync_no_write", func(t *testing.T) {
		dir := t.TempDir()
		regPath := filepath.Join(dir, "registry.yaml")
		mfPath := filepath.Join(dir, "overlay.manifest")

		if err := os.WriteFile(regPath, []byte(syncCoreTestReg), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(mfPath, []byte(alignedManifest), 0644); err != nil {
			t.Fatal(err)
		}

		origBytes, err := os.ReadFile(mfPath)
		if err != nil {
			t.Fatal(err)
		}

		var stdout, stderr bytes.Buffer
		exitCode := -1
		SyncCore(
			[]string{"--registry", regPath, "--manifest", mfPath},
			os.ReadFile, &stdout, &stderr,
			func(c int) { exitCode = c },
		)
		if exitCode != 0 {
			t.Errorf("exit code %d; stderr=%q", exitCode, stderr.String())
		}
		if !strings.Contains(stdout.String(), "already in sync") {
			t.Errorf("stdout %q should contain 'already in sync'", stdout.String())
		}

		// Manifest bytes must be unchanged (no write occurred).
		afterBytes, err := os.ReadFile(mfPath)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(origBytes, afterBytes) {
			t.Errorf("manifest was written when already in sync")
		}
	})

	t.Run("flag_parsing_custom_paths", func(t *testing.T) {
		dir := t.TempDir()
		// Use non-default filenames to verify flag parsing.
		regPath := filepath.Join(dir, "custom.registry.yaml")
		mfPath := filepath.Join(dir, "custom.manifest")

		if err := os.WriteFile(regPath, []byte(syncCoreTestReg), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(mfPath, []byte(alignedManifest), 0644); err != nil {
			t.Fatal(err)
		}

		var stdout, stderr bytes.Buffer
		exitCode := -1
		SyncCore(
			[]string{"--registry", regPath, "--manifest", mfPath},
			os.ReadFile, &stdout, &stderr,
			func(c int) { exitCode = c },
		)
		if exitCode != 0 {
			t.Errorf("exit code %d; stderr=%q", exitCode, stderr.String())
		}
	})

	t.Run("registry_read_error_exit_1", func(t *testing.T) {
		failRead := func(_ string) ([]byte, error) {
			return nil, os.ErrNotExist
		}
		var stdout, stderr bytes.Buffer
		exitCode := -1
		SyncCore(
			[]string{"--registry", "missing.yaml", "--manifest", "overlay.manifest"},
			failRead, &stdout, &stderr,
			func(c int) { exitCode = c },
		)
		if exitCode != 1 {
			t.Errorf("expected exit 1 on registry read error, got %d", exitCode)
		}
		if stderr.Len() == 0 {
			t.Error("expected non-empty stderr on registry read error")
		}
	})

	t.Run("manifest_read_error_exit_1", func(t *testing.T) {
		callCount := 0
		selectiveRead := func(path string) ([]byte, error) {
			callCount++
			if callCount == 1 {
				return []byte(syncCoreTestReg), nil // registry reads fine
			}
			return nil, os.ErrNotExist // manifest fails
		}
		var stdout, stderr bytes.Buffer
		exitCode := -1
		SyncCore(
			[]string{"--registry", "reg.yaml", "--manifest", "missing.manifest"},
			selectiveRead, &stdout, &stderr,
			func(c int) { exitCode = c },
		)
		if exitCode != 1 {
			t.Errorf("expected exit 1 on manifest read error, got %d", exitCode)
		}
		if stderr.Len() == 0 {
			t.Error("expected non-empty stderr on manifest read error")
		}
	})
}
