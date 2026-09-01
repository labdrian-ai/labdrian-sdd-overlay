package register

import "testing"

// TestDecide_SemanticsTableIsExhaustive proves Decide (D9) is a total,
// deterministic function over its full bool^3 input domain: every one of
// the 8 (entryPresent, recordPresent, fingerprintMatches) combinations maps
// to exactly the Action the D9 semantics table names, with refuse and noop
// asserted as distinct, never-collapsed outcomes (11a.6).
func TestDecide_SemanticsTableIsExhaustive(t *testing.T) {
	cases := []struct {
		name               string
		entryPresent       bool
		recordPresent      bool
		fingerprintMatches bool
		want               Action
	}{
		// !entryPresent && !recordPresent → insert, regardless of
		// fingerprintMatches (meaningless when there is no entry and no
		// record to compare).
		{"no entry, no record, fp false", false, false, false, ActionInsert},
		{"no entry, no record, fp true", false, false, true, ActionInsert},

		// !entryPresent && recordPresent → replace: we own this target
		// but the entry was deleted out from under us; (re)write it.
		{"no entry, record present, fp false", false, true, false, ActionReplace},
		{"no entry, record present, fp true", false, true, true, ActionReplace},

		// entryPresent && !recordPresent → refuse: an entry with this
		// name exists but we never wrote it — never overwrite someone
		// else's configuration.
		{"entry present, no record, fp false", true, false, false, ActionRefuse},
		{"entry present, no record, fp true", true, false, true, ActionRefuse},

		// entryPresent && recordPresent → fingerprintMatches decides:
		// noop when it matches (nothing to do), replace when it doesn't
		// (stale or hand-edited entry we own — rewrite it).
		{"entry present, record present, fp false", true, true, false, ActionReplace},
		{"entry present, record present, fp true", true, true, true, ActionNoop},
	}

	if len(cases) != 8 {
		t.Fatalf("test table has %d cases, want all 8 combinations of bool^3", len(cases))
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Decide(c.entryPresent, c.recordPresent, c.fingerprintMatches)
			if got != c.want {
				t.Fatalf("Decide(%v, %v, %v) = %v, want %v",
					c.entryPresent, c.recordPresent, c.fingerprintMatches, got, c.want)
			}
		})
	}
}

// TestDecide_RefuseIsDistinctFromNoop proves refuse is never collapsed
// into noop: two inputs that differ only in whether an ownership record
// exists must produce genuinely different Action values, not just
// different in name.
func TestDecide_RefuseIsDistinctFromNoop(t *testing.T) {
	refuse := Decide(true, false, false)
	noop := Decide(true, true, true)

	if refuse == noop {
		t.Fatalf("Decide collapsed refuse and noop into the same Action value %v", refuse)
	}
	if refuse != ActionRefuse {
		t.Fatalf("Decide(true, false, false) = %v, want ActionRefuse", refuse)
	}
	if noop != ActionNoop {
		t.Fatalf("Decide(true, true, true) = %v, want ActionNoop", noop)
	}
}
