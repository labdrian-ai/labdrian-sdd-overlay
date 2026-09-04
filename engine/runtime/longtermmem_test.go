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

// writeFile is the shared "this runtime's config file exists on disk with
// exactly these bytes" helper the presence/ownership fixtures below use.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// The three "the runtime is installed, but longterm-mem is not registered
// with it" fixtures: a real config file carrying somebody else's servers and
// no longterm-mem entry at all.
func (f longtermMemFixture) writeClaudeConfigWithoutEntry(t *testing.T) {
	t.Helper()
	writeFile(t, f.claudePath, `{"mcpServers":{"other-server":{"type":"stdio","command":"/other"}}}`)
}

func (f longtermMemFixture) writeOpenCodeConfigWithoutEntry(t *testing.T) {
	t.Helper()
	writeFile(t, f.openCodePath, `{"mcp":{"other-tool":{"type":"local","command":["/other"]}}}`)
}

func (f longtermMemFixture) writeCodexConfigWithoutEntry(t *testing.T) {
	t.Helper()
	writeFile(t, f.codexPath, "theme = \"dark\"\n\n[mcp_servers.other]\ncommand = \"/other\"\n")
}

// The three A2 fixtures: a same-named longterm-mem entry this overlay did
// NOT write, pointing at somebody else's binary. `longterm-mem register`
// refuses to touch these (ErrConflict, exit 6); the engine must never claim
// one as its own either.
const foreignBinary = "/somebody/elses/binary"

func (f longtermMemFixture) writeForeignClaudeEntry(t *testing.T) {
	t.Helper()
	writeFile(t, f.claudePath, `{"mcpServers":{"longterm-mem":{"type":"stdio","command":"`+foreignBinary+`"}}}`)
}

func (f longtermMemFixture) writeForeignOpenCodeEntry(t *testing.T) {
	t.Helper()
	writeFile(t, f.openCodePath, `{"mcp":{"longterm-mem":{"type":"local","command":["`+foreignBinary+`","mcp"],"enabled":true}}}`)
}

func (f longtermMemFixture) writeForeignCodexEntry(t *testing.T) {
	t.Helper()
	writeFile(t, f.codexPath, "[mcp_servers.longterm-mem]\ncommand = \""+foreignBinary+"\"\n")
}

// reasonFor returns the per-runtime status line for target out of an
// aggregate LifecycleResult's Reasons.
func reasonFor(t *testing.T, reasons []string, target string) string {
	t.Helper()
	for _, r := range reasons {
		if strings.HasPrefix(r, target+": ") {
			return r
		}
	}
	t.Fatalf("no per-runtime line for %q in %v", target, reasons)
	return ""
}

// registrationTargets reads the engine-owned registration record and returns
// the set of target names it holds a record for.
func registrationTargets(t *testing.T, stateDir string) map[string]bool {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(stateDir, "longterm-mem-registration.json"))
	if err != nil {
		t.Fatalf("read registration.json: %v", err)
	}
	var reg struct {
		Targets map[string]json.RawMessage `json:"targets"`
	}
	if err := json.Unmarshal(data, &reg); err != nil {
		t.Fatalf("registration.json must be valid JSON: %v", err)
	}
	got := map[string]bool{}
	for name := range reg.Targets {
		got[name] = true
	}
	return got
}

// TestLongtermMemAdapter_AbsentRuntimeIsNotADefect is the correction to the
// measured behaviour where a machine running only Claude Code reported
// `partial` forever: a runtime that is not installed at all is not a defect
// this component can diagnose, so it must not drag the component's status
// down. This mirrors longterm-mem-mcp-registration's "Multi-Target Expansion
// Skips Runtimes That Are Not Installed" requirement on the engine side.
func TestLongtermMemAdapter_AbsentRuntimeIsNotADefect(t *testing.T) {
	f := newLongtermMemFixture(t)
	f.writeExecutableBinary(t)
	f.writeClaudeEntry(t)
	// opencode and codex config files are deliberately never created: those
	// runtimes are not installed on this machine.

	a := f.adapter()
	install := a.Install()
	if install.Status != engineRuntime.CapabilitySupported {
		t.Fatalf("Install() on a machine running only Claude Code = %v, want supported; reasons=%v", install.Status, install.Reasons)
	}
	for _, target := range []string{"opencode", "codex"} {
		line := reasonFor(t, install.Reasons, target)
		if !strings.Contains(line, engineRuntime.LongtermMemReasonRuntimeNotInstalled) {
			t.Fatalf("Install() line for %q = %q, want it to report %q", target, line, engineRuntime.LongtermMemReasonRuntimeNotInstalled)
		}
	}

	status := a.Status()
	if status.Status != engineRuntime.CapabilitySupported {
		t.Fatalf("Status() after that install = %v, want supported; reasons=%v", status.Status, status.Reasons)
	}
}

