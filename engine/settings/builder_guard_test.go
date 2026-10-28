package settings

// The builder guard family: the one PreToolUse entry on the Agent tool that runs 'roles builder-guard' (own-rdd T11d, S14
// g). It is owned, repaired and removed as a unit like every other family; the rules every family keeps are held by
// legacy_families_test.go over namedFamilies, and these tests hold what is this family's own, above all that it shares
// the Agent matcher with the minimalism and design entries without claiming, repairing or removing any of them.

import (
	"reflect"
	"strings"
	"testing"
)

// The entry is a PreToolUse entry on the Agent tool, and its command is the one of every family that answers by output,
// which never blocks: a missing binary is a no-op and the exit is always 0.
func TestBuilderGuardFamilyWritesOneAgentEntryThatNeverBlocks(t *testing.T) {
	want := map[string][]interface{}{"PreToolUse": {map[string]interface{}{
		"matcher": "Agent",
		"hooks": []interface{}{map[string]interface{}{
			"type":    "command",
			"command": "command -v " + toyBinary + " >/dev/null 2>&1 && " + toyBinary + " roles builder-guard || true",
		}},
	}}}

	if got := BuilderGuardEntries(toyBinary); !reflect.DeepEqual(got, want) {
		t.Errorf("BuilderGuardEntries() = %v, want %v", got, want)
	}
}

// No identity is a piece of another, because the entries are told apart by the binary and the identity alone.
func TestBuilderGuardIdentityIsItsOwn(t *testing.T) {
	ids := []string{
		LabdrianBuilderGuardIdentity, LabdrianReviewStopIdentity, LabdrianApproveGuardIdentity, LabdrianProjectionIdentity,
		LabdrianShaperGuardIdentity, LabdrianReviewReceiptIdentity, LabdrianSyncTriggerIdentity, LabdrianMinimalismIdentity,
		LabdrianDesignIdentity,
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
// expects hold none of the family. An entry on another matcher is not the entry of the Agent tool.
func TestMissingBuilderGuardPartsNamesWhatIsMissingOrDrifted(t *testing.T) {
	entry := BuilderGuardEntries(toyBinary)["PreToolUse"][0]
	command := func(matcher, c string) interface{} {
		return map[string]interface{}{"matcher": matcher, "hooks": []interface{}{map[string]interface{}{"type": "command", "command": c}}}
	}
	older := command("Agent", toyBinary+" roles builder-guard")
	wrongMatcher := command("Bash", entry.(map[string]interface{})["hooks"].([]interface{})[0].(map[string]interface{})["command"].(string))
	const missing = `PreToolUse matcher="Agent" roles builder-guard`
	const drifted = `1 drifted or duplicate PreToolUse roles builder-guard entries`
	for _, tc := range []struct {
		name string
		root map[string]interface{}
		want []string
	}{
		{"in place", preToolUseRoot(entry), nil},
		{"missing", preToolUseRoot(), []string{missing}},
		{"a command that is not the one this version writes", preToolUseRoot(older), []string{missing, drifted}},
		{"the entry twice", preToolUseRoot(entry, entry), []string{drifted}},
		{"the entry on another matcher", preToolUseRoot(wrongMatcher), []string{missing, drifted}},
		{"hooks that are not an object", map[string]interface{}{"hooks": "none"}, []string{missing}},
		{"a PreToolUse list that is not an array", map[string]interface{}{"hooks": map[string]interface{}{"PreToolUse": "none"}}, []string{missing}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := MissingBuilderGuardParts(tc.root, toyBinary)

			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("MissingBuilderGuardParts() = %q, want %q", got, tc.want)
			}
			if has := HasBuilderGuard(tc.root, toyBinary); has != (tc.want == nil) {
				t.Errorf("HasBuilderGuard() = %v, want %v", has, tc.want == nil)
			}
		})
	}
}

