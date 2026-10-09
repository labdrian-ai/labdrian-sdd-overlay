package settings

import (
	"reflect"
	"testing"
)

const toyBinary = "/opt/toy/bin/overlay"

// toyFamily is a family of the shape the installed ones have, over a verb no real family uses.
func toyFamily(u upkeep, specs ...hookSpec) hookFamily {
	return hookFamily{
		identity: "toy-verb",
		specs:    specs,
		upkeep:   u,
		build: func(hookCommand string, s hookSpec) map[string]interface{} {
			entry := map[string]interface{}{"hooks": []interface{}{map[string]interface{}{
				"type": "command", "command": hookCommand + " toy-verb --event " + s.event,
			}}}
			if s.matcher != "" {
				entry["matcher"] = s.matcher
			}
			return entry
		},
	}
}

func command(cmd string, matcher ...interface{}) map[string]interface{} {
	entry := map[string]interface{}{"hooks": []interface{}{map[string]interface{}{"type": "command", "command": cmd}}}
	if len(matcher) == 1 {
		entry["matcher"] = matcher[0]
	}
	return entry
}

func list(entries ...interface{}) []interface{} { return entries }

func TestKeepingOneAppendsAfterForeignEntriesAndOnlyOnce(t *testing.T) {
	family := toyFamily(keepingOne, hookSpec{event: "SessionEnd"})
	foreign := command("echo bye")
	hooks := map[string]interface{}{"SessionEnd": list(foreign)}

	if !family.merge(hooks, toyBinary) {
		t.Fatal("merge() reported no change for a missing entry")
	}
	want := list(foreign, family.build(toyBinary, hookSpec{event: "SessionEnd"}))
	if !reflect.DeepEqual(hooks["SessionEnd"], want) {
		t.Errorf("SessionEnd = %v, want the foreign entry then ours", hooks["SessionEnd"])
	}
	if family.merge(hooks, toyBinary) {
		t.Error("a second merge() reported a change")
	}
}

// The difference from repairing, and the reason the older families use keepingOne: an owned entry
// whose command is not what this version writes is left as it is.
func TestKeepingOneLeavesAnOwnedEntryThatDiffersAndRepairingReplacesIt(t *testing.T) {
	old := command(toyBinary+" toy-verb --event SessionEnd --an-older-flag", "Anything")
	for _, c := range []struct {
		name    string
		upkeep  upkeep
		changed bool
	}{{"keeping", keepingOne, false}, {"repairing", repairing, true}} {
		t.Run(c.name, func(t *testing.T) {
			family := toyFamily(c.upkeep, hookSpec{event: "SessionEnd"})
			hooks := map[string]interface{}{"SessionEnd": list(old)}

			if got := family.merge(hooks, toyBinary); got != c.changed {
				t.Fatalf("merge() = %v, want %v", got, c.changed)
			}
			kept := reflect.DeepEqual(hooks["SessionEnd"], list(old))
			if kept == c.changed {
				t.Errorf("SessionEnd = %v: the older entry kept = %v, want %v", hooks["SessionEnd"], kept, !c.changed)
			}
		})
	}
}

func TestKeepingOneDoesNotTakeAForeignEntryForOurs(t *testing.T) {
	family := toyFamily(keepingOne, hookSpec{event: "PreToolUse", matcher: "Agent"})
	sameBinaryOtherVerb := command(toyBinary+" other-verb", "Agent")
	otherBinarySameVerb := command("/somewhere/else toy-verb --event PreToolUse", "Agent")
	hooks := map[string]interface{}{"PreToolUse": list(sameBinaryOtherVerb, otherBinarySameVerb)}

	if !family.merge(hooks, toyBinary) {
		t.Fatal("merge() reported no change although neither entry is ours")
	}
	if got := hooks["PreToolUse"].([]interface{}); len(got) != 3 || !reflect.DeepEqual(got[0], sameBinaryOtherVerb) || !reflect.DeepEqual(got[1], otherBinarySameVerb) {
		t.Errorf("PreToolUse = %v, want the two foreign entries untouched and ours after them", got)
	}
}

// A hook list that is not an array cannot hold hooks: it is replaced, as every family does.
func TestKeepingOneReplacesAHookListThatIsNotAnArray(t *testing.T) {
	family := toyFamily(keepingOne, hookSpec{event: "SessionEnd"})
	for _, value := range []interface{}{"x", 3.0, nil, map[string]interface{}{"a": 1.0}} {
		hooks := map[string]interface{}{"SessionEnd": value}
		if !family.merge(hooks, toyBinary) {
			t.Errorf("SessionEnd = %v: merge() reported no change", value)
		}
		if got, ok := hooks["SessionEnd"].([]interface{}); !ok || len(got) != 1 {
			t.Errorf("SessionEnd = %v, want a list holding our entry", hooks["SessionEnd"])
		}
	}
}

func TestKeepingOnePerMatcherNeedsAnOwnedEntryWithTheSpecsMatcher(t *testing.T) {
	bash := hookSpec{event: "PreToolUse", matcher: "Bash"}
	files := hookSpec{event: "PreToolUse", matcher: "Write|Edit"}
	family := toyFamily(keepingOnePerMatcher, bash, files)
	bashEntry := family.build(toyBinary, bash)

	for _, c := range []struct {
		name   string
		before []interface{}
		added  []hookSpec
	}{
		{"none", nil, []hookSpec{bash, files}},
		{"one matcher present", list(bashEntry), []hookSpec{files}},
		{"both present", list(bashEntry, family.build(toyBinary, files)), nil},
		{"an owned entry with another matcher", list(command(toyBinary+" toy-verb", "Task")), []hookSpec{bash, files}},
		{"an owned entry with no matcher", list(command(toyBinary + " toy-verb")), []hookSpec{bash, files}},
		{"an owned entry whose matcher is not a string", list(command(toyBinary+" toy-verb", 7.0)), []hookSpec{bash, files}},
	} {
		t.Run(c.name, func(t *testing.T) {
			hooks := map[string]interface{}{"PreToolUse": c.before}
			changed := family.merge(hooks, toyBinary)

			want := append([]interface{}{}, c.before...)
			for _, s := range c.added {
				want = append(want, family.build(toyBinary, s))
			}
			if changed != (len(c.added) > 0) {
				t.Errorf("merge() = %v with %d entries to add", changed, len(c.added))
			}
			got, _ := hooks["PreToolUse"].([]interface{})
			if len(got) != len(want) || (len(want) > 0 && !reflect.DeepEqual(got, want)) {
				t.Errorf("PreToolUse = %v, want %v", got, want)
			}
		})
	}
}

func TestAFamilyWithSeveralEventsAppendsToEachInSpecOrder(t *testing.T) {
	family := toyFamily(keepingOne, hookSpec{event: "UserPromptSubmit"}, hookSpec{event: "PreToolUse", matcher: "Agent"})
	hooks := map[string]interface{}{}

	if !family.merge(hooks, toyBinary) {
		t.Fatal("merge() reported no change")
	}
	for _, event := range []string{"UserPromptSubmit", "PreToolUse"} {
		if got, ok := hooks[event].([]interface{}); !ok || len(got) != 1 {
			t.Errorf("%s = %v, want one entry", event, hooks[event])
		}
	}
}