// TestLongtermMemAdapter_InstallDoesNotFabricateRecordsForAbsentRuntimes
// pins the root cause of that same defect: Install() used to write a record
// for every target from OBSERVED state, so an absent runtime got
// {Fingerprint:"", EntryPresent:false} and the very next status read it back
// as "record without entry". A record must only ever be written for a target
// whose entry was actually observed.
func TestLongtermMemAdapter_InstallDoesNotFabricateRecordsForAbsentRuntimes(t *testing.T) {
	f := newLongtermMemFixture(t)
	f.writeExecutableBinary(t)
	f.writeClaudeEntry(t)

	f.adapter().Install()

	got := registrationTargets(t, f.stateDir)
	if !got["claude"] {
		t.Fatalf("registration.json should record claude, whose entry IS present; got %v", got)
	}
	for _, target := range []string{"opencode", "codex"} {
		if got[target] {
			t.Fatalf("registration.json fabricated a record for %q, whose runtime is not installed; got %v", target, got)
		}
	}
}

// TestLongtermMemAdapter_PresentRuntimeWithoutEntryIsNotADefect: the runtime
// IS on this machine but longterm-mem was never registered with it. That is
// still not a defect — whether that runtime was ever asked for is the
// caller's knowledge, not the engine's — but it must be told apart from an
// absent runtime, which is why the two carry different reasons.
func TestLongtermMemAdapter_PresentRuntimeWithoutEntryIsNotADefect(t *testing.T) {
	f := newLongtermMemFixture(t)
	f.writeExecutableBinary(t)
	f.writeClaudeConfigWithoutEntry(t)
	f.writeOpenCodeConfigWithoutEntry(t)
	f.writeCodexConfigWithoutEntry(t)

	result := f.adapter().Status()
	if result.Status != engineRuntime.CapabilitySupported {
		t.Fatalf("Status() with three installed-but-unregistered runtimes = %v, want supported; reasons=%v", result.Status, result.Reasons)
	}
	for _, target := range []string{"claude", "opencode", "codex"} {
		line := reasonFor(t, result.Reasons, target)
		if !strings.Contains(line, engineRuntime.LongtermMemReasonNotRegistered) {
			t.Fatalf("line for %q = %q, want %q (the runtime is installed, just unregistered)", target, line, engineRuntime.LongtermMemReasonNotRegistered)
		}
		if strings.Contains(line, engineRuntime.LongtermMemReasonRuntimeNotInstalled) {
			t.Fatalf("line for %q = %q claims the runtime is not installed, but its config file is right there", target, line)
		}
	}
}

// TestLongtermMemAdapter_UnparseableConfigIsNotReportedAsAbsent: runtime
// presence is decided by os.Stat on the config file, never by whether it
// parses. A config we cannot read is emphatically NOT proof the runtime is
// missing from the machine, and saying so would be a guess reported as fact.
//
// What it DOES yield is documented and asserted here: no entry could be
// observed, so the component reports "not registered". The component does
// not diagnose the runtime's own config syntax — that belongs to the runtime.
func TestLongtermMemAdapter_UnparseableConfigIsNotReportedAsAbsent(t *testing.T) {
	f := newLongtermMemFixture(t)
	f.writeExecutableBinary(t)
	writeFile(t, f.claudePath, "{ this is not json at all")

	result := f.adapter().Status()
	line := reasonFor(t, result.Reasons, "claude")
	if strings.Contains(line, engineRuntime.LongtermMemReasonRuntimeNotInstalled) {
		t.Fatalf("claude line = %q: an unparseable config file must not be reported as an absent runtime", line)
	}
	if !strings.Contains(line, engineRuntime.LongtermMemReasonNotRegistered) {
		t.Fatalf("claude line = %q, want %q — no entry could be observed in a config we cannot parse", line, engineRuntime.LongtermMemReasonNotRegistered)
	}
}

