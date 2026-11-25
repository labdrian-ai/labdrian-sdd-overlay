package main

// The member of `review findings` that the Classify step reads is named alike by the command and by step 1 of
// skills/role-chain/references/classify.md (`665b99f747e2-3.polisher` W1, L98 choice 2): the step stops, not blocked, on a
// non-empty `unverified`, so a rename of the member on either side would leave the step reading nothing.

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestTheClassifyStepReadsTheUnverifiedMemberOfTheFindingsFeed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const member = "unverified"
	found := false
	typ := reflect.TypeOf(reviewFindingsJSON{})
	for i := 0; i < typ.NumField(); i++ {
		if name, _, _ := strings.Cut(typ.Field(i).Tag.Get("json"), ","); name == member {
			found = true
		}
	}
	if !found {
		t.Errorf("reviewFindingsJSON has no field tagged json:%q, the member step 1 of classify.md reads", member)
	}
	step, err := os.ReadFile(filepath.Join("..", "..", "skills", "role-chain", "references", "classify.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, after, _ := strings.Cut(string(step), "\n1. ")
	one, _, _ := strings.Cut(after, "\n2. ")
	if !strings.Contains(one, "`"+member+"`") {
		t.Errorf("step 1 of classify.md does not name the member `%s` of the findings feed: %q", member, one)
	}
	if !strings.Contains(one, "stop, not blocked") {
		t.Errorf("step 1 of classify.md does not stop, not blocked, on a non-empty `%s`: %q", member, one)
	}
}
