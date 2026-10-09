package settings

import (
	"testing"
)

// installedFamilies is every family the overlay installs, by name, for the tests that hold them to
// the same rules.
func installedFamilies() map[string]hookFamily {
	return map[string]hookFamily{
		"minimalism": minimalismFamily,
		"design":     designFamily,
		"projection": projectionFamily,
		"approve":    approveGuardFamily,
	}
}

// A spec says which entry a family writes, and the entry it builds must be that one: the event it
// sits under and the matcher it carries (none when the spec has none). A family whose spec and
// builder drift apart reports one thing and writes another.
func TestEverySpecDescribesTheEntryItsFamilyBuilds(t *testing.T) {
	for name, family := range installedFamilies() {
		for _, spec := range family.specs {
			entry := family.build(toyBinary, spec)
			matcher, has := entry["matcher"]
			switch {
			case spec.matcher == "" && has:
				t.Errorf("%s %s: the spec has no matcher and the entry carries %v", name, spec.event, matcher)
			case spec.matcher != "" && matcher != spec.matcher:
				t.Errorf("%s %s: the spec says matcher %q and the entry carries %v", name, spec.event, spec.matcher, matcher)
			}
		}
	}
}

// Every entry a family builds is its own by the rule that finds it again: it runs the binary the
// family was built for and carries the family's identity, so an install can be undone and a second
// install adds nothing.
func TestEveryEntryAFamilyBuildsIsOwnedByIt(t *testing.T) {
	for name, family := range installedFamilies() {
		for _, spec := range family.specs {
			if entry := family.build(toyBinary, spec); !family.owns(entry, toyBinary) {
				t.Errorf("%s %s %q: the entry it builds is not recognized as its own: %v", name, spec.event, spec.matcher, entry)
			}
		}
	}
}

// Two families never own the same entry: the minimalism and design pairs run the same binary and
// differ only by identity, and an entry that both claimed would be removed or kept for the wrong
// reason.
func TestNoEntryIsOwnedByTwoFamilies(t *testing.T) {
	families := installedFamilies()
	for builder, family := range families {
		for _, spec := range family.specs {
			entry := family.build(toyBinary, spec)
			for other, candidate := range families {
				if other != builder && candidate.owns(entry, toyBinary) {
					t.Errorf("the %s entry %q is also owned by the %s family", builder, spec.event, other)
				}
			}
		}
	}
}
