package gate_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gate"
)

// ---- helpers ---------------------------------------------------------------

const contractContent = `---
applies_to_phases: [sdd-tasks, sdd-apply]
excluded_phases: [sdd-propose, sdd-spec, sdd-design, sdd-verify, sdd-archive]
injection_point: "## Skills to load before work"
---
# Minimalism Contract

Content here.
`

const contractPath = "skills/_shared/minimalism-contract.md"

// buildInput returns a Claude Code PreToolUse hook JSON for the Task tool.
func buildInput(subagentType, prompt string) string {
	input := map[string]interface{}{
		"tool_name": "Task",
		"tool_input": map[string]interface{}{
			"subagent_type": subagentType,
			"prompt":        prompt,
		},
	}
	b, _ := json.Marshal(input)
	return string(b)
}

// ---- tests -----------------------------------------------------------------

// TC-1: sdd-tasks → contract path injected into prompt under injection_point header.
func TestInjectsForSddTasks(t *testing.T) {
	input := buildInput("sdd-tasks", "Do the tasks phase.")
	cfg := gate.Config{ContractPath: contractPath, ContractContent: contractContent}

	resp, err := gate.Process(input, cfg)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		t.Fatalf("response is not valid JSON: %v\nresponse: %s", err, resp)
	}

	// Must have hookSpecificOutput with updatedInput.
	hso, ok := result["hookSpecificOutput"].(map[string]interface{})
	if !ok {
		t.Fatalf("response missing hookSpecificOutput; got: %s", resp)
	}
	updatedInput, ok := hso["updatedInput"].(map[string]interface{})
	if !ok {
		t.Fatalf("hookSpecificOutput missing updatedInput; got: %v", hso)
	}
	newPrompt, ok := updatedInput["prompt"].(string)
	if !ok {
		t.Fatal("updatedInput missing prompt string")
	}
	if !strings.Contains(newPrompt, contractPath) {
		t.Errorf("injected prompt should contain contract path %q; got:\n%s", contractPath, newPrompt)
	}
	if !strings.Contains(newPrompt, "## Skills to load before work") {
		t.Errorf("injected prompt should contain injection_point header; got:\n%s", newPrompt)
	}
}

// TC-2: sdd-apply → contract path injected into prompt.
func TestInjectsForSddApply(t *testing.T) {
	input := buildInput("sdd-apply", "Apply the tasks.")
	cfg := gate.Config{ContractPath: contractPath, ContractContent: contractContent}

	resp, err := gate.Process(input, cfg)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}

	hso, ok := result["hookSpecificOutput"].(map[string]interface{})
	if !ok {
		t.Fatalf("response missing hookSpecificOutput; got: %s", resp)
	}
	updatedInput, _ := hso["updatedInput"].(map[string]interface{})
	newPrompt, _ := updatedInput["prompt"].(string)
	if !strings.Contains(newPrompt, contractPath) {
		t.Errorf("sdd-apply should have contract injected; got:\n%s", newPrompt)
	}
}

// TC-3: sdd-propose → contract path stripped if present, no-op if absent.
func TestStripsFromSddPropose(t *testing.T) {
	// Prompt already contains the contract path — it should be stripped.
	promptWithContract := "Do propose.\n\n## Skills to load before work\nRead " + contractPath + " fully.\n"
	input := buildInput("sdd-propose", promptWithContract)
	cfg := gate.Config{ContractPath: contractPath, ContractContent: contractContent}

	resp, err := gate.Process(input, cfg)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}

	hso, ok := result["hookSpecificOutput"].(map[string]interface{})
	if !ok {
		t.Fatalf("response missing hookSpecificOutput; got: %s", resp)
	}
	updatedInput, _ := hso["updatedInput"].(map[string]interface{})
	newPrompt, _ := updatedInput["prompt"].(string)
	if strings.Contains(newPrompt, contractPath) {
		t.Errorf("sdd-propose prompt should NOT contain contract path after strip; got:\n%s", newPrompt)
	}
}

