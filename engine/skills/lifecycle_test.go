package skills

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// ── T-04: Pure-transform tests (AddEntry / RemoveEntry) ──────────────────────

// TestAddEntryProvenance verifies T-06 pure-function paths for AddEntry (ADR-14).
func TestAddEntryProvenance(t *testing.T) {
	reg := Registry{Version: "1"}

	t.Run("no_repo_produces_custom", func(t *testing.T) {
		// SC-67 (pure): repo=="" → Source.Type=="custom", Repo=="", ref ignored.
		got, err := AddEntry(reg, "baz", "", "")
		if err != nil {
			t.Fatalf("AddEntry: %v", err)
		}
		e := got.Skills[0]
		if e.Source.Type != "custom" {
			t.Errorf("Source.Type = %q, want custom", e.Source.Type)
		}
		if e.Source.Repo != "" {
			t.Errorf("Source.Repo = %q, want empty", e.Source.Repo)
		}
	})

	t.Run("repo_produces_external", func(t *testing.T) {
		// SC-65/SC-66 (pure): repo set → Source.Type=="external", Repo set.
		got, err := AddEntry(reg, "foo", "https://github.com/example/skills", "")
		if err != nil {
			t.Fatalf("AddEntry: %v", err)
		}
		e := got.Skills[0]
		if e.Source.Type != "external" {
			t.Errorf("Source.Type = %q, want external", e.Source.Type)
		}
		if e.Source.Repo != "https://github.com/example/skills" {
			t.Errorf("Source.Repo = %q, want https://github.com/example/skills", e.Source.Repo)
		}
		if e.Source.Ref != "" {
			t.Errorf("Source.Ref = %q, want empty", e.Source.Ref)
		}
	})

	t.Run("repo_and_ref_produces_external_with_ref", func(t *testing.T) {
		// SC-66 (pure): repo+ref → Source.Ref set.
		got, err := AddEntry(reg, "bar", "https://example.com/repo", "deadbeef")
		if err != nil {
			t.Fatalf("AddEntry: %v", err)
		}
		e := got.Skills[0]
		if e.Source.Type != "external" {
			t.Errorf("Source.Type = %q, want external", e.Source.Type)
		}
		if e.Source.Ref != "deadbeef" {
			t.Errorf("Source.Ref = %q, want deadbeef", e.Source.Ref)
		}
	})
}

// TestAddEntryDefaults verifies R-062: AddEntry on a valid id and empty registry
// returns an entry populated with all required default values.
func TestAddEntryDefaults(t *testing.T) {
	reg := Registry{Version: "1"}

	got, err := AddEntry(reg, "my-skill", "", "")
	if err != nil {
		t.Fatalf("AddEntry: unexpected error: %v", err)
	}
	if len(got.Skills) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got.Skills))
	}

	e := got.Skills[0]
	if e.ID != "my-skill" {
		t.Errorf("ID: got %q, want %q", e.ID, "my-skill")
	}
	if e.Path != "my-skill" {
		t.Errorf("Path: got %q, want %q", e.Path, "my-skill")
	}
	if e.Source.Type != "custom" {
		t.Errorf("Source.Type: got %q, want %q", e.Source.Type, "custom")
	}
	if e.Source.Upstream != nil {
		t.Errorf("Source.Upstream: expected nil, got %+v", e.Source.Upstream)
	}
	if e.Install.DefaultScope != "global" {
		t.Errorf("Install.DefaultScope: got %q, want %q", e.Install.DefaultScope, "global")
	}
	wantTargets := []string{"claude", "opencode", "codex"}
	if !reflect.DeepEqual(e.Install.Targets, wantTargets) {
		t.Errorf("Install.Targets: got %v, want %v", e.Install.Targets, wantTargets)
	}
	if e.Lifecycle.UpdateStrategy != "overlay-only" {
		t.Errorf("Lifecycle.UpdateStrategy: got %q, want %q", e.Lifecycle.UpdateStrategy, "overlay-only")
	}
}

// TestAddEntryNilAllowedProjects verifies ADR-8: AllowedProjects MUST be nil (not
// []string{}) so the round-trip through Serialize → ParseRegistry produces DeepEqual.
func TestAddEntryNilAllowedProjects(t *testing.T) {
	reg := Registry{Version: "1"}

	got, err := AddEntry(reg, "skill1", "", "")
	if err != nil {
		t.Fatalf("AddEntry: %v", err)
	}
	if got.Skills[0].Install.AllowedProjects != nil {
		t.Errorf("AllowedProjects: expected nil, got %v", got.Skills[0].Install.AllowedProjects)
	}

	// Round-trip through serialize → parse must preserve DeepEqual (ADR-8, ADR-7).
	out, err := Serialize(got)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	reparsed, err := ParseRegistry(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("ParseRegistry: %v", err)
	}
	if !reflect.DeepEqual(got, reparsed) {
		t.Errorf("round-trip not equal:\n  before: %+v\n  after:  %+v", got, reparsed)
	}
}

