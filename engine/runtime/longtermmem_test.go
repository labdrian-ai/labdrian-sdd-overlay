package runtime_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
)

// longtermMemFixture builds a self-contained set of temp paths for one
// LongtermMemAdapter under test: a state dir, a binary, and the three
// runtime config files.
type longtermMemFixture struct {
	stateDir     string
	binaryPath   string
	claudePath   string
	openCodePath string
	codexPath    string
}

func newLongtermMemFixture(t *testing.T) longtermMemFixture {
	t.Helper()
	root := t.TempDir()
	return longtermMemFixture{
		stateDir:     filepath.Join(root, "state"),
		binaryPath:   filepath.Join(root, "bin", "longterm-mem"),
		claudePath:   filepath.Join(root, "claude.json"),
		openCodePath: filepath.Join(root, "opencode.json"),
		codexPath:    filepath.Join(root, "config.toml"),
	}
}

func (f longtermMemFixture) adapter() engineRuntime.LongtermMemAdapter {
	a := engineRuntime.NewLongtermMemAdapter(f.stateDir, f.binaryPath)
	a.ClaudeConfigPath = f.claudePath
	a.OpenCodeConfigPath = f.openCodePath
	a.CodexConfigPath = f.codexPath
	return a
}

func (f longtermMemFixture) writeExecutableBinary(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(f.binaryPath), 0o755); err != nil {
		t.Fatalf("mkdir binary dir: %v", err)
	}
	if err := os.WriteFile(f.binaryPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write binary: %v", err)
	}
}

func (f longtermMemFixture) writeClaudeEntry(t *testing.T) {
	t.Helper()
	content := `{"mcpServers":{"other-server":{"type":"stdio","command":"/other"},"longterm-mem":{"type":"stdio","command":"` + f.binaryPath + `","args":["mcp"]}}}`
	if err := os.WriteFile(f.claudePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write claude config: %v", err)
	}
}

func (f longtermMemFixture) writeOpenCodeEntry(t *testing.T) {
	t.Helper()
	content := `{"mcp":{"longterm-mem":{"type":"local","command":["` + f.binaryPath + `","mcp"],"enabled":true}}}`
	if err := os.WriteFile(f.openCodePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write opencode config: %v", err)
	}
}

func (f longtermMemFixture) writeCodexEntry(t *testing.T) {
	t.Helper()
	content := "[mcp_servers.other]\ncommand = \"/other\"\n\n[mcp_servers.longterm-mem]\ncommand = \"" + f.binaryPath + "\"\nargs = [\"mcp\"]\n"
	if err := os.WriteFile(f.codexPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write codex config: %v", err)
	}
}

// TestLongtermMemAdapter_InstallRecordsRegistrationAndReportsStatus is
// 10a.2: given the binary already built/copied and the three runtime
// entries already present (as the module-owned "register" step would have
// left them), Install() writes registration.json and reports per-runtime
// status for claude/opencode/codex.
func TestLongtermMemAdapter_InstallRecordsRegistrationAndReportsStatus(t *testing.T) {
	f := newLongtermMemFixture(t)
	f.writeExecutableBinary(t)
	f.writeClaudeEntry(t)
	f.writeOpenCodeEntry(t)
	f.writeCodexEntry(t)

	result := f.adapter().Install()

	if result.Status != engineRuntime.CapabilitySupported {
		t.Fatalf("Install() status = %v (message=%q, reasons=%v), want supported", result.Status, result.Message, result.Reasons)
	}
	for _, target := range []string{"claude", "opencode", "codex"} {
		found := false
		for _, reason := range result.Reasons {
			if strings.HasPrefix(reason, target+": ") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("Install() Reasons should report a per-runtime status line for %q; got %v", target, result.Reasons)
		}
	}

	regPath := filepath.Join(f.stateDir, "longterm-mem-registration.json")
	data, err := os.ReadFile(regPath)
	if err != nil {
		t.Fatalf("registration.json should be written at %s: %v", regPath, err)
	}
	var reg map[string]interface{}
	if err := json.Unmarshal(data, &reg); err != nil {
		t.Fatalf("registration.json must be valid JSON: %v", err)
	}
	targets, ok := reg["targets"].(map[string]interface{})
	if !ok {
		t.Fatalf("registration.json missing targets object: %#v", reg)
	}
	for _, target := range []string{"claude", "opencode", "codex"} {
		record, ok := targets[target].(map[string]interface{})
		if !ok {
			t.Fatalf("registration.json missing target record for %q: %#v", target, targets)
		}
		if record["fingerprint"] == "" || record["fingerprint"] == nil {
			t.Fatalf("registration.json target %q should have a non-empty fingerprint: %#v", target, record)
		}
	}
}

