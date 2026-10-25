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
	} {
		if got := CommandOwnedBy(c.command, c.binary, identity); got != c.want {
			t.Errorf("%s: CommandOwnedBy(%q, %q) = %t, want %t", c.name, c.command, c.binary, got, c.want)
		}
	}
}

// The status helper and the family read one entry the same way: whatever HasLabdrianOwnedHook
// answers for an event's entries is what the family's carries answers for them.
func TestHasLabdrianOwnedHookAgreesWithTheFamilyRecognizer(t *testing.T) {
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
		}
	}
}
