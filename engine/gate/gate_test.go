package gate_test

// Tests for the policy of the gate: what it does to the prompt of a sub-agent of a given
// type, from the contracts it manages and the work it is told is under way. Nothing here
// knows a JSON document: how a hook input is read and how the answer is written is
// engine/hookwire's, and the whole chain, on the format the orchestrator really produces, is
// tested through the command in engine/cmd (gate_task_e2e_test.go and the golden files).

import (
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gate"
)

// singleContract is the configuration of a gate that manages one contract, at path, whose
// document is content.
func singleContract(path, content string) gate.Config {
	return gate.Config{Contracts: []gate.ContractConfig{{Path: path, Content: content}}}
}

// rewrite applies the gate to a sub-agent of the given type with the given prompt, and returns
// the prompt it comes out with and whether the gate changed it.
func rewrite(cfg gate.Config, subagentType, prompt string) (string, bool) {
	return gate.Rewrite(gate.Call{SubagentType: subagentType, Prompt: prompt}, cfg)
}

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

const ooContractContent = `---
applies_to_phases: [sdd-design, sdd-tasks, sdd-apply]
excluded_phases: [sdd-propose, sdd-spec, sdd-archive, sdd-verify]
injection_point: "## Skills to load before work"
language_context: [typescript, nestjs]
activation_context: [oo-domain-design, domain-heavy-application-code, review]
---
# OO Quality Contract
`

const ooContractPath = "skills/_shared/oo-quality-contract.md"

// canonicalContractEntry is the exact line the gate emits/recognizes for contractPath.
// This is a BARE path line — just the contract path itself, no prefix.
// This matches the orchestrator's real format (bare path lines under the header).
const canonicalContractEntry = contractPath

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

// ---- tests -----------------------------------------------------------------

// TC-1: sdd-tasks → contract path injected into prompt under injection_point header.
func TestInjectsForSddTasks(t *testing.T) {
	got, changed := rewrite(singleContract(contractPath, contractContent), "sdd-tasks", "Do the tasks phase.")
	if !changed {
		t.Fatal("the gate left the prompt of sdd-tasks alone, want the contract injected")
	}
	// Contract path must appear as an exact bare line.
	if !hasLine(got, contractPath) {
		t.Errorf("injected prompt should contain contract path %q as exact line; got:\n%s", contractPath, got)
	}
	if !strings.Contains(got, "## Skills to load before work") {
		t.Errorf("injected prompt should contain injection_point header; got:\n%s", got)
	}
	if !strings.HasPrefix(got, "Do the tasks phase.") {
		t.Errorf("the prompt must be kept; got:\n%s", got)
	}
}

// TC-2: sdd-apply → contract path injected into prompt.
func TestInjectsForSddApply(t *testing.T) {
	got, changed := rewrite(singleContract(contractPath, contractContent), "sdd-apply", "Apply the tasks.")
	if !changed || !hasLine(got, contractPath) {
		t.Errorf("sdd-apply should have contract injected as bare path line (changed=%v); got:\n%s", changed, got)
	}
}

// TC-3 to TC-7: the excluded phases → the canonical contract entry is stripped if present.
func TestStripsFromSddPropose(t *testing.T) {
	// Prompt already contains the canonical contract entry (bare path) — it should be stripped.
	promptWithContract := "Do propose.\n\n## Skills to load before work\n" + canonicalContractEntry + "\n"
	got, changed := rewrite(singleContract(contractPath, contractContent), "sdd-propose", promptWithContract)
	if !changed {
		t.Fatal("the gate left the prompt of sdd-propose alone, want the contract stripped")
	}
	assertNoCanonicalEntry(t, got)
}

func TestStripsFromSddSpec(t *testing.T) {
	got, changed := rewrite(singleContract(contractPath, contractContent), "sdd-spec", "Spec phase.\n## Skills to load before work\n"+canonicalContractEntry+"\n")
	if !changed {
		t.Fatal("the gate left the prompt of sdd-spec alone, want the contract stripped")
	}
	assertNoCanonicalEntry(t, got)
}

