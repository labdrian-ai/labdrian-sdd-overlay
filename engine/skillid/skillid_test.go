package skillid_test

// skillid (labdrian-orchestrator-opencode T60 commit A, S8 L173 F5): the one form of a skill id and of the
// characters of a word that holds one, shared by the gate and the roles.

import (
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skillid"
)

func TestValidIsTheWholeStringMatchingThePattern(t *testing.T) {
	for s, want := range map[string]bool{
		"test-first": true, "0abc": true, "a": true,
		"": false, "-a": false, "A": false, "a_b": false, "a/b": false, "a.b": false,
	} {
		if got := skillid.Valid(s); got != want {
			t.Errorf("Valid(%q) = %v, want %v", s, got, want)
		}
	}
}

func TestWordCharIsTheCharactersOfASkillIDUppercaseAndUnderscore(t *testing.T) {
	for b, want := range map[byte]bool{
		'_': true, '-': true, '0': true, '9': true, 'a': true, 'z': true, 'A': true, 'Z': true,
		' ': false, '.': false, '/': false, '`': false, 0x80: false,
	} {
		if got := skillid.WordChar(b); got != want {
			t.Errorf("WordChar(%q) = %v, want %v", b, got, want)
		}
	}
}
