package capability

import (
	"strings"
	"testing"
)

// C23 M12b decision 3: the public declaration upgrades only the bounded MCP gate.
// Evidence and availability retain their separate statuses in the M11 process table.
func TestCodexMemoryEnforcementClaimIsPartialAndBounded(t *testing.T) {
	d, err := Declare(TargetCodex)
	if err != nil {
		t.Fatal(err)
	}
	var claim Claim
	for _, c := range d.Claims {
		if c.Capability == MemoryEnforcement {
			claim = c
		}
	}
	if claim.Status != Partial {
		t.Errorf("Codex memory enforcement = %s, want partial", claim.Status)
	}
	for _, ref := range []string{"cmd:TestMemoryWiringAcceptance", "cmd:TestCodexSourceProcesses"} {
		found := false
		for _, got := range claim.Tests {
			found = found || got == ref
		}
		if !found {
			t.Errorf("Codex claim has no process proof %q: %v", ref, claim.Tests)
		}
	}
	for _, want := range []string{
		"PreToolUse", "MCP Engram reads", "longterm-mem query/get", "plan sources",
		"project", "goal", "--source codex", "trust each handler", "fails open",
		"no procedural skills, Bash or apply_patch enforcement", "no project check for reads by id",
		"agent_id ignored", "successful MCP reads only", "not live validation",
	} {
		if !strings.Contains(claim.Detail, want) {
			t.Errorf("Codex detail %q does not say %q", claim.Detail, want)
		}
	}
	if len(claim.Detail) > 512 {
		t.Errorf("Codex detail is %d bytes, over the 512-byte cap", len(claim.Detail))
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