// TestAddEntrySlugGuard verifies ADR-8: invalid ids are rejected before touching the registry.
func TestAddEntrySlugGuard(t *testing.T) {
	// Note: a digit-starting id like "0abc" is VALID per ^[a-z0-9][a-z0-9-]*$,
	// so it lives in `valid` below, not here.
	invalidStrict := []string{
		"",
		"../evil",
		"foo bar",
		"FOO",
		"Skill",
		"-leading-dash",
		"has/slash",
		"has.dot",
		"..",
	}
	valid := []string{"my-skill", "skill1", "a", "abc-def", "0abc"}

	reg := Registry{Version: "1"}

	for _, id := range invalidStrict {
		_, err := AddEntry(reg, id, "", "")
		if err == nil {
			t.Errorf("AddEntry(%q): expected error, got nil", id)
		}
	}
	for _, id := range valid {
		_, err := AddEntry(reg, id, "", "")
		if err != nil {
			t.Errorf("AddEntry(%q): unexpected error: %v", id, err)
		}
		// Reset for next iteration (avoid duplicate errors).
		reg = Registry{Version: "1"}
	}
}

// TestAddEntryDuplicate verifies R-061: AddEntry with an id already present returns a non-nil error.
func TestAddEntryDuplicate(t *testing.T) {
	reg := Registry{
		Version: "1",
		Skills: []Entry{
			{
				ID:   "existing",
				Path: "existing",
				Source: Source{Type: "custom"},
				Install: Install{
					DefaultScope: "global",
					Targets:      []string{"claude"},
				},
				Lifecycle: Lifecycle{UpdateStrategy: "overlay-only"},
			},
		},
	}

	_, err := AddEntry(reg, "existing", "", "")
	if err == nil {
		t.Error("AddEntry with duplicate id: expected error, got nil")
	}
}

// TestRemoveEntrySuccess verifies R-070: RemoveEntry removes the correct entry and
// preserves the relative order of the remaining entries.
func TestRemoveEntrySuccess(t *testing.T) {
	makeEntry := func(id string) Entry {
		return Entry{
			ID:   id,
			Path: id,
			Source: Source{Type: "custom"},
			Install: Install{
				DefaultScope: "global",
				Targets:      []string{"claude"},
			},
			Lifecycle: Lifecycle{UpdateStrategy: "overlay-only"},
		}
	}

	reg := Registry{
		Version: "1",
		Skills:  []Entry{makeEntry("alpha"), makeEntry("beta"), makeEntry("gamma")},
	}

	got, err := RemoveEntry(reg, "beta")
	if err != nil {
		t.Fatalf("RemoveEntry: unexpected error: %v", err)
	}
	if len(got.Skills) != 2 {
		t.Fatalf("expected 2 entries after removal, got %d", len(got.Skills))
	}
	if got.Skills[0].ID != "alpha" || got.Skills[1].ID != "gamma" {
		t.Errorf("order not preserved: got [%s, %s], want [alpha, gamma]",
			got.Skills[0].ID, got.Skills[1].ID)
	}
}

// TestRemoveEntryAbsent verifies R-069: RemoveEntry returns a non-nil error when id is absent.
func TestRemoveEntryAbsent(t *testing.T) {
	reg := Registry{Version: "1"}

	_, err := RemoveEntry(reg, "nonexistent")
	if err == nil {
		t.Error("RemoveEntry with absent id: expected error, got nil")
	}
}

// TestAddEntryOrderPreservation verifies that adding multiple entries to an existing registry
// appends in the correct order and does not mutate the input slice.
func TestAddEntryOrderPreservation(t *testing.T) {
	reg := Registry{Version: "1"}

	reg1, err := AddEntry(reg, "first", "", "")
	if err != nil {
		t.Fatalf("AddEntry first: %v", err)
	}
	reg2, err := AddEntry(reg1, "second", "", "")
	if err != nil {
		t.Fatalf("AddEntry second: %v", err)
	}

	if len(reg2.Skills) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(reg2.Skills))
	}
	if reg2.Skills[0].ID != "first" || reg2.Skills[1].ID != "second" {
		t.Errorf("order wrong: [%s, %s]", reg2.Skills[0].ID, reg2.Skills[1].ID)
	}

	// Input registry must not be mutated.
	if len(reg.Skills) != 0 {
		t.Errorf("original registry was mutated: got %d entries", len(reg.Skills))
	}
	if len(reg1.Skills) != 1 {
		t.Errorf("reg1 was mutated: got %d entries", len(reg1.Skills))
	}
}