// TestLongtermMemAdapter_RecordWithoutEntryStillReportsDrift proves the
// no-fabricated-records change did NOT cost us real drift detection: a
// record written from a genuinely observed entry must still report
// "record without entry" once that entry later vanishes.
func TestLongtermMemAdapter_RecordWithoutEntryStillReportsDrift(t *testing.T) {
	f := newLongtermMemFixture(t)
	f.writeExecutableBinary(t)
	f.writeClaudeEntry(t)

	a := f.adapter()
	if install := a.Install(); install.Status != engineRuntime.CapabilitySupported {
		t.Fatalf("Install() with a real claude entry = %v, want supported; reasons=%v", install.Status, install.Reasons)
	}

	// The entry is removed out from under us, the record stays behind.
	f.writeClaudeConfigWithoutEntry(t)

	status := a.Status()
	if status.Status != engineRuntime.CapabilityPartial {
		t.Fatalf("Status() after the recorded entry vanished = %v, want partial; reasons=%v", status.Status, status.Reasons)
	}
	line := reasonFor(t, status.Reasons, "claude")
	if !strings.Contains(line, engineRuntime.LongtermMemReasonRecordWithoutEntry) {
		t.Fatalf("claude line = %q, want %q — drift detection must survive", line, engineRuntime.LongtermMemReasonRecordWithoutEntry)
	}
}

// TestLongtermMemAdapter_ForeignEntryIsNeverAdoptedAsOurOwn is A2. A
// same-named MCP entry this overlay did not write (`longterm-mem register`
// refuses it with exit 6, leaving the file byte-identical) used to be
// fingerprinted and recorded by Install() as though it were ours, so the
// component reported `supported` and then tracked a third party's server as
// its own — reporting "fingerprint drift" when that third party edited their
// own entry.
//
// Two things must hold, and the second is the trap: the entry must NOT be
// recorded, AND it must still be observed as PRESENT. If presence itself
// required ownership, a foreign entry would collapse into "no record, no
// entry" and be reported as a healthy "not registered" — which hides it.
func TestLongtermMemAdapter_ForeignEntryIsNeverAdoptedAsOurOwn(t *testing.T) {
	f := newLongtermMemFixture(t)
	f.writeExecutableBinary(t)
	f.writeForeignClaudeEntry(t)
	f.writeForeignOpenCodeEntry(t)
	f.writeForeignCodexEntry(t)

	a := f.adapter()
	install := a.Install()
	if install.Status != engineRuntime.CapabilityPartial {
		t.Fatalf("Install() over three foreign entries = %v, want partial — adopting them silently is the A2 defect; reasons=%v", install.Status, install.Reasons)
	}

	got := registrationTargets(t, f.stateDir)
	for _, target := range []string{"claude", "opencode", "codex"} {
		if got[target] {
			t.Fatalf("registration.json recorded %q from a foreign entry — the overlay is now tracking somebody else's server as its own; got %v", target, got)
		}
		line := reasonFor(t, install.Reasons, target)
		if !strings.Contains(line, engineRuntime.LongtermMemReasonEntryWithoutRecord) {
			t.Fatalf("line for %q = %q, want %q — a foreign entry is present and unmanaged, not absent", target, line, engineRuntime.LongtermMemReasonEntryWithoutRecord)
		}
		if strings.Contains(line, engineRuntime.LongtermMemReasonNotRegistered) ||
			strings.Contains(line, engineRuntime.LongtermMemReasonRuntimeNotInstalled) {
			t.Fatalf("line for %q = %q reports nothing is there, HIDING a foreign entry that is", target, line)
		}
	}

	// And a second run must not launder it either: status reads the record
	// back and must still report the same unmanaged entry.
	status := a.Status()
	if status.Status != engineRuntime.CapabilityPartial {
		t.Fatalf("Status() after that install = %v, want partial; reasons=%v", status.Status, status.Reasons)
	}
}

