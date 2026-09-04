package register

import (
	"strings"
	"testing"
)

// assertHelperErrorAttribution pins this package's error-attribution
// convention on the format-level helpers (Splice/Remove/TOMLSplice/
// TOMLRemove/WriteMember/RemoveMember/WriteTOMLSection/RemoveTOMLSection
// and the install-state file helpers): they are shared verbatim by the
// install and the uninstall direction, so they name the FILE and the
// FAILURE and never a command word. The one per-target wrap in each
// direction (writer.go) supplies "register:" or "unregister:", so the line
// a user reads names the command they actually ran, exactly once.
//
// This replaces an assertion that merely required the string "register:"
// somewhere in the message. That check was weaker in both directions at
// once. It passed on a message that named the package and nothing else --
// no path, no cause -- so it never protected the thing it was written to
// protect. And it actively enforced the defect: because the package name
// and the subcommand name are the same word, an error from a helper
// reached the user through `unregister` reading "longterm-mem: register:
// claude: register: read ...", naming a command that was never run, twice.
func assertHelperErrorAttribution(t *testing.T, err error, mustName ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	got := err.Error()
	for _, needle := range mustName {
		if !strings.Contains(got, needle) {
			t.Fatalf("error %q does not name %q, so it cannot tell the user what failed or where", got, needle)
		}
	}
	for _, commandWord := range []string{"register:", "unregister:"} {
		if strings.HasPrefix(got, commandWord) {
			t.Fatalf("error %q begins with the command word %q; a helper shared by both directions must not name a command -- its caller does, and stacking the two is what printed %q for a run of `unregister`", got, commandWord, "register: claude: register: read ...")
		}
	}
}
