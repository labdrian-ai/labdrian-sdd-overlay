package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/assets"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator"
)

// contractFrontmatter is a minimal valid contract used by tests.
const testContractContent = `---
applies_to_phases: [sdd-tasks, sdd-apply]
excluded_phases: [sdd-propose, sdd-spec, sdd-design, sdd-verify, sdd-archive]
injection_point: "## Skills to load before work"
---
# Minimalism Contract
`

// brokenContractContent has no YAML frontmatter.
const brokenContractContent = "no frontmatter here"

// validTaskInput is a minimal hook JSON for sdd-tasks using the verified Agent tool format.
// Verified reality (Claude Code 2.1.185): the sub-agent spawn tool is "Agent", NOT "Task".
// tool_input fields: description, prompt, subagent_type, model (optional).
const validTaskInput = `{"tool_name":"Agent","tool_input":{"description":"sdd-tasks sub-agent for scoping-fixes","subagent_type":"sdd-tasks","prompt":"Do the tasks phase."}}`

// passThrough is what the gate returns on a fail-safe no-op.
const passThrough = "{}"

// captureGateTask runs gateTaskCore and returns stdout, stderr contents.
func captureGateTask(args []string, stdinContent string, content []byte, readErr error) (stdout, stderr string) {
	var outBuf, errBuf bytes.Buffer
	var stdinReader io.Reader = strings.NewReader(stdinContent)

	mockReadFile := func(_ string) ([]byte, error) {
		return content, readErr
	}

	gateTaskCore(args, stdinReader, &outBuf, &errBuf, mockReadFile)
	return outBuf.String(), errBuf.String()
}

// TC-CLI-1: stdin read error → '{}' on stdout + exit 0 + diagnostic on stderr.
// We simulate a broken reader by wrapping an error-returning Reader.
func TestGateTaskCore_StdinReadError(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	brokenReader := &errorReader{err: errors.New("simulated read error")}

	gateTaskCore(
		[]string{"--contract-file", "/fake/contract.md"},
		brokenReader,
		&outBuf,
		&errBuf,
		func(_ string) ([]byte, error) { return []byte(testContractContent), nil },
	)

	outStr := strings.TrimSpace(outBuf.String())
	if outStr != "{}" {
		t.Errorf("stdin read error: stdout should be '{}'; got %q", outStr)
	}
	if errBuf.Len() == 0 {
		t.Error("stdin read error: stderr diagnostic must be written but was empty")
	}
}

// errorReader is an io.Reader that always returns an error.
type errorReader struct {
	err error
}

func (r *errorReader) Read(_ []byte) (int, error) {
	return 0, r.err
}

// TC-CLI-2: missing --contract-file → '{}' on stdout + exit 0 + diagnostic on stderr.
func TestGateTaskCore_MissingContractFile(t *testing.T) {
	stdout, stderr := captureGateTask(
		[]string{}, // no --contract-file
		validTaskInput,
		nil, nil,
	)

	outStr := strings.TrimSpace(stdout)
	if outStr != "{}" {
		t.Errorf("missing --contract-file: stdout should be '{}'; got %q", outStr)
	}
	if stderr == "" {
		t.Error("missing --contract-file: stderr diagnostic must be written but was empty")
	}
	if !strings.Contains(stderr, "contract-file") {
		t.Errorf("stderr diagnostic should mention 'contract-file'; got: %q", stderr)
	}
}

// TC-CLI-3: unparseable contract → '{}' on stdout + exit 0 + stderr diagnostic.
// A contract with no frontmatter causes gate.Process to produce a pass-through.
// The gate MUST emit a one-line stderr diagnostic so wiring mistakes with a
// corrupt contract are immediately visible (item 2 observability fix).
func TestGateTaskCore_UnparseableContract(t *testing.T) {
	stdout, stderr := captureGateTask(
		[]string{"--contract-file", "/fake/contract.md"},
		validTaskInput,
		[]byte(brokenContractContent), nil,
	)

	outStr := strings.TrimSpace(stdout)
	// Strict assertion: broken frontmatter must produce exactly '{}' pass-through,
	// not a hook-specific response (e.g. {"hookSpecificOutput":...}). This mirrors
	// the assertions used by TC-CLI-1/2/4 and would FAIL if the broken-frontmatter
	// path ever injected additional content.
	if outStr != "{}" {
		t.Errorf("unparseable contract: stdout must be exactly '{}'; got %q", outStr)
	}
	// REAL ASSERTION: stderr must contain a diagnostic (non-tautological).
	if stderr == "" {
		t.Error("unparseable contract: stderr diagnostic must be written but was empty")
	}
	if !strings.Contains(stderr, "frontmatter") && !strings.Contains(stderr, "contract") {
		t.Errorf("stderr diagnostic should mention 'frontmatter' or 'contract'; got: %q", stderr)
	}
}

// TC-CLI-4: contract file cannot be read (I/O error) → '{}' + exit 0 + stderr diagnostic.
func TestGateTaskCore_ContractFileReadError(t *testing.T) {
	stdout, stderr := captureGateTask(
		[]string{"--contract-file", "/non/existent/path.md"},
		validTaskInput,
		nil, errors.New("file not found"),
	)

	outStr := strings.TrimSpace(stdout)
	if outStr != "{}" {
		t.Errorf("contract read error: stdout should be '{}'; got %q", outStr)
	}
	if stderr == "" {
		t.Error("contract read error: stderr diagnostic must be written but was empty")
	}
}

// TC-CLI-5: F4 — input cap. A reader that produces stdinSizeLimit+1 bytes must
// not panic or OOM; the gate produces a pass-through (truncated JSON is malformed).
func TestGateTaskCore_InputCapPassThrough(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	// Produce stdinSizeLimit + 1 bytes of valid-looking but huge input.
	oversizedInput := strings.Repeat("A", int(stdinSizeLimit)+1)
	stdinReader := strings.NewReader(oversizedInput)

	gateTaskCore(
		[]string{"--contract-file", "/fake/contract.md"},
		stdinReader,
		&outBuf,
		&errBuf,
		func(_ string) ([]byte, error) { return []byte(testContractContent), nil },
	)

	outStr := strings.TrimSpace(outBuf.String())
	// The truncated/malformed JSON must result in a fail-safe pass-through.
	if outStr == "" {
		t.Error("oversized stdin: stdout must not be empty")
	}
	if !strings.HasPrefix(outStr, "{") {
		t.Errorf("oversized stdin: stdout must be valid JSON; got %q", outStr)
	}
}

// TC-CLI-6: happy path — valid contract + valid Agent JSON → full verified output contract.
//
// This test asserts the COMPLETE output contract at the CLI layer so that a regression
// dropping hookSpecificOutput, permissionDecision, description, or subagent_type FAILS:
//   - hookSpecificOutput.hookEventName == "PreToolUse"
//   - hookSpecificOutput.permissionDecision == "allow"
//   - hookSpecificOutput.updatedInput.prompt contains the injected contract path
//   - hookSpecificOutput.updatedInput.description echoes the original description unchanged
//   - hookSpecificOutput.updatedInput.subagent_type echoes the original subagent_type unchanged
func TestGateTaskCore_HappyPath_Inject(t *testing.T) {
	stdout, stderr := captureGateTask(
		[]string{"--contract-file", "/fake/contract.md", "--contract-path", "skills/_shared/minimalism-contract.md"},
		validTaskInput,
		[]byte(testContractContent), nil,
	)

	outStr := strings.TrimSpace(stdout)

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(outStr), &result); err != nil {
		t.Fatalf("happy path: response must be valid JSON; got: %s\nstderr: %s\nerr: %v", outStr, stderr, err)
	}

	// hookSpecificOutput must be present — a response without it means the gate
	// silently dropped the mutation and this test MUST fail.
	hso, ok := result["hookSpecificOutput"].(map[string]interface{})
	if !ok {
		t.Fatalf("happy path: response missing hookSpecificOutput; got: %s\nstderr: %s", outStr, stderr)
	}

	// hookEventName and permissionDecision are REQUIRED by Claude Code — without them
	// updatedInput is silently ignored.
	if hso["hookEventName"] != "PreToolUse" {
		t.Errorf("happy path: hookSpecificOutput.hookEventName must be 'PreToolUse'; got: %v", hso["hookEventName"])
	}
	if hso["permissionDecision"] != "allow" {
		t.Errorf("happy path: hookSpecificOutput.permissionDecision must be 'allow'; got: %v", hso["permissionDecision"])
	}

	// updatedInput must carry the mutated prompt AND echo all required tool_input fields.
	updatedInput, ok := hso["updatedInput"].(map[string]interface{})
	if !ok {
		t.Fatalf("happy path: hookSpecificOutput missing updatedInput; got: %v", hso)
	}

	// W-2: the injected contract path must appear in the mutated prompt.
	newPrompt, ok := updatedInput["prompt"].(string)
	if !ok {
		t.Fatal("happy path: updatedInput missing prompt string")
	}
	if !strings.Contains(newPrompt, "skills/_shared/minimalism-contract.md") {
		t.Errorf("happy path: updatedInput.prompt should contain injected contract path; got:\n%s", newPrompt)
	}

	// W-4: description must be echoed unchanged — dropping it causes Claude Code to
	// reject the hook response ("required parameter description is missing").
	if updatedInput["description"] != "sdd-tasks sub-agent for scoping-fixes" {
		t.Errorf("happy path: updatedInput.description must echo original value unchanged; got: %v", updatedInput["description"])
	}

	// subagent_type must be echoed unchanged.
	if updatedInput["subagent_type"] != "sdd-tasks" {
		t.Errorf("happy path: updatedInput.subagent_type must echo original value; got: %v", updatedInput["subagent_type"])
	}
}

// ---- runPropagateCore tests (item 7 coverage) --------------------------------

// minimalRegistry is a bare-bones registry missing the minimalism-contract row.
const minimalRegistry = `# Skill Registry

### Shared Contracts

| Artifact | Path | Description |
|----------|------|-------------|
| pre-sdd-contracts | skills/_shared/pre-sdd-contracts.md | Shared contracts |
`

