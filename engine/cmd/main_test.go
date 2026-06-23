package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator"
	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
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
			"/fake/contract.md": []byte("no frontmatter"),
			"/fake/registry.md": []byte(minimalRegistry),
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
	result := statusCore(&outBuf, deps)
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
	result := statusCore(&outBuf, deps)
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
	result := statusCore(&outBuf, deps)
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
	result := statusCore(&outBuf, deps)
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
	result := statusCore(&outBuf, deps)
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
	result := statusCore(&outBuf, deps)
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
	result := statusCore(&outBuf, deps)
	out := outBuf.String()

	if result {
		t.Errorf("statusCore: expected false when contract frontmatter broken; output:\n%s", out)
	}
	if !strings.Contains(out, "frontmatter") {
		t.Errorf("statusCore: output should mention 'frontmatter'; output:\n%s", out)
	}
}

// TC-STATUS-8: registry check best-effort — even when registry is absent,
// statusCore returns true (if other checks pass). The registry check never
// fails the suite.
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
	result := statusCore(&outBuf, deps)
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

// TC-STATUS-9: registry present WITH scoped block → [OK  ] with note "scoped block present".
func TestStatusCore_RegistryScopedBlockPresent(t *testing.T) {
	homeDir, binaryPath := buildFakeHomeWithBinary(t)
	buildFakeContract(t, homeDir)

	settingsData := buildSettingsWithHooks(binaryPath)

	cwdDir := t.TempDir()
	registryDir := filepath.Join(cwdDir, ".atl")
	os.MkdirAll(registryDir, 0o755)
	registryContent := "# Registry\n" + propagator.BeginMarker + "\n| row |\n" + propagator.EndMarker + "\n"
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
	result := statusCore(&outBuf, deps)
	out := outBuf.String()

	if !result {
		t.Errorf("statusCore: expected true when scoped block present; output:\n%s", out)
	}
	if !strings.Contains(out, "scoped block present") {
		t.Errorf("statusCore: output should say 'scoped block present'; output:\n%s", out)
	}
}

func TestLifecycleCoreAcceptsTargetAwareActions(t *testing.T) {
	tests := []struct {
		name         string
		action       engineRuntime.Action
		target       string
		wantContains []string
		wantExit     int
	}{
		{name: "apply claude fails on unsupported", action: engineRuntime.ActionApply, target: "claude", wantContains: []string{"apply", "claude", "unsupported"}, wantExit: 1},
		{name: "status opencode remains read-only", action: engineRuntime.ActionStatus, target: "opencode", wantContains: []string{"status", "opencode", "unsupported"}, wantExit: -1},
		{name: "sync codex remains read-only", action: engineRuntime.ActionSyncCheck, target: "codex", wantContains: []string{"sync-check", "codex", "unsupported"}, wantExit: -1},
		{name: "update all expands before failing", action: engineRuntime.ActionUpdate, target: "all", wantContains: []string{"update", "claude", "opencode", "codex"}, wantExit: 1},
		{name: "rollback claude fails on unsupported", action: engineRuntime.ActionRollback, target: "claude", wantContains: []string{"rollback", "claude", "unsupported"}, wantExit: 1},
		{name: "uninstall opencode fails on unsupported", action: engineRuntime.ActionUninstall, target: "opencode", wantContains: []string{"uninstall", "opencode", "unsupported"}, wantExit: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var outBuf, errBuf bytes.Buffer
			exitCode := -1

			runLifecycleCore(tt.action, []string{tt.target}, &outBuf, &errBuf, lifecycleDeps{
				adapterFor: newTestLifecycleAdapter,
			}, func(code int) { exitCode = code })

			if exitCode != tt.wantExit {
				t.Fatalf("exit code = %d, want %d; stdout: %s stderr: %s", exitCode, tt.wantExit, outBuf.String(), errBuf.String())
			}
			out := outBuf.String()
			for _, want := range tt.wantContains {
				if !strings.Contains(out, want) {
					t.Errorf("output should include %q; got %q", want, out)
				}
			}
		})
	}
}

type testLifecycleAdapter struct {
	target engineRuntime.Target
	status engineRuntime.CapabilityStatus
}

