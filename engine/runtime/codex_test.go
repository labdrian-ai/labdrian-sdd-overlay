package runtime_test

import (
	"strings"
	"testing"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
)

func TestCodexStatusSupportedOnlyWhenDeterministicScopedRewriteVerified(t *testing.T) {
	// R-001/R-104: Codex is supported only when scoped rewrite capability is verified.
	adapter := engineRuntime.NewCodexAdapter(engineRuntime.CodexCapabilities{
		RuntimeDetected:            true,
		DeterministicScopedRewrite: true,
		ScopedSubagentTaskLaunches: true,
		VerifiedRuntimeVersion:     "codex-test-1.0",
	})

	result := adapter.Status()
	if result.Status != engineRuntime.CapabilitySupported {
		t.Fatalf("Codex status = %q, want supported; message: %s", result.Status, result.Message)
	}
	if !strings.Contains(result.String(), "codex") || !strings.Contains(result.String(), "codex-test-1.0") {
		t.Fatalf("supported result should name codex and verified runtime, got %q", result.String())
	}
}

func TestCodexStatusPartialWhenRewriteIsUnverified(t *testing.T) {
	// R-001/R-104: unverified scoped rewrite produces partial status with reasons.
	adapter := engineRuntime.NewCodexAdapter(engineRuntime.CodexCapabilities{
		RuntimeDetected:            true,
		ScopedSubagentTaskLaunches: true,
	})

	result := adapter.SyncCheck()
	if result.Status != engineRuntime.CapabilityPartial {
		t.Fatalf("Codex sync-check = %q, want partial; message: %s", result.Status, result.Message)
	}
	for _, want := range []string{"deterministic scoped subagent/task rewrite", "not verified"} {
		if !strings.Contains(result.String(), want) {
			t.Errorf("partial result should explain missing %q; got %q", want, result.String())
		}
	}
}

func TestCodexStatusUnsupportedWhenRuntimeIsMissing(t *testing.T) {
	// R-104: missing Codex runtime reports unsupported instead of false green.
	adapter := engineRuntime.NewCodexAdapter(engineRuntime.CodexCapabilities{})

	result := adapter.Apply()
	if result.Status != engineRuntime.CapabilityUnsupported {
		t.Fatalf("Codex apply = %q, want unsupported; message: %s", result.Status, result.Message)
	}
	for _, want := range []string{"Codex runtime", "not detected"} {
		if !strings.Contains(result.String(), want) {
			t.Errorf("unsupported result should explain %q; got %q", want, result.String())
		}
	}
}

func TestLifecycleResultStringRendersReasonsOnce(t *testing.T) {
	// R-104: capability reasons render once for conditional runtime support.
	result := engineRuntime.NewLifecycleResult(
		engineRuntime.TargetCodex,
		engineRuntime.ActionStatus,
		engineRuntime.CapabilityPartial,
		"Codex support is conditional",
		[]string{"deterministic scoped subagent/task rewrite not verified"},
	)

	got := result.String()
	if strings.Count(got, "codex") != 1 {
		t.Fatalf("target should be rendered once, got %q", got)
	}
	if !strings.Contains(got, "reasons: deterministic scoped subagent/task rewrite not verified") {
		t.Fatalf("reasons should be normalized in lifecycle rendering, got %q", got)
	}
}

func TestCodexLifecycleAliasesEvaluateCapabilities(t *testing.T) {
	// R-104/R-105: all Codex lifecycle actions report the same honest capability evaluation.
	adapter := engineRuntime.NewCodexAdapter(engineRuntime.CodexCapabilities{
		RuntimeDetected:            true,
		DeterministicScopedRewrite: true,
		ScopedSubagentTaskLaunches: true,
		VerifiedRuntimeVersion:     "codex-test-2.0",
	})
	if adapter.Target() != engineRuntime.TargetCodex {
		t.Fatalf("Target() = %q, want codex", adapter.Target())
	}

	for _, result := range []engineRuntime.LifecycleResult{
		adapter.Install(), adapter.Update(), adapter.Rollback(), adapter.Uninstall(),
	} {
		if result.Target != engineRuntime.TargetCodex || result.Status != engineRuntime.CapabilitySupported {
			t.Fatalf("Codex lifecycle result = %#v, want codex supported", result)
		}
		if !strings.Contains(result.Message, "codex-test-2.0") {
			t.Fatalf("Codex lifecycle result should include verified runtime version, got %#v", result)
		}
	}
}