// capturePropagateCore runs runPropagateCore with injectable I/O and returns
// stdout, stderr, and the exit code (or -1 if exit was not called).
func capturePropagateCore(
	args []string,
	readFiles map[string][]byte,
	readErr map[string]error,
	writtenFiles map[string][]byte,
) (stdout, stderr string, exitCode int) {
	var outBuf, errBuf bytes.Buffer
	exitCode = -1 // sentinel: exit not called

	readFile := func(path string) ([]byte, error) {
		if err, ok := readErr[path]; ok && err != nil {
			return nil, err
		}
		if b, ok := readFiles[path]; ok {
			return b, nil
		}
		return nil, errors.New("file not found: " + path)
	}
	writeFile := func(path string, data []byte, _ os.FileMode) error {
		if writtenFiles != nil {
			writtenFiles[path] = data
		}
		return nil
	}
	exit := func(code int) {
		exitCode = code
	}

	runPropagateCore(args, &outBuf, &errBuf, readFile, writeFile, exit)
	return outBuf.String(), errBuf.String(), exitCode
}

// TC-CLI-P1: missing --registry → exit 1 + stderr diagnostic.
func TestRunPropagateCore_MissingRegistry(t *testing.T) {
	_, stderr, exitCode := capturePropagateCore(
		[]string{"--contract-file", "/fake/contract.md"},
		map[string][]byte{"/fake/contract.md": []byte(testContractContent)},
		nil, nil,
	)
	if exitCode != 1 {
		t.Errorf("missing --registry: expected exit 1, got %d", exitCode)
	}
	if !strings.Contains(stderr, "registry") {
		t.Errorf("missing --registry: stderr should mention 'registry'; got %q", stderr)
	}
}

// TC-CLI-P2: missing --contract-file → exit 1 + stderr diagnostic.
func TestRunPropagateCore_MissingContractFile(t *testing.T) {
	_, stderr, exitCode := capturePropagateCore(
		[]string{"--registry", "/fake/registry.md"},
		nil, nil, nil,
	)
	if exitCode != 1 {
		t.Errorf("missing --contract-file: expected exit 1, got %d", exitCode)
	}
	if !strings.Contains(stderr, "contract-file") {
		t.Errorf("missing --contract-file: stderr should mention 'contract-file'; got %q", stderr)
	}
}

// TC-CLI-P3: contract file read error → exit 1 + stderr diagnostic.
func TestRunPropagateCore_ContractReadError(t *testing.T) {
	_, stderr, exitCode := capturePropagateCore(
		[]string{"--registry", "/fake/registry.md", "--contract-file", "/bad/contract.md"},
		nil,
		map[string]error{"/bad/contract.md": errors.New("permission denied")},
		nil,
	)
	if exitCode != 1 {
		t.Errorf("contract read error: expected exit 1, got %d", exitCode)
	}
	if stderr == "" {
		t.Error("contract read error: stderr must not be empty")
	}
}

// TC-CLI-P4: broken contract frontmatter → exit 1 + stderr diagnostic.
func TestRunPropagateCore_BrokenFrontmatter(t *testing.T) {
	_, stderr, exitCode := capturePropagateCore(
		[]string{"--registry", "/fake/registry.md", "--contract-file", "/fake/contract.md"},
		map[string][]byte{
			"/fake/contract.md":  []byte("no frontmatter"),
			"/fake/registry.md":  []byte(minimalRegistry),
		},
		nil, nil,
	)
	if exitCode != 1 {
		t.Errorf("broken frontmatter: expected exit 1, got %d", exitCode)
	}
	if stderr == "" {
		t.Error("broken frontmatter: stderr must not be empty")
	}
}

// TC-CLI-P5: registry read error → exit 1 + stderr diagnostic.
func TestRunPropagateCore_RegistryReadError(t *testing.T) {
	_, stderr, exitCode := capturePropagateCore(
		[]string{"--registry", "/bad/registry.md", "--contract-file", "/fake/contract.md"},
		map[string][]byte{"/fake/contract.md": []byte(testContractContent)},
		map[string]error{"/bad/registry.md": errors.New("file not found")},
		nil,
	)
	if exitCode != 1 {
		t.Errorf("registry read error: expected exit 1, got %d", exitCode)
	}
	if stderr == "" {
		t.Error("registry read error: stderr must not be empty")
	}
}

// TC-CLI-P6: happy path — valid contract + registry missing block → block inserted,
// file written with scoped block, exit -1 (no exit call = success).
func TestRunPropagateCore_HappyPath_InsertsScopedBlock(t *testing.T) {
	writtenFiles := make(map[string][]byte)
	stdout, stderr, exitCode := capturePropagateCore(
		[]string{
			"--registry", "/fake/registry.md",
			"--contract-file", "/fake/contract.md",
			"--contract-path", "skills/_shared/minimalism-contract.md",
		},
		map[string][]byte{
			"/fake/contract.md": []byte(testContractContent),
			"/fake/registry.md": []byte(minimalRegistry),
		},
		nil,
		writtenFiles,
	)

	if exitCode != -1 {
		t.Errorf("happy path: unexpected exit call with code %d; stderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stdout, "inserted/updated") {
		t.Errorf("happy path: stdout should mention 'inserted/updated'; got: %q", stdout)
	}

	// The written registry must contain the scoped block.
	written, ok := writtenFiles["/fake/registry.md"]
	if !ok {
		t.Fatal("happy path: registry file was not written")
	}
	if !strings.Contains(string(written), "<!-- BEGIN: minimalism-contract-scope (auto-generated) -->") {
		t.Errorf("happy path: written registry must contain BEGIN marker; got:\n%s", string(written))
	}
	if !strings.Contains(string(written), "sdd-tasks") {
		t.Errorf("happy path: written registry must reference sdd-tasks; got:\n%s", string(written))
	}
}

// TC-CLI-P6b: writeFile error on the happy path → exit 1 + stderr diagnostic.
// This covers the previously-uncovered branch at main.go lines 135-139:
//
//	if err := writeFile(registryPath, []byte(out), 0644); err != nil { ... exit(1) }
//
// The writeFile mock returns an error only for the registry path, so we reach
// the write call (changed == true) and hit the error branch.
// This test would FAIL if the error branch were removed or silently swallowed.
func TestRunPropagateCore_WriteFileError(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	exitCode := -1

	registryPath := "/fake/registry.md"
	contractFilePath := "/fake/contract.md"

	runPropagateCore(
		[]string{
			"--registry", registryPath,
			"--contract-file", contractFilePath,
			"--contract-path", "skills/_shared/minimalism-contract.md",
		},
		&outBuf,
		&errBuf,
		func(path string) ([]byte, error) {
			switch path {
			case contractFilePath:
				return []byte(testContractContent), nil
			case registryPath:
				return []byte(minimalRegistry), nil
			}
			return nil, errors.New("file not found: " + path)
		},
		func(_ string, _ []byte, _ os.FileMode) error {
			return errors.New("simulated disk full")
		},
		func(code int) { exitCode = code },
	)

	if exitCode != 1 {
		t.Errorf("writeFile error: expected exit 1, got %d", exitCode)
	}
	if errBuf.Len() == 0 {
		t.Error("writeFile error: stderr diagnostic must be written but was empty")
	}
}

// TC-CLI-P7: no-op path — registry already correct → no file written, stdout says no-op.
func TestRunPropagateCore_NoOp_AlreadyCorrect(t *testing.T) {
	// Build the already-correct registry using BuildScopedRow.
	// We do this programmatically to avoid hardcoding the exact string.
	alreadyScopedContent := minimalRegistry

	// Run once to get the correct output.
	writtenFiles1 := make(map[string][]byte)
	runPropagateCore(
		[]string{
			"--registry", "/fake/registry.md",
			"--contract-file", "/fake/contract.md",
			"--contract-path", "skills/_shared/minimalism-contract.md",
		},
		&bytes.Buffer{}, &bytes.Buffer{},
		func(path string) ([]byte, error) {
			if path == "/fake/contract.md" {
				return []byte(testContractContent), nil
			}
			return []byte(alreadyScopedContent), nil
		},
		func(path string, data []byte, _ os.FileMode) error {
			writtenFiles1[path] = data
			return nil
		},
		func(_ int) {},
	)
	firstWrite, ok := writtenFiles1["/fake/registry.md"]
	if !ok {
		t.Fatal("first run: registry must be written")
	}

	// Run again with the corrected content — must be a no-op.
	writtenFiles2 := make(map[string][]byte)
	var outBuf bytes.Buffer
	runPropagateCore(
		[]string{
			"--registry", "/fake/registry.md",
			"--contract-file", "/fake/contract.md",
			"--contract-path", "skills/_shared/minimalism-contract.md",
		},
		&outBuf, &bytes.Buffer{},
		func(path string) ([]byte, error) {
			if path == "/fake/contract.md" {
				return []byte(testContractContent), nil
			}
			return firstWrite, nil // use the already-correct content from first run
		},
		func(path string, data []byte, _ os.FileMode) error {
			writtenFiles2[path] = data
			return nil
		},
		func(_ int) {},
	)

	if _, wrote := writtenFiles2["/fake/registry.md"]; wrote {
		t.Error("no-op: file should NOT be written when registry is already correct")
	}
	if !strings.Contains(outBuf.String(), "no-op") || !strings.Contains(outBuf.String(), "already correct") {
		t.Errorf("no-op: stdout should say already correct/no-op; got: %q", outBuf.String())
	}
}

// ---- parseMergeSettingsArgs tests -------------------------------------------

// TC-MS-PARSE-1: both flags present → parsed correctly.
func TestParseMergeSettingsArgs_BothFlags(t *testing.T) {
	sp, hc := parseMergeSettingsArgs([]string{
		"--settings", "/tmp/settings.json",
		"--hook-command", "/home/user/.claude/bin/gentle-ai-overlay",
	})
	if sp != "/tmp/settings.json" {
		t.Errorf("settingsPath: got %q, want /tmp/settings.json", sp)
	}
	if hc != "/home/user/.claude/bin/gentle-ai-overlay" {
		t.Errorf("hookCommand: got %q", hc)
	}
}

// TC-MS-PARSE-2: flags absent → empty strings.
func TestParseMergeSettingsArgs_NoFlags(t *testing.T) {
	sp, hc := parseMergeSettingsArgs([]string{})
	if sp != "" || hc != "" {
		t.Errorf("empty args should yield empty strings; got %q, %q", sp, hc)
	}
}

// ---- merge-settings integration via runMergeSettings (uses real files) ------

// TC-MS-CLI-1: absent settings file → created with both hooks + success stdout.
func TestRunMergeSettings_AbsentFile_CreatesHooks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	// Redirect stdout/stderr by temporarily overriding os.Stdout/os.Stderr is not
	// feasible for the run* functions which call os.Exit. Instead we test through
	// runMergeSettings by checking file output after calling through a wrapper that
	// catches the exit. Since runMergeSettings calls os.Exit on error and the happy
	// path does not, we can call it directly and check the file.
	//
	// NOTE: these tests are intentionally NOT using captureRun helpers that would
	// require intercepting os.Exit (which is complex in Go). The assertions focus on
	// the file-system side effects (the real contract) instead of stdout capture.

	// Call runMergeSettings — happy path should not panic or os.Exit.
	runMergeSettings([]string{
		"--settings", path,
		"--hook-command", "/test/.claude/bin/gentle-ai-overlay",
	})

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("settings.json not created: %v", err)
	}
	var root map[string]interface{}
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("settings.json not valid JSON: %v", err)
	}
	hooks, ok := root["hooks"].(map[string]interface{})
	if !ok {
		t.Fatalf("hooks key not present in %v", root)
	}
	if _, ok := hooks["UserPromptSubmit"]; !ok {
		t.Error("UserPromptSubmit not present")
	}
	if _, ok := hooks["PreToolUse"]; !ok {
		t.Error("PreToolUse not present")
	}
}

