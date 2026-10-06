package skills

import "testing"

// --- the three questions the domain asks of a registry that was read in part ---

// The rule that a registry the reader did not read whole is not used for what needs all of it has
// one owner and one skeleton: each question says what it would cost to go on.
func TestTheChecksOfARegistryReadInPartShareOneSkeleton(t *testing.T) {
	whole := unreadRegistry()
	reg := unreadRegistry(`line 3: unknown key "color" in skill entry`, "line 9: unknown key \"mirror\" in source")
	const left = `line 3: unknown key "color" in skill entry (and 1 more)`
	for name, tc := range map[string]struct {
		check func(Registry) error
		want  string
	}{
		"writing":   {Registry.CheckWritable, `skills: the registry has fields this program does not read, and rewriting it would drop them: ` + left},
		"building":  {Registry.CheckBuildable, `skills: the registry has fields this program does not read, and a package built from it would be built from a partial read: ` + left},
		"verifying": {Registry.CheckVerifiable, `skills: the registry has fields this program does not read, and validate cannot vouch for a registry it read in part: ` + left},
	} {
		t.Run(name, func(t *testing.T) {
			if err := tc.check(whole); err != nil {
				t.Errorf("the check of a registry read whole = %v, want nil", err)
			}
			if err := tc.check(reg); err == nil || err.Error() != tc.want {
				t.Errorf("the check = %v, want %q", err, tc.want)
			}
		})
	}
}