// TestAddEntryRoundTrip verifies that serialize(AddEntry(...)) re-parses to a DeepEqual value.
func TestAddEntryRoundTrip(t *testing.T) {
	base := Registry{
		Version: "1",
		Skills: []Entry{
			{
				ID:   "base",
				Path: "base",
				Source: Source{
					Type: "core",
					Upstream: &Upstream{Owner: "gentleman-programming"},
				},
				Install: Install{
					DefaultScope: "global",
					Targets:      []string{"claude", "opencode"},
				},
				Lifecycle: Lifecycle{UpdateStrategy: "vendor-merge"},
			},
		},
	}

	got, err := AddEntry(base, "new-skill", "", "")
	if err != nil {
		t.Fatalf("AddEntry: %v", err)
	}

	out, err := Serialize(got)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}

	reparsed, err := ParseRegistry(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("ParseRegistry after serialize: %v", err)
	}

	if !reflect.DeepEqual(got, reparsed) {
		t.Errorf("round-trip failed:\n  want: %+v\n  got:  %+v", got, reparsed)
	}
}

// ── T-06: I/O core tests (AddCore / RemoveCore) ─────────────────────────────

// minimalRegistry returns a minimal single-entry registry YAML with the given
// entry id, suitable for fixture writes.
func minimalRegistry(ids ...string) string {
	var sb strings.Builder
	sb.WriteString("version: \"1\"\nskills:\n")
	for _, id := range ids {
		sb.WriteString("  - id: " + id + "\n")
		sb.WriteString("    path: " + id + "\n")
		sb.WriteString("    source:\n      type: custom\n")
		sb.WriteString("    install:\n      defaultScope: global\n      targets:\n        - claude\n")
		sb.WriteString("    lifecycle:\n      updateStrategy: overlay-only\n")
	}
	return sb.String()
}

// minimalManifest returns a manifest that lists SKILL.md rows for the given ids.
func minimalManifest(ids ...string) string {
	var sb strings.Builder
	for _, id := range ids {
		sb.WriteString(id + "/SKILL.md custom\n")
	}
	return sb.String()
}