func newTestLifecycleAdapter(target engineRuntime.Target) engineRuntime.Adapter {
	return testLifecycleAdapter{target: target, status: engineRuntime.CapabilityUnsupported}
}

func newTestLifecycleAdapterWithStatus(status engineRuntime.CapabilityStatus) func(engineRuntime.Target) engineRuntime.Adapter {
	return func(target engineRuntime.Target) engineRuntime.Adapter {
		return testLifecycleAdapter{target: target, status: status}
	}
}

func (a testLifecycleAdapter) Target() engineRuntime.Target { return a.target }
func (a testLifecycleAdapter) Apply() engineRuntime.LifecycleResult {
	return a.result(engineRuntime.ActionApply)
}
func (a testLifecycleAdapter) Install() engineRuntime.LifecycleResult {
	return a.result(engineRuntime.ActionInstall)
}
func (a testLifecycleAdapter) Status() engineRuntime.LifecycleResult {
	return a.result(engineRuntime.ActionStatus)
}
func (a testLifecycleAdapter) SyncCheck() engineRuntime.LifecycleResult {
	return a.result(engineRuntime.ActionSyncCheck)
}
func (a testLifecycleAdapter) Update() engineRuntime.LifecycleResult {
	return a.result(engineRuntime.ActionUpdate)
}
func (a testLifecycleAdapter) Rollback() engineRuntime.LifecycleResult {
	return a.result(engineRuntime.ActionRollback)
}
func (a testLifecycleAdapter) Uninstall() engineRuntime.LifecycleResult {
	return a.result(engineRuntime.ActionUninstall)
}
func (a testLifecycleAdapter) result(action engineRuntime.Action) engineRuntime.LifecycleResult {
	return engineRuntime.NewLifecycleResult(a.target, action, a.status, "test adapter: no filesystem writes", nil)
}

func TestLifecycleCoreReportsClaudeLifecycleHonesty(t *testing.T) {
	// R-102: target-aware Claude lifecycle must not return false supported for hook state.
	tests := []struct {
		action   engineRuntime.Action
		wantExit int
	}{
		{action: engineRuntime.ActionInstall, wantExit: 1},
		{action: engineRuntime.ActionStatus, wantExit: -1},
		{action: engineRuntime.ActionSyncCheck, wantExit: -1},
		{action: engineRuntime.ActionUpdate, wantExit: 1},
		{action: engineRuntime.ActionRollback, wantExit: 1},
		{action: engineRuntime.ActionUninstall, wantExit: 1},
	}
	for _, tt := range tests {
		var outBuf, errBuf bytes.Buffer
		exitCode := -1
		runLifecycleCore(tt.action, []string{"claude"}, &outBuf, &errBuf, lifecycleDeps{
			adapterFor: engineRuntime.NewFoundationAdapter,
		}, func(code int) { exitCode = code })
		if exitCode != tt.wantExit {
			t.Fatalf("Claude %s exit = %d, want %d; stdout=%q stderr=%q", tt.action, exitCode, tt.wantExit, outBuf.String(), errBuf.String())
		}
		out := outBuf.String()
		if !strings.Contains(out, "partial") || !strings.Contains(out, "install-hooks") || !strings.Contains(out, "status-hooks") {
			t.Fatalf("Claude %s should report honest partial legacy lifecycle guidance, got stdout=%q stderr=%q", tt.action, out, errBuf.String())
		}
	}
}