func TestStripsFromSddDesign(t *testing.T) {
	got, changed := rewrite(singleContract(contractPath, contractContent), "sdd-design", "Design phase.\n## Skills to load before work\n"+canonicalContractEntry+"\n")
	if !changed {
		t.Fatal("the gate left the prompt of sdd-design alone, want the contract stripped")
	}
	assertNoCanonicalEntry(t, got)
}

func TestStripsFromSddVerify(t *testing.T) {
	got, changed := rewrite(singleContract(contractPath, contractContent), "sdd-verify", "Verify.\n## Skills to load before work\n"+canonicalContractEntry+"\n")
	if !changed {
		t.Fatal("the gate left the prompt of sdd-verify alone, want the contract stripped")
	}
	assertNoCanonicalEntry(t, got)
}

func TestStripsFromSddArchive(t *testing.T) {
	got, changed := rewrite(singleContract(contractPath, contractContent), "sdd-archive", "Archive.\n## Skills to load before work\n"+canonicalContractEntry+"\n")
	if !changed {
		t.Fatal("the gate left the prompt of sdd-archive alone, want the contract stripped")
	}
	assertNoCanonicalEntry(t, got)
}

// TC-8: excluded phase with NO contract in prompt → nothing to do (no-op, no error).
func TestExcludedPhaseNoContractIsPassThrough(t *testing.T) {
	const prompt = "Do propose phase. No contract here."
	got, changed := rewrite(singleContract(contractPath, contractContent), "sdd-propose", prompt)
	if changed || got != prompt {
		t.Errorf("excluded phase with no contract must be left alone (changed=%v); got:\n%s", changed, got)
	}
}

// TC-9: unknown subagent_type → left unchanged (FAIL-SAFE).
func TestUnknownSubagentTypePassThrough(t *testing.T) {
	const prompt = "Do something unknown."
	got, changed := rewrite(singleContract(contractPath, contractContent), "some-future-phase", prompt)
	if changed || got != prompt {
		t.Errorf("an unknown sub-agent type must be left alone (changed=%v); got:\n%s", changed, got)
	}
}

// TC-10 and TC-11: nothing to act on → left unchanged (FAIL-SAFE). A call with no type, or with
// no prompt (the tool input had none), is not touched, and the zero value of everything is not an
// error or a panic.
func TestACallWithNothingToActOnIsLeftAlone(t *testing.T) {
	cfg := singleContract(contractPath, contractContent)
	for name, call := range map[string]gate.Call{
		"no sub-agent type": {Prompt: "Do tasks."},
		"no prompt":         {SubagentType: "sdd-tasks"},
		"neither":           {},
	} {
		got, changed := gate.Rewrite(call, cfg)
		if changed || got != call.Prompt {
			t.Errorf("%s: Rewrite() = %q, %v, want the prompt unchanged", name, got, changed)
		}
	}
	if got, changed := gate.Rewrite(gate.Call{SubagentType: "sdd-tasks", Prompt: "p"}, gate.Config{}); changed || got != "p" {
		t.Errorf("a gate that manages no contract changed a prompt: %q, %v", got, changed)
	}
}

// TC-12: contract frontmatter broken/missing → left unchanged (FAIL-SAFE, not loud).
func TestBrokenFrontmatterPassThrough(t *testing.T) {
	got, changed := rewrite(singleContract(contractPath, "no frontmatter here at all"), "sdd-tasks", "Do tasks.")
	// Must not inject (no valid frontmatter to derive phases from).
	if changed || strings.Contains(got, contractPath) {
		t.Errorf("broken frontmatter must not inject (changed=%v); got:\n%s", changed, got)
	}
}

// TC-13b: inject when injection header already exists in the prompt.
func TestInjectsUnderExistingHeader(t *testing.T) {
	// Prompt has the header but NOT the contract path yet.
	promptWithHeader := "Do tasks phase.\n\n## Skills to load before work\nRead some-other-skill.md\n"
	got, changed := rewrite(singleContract(contractPath, contractContent), "sdd-tasks", promptWithHeader)
	if !changed || !hasLine(got, contractPath) {
		t.Errorf("contract path should be injected under existing header (changed=%v); got:\n%s", changed, got)
	}
	// Header must appear only once.
	if strings.Count(got, "## Skills to load before work") != 1 {
		t.Errorf("header should appear exactly once; got:\n%s", got)
	}
}