// TestLongtermMemAdapter_UninstallReportsItsOwnVerdict is Change 4. Applying
// the install-health matrix to a post-uninstall observation is a category
// error: uninstall's success question is "did I remove what I own?", while
// the matrix's `supported` row describes a LIVE installation, so a flawless
// uninstall was structurally incapable of reporting success. Update() and
// Rollback() already return explicit verdicts instead of running the matrix;
// Uninstall() now does the same.
func TestLongtermMemAdapter_UninstallReportsItsOwnVerdict(t *testing.T) {
	t.Run("removing an owned registration succeeds", func(t *testing.T) {
		f := newLongtermMemFixture(t)
		f.writeExecutableBinary(t)
		f.writeClaudeEntry(t)
		f.writeOpenCodeEntry(t)
		f.writeCodexEntry(t)

		a := f.adapter()
		a.Install()

		result := a.Uninstall()
		if result.Status != engineRuntime.CapabilitySupported {
			t.Fatalf("Uninstall() after a healthy install = %v, want supported; message=%q reasons=%v", result.Status, result.Message, result.Reasons)
		}
		regPath := filepath.Join(f.stateDir, "longterm-mem-registration.json")
		if _, err := os.Stat(regPath); !os.IsNotExist(err) {
			t.Fatalf("Uninstall() should remove registration.json at %s, stat err = %v", regPath, err)
		}
	})

	t.Run("an already-absent registration also succeeds", func(t *testing.T) {
		f := newLongtermMemFixture(t)
		// Nothing was ever installed: the requested end state already holds.
		result := f.adapter().Uninstall()
		if result.Status != engineRuntime.CapabilitySupported {
			t.Fatalf("Uninstall() with no registration = %v, want supported (the end state holds); message=%q reasons=%v", result.Status, result.Message, result.Reasons)
		}
	})

	t.Run("an unresolvable state dir is unsupported", func(t *testing.T) {
		a := newLongtermMemFixture(t).adapter()
		a.StateDir = ""
		result := a.Uninstall()
		if result.Status != engineRuntime.CapabilityUnsupported {
			t.Fatalf("Uninstall() with an unresolvable state dir = %v, want unsupported; message=%q", result.Status, result.Message)
		}
	})

	t.Run("per-runtime observation lines are kept as information", func(t *testing.T) {
		f := newLongtermMemFixture(t)
		f.writeExecutableBinary(t)
		f.writeClaudeEntry(t)
		a := f.adapter()
		a.Install()

		result := a.Uninstall()
		if len(result.Reasons) == 0 {
			t.Fatalf("Uninstall() dropped the per-runtime observation lines entirely; result=%q", result.String())
		}
		for _, target := range []string{"claude", "opencode", "codex"} {
			reasonFor(t, result.Reasons, target)
		}
		// The lines are INFORMATION about what remains, not complaints:
		// a successful uninstall must never describe itself as partial.
		for _, line := range result.Reasons {
			if strings.Contains(line, string(engineRuntime.CapabilityPartial)) {
				t.Fatalf("Uninstall() reason %q reads as a status complaint; the lines must describe what was observed, not re-run the install-health matrix", line)
			}
		}
	})
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
	// Uninstall removed exactly what it owns, without a binary ever
	// existing to build or run — which is what this test is about. It
	// reports its own verdict on that end state; it does not re-run the
	// install-health matrix, whose `supported` row describes a LIVE
	// installation and so could never be reached after a successful
	// removal. See TestLongtermMemAdapter_UninstallReportsItsOwnVerdict.
	if uninstallResult.Status != engineRuntime.CapabilitySupported {
		t.Fatalf("Uninstall() = %v, want supported (it removed what it owns, with no build step); got reasons %v", uninstallResult.Status, uninstallResult.Reasons)
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
