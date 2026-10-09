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
		"sync":       syncTriggerFamily,
		"receipt":    reviewReceiptFamily,
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

// An entry of a keeping family that an older version of the overlay wrote, with another command
// line, is left exactly as it is: installing again neither rewrites it nor adds a second one.
// (Upgrading installed entries in place is a separate decision, C7.) The projection and approve
// guard families repair; which family does which is stated here, so a family cannot change its
// upkeep without this table changing.
func TestAKeepingFamilyLeavesAnInstalledEntryWithAnOlderCommandAsItIs(t *testing.T) {
	keeping := map[string]bool{"minimalism": true, "design": true, "sync": true, "receipt": true, "projection": false, "approve": false}
	for name, family := range installedFamilies() {
		if (family.upkeep != repairing) != keeping[name] {
			t.Errorf("%s: keeping = %v, want %v", name, family.upkeep != repairing, keeping[name])
		}
		if !keeping[name] {
			continue
		}
		t.Run(name, func(t *testing.T) {
			hooks := map[string]interface{}{}
			for _, spec := range family.specs {
				entry := family.build(toyBinary, spec)
				inner := entry["hooks"].([]interface{})[0].(map[string]interface{})
				inner["command"] = inner["command"].(string) + " --an-older-flag"
				existing, _ := hooks[spec.event].([]interface{})
				hooks[spec.event] = append(existing, entry)
			}
			before := len(hooks)

			if family.merge(hooks, toyBinary) {
				t.Errorf("merge() rewrote or added an entry although the family is installed: %v", hooks)
			}
			if len(hooks) != before {
				t.Errorf("merge() changed the events: %v", hooks)
			}
		})
	}
}