func TestLifecycleCoreExitCodePolicy(t *testing.T) {
	tests := []struct {
		name     string
		action   engineRuntime.Action
		adapter  func(engineRuntime.Target) engineRuntime.Adapter
		wantExit int
	}{
		{name: "mutating unsupported exits non-zero", action: engineRuntime.ActionInstall, adapter: newTestLifecycleAdapterWithStatus(engineRuntime.CapabilityUnsupported), wantExit: 1},
		{name: "mutating partial exits non-zero", action: engineRuntime.ActionUpdate, adapter: newTestLifecycleAdapterWithStatus(engineRuntime.CapabilityPartial), wantExit: 1},
		{name: "mutating supported exits zero", action: engineRuntime.ActionInstall, adapter: newTestLifecycleAdapterWithStatus(engineRuntime.CapabilitySupported), wantExit: -1},
		{name: "mutating restart-required exits zero", action: engineRuntime.ActionUninstall, adapter: newTestLifecycleAdapterWithStatus(engineRuntime.CapabilityRestartRequired), wantExit: -1},
		{name: "status partial preserves zero", action: engineRuntime.ActionStatus, adapter: newTestLifecycleAdapterWithStatus(engineRuntime.CapabilityPartial), wantExit: -1},
		{name: "sync-check unsupported preserves zero", action: engineRuntime.ActionSyncCheck, adapter: newTestLifecycleAdapterWithStatus(engineRuntime.CapabilityUnsupported), wantExit: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var outBuf, errBuf bytes.Buffer
			exitCode := -1
			runLifecycleCore(tt.action, []string{"claude"}, &outBuf, &errBuf, lifecycleDeps{
				adapterFor: tt.adapter,
			}, func(code int) { exitCode = code })
			if exitCode != tt.wantExit {
				t.Fatalf("exit code = %d, want %d; stdout=%q stderr=%q", exitCode, tt.wantExit, outBuf.String(), errBuf.String())
			}
		})
	}
}

func TestLifecycleCoreRejectsUnknownTarget(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	exitCode := -1

	runLifecycleCore(engineRuntime.ActionApply, []string{"future-cli"}, &outBuf, &errBuf, lifecycleDeps{
		adapterFor: engineRuntime.NewFoundationAdapter,
	}, func(code int) { exitCode = code })

	if exitCode != 1 {
		t.Fatalf("unknown target should exit 1, got %d; stdout: %s", exitCode, outBuf.String())
	}
	if !strings.Contains(errBuf.String(), "future-cli") {
		t.Errorf("stderr should name rejected target, got %q", errBuf.String())
	}
}

func TestLifecycleWrappersAndDefaultActionBranches(t *testing.T) {
	// R-101/R-104: public lifecycle wrappers and unknown actions preserve target-specific status output.
	runLifecycle(engineRuntime.ActionStatus, []string{"claude"})
	runStatus([]string{"codex"})

	result := dispatchLifecycle(engineRuntime.NewFoundationAdapter(engineRuntime.Target("future")), engineRuntime.Action("future-action"))
	if result.Target != engineRuntime.Target("future") || result.Status != engineRuntime.CapabilityUnsupported || !strings.Contains(result.Message, "unknown lifecycle action") {
		t.Fatalf("unknown lifecycle action should report unsupported with reason, got %#v", result)
	}
}

func TestDefaultStatusDepsLoadSettingsBranches(t *testing.T) {
	// R-102: production status dependencies distinguish absent, invalid, and valid Claude settings.
	home := t.TempDir()
	t.Setenv("HOME", home)
	deps := defaultStatusDeps()
	settingsPath := filepath.Join(home, ".claude", "settings.json")

	missing, err := deps.loadSettings(settingsPath)
	if err != nil || missing != nil {
		t.Fatalf("missing settings should load as nil without error, got settings=%#v err=%v", missing, err)
	}

	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir settings dir: %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte("not json"), 0o644); err != nil {
		t.Fatalf("write invalid settings: %v", err)
	}
	if _, err := deps.loadSettings(settingsPath); err == nil || !strings.Contains(err.Error(), "invalid JSON") {
		t.Fatalf("invalid settings should return invalid JSON error, got %v", err)
	}

	if err := os.WriteFile(settingsPath, []byte(`{"hooks":{}}`), 0o644); err != nil {
		t.Fatalf("write valid settings: %v", err)
	}
	loaded, err := deps.loadSettings(settingsPath)
	if err != nil || loaded["hooks"] == nil {
		t.Fatalf("valid settings should decode hooks, got settings=%#v err=%v", loaded, err)
	}
	if deps.home() != home {
		t.Fatalf("default status home = %q, want %q", deps.home(), home)
	}
	if deps.cwd() == "" {
		t.Fatal("default status cwd should not be empty")
	}
}

