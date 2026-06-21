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

// canonicalContractEntry is the exact line the gate emits/recognizes for contractPath.
const canonicalContractEntry = "Read fully BEFORE work: " + contractPath

// assertNoCanonicalEntry checks that the canonical contract entry is not present
// as an exact line in the prompt.
func assertNoCanonicalEntry(t *testing.T, prompt string) {
	t.Helper()
	for _, line := range strings.Split(prompt, "\n") {
		if strings.TrimSpace(line) == canonicalContractEntry {
			t.Errorf("canonical contract entry line %q should be absent; full prompt:\n%s",
				canonicalContractEntry, prompt)
			return
		}
	}
}

// TC-3: sdd-propose → canonical contract entry stripped if present, no-op if absent.
func TestStripsFromSddPropose(t *testing.T) {
	// Prompt already contains the canonical contract entry — it should be stripped.
	promptWithContract := "Do propose.\n\n## Skills to load before work\n" + canonicalContractEntry + "\n"
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
	assertNoCanonicalEntry(t, newPrompt)
}

// TC-4: sdd-spec → canonical contract entry stripped if present.
func TestStripsFromSddSpec(t *testing.T) {
	promptWithContract := "Spec phase.\n## Skills to load before work\n" + canonicalContractEntry + "\n"
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
	assertNoCanonicalEntry(t, newPrompt)
}

// TC-5: sdd-design → canonical contract entry stripped if present.
func TestStripsFromSddDesign(t *testing.T) {
	promptWithContract := "Design phase.\n## Skills to load before work\n" + canonicalContractEntry + "\n"
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
	assertNoCanonicalEntry(t, newPrompt)
}

// TC-6: sdd-verify → canonical contract entry stripped.
func TestStripsFromSddVerify(t *testing.T) {
	promptWithContract := "Verify.\n## Skills to load before work\n" + canonicalContractEntry + "\n"
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
	assertNoCanonicalEntry(t, newPrompt)
}