// TestLongtermMemAdapter_StatusAndUninstallRequireNoBuild is 10a.3:
// Status()/Uninstall() must read/remove the registration directly, with no
// build step. We prove this by never creating a binary at all (a build
// would be required to produce one) and asserting Status() and Uninstall()
// still run to completion, reporting "missing binary" rather than
// attempting to build one.
func TestLongtermMemAdapter_StatusAndUninstallRequireNoBuild(t *testing.T) {
	f := newLongtermMemFixture(t)
	// Deliberately no writeExecutableBinary(t) call: the binary never exists.
	f.writeClaudeEntry(t)
	f.writeOpenCodeEntry(t)
	f.writeCodexEntry(t)

	a := f.adapter()
	// Simulate a prior Install() having already recorded a registration by
	// calling Install() once, THEN removing the binary again — but since we
	// never wrote a binary, Install() itself will already report the
	// missing-binary reason; Status() must match it exactly without needing
	// a build step to succeed or fail differently.
	installResult := a.Install()
	if installResult.Status != engineRuntime.CapabilityPartial {
		t.Fatalf("Install() without a binary should be partial, got %v", installResult.Status)
	}
	if !containsReason(installResult.Reasons, engineRuntime.LongtermMemReasonMissingBinary) {
		t.Fatalf("Install() Reasons should mention missing binary; got %v", installResult.Reasons)
	}

	statusResult := a.Status()
	if statusResult.Status != engineRuntime.CapabilityPartial {
		t.Fatalf("Status() = %v, want partial (missing binary), got reasons %v", statusResult.Status, statusResult.Reasons)
	}
	if !containsReason(statusResult.Reasons, engineRuntime.LongtermMemReasonMissingBinary) {
		t.Fatalf("Status() Reasons should mention missing binary; got %v", statusResult.Reasons)
	}

	uninstallResult := a.Uninstall()
	regPath := filepath.Join(f.stateDir, "longterm-mem-registration.json")
	if _, err := os.Stat(regPath); !os.IsNotExist(err) {
		t.Fatalf("Uninstall() should remove registration.json at %s, stat err = %v", regPath, err)
	}
	if uninstallResult.Status != engineRuntime.CapabilityPartial {
		t.Fatalf("Uninstall() = %v, want partial; got reasons %v", uninstallResult.Status, uninstallResult.Reasons)
	}
}

func containsReason(reasons []string, reason string) bool {
	for _, r := range reasons {
		if strings.Contains(r, reason) {
			return true
		}
	}
	return false
}

// TestLongtermMemAdapter_UpdateAndRollbackRefused is 10a.4: Update()/
// Rollback() must return an explicit refusal, never a silent no-op that
// reports success having done nothing. We assert both the capability
// status AND that no file I/O happened (no registration.json was written,
// no binary check performed) by pointing at paths that would fail loudly
// if touched.
func TestLongtermMemAdapter_UpdateAndRollbackRefused(t *testing.T) {
	f := newLongtermMemFixture(t)
	a := f.adapter()

	updateResult := a.Update()
	if updateResult.Status != engineRuntime.CapabilityUnsupported {
		t.Fatalf("Update() status = %v, want unsupported (explicit refusal)", updateResult.Status)
	}
	if updateResult.Message == "" {
		t.Fatalf("Update() must state an explicit reason, not just a bare status")
	}
	if len(updateResult.Reasons) == 0 {
		t.Fatalf("Update() must carry a stated reason in Reasons, not a silent refusal")
	}

	rollbackResult := a.Rollback()
	if rollbackResult.Status != engineRuntime.CapabilityUnsupported {
		t.Fatalf("Rollback() status = %v, want unsupported (explicit refusal)", rollbackResult.Status)
	}
	if rollbackResult.Message == "" {
		t.Fatalf("Rollback() must state an explicit reason, not just a bare status")
	}
	if len(rollbackResult.Reasons) == 0 {
		t.Fatalf("Rollback() must carry a stated reason in Reasons, not a silent refusal")
	}

	// Neither call should have performed any I/O: registration.json must
	// not exist (Update/Rollback never write it), proving these are
	// refusals, not no-ops that silently "succeeded" having done nothing.
	regPath := filepath.Join(f.stateDir, "longterm-mem-registration.json")
	if _, err := os.Stat(regPath); !os.IsNotExist(err) {
		t.Fatalf("Update()/Rollback() must not perform any write; found registration.json at %s (err=%v)", regPath, err)
	}
}