// TC-MS-CLI-2: idempotent — calling twice produces same output without duplicates.
func TestRunMergeSettings_Idempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	args := []string{
		"--settings", path,
		"--hook-command", "/test/.claude/bin/gentle-ai-overlay",
	}

	runMergeSettings(args)
	runMergeSettings(args)

	data, _ := os.ReadFile(path)
	var root map[string]interface{}
	json.Unmarshal(data, &root)

	hooks := root["hooks"].(map[string]interface{})
	// countEntries uses binary-substring match inside inner hooks[].command
	// to match the verified entry shape: {"hooks":[{"type":"command","command":"..."}]}.
	countEntries := func(key string) int {
		entries, _ := hooks[key].([]interface{})
		n := 0
		for _, e := range entries {
			em, ok := e.(map[string]interface{})
			if !ok {
				continue
			}
			innerHooks, ok := em["hooks"].([]interface{})
			if !ok {
				continue
			}
			for _, ih := range innerHooks {
				ihm, ok := ih.(map[string]interface{})
				if !ok {
					continue
				}
				if cmdStr, ok := ihm["command"].(string); ok {
					if strings.Contains(cmdStr, "/test/.claude/bin/gentle-ai-overlay") {
						n++
						break
					}
				}
			}
		}
		return n
	}
	// Two pairs install (minimalism + skill-discovery-safety) → 2 entries per
	// key; merge-settings run twice stays at 2 (idempotent).
	if n := countEntries("UserPromptSubmit"); n != 2 {
		t.Errorf("UserPromptSubmit: expected 2 entries, got %d", n)
	}
	if n := countEntries("PreToolUse"); n != 2 {
		t.Errorf("PreToolUse: expected 2 entries, got %d", n)
	}
}

// TC-MS-CLI-3: backup created when file pre-exists.
func TestRunMergeSettings_BackupCreated(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	bakPath := path + ".bak"

	original := []byte(`{"existing":true}`)
	os.WriteFile(path, original, 0644)

	runMergeSettings([]string{
		"--settings", path,
		"--hook-command", "/test/.claude/bin/gentle-ai-overlay",
	})

	bak, err := os.ReadFile(bakPath)
	if err != nil {
		t.Fatalf("backup not created: %v", err)
	}
	if string(bak) != string(original) {
		t.Errorf("backup content mismatch: got %q", string(bak))
	}
}

// ---- uninstall-hooks integration via runUninstallHooks ----------------------

// TC-UH-CLI-1: install then uninstall → hooks gone, other keys preserved.
func TestRunUninstallHooks_RemovesHooks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	hookCmd := "/test/.claude/bin/gentle-ai-overlay"
	args := []string{"--settings", path, "--hook-command", hookCmd}

	runMergeSettings(args)
	runUninstallHooks(args)

	data, _ := os.ReadFile(path)
	var root map[string]interface{}
	json.Unmarshal(data, &root)

	hooks, _ := root["hooks"].(map[string]interface{})
	hasCmd := func(key string) bool {
		entries, _ := hooks[key].([]interface{})
		for _, e := range entries {
			if em, ok := e.(map[string]interface{}); ok && em["command"] == hookCmd {
				return true
			}
		}
		return false
	}
	if hasCmd("UserPromptSubmit") {
		t.Error("UserPromptSubmit hook should be removed")
	}
	if hasCmd("PreToolUse") {
		t.Error("PreToolUse hook should be removed")
	}
}

// TC-UH-CLI-2: uninstall on absent file → no panic, no error.
func TestRunUninstallHooks_AbsentFile_NoOp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.json")
	// Should not panic or call os.Exit(1).
	runUninstallHooks([]string{
		"--settings", path,
		"--hook-command", "/test/binary",
	})
}

// ---------------------------------------------------------------------------
// Task 2: filepath.Clean on --registry
// ---------------------------------------------------------------------------

// TC-CLEAN-1: a --registry path containing "../" segments must be cleaned
// before use so the actual read path does not contain ".." traversal.
func TestRunPropagateCore_RegistryPathCleaned(t *testing.T) {
	// We capture which path the readFile fn was actually called with for the
	// registry. It must NOT contain "..".
	var calledRegistryPath string
	var outBuf, errBuf bytes.Buffer
	exitCode := -1

	contractPath := "/fake/contract.md"
	// A registry path with traversal segments.
	dirtyRegistryPath := "/some/dir/../other/registry.md"
	// The expected cleaned path.
	wantRegistryPath := "/some/other/registry.md"

	readFile := func(path string) ([]byte, error) {
		if path == contractPath {
			return []byte(testContractContent), nil
		}
		// Record the first non-contract path (the registry path).
		if calledRegistryPath == "" {
			calledRegistryPath = path
		}
		return []byte(minimalRegistry), nil
	}

	runPropagateCore(
		[]string{"--registry", dirtyRegistryPath, "--contract-file", contractPath},
		&outBuf, &errBuf,
		readFile,
		func(_ string, _ []byte, _ os.FileMode) error { return nil },
		func(code int) { exitCode = code },
	)

	if exitCode == 1 {
		t.Fatalf("unexpected exit 1; stderr: %s", errBuf.String())
	}
	if calledRegistryPath != wantRegistryPath {
		t.Errorf("registry path not cleaned: got %q, want %q", calledRegistryPath, wantRegistryPath)
	}
}

// TC-CLEAN-2: a normal (already clean) path must not be altered.
func TestRunPropagateCore_RegistryPathClean_NormalPath(t *testing.T) {
	var calledRegistryPath string
	var outBuf, errBuf bytes.Buffer
	exitCode := -1

	contractPath := "/fake/contract.md"
	normalRegistryPath := "/project/.atl/skill-registry.md"

	readFile := func(path string) ([]byte, error) {
		if path == contractPath {
			return []byte(testContractContent), nil
		}
		if calledRegistryPath == "" {
			calledRegistryPath = path
		}
		return []byte(minimalRegistry), nil
	}

	runPropagateCore(
		[]string{"--registry", normalRegistryPath, "--contract-file", contractPath},
		&outBuf, &errBuf,
		readFile,
		func(_ string, _ []byte, _ os.FileMode) error { return nil },
		func(code int) { exitCode = code },
	)

	if exitCode == 1 {
		t.Fatalf("unexpected exit 1; stderr: %s", errBuf.String())
	}
	if calledRegistryPath != normalRegistryPath {
		t.Errorf("normal path was modified: got %q, want %q", calledRegistryPath, normalRegistryPath)
	}
}

// ---------------------------------------------------------------------------
// Task 4: propagate graceful no-op on missing registry
// ---------------------------------------------------------------------------

// TC-PROP-ABSENT-1: registry file absent → exit 0, stdout informative message,
// no stderr, no error — clean no-op for projects not using the overlay.
func TestRunPropagateCore_RegistryAbsent_NoOp(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	exitCode := -1

	contractPath := "/fake/contract.md"
	registryPath := "/project/.atl/skill-registry.md"

	readFile := func(path string) ([]byte, error) {
		if path == contractPath {
			return []byte(testContractContent), nil
		}
		// Registry absent: return os.ErrNotExist.
		return nil, os.ErrNotExist
	}

	runPropagateCore(
		[]string{"--registry", registryPath, "--contract-file", contractPath},
		&outBuf, &errBuf,
		readFile,
		func(_ string, _ []byte, _ os.FileMode) error { return nil },
		func(code int) { exitCode = code },
	)

	// Must exit 0 (exitCode remains -1 = no exit call = success path).
	if exitCode != -1 {
		t.Errorf("registry absent: expected exit 0 (no exit call), got exit %d; stderr: %s", exitCode, errBuf.String())
	}
	// stderr must be empty — this is a clean no-op, not an error.
	if errBuf.Len() != 0 {
		t.Errorf("registry absent: stderr must be empty; got: %q", errBuf.String())
	}
	// stdout must contain an informative message.
	out := outBuf.String()
	if out == "" {
		t.Error("registry absent: stdout must contain an informative message")
	}
	if !strings.Contains(out, "not found") && !strings.Contains(out, "no-op") {
		t.Errorf("registry absent: stdout should mention 'not found' or 'no-op'; got: %q", out)
	}
}

