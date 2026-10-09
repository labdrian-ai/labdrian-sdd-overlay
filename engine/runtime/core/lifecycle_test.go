package core_test

import (
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

func TestCapabilityStatusValuesAreStable(t *testing.T) {
	tests := []struct {
		name string
		got  core.CapabilityStatus
		want string
	}{
		{name: "supported", got: core.CapabilitySupported, want: "supported"},
		{name: "partial", got: core.CapabilityPartial, want: "partial"},
		{name: "unsupported", got: core.CapabilityUnsupported, want: "unsupported"},
		{name: "restart required", got: core.CapabilityRestartRequired, want: "restart_required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.got) != tt.want {
				t.Fatalf("status = %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestLifecycleResultRendersTargetStatusAndMessage(t *testing.T) {
	result := core.LifecycleResult{
		Target:  core.TargetClaude,
		Action:  core.ActionStatus,
		Status:  core.CapabilitySupported,
		Message: "Claude hooks are the deterministic baseline",
	}

	got := result.String()
	for _, want := range []string{"claude", "status", "supported", "deterministic"} {
		if !strings.Contains(got, want) {
			t.Fatalf("LifecycleResult.String() should contain %q; got %q", want, got)
		}
	}
}
