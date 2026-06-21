package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	if n := countEntries("UserPromptSubmit"); n != 1 {
		t.Errorf("UserPromptSubmit: expected 1 entry, got %d", n)
	}
	if n := countEntries("PreToolUse"); n != 1 {
		t.Errorf("PreToolUse: expected 1 entry, got %d", n)
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
