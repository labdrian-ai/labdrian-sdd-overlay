package skills

import (
	"bytes"
	"reflect"
	"testing"
)

// ── T-04: Pure-transform tests (AddEntry / RemoveEntry) ──────────────────────

// TestAddEntryDefaults verifies R-062: AddEntry on a valid id and empty registry
// returns an entry populated with all required default values.
func TestAddEntryDefaults(t *testing.T) {
	reg := Registry{Version: "1"}

	got, err := AddEntry(reg, "my-skill")
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

	got, err := AddEntry(reg, "skill1")
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
	invalid := []string{
		"",
		"../evil",
		"foo bar",
		"FOO",
		"Skill",
		"-leading-dash",
		"has/slash",
		"has.dot",
		"..",
		"0abc", // starts with digit — actually valid per ^[a-z0-9][a-z0-9-]*$
	}
	// ids starting with digit ARE valid per the regex; remove from invalid set.
	// The regex is ^[a-z0-9][a-z0-9-]*$ so "0abc" actually passes.
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
		_, err := AddEntry(reg, id)
		if err == nil {
			t.Errorf("AddEntry(%q): expected error, got nil", id)
		}
	}
	for _, id := range valid {
		_, err := AddEntry(reg, id)
		if err != nil {
			t.Errorf("AddEntry(%q): unexpected error: %v", id, err)
		}
		// Reset for next iteration (avoid duplicate errors).
		reg = Registry{Version: "1"}
	}

	// Suppress the unused variable warning from the first slice.
	_ = invalid
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

	_, err := AddEntry(reg, "existing")
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

	reg1, err := AddEntry(reg, "first")
	if err != nil {
		t.Fatalf("AddEntry first: %v", err)
	}
	reg2, err := AddEntry(reg1, "second")
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

	got, err := AddEntry(base, "new-skill")
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