// TC-13c: inject when contract path is already in the prompt → no-op.
func TestNoOpWhenContractAlreadyPresent(t *testing.T) {
	// Prompt already has the bare contract path line.
	promptAlreadyHas := "Do tasks.\n\n## Skills to load before work\n" + contractPath + "\n"
	got, changed := rewrite(singleContract(contractPath, contractContent), "sdd-tasks", promptAlreadyHas)
	if changed || got != promptAlreadyHas {
		t.Errorf("a prompt that already names the contract must be left alone (changed=%v); got:\n%s", changed, got)
	}
}

// TC-F1a: a DIFFERENT skill path that merely CONTAINS the contract path as a
// substring (e.g. 'other/skills/_shared/minimalism-contract.md') does NOT
// suppress injection for sdd-tasks — the real contract IS injected.
func TestExactMatchInjection_SubstringPathDoesNotSuppressInject(t *testing.T) {
	// Prompt already contains a DIFFERENT path that merely contains contractPath
	// as a substring. inject() must NOT treat this as "already present".
	superPath := "other/skills/_shared/minimalism-contract.md"
	// The super-path appears as a bare line (not as the canonical entry for contractPath).
	prompt := "Do tasks.\n\n## Skills to load before work\n" + superPath + "\n"
	got, changed := rewrite(singleContract(contractPath, contractContent), "sdd-tasks", prompt)
	if !changed {
		t.Fatalf("injection must have happened; got:\n%s", got)
	}
	// The exact contract path must now be present as a bare line.
	if !hasLine(got, contractPath) {
		t.Errorf("exact contract bare path line should be injected; got:\n%s", got)
	}
	// The super-path line must still be present (not stripped).
	if !strings.Contains(got, superPath) {
		t.Errorf("unrelated super-path should remain; got:\n%s", got)
	}
}

// TC-F1b: a 'minimalism-contract.md.bak' line in an excluded phase prompt is
// NOT collaterally stripped — only the exact contract entry is removed.
func TestExactMatchStrip_BackupLineNotStripped(t *testing.T) {
	backupLine := "skills/_shared/minimalism-contract.md.bak"
	// Prompt for an excluded phase (sdd-propose) that contains:
	//   - the exact contract entry (bare path — must be stripped)
	//   - a .bak line that must NOT be stripped
	prompt := "Do propose.\n\n## Skills to load before work\n" + contractPath + "\n" + backupLine + "\n"
	got, changed := rewrite(singleContract(contractPath, contractContent), "sdd-propose", prompt)
	if !changed {
		t.Fatalf("the strip should have happened; got:\n%s", got)
	}
	// The exact contract entry must be gone.
	for _, line := range strings.Split(got, "\n") {
		if strings.TrimSpace(line) == contractPath {
			t.Errorf("exact contract path line %q should be stripped but was found; full prompt:\n%s", contractPath, got)
		}
	}
	// The .bak line must remain.
	if !strings.Contains(got, backupLine) {
		t.Errorf(".bak line was collaterally stripped and must NOT be; got:\n%s", got)
	}
}