// setupFixture creates registry, manifest, and optional SKILL.md file under
// dir/skills/<id>/SKILL.md when createSKILLMD is true.
func setupFixture(t *testing.T, dir string, regContent, mfContent string, skillIDs []string) (regPath, mfPath, skillsRoot string) {
	t.Helper()
	regPath = filepath.Join(dir, "registry.yaml")
	mfPath = filepath.Join(dir, "overlay.manifest")
	skillsRoot = filepath.Join(dir, "skills")

	if err := os.WriteFile(regPath, []byte(regContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mfPath, []byte(mfContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(skillsRoot, 0755); err != nil {
		t.Fatal(err)
	}
	for _, id := range skillIDs {
		skillDir := filepath.Join(skillsRoot, id)
		if err := os.MkdirAll(skillDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# "+id+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return regPath, mfPath, skillsRoot
}

// TestAddCoreSuccess verifies SC-26: AddCore adds an entry to both registry and
// manifest and the result validates clean.
func TestAddCoreSuccess(t *testing.T) {
	dir := t.TempDir()
	regPath, mfPath, skillsRoot := setupFixture(t,
		dir,
		minimalRegistry("existing"),
		minimalManifest("existing"),
		[]string{"existing", "foo"},
	)

	var out, errBuf bytes.Buffer
	exitCode := -1
	AddCore(
		[]string{"--registry", regPath, "--manifest", mfPath, "--source-root", skillsRoot, "foo"},
		os.ReadFile, os.Stat,
		&out, &errBuf,
		func(c int) { exitCode = c },
	)

	if exitCode != 0 {
		t.Fatalf("AddCore exit %d; stderr=%q", exitCode, errBuf.String())
	}

	// Registry must now contain "foo".
	regBytes, err := os.ReadFile(regPath)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := ParseRegistry(bytes.NewReader(regBytes))
	if err != nil {
		t.Fatalf("re-parse after add: %v", err)
	}
	found := false
	for _, e := range reg.Skills {
		if e.ID == "foo" {
			found = true
		}
	}
	if !found {
		t.Error("AddCore: registry missing 'foo' entry after add")
	}

	// Manifest must contain foo/SKILL.md.
	mfBytes, err := os.ReadFile(mfPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mfBytes), "foo/SKILL.md") {
		t.Errorf("manifest missing 'foo/SKILL.md' row; content=%q", string(mfBytes))
	}

	// Validate must return zero divergences.
	divs, err := Validate(reg, mfPath)
	if err != nil || len(divs) > 0 {
		t.Errorf("Validate after add: err=%v, divs=%v", err, divs)
	}
}

// TestAddCoreMissingSkillMD verifies SC-27: if the skills/<id>/SKILL.md file does
// not exist, AddCore fails loud and leaves both files byte-unchanged.
func TestAddCoreMissingSkillMD(t *testing.T) {
	dir := t.TempDir()
	regContent := minimalRegistry("existing")
	mfContent := minimalManifest("existing")
	regPath, mfPath, skillsRoot := setupFixture(t, dir, regContent, mfContent, []string{"existing"})
	// Create skills/foo/ dir but NOT SKILL.md.
	if err := os.MkdirAll(filepath.Join(skillsRoot, "foo"), 0755); err != nil {
		t.Fatal(err)
	}

	var out, errBuf bytes.Buffer
	exitCode := 0
	AddCore(
		[]string{"--registry", regPath, "--manifest", mfPath, "--source-root", skillsRoot, "foo"},
		os.ReadFile, os.Stat,
		&out, &errBuf,
		func(c int) { exitCode = c },
	)

	if exitCode == 0 {
		t.Fatal("AddCore: expected non-zero exit for missing SKILL.md")
	}
	if !strings.Contains(errBuf.String(), "foo") {
		t.Errorf("stderr %q should mention 'foo'", errBuf.String())
	}

	// Both files must be byte-unchanged.
	gotReg, _ := os.ReadFile(regPath)
	if string(gotReg) != regContent {
		t.Error("registry was mutated despite error")
	}
	gotMf, _ := os.ReadFile(mfPath)
	if string(gotMf) != mfContent {
		t.Error("manifest was mutated despite error")
	}
}

// TestAddCoreIDAlreadyPresent verifies SC-28: adding a duplicate id fails loud
// and leaves both files byte-unchanged.
func TestAddCoreIDAlreadyPresent(t *testing.T) {
	dir := t.TempDir()
	regContent := minimalRegistry("foo")
	mfContent := minimalManifest("foo")
	regPath, mfPath, skillsRoot := setupFixture(t, dir, regContent, mfContent, []string{"foo"})

	var out, errBuf bytes.Buffer
	exitCode := 0
	AddCore(
		[]string{"--registry", regPath, "--manifest", mfPath, "--source-root", skillsRoot, "foo"},
		os.ReadFile, os.Stat,
		&out, &errBuf,
		func(c int) { exitCode = c },
	)

	if exitCode == 0 {
		t.Fatal("AddCore: expected non-zero exit for duplicate id")
	}
	gotReg, _ := os.ReadFile(regPath)
	if string(gotReg) != regContent {
		t.Error("registry was mutated despite error")
	}
}

// TestAddCoreRegistryWriteFailureIsAtomic verifies SC-29: if the temp file
// creation fails (directory is read-only), both files remain byte-unchanged.
func TestAddCoreRegistryWriteFailureIsAtomic(t *testing.T) {
	dir := t.TempDir()
	regContent := minimalRegistry("existing")
	mfContent := minimalManifest("existing")
	regPath, mfPath, skillsRoot := setupFixture(t, dir, regContent, mfContent, []string{"existing", "new-skill"})

	// Make the directory read-only so os.CreateTemp fails.
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0755) }) // restore for cleanup

	var out, errBuf bytes.Buffer
	exitCode := 0
	AddCore(
		[]string{"--registry", regPath, "--manifest", mfPath, "--source-root", skillsRoot, "new-skill"},
		os.ReadFile, os.Stat,
		&out, &errBuf,
		func(c int) { exitCode = c },
	)

	if exitCode == 0 {
		t.Fatal("AddCore: expected non-zero exit when write fails")
	}
	gotReg, _ := os.ReadFile(regPath)
	if string(gotReg) != regContent {
		t.Error("registry was mutated despite write failure")
	}
	gotMf, _ := os.ReadFile(mfPath)
	if string(gotMf) != mfContent {
		t.Error("manifest was mutated despite write failure")
	}
}

// TestAddCoreValidateBeforeWrite verifies SC-30: if the in-memory state would
// produce a divergence, AddCore fails and leaves files unchanged.
// We test this by passing a corrupt readFile that returns a parse-breaking payload
// for the registry path to prevent even reaching the validate step cleanly.
// Instead we invoke AddCore with an invalid id to trigger failure before write.
func TestAddCoreValidateBeforeWrite(t *testing.T) {
	// This test verifies the validate-before-write guard by using a readFile
	// that returns a valid registry but an INCOMPATIBLE manifest (manifest has
	// extra entry that will cause a MISSING_IN_REGISTRY divergence after add).
	dir := t.TempDir()
	// Registry has "existing"; manifest has "existing" AND "ghost" (registry-unknown).
	regContent := minimalRegistry("existing")
	mfContent := minimalManifest("existing") + "ghost/SKILL.md custom\n"
	regPath, mfPath, skillsRoot := setupFixture(t, dir, regContent, mfContent, []string{"existing", "new-skill"})

	var out, errBuf bytes.Buffer
	exitCode := 0
	AddCore(
		[]string{"--registry", regPath, "--manifest", mfPath, "--source-root", skillsRoot, "new-skill"},
		os.ReadFile, os.Stat,
		&out, &errBuf,
		func(c int) { exitCode = c },
	)

	// The manifest has a ghost entry that is unknown to the registry, so
	// the cross-check (ADR-9 step 7) must catch it and fail before writing.
	if exitCode == 0 {
		t.Fatal("AddCore: expected non-zero exit when validate-before-write detects divergence")
	}
	gotReg, _ := os.ReadFile(regPath)
	if string(gotReg) != regContent {
		t.Error("registry was written despite validate failure")
	}
}

// TestRemoveCoreSuccess verifies SC-31: RemoveCore removes the entry from both
// registry and manifest while preserving other entries.
func TestRemoveCoreSuccess(t *testing.T) {
	dir := t.TempDir()
	regContent := minimalRegistry("existing", "foo", "other")
	mfContent := minimalManifest("existing", "foo", "other")
	regPath, mfPath, skillsRoot := setupFixture(t, dir, regContent, mfContent, []string{"existing", "foo", "other"})

	var out, errBuf bytes.Buffer
	exitCode := -1
	RemoveCore(
		[]string{"--registry", regPath, "--manifest", mfPath, "--source-root", skillsRoot, "foo"},
		os.ReadFile,
		&out, &errBuf,
		func(c int) { exitCode = c },
	)

	if exitCode != 0 {
		t.Fatalf("RemoveCore exit %d; stderr=%q", exitCode, errBuf.String())
	}

	regBytes, _ := os.ReadFile(regPath)
	reg, err := ParseRegistry(bytes.NewReader(regBytes))
	if err != nil {
		t.Fatalf("re-parse after remove: %v", err)
	}

	// Must have exactly existing + other.
	if len(reg.Skills) != 2 {
		t.Fatalf("expected 2 skills after remove, got %d", len(reg.Skills))
	}
	if reg.Skills[0].ID != "existing" || reg.Skills[1].ID != "other" {
		t.Errorf("order wrong: [%s, %s]", reg.Skills[0].ID, reg.Skills[1].ID)
	}

	mfBytes, _ := os.ReadFile(mfPath)
	if strings.Contains(string(mfBytes), "foo/SKILL.md") {
		t.Error("manifest still contains 'foo/SKILL.md' after remove")
	}

	// Validate must return zero divergences.
	divs, err := Validate(reg, mfPath)
	if err != nil || len(divs) > 0 {
		t.Errorf("Validate after remove: err=%v, divs=%v", err, divs)
	}
}

// TestRemoveCoreIDAbsent verifies SC-32: removing an id not in the registry
// fails loud with non-empty stderr and leaves registry unchanged.
func TestRemoveCoreIDAbsent(t *testing.T) {
	dir := t.TempDir()
	regContent := minimalRegistry("existing")
	mfContent := minimalManifest("existing")
	regPath, mfPath, skillsRoot := setupFixture(t, dir, regContent, mfContent, []string{"existing"})

	var out, errBuf bytes.Buffer
	exitCode := 0
	RemoveCore(
		[]string{"--registry", regPath, "--manifest", mfPath, "--source-root", skillsRoot, "foo"},
		os.ReadFile,
		&out, &errBuf,
		func(c int) { exitCode = c },
	)

	if exitCode == 0 {
		t.Fatal("RemoveCore: expected non-zero exit for absent id")
	}
	if errBuf.Len() == 0 {
		t.Error("RemoveCore: expected non-empty stderr for absent id")
	}
	gotReg, _ := os.ReadFile(regPath)
	if string(gotReg) != regContent {
		t.Error("registry was mutated despite absent-id error")
	}
}

// TestRemoveCoreDoesNotDeleteDir verifies SC-33: RemoveCore removes registry and
// manifest entries but does NOT delete the skills/<id>/ directory on disk.
func TestRemoveCoreDoesNotDeleteDir(t *testing.T) {
	dir := t.TempDir()
	regContent := minimalRegistry("foo")
	mfContent := minimalManifest("foo")
	regPath, mfPath, skillsRoot := setupFixture(t, dir, regContent, mfContent, []string{"foo"})

	var out, errBuf bytes.Buffer
	exitCode := -1
	RemoveCore(
		[]string{"--registry", regPath, "--manifest", mfPath, "--source-root", skillsRoot, "foo"},
		os.ReadFile,
		&out, &errBuf,
		func(c int) { exitCode = c },
	)

	if exitCode != 0 {
		t.Fatalf("RemoveCore exit %d; stderr=%q", exitCode, errBuf.String())
	}

	// skills/foo/SKILL.md must still exist.
	skillMDPath := filepath.Join(skillsRoot, "foo", "SKILL.md")
	if _, err := os.Stat(skillMDPath); err != nil {
		t.Errorf("RemoveCore must not delete skills dir: %v", err)
	}
}

// TestRemoveCoreManifestWriteFailureIsAtomic verifies SC-34: if a write fails
// during the atomic sequence, both files remain byte-unchanged.
func TestRemoveCoreManifestWriteFailureIsAtomic(t *testing.T) {
	dir := t.TempDir()
	regContent := minimalRegistry("foo")
	mfContent := minimalManifest("foo")
	regPath, mfPath, skillsRoot := setupFixture(t, dir, regContent, mfContent, []string{"foo"})

	// Make the directory read-only so temp creation fails.
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0755) })

	var out, errBuf bytes.Buffer
	exitCode := 0
	RemoveCore(
		[]string{"--registry", regPath, "--manifest", mfPath, "--source-root", skillsRoot, "foo"},
		os.ReadFile,
		&out, &errBuf,
		func(c int) { exitCode = c },
	)

	if exitCode == 0 {
		t.Fatal("RemoveCore: expected non-zero exit when write fails")
	}
	gotReg, _ := os.ReadFile(regPath)
	if string(gotReg) != regContent {
		t.Error("registry was mutated despite write failure")
	}
	gotMf, _ := os.ReadFile(mfPath)
	if string(gotMf) != mfContent {
		t.Error("manifest was mutated despite write failure")
	}
}