// An empty hook command claims nothing: there is no binary to tell the entry from a foreign one that mentions the identity.
func TestBuilderGuardWithAnEmptyHookCommandClaimsNothing(t *testing.T) {
	foreign := map[string]interface{}{"matcher": "Agent", "hooks": []interface{}{map[string]interface{}{"type": "command", "command": "/opt/other/tool roles builder-guard"}}}

	parts := MissingBuilderGuardParts(preToolUseRoot(foreign), "")

	if want := []string{`PreToolUse matcher="Agent" roles builder-guard`}; !reflect.DeepEqual(parts, want) {
		t.Errorf("MissingBuilderGuardParts() = %q, want %q: the foreign entry is none of ours, drifted or not", parts, want)
	}
}

// The guard shares the Agent matcher with the entries of gate-task. Merging the minimalism and design families beside it
// leaves each family with its own entry and nothing missing or drifted, and the guard's entry is the last of the list.
func TestBuilderGuardSharesTheAgentMatcherWithTheGateEntriesWithoutClaimingThem(t *testing.T) {
	hooks := map[string]interface{}{}
	foreign := map[string]interface{}{"matcher": "Agent", "hooks": []interface{}{map[string]interface{}{"type": "command", "command": "other-tool agent-hook"}}}
	hooks["PreToolUse"] = []interface{}{foreign}

	for _, family := range installedFamilies {
		family.merge(hooks, toyBinary)
	}

	list := hooks["PreToolUse"].([]interface{})
	var agents []interface{}
	for _, e := range list {
		if e.(map[string]interface{})["matcher"] == "Agent" {
			agents = append(agents, e)
		}
	}
	if len(agents) != 4 {
		t.Fatalf("%d Agent entries after the merge, want the foreign one and the three of minimalism, design and the builder guard: %v", len(agents), agents)
	}
	if !reflect.DeepEqual(agents[0], interface{}(foreign)) {
		t.Errorf("the foreign Agent entry is %v, want it as it was", agents[0])
	}
	if last := list[len(list)-1]; !builderGuardFamily.owns(last, toyBinary) {
		t.Errorf("the last PreToolUse entry is %v, want the builder guard's: it is appended last to installedFamilies", last)
	}
	for name, family := range map[string]hookFamily{"minimalism": minimalismFamily, "design": designFamily} {
		owned := 0
		for _, e := range agents {
			if family.owns(e, toyBinary) {
				owned++
			}
		}
		if owned != 1 {
			t.Errorf("the %s family owns %d Agent entries, want 1", name, owned)
		}
	}
	if parts := MissingBuilderGuardParts(map[string]interface{}{"hooks": hooks}, toyBinary); parts != nil {
		t.Errorf("MissingBuilderGuardParts() = %q after a merge, want none", parts)
	}
}

// Removing the hooks takes the guard's entry off the PreToolUse list with the others of the overlay, and leaves a foreign
// entry on the same Agent matcher, and an entry that runs our binary with another verb, as they were.
func TestRemoveHooksTakesTheBuilderGuardOffTheAgentMatcherAndNothingForeign(t *testing.T) {
	foreign := map[string]interface{}{"matcher": "Agent", "hooks": []interface{}{map[string]interface{}{"type": "command", "command": "other-tool agent-hook"}}}
	otherVerb := map[string]interface{}{"matcher": "Agent", "hooks": []interface{}{map[string]interface{}{"type": "command", "command": toyBinary + " roles other-verb"}}}
	guard := BuilderGuardEntries(toyBinary)["PreToolUse"][0]
	root := preToolUseRoot(foreign, guard, otherVerb)

	changed := removeHooks(root, toyBinary)

	if !changed {
		t.Error("removeHooks() = false, want true: the entry of the builder guard was removed")
	}
	if got, want := root["hooks"].(map[string]interface{})["PreToolUse"], []interface{}{foreign, otherVerb}; !reflect.DeepEqual(got, want) {
		t.Errorf("the PreToolUse list is %v, want the entries that are not the builder guard's: %v", got, want)
	}
}

// preToolUseRoot is a settings object whose PreToolUse list holds the entries given.
func preToolUseRoot(entries ...interface{}) map[string]interface{} {
	list := make([]interface{}, 0, len(entries))
	list = append(list, entries...)
	return map[string]interface{}{"hooks": map[string]interface{}{"PreToolUse": list}}
}