// TC-4: sdd-spec → contract stripped if present.
func TestStripsFromSddSpec(t *testing.T) {
	promptWithContract := "Spec phase.\n## Skills to load before work\nRead " + contractPath + ".\n"
	input := buildInput("sdd-spec", promptWithContract)
	cfg := gate.Config{ContractPath: contractPath, ContractContent: contractContent}

	resp, err := gate.Process(input, cfg)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		t.Fatalf("response is not valid JSON: %v\nresponse: %s", err, resp)
	}
	hso, _ := result["hookSpecificOutput"].(map[string]interface{})
	updatedInput, _ := hso["updatedInput"].(map[string]interface{})
	newPrompt, _ := updatedInput["prompt"].(string)
	if strings.Contains(newPrompt, contractPath) {
		t.Errorf("sdd-spec prompt should not contain contract path; got:\n%s", newPrompt)
	}
}

// TC-5: sdd-design → contract stripped if present.
func TestStripsFromSddDesign(t *testing.T) {
	promptWithContract := "Design phase.\n## Skills to load before work\nRead " + contractPath + ".\n"
	input := buildInput("sdd-design", promptWithContract)
	cfg := gate.Config{ContractPath: contractPath, ContractContent: contractContent}

	resp, err := gate.Process(input, cfg)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	hso, _ := result["hookSpecificOutput"].(map[string]interface{})
	updatedInput, _ := hso["updatedInput"].(map[string]interface{})
	newPrompt, _ := updatedInput["prompt"].(string)
	if strings.Contains(newPrompt, contractPath) {
		t.Errorf("sdd-design prompt should not contain contract path; got:\n%s", newPrompt)
	}
}

// TC-6: sdd-verify → contract stripped.
func TestStripsFromSddVerify(t *testing.T) {
	promptWithContract := "Verify.\n## Skills to load before work\nRead " + contractPath + ".\n"
	input := buildInput("sdd-verify", promptWithContract)
	cfg := gate.Config{ContractPath: contractPath, ContractContent: contractContent}

	resp, err := gate.Process(input, cfg)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	hso, _ := result["hookSpecificOutput"].(map[string]interface{})
	updatedInput, _ := hso["updatedInput"].(map[string]interface{})
	newPrompt, _ := updatedInput["prompt"].(string)
	if strings.Contains(newPrompt, contractPath) {
		t.Errorf("sdd-verify prompt should not contain contract path; got:\n%s", newPrompt)
	}
}

// TC-7: sdd-archive → contract stripped.
func TestStripsFromSddArchive(t *testing.T) {
	promptWithContract := "Archive.\n## Skills to load before work\nRead " + contractPath + ".\n"
	input := buildInput("sdd-archive", promptWithContract)
	cfg := gate.Config{ContractPath: contractPath, ContractContent: contractContent}

	resp, err := gate.Process(input, cfg)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	hso, _ := result["hookSpecificOutput"].(map[string]interface{})
	updatedInput, _ := hso["updatedInput"].(map[string]interface{})
	newPrompt, _ := updatedInput["prompt"].(string)
	if strings.Contains(newPrompt, contractPath) {
		t.Errorf("sdd-archive prompt should not contain contract path; got:\n%s", newPrompt)
	}
}

// TC-8: excluded phase with NO contract in prompt → pass-through (no-op, no error).
func TestExcludedPhaseNoContractIsPassThrough(t *testing.T) {
	input := buildInput("sdd-propose", "Do propose phase. No contract here.")
	cfg := gate.Config{ContractPath: contractPath, ContractContent: contractContent}

	resp, err := gate.Process(input, cfg)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}

	// Should be a benign allow response (no updatedInput needed since nothing changed),
	// or an updatedInput with the prompt unchanged. Either is acceptable — what
	// matters is that the response is valid JSON and the prompt is NOT modified
	// to add the contract.
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		t.Fatalf("response is not valid JSON: %v\nresponse: %s", err, resp)
	}

	// If hookSpecificOutput.updatedInput.prompt exists, it must not contain contractPath.
	if hso, ok := result["hookSpecificOutput"].(map[string]interface{}); ok {
		if ui, ok := hso["updatedInput"].(map[string]interface{}); ok {
			if p, ok := ui["prompt"].(string); ok {
				if strings.Contains(p, contractPath) {
					t.Errorf("excluded phase pass-through must not inject contract; got:\n%s", p)
				}
			}
		}
	}
}