// TestAddRemoveCycleValidateAligned verifies SC-35: add then remove a skill;
// after each step Validate returns zero divergences; base entry is unchanged.
func TestAddRemoveCycleValidateAligned(t *testing.T) {
	dir := t.TempDir()
	regContent := minimalRegistry("base")
	mfContent := minimalManifest("base")
	regPath, mfPath, skillsRoot := setupFixture(t, dir, regContent, mfContent, []string{"base", "new-skill"})

	runAddCore := func() {
		t.Helper()
		var out, errBuf bytes.Buffer
		exitCode := -1
		AddCore(
			[]string{"--registry", regPath, "--manifest", mfPath, "--source-root", skillsRoot, "new-skill"},
			os.ReadFile, os.Stat,
			&out, &errBuf,
			func(c int) { exitCode = c },
		)
		if exitCode != 0 {
			t.Fatalf("AddCore exit %d; stderr=%q", exitCode, errBuf.String())
		}
	}

	runRemoveCore := func() {
		t.Helper()
		var out, errBuf bytes.Buffer
		exitCode := -1
		RemoveCore(
			[]string{"--registry", regPath, "--manifest", mfPath, "--source-root", skillsRoot, "new-skill"},
			os.ReadFile,
			&out, &errBuf,
			func(c int) { exitCode = c },
		)
		if exitCode != 0 {
			t.Fatalf("RemoveCore exit %d; stderr=%q", exitCode, errBuf.String())
		}
	}

	checkValidate := func(label string, wantSkills int) {
		t.Helper()
		regBytes, _ := os.ReadFile(regPath)
		reg, err := ParseRegistry(bytes.NewReader(regBytes))
		if err != nil {
			t.Fatalf("%s: re-parse: %v", label, err)
		}
		if len(reg.Skills) != wantSkills {
			t.Errorf("%s: expected %d skills, got %d", label, wantSkills, len(reg.Skills))
		}
		divs, vErr := Validate(reg, mfPath)
		if vErr != nil || len(divs) > 0 {
			t.Errorf("%s: Validate: err=%v, divs=%v", label, vErr, divs)
		}
		// base entry must always be present.
		baseFound := false
		for _, e := range reg.Skills {
			if e.ID == "base" {
				baseFound = true
			}
		}
		if !baseFound {
			t.Errorf("%s: base entry missing", label)
		}
	}

	runAddCore()
	checkValidate("after add", 2)

	runRemoveCore()
	checkValidate("after remove", 1)
}