func TestPublicCommandWrappersHappyPaths(t *testing.T) {
	// R-001/R-102: production wrappers execute successful paths without hitting os.Exit.
	t.Run("usage", func(t *testing.T) {
		usage()
	})

	t.Run("runGateTaskMissingContract", func(t *testing.T) {
		oldStdin := os.Stdin
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("pipe stdin: %v", err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("close stdin writer: %v", err)
		}
		os.Stdin = r
		defer func() {
			os.Stdin = oldStdin
			_ = r.Close()
		}()
		runGateTask(nil)
	})

	t.Run("runPropagate", func(t *testing.T) {
		root := t.TempDir()
		registry := filepath.Join(root, ".atl", "skill-registry.md")
		contract := filepath.Join(root, "minimalism-contract.md")
		if err := os.MkdirAll(filepath.Dir(registry), 0o755); err != nil {
			t.Fatalf("mkdir registry dir: %v", err)
		}
		if err := os.WriteFile(registry, []byte("# Registry\n"), 0o644); err != nil {
			t.Fatalf("write registry: %v", err)
		}
		if err := os.WriteFile(contract, []byte(testContractContent), 0o644); err != nil {
			t.Fatalf("write contract: %v", err)
		}
		runPropagate([]string{"--registry", registry, "--contract-file", contract})
		updated, err := os.ReadFile(registry)
		if err != nil {
			t.Fatalf("read propagated registry: %v", err)
		}
		if !strings.Contains(string(updated), "minimalism-contract") {
			t.Fatalf("runPropagate should update registry, got:\n%s", string(updated))
		}
	})

	t.Run("runStatus", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		binaryPath := filepath.Join(home, ".claude", "bin", "gentle-ai-overlay")
		settingsPath := filepath.Join(home, ".claude", "settings.json")
		contractPath := filepath.Join(home, ".claude", "skills", "_shared", "minimalism-contract.md")
		for _, dir := range []string{filepath.Dir(binaryPath), filepath.Dir(settingsPath), filepath.Dir(contractPath)} {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatalf("mkdir %s: %v", dir, err)
			}
		}
		if err := os.WriteFile(binaryPath, []byte("#!/usr/bin/env bash\n"), 0o755); err != nil {
			t.Fatalf("write binary: %v", err)
		}
		settingsJSON := `{"hooks":{"UserPromptSubmit":[{"hooks":[{"command":"/tmp/gentle-ai-overlay propagate"}]}],"PreToolUse":[{"matcher":"Agent","hooks":[{"command":"/tmp/gentle-ai-overlay gate-task"}]}]}}`
		settingsJSON = strings.ReplaceAll(settingsJSON, `\"`, `"`)
		if err := os.WriteFile(settingsPath, []byte(settingsJSON), 0o644); err != nil {
			t.Fatalf("write settings: %v", err)
		}
		if err := os.WriteFile(contractPath, []byte(testContractContent), 0o644); err != nil {
			t.Fatalf("write contract: %v", err)
		}
		runStatus(nil)
	})
}

func TestOverlayShellLifecycleCommandsAndClaudeCompatibility(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("..", "..", "bin", "overlay"))
	if err != nil {
		t.Fatalf("read bin/overlay: %v", err)
	}
	text := string(source)

	// R-101: target lifecycle commands remain adapter-backed.
	for _, want := range []string{
		"install|update|rollback|uninstall",
		"cmd_lifecycle",
		"run_engine_lifecycle",
		"run_engine_lifecycle_readonly",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("bin/overlay should contain %q", want)
		}
	}

	// R-102: legacy Claude hook commands must call the real hook functions,
	// not the target lifecycle status stubs.
	for _, want := range []string{
		"install-hooks)    cmd_install_hooks \"$@\"",
		"uninstall-hooks)  cmd_uninstall_hooks \"$@\"",
		"status-hooks)     cmd_status_hooks \"$@\"",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("legacy hook dispatch should contain %q", want)
		}
	}
	for _, forbidden := range []string{
		"install-hooks)    cmd_lifecycle install --target claude",
		"uninstall-hooks)  cmd_lifecycle uninstall --target claude",
		"status-hooks)     cmd_lifecycle status --target claude",
	} {
		if strings.Contains(text, forbidden) {
			t.Errorf("legacy hook dispatch must not use lifecycle stub %q", forbidden)
		}
	}
}

