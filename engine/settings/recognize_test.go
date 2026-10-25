package settings

import "testing"

func TestCommandOwnedByNeedsTheBinaryBareOrQuotedAndTheIdentity(t *testing.T) {
	const identity = LabdrianProjectionIdentity
	for _, c := range []struct {
		name, command, binary string
		want                  bool
	}{
		{"bare path and identity", "/opt/bin/overlay projection hook --event SessionStart", "/opt/bin/overlay", true},
		{"quoted path and identity", `'/p/it'\''s/overlay' projection hook --event SessionStart`, "/p/it's/overlay", true},
		{"identity without the binary", "/other/bin/tool projection hook", "/opt/bin/overlay", false},
		{"binary without the identity", "/opt/bin/overlay gate-task", "/opt/bin/overlay", false},
		{"empty command", "", "/opt/bin/overlay", false},
		{"empty binary path, which every command contains", "/opt/bin/overlay projection hook", "", false},
	} {
		if got := CommandOwnedBy(c.command, c.binary, identity); got != c.want {
			t.Errorf("%s: CommandOwnedBy(%q, %q) = %t, want %t", c.name, c.command, c.binary, got, c.want)
		}
	}
}

// An empty word is mentioned by no command, so a query or a status check handed an empty binary
// path finds nothing instead of answering on the family's token alone.
func TestAnEmptyWordIsMentionedByNoEntry(t *testing.T) {
	root := map[string]interface{}{"hooks": map[string]interface{}{
		"SessionEnd": []interface{}{map[string]interface{}{"hooks": []interface{}{
			map[string]interface{}{"command": "/opt/bin/overlay sync-trigger"}}}},
	}}
	if HasHookContaining(root, "SessionEnd", "", LabdrianSyncTriggerIdentity) {
		t.Error("HasHookContaining with an empty binary path matched on the token alone")
	}
	if HasLabdrianOwnedHook(root, "SessionEnd", "", LabdrianSyncTriggerIdentity) {
		t.Error("HasLabdrianOwnedHook with an empty binary path matched on the token alone")
	}
}

// Every family's written entries are recognized by HasLabdrianOwnedHook for the binary they were
// written for, quoted path included, and not for another binary or another family's token.
func TestHasLabdrianOwnedHookRecognizesEachFamilyForItsOwnBinaryAndTokenOnly(t *testing.T) {
	binary := "/p/it's/overlay"
	for _, f := range installedFamilies {
		for event, entries := range f.entries(binary) {
			root := map[string]interface{}{"hooks": map[string]interface{}{event: entries}}
			if !HasLabdrianOwnedHook(root, event, binary, f.identity) {
				t.Errorf("%s %s: written entries not recognized", f.identity, event)
			}
			if HasLabdrianOwnedHook(root, event, "/elsewhere/overlay", f.identity) {
				t.Errorf("%s %s: recognized for another binary", f.identity, event)
			}
			if HasLabdrianOwnedHook(root, event, binary, "no-such-family-token") {
				t.Errorf("%s %s: recognized for another family's token", f.identity, event)
			}
		}
	}
}