// ── T-06: SC-65..SC-68 external provenance via AddCore ───────────────────────

// TestAddCoreExternalRepo verifies SC-65: AddCore --repo creates an external
// entry with the custom manifest tag and Validate returns nil (R-127, R-130).
func TestAddCoreExternalRepo(t *testing.T) {
	dir := t.TempDir()
	regPath, mfPath, skillsRoot := setupFixture(t, dir,
		minimalRegistry("existing"),
		minimalManifest("existing"),
		[]string{"existing", "foo"},
	)

	var out, errBuf bytes.Buffer
	exitCode := -1
	AddCore(
		[]string{"--registry", regPath, "--manifest", mfPath, "--source-root", skillsRoot,
			"foo", "--repo", "https://github.com/example/skills"},
		os.ReadFile, os.Stat,
		&out, &errBuf,
		func(c int) { exitCode = c },
	)

	if exitCode != 0 {
		t.Fatalf("SC-65: AddCore exit %d; stderr=%q", exitCode, errBuf.String())
	}

	// Registry must have an external entry for foo.
	regBytes, err := os.ReadFile(regPath)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := ParseRegistry(bytes.NewReader(regBytes))
	if err != nil {
		t.Fatalf("SC-65: re-parse: %v", err)
	}
	var found *Entry
	for i := range reg.Skills {
		if reg.Skills[i].ID == "foo" {
			found = &reg.Skills[i]
		}
	}
	if found == nil {
		t.Fatal("SC-65: entry 'foo' not found in registry")
	}
	if found.Source.Type != "external" {
		t.Errorf("SC-65: Source.Type = %q, want external", found.Source.Type)
	}
	if found.Source.Repo != "https://github.com/example/skills" {
		t.Errorf("SC-65: Source.Repo = %q, want https://github.com/example/skills", found.Source.Repo)
	}

	// Manifest must contain foo/SKILL.md custom (not managed).
	mfBytes, err := os.ReadFile(mfPath)
	if err != nil {
		t.Fatal(err)
	}
	mfStr := string(mfBytes)
	if !strings.Contains(mfStr, "foo/SKILL.md custom") {
		t.Errorf("SC-65: manifest missing 'foo/SKILL.md custom'; content=%q", mfStr)
	}
	if strings.Contains(mfStr, "foo/SKILL.md managed") {
		t.Errorf("SC-65: manifest must not have 'foo/SKILL.md managed'; content=%q", mfStr)
	}

	// Validate must return zero divergences.
	divs, vErr := Validate(reg, mfPath)
	if vErr != nil || len(divs) > 0 {
		t.Errorf("SC-65: Validate after external add: err=%v, divs=%v", vErr, divs)
	}
}