func TestOverlayShellLifecycleAllTargetsAggregatesFailures(t *testing.T) {
	// R-101: shell dispatch for the default mutating lifecycle target set must
	// run every engine target even when an earlier target returns partial/failure.
	repoRoot := filepath.Join("..", "..")
	overlay, err := filepath.Abs(filepath.Join(repoRoot, "bin", "overlay"))
	if err != nil {
		t.Fatalf("resolve overlay path: %v", err)
	}

	for _, tt := range []struct {
		name string
		args []string
	}{
		{name: "default all", args: []string{"install"}},
		{name: "explicit all", args: []string{"install", "--target", "all"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			invocations := filepath.Join(home, "lifecycle-invocations.log")
			writeFutureFakeLifecycleEngine(t, home)

			cmd := exec.Command(overlay, tt.args...)
			cmd.Dir = repoRoot
			cmd.Env = append(os.Environ(), "HOME="+home, "LABDRIAN_TEST_INVOCATIONS="+invocations)
			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("lifecycle all should return non-zero when any target fails/partials; output:\n%s", string(out))
			}
			logBytes, readErr := os.ReadFile(invocations)
			if readErr != nil {
				t.Fatalf("read lifecycle invocations: %v\noutput:\n%s", readErr, string(out))
			}
			got := strings.TrimSpace(string(logBytes))
			want := strings.Join([]string{
				"install claude",
				"install opencode",
				"install codex",
			}, "\n")
			if got != want {
				t.Fatalf("lifecycle all should visit every target in order despite failures\nwant:\n%s\ngot:\n%s\noutput:\n%s", want, got, string(out))
			}
		})
	}
}

func writeFutureFakeLifecycleEngine(t *testing.T, home string) {
	t.Helper()
	engineBinary := filepath.Join(home, ".claude", "bin", "gentle-ai-overlay")
	if err := os.MkdirAll(filepath.Dir(engineBinary), 0o755); err != nil {
		t.Fatalf("mkdir fake engine dir: %v", err)
	}
	fakeEngine := `#!/usr/bin/env bash
printf '%s %s\n' "$1" "$2" >> "$LABDRIAN_TEST_INVOCATIONS"
case "$2" in
  claude) echo "partial"; exit 2 ;;
  opencode) echo "ok"; exit 0 ;;
  codex) echo "unsupported"; exit 3 ;;
  *) echo "unexpected target: $2"; exit 4 ;;
esac
`
	if err := os.WriteFile(engineBinary, []byte(fakeEngine), 0o755); err != nil {
		t.Fatalf("write fake lifecycle engine: %v", err)
	}
	future := time.Now().Add(2 * time.Hour)
	if err := os.Chtimes(engineBinary, future, future); err != nil {
		t.Fatalf("set fake engine time: %v", err)
	}
}

func TestOverlayReadOnlyLifecycleDoesNotBuildEngineAndReportsRuntimeEvidence(t *testing.T) {
	// R-101/R-102/R-104: status/sync-check remain read-only and still emit runtime status if target dirs are missing.
	repoRoot := filepath.Join("..", "..")
	overlay, err := filepath.Abs(filepath.Join(repoRoot, "bin", "overlay"))
	if err != nil {
		t.Fatalf("resolve overlay path: %v", err)
	}
	home := t.TempDir()
	engineBinary := filepath.Join(home, ".claude", "bin", "gentle-ai-overlay")

	runOverlay := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(overlay, args...)
		cmd.Dir = repoRoot
		cmd.Env = append(os.Environ(), "HOME="+home)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("bin/overlay %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
		}
		return string(out)
	}

	statusOut := runOverlay("status", "--target", "claude")
	if _, err := os.Stat(engineBinary); !os.IsNotExist(err) {
		t.Fatalf("read-only status must not build engine binary, stat err: %v", err)
	}
	if !strings.Contains(statusOut, "[claude] status: partial") || !strings.Contains(statusOut, "engine binary not installed") {
		t.Fatalf("status should emit partial runtime evidence without binary, got:\n%s", statusOut)
	}

	if err := os.MkdirAll(filepath.Dir(engineBinary), 0o755); err != nil {
		t.Fatalf("mkdir engine binary dir: %v", err)
	}
	if err := os.WriteFile(engineBinary, []byte("#!/usr/bin/env bash\necho stale\n"), 0o755); err != nil {
		t.Fatalf("write stale engine binary: %v", err)
	}
	oldTime := time.Now().Add(-24 * time.Hour)
	if err := os.Chtimes(engineBinary, oldTime, oldTime); err != nil {
		t.Fatalf("set stale engine binary time: %v", err)
	}
	staleOut := runOverlay("status", "--target", "claude")
	if strings.Contains(staleOut, "stale") || !strings.Contains(staleOut, "older than engine source") {
		t.Fatalf("read-only status should not execute stale engine binary, got:\n%s", staleOut)
	}
	if err := os.Remove(engineBinary); err != nil {
		t.Fatalf("remove stale engine binary: %v", err)
	}

	syncOut := runOverlay("sync-check", "--target", "codex")
	if _, err := os.Stat(engineBinary); !os.IsNotExist(err) {
		t.Fatalf("read-only sync-check must not build engine binary, stat err: %v", err)
	}
	if !strings.Contains(syncOut, "target dir not found") || !strings.Contains(syncOut, "[codex] sync-check: partial") {
		t.Fatalf("sync-check should emit runtime evidence even when target dir is missing, got:\n%s", syncOut)
	}
}

