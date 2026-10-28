package settings

import (
	"reflect"
	"testing"
)

// namedFamilies is every family the overlay installs, by name, for the tests that hold them to the
// same rules. TestNamedFamiliesAreTheInstalledOnes keeps it complete.
func namedFamilies() map[string]hookFamily {
	return map[string]hookFamily{
		"minimalism":   minimalismFamily,
		"design":       designFamily,
		"sync":         syncTriggerFamily,
		"receipt":      reviewReceiptFamily,
		"shaper":       shaperGuardFamily,
		"projection":   projectionFamily,
		"approve":      approveGuardFamily,
		"reviewstop":   reviewStopFamily,
		"builderguard": builderGuardFamily,
	}
}

// A spec says which entry a family writes, and the entry it builds must be that one: the event it
// sits under and the matcher it carries (none when the spec has none). A family whose spec and
// builder drift apart reports one thing and writes another.
func TestEverySpecDescribesTheEntryItsFamilyBuilds(t *testing.T) {
	for name, family := range namedFamilies() {
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
	for name, family := range namedFamilies() {
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
	families := namedFamilies()
	for _, binary := range []string{toyBinary, "/my dir/it's here/overlay"} {
		for builder, family := range families {
			for _, spec := range family.specs {
				entry := family.build(binary, spec)
				for other, candidate := range families {
					if other != builder && candidate.owns(entry, binary) {
						t.Errorf("the %s entry %q for %q is also owned by the %s family", builder, spec.event, binary, other)
					}
				}
			}
		}
	}
}

// An entry of any family that an older version of the overlay wrote, with another command line, is
// replaced by the one this version writes: installing again leaves exactly one entry per spec and
// a second install changes nothing.
func TestEveryFamilyReplacesAnInstalledEntryWithAnOlderCommand(t *testing.T) {
	for name, family := range namedFamilies() {
		t.Run(name, func(t *testing.T) {
			hooks := map[string]interface{}{}
			for _, spec := range family.specs {
				entry := family.build(toyBinary, spec)
				inner := entry["hooks"].([]interface{})[0].(map[string]interface{})
				inner["command"] = inner["command"].(string) + " --an-older-flag"
				existing, _ := hooks[spec.event].([]interface{})
				hooks[spec.event] = append(existing, entry)
			}

			if !family.merge(hooks, toyBinary) {
				t.Fatal("merge() left the older entries as they were")
			}
			for event, want := range family.entries(toyBinary) {
				if got := hooks[event]; !reflect.DeepEqual(got, interface{}(want)) {
					t.Errorf("%s = %v, want exactly the entries this version writes: %v", event, got, want)
				}
			}
			if family.merge(hooks, toyBinary) {
				t.Error("a second merge() changed the file")
			}
		})
	}
}

// The table above is the list of installed families by name; it must be that list, so a family
// added to Merge cannot escape the tests that hold every family to the same rules.
func TestNamedFamiliesAreTheInstalledOnes(t *testing.T) {
	named := namedFamilies()
	if len(named) != len(installedFamilies) {
		t.Fatalf("%d families are named and %d are installed", len(named), len(installedFamilies))
	}
	for _, installed := range installedFamilies {
		found := false
		for _, family := range named {
			found = found || family.identity == installed.identity
		}
		if !found {
			t.Errorf("the family with identity %q is installed and not named in the tests", installed.identity)
		}
	}
}