// TestAddCoreExternalRepoRef verifies SC-66: --repo --ref sets Source.Ref and
// the round-trip holds (R-127, R-129).
func TestAddCoreExternalRepoRef(t *testing.T) {
	dir := t.TempDir()
	regPath, mfPath, skillsRoot := setupFixture(t, dir,
		minimalRegistry("existing"),
		minimalManifest("existing"),
		[]string{"existing", "bar"},
	)

	var out, errBuf bytes.Buffer
	exitCode := -1
	AddCore(
		[]string{"--registry", regPath, "--manifest", mfPath, "--source-root", skillsRoot,
			"bar", "--repo", "https://example.com/repo", "--ref", "deadbeef"},
		os.ReadFile, os.Stat,
		&out, &errBuf,
		func(c int) { exitCode = c },
	)

	if exitCode != 0 {
		t.Fatalf("SC-66: AddCore exit %d; stderr=%q", exitCode, errBuf.String())
	}

	regBytes, err := os.ReadFile(regPath)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := ParseRegistry(bytes.NewReader(regBytes))
	if err != nil {
		t.Fatalf("SC-66: re-parse: %v", err)
	}

	var found *Entry
	for i := range reg.Skills {
		if reg.Skills[i].ID == "bar" {
			found = &reg.Skills[i]
		}
	}
	if found == nil {
		t.Fatal("SC-66: entry 'bar' not found")
	}
	if found.Source.Ref != "deadbeef" {
		t.Errorf("SC-66: Source.Ref = %q, want deadbeef", found.Source.Ref)
	}

	// Round-trip: parse(serialize(reg)) must DeepEqual reg.
	serialized, serErr := Serialize(reg)
	if serErr != nil {
		t.Fatalf("SC-66: Serialize: %v", serErr)
	}
	reparsed, rpErr := ParseRegistry(bytes.NewReader(serialized))
	if rpErr != nil {
		t.Fatalf("SC-66: re-parse after serialize: %v", rpErr)
	}
	if !reflect.DeepEqual(reg, reparsed) {
		t.Errorf("SC-66: round-trip not equal:\n  before: %+v\n  after:  %+v", reg, reparsed)
	}
}