// TC-F1c: only the exact contract entry is added/removed.
// inject() must emit exactly the canonical bare path line and strip() must remove
// exactly that line (no broader text removal).
func TestExactMatchCanonicalEntry(t *testing.T) {
	cfg := singleContract(contractPath, contractContent)
	// Inject: start from blank prompt, verify canonical bare path line appears.
	injected, changed := rewrite(cfg, "sdd-tasks", "Do tasks.")
	if !changed {
		t.Fatalf("inject: the gate left the prompt alone; got:\n%s", injected)
	}
	// The canonical entry is the bare contract path as an exact line.
	if !hasLine(injected, contractPath) {
		t.Errorf("inject must emit canonical bare path line %q; got:\n%s", contractPath, injected)
	}
	// Must NOT use the old "Read fully BEFORE work:" prefix.
	if strings.Contains(injected, "Read fully BEFORE work:") {
		t.Errorf("injected entry must use bare path format, not 'Read fully BEFORE work:'; got:\n%s", injected)
	}

	// Strip: feed the injected prompt to an excluded phase, verify removal.
	stripped, changed := rewrite(cfg, "sdd-propose", injected)
	if !changed {
		t.Fatalf("strip: the gate left the prompt alone; got:\n%s", stripped)
	}
	for _, line := range strings.Split(stripped, "\n") {
		if strings.TrimSpace(line) == contractPath {
			t.Errorf("strip must remove canonical bare path line; got:\n%s", stripped)
		}
	}
	// The original task text must remain.
	if !strings.Contains(stripped, "Do tasks.") {
		t.Errorf("strip must not remove unrelated prompt content; got:\n%s", stripped)
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
	got, changed := rewrite(singleContract(contractPath, contractContent), "sdd-tasks", prompt)
	if !changed || !hasLine(got, contractPath) {
		t.Errorf("contract path must be injected as bare line even when header has a variant (changed=%v); got:\n%s", changed, got)
	}
}

// TC-F5: inject double-newline separator when prompt does not end with newline.
func TestInjectDoubleNewlineSeparator(t *testing.T) {
	// Prompt ends without a trailing newline — inject must use \n\n separator
	// before the header so the new section is visually separate.
	got, _ := rewrite(singleContract(contractPath, contractContent), "sdd-tasks", "Do tasks.")
	// The injected section must be separated from the original content by \n\n.
	if !strings.Contains(got, "Do tasks.\n\n## Skills to load before work") {
		t.Errorf("inject should use double-newline separator when prompt has no trailing newline; got:\n%q", got)
	}
}

// TC-W3: a contract whose frontmatter sets a non-default injection_point causes
// the contract path to be injected under THAT header, not the default.
// This test FAILS if the gate ignores injection_point and falls back to the
// default "## Skills to load before work" header instead.
func TestCustomInjectionPoint_UsesContractHeader(t *testing.T) {
	customContract := `---
applies_to_phases: [sdd-tasks, sdd-apply]
excluded_phases: [sdd-propose, sdd-spec, sdd-design, sdd-verify, sdd-archive]
injection_point: "## Custom Injection Header"
---
# Custom Header Contract
`
	const customInjectionHeader = "## Custom Injection Header"
	const defaultInjectionHeader = "## Skills to load before work"

	// Prompt has neither the custom header nor the default header.
	got, changed := rewrite(singleContract(contractPath, customContract), "sdd-tasks", "Do the tasks phase.")
	if !changed {
		t.Fatalf("the gate left the prompt alone; got:\n%s", got)
	}

	// The contract path must appear in the new prompt.
	if !hasLine(got, contractPath) {
		t.Errorf("contract path %q must be injected; got:\n%s", contractPath, got)
	}

	// CRITICAL: the custom injection header must be present, not the default one.
	// This assertion FAILS if the gate ignores injection_point and uses the default header.
	if !strings.Contains(got, customInjectionHeader) {
		t.Errorf("injection must use the custom injection_point header %q; got:\n%s", customInjectionHeader, got)
	}

	// The default injection header must NOT appear — it would mean injection_point was ignored.
	if strings.Contains(got, defaultInjectionHeader) {
		t.Errorf("default injection header %q must NOT appear when injection_point is customized; got:\n%s",
			defaultInjectionHeader, got)
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
	cfgSwapped := singleContract(contractPath, swappedContract)

	// sdd-spec should now INJECT.
	gotSpec, changed := rewrite(cfgSwapped, "sdd-spec", "Do spec phase.")
	if !changed || !hasLine(gotSpec, contractPath) {
		t.Errorf("sdd-spec should inject bare path line when in applies_to_phases (changed=%v); got:\n%s", changed, gotSpec)
	}

	// sdd-tasks should now STRIP (it's in excluded_phases for the swapped contract).
	// Use the bare path canonical entry format so strip() fires.
	gotTasks, changed := rewrite(cfgSwapped, "sdd-tasks", "Tasks.\n## Skills to load before work\n"+canonicalContractEntry+"\n")
	if !changed {
		t.Fatalf("sdd-tasks should strip the contract; got:\n%s", gotTasks)
	}
	assertNoCanonicalEntry(t, gotTasks)
}

func TestMultiContractDecisionsAreIndependent(t *testing.T) {
	cfg := gate.Config{Contracts: []gate.ContractConfig{
		{Path: contractPath, Content: contractContent},
		{Path: ooContractPath, Content: ooContractContent},
	}}

	got, _ := rewrite(cfg, "sdd-apply", "Apply implementation.")
	if !hasLine(got, contractPath) {
		t.Fatalf("phase-only contract should still inject; got:\n%s", got)
	}
	if hasLine(got, ooContractPath) {
		t.Fatalf("OO contract must not inject without trusted work context; got:\n%s", got)
	}
}

func TestOOContractRequiresTrustedMatchingContext(t *testing.T) {
	tests := []struct {
		name        string
		workContext *gate.WorkContext
		wantInject  bool
	}{
		{
			name: "trusted TypeScript domain work injects",
			workContext: &gate.WorkContext{
				Trusted:     true,
				Languages:   []string{"typescript"},
				Activations: []string{"oo-domain-design"},
				WorkKinds:   []string{"application-code"},
			},
			wantInject: true,
		},
		{
			name:        "prompt text is not proof",
			workContext: nil,
			wantInject:  false,
		},
		{
			name: "non-domain Go work passes through",
			workContext: &gate.WorkContext{
				Trusted:     true,
				Languages:   []string{"go"},
				Activations: []string{"non-domain"},
				WorkKinds:   []string{"application-code"},
			},
			wantInject: false,
		},
		{
			name: "untrusted matching context passes through",
			workContext: &gate.WorkContext{
				Trusted:     false,
				Languages:   []string{"typescript"},
				Activations: []string{"oo-domain-design"},
				WorkKinds:   []string{"application-code"},
			},
			wantInject: false,
		},
		{
			name: "missing work kinds are insufficient proof",
			workContext: &gate.WorkContext{
				Trusted:     true,
				Languages:   []string{"typescript"},
				Activations: []string{"oo-domain-design"},
			},
			wantInject: false,
		},
		{
			name: "empty work kinds are insufficient proof",
			workContext: &gate.WorkContext{
				Trusted:     true,
				Languages:   []string{"typescript"},
				Activations: []string{"oo-domain-design"},
				WorkKinds:   []string{},
			},
			wantInject: false,
		},
		{
			name: "docs work passes through",
			workContext: &gate.WorkContext{
				Trusted:     true,
				Languages:   []string{"typescript"},
				Activations: []string{"oo-domain-design"},
				WorkKinds:   []string{"docs"},
			},
			wantInject: false,
		},
		{
			name: "config work passes through",
			workContext: &gate.WorkContext{
				Trusted:     true,
				Languages:   []string{"typescript"},
				Activations: []string{"oo-domain-design"},
				WorkKinds:   []string{"config"},
			},
			wantInject: false,
		},
		{
			name: "generated artifact work passes through",
			workContext: &gate.WorkContext{
				Trusted:     true,
				Languages:   []string{"typescript"},
				Activations: []string{"oo-domain-design"},
				WorkKinds:   []string{"generated-artifact"},
			},
			wantInject: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := gate.Config{
				Contracts:   []gate.ContractConfig{{Path: ooContractPath, Content: ooContractContent}},
				WorkContext: tt.workContext,
			}
			prompt, _ := rewrite(cfg, "sdd-apply", "Please work on TypeScript NestJS SOLID domain modeling.")
			if got := hasLine(prompt, ooContractPath); got != tt.wantInject {
				t.Fatalf("OO injection = %v, want %v; prompt:\n%s", got, tt.wantInject, prompt)
			}
		})
	}
}

// The context lists of a contract keep the case they were written in (engine/contract does not
// fold it), so the gate is what makes a context match without regard to case, on both sides:
// a contract that writes "TypeScript" still applies to work that says "typescript", and the
// other way round. A comparison that lost the folding on either side would stop injecting the
// contract, with no error to say why.
func TestContextItemsMatchWithoutRegardToCase(t *testing.T) {
	withContext := func(languages, activations string) string {
		return "---\napplies_to_phases: [sdd-apply]\nexcluded_phases: []\ninjection_point: \"## Skills to load before work\"\n" +
			"language_context: " + languages + "\nactivation_context: " + activations + "\n---\n# OO Quality Contract\n"
	}
	work := func(languages, activations string) *gate.WorkContext {
		return &gate.WorkContext{Trusted: true, Languages: []string{languages}, Activations: []string{activations}, WorkKinds: []string{"Application-Code"}}
	}
	for _, tt := range []struct {
		name       string
		content    string
		workCtx    *gate.WorkContext
		wantInject bool
	}{
		{"a contract in mixed case, work in lower case", withContext("[TypeScript, NestJS]", "[OO-Domain-Design]"), work("typescript", "oo-domain-design"), true},
		{"a contract in lower case, work in mixed case", withContext("[typescript]", "[oo-domain-design]"), work("TypeScript", "OO-Domain-Design"), true},
		{"both in upper case", withContext("[TYPESCRIPT]", "[OO-DOMAIN-DESIGN]"), work("TYPESCRIPT", "OO-DOMAIN-DESIGN"), true},
		{"padding around the work's items is ignored", withContext("[typescript]", "[review]"), work(" typescript ", "\treview"), true},
		{"another language does not match whatever the case", withContext("[TypeScript]", "[review]"), work("Go", "review"), false},
		{"another activation does not match whatever the case", withContext("[TypeScript]", "[Review]"), work("typescript", "planning"), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := gate.Config{Contracts: []gate.ContractConfig{{Path: ooContractPath, Content: tt.content}}, WorkContext: tt.workCtx}
			prompt, _ := rewrite(cfg, "sdd-apply", "Apply implementation.")
			if got := hasLine(prompt, ooContractPath); got != tt.wantInject {
				t.Fatalf("injection = %v, want %v; prompt:\n%s", got, tt.wantInject, prompt)
			}
		})
	}
}