// TC-9: unknown subagent_type → pass-through unchanged (FAIL-SAFE).
func TestUnknownSubagentTypePassThrough(t *testing.T) {
	input := buildInput("some-future-phase", "Do something unknown.")
	cfg := gate.Config{ContractPath: contractPath, ContractContent: contractContent}

	resp, err := gate.Process(input, cfg)
	if err != nil {
		t.Fatalf("Process must not error on unknown subagent_type (FAIL-SAFE): %v", err)
	}
	if resp == "" {
		t.Fatal("response must not be empty")
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		t.Fatalf("response must be valid JSON on unknown type: %v\nresponse: %s", err, resp)
	}
	// Must not inject the contract.
	if hso, ok := result["hookSpecificOutput"].(map[string]interface{}); ok {
		if ui, ok := hso["updatedInput"].(map[string]interface{}); ok {
			if p, ok := ui["prompt"].(string); ok {
				if strings.Contains(p, contractPath) {
					t.Errorf("unknown type pass-through must not inject contract; prompt: %s", p)
				}
			}
		}
	}
}

// TC-10: malformed/empty STDIN JSON → pass-through unchanged + exit 0 (FAIL-SAFE).
// We test Process() returning a benign response with no error on bad input.
func TestMalformedJSONPassThrough(t *testing.T) {
	inputs := []string{
		"",
		"not json",
		"{broken",
		"null",
		"[]",
	}
	cfg := gate.Config{ContractPath: contractPath, ContractContent: contractContent}

	for _, bad := range inputs {
		resp, err := gate.Process(bad, cfg)
		if err != nil {
			t.Errorf("Process(%q): must not error on malformed JSON (FAIL-SAFE), got: %v", bad, err)
			continue
		}
		if resp == "" {
			t.Errorf("Process(%q): response must not be empty", bad)
			continue
		}
		var result map[string]interface{}
		if err := json.Unmarshal([]byte(resp), &result); err != nil {
			t.Errorf("Process(%q): response must be valid JSON: %v\nresp: %s", bad, err, resp)
		}
	}
}

