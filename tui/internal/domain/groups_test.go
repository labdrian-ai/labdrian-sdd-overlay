package domain

// The section of the menu each action is listed under is declared by the
// action (T3). The screen opens a header at the head of each run of one group,
// so the shipped menu must keep the actions of a group next to each other, in
// the order the sections have always had.

import (
	"strings"
	"testing"
)

// TestActions_EachRowDeclaresItsGroupAndEachGroupIsOneRun: every row of the
// menu says which section it is in; the sections come in the order of the
// usage flow (operations, hooks, skills, memory) and no section is split in two
// by a row of another.
func TestActions_EachRowDeclaresItsGroupAndEachGroupIsOneRun(t *testing.T) {
	want := []Group{GroupSync, GroupHooks, GroupSkills, GroupMemory}
	var order []Group
	for _, a := range Actions() {
		if a.Group == "" {
			t.Errorf("%s (%v) declares no group", a.Command, a.Args)
			continue
		}
		if len(order) == 0 || order[len(order)-1] != a.Group {
			order = append(order, a.Group)
		}
	}
	if strings.Join(groupLabels(order), ",") != strings.Join(groupLabels(want), ",") {
		t.Errorf("the menu's sections, in order of appearance, are %v, want %v: a group is split by a row of another, or out of order", order, want)
	}
}

// TestActions_ActualizarRepositorioIsInTheOperationalFlow: self-update is
// target-agnostic like the hooks rows, and it used to be listed under the Hooks
// header for no other reason. It fast-forwards main and deploys it: it belongs
// with the operations, right after restore.
func TestActions_ActualizarRepositorioIsInTheOperationalFlow(t *testing.T) {
	actions := Actions()
	for i, a := range actions {
		if a.Command != "self-update" {
			continue
		}
		if a.Group != GroupSync {
			t.Errorf("self-update is in group %q, want %q", a.Group, GroupSync)
		}
		if i == 0 || actions[i-1].Group != GroupSync || actions[i-1].Command != "restore" {
			t.Errorf("self-update follows %q, want it right after restore in the same section", actions[i-1].Command)
		}
		return
	}
	t.Fatal("the menu has no self-update row")
}

// TestActions_OnlyTheRowsOfTheMenuDeclareAGroup: an action merged into a row
// is not listed, so it has no section.
func TestActions_OnlyTheRowsOfTheMenuDeclareAGroup(t *testing.T) {
	for _, a := range Actions() {
		for _, sub := range a.Also {
			if sub.Group != "" {
				t.Errorf("%s (%v) merged into %q declares group %q: it is not a row", sub.Command, sub.Args, a.Command, sub.Group)
			}
		}
	}
}

func groupLabels(groups []Group) []string {
	out := make([]string, len(groups))
	for i, g := range groups {
		out[i] = string(g)
	}
	return out
}