// TC-PROP-ABSENT-2: registry present but unreadable (e.g. permission error) →
// exit 1 + stderr error. This distinguishes "absent" from "present but broken".
func TestRunPropagateCore_RegistryUnreadable_ExitOne(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	exitCode := -1

	contractPath := "/fake/contract.md"
	registryPath := "/project/.atl/skill-registry.md"

	readFile := func(path string) ([]byte, error) {
		if path == contractPath {
			return []byte(testContractContent), nil
		}
		// Present but unreadable: return a non-NotExist error.
		return nil, errors.New("permission denied")
	}

	runPropagateCore(
		[]string{"--registry", registryPath, "--contract-file", contractPath},
		&outBuf, &errBuf,
		readFile,
		func(_ string, _ []byte, _ os.FileMode) error { return nil },
		func(code int) { exitCode = code },
	)

	if exitCode != 1 {
		t.Errorf("registry unreadable: expected exit 1, got %d; stdout: %q", exitCode, outBuf.String())
	}
	if errBuf.Len() == 0 {
		t.Error("registry unreadable: stderr must contain error message")
	}
}

// ---------------------------------------------------------------------------
// Task 1: statusCore tests
// ---------------------------------------------------------------------------

// buildSettingsWithHooks builds a minimal settings.json map with both our
// hook entries present (using the given hookCommand substring).
func buildSettingsWithHooks(hookCmd string) map[string]interface{} {
	makeEntry := func(extraKey, extraVal string) map[string]interface{} {
		entry := map[string]interface{}{
			"hooks": []interface{}{map[string]interface{}{
				"type":    "command",
				"command": "command -v " + hookCmd + " && " + hookCmd + " gate-task || true",
			}},
		}
		if extraKey != "" {
			entry[extraKey] = extraVal
		}
		return entry
	}

	preToolUse := makeEntry("matcher", "Agent")
	userPromptSubmit := makeEntry("", "")

	return map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreToolUse":       []interface{}{preToolUse},
			"UserPromptSubmit": []interface{}{userPromptSubmit},
		},
	}
}

// buildFakeHomeWithBinary creates the $HOME/.claude/bin/gentle-ai-overlay tree in a TempDir.
func buildFakeHomeWithBinary(t *testing.T) (homeDir, binaryPath string) {
	t.Helper()
	homeDir = t.TempDir()
	binaryDir := filepath.Join(homeDir, ".claude", "bin")
	if err := os.MkdirAll(binaryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	binaryPath = filepath.Join(binaryDir, "gentle-ai-overlay")
	if err := os.WriteFile(binaryPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return
}

// buildFakeContract creates a minimalism-contract.md in the fake home's skills tree.
func buildFakeContract(t *testing.T, homeDir string) string {
	t.Helper()
	contractDir := filepath.Join(homeDir, ".claude", "skills", "_shared")
	if err := os.MkdirAll(contractDir, 0o755); err != nil {
		t.Fatal(err)
	}
	contractPath := filepath.Join(contractDir, "minimalism-contract.md")
	if err := os.WriteFile(contractPath, []byte(testContractContent), 0o644); err != nil {
		t.Fatal(err)
	}
	return contractPath
}

// TC-STATUS-1: all checks OK → output contains [OK] for every line, no [FAIL],
// statusCore returns true.
func TestStatusCore_AllOK(t *testing.T) {
	homeDir, binaryPath := buildFakeHomeWithBinary(t)
	buildFakeContract(t, homeDir)

	settingsData := buildSettingsWithHooks(binaryPath)

	deps := statusDeps{
		stat:     os.Stat,
		readFile: os.ReadFile,
		loadSettings: func(_ string) (map[string]interface{}, error) {
			return settingsData, nil
		},
		home: func() string { return homeDir },
		cwd:  func() string { return "" },
	}

	var outBuf bytes.Buffer
	result, _ := statusCore(&outBuf, deps)
	out := outBuf.String()

	if !result {
		t.Errorf("statusCore: expected true (all OK); output:\n%s", out)
	}
	if strings.Contains(out, "[FAIL]") {
		t.Errorf("statusCore: no [FAIL] expected when all OK; output:\n%s", out)
	}
	if !strings.Contains(out, "[OK  ]") {
		t.Errorf("statusCore: output must contain [OK  ] lines; output:\n%s", out)
	}
}

// TC-STATUS-2: binary missing → [FAIL] for binary check, statusCore returns false.
func TestStatusCore_BinaryMissing(t *testing.T) {
	homeDir := t.TempDir() // no binary created
	buildFakeContract(t, homeDir)

	settingsData := buildSettingsWithHooks("gentle-ai-overlay")
	deps := statusDeps{
		stat:     os.Stat,
		readFile: os.ReadFile,
		loadSettings: func(_ string) (map[string]interface{}, error) {
			return settingsData, nil
		},
		home: func() string { return homeDir },
		cwd:  func() string { return "" },
	}

	var outBuf bytes.Buffer
	result, _ := statusCore(&outBuf, deps)
	out := outBuf.String()

	if result {
		t.Errorf("statusCore: expected false when binary missing; output:\n%s", out)
	}
	if !strings.Contains(out, "[FAIL]") {
		t.Errorf("statusCore: expected [FAIL] for binary; output:\n%s", out)
	}
}

// TC-STATUS-3: settings.json absent → [FAIL] for both hook checks.
func TestStatusCore_SettingsAbsent(t *testing.T) {
	homeDir, _ := buildFakeHomeWithBinary(t)
	buildFakeContract(t, homeDir)

	deps := statusDeps{
		stat:     os.Stat,
		readFile: os.ReadFile,
		loadSettings: func(_ string) (map[string]interface{}, error) {
			return nil, nil // file absent
		},
		home: func() string { return homeDir },
		cwd:  func() string { return "" },
	}

	var outBuf bytes.Buffer
	result, _ := statusCore(&outBuf, deps)
	out := outBuf.String()

	if result {
		t.Errorf("statusCore: expected false when settings absent; output:\n%s", out)
	}
	// At least the two hook checks should fail.
	if strings.Count(out, "[FAIL]") < 2 {
		t.Errorf("statusCore: expected at least 2 [FAIL] lines; output:\n%s", out)
	}
}

// TC-STATUS-4: UserPromptSubmit hook missing → [FAIL] for that check only.
func TestStatusCore_UserPromptSubmitMissing(t *testing.T) {
	homeDir, binaryPath := buildFakeHomeWithBinary(t)
	buildFakeContract(t, homeDir)

	// Settings with PreToolUse but no UserPromptSubmit.
	settingsData := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreToolUse": []interface{}{map[string]interface{}{
				"matcher": "Agent",
				"hooks": []interface{}{map[string]interface{}{
					"type":    "command",
					"command": "command -v " + binaryPath + " && " + binaryPath + " gate-task || true",
				}},
			}},
		},
	}

	deps := statusDeps{
		stat:     os.Stat,
		readFile: os.ReadFile,
		loadSettings: func(_ string) (map[string]interface{}, error) {
			return settingsData, nil
		},
		home: func() string { return homeDir },
		cwd:  func() string { return "" },
	}

	var outBuf bytes.Buffer
	result, _ := statusCore(&outBuf, deps)
	out := outBuf.String()

	if result {
		t.Errorf("statusCore: expected false when UserPromptSubmit missing; output:\n%s", out)
	}
	if !strings.Contains(out, "UserPromptSubmit") {
		t.Errorf("statusCore: output should mention UserPromptSubmit; output:\n%s", out)
	}
}

// TC-STATUS-5: PreToolUse hook missing → [FAIL] for that check.
func TestStatusCore_PreToolUseMissing(t *testing.T) {
	homeDir, binaryPath := buildFakeHomeWithBinary(t)
	buildFakeContract(t, homeDir)

	// Settings with UserPromptSubmit but no PreToolUse.
	settingsData := map[string]interface{}{
		"hooks": map[string]interface{}{
			"UserPromptSubmit": []interface{}{map[string]interface{}{
				"hooks": []interface{}{map[string]interface{}{
					"type":    "command",
					"command": "command -v " + binaryPath + " && " + binaryPath + " propagate || true",
				}},
			}},
		},
	}

	deps := statusDeps{
		stat:     os.Stat,
		readFile: os.ReadFile,
		loadSettings: func(_ string) (map[string]interface{}, error) {
			return settingsData, nil
		},
		home: func() string { return homeDir },
		cwd:  func() string { return "" },
	}

	var outBuf bytes.Buffer
	result, _ := statusCore(&outBuf, deps)
	out := outBuf.String()

	if result {
		t.Errorf("statusCore: expected false when PreToolUse missing; output:\n%s", out)
	}
	if !strings.Contains(out, "gate-task") || !strings.Contains(out, "Agent") {
		t.Errorf("statusCore: output should mention gate-task/Agent for PreToolUse check; output:\n%s", out)
	}
}

// TC-STATUS-6: contract missing → [FAIL] for contract check.
func TestStatusCore_ContractMissing(t *testing.T) {
	homeDir, binaryPath := buildFakeHomeWithBinary(t)
	// Do NOT create contract file.

	settingsData := buildSettingsWithHooks(binaryPath)
	deps := statusDeps{
		stat:     os.Stat,
		readFile: os.ReadFile,
		loadSettings: func(_ string) (map[string]interface{}, error) {
			return settingsData, nil
		},
		home: func() string { return homeDir },
		cwd:  func() string { return "" },
	}

	var outBuf bytes.Buffer
	result, _ := statusCore(&outBuf, deps)
	out := outBuf.String()

	if result {
		t.Errorf("statusCore: expected false when contract missing; output:\n%s", out)
	}
	if !strings.Contains(out, "contract") {
		t.Errorf("statusCore: output should mention 'contract'; output:\n%s", out)
	}
}

// TC-STATUS-7: contract present but frontmatter broken → [FAIL] for contract check.
func TestStatusCore_ContractBrokenFrontmatter(t *testing.T) {
	homeDir, binaryPath := buildFakeHomeWithBinary(t)

	// Create contract directory but write broken content.
	contractDir := filepath.Join(homeDir, ".claude", "skills", "_shared")
	os.MkdirAll(contractDir, 0o755)
	contractPath := filepath.Join(contractDir, "minimalism-contract.md")
	os.WriteFile(contractPath, []byte("no frontmatter here"), 0o644)

	settingsData := buildSettingsWithHooks(binaryPath)
	deps := statusDeps{
		stat:     os.Stat,
		readFile: os.ReadFile,
		loadSettings: func(_ string) (map[string]interface{}, error) {
			return settingsData, nil
		},
		home: func() string { return homeDir },
		cwd:  func() string { return "" },
	}

	var outBuf bytes.Buffer
	result, _ := statusCore(&outBuf, deps)
	out := outBuf.String()

	if result {
		t.Errorf("statusCore: expected false when contract frontmatter broken; output:\n%s", out)
	}
	if !strings.Contains(out, "frontmatter") {
		t.Errorf("statusCore: output should mention 'frontmatter'; output:\n%s", out)
	}
}

