package roles

import (
	"reflect"
	"testing"
)

func TestSkillsNamedReturnsTheIdsABodyNamesInTheFormIdSkillInOrderOnce(t *testing.T) {
	body := "Follow the `test-first` skill.\nRead the `odd-feature-doc` skill, then the `test-first` skill again.\n"
	want := []string{"test-first", "odd-feature-doc"}
	if got := SkillsNamed(body); !reflect.DeepEqual(got, want) {
		t.Errorf("SkillsNamed = %v, want %v", got, want)
	}
}

func TestSkillsNamedIgnoresWhatIsNotThatForm(t *testing.T) {
	body := "Load skill test-first.\nThe `go-test` tool, a `Test_First` skill, and `skill` alone.\n"
	if got := SkillsNamed(body); len(got) != 0 {
		t.Errorf("SkillsNamed = %v, want none", got)
	}
}

func TestSkillsNamedEndsAMentionAtAWordEnd(t *testing.T) {
	for name, c := range map[string]struct {
		body string
		want []string
	}{
		"a plural":                      {"Read the `foo` skills.", nil},
		"a hyphenated word":             {"Read the `foo` skill-set.", nil},
		"an underscored word":           {"Read the `foo` skill_x.", nil},
		"a longer word":                 {"Read the `foo` skillful way.", nil},
		"a full stop":                   {"Read the `foo` skill.", []string{"foo"}},
		"the end of the body":           {"Read the `foo` skill", []string{"foo"}},
		"a line break":                  {"Read the `foo` skill\nnext", []string{"foo"}},
		"a mention right after another": {"`a` skill`b` skill", []string{"a", "b"}},
	} {
		t.Run(name, func(t *testing.T) {
			if got := SkillsNamed(c.body); !reflect.DeepEqual(got, c.want) {
				t.Errorf("SkillsNamed(%q) = %v, want %v", c.body, got, c.want)
			}
		})
	}
}

func TestReadsOnlyItsCandidateIsTrueForTheReviewerAndThePolisherOnly(t *testing.T) {
	for agent, want := range map[string]bool{
		"reviewer": true, "polisher": true,
		"builder": false, "shaper": false, "prototyper": false, "sweeper": false, "verifier": false,
		"delivery": false, "foreman": false, "": false, "sdd-tasks": false,
	} {
		if got := ReadsOnlyItsCandidate(agent); got != want {
			t.Errorf("ReadsOnlyItsCandidate(%q) = %v, want %v", agent, got, want)
		}
	}
}
