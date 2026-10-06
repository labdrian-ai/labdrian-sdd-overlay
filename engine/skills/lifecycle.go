package skills

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

// slugRe matches valid skill identifiers: lowercase alphanumeric, may contain
// hyphens, must start with a letter or digit (ADR-8 slug guard).
var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// IsSlug reports whether id is a valid skill identifier: lowercase alphanumeric, hyphens allowed,
// starting with a letter or a digit.
func IsSlug(id string) bool { return slugRe.MatchString(id) }

// AddEntry returns a new Registry with a new entry appended for id, using the
// inferred defaults defined in ADR-8. It fails loudly if:
//   - id fails the slug guard (R-062)
//   - id is already present in reg.Skills (R-061)
//   - reg has fields its reader left out (reg.Unread), which the registry written back would
//     drop; this is the first thing checked
//
// repo and ref control the source type (ADR-14):
//   - repo == "" → Source.Type = "custom" (backward-compatible default, R-128)
//   - repo != "" → Source.Type = "external" with Repo/Ref set (R-127)
//
// The input registry is never mutated (pure function).
func AddEntry(reg Registry, id, repo, ref string) (Registry, error) {
	if err := reg.CheckWritable(); err != nil {
		return Registry{}, err
	}
	if !slugRe.MatchString(id) {
		return Registry{}, fmt.Errorf("id %q: invalid slug (must match ^[a-z0-9][a-z0-9-]*$)", id)
	}
	for _, e := range reg.Skills {
		if e.ID == id {
			return Registry{}, fmt.Errorf("id %q: already registered", id)
		}
	}

	src := Source{Type: "custom"}
	if repo != "" {
		src = Source{Type: "external", Repo: repo, Ref: ref}
	}

	newEntry := Entry{
		ID:     id,
		Path:   id,
		Source: src,
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
// loudly if reg has fields its reader left out (reg.Unread: the registry written back would drop
// them), which is checked first, and if id is not present in reg.Skills (R-069). The relative
// order of remaining entries is preserved (R-070). The input registry is never mutated.
func RemoveEntry(reg Registry, id string) (Registry, error) {
	if err := reg.CheckWritable(); err != nil {
		return Registry{}, err
	}
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

	// Normalize to nil when empty so serialize→parse round-trip holds:
	// A registry decoded from an empty sequence has nil Skills (ADR-8).
	if len(skills) == 0 {
		skills = nil
	}

	return Registry{Version: reg.Version, Skills: skills}, nil
}

// ── I/O helpers ─────────────────────────────────────────────────────────────

// ManifestWithSkill returns src with "<id>/SKILL.md custom" appended,
// ensuring exactly one trailing newline before appending (ADR-5).
func ManifestWithSkill(src []byte, id string) []byte {
	out := make([]byte, len(src))
	copy(out, src)
	if len(out) > 0 && out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	return append(out, []byte(id+"/SKILL.md custom\n")...)
}

// ManifestWithoutSkill returns src with all lines whose first field equals
// "<id>/SKILL.md" removed. All other lines are preserved verbatim (ADR-5).
func ManifestWithoutSkill(src []byte, id string) []byte {
	prefix := id + "/SKILL.md"
	var out []byte
	for _, line := range strings.Split(string(src), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == prefix {
			continue
		}
		out = append(out, []byte(line+"\n")...)
	}
	// Remove the extra trailing newline that the split+join adds.
	if len(out) > 0 && bytes.HasSuffix(out, []byte("\n\n")) && !bytes.HasSuffix(src, []byte("\n\n")) {
		out = out[:len(out)-1]
	}
	return out
}
