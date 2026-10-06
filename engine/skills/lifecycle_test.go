package skills

import (
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
// []string{}) so the round-trip through encode → decode produces DeepEqual.
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
	out, err := serializeRegistry(got)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	reparsed, err := parseRegistry(out)
	if err != nil {
		t.Fatalf("decoding the encoded registry: %v", err)
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
				ID:     "existing",
				Path:   "existing",
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
			ID:     id,
			Path:   id,
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
					Type:     "core",
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

	out, err := serializeRegistry(got)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}

	reparsed, err := parseRegistry(out)
	if err != nil {
		t.Fatalf("decoding after encoding: %v", err)
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

// lintCleanSkillMD returns the smallest valid SKILL.md fixture for AddCore
// tests. The lifecycle gate applies to every existing fixture, so these
// helpers keep unrelated lifecycle assertions focused on their original
// behavior rather than on lint failures.
func lintCleanSkillMD(id string) string {
	return "---\n" +
		"name: " + id + "\n" +
		"description: A concise procedural skill for " + id + ".\n" +
		"license: MIT\n" +
		"metadata:\n" +
		"  author: tester\n" +
		"  version: \"1.0\"\n" +
		"---\n" +
		"## Activation Contract\n" +
		"Load this skill for its documented procedure.\n\n" +
		"## Hard Rules\n" +
		"- Keep the procedure explicit.\n\n" +
		"## Execution Steps\n" +
		"1. Follow the procedure.\n"
}

// setupFixture creates registry, manifest, and lint-clean SKILL.md files under
// dir/skills/<id>/SKILL.md for the requested skill IDs.
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
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(lintCleanSkillMD(id)), 0644); err != nil {
			t.Fatal(err)
		}
		// A global skill needs a human approval record to be added or to
		// validate; the fixture starts approved so tests of the other add
		// behaviors stay about those behaviors. The approval-specific tests
		// (approval_gate_test.go) remove or spoil the record explicitly.
		writeValidApproval(t, skillsRoot, id)
	}
	return regPath, mfPath, skillsRoot
}

// ── T-06: SC-65..SC-68 external provenance via AddCore ───────────────────────