func TestMalformedOrUnsupportedContextContractSkipsOnlyThatContract(t *testing.T) {
	malformedOO := `---
applies_to_phases: [sdd-apply]
excluded_phases: []
injection_point: "## Skills to load before work"
language_context: typescript
activation_context: [oo-domain-design]
---
# Broken OO Contract
`
	unsupportedOO := `---
applies_to_phases: [sdd-apply]
excluded_phases: []
injection_point: "## Skills to load before work"
language_context: [typescript]
activation_context: [oo-domain-design]
context_operator: prompt_contains
---
# Unsupported OO Contract
`

	valid := gate.ContractConfig{Path: contractPath, Content: contractContent}
	for _, content := range []string{malformedOO, unsupportedOO} {
		bad := gate.ContractConfig{Path: ooContractPath, Content: content}
		// The bad contract comes before the valid one as well as after it: skipping it must not
		// stop the gate from reading the contracts that follow.
		for name, contracts := range map[string][]gate.ContractConfig{"after": {valid, bad}, "before": {bad, valid}} {
			cfg := gate.Config{Contracts: contracts,
				WorkContext: &gate.WorkContext{Trusted: true, Languages: []string{"typescript"}, Activations: []string{"oo-domain-design"}}}
			prompt, _ := rewrite(cfg, "sdd-apply", "Apply implementation.")
			if !hasLine(prompt, contractPath) {
				t.Fatalf("bad contract %s the valid one: the valid phase-only contract should still inject; got:\n%s", name, prompt)
			}
			if hasLine(prompt, ooContractPath) {
				t.Fatalf("bad contract %s the valid one: the bad OO contract should be skipped; got:\n%s", name, prompt)
			}
		}
	}
}

func hasLine(prompt, want string) bool {
	for _, line := range strings.Split(prompt, "\n") {
		if strings.TrimSpace(line) == want {
			return true
		}
	}
	return false
}
