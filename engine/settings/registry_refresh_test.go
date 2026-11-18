package settings

// The registry refresh family: the one UserPromptSubmit entry that runs 'registry refresh' so the table of skills
// of the project's registry is kept current without gentle-ai (labdrian-orchestrator-opencode T30 commit d, S17). It is
// owned, repaired and removed as a unit like every other family; the rules every family keeps are held by
// legacy_families_test.go over namedFamilies, and these tests hold what is this family's own.

import (
	"reflect"
	"strings"
	"testing"
)

// The entry never blocks a prompt: a missing binary is a no-op and the exit is always 0. The project is the one Claude
// Code names, with the working directory as the fallback, and --quiet keeps the hook silent.
func TestRegistryRefreshFamilyWritesOneQuietUserPromptSubmitEntryThatNeverBlocks(t *testing.T) {
	want := map[string][]interface{}{"UserPromptSubmit": {map[string]interface{}{
		"hooks": []interface{}{map[string]interface{}{
			"type":    "command",
			"command": "command -v " + toyBinary + " >/dev/null 2>&1 && " + toyBinary + ` registry refresh --cwd "${CLAUDE_PROJECT_DIR:-$PWD}" --quiet --skip-if-busy || true`,
		}},
	}}}

	if got := RegistryRefreshHookEntries(toyBinary); !reflect.DeepEqual(got, want) {
		t.Errorf("RegistryRefreshHookEntries() = %v, want %v", got, want)
	}
}

// No identity is a piece of another, because the entries are told apart by the binary and the identity alone.
func TestRegistryRefreshIdentityIsItsOwn(t *testing.T) {
	ids := []string{
		LabdrianRegistryRefreshIdentity, LabdrianBuilderGuardIdentity, LabdrianReviewStopIdentity, LabdrianApproveGuardIdentity,
		LabdrianProjectionIdentity, LabdrianShaperGuardIdentity, LabdrianReviewReceiptIdentity, LabdrianSyncTriggerIdentity,
		LabdrianMinimalismIdentity, LabdrianDesignIdentity,
	}
	for i, a := range ids {
		for j, b := range ids {
			if i != j && strings.Contains(a, b) {
				t.Errorf("identity %q contains identity %q", a, b)
			}
		}
	}
}

// What is missing or has drifted is named, in the words of every family.
func TestMissingRegistryRefreshHookPartsNamesWhatIsMissingOrDrifted(t *testing.T) {
	entry := RegistryRefreshHookEntries(toyBinary)["UserPromptSubmit"][0]
	older := promptEntry(toyBinary + " registry refresh --cwd .")
	for _, tc := range []struct {
		name string
		root map[string]interface{}
		want []string
	}{
		{"in place", promptRoot(entry), nil},
		{"missing", promptRoot(), []string{"UserPromptSubmit registry refresh"}},
		{"a command that is not the one this version writes", promptRoot(older), []string{"UserPromptSubmit registry refresh", "1 drifted or duplicate UserPromptSubmit registry refresh entries"}},
		{"the entry twice", promptRoot(entry, entry), []string{"1 drifted or duplicate UserPromptSubmit registry refresh entries"}},
		{"hooks that are not an object", map[string]interface{}{"hooks": "none"}, []string{"UserPromptSubmit registry refresh"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := MissingRegistryRefreshHookParts(tc.root, toyBinary)

			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("MissingRegistryRefreshHookParts() = %q, want %q", got, tc.want)
			}
			if has := HasRegistryRefreshHook(tc.root, toyBinary); has != (tc.want == nil) {
				t.Errorf("HasRegistryRefreshHook() = %v, want %v", has, tc.want == nil)
			}
		})
	}
}

// An empty hook command claims nothing: there is no binary to tell the entry from a foreign one that mentions the identity.
func TestRegistryRefreshHookWithAnEmptyHookCommandClaimsNothing(t *testing.T) {
	foreign := promptEntry("/opt/other/tool registry refresh")

	parts := MissingRegistryRefreshHookParts(promptRoot(foreign), "")

	if want := []string{"UserPromptSubmit registry refresh"}; !reflect.DeepEqual(parts, want) {
		t.Errorf("MissingRegistryRefreshHookParts() = %q, want %q: the foreign entry is none of ours, drifted or not", parts, want)
	}
}

// Installing puts the entry after the others on UserPromptSubmit and touches no foreign entry, the refresh of gentle-ai
// and the wrapper of the codegraph prompt hook included; installing again adds nothing; removing takes exactly the entry
// off and leaves every foreign one.
func TestTheRegistryRefreshEntryIsInstalledOnceAndRemovedAloneBesideForeignPromptHooks(t *testing.T) {
	gentle := promptEntry(`gentle-ai skill-registry refresh --cwd "$CLAUDE_PROJECT_DIR" || true`)
	codegraph := promptEntry("codegraph prompt-hook")
	root := promptRoot(gentle, codegraph)

	if !mergeHooks(root, toyBinary) {
		t.Fatal("mergeHooks() = false on a settings file with none of our hooks")
	}
	list := root["hooks"].(map[string]interface{})["UserPromptSubmit"].([]interface{})
	if list[0] == nil || !reflect.DeepEqual(list[0], gentle) || !reflect.DeepEqual(list[1], codegraph) {
		t.Errorf("the foreign UserPromptSubmit entries moved or changed: %v", list[:2])
	}
	if !HasRegistryRefreshHook(root, toyBinary) {
		t.Error("HasRegistryRefreshHook() = false after mergeHooks()")
	}
	before := len(list)
	mergeHooks(root, toyBinary)
	if after := len(root["hooks"].(map[string]interface{})["UserPromptSubmit"].([]interface{})); after != before {
		t.Errorf("a second mergeHooks() changed the UserPromptSubmit list from %d to %d entries", before, after)
	}

	removeHooks(root, toyBinary)

	left := root["hooks"].(map[string]interface{})["UserPromptSubmit"]
	if want := []interface{}{gentle, codegraph}; !reflect.DeepEqual(left, want) {
		t.Errorf("UserPromptSubmit after removeHooks() = %v, want only the foreign entries %v", left, want)
	}
}

func promptEntry(command string) interface{} {
	return map[string]interface{}{"hooks": []interface{}{map[string]interface{}{"type": "command", "command": command}}}
}

// promptRoot is a settings object whose UserPromptSubmit list holds the entries given.
func promptRoot(entries ...interface{}) map[string]interface{} {
	list := make([]interface{}, 0, len(entries))
	list = append(list, entries...)
	return map[string]interface{}{"hooks": map[string]interface{}{"UserPromptSubmit": list}}
}
