package main

// TC-E2E-orchestrator: validates 'gate-task', the whole chain from the hook input to the answer
// (engine/hookwire reads the input and writes the answer, engine/gate decides), against the EXACT
// format the orchestrator produces, as empirically verified in Claude Code 2.1.185.
//
// Verified facts this test encodes:
//   - The sub-agent spawn tool is named "Agent", NOT "Task".
//   - The hook receives tool_input with: description, prompt, subagent_type, model (optional).
//   - updatedInput must echo FULL tool_input (description, prompt[mutated], subagent_type,
//     model if present) — returning only {prompt:...} fails schema validation.
//   - hookSpecificOutput must include hookEventName:"PreToolUse" and
//     permissionDecision:"allow" — without them, updatedInput is ignored by Claude Code.
//   - The canonical injected entry is a BARE absolute path line (not "Read fully BEFORE work: <path>").
//   - Exact trimmed-line matching prevents double-injection and makes strip work correctly.
//
// These tests used to run the gate on the JSON itself, before the hook format had one home;
// the policy half of them is in engine/gate and the bytes are pinned by the golden files
// (testdata/hook-golden).

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"strings"
	"testing"
)

// absoluteContractPath is the absolute path the orchestrator injects — what the hook
// --contract-path flag resolves to after $HOME expansion.
const absoluteContractPath = "/home/testuser/.claude/skills/_shared/minimalism-contract.md"

// contractContentForE2E is a realistic contract frontmatter for E2E tests.
const contractContentForE2E = `---
applies_to_phases: [sdd-tasks, sdd-apply]
excluded_phases: [sdd-propose, sdd-spec, sdd-design, sdd-verify, sdd-archive]
injection_point: "## Skills to load before work"
---
# Minimalism Contract

Content here.
`

// buildAgentInput builds a Claude Code PreToolUse hook JSON for the Agent tool
// with the EXACT shape the orchestrator produces. This is the verified format
// from CC 2.1.185: tool_input contains description, prompt, subagent_type, model.
func buildAgentInput(subagentType, description, prompt string, includeModel bool) string {
	toolInput := map[string]interface{}{
		"description":   description,
		"prompt":        prompt,
		"subagent_type": subagentType,
	}
	if includeModel {
		toolInput["model"] = "claude-sonnet-4-5"
	}
	input := map[string]interface{}{
		"tool_name":  "Agent",
		"tool_input": toolInput,
	}
	b, _ := json.Marshal(input)
	return string(b)
}

// inMemoryFiles is a readFile over files: a path that is not one of them does not exist, as it
// would not on disk, so a test can tell a file that is missing from one that is empty.
func inMemoryFiles(files map[string]string) func(string) ([]byte, error) {
	return func(path string) ([]byte, error) {
		content, ok := files[path]
		if !ok {
			return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
		}
		return []byte(content), nil
	}
}

// gateTaskAnswer runs 'gate-task' for the managed contract at absoluteContractPath whose
// document is content, over the hook input, and returns the line it printed (without its line
// break). Whatever the input is, the command prints one line.
func gateTaskAnswer(t *testing.T, input, content string) string {
	t.Helper()
	files := map[string]string{"/virtual/contract.md": content}
	var stdout, stderr bytes.Buffer
	gateTaskCore([]string{"--contract-file", "/virtual/contract.md", "--contract-path", absoluteContractPath},
		strings.NewReader(input), &stdout, &stderr, inMemoryFiles(files))
	out := stdout.String()
	if strings.Count(out, "\n") != 1 || !strings.HasSuffix(out, "\n") {
		t.Fatalf("gate-task printed %q, want exactly one line", out)
	}
	return strings.TrimSuffix(out, "\n")
}

// decodeAnswer decodes the line gate-task printed.
func decodeAnswer(t *testing.T, resp string) map[string]interface{} {
	t.Helper()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		t.Fatalf("response is not valid JSON: %v\nresponse: %s", err, resp)
	}
	return result
}

// countContractLines counts the lines of prompt that are the contract path.
func countContractLines(prompt string) int {
	count := 0
	for _, line := range strings.Split(prompt, "\n") {
		if strings.TrimSpace(line) == absoluteContractPath {
			count++
		}
	}
	return count
}