func TestOverlayMutatingLifecycleRebuildsStaleEngineBinary(t *testing.T) {
	// R-101: mutating lifecycle commands may build and must refresh stale deployed engine binaries.
	repoRoot := filepath.Join("..", "..")
	overlay, err := filepath.Abs(filepath.Join(repoRoot, "bin", "overlay"))
	if err != nil {
		t.Fatalf("resolve overlay path: %v", err)
	}
	home := t.TempDir()
	engineBinary := filepath.Join(home, ".claude", "bin", "gentle-ai-overlay")
	if err := os.MkdirAll(filepath.Dir(engineBinary), 0o755); err != nil {
		t.Fatalf("mkdir engine binary dir: %v", err)
	}
	if err := os.WriteFile(engineBinary, []byte("#!/usr/bin/env bash\necho stale\n"), 0o755); err != nil {
		t.Fatalf("write stale engine binary: %v", err)
	}
	oldTime := time.Now().Add(-24 * time.Hour)
	if err := os.Chtimes(engineBinary, oldTime, oldTime); err != nil {
		t.Fatalf("set stale engine binary time: %v", err)
	}

	cmd := exec.Command(overlay, "install", "--target", "codex")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "HOME="+home)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("mutating lifecycle should fail after rebuilding because Codex install is unsupported:\n%s", string(out))
	}
	if strings.Contains(string(out), "stale") {
		t.Fatalf("mutating lifecycle should rebuild stale binary before execution, got:\n%s", string(out))
	}
	if !strings.Contains(string(out), "unsupported") {
		t.Fatalf("mutating lifecycle should report unsupported target result, got:\n%s", string(out))
	}
	rebuilt, err := os.ReadFile(engineBinary)
	if err != nil {
		t.Fatalf("read rebuilt engine binary: %v", err)
	}
	if strings.Contains(string(rebuilt), "echo stale") {
		t.Fatalf("engine binary was not refreshed from stale script")
	}
}