// TC-STATUS-8: registry check best-effort — even when registry is absent,
// statusCore returns true (if other checks pass). An absent registry is a
// benign no-op (the project simply may not use the overlay); it is NOT a hard
// failure. The registry check only fails on a real IO error or an empty file.
func TestStatusCore_RegistryAbsent_BestEffort(t *testing.T) {
	homeDir, binaryPath := buildFakeHomeWithBinary(t)
	buildFakeContract(t, homeDir)

	settingsData := buildSettingsWithHooks(binaryPath)

	cwdDir := t.TempDir() // no registry here

	deps := statusDeps{
		stat:     os.Stat,
		readFile: os.ReadFile,
		loadSettings: func(_ string) (map[string]interface{}, error) {
			return settingsData, nil
		},
		home: func() string { return homeDir },
		cwd:  func() string { return cwdDir },
	}

	var outBuf bytes.Buffer
	result, _ := statusCore(&outBuf, deps)
	out := outBuf.String()

	// Overall result must be true since registry check is best-effort.
	if !result {
		t.Errorf("statusCore: expected true even when registry absent; output:\n%s", out)
	}
	// Registry line should appear with [OK  ] and mention "not present".
	if !strings.Contains(out, "not present") {
		t.Errorf("statusCore: registry absent output should mention 'not present'; output:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// T-06: prespec subcommand dispatch (cmd layer)
// ---------------------------------------------------------------------------

// TC-PRESPEC-1: lint verb wired into main dispatch — happy path.
// Verifies runPrespec passes the verb and stdin to PrespecCore correctly.
func TestRunPrespec_LintHappyPath(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	code := -1
	runPrespecCore(
		"lint",
		strings.NewReader(`{"question":"What is the main obstacle?"}`),
		&outBuf,
		&errBuf,
		func(c int) { code = c },
	)
	if code != -1 {
		t.Fatalf("lint happy path: unexpected exit %d; stderr=%q", code, errBuf.String())
	}
	var out struct {
		Accepted bool `json:"accepted"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(outBuf.String())), &out); err != nil {
		t.Fatalf("lint happy path: invalid JSON output: %v\nout=%q", err, outBuf.String())
	}
	if !out.Accepted {
		t.Error("lint happy path: expected accepted=true")
	}
}

// TC-PRESPEC-2: unknown verb exits 1.
func TestRunPrespec_UnknownVerbExitsOne(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	code := -1
	runPrespecCore(
		"bad-verb",
		strings.NewReader("{}"),
		&outBuf,
		&errBuf,
		func(c int) { code = c },
	)
	if code != 1 {
		t.Errorf("unknown verb: want exit 1; got %d", code)
	}
}

// TC-PRESPEC-3: malformed JSON exits 1 with stderr diagnostic.
func TestRunPrespec_MalformedJSONExitsOne(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	code := -1
	runPrespecCore(
		"rank",
		strings.NewReader("not-json"),
		&outBuf,
		&errBuf,
		func(c int) { code = c },
	)
	if code != 1 {
		t.Errorf("malformed JSON: want exit 1; got %d", code)
	}
	if errBuf.Len() == 0 {
		t.Error("malformed JSON: expected stderr diagnostic")
	}
}

// TC-STATUS-9: registry present WITH all three scoped blocks → [OK  ] with note "scoped block present".
func TestStatusCore_RegistryScopedBlockPresent(t *testing.T) {
	homeDir, binaryPath := buildFakeHomeWithBinary(t)
	buildFakeContract(t, homeDir)

	settingsData := buildSettingsWithHooks(binaryPath)

	cwdDir := t.TempDir()
	registryDir := filepath.Join(cwdDir, ".atl")
	os.MkdirAll(registryDir, 0o755)
	// Registry must contain ALL THREE managed contract blocks to report [OK].
	registryContent := "# Registry\n" +
		propagator.BeginMarker + "\n| minimalism-contract | x | y |\n" + propagator.EndMarker + "\n" +
		propagator.DiscoverySafetyBeginMarker + "\n| skill-discovery-safety | a | b |\n" + propagator.DiscoverySafetyEndMarker + "\n" +
		propagator.AntiGenericDesignBeginMarker + "\n| anti-generic-design | c | d |\n" + propagator.AntiGenericDesignEndMarker + "\n"
	os.WriteFile(filepath.Join(registryDir, "skill-registry.md"), []byte(registryContent), 0o644)

	deps := statusDeps{
		stat:     os.Stat,
		readFile: os.ReadFile,
		loadSettings: func(_ string) (map[string]interface{}, error) {
			return settingsData, nil
		},
		home: func() string { return homeDir },
		cwd:  func() string { return cwdDir },
	}

	var outBuf bytes.Buffer
	result, _ := statusCore(&outBuf, deps)
	out := outBuf.String()

	if !result {
		t.Errorf("statusCore: expected true when all three scoped blocks present; output:\n%s", out)
	}
	if !strings.Contains(out, "scoped block present") {
		t.Errorf("statusCore: output should say 'scoped block present'; output:\n%s", out)
	}
}

// TC-STATUS-9b: registry present with only the minimalism block (safety block missing)
// → WARN/degraded, naming which block is absent.
func TestStatusCore_RegistryOnlyMinimalismBlock_Degraded(t *testing.T) {
	homeDir, binaryPath := buildFakeHomeWithBinary(t)
	buildFakeContract(t, homeDir)

	settingsData := buildSettingsWithHooks(binaryPath)

	cwdDir := t.TempDir()
	registryDir := filepath.Join(cwdDir, ".atl")
	os.MkdirAll(registryDir, 0o755)
	registryContent := "# Registry\n" + propagator.BeginMarker + "\n| minimalism-contract | x | y |\n" + propagator.EndMarker + "\n"
	os.WriteFile(filepath.Join(registryDir, "skill-registry.md"), []byte(registryContent), 0o644)

	deps := statusDeps{
		stat:     os.Stat,
		readFile: os.ReadFile,
		loadSettings: func(_ string) (map[string]interface{}, error) {
			return settingsData, nil
		},
		home: func() string { return homeDir },
		cwd:  func() string { return cwdDir },
	}

	var outBuf bytes.Buffer
	allOK, degraded := statusCore(&outBuf, deps)
	out := outBuf.String()

	if !allOK {
		t.Errorf("statusCore: expected allOK=true (no hard FAIL) for missing safety block; output:\n%s", out)
	}
	if !degraded {
		t.Errorf("statusCore: expected degraded=true for missing safety block; output:\n%s", out)
	}
	if !strings.Contains(out, "[WARN]") {
		t.Errorf("statusCore: output should contain [WARN] for degraded registry; output:\n%s", out)
	}
	if !strings.Contains(out, "skill-discovery-safety-scope") {
		t.Errorf("statusCore: WARN note should name which block is absent; output:\n%s", out)
	}
}

// TC-STATUS-9c: registry present with only the safety block (minimalism block missing)
// → WARN/degraded, naming which block is absent.
func TestStatusCore_RegistryOnlySafetyBlock_Degraded(t *testing.T) {
	homeDir, binaryPath := buildFakeHomeWithBinary(t)
	buildFakeContract(t, homeDir)

	settingsData := buildSettingsWithHooks(binaryPath)

	cwdDir := t.TempDir()
	registryDir := filepath.Join(cwdDir, ".atl")
	os.MkdirAll(registryDir, 0o755)
	registryContent := "# Registry\n" +
		propagator.DiscoverySafetyBeginMarker + "\n| skill-discovery-safety | a | b |\n" + propagator.DiscoverySafetyEndMarker + "\n"
	os.WriteFile(filepath.Join(registryDir, "skill-registry.md"), []byte(registryContent), 0o644)

	deps := statusDeps{
		stat:     os.Stat,
		readFile: os.ReadFile,
		loadSettings: func(_ string) (map[string]interface{}, error) {
			return settingsData, nil
		},
		home: func() string { return homeDir },
		cwd:  func() string { return cwdDir },
	}

	var outBuf bytes.Buffer
	allOK, degraded := statusCore(&outBuf, deps)
	out := outBuf.String()

	if !allOK {
		t.Errorf("statusCore: expected allOK=true (no hard FAIL) for missing minimalism block; output:\n%s", out)
	}
	if !degraded {
		t.Errorf("statusCore: expected degraded=true for missing minimalism block; output:\n%s", out)
	}
	if !strings.Contains(out, "[WARN]") {
		t.Errorf("statusCore: output should contain [WARN] for degraded registry; output:\n%s", out)
	}
	if !strings.Contains(out, "minimalism-contract-scope") {
		t.Errorf("statusCore: WARN note should name which block is absent; output:\n%s", out)
	}
}

// TC-STATUS-10: registry present but scoped block MISSING → WARN/degraded.
// statusCore returns allOK=true (no hard FAIL) and degraded=true, and the
// report shows [WARN]. This is the fail-loud-but-not-fatal tier.
func TestStatusCore_RegistryScopedBlockMissing_Degraded(t *testing.T) {
	homeDir, binaryPath := buildFakeHomeWithBinary(t)
	buildFakeContract(t, homeDir)

	settingsData := buildSettingsWithHooks(binaryPath)

	cwdDir := t.TempDir()
	registryDir := filepath.Join(cwdDir, ".atl")
	os.MkdirAll(registryDir, 0o755)
	// Present and non-empty, but with NO scoped marker block.
	registryContent := "# Registry\n\n## Skills\n| a | b | c |\n"
	os.WriteFile(filepath.Join(registryDir, "skill-registry.md"), []byte(registryContent), 0o644)

	deps := statusDeps{
		stat:     os.Stat,
		readFile: os.ReadFile,
		loadSettings: func(_ string) (map[string]interface{}, error) {
			return settingsData, nil
		},
		home: func() string { return homeDir },
		cwd:  func() string { return cwdDir },
	}

	var outBuf bytes.Buffer
	allOK, degraded := statusCore(&outBuf, deps)
	out := outBuf.String()

	if !allOK {
		t.Errorf("statusCore: expected allOK=true (no hard FAIL) for missing scoped block; output:\n%s", out)
	}
	if !degraded {
		t.Errorf("statusCore: expected degraded=true for missing scoped block; output:\n%s", out)
	}
	if !strings.Contains(out, "[WARN]") {
		t.Errorf("statusCore: output should contain [WARN] for degraded registry; output:\n%s", out)
	}
	if !strings.Contains(out, "scoped block") || !strings.Contains(out, "missing") {
		t.Errorf("statusCore: output should mention 'scoped block' and 'missing'; output:\n%s", out)
	}
}

// TC-STATUS-11: registry present but EMPTY/whitespace-only → hard FAIL.
// An emptied registry is the incident's misread state and must be loud, never
// treated as "zero skills".
func TestStatusCore_RegistryEmpty_Fails(t *testing.T) {
	homeDir, binaryPath := buildFakeHomeWithBinary(t)
	buildFakeContract(t, homeDir)

	settingsData := buildSettingsWithHooks(binaryPath)

	cwdDir := t.TempDir()
	registryDir := filepath.Join(cwdDir, ".atl")
	os.MkdirAll(registryDir, 0o755)
	os.WriteFile(filepath.Join(registryDir, "skill-registry.md"), []byte("   \n\t\n"), 0o644)

	deps := statusDeps{
		stat:     os.Stat,
		readFile: os.ReadFile,
		loadSettings: func(_ string) (map[string]interface{}, error) {
			return settingsData, nil
		},
		home: func() string { return homeDir },
		cwd:  func() string { return cwdDir },
	}

	var outBuf bytes.Buffer
	allOK, _ := statusCore(&outBuf, deps)
	out := outBuf.String()

	if allOK {
		t.Errorf("statusCore: expected allOK=false for empty registry; output:\n%s", out)
	}
	if !strings.Contains(out, "[FAIL]") {
		t.Errorf("statusCore: empty registry should produce [FAIL]; output:\n%s", out)
	}
	if !strings.Contains(out, "EMPTY") {
		t.Errorf("statusCore: empty registry note should mention EMPTY; output:\n%s", out)
	}
}

// TC-STATUS-12: registry present but UNREADABLE (real OS error, not IsNotExist)
// → hard FAIL. A genuine OS error must surface, never be downgraded to OK.
func TestStatusCore_RegistryUnreadable_Fails(t *testing.T) {
	homeDir, binaryPath := buildFakeHomeWithBinary(t)
	buildFakeContract(t, homeDir)

	settingsData := buildSettingsWithHooks(binaryPath)
	cwdDir := t.TempDir()

	// readFile returns a non-IsNotExist error only for the registry path.
	registryPath := filepath.Join(cwdDir, ".atl", "skill-registry.md")
	readFile := func(path string) ([]byte, error) {
		if path == registryPath {
			return nil, errors.New("permission denied")
		}
		return os.ReadFile(path)
	}

	deps := statusDeps{
		stat:     os.Stat,
		readFile: readFile,
		loadSettings: func(_ string) (map[string]interface{}, error) {
			return settingsData, nil
		},
		home: func() string { return homeDir },
		cwd:  func() string { return cwdDir },
	}

	var outBuf bytes.Buffer
	allOK, _ := statusCore(&outBuf, deps)
	out := outBuf.String()

	if allOK {
		t.Errorf("statusCore: expected allOK=false for unreadable registry; output:\n%s", out)
	}
	if !strings.Contains(out, "[FAIL]") || !strings.Contains(out, "cannot read") {
		t.Errorf("statusCore: unreadable registry should FAIL with 'cannot read'; output:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// propagate --require-registry
// ---------------------------------------------------------------------------

// TC-PROP-REQUIRE-1: --require-registry + registry absent → exit 1 + stderr.
// Default (no flag) keeps the silent no-op; the flag turns absence into a loud
// error so an expected-but-missing registry is never an invisible no-op.
func TestRunPropagateCore_RequireRegistry_AbsentFails(t *testing.T) {
	contractPath := "/fake/contract.md"
	registryPath := "/project/.atl/skill-registry.md"

	_, stderr, exitCode := capturePropagateCore(
		[]string{"--registry", registryPath, "--contract-file", contractPath, "--require-registry"},
		map[string][]byte{contractPath: []byte(testContractContent)},
		map[string]error{registryPath: os.ErrNotExist},
		nil,
	)

	if exitCode != 1 {
		t.Errorf("--require-registry absent: expected exit 1, got %d", exitCode)
	}
	if !strings.Contains(stderr, "registry required") {
		t.Errorf("--require-registry absent: stderr should mention 'registry required'; got %q", stderr)
	}
}

// ---------------------------------------------------------------------------
// embedded skill-discovery-safety contract
// ---------------------------------------------------------------------------

// TC-EMBED-PROP-1: --embedded-contract skill-discovery-safety writes a DISTINCT
// marker block (skill-discovery-safety-scope), never the minimalism-contract one,
// so the two contracts cannot fight over the same block.
func TestRunPropagateCore_EmbeddedSafetyContract_DistinctBlock(t *testing.T) {
	registryPath := "/project/.atl/skill-registry.md"
	written := make(map[string][]byte)

	stdout, stderr, exitCode := capturePropagateCore(
		[]string{"--registry", registryPath, "--embedded-contract", "skill-discovery-safety"},
		map[string][]byte{registryPath: []byte(minimalRegistry)},
		nil,
		written,
	)

	if exitCode != -1 {
		t.Fatalf("embedded safety: unexpected exit %d; stderr: %s", exitCode, stderr)
	}
	out := string(written[registryPath])
	if !strings.Contains(out, propagator.DiscoverySafetyBeginMarker) {
		t.Errorf("embedded safety: registry should contain the distinct BEGIN marker; out:\n%s", out)
	}
	if strings.Contains(out, propagator.BeginMarker) {
		t.Errorf("embedded safety: must NOT write the minimalism-contract block; out:\n%s", out)
	}
	if !strings.Contains(out, "skill-discovery-safety") {
		t.Errorf("embedded safety: row label should be skill-discovery-safety; out:\n%s", out)
	}
	if !strings.Contains(stdout, "skill-discovery-safety scoped row inserted/updated") {
		t.Errorf("embedded safety: stdout should confirm the labeled row; got %q", stdout)
	}
}

// TC-EMBED-PROP-2: unknown embedded contract → exit 1 + stderr (fail loud).
func TestRunPropagateCore_EmbeddedUnknown_Fails(t *testing.T) {
	registryPath := "/project/.atl/skill-registry.md"
	_, stderr, exitCode := capturePropagateCore(
		[]string{"--registry", registryPath, "--embedded-contract", "does-not-exist"},
		map[string][]byte{registryPath: []byte(minimalRegistry)},
		nil, nil,
	)
	if exitCode != 1 {
		t.Errorf("unknown embedded contract: expected exit 1, got %d", exitCode)
	}
	if !strings.Contains(stderr, "unknown embedded contract") {
		t.Errorf("unknown embedded contract: stderr should mention it; got %q", stderr)
	}
}

// TC-EMBED-PROP-3: minimalism + safety blocks coexist — propagating the safety
// contract into a registry that already has the minimalism block leaves the
// minimalism block intact and adds a second, distinct block.
func TestRunPropagateCore_EmbeddedSafety_CoexistsWithMinimalism(t *testing.T) {
	registryPath := "/project/.atl/skill-registry.md"

	// Registry already carries the minimalism-contract block.
	base := minimalRegistry + "\n" + propagator.BeginMarker +
		"\n| minimalism-contract | x | y |\n" + propagator.EndMarker + "\n"

	written := make(map[string][]byte)
	_, stderr, exitCode := capturePropagateCore(
		[]string{"--registry", registryPath, "--embedded-contract", "skill-discovery-safety"},
		map[string][]byte{registryPath: []byte(base)},
		nil, written,
	)
	if exitCode != -1 {
		t.Fatalf("coexist: unexpected exit %d; stderr: %s", exitCode, stderr)
	}
	out := string(written[registryPath])
	if !strings.Contains(out, propagator.BeginMarker) {
		t.Errorf("coexist: minimalism block must remain; out:\n%s", out)
	}
	if !strings.Contains(out, propagator.DiscoverySafetyBeginMarker) {
		t.Errorf("coexist: safety block must be added; out:\n%s", out)
	}
}

// TC-EMBED-GATE-1: gate-task with --embedded-contract injects the safety contract
// path into an in-scope sub-agent prompt (sdd-explore) without any contract file.
func TestGateTaskCore_EmbeddedSafetyContract_Injects(t *testing.T) {
	input := `{"tool_name":"Agent","tool_input":{"description":"explore","subagent_type":"sdd-explore","prompt":"Do the explore phase."}}`
	contractAbsPath := "/home/user/.claude/engine/skill-discovery-safety.md"

	var outBuf, errBuf bytes.Buffer
	gateTaskCore(
		[]string{"--embedded-contract", "skill-discovery-safety", "--contract-path", contractAbsPath},
		strings.NewReader(input), &outBuf, &errBuf,
		func(_ string) ([]byte, error) { return nil, errors.New("must not read a file in embedded mode") },
	)

	out := outBuf.String()
	if !strings.Contains(out, contractAbsPath) {
		t.Errorf("embedded gate: expected injection of %q; got %q", contractAbsPath, out)
	}
	if !strings.Contains(out, `"hookEventName":"PreToolUse"`) {
		t.Errorf("embedded gate: response should carry PreToolUse hookSpecificOutput; got %q", out)
	}
}

// TC-EMBED-GATE-2: gate-task with --embedded-contract passes through an
// out-of-scope sub-agent (sdd-archive is excluded) — fail-safe '{}'.
func TestGateTaskCore_EmbeddedSafetyContract_PassThroughExcluded(t *testing.T) {
	input := `{"tool_name":"Agent","tool_input":{"description":"archive","subagent_type":"sdd-archive","prompt":"Do the archive phase."}}`

	var outBuf, errBuf bytes.Buffer
	gateTaskCore(
		[]string{"--embedded-contract", "skill-discovery-safety"},
		strings.NewReader(input), &outBuf, &errBuf,
		func(_ string) ([]byte, error) { return nil, errors.New("must not read a file in embedded mode") },
	)

	if strings.TrimSpace(outBuf.String()) != "{}" {
		t.Errorf("embedded gate excluded: expected pass-through '{}'; got %q", outBuf.String())
	}
}

// TC-EMBED-GATE-3: gate-task --embedded-contract without --contract-path must
// inject the embedded contract's own default path (skills/_shared/skill-discovery-safety.md),
// NOT the minimalism-contract default. This validates fix C: contractPath must
// fall back to spec.defaultPath rather than the hardcoded minimalism path.
func TestGateTaskCore_EmbeddedSafetyContract_DefaultPath(t *testing.T) {
	input := `{"tool_name":"Agent","tool_input":{"description":"explore","subagent_type":"sdd-explore","prompt":"Do the explore phase."}}`

	var outBuf, errBuf bytes.Buffer
	gateTaskCore(
		// NO --contract-path: the embedded contract's defaultPath must be used.
		[]string{"--embedded-contract", "skill-discovery-safety"},
		strings.NewReader(input), &outBuf, &errBuf,
		func(_ string) ([]byte, error) { return nil, errors.New("must not read a file in embedded mode") },
	)

	out := outBuf.String()
	const safetyDefaultPath = "skills/_shared/skill-discovery-safety.md"
	const minimalistPath = "skills/_shared/minimalism-contract.md"
	if !strings.Contains(out, safetyDefaultPath) {
		t.Errorf("embedded gate default path: expected injection of %q; got %q", safetyDefaultPath, out)
	}
	if strings.Contains(out, minimalistPath) {
		t.Errorf("embedded gate default path: must NOT inject minimalism path %q; got %q", minimalistPath, out)
	}
}

// ---------------------------------------------------------------------------
// T-16b: anti-generic-design embedded contract wiring (R-102, PR-2 of the
// anti-generic-design-runtime-wiring chain)
// ---------------------------------------------------------------------------

// TC-EMBED-DESIGN-1: embeddedContract("anti-generic-design") resolves to the
// R-101 marker pair, the compiled-in asset content, the "anti-generic-design"
// row label, and the skills/_shared default path.
func TestEmbeddedContract_AntiGenericDesign(t *testing.T) {
	spec, ok := embeddedContract("anti-generic-design")
	if !ok {
		t.Fatalf("embeddedContract(\"anti-generic-design\"): expected ok=true, got ok=false")
	}
	if spec.content != assets.AntiGenericDesign {
		t.Errorf("embeddedContract: content should be assets.AntiGenericDesign")
	}
	if spec.beginMarker != propagator.AntiGenericDesignBeginMarker {
		t.Errorf("embeddedContract: beginMarker = %q, want %q", spec.beginMarker, propagator.AntiGenericDesignBeginMarker)
	}
	if spec.endMarker != propagator.AntiGenericDesignEndMarker {
		t.Errorf("embeddedContract: endMarker = %q, want %q", spec.endMarker, propagator.AntiGenericDesignEndMarker)
	}
	if spec.rowLabel != "anti-generic-design" {
		t.Errorf("embeddedContract: rowLabel = %q, want %q", spec.rowLabel, "anti-generic-design")
	}
	if spec.defaultPath != "skills/_shared/anti-generic-design.md" {
		t.Errorf("embeddedContract: defaultPath = %q, want %q", spec.defaultPath, "skills/_shared/anti-generic-design.md")
	}
}

// TC-EMBED-PROP-DESIGN-1: propagate --embedded-contract anti-generic-design
// does not hit the "unknown embedded contract" fail-loud branch, and the
// registry row's Path cell reads skills/_shared/anti-generic-design.md.
func TestRunPropagateCore_EmbeddedDesignContract_ResolvesAndWritesPath(t *testing.T) {
	registryPath := "/project/.atl/skill-registry.md"
	written := make(map[string][]byte)

	stdout, stderr, exitCode := capturePropagateCore(
		[]string{"--registry", registryPath, "--embedded-contract", "anti-generic-design"},
		map[string][]byte{registryPath: []byte(minimalRegistry)},
		nil,
		written,
	)

	if exitCode != -1 {
		t.Fatalf("embedded design: unexpected exit %d; stderr: %s", exitCode, stderr)
	}
	if strings.Contains(stderr, "unknown embedded contract") {
		t.Errorf("embedded design: must not hit the unknown embedded contract branch; stderr: %s", stderr)
	}
	out := string(written[registryPath])
	if !strings.Contains(out, propagator.AntiGenericDesignBeginMarker) {
		t.Errorf("embedded design: registry should contain the distinct BEGIN marker; out:\n%s", out)
	}
	if !strings.Contains(out, "skills/_shared/anti-generic-design.md") {
		t.Errorf("embedded design: row Path cell should read skills/_shared/anti-generic-design.md; out:\n%s", out)
	}
	if !strings.Contains(stdout, "anti-generic-design scoped row inserted/updated") {
		t.Errorf("embedded design: stdout should confirm the labeled row; got %q", stdout)
	}
}

// TC-EMBED-GATE-DESIGN-1: gate-task --embedded-contract anti-generic-design on
// a well-formed PreToolUse Agent payload for an in-scope phase (sdd-apply)
// emits an injection referencing the contract path, not the {} fail-safe.
func TestGateTaskCore_EmbeddedDesignContract_Injects(t *testing.T) {
	input := `{"tool_name":"Agent","tool_input":{"description":"apply","subagent_type":"sdd-apply","prompt":"Do the apply phase."}}`
	contractAbsPath := "/home/user/.claude/engine/anti-generic-design.md"

	var outBuf, errBuf bytes.Buffer
	gateTaskCore(
		[]string{"--embedded-contract", "anti-generic-design", "--contract-path", contractAbsPath},
		strings.NewReader(input), &outBuf, &errBuf,
		func(_ string) ([]byte, error) { return nil, errors.New("must not read a file in embedded mode") },
	)

	out := outBuf.String()
	if out == "" || strings.TrimSpace(out) == "{}" {
		t.Fatalf("embedded design gate: expected an injection response, got fail-safe pass-through %q; stderr: %s", out, errBuf.String())
	}
	if !strings.Contains(out, contractAbsPath) {
		t.Errorf("embedded design gate: expected injection of %q; got %q", contractAbsPath, out)
	}
	if !strings.Contains(out, `"hookEventName":"PreToolUse"`) {
		t.Errorf("embedded design gate: response should carry PreToolUse hookSpecificOutput; got %q", out)
	}
}

// TC-CHECKREG-DESIGN: checkRegistry recognizes the third
// anti-generic-design-scope marker alongside the two pre-existing blocks.
// All three present -> ok, not degraded. Only anti-generic-design-scope
// missing -> ok, degraded, note names it.
func TestCheckRegistry_ThreeBlockCombinations(t *testing.T) {
	const registryPath = "/project/.atl/skill-registry.md"

	allThree := "# Registry\n" +
		propagator.BeginMarker + "\n| minimalism-contract | x | y |\n" + propagator.EndMarker + "\n" +
		propagator.DiscoverySafetyBeginMarker + "\n| skill-discovery-safety | a | b |\n" + propagator.DiscoverySafetyEndMarker + "\n" +
		propagator.AntiGenericDesignBeginMarker + "\n| anti-generic-design | c | d |\n" + propagator.AntiGenericDesignEndMarker + "\n"

	onlyMinimalismAndSafety := "# Registry\n" +
		propagator.BeginMarker + "\n| minimalism-contract | x | y |\n" + propagator.EndMarker + "\n" +
		propagator.DiscoverySafetyBeginMarker + "\n| skill-discovery-safety | a | b |\n" + propagator.DiscoverySafetyEndMarker + "\n"

	tests := []struct {
		name         string
		content      string
		wantOK       bool
		wantDegraded bool
		wantNoteHas  string
	}{
		{
			name:         "all three blocks present",
			content:      allThree,
			wantOK:       true,
			wantDegraded: false,
			wantNoteHas:  "scoped block",
		},
		{
			name:         "only anti-generic-design-scope missing",
			content:      onlyMinimalismAndSafety,
			wantOK:       true,
			wantDegraded: true,
			wantNoteHas:  "anti-generic-design-scope",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			readFile := func(_ string) ([]byte, error) { return []byte(tc.content), nil }
			result := checkRegistry(registryPath, readFile)
			if result.ok != tc.wantOK {
				t.Errorf("checkRegistry(%s): ok = %v, want %v (note: %s)", tc.name, result.ok, tc.wantOK, result.note)
			}
			if result.degraded != tc.wantDegraded {
				t.Errorf("checkRegistry(%s): degraded = %v, want %v (note: %s)", tc.name, result.degraded, tc.wantDegraded, result.note)
			}
			if !strings.Contains(result.note, tc.wantNoteHas) {
				t.Errorf("checkRegistry(%s): note = %q, want it to contain %q", tc.name, result.note, tc.wantNoteHas)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// T-17: skills subcommand tests (R-040..R-042, SC-13)
// ---------------------------------------------------------------------------

// TestRunSkillsCore_list verifies that runSkillsCore dispatches "list" correctly
// and exits 0 on a valid registry temp file (smoke test, R-019).
func TestRunSkillsCore_list(t *testing.T) {
	dir := t.TempDir()
	regPath := filepath.Join(dir, "registry.yaml")
	const yaml = `version: "1"
skills:
  - id: smoke-skill
    path: smoke-skill
    source:
      type: custom
    install:
      defaultScope: global
      targets:
        - claude
    lifecycle:
      updateStrategy: overlay-only
`
	if err := os.WriteFile(regPath, []byte(yaml), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	var outBuf, errBuf bytes.Buffer
	exitCode := 0
	runSkillsCore("list", []string{"list", "--registry", regPath}, &outBuf, &errBuf, func(c int) { exitCode = c })
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0; stderr=%q", exitCode, errBuf.String())
	}
	if !strings.Contains(outBuf.String(), "smoke-skill") {
		t.Errorf("stdout %q missing smoke-skill entry", outBuf.String())
	}
}

// TestRunSkillsCore_unknown_verb verifies that an unrecognised verb exits 1 (ADR-4).
func TestRunSkillsCore_unknown_verb(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	exitCode := 0
	runSkillsCore("bogus", []string{"bogus"}, &outBuf, &errBuf, func(c int) { exitCode = c })
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}
	if errBuf.Len() == 0 {
		t.Error("stderr must be non-empty on unknown verb")
	}
}

// TestApplyIgnoresRegistry is a SC-13 regression guard: cmd_apply must never
// open skills.registry.yaml. This is an integration-style smoke test; it is
// skipped under -short to keep the fast unit-test suite lean.
//
// Invariant asserted: in a directory that contains NO skills.registry.yaml,
// runPropagateCore succeeds (registry absent is a clean no-op) while
// runSkillsCore("list") fails with exit 1 (skills subcommand requires the
// YAML registry). This proves that propagate never opens skills.registry.yaml.
func TestApplyIgnoresRegistry(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: skipped under -short")
	}

	dir := t.TempDir()
	// No skills.registry.yaml in dir (the file is deliberately absent).

	// The .atl/skill-registry.md (markdown registry for agents) IS present;
	// this is what propagate reads via --registry. It is distinct from
	// skills.registry.yaml (the YAML skills descriptor).
	atlDir := filepath.Join(dir, ".atl")
	if err := os.MkdirAll(atlDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mdRegistryPath := filepath.Join(atlDir, "skill-registry.md")
	if err := os.WriteFile(mdRegistryPath, []byte("# Skill Registry\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	contractPath := filepath.Join(dir, "contract.md")
	if err := os.WriteFile(contractPath, []byte(testContractContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// --- Assert 1: propagate succeeds even with no skills.registry.yaml.
	// runPropagateCore reads mdRegistryPath (--registry), never skills.registry.yaml.
	var propOut, propErr bytes.Buffer
	propExitCode := -1
	runPropagateCore(
		[]string{"--registry", mdRegistryPath, "--contract-file", contractPath},
		&propOut, &propErr,
		os.ReadFile,
		func(_ string, _ []byte, _ os.FileMode) error { return nil },
		func(c int) { propExitCode = c },
	)
	if propExitCode == 1 {
		t.Errorf("SC-13: runPropagateCore must not fail when skills.registry.yaml is absent; stderr=%q", propErr.String())
	}

	// --- Assert 2: runSkillsCore("list") DOES fail (exit 1) when the YAML
	// registry is absent — confirming it is the *only* subcommand that reads it.
	// This is the expected failure that proves the interface boundary is sharp.
	var skillsOut, skillsErr bytes.Buffer
	skillsExitCode := -1
	missingYAML := filepath.Join(dir, "skills.registry.yaml") // does not exist
	runSkillsCore("list", []string{"list", "--registry", missingYAML},
		&skillsOut, &skillsErr, func(c int) { skillsExitCode = c })
	if skillsExitCode != 1 {
		t.Errorf("SC-13: runSkillsCore(list) must exit 1 when skills.registry.yaml is absent; got exit %d", skillsExitCode)
	}
}

// ---------------------------------------------------------------------------
// propagate write-race fix: empty-registry guard, atomic write, registry lock
// ---------------------------------------------------------------------------

// TC-RACE-1: registry exists but is empty (or whitespace-only) → exit 1,
// stderr diagnostic, and NO write. Propagating over an emptied registry would
// replace the whole file with a lone marker block — the incident state.
func TestRunPropagateCore_EmptyRegistry_FailLoudNoWrite(t *testing.T) {
	for _, content := range []string{"", "   \n\t\n"} {
		writtenFiles := make(map[string][]byte)
		_, stderr, exitCode := capturePropagateCore(
			[]string{"--registry", "/fake/registry.md", "--contract-file", "/fake/contract.md"},
			map[string][]byte{
				"/fake/contract.md": []byte(testContractContent),
				"/fake/registry.md": []byte(content),
			},
			nil,
			writtenFiles,
		)
		if exitCode != 1 {
			t.Errorf("empty registry %q: expected exit 1, got %d", content, exitCode)
		}
		if !strings.Contains(stderr, "empty") || !strings.Contains(stderr, "refusing") {
			t.Errorf("empty registry %q: stderr should say empty + refusing; got %q", content, stderr)
		}
		if len(writtenFiles) != 0 {
			t.Errorf("empty registry %q: nothing must be written; wrote %v", content, writtenFiles)
		}
	}
}

// TC-RACE-2: atomicWriteFile writes the exact content into place and leaves no
// temp file behind, both when creating a new file and when replacing one.
func TestAtomicWriteFile_ContentAndNoLeftoverTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "skill-registry.md")

	if err := atomicWriteFile(path, []byte("first version\n"), 0o644); err != nil {
		t.Fatalf("atomicWriteFile (create): %v", err)
	}
	if err := atomicWriteFile(path, []byte("second version\n"), 0o644); err != nil {
		t.Fatalf("atomicWriteFile (replace): %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading result: %v", err)
	}
	if string(got) != "second version\n" {
		t.Errorf("content mismatch: got %q", string(got))
	}

	// The directory must contain ONLY the target file — no leftover temp files.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "skill-registry.md" {
			t.Errorf("leftover file in dir: %s", e.Name())
		}
	}
}

// TC-RACE-3: atomicWriteFile is wired as the real registry writer — the end-to-end
// propagate path against a real temp dir must produce the scoped block on disk.
func TestRunPropagateCore_AtomicWriter_EndToEnd(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, "skill-registry.md")
	contractPath := filepath.Join(dir, "contract.md")
	if err := os.WriteFile(registryPath, []byte(minimalRegistry), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(contractPath, []byte(testContractContent), 0o644); err != nil {
		t.Fatal(err)
	}

	var outBuf, errBuf bytes.Buffer
	exitCode := -1
	runPropagateCore(
		[]string{"--registry", registryPath, "--contract-file", contractPath},
		&outBuf, &errBuf,
		os.ReadFile,
		atomicWriteFile, // the production writer
		func(c int) { exitCode = c },
	)

	if exitCode != -1 {
		t.Fatalf("unexpected exit %d; stderr: %s", exitCode, errBuf.String())
	}
	got, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), propagator.BeginMarker) {
		t.Errorf("registry on disk must contain scoped BEGIN marker; got:\n%s", string(got))
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 { // registry + contract only, no temp leftovers
		t.Errorf("expected exactly 2 files in dir, got %d: %v", len(entries), entries)
	}
}

// TC-RACE-4: acquireRegistryLock takes an exclusive flock — while held, a
// second non-blocking flock on the same path fails with EWOULDBLOCK; after
// release it succeeds. flock is per open-file-description, so a second fd in
// the same process is an honest contention probe.
func TestAcquireRegistryLock_Exclusive(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "skill-registry.md.lock")

	release, err := acquireRegistryLock(lockPath)
	if err != nil {
		t.Fatalf("acquireRegistryLock: %v", err)
	}

	probe := func() error {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
		if err != nil {
			t.Fatalf("opening probe fd: %v", err)
		}
		defer f.Close()
		return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	}

	if err := probe(); err != syscall.EWOULDBLOCK {
		t.Errorf("lock held: probe flock should fail with EWOULDBLOCK; got %v", err)
	}

	release()

	if err := probe(); err != nil {
		t.Errorf("lock released: probe flock should succeed; got %v", err)
	}
}

// TC-RACE-5: registryPathFromArgs extracts and cleans --registry, empty if absent.
func TestRegistryPathFromArgs(t *testing.T) {
	if got := registryPathFromArgs([]string{"--registry", "/a/b/../c/registry.md"}); got != "/a/c/registry.md" {
		t.Errorf("cleaned path: got %q", got)
	}
	if got := registryPathFromArgs([]string{"--contract-file", "/x.md"}); got != "" {
		t.Errorf("absent flag: got %q, want empty", got)
	}
	if got := registryPathFromArgs([]string{"--registry"}); got != "" {
		t.Errorf("dangling flag: got %q, want empty", got)
	}
}

// TC-RACE-6: concurrency regression. N goroutines run the same composition the
// real runPropagate uses — acquire the registry lock, then run the core with
// os.ReadFile + atomicWriteFile — alternating between the two contracts that
// fire concurrently in production (minimalism via --contract-file and the
// embedded skill-discovery-safety). The registry must never be gutted: the
// final file keeps the original row and gains BOTH scoped blocks.
func TestPropagate_ConcurrentProcesses_RegistryNeverGutted(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, "skill-registry.md")
	contractPath := filepath.Join(dir, "contract.md")
	if err := os.WriteFile(registryPath, []byte(minimalRegistry), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(contractPath, []byte(testContractContent), 0o644); err != nil {
		t.Fatal(err)
	}

	argSets := [][]string{
		{"--registry", registryPath, "--contract-file", contractPath},
		{"--registry", registryPath, "--embedded-contract", "skill-discovery-safety"},
	}

	const rounds = 8
	var wg sync.WaitGroup
	errCh := make(chan string, rounds*len(argSets))
	for r := 0; r < rounds; r++ {
		for _, args := range argSets {
			wg.Add(1)
			go func(args []string) {
				defer wg.Done()
				release, err := acquireRegistryLock(registryPath + ".lock")
				if err != nil {
					errCh <- "lock: " + err.Error()
					return
				}
				defer release()
				var outBuf, errBuf bytes.Buffer
				exitCode := -1
				runPropagateCore(args, &outBuf, &errBuf,
					os.ReadFile, atomicWriteFile,
					func(c int) { exitCode = c })
				if exitCode != -1 {
					errCh <- "exit " + errBuf.String()
				}
			}(args)
		}
	}
	wg.Wait()
	close(errCh)
	for msg := range errCh {
		t.Errorf("concurrent propagate failed: %s", msg)
	}

	got, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(got)
	if !strings.Contains(content, "pre-sdd-contracts") {
		t.Errorf("original registry row was lost:\n%s", content)
	}
	if !strings.Contains(content, propagator.BeginMarker) {
		t.Errorf("minimalism scoped block missing:\n%s", content)
	}
	if !strings.Contains(content, propagator.DiscoverySafetyBeginMarker) {
		t.Errorf("skill-discovery-safety scoped block missing:\n%s", content)
	}
}