// TC-11: missing tool_input.prompt → pass-through (FAIL-SAFE).
func TestMissingPromptPassThrough(t *testing.T) {
	// tool_input exists but has no "prompt" key.
	input := `{"tool_name":"Task","tool_input":{"subagent_type":"sdd-tasks"}}`
	cfg := gate.Config{ContractPath: contractPath, ContractContent: contractContent}

	resp, err := gate.Process(input, cfg)
	if err != nil {
		t.Fatalf("Process must not error when prompt is missing (FAIL-SAFE): %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		t.Fatalf("response must be valid JSON: %v\nresponse: %s", err, resp)
	}
}

// TC-12: contract frontmatter broken/missing → pass-through (FAIL-SAFE, not loud).
func TestBrokenFrontmatterPassThrough(t *testing.T) {
	brokenContract := "no frontmatter here at all"
	input := buildInput("sdd-tasks", "Do tasks.")
	cfg := gate.Config{ContractPath: contractPath, ContractContent: brokenContract}

	resp, err := gate.Process(input, cfg)
	if err != nil {
		t.Fatalf("Process must not error on broken frontmatter (FAIL-SAFE): %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		t.Fatalf("response must be valid JSON: %v\nresponse: %s", err, resp)
	}
	// Must not inject (no valid frontmatter to derive phases from).
	if hso, ok := result["hookSpecificOutput"].(map[string]interface{}); ok {
		if ui, ok := hso["updatedInput"].(map[string]interface{}); ok {
			if p, ok := ui["prompt"].(string); ok {
				if strings.Contains(p, contractPath) {
					t.Errorf("broken frontmatter pass-through must not inject; prompt: %s", p)
				}
			}
		}
	}
}

// TC-13b: inject when injection header already exists in the prompt.
func TestInjectsUnderExistingHeader(t *testing.T) {
	// Prompt has the header but NOT the contract path yet.
	promptWithHeader := "Do tasks phase.\n\n## Skills to load before work\nRead some-other-skill.md\n"
	input := buildInput("sdd-tasks", promptWithHeader)
	cfg := gate.Config{ContractPath: contractPath, ContractContent: contractContent}

	resp, err := gate.Process(input, cfg)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		t.Fatalf("response not JSON: %v\nresponse: %s", err, resp)
	}
	hso, _ := result["hookSpecificOutput"].(map[string]interface{})
	updatedInput, _ := hso["updatedInput"].(map[string]interface{})
	newPrompt, _ := updatedInput["prompt"].(string)
	if !strings.Contains(newPrompt, contractPath) {
		t.Errorf("contract path should be injected under existing header; got:\n%s", newPrompt)
	}
	// Header must appear only once.
	if strings.Count(newPrompt, "## Skills to load before work") != 1 {
		t.Errorf("header should appear exactly once; got:\n%s", newPrompt)
	}
}

// TC-13c: inject when contract path is already in the prompt → no-op pass-through.
func TestNoOpWhenContractAlreadyPresent(t *testing.T) {
	promptAlreadyHas := "Do tasks.\n\n## Skills to load before work\nRead fully BEFORE work: " + contractPath + "\n"
	input := buildInput("sdd-tasks", promptAlreadyHas)
	cfg := gate.Config{ContractPath: contractPath, ContractContent: contractContent}

	resp, err := gate.Process(input, cfg)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	// Should be a pass-through (no hookSpecificOutput.updatedInput with new prompt).
	if hso, ok := result["hookSpecificOutput"].(map[string]interface{}); ok {
		if ui, ok := hso["updatedInput"].(map[string]interface{}); ok {
			if p, ok := ui["prompt"].(string); ok {
				// If updatedInput is present, the prompt must not duplicate the contract.
				if strings.Count(p, contractPath) > 1 {
					t.Errorf("contract path duplicated in already-present prompt; got:\n%s", p)
				}
			}
		}
	}
}

// TC-13: phase sets are derived from frontmatter, not hardcoded.
// Use a swapped frontmatter (sdd-spec injects, sdd-tasks excluded) and verify
// sdd-spec gets the injection and sdd-tasks gets stripped.
func TestPhaseSetsDerivedFromFrontmatter(t *testing.T) {
	swappedContract := `---
applies_to_phases: [sdd-spec]
excluded_phases: [sdd-tasks]
injection_point: "## Skills to load before work"
---
# Swapped Contract
`
	// sdd-spec should now INJECT.
	inputSpec := buildInput("sdd-spec", "Do spec phase.")
	cfgSwapped := gate.Config{ContractPath: contractPath, ContractContent: swappedContract}

	respSpec, err := gate.Process(inputSpec, cfgSwapped)
	if err != nil {
		t.Fatalf("Process(sdd-spec with swapped contract): %v", err)
	}
	var resultSpec map[string]interface{}
	if err := json.Unmarshal([]byte(respSpec), &resultSpec); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	hso, _ := resultSpec["hookSpecificOutput"].(map[string]interface{})
	updatedInput, _ := hso["updatedInput"].(map[string]interface{})
	newPromptSpec, _ := updatedInput["prompt"].(string)
	if !strings.Contains(newPromptSpec, contractPath) {
		t.Errorf("sdd-spec should inject when in applies_to_phases; got:\n%s", newPromptSpec)
	}

	// sdd-tasks should now STRIP (it's in excluded_phases for the swapped contract).
	promptWithContract := "Tasks.\n## Skills to load before work\nRead " + contractPath + ".\n"
	inputTasks := buildInput("sdd-tasks", promptWithContract)
	respTasks, err := gate.Process(inputTasks, cfgSwapped)
	if err != nil {
		t.Fatalf("Process(sdd-tasks with swapped contract): %v", err)
	}
	var resultTasks map[string]interface{}
	if err := json.Unmarshal([]byte(respTasks), &resultTasks); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	hsoT, _ := resultTasks["hookSpecificOutput"].(map[string]interface{})
	updatedInputT, _ := hsoT["updatedInput"].(map[string]interface{})
	newPromptTasks, _ := updatedInputT["prompt"].(string)
	if strings.Contains(newPromptTasks, contractPath) {
		t.Errorf("sdd-tasks should strip contract when in excluded_phases; got:\n%s", newPromptTasks)
	}
}
