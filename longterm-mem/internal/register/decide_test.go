package register

import "testing"

// TestDecide_SemanticsTableIsExhaustive proves Decide (D9) is a total,
// deterministic function over its full bool^4 input domain: every one of
// the 16 (entryPresent, recordPresent, entryOwned, fingerprintMatches)
// combinations maps to exactly the Action the D9 semantics table names,
// with refuse, adopt and noop asserted as distinct, never-collapsed
// outcomes (11a.6).
func TestDecide_SemanticsTableIsExhaustive(t *testing.T) {
	cases := []struct {
		name               string
		entryPresent       bool
		recordPresent      bool
		entryOwned         bool
		fingerprintMatches bool
		want               Action
	}{
		// !entryPresent && !recordPresent → insert, regardless of
		// entryOwned/fingerprintMatches (both meaningless when there is no
		// entry and no record to compare).
		{"no entry, no record", false, false, false, false, ActionInsert},
		{"no entry, no record, fp true", false, false, false, true, ActionInsert},
		{"no entry, no record, owned true", false, false, true, false, ActionInsert},
		{"no entry, no record, owned and fp true", false, false, true, true, ActionInsert},

		// !entryPresent && recordPresent → replace: we own this target
		// but the entry was deleted out from under us; (re)write it.
		{"no entry, record present", false, true, false, false, ActionReplace},
		{"no entry, record present, fp true", false, true, false, true, ActionReplace},
		{"no entry, record present, owned true", false, true, true, false, ActionReplace},
		{"no entry, record present, owned and fp true", false, true, true, true, ActionReplace},

		// entryPresent && !recordPresent → entryOwned decides. Refuse when
		// the entry on disk is not the one we would write (someone else's
		// configuration, never overwritten); adopt when it IS, because only
		// longterm-mem writes that exact entry — the record was lost, not
		// the ownership.
		{"entry present, no record, not ours", true, false, false, false, ActionRefuse},
		{"entry present, no record, not ours, fp true", true, false, false, true, ActionRefuse},
		{"entry present, no record, ours", true, false, true, false, ActionAdopt},
		{"entry present, no record, ours, fp true", true, false, true, true, ActionAdopt},

		// entryPresent && recordPresent → fingerprintMatches decides:
		// noop when it matches (nothing to do), replace when it doesn't
		// (stale or hand-edited entry we own — rewrite it). entryOwned is a
		// don't-care here: the record already settles ownership.
		{"entry present, record present, fp false", true, true, false, false, ActionReplace},
		{"entry present, record present, owned, fp false", true, true, true, false, ActionReplace},
		{"entry present, record present, fp true", true, true, false, true, ActionNoop},
		{"entry present, record present, owned, fp true", true, true, true, true, ActionNoop},
	}

	if len(cases) != 16 {
		t.Fatalf("test table has %d cases, want all 16 combinations of bool^4", len(cases))
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Decide(c.entryPresent, c.recordPresent, c.entryOwned, c.fingerprintMatches)
			if got != c.want {
				t.Fatalf("Decide(%v, %v, %v, %v) = %v, want %v",
					c.entryPresent, c.recordPresent, c.entryOwned, c.fingerprintMatches, got, c.want)
			}
		})
	}
}

// TestDecide_RefuseAdoptAndNoopAreDistinct proves the three outcomes for
// "an entry with this name is already there" are never collapsed into one
// another: adopting an entry we can prove is ours, refusing one we cannot,
// and doing nothing to one we already have a matching record for are three
// genuinely different Action values, not three names for the same thing.
//
// The pair that matters most is refuse vs. adopt. They differ in exactly
// one input — whether the entry on disk is byte-identical to the one we
// would write — and collapsing them in either direction is a real defect:
// collapsing adopt into refuse locks a user out of their own installation
// the moment install-state.json is lost, and collapsing refuse into adopt
// overwrites a third party's server.
func TestDecide_RefuseAdoptAndNoopAreDistinct(t *testing.T) {
	refuse := Decide(true, false, false, false)
	adopt := Decide(true, false, true, false)
	noop := Decide(true, true, false, true)

	if refuse == adopt || refuse == noop || adopt == noop {
		t.Fatalf("Decide collapsed refuse/adopt/noop: refuse = %v, adopt = %v, noop = %v", refuse, adopt, noop)
	}
	if refuse != ActionRefuse {
		t.Fatalf("Decide(true, false, false, false) = %v, want ActionRefuse", refuse)
	}
	if adopt != ActionAdopt {
		t.Fatalf("Decide(true, false, true, false) = %v, want ActionAdopt", adopt)
	}
	if noop != ActionNoop {
		t.Fatalf("Decide(true, true, false, true) = %v, want ActionNoop", noop)
	}
}
