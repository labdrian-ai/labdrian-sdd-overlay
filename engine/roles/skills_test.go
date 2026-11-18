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