// TC-E2E-1a: sdd-tasks with orchestrator-format prompt that ALREADY contains the
// contract path → gate is a no-op (idempotency), pass-through response.
// The contract path appears EXACTLY ONCE — no duplication.
func TestE2E_OrchestratorFormat_SddTasks_AlreadyPresent_NoOp(t *testing.T) {
	// Build a prompt EXACTLY as the orchestrator produces it — contract path already there:
	// "...\n\n## Skills to load before work\n<ABSOLUTE contract path>\n<another skill path>\n"
	anotherSkillPath := "/home/testuser/.claude/skills/chained-pr/SKILL.md"
	prompt := "Run the sdd-tasks phase for change 'scoping-fixes'.\n\n## Skills to load before work\n" +
		absoluteContractPath + "\n" +
		anotherSkillPath + "\n"
	description := "sdd-tasks sub-agent for scoping-fixes"

	resp := gateTaskAnswer(t, buildAgentInput("sdd-tasks", description, prompt, true), contractContentForE2E)
	result := decodeAnswer(t, resp)

	// The contract path is already present → gate is a no-op (idempotency).
	// Pass-through: hookSpecificOutput must be absent.
	if _, present := result["hookSpecificOutput"]; present {
		t.Errorf("contract already in prompt: hookSpecificOutput must be absent (no-op pass-through); got: %s", resp)
	}

	// Verify: the prompt was not modified (contract appears exactly once, in the original).
	// We verify this by checking the response is {} (pass-through).
	if resp != `{}` {
		t.Errorf("pass-through response must be '{}'; got: %s", resp)
	}
}

// TC-E2E-1b: sdd-tasks with orchestrator-format prompt that does NOT yet have
// the contract path → gate injects it ONCE, output has hookEventName +
// permissionDecision, updatedInput echoes full tool_input.
func TestE2E_OrchestratorFormat_SddTasks_InjectsOnce(t *testing.T) {
	// Build a prompt EXACTLY as the orchestrator produces it — contract path NOT yet present.
	// The orchestrator puts another skill under the header but not the contract.
	anotherSkillPath := "/home/testuser/.claude/skills/chained-pr/SKILL.md"
	prompt := "Run the sdd-tasks phase for change 'scoping-fixes'.\n\n## Skills to load before work\n" +
		anotherSkillPath + "\n"
	description := "sdd-tasks sub-agent for scoping-fixes"

	resp := gateTaskAnswer(t, buildAgentInput("sdd-tasks", description, prompt, true), contractContentForE2E)
	result := decodeAnswer(t, resp)

	// Must have hookSpecificOutput (injection happened).
	hso, ok := result["hookSpecificOutput"].(map[string]interface{})
	if !ok {
		t.Fatalf("response missing hookSpecificOutput; got: %s", resp)
	}

	// F-OUTPUT: hookSpecificOutput must have hookEventName and permissionDecision.
	if hso["hookEventName"] != "PreToolUse" {
		t.Errorf("hookSpecificOutput.hookEventName must be 'PreToolUse'; got: %v", hso["hookEventName"])
	}
	if hso["permissionDecision"] != "allow" {
		t.Errorf("hookSpecificOutput.permissionDecision must be 'allow'; got: %v", hso["permissionDecision"])
	}

	// updatedInput must exist.
	updatedInput, ok := hso["updatedInput"].(map[string]interface{})
	if !ok {
		t.Fatalf("hookSpecificOutput missing updatedInput; got: %v", hso)
	}

	// F-OUTPUT: updatedInput must echo FULL tool_input (description, subagent_type, model).
	if updatedInput["description"] != description {
		t.Errorf("updatedInput must echo description; got: %v", updatedInput["description"])
	}
	if updatedInput["subagent_type"] != "sdd-tasks" {
		t.Errorf("updatedInput must echo subagent_type; got: %v", updatedInput["subagent_type"])
	}
	if updatedInput["model"] != "claude-sonnet-4-5" {
		t.Errorf("updatedInput must echo model when present; got: %v", updatedInput["model"])
	}

	newPrompt, ok := updatedInput["prompt"].(string)
	if !ok {
		t.Fatal("updatedInput missing prompt string")
	}

	// F-FORMAT/C1: the canonical injected entry is a BARE absolute path line.
	// The contract path must appear EXACTLY ONCE (no duplication).
	if count := countContractLines(newPrompt); count != 1 {
		t.Errorf("contract path should appear EXACTLY ONCE in the prompt (no duplication); count=%d\nprompt:\n%s",
			count, newPrompt)
	}

	// The other skill path must survive.
	if !strings.Contains(newPrompt, anotherSkillPath) {
		t.Errorf("other skill path %q must survive; got:\n%s", anotherSkillPath, newPrompt)
	}

	// The injection_point header must be present exactly once.
	if strings.Count(newPrompt, "## Skills to load before work") != 1 {
		t.Errorf("injection_point header must appear exactly once; got:\n%s", newPrompt)
	}
}

