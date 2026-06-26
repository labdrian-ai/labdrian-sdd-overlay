package skills

import (
	"fmt"
	"regexp"
)

// slugRe matches valid skill identifiers: lowercase alphanumeric, may contain
// hyphens, must start with a letter or digit (ADR-8 slug guard).
var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// AddEntry returns a new Registry with a new entry appended for id, using the
// inferred defaults defined in ADR-8. It fails loudly if:
//   - id fails the slug guard (R-062)
//   - id is already present in reg.Skills (R-061)
//
// The input registry is never mutated (pure function).
func AddEntry(reg Registry, id string) (Registry, error) {
	if !slugRe.MatchString(id) {
		return Registry{}, fmt.Errorf("id %q: invalid slug (must match ^[a-z0-9][a-z0-9-]*$)", id)
	}
	for _, e := range reg.Skills {
		if e.ID == id {
			return Registry{}, fmt.Errorf("id %q: already registered", id)
		}
	}

	newEntry := Entry{
		ID:   id,
		Path: id,
		Source: Source{
			Type:     "custom",
			Upstream: nil,
		},
		Install: Install{
			DefaultScope:    "global",
			Targets:         []string{"claude", "opencode", "codex"},
			AllowedProjects: nil, // must be nil, not []string{} — ADR-8, ADR-7
		},
		Lifecycle: Lifecycle{
			UpdateStrategy: "overlay-only",
		},
	}

	// Copy input slice to avoid mutating the caller's backing array.
	skills := make([]Entry, len(reg.Skills), len(reg.Skills)+1)
	copy(skills, reg.Skills)
	skills = append(skills, newEntry)

	return Registry{Version: reg.Version, Skills: skills}, nil
}

// RemoveEntry returns a new Registry with the entry for id removed. It fails
// loudly if id is not present in reg.Skills (R-069). The relative order of
// remaining entries is preserved (R-070). The input registry is never mutated.
func RemoveEntry(reg Registry, id string) (Registry, error) {
	found := false
	for _, e := range reg.Skills {
		if e.ID == id {
			found = true
			break
		}
	}
	if !found {
		return Registry{}, fmt.Errorf("id %q: not found in registry", id)
	}

	skills := make([]Entry, 0, len(reg.Skills)-1)
	for _, e := range reg.Skills {
		if e.ID != id {
			skills = append(skills, e)
		}
	}

	return Registry{Version: reg.Version, Skills: skills}, nil
}