func TestOverlayLifecycleFreshnessIncludesEmbeddedPluginSource(t *testing.T) {
	// R-103: embedded runtime assets such as the OpenCode plugin source participate in freshness checks.
	realRepoRoot := filepath.Join("..", "..")
	repoRoot := makeTempOverlayRepoWithPlugin(t, realRepoRoot)
	overlay, err := filepath.Abs(filepath.Join(repoRoot, "bin", "overlay"))
	if err != nil {
		t.Fatalf("resolve overlay path: %v", err)
	}
	pluginSource := filepath.Join(repoRoot, "engine", "runtime", "labdrian-runtime-parity-plugin.mjs")
	if _, err := os.Stat(pluginSource); err != nil {
		t.Fatalf("stat plugin source: %v", err)
	}

	home := t.TempDir()
	engineBinary := filepath.Join(home, ".claude", "bin", "gentle-ai-overlay")
	if err := os.MkdirAll(filepath.Dir(engineBinary), 0o755); err != nil {
		t.Fatalf("mkdir engine binary dir: %v", err)
	}
	futureBinary := time.Now().Add(2 * time.Hour)
	if err := os.WriteFile(engineBinary, []byte("#!/usr/bin/env bash\necho should-not-run\n"), 0o755); err != nil {
		t.Fatalf("write engine binary: %v", err)
	}
	if err := os.Chtimes(engineBinary, futureBinary, futureBinary); err != nil {
		t.Fatalf("set engine binary time: %v", err)
	}
	futurePlugin := futureBinary.Add(time.Hour)
	if err := os.Chtimes(pluginSource, futurePlugin, futurePlugin); err != nil {
		t.Fatalf("set plugin source time: %v", err)
	}

	cmd := exec.Command(overlay, "status", "--target", "opencode")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "HOME="+home)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("read-only status failed: %v\n%s", err, string(out))
	}
	if strings.Contains(string(out), "should-not-run") || !strings.Contains(string(out), "older than engine source") {
		t.Fatalf("read-only status should treat newer embedded plugin source as stale evidence, got:\n%s", string(out))
	}
}

func makeTempOverlayRepoWithPlugin(t *testing.T, realRepoRoot string) string {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{filepath.Join(root, "bin"), filepath.Join(root, "engine", "runtime")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	copyFileForTest(t, filepath.Join(realRepoRoot, "bin", "overlay"), filepath.Join(root, "bin", "overlay"), 0o755)
	copyFileForTest(t,
		filepath.Join(realRepoRoot, "engine", "runtime", "labdrian-runtime-parity-plugin.mjs"),
		filepath.Join(root, "engine", "runtime", "labdrian-runtime-parity-plugin.mjs"),
		0o644,
	)
	if err := os.WriteFile(filepath.Join(root, "overlay.manifest"), []byte(""), 0o644); err != nil {
		t.Fatalf("write temp overlay manifest: %v", err)
	}
	cmd := exec.Command("git", "init", "-b", "main")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("init temp git repo: %v\n%s", err, string(out))
	}
	cmd = exec.Command("git", "commit", "--allow-empty", "-m", "test baseline")
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("commit temp git baseline: %v\n%s", err, string(out))
	}
	return root
}

func copyFileForTest(t *testing.T, src, dst string, mode os.FileMode) {
	t.Helper()
	content, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}
	if err := os.WriteFile(dst, content, mode); err != nil {
		t.Fatalf("write %s: %v", dst, err)
	}
}

func TestOverlayAtomicRebuildPreservesPreviousBinaryOnFailure(t *testing.T) {
	// R-101: failed mutating rebuild must leave the last-known-good binary in place.
	repoRoot := filepath.Join("..", "..")
	overlay, err := filepath.Abs(filepath.Join(repoRoot, "bin", "overlay"))
	if err != nil {
		t.Fatalf("resolve overlay path: %v", err)
	}
	home := t.TempDir()
	engineBinary := filepath.Join(home, ".claude", "bin", "gentle-ai-overlay")
	if err := os.MkdirAll(filepath.Dir(engineBinary), 0o755); err != nil {
		t.Fatalf("mkdir engine binary dir: %v", err)
	}
	oldContent := []byte("#!/usr/bin/env bash\necho last-known-good\n")
	if err := os.WriteFile(engineBinary, oldContent, 0o755); err != nil {
		t.Fatalf("write old engine binary: %v", err)
	}
	oldTime := time.Now().Add(-24 * time.Hour)
	if err := os.Chtimes(engineBinary, oldTime, oldTime); err != nil {
		t.Fatalf("set stale engine binary time: %v", err)
	}

	cmd := exec.Command(overlay, "install", "--target", "codex")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "HOME="+home, "GOFLAGS=-definitely-invalid-flag")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected mutating rebuild to fail with invalid GOFLAGS, got success:\n%s", string(out))
	}
	kept, readErr := os.ReadFile(engineBinary)
	if readErr != nil {
		t.Fatalf("previous binary should remain readable: %v", readErr)
	}
	if string(kept) != string(oldContent) {
		t.Fatalf("failed rebuild should preserve previous binary, got:\n%s", string(kept))
	}
}