// TC-E2E-2: sdd-propose (excluded phase) → contract path STRIPPED,
// other skill survives, full tool_input echoed, hookEventName+permissionDecision present.
func TestE2E_OrchestratorFormat_SddPropose_StripsContract(t *testing.T) {
	anotherSkillPath := "/home/testuser/.claude/skills/chained-pr/SKILL.md"
	// The orchestrator prompt already contains the contract path (it was injected).
	prompt := "Run the sdd-propose phase for change 'scoping-fixes'.\n\n## Skills to load before work\n" +
		absoluteContractPath + "\n" +
		anotherSkillPath + "\n"
	description := "sdd-propose sub-agent for scoping-fixes"

	resp := gateTaskAnswer(t, buildAgentInput("sdd-propose", description, prompt, true), contractContentForE2E)
	result := decodeAnswer(t, resp)

	// Must have hookSpecificOutput (strip produced a change).
	hso, ok := result["hookSpecificOutput"].(map[string]interface{})
	if !ok {
		t.Fatalf("response missing hookSpecificOutput; got: %s", resp)
	}

	// F-OUTPUT: hookSpecificOutput must have hookEventName and permissionDecision.
	if hso["hookEventName"] != "PreToolUse" {
		t.Errorf("hookSpecificOutput.hookEventName must be 'PreToolUse'; got: %v", hso["hookEventName"])
	}
	if hso["permissionDecision"] != "allow" {
		t.Errorf("hookSpecificOutput.permissionDecision must be 'allow'; got: %v", hso["permissionDecision"])
	}

	updatedInput, ok := hso["updatedInput"].(map[string]interface{})
	if !ok {
		t.Fatalf("hookSpecificOutput missing updatedInput; got: %v", hso)
	}

	// F-OUTPUT: full tool_input echoed.
	if updatedInput["description"] != description {
		t.Errorf("updatedInput must echo description; got: %v", updatedInput["description"])
	}
	if updatedInput["subagent_type"] != "sdd-propose" {
		t.Errorf("updatedInput must echo subagent_type; got: %v", updatedInput["subagent_type"])
	}

	newPrompt, ok := updatedInput["prompt"].(string)
	if !ok {
		t.Fatal("updatedInput missing prompt string")
	}

	// Contract path line must be STRIPPED (count 0).
	if count := countContractLines(newPrompt); count != 0 {
		t.Errorf("contract path should be STRIPPED (count 0) for excluded phase; count=%d\nprompt:\n%s",
			count, newPrompt)
	}

	// The other skill path must survive.
	if !strings.Contains(newPrompt, anotherSkillPath) {
		t.Errorf("other skill path %q must survive stripping; got:\n%s", anotherSkillPath, newPrompt)
	}
}

// TC-E2E-3: malformed/empty stdin → benign pass-through + exit 0 (no error).
func TestE2E_MalformedInput_PassThrough(t *testing.T) {
	for _, bad := range []string{"", "not json", "{broken", "null", "[]"} {
		resp := gateTaskAnswer(t, bad, contractContentForE2E)
		result := decodeAnswer(t, resp)
		// Pass-through: hookSpecificOutput must be absent.
		if _, present := result["hookSpecificOutput"]; present {
			t.Errorf("gate-task(%q): hookSpecificOutput must be absent on pass-through; got: %s", bad, resp)
		}
	}
}

// TC-E2E-4: missing contract file → benign pass-through (fail-safe), and the warning that says
// the file could not be read.
func TestE2E_MissingContractFile_PassThrough(t *testing.T) {
	var stdout, stderr bytes.Buffer
	gateTaskCore([]string{"--contract-file", "/virtual/missing.md", "--contract-path", absoluteContractPath},
		strings.NewReader(buildAgentInput("sdd-tasks", "desc", "do tasks", false)), &stdout, &stderr, inMemoryFiles(map[string]string{"/virtual/contract.md": contractContentForE2E}))
	if stdout.String() != "{}\n" {
		t.Errorf("gate-task printed %q for a contract file that does not exist, want the pass-through", stdout.String())
	}
	if !strings.Contains(stderr.String(), "cannot read contract file") || !strings.Contains(stderr.String(), "/virtual/missing.md") {
		t.Errorf("stderr %q does not say the contract file could not be read", stderr.String())
	}
}