// TestAddCoreNoRepoStaysCustom verifies SC-67: AddCore without --repo produces
// a custom entry (non-regression, R-128).
func TestAddCoreNoRepoStaysCustom(t *testing.T) {
	dir := t.TempDir()
	regPath, mfPath, skillsRoot := setupFixture(t, dir,
		minimalRegistry("existing"),
		minimalManifest("existing"),
		[]string{"existing", "baz"},
	)

	var out, errBuf bytes.Buffer
	exitCode := -1
	AddCore(
		[]string{"--registry", regPath, "--manifest", mfPath, "--source-root", skillsRoot, "baz"},
		os.ReadFile, os.Stat,
		&out, &errBuf,
		func(c int) { exitCode = c },
	)

	if exitCode != 0 {
		t.Fatalf("SC-67: AddCore exit %d; stderr=%q", exitCode, errBuf.String())
	}

	regBytes, err := os.ReadFile(regPath)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := ParseRegistry(bytes.NewReader(regBytes))
	if err != nil {
		t.Fatalf("SC-67: re-parse: %v", err)
	}

	var found *Entry
	for i := range reg.Skills {
		if reg.Skills[i].ID == "baz" {
			found = &reg.Skills[i]
		}
	}
	if found == nil {
		t.Fatal("SC-67: entry 'baz' not found")
	}
	if found.Source.Type != "custom" {
		t.Errorf("SC-67: Source.Type = %q, want custom", found.Source.Type)
	}
	if found.Source.Repo != "" {
		t.Errorf("SC-67: Source.Repo = %q, want empty", found.Source.Repo)
	}
}

// TestAddCoreExternalMissingSkillMD verifies SC-68: --repo still enforces the
// SKILL.md precondition; missing dir → exit != 0, files unchanged (R-129).
func TestAddCoreExternalMissingSkillMD(t *testing.T) {
	dir := t.TempDir()
	regContent := minimalRegistry("existing")
	mfContent := minimalManifest("existing")
	regPath, mfPath, skillsRoot := setupFixture(t, dir, regContent, mfContent, []string{"existing"})

	var out, errBuf bytes.Buffer
	exitCode := 0
	AddCore(
		[]string{"--registry", regPath, "--manifest", mfPath, "--source-root", skillsRoot,
			"missing", "--repo", "https://example.com/repo"},
		os.ReadFile, os.Stat,
		&out, &errBuf,
		func(c int) { exitCode = c },
	)

	if exitCode == 0 {
		t.Fatal("SC-68: expected non-zero exit for missing SKILL.md with --repo")
	}
	if !strings.Contains(errBuf.String(), "missing") {
		t.Errorf("SC-68: stderr %q should mention 'missing'", errBuf.String())
	}

	// Files must be byte-unchanged.
	gotReg, _ := os.ReadFile(regPath)
	if string(gotReg) != regContent {
		t.Error("SC-68: registry was mutated despite missing SKILL.md")
	}
	gotMf, _ := os.ReadFile(mfPath)
	if string(gotMf) != mfContent {
		t.Error("SC-68: manifest was mutated despite missing SKILL.md")
	}
}

// TestAddCoreRefWithoutRepo verifies ADR-14: --ref without --repo must fail loudly.
func TestAddCoreRefWithoutRepo(t *testing.T) {
	dir := t.TempDir()
	regPath, mfPath, skillsRoot := setupFixture(t, dir,
		minimalRegistry("existing"),
		minimalManifest("existing"),
		[]string{"existing", "foo"},
	)

	var out, errBuf bytes.Buffer
	exitCode := 0
	AddCore(
		[]string{"--registry", regPath, "--manifest", mfPath, "--source-root", skillsRoot,
			"foo", "--ref", "deadbeef"},
		os.ReadFile, os.Stat,
		&out, &errBuf,
		func(c int) { exitCode = c },
	)

	if exitCode == 0 {
		t.Fatal("ADR-14: expected non-zero exit for --ref without --repo")
	}
	if errBuf.Len() == 0 {
		t.Error("ADR-14: expected non-empty stderr for --ref without --repo")
	}
}