func TestOverlayShellIgnoresRelativeXDGConfigHomeForOpenCodeRoot(t *testing.T) {
	// R-103: shell target paths must not deploy/status-check OpenCode under a relative XDG_CONFIG_HOME.
	source, err := os.ReadFile(filepath.Join("..", "..", "bin", "overlay"))
	if err != nil {
		t.Fatalf("read bin/overlay: %v", err)
	}
	text := string(source)
	for _, want := range []string{
		`if [[ -n "${XDG_CONFIG_HOME:-}" && "${XDG_CONFIG_HOME:-}" = /* ]]; then`,
		`OPENCODE_CONFIG_ROOT="$XDG_CONFIG_HOME/opencode"`,
		`OPENCODE_CONFIG_ROOT="$HOME/.config/opencode"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("bin/overlay should ignore relative XDG_CONFIG_HOME and use an absolute OpenCode root; missing %q", want)
		}
	}
}

func TestReadmeDocumentsThreeRuntimeGuarantees(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	text := string(readme)
	for _, want := range []string{
		"Claude Code is the deterministic baseline",
		"OpenCode plugin changes require an OpenCode restart",
		"Codex is a first-class target with explicit `unsupported`/`partial`/`supported` states",
		"GADU is intentionally out of scope",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("README should document %q", want)
		}
	}
}

func TestOverlayShellLegacyHookCommandsExecuteRealHookLifecycle(t *testing.T) {
	// R-102/R-105: execute the shell wrapper against a temporary HOME so legacy
	// hook commands prove real install/status/uninstall behavior, not just dispatch text.
	repoRoot := filepath.Join("..", "..")
	overlay, err := filepath.Abs(filepath.Join(repoRoot, "bin", "overlay"))
	if err != nil {
		t.Fatalf("resolve overlay path: %v", err)
	}
	home := t.TempDir()
	settingsPath := filepath.Join(home, ".claude", "settings.json")

	runOverlay := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(overlay, args...)
		cmd.Dir = repoRoot
		cmd.Env = append(os.Environ(), "HOME="+home)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("bin/overlay %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
		}
		return string(out)
	}

	installOut := runOverlay("install-hooks")
	if !strings.Contains(installOut, "install-hooks complete") || !strings.Contains(installOut, "Deterministic scoping is now ACTIVE") {
		t.Fatalf("install-hooks should report real hook installation, got:\n%s", installOut)
	}
	settingsAfterInstall, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("install-hooks should create Claude settings at %s: %v", settingsPath, err)
	}
	for _, want := range []string{"UserPromptSubmit", "PreToolUse", "gate-task", "propagate"} {
		if !strings.Contains(string(settingsAfterInstall), want) {
			t.Fatalf("settings after install-hooks should contain %q, got:\n%s", want, string(settingsAfterInstall))
		}
	}
	contractPath := filepath.Join(home, ".claude", "skills", "_shared", "minimalism-contract.md")
	if err := os.MkdirAll(filepath.Dir(contractPath), 0o755); err != nil {
		t.Fatalf("mkdir contract dir: %v", err)
	}
	if err := os.WriteFile(contractPath, []byte(testContractContent), 0o644); err != nil {
		t.Fatalf("write contract for status-hooks: %v", err)
	}

	statusOut := runOverlay("status-hooks")
	for _, want := range []string{"binary:", "hook: UserPromptSubmit", "hook: PreToolUse", "contract:"} {
		if !strings.Contains(statusOut, want) {
			t.Fatalf("status-hooks should report %q, got:\n%s", want, statusOut)
		}
	}

	uninstallOut := runOverlay("uninstall-hooks")
	if !strings.Contains(uninstallOut, "uninstall-hooks complete") {
		t.Fatalf("uninstall-hooks should report real hook removal, got:\n%s", uninstallOut)
	}
	settingsAfterUninstall, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("settings should remain readable after uninstall-hooks: %v", err)
	}
	for _, forbidden := range []string{"gate-task", "propagate"} {
		if strings.Contains(string(settingsAfterUninstall), forbidden) {
			t.Fatalf("settings after uninstall-hooks should remove %q, got:\n%s", forbidden, string(settingsAfterUninstall))
		}
	}
}