// TC-E2E-4b: a contract file that exists and is empty or broken has no frontmatter to derive a
// phase from → pass-through. It is not the missing file above: that one is not read at all.
func TestE2E_EmptyContractFile_PassThrough(t *testing.T) {
	resp := gateTaskAnswer(t, buildAgentInput("sdd-tasks", "desc", "do tasks", false), "")
	if _, present := decodeAnswer(t, resp)["hookSpecificOutput"]; present {
		t.Errorf("hookSpecificOutput must be absent on broken contract pass-through; got: %s", resp)
	}
}

// TC-E2E-11: a call with no prompt is left exactly as it came, which is the pass-through and not
// a reply with an empty hookSpecificOutput: any presence of the key (empty or not) would be a
// rewrite that Claude Code applies. The gate's own test says the prompt is left alone; this one
// says what the wire then holds, because the gate knows no JSON.
func TestE2E_MissingPrompt_LeavesNoHookSpecificOutputAtAll(t *testing.T) {
	input := `{"tool_name":"Agent","tool_input":{"description":"desc","subagent_type":"sdd-tasks"}}`
	resp := gateTaskAnswer(t, input, contractContentForE2E)
	if resp != "{}" {
		t.Errorf("gate-task answered %q for a call with no prompt, want exactly {}", resp)
	}
	if _, present := decodeAnswer(t, resp)["hookSpecificOutput"]; present {
		t.Errorf("hookSpecificOutput must be absent when the prompt is missing; got: %s", resp)
	}
}

// TC-E2E-5: unknown subagent_type → benign pass-through + exit 0.
func TestE2E_UnknownSubagentType_PassThrough(t *testing.T) {
	resp := gateTaskAnswer(t, buildAgentInput("some-future-phase", "desc", "do something", false), contractContentForE2E)
	// Pass-through: no injection.
	if hso, ok := decodeAnswer(t, resp)["hookSpecificOutput"].(map[string]interface{}); ok {
		if ui, ok := hso["updatedInput"].(map[string]interface{}); ok {
			if p, ok := ui["prompt"].(string); ok && strings.Contains(p, absoluteContractPath) {
				t.Errorf("unknown type must not inject contract; prompt: %s", p)
			}
		}
	}
}

// TC-E2E-6: Agent input without model field → updatedInput must NOT include model key
// (do not inject a zero-value model field).
func TestE2E_NoModelField_NotEchoed(t *testing.T) {
	resp := gateTaskAnswer(t, buildAgentInput("sdd-tasks", "desc", "Do tasks without model field.", false /* no model */), contractContentForE2E)
	hso, ok := decodeAnswer(t, resp)["hookSpecificOutput"].(map[string]interface{})
	if !ok {
		t.Fatalf("response missing hookSpecificOutput; got: %s", resp)
	}
	updatedInput, ok := hso["updatedInput"].(map[string]interface{})
	if !ok {
		t.Fatalf("hookSpecificOutput missing updatedInput; got: %v", hso)
	}
	// model must not be present (it was not in tool_input).
	if _, modelPresent := updatedInput["model"]; modelPresent {
		t.Errorf("updatedInput must not include model when absent from tool_input; got: %v", updatedInput)
	}
}

// TC-E2E-inject-bare: inject into a prompt that has NO existing header.
// The bare absolute path is appended under the injection_point header.
func TestE2E_InjectBarePath_NoExistingHeader(t *testing.T) {
	resp := gateTaskAnswer(t, buildAgentInput("sdd-tasks", "desc", "Do tasks with no header.", false), contractContentForE2E)
	hso, _ := decodeAnswer(t, resp)["hookSpecificOutput"].(map[string]interface{})
	updatedInput, _ := hso["updatedInput"].(map[string]interface{})
	newPrompt, _ := updatedInput["prompt"].(string)

	// The bare path must appear as an exact line.
	if countContractLines(newPrompt) == 0 {
		t.Errorf("bare absolute path %q must appear as exact line; got:\n%s", absoluteContractPath, newPrompt)
	}

	// Must NOT use the old "Read fully BEFORE work:" prefix format.
	if strings.Contains(newPrompt, "Read fully BEFORE work:") {
		t.Errorf("injected entry must use bare path format, not 'Read fully BEFORE work:'; got:\n%s", newPrompt)
	}
}