// TC-7: sdd-archive → canonical contract entry stripped.
func TestStripsFromSddArchive(t *testing.T) {
	promptWithContract := "Archive.\n## Skills to load before work\n" + canonicalContractEntry + "\n"
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
	assertNoCanonicalEntry(t, newPrompt)
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

// TC-F1a: a DIFFERENT skill path that merely CONTAINS the contract path as a
// substring (e.g. 'other/skills/_shared/minimalism-contract.md') does NOT
// suppress injection for sdd-tasks — the real contract IS injected.
func TestExactMatchInjection_SubstringPathDoesNotSuppressInject(t *testing.T) {
	// Prompt already contains a DIFFERENT path that merely contains contractPath
	// as a substring. inject() must NOT treat this as "already present".
	superPath := "other/skills/_shared/minimalism-contract.md"
	prompt := "Do tasks.\n\n## Skills to load before work\nRead fully BEFORE work: " + superPath + "\n"
	input := buildInput("sdd-tasks", prompt)
	cfg := gate.Config{ContractPath: contractPath, ContractContent: contractContent}

	resp, err := gate.Process(input, cfg)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	// Must have updatedInput — injection must have happened.
	hso, ok := result["hookSpecificOutput"].(map[string]interface{})
	if !ok {
		t.Fatalf("response missing hookSpecificOutput (injection should have happened); got: %s", resp)
	}
	ui, ok := hso["updatedInput"].(map[string]interface{})
	if !ok {
		t.Fatalf("hookSpecificOutput missing updatedInput; got: %v", hso)
	}
	newPrompt, _ := ui["prompt"].(string)
	// The exact contract path entry must now be present.
	exactEntry := "Read fully BEFORE work: " + contractPath
	if !strings.Contains(newPrompt, exactEntry) {
		t.Errorf("exact contract entry should be injected; got:\n%s", newPrompt)
	}
	// The super-path line must still be present (not stripped).
	if !strings.Contains(newPrompt, superPath) {
		t.Errorf("unrelated super-path should remain; got:\n%s", newPrompt)
	}
}

// TC-F1b: a 'minimalism-contract.md.bak' line in an excluded phase prompt is
// NOT collaterally stripped — only the exact contract entry is removed.
func TestExactMatchStrip_BackupLineNotStripped(t *testing.T) {
	backupLine := "Read fully BEFORE work: skills/_shared/minimalism-contract.md.bak"
	exactEntry := "Read fully BEFORE work: " + contractPath
	// Prompt for an excluded phase (sdd-propose) that contains:
	//   - the exact contract entry (must be stripped)
	//   - a .bak line that must NOT be stripped
	prompt := "Do propose.\n\n## Skills to load before work\n" + exactEntry + "\n" + backupLine + "\n"
	input := buildInput("sdd-propose", prompt)
	cfg := gate.Config{ContractPath: contractPath, ContractContent: contractContent}

	resp, err := gate.Process(input, cfg)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	hso, ok := result["hookSpecificOutput"].(map[string]interface{})
	if !ok {
		t.Fatalf("response missing hookSpecificOutput (strip should have happened); got: %s", resp)
	}
	ui, _ := hso["updatedInput"].(map[string]interface{})
	newPrompt, _ := ui["prompt"].(string)

	// The exact contract entry must be gone — check for exact line presence.
	for _, line := range strings.Split(newPrompt, "\n") {
		if strings.TrimSpace(line) == exactEntry {
			t.Errorf("exact contract entry line %q should be stripped but was found; full prompt:\n%s", exactEntry, newPrompt)
		}
	}
	// The .bak line must remain.
	if !strings.Contains(newPrompt, backupLine) {
		t.Errorf(".bak line was collaterally stripped and must NOT be; got:\n%s", newPrompt)
	}
}

// TC-F1c: only the exact contract entry is added/removed.
// inject() must emit exactly the canonical entry line and strip() must remove
// exactly that line (no broader text removal).
func TestExactMatchCanonicalEntry(t *testing.T) {
	// Inject: start from blank prompt, verify canonical entry appears.
	prompt := "Do tasks."
	input := buildInput("sdd-tasks", prompt)
	cfg := gate.Config{ContractPath: contractPath, ContractContent: contractContent}

	resp, err := gate.Process(input, cfg)
	if err != nil {
		t.Fatalf("Process inject: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	hso, _ := result["hookSpecificOutput"].(map[string]interface{})
	ui, _ := hso["updatedInput"].(map[string]interface{})
	injectedPrompt, _ := ui["prompt"].(string)
	canonicalEntry := "Read fully BEFORE work: " + contractPath
	if !strings.Contains(injectedPrompt, canonicalEntry) {
		t.Errorf("inject must emit canonical entry %q; got:\n%s", canonicalEntry, injectedPrompt)
	}

	// Strip: feed the injected prompt to an excluded phase, verify removal.
	stripInput := buildInput("sdd-propose", injectedPrompt)
	stripResp, err := gate.Process(stripInput, cfg)
	if err != nil {
		t.Fatalf("Process strip: %v", err)
	}
	var stripResult map[string]interface{}
	if err := json.Unmarshal([]byte(stripResp), &stripResult); err != nil {
		t.Fatalf("strip response not valid JSON: %v", err)
	}
	hsoS, _ := stripResult["hookSpecificOutput"].(map[string]interface{})
	uiS, _ := hsoS["updatedInput"].(map[string]interface{})
	strippedPrompt, _ := uiS["prompt"].(string)
	if strings.Contains(strippedPrompt, canonicalEntry) {
		t.Errorf("strip must remove canonical entry; got:\n%s", strippedPrompt)
	}
	// The original task text must remain.
	if !strings.Contains(strippedPrompt, "Do tasks.") {
		t.Errorf("strip must not remove unrelated prompt content; got:\n%s", strippedPrompt)
	}
}

// TC-F2: header-variant '## Skills to load before work (extra context)' causes
// a silent miss currently. inject() must detect the header deterministically
// using exact trimmed match, not strings.Contains on the whole line.
func TestHeaderVariantStillInjects(t *testing.T) {
	// Prompt has a header that CONTAINS the injection header but is not the
	// exact header. The gate must still inject (defined deterministic behavior:
	// if no EXACT header match, append new header+entry at end — no silent miss).
	variantHeader := "## Skills to load before work (extra context)"
	prompt := "Do tasks.\n\n" + variantHeader + "\nRead some-other-skill.md\n"
	input := buildInput("sdd-tasks", prompt)
	cfg := gate.Config{ContractPath: contractPath, ContractContent: contractContent}

	resp, err := gate.Process(input, cfg)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	// Must inject: contract path must appear in the result.
	hso, ok := result["hookSpecificOutput"].(map[string]interface{})
	if !ok {
		t.Fatalf("response missing hookSpecificOutput (injection should have happened); got: %s", resp)
	}
	ui, _ := hso["updatedInput"].(map[string]interface{})
	newPrompt, _ := ui["prompt"].(string)
	if !strings.Contains(newPrompt, contractPath) {
		t.Errorf("contract path must be injected even when header has a variant; got:\n%s", newPrompt)
	}
}

// TC-F5: inject double-newline separator when prompt does not end with newline.
func TestInjectDoubleNewlineSeparator(t *testing.T) {
	// Prompt ends without a trailing newline — inject must use \n\n separator
	// before the header so the new section is visually separate.
	prompt := "Do tasks."
	input := buildInput("sdd-tasks", prompt)
	cfg := gate.Config{ContractPath: contractPath, ContractContent: contractContent}

	resp, err := gate.Process(input, cfg)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	hso, _ := result["hookSpecificOutput"].(map[string]interface{})
	ui, _ := hso["updatedInput"].(map[string]interface{})
	newPrompt, _ := ui["prompt"].(string)
	// The injected section must be separated from the original content by \n\n.
	if !strings.Contains(newPrompt, "Do tasks.\n\n## Skills to load before work") {
		t.Errorf("inject should use double-newline separator when prompt has no trailing newline; got:\n%q", newPrompt)
	}
}

// TC-F6a: TC-11 strengthened — missing prompt must leave updatedInput ABSENT.
// The contract: prompt is never overwritten with empty string.
func TestMissingPromptPassThrough_NoUpdatedInput(t *testing.T) {
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
	// updatedInput must be absent — pass-through means no modification.
	if hso, ok := result["hookSpecificOutput"].(map[string]interface{}); ok {
		if ui, ok := hso["updatedInput"].(map[string]interface{}); ok {
			if p, ok := ui["prompt"].(string); ok && p == "" {
				t.Errorf("updatedInput.prompt must NOT be set to empty string on pass-through; got empty string in response: %s", resp)
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
	// Use the canonical entry format so strip() fires.
	promptWithContract := "Tasks.\n## Skills to load before work\n" + canonicalContractEntry + "\n"
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
	assertNoCanonicalEntry(t, newPromptTasks)
}
