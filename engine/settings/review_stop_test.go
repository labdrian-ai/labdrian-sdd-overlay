package settings

// The review stop-hook family: the one Stop entry that runs 'review stop-hook' (own-rdd T6a, S6, L18). It is owned,
// repaired and removed as a unit like every other family; the rules every family keeps are held by
// legacy_families_test.go over namedFamilies, and these tests hold what is this family's own.

import (
	"reflect"
	"strings"
	"testing"
)

// The entry is the one a Stop hook can be: it has no matcher, because Stop has none, and its command is the one of every
// family that answers by output, which never blocks: a missing binary is a no-op and the exit is always 0, since an exit
// of 2 blocks the stop of the agent.
func TestReviewStopFamilyWritesOneStopEntryThatNeverBlocks(t *testing.T) {
	want := map[string][]interface{}{"Stop": {map[string]interface{}{
		"hooks": []interface{}{map[string]interface{}{
			"type":    "command",
			"command": "command -v " + toyBinary + " >/dev/null 2>&1 && " + toyBinary + " review stop-hook || true",
		}},
	}}}

	if got := ReviewStopHookEntries(toyBinary); !reflect.DeepEqual(got, want) {
		t.Errorf("ReviewStopHookEntries() = %v, want %v", got, want)
	}
}

// No identity is a piece of another, because the entries are told apart by the binary and the identity alone.
func TestReviewStopIdentityIsItsOwn(t *testing.T) {
	ids := []string{
		LabdrianReviewStopIdentity, LabdrianApproveGuardIdentity, LabdrianProjectionIdentity, LabdrianShaperGuardIdentity,
		LabdrianReviewReceiptIdentity, LabdrianSyncTriggerIdentity, LabdrianMinimalismIdentity, LabdrianDesignIdentity,
	}
	for i, a := range ids {
		for j, b := range ids {
			if i != j && strings.Contains(a, b) {
				t.Errorf("identity %q contains identity %q", a, b)
			}
		}
	}
}

// What is missing or has drifted is named, in the words of every family, and settings that are not shaped as Claude Code
// expects hold none of the family.
func TestMissingReviewStopHookPartsNamesWhatIsMissingOrDrifted(t *testing.T) {
	entry := ReviewStopHookEntries(toyBinary)["Stop"][0]
	older := map[string]interface{}{"hooks": []interface{}{map[string]interface{}{"type": "command", "command": toyBinary + " review stop-hook"}}}
	for _, tc := range []struct {
		name string
		root map[string]interface{}
		want []string
	}{
		{"in place", stopRoot(entry), nil},
		{"missing", stopRoot(), []string{"Stop review stop-hook"}},
		{"a command that is not the one this version writes", stopRoot(older), []string{"Stop review stop-hook", "1 drifted or duplicate Stop review stop-hook entries"}},
		{"the entry twice", stopRoot(entry, entry), []string{"1 drifted or duplicate Stop review stop-hook entries"}},
		{"hooks that are not an object", map[string]interface{}{"hooks": "none"}, []string{"Stop review stop-hook"}},
		{"a Stop list that is not an array", map[string]interface{}{"hooks": map[string]interface{}{"Stop": "none"}}, []string{"Stop review stop-hook"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := MissingReviewStopHookParts(tc.root, toyBinary)

			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("MissingReviewStopHookParts() = %q, want %q", got, tc.want)
			}
			if has := HasReviewStopHook(tc.root, toyBinary); has != (tc.want == nil) {
				t.Errorf("HasReviewStopHook() = %v, want %v", has, tc.want == nil)
			}
		})
	}
}

// An empty hook command claims nothing: there is no binary to tell the entry from a foreign one that mentions the identity.
func TestReviewStopHookWithAnEmptyHookCommandClaimsNothing(t *testing.T) {
	foreign := map[string]interface{}{"hooks": []interface{}{map[string]interface{}{"type": "command", "command": "/opt/other/tool review stop-hook"}}}

	parts := MissingReviewStopHookParts(stopRoot(foreign), "")

	if want := []string{"Stop review stop-hook"}; !reflect.DeepEqual(parts, want) {
		t.Errorf("MissingReviewStopHookParts() = %q, want %q: the foreign entry is none of ours, drifted or not", parts, want)
	}
}

// Nothing but the review stop-hook was ever installed on Stop, so removing the hooks takes only its entry off that list: an
// entry of another family that an older version left there (the sync-trigger ran on Stop once) and the entries of other
// programs stay, as they have always stayed. Everywhere else an entry of any family is removed.
func TestRemoveHooksTakesOnlyTheReviewStopHookOffStop(t *testing.T) {
	command := func(c string) interface{} {
		return map[string]interface{}{"hooks": []interface{}{map[string]interface{}{"type": "command", "command": c}}}
	}
	older := command(toyBinary + " sync-trigger --event stop")
	foreign := command("moshi-hook stop")
	root := stopRoot(older, ReviewStopHookEntries(toyBinary)["Stop"][0], foreign)

	changed := removeHooks(root, toyBinary)

	if !changed {
		t.Error("removeHooks() = false, want true: the entry of the review stop-hook was removed")
	}
	if got, want := root["hooks"].(map[string]interface{})["Stop"], []interface{}{older, foreign}; !reflect.DeepEqual(got, want) {
		t.Errorf("the Stop list is %v, want the entries that are not the review stop-hook's: %v", got, want)
	}
}

// stopRoot is a settings object whose Stop list holds the entries given.
func stopRoot(entries ...interface{}) map[string]interface{} {
	stop := make([]interface{}, 0, len(entries))
	stop = append(stop, entries...)
	return map[string]interface{}{"hooks": map[string]interface{}{"Stop": stop}}
}
