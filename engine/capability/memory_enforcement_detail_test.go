package capability

import (
	"strings"
	"testing"
)

// Codex receives the memory plan as instructions through a SessionStart hook (C23 M12), so its
// declaration must not say that nothing is projected; it still enforces nothing, and says so.
func TestCodexMemoryEnforcementDetailSaysTheHookProjectsAndNothingIsEnforced(t *testing.T) {
	got := memoryEnforcementDetail("Codex", " scope")
	for _, want := range []string{"SessionStart hook", "trusts it", "no hook gates a read", "not by a live session"} {
		if !strings.Contains(got, want) {
			t.Errorf("Codex detail %q does not say %q", got, want)
		}
	}
	if strings.Contains(got, "no memory plan is projected") {
		t.Errorf("Codex detail still says nothing is projected: %q", got)
	}
	if len(got) > 512 {
		t.Errorf("Codex detail is %d bytes, over the 512-byte cap", len(got))
	}
}

// Every other declared-only runtime keeps the text it had.
func TestOtherDeclaredOnlyRuntimesKeepTheMemoryEnforcementDetail(t *testing.T) {
	for _, runtime := range []string{"Pi", "OpenCode"} {
		want := "Not implemented: no memory plan is projected into or enforced in a " + runtime + " session. scope"
		if got := memoryEnforcementDetail(runtime, " scope"); got != want {
			t.Errorf("%s detail = %q, want %q", runtime, got, want)
		}
	}
}
