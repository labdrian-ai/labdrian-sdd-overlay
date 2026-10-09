package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The golden files under testdata/contract-golden record what the verbs that read a
// contract document print today: 'gate-task' (the PreToolUse hook that injects a contract
// into a sub-agent prompt, or strips it), 'propagate' (which scopes a registry row from the
// contract's frontmatter), the contract check of the status verb, and 'runtime install
// --target opencode' (which derives the prompt configuration of the OpenCode plugin from
// the contracts of the overlay). They were recorded from the program as it was before
// Phase 9 unit H13 (docs/architecture/hexagonal-target.md) put one parse of a contract
// document, and one strict list parser, in engine/contract: until then three places read
// a contract's frontmatter with three different list parsers (propagator, gate, runtime),
// and the lists of phases were read leniently.
//
// The cases named *-accepted-today are the places where the lenient parser took a list that
// the strict one refuses (decision D4: contract lists use the strict parser). They are the
// only goldens the H13 refactor changes, each on purpose, and the commit that changes them
// says so; every other golden stays byte for byte. Rewrite them deliberately with
//
//	go test ./cmd -run TestContractGolden -update-contract-golden
//
// and read the diff before committing it.
//
// The verbs run in process over an in-memory file system, except 'runtime install', which
// is given a temporary overlay and configuration root. Each case is its own subtest.
var updateContractGolden = flag.Bool("update-contract-golden", false, "rewrite the golden files of the contract-reading verbs")

// contractWorld is the scratch space of one case: an in-memory file system the verbs read
// from, and the transcript of what they printed.
type contractWorld struct {
	t     *testing.T
	files map[string]string
	b     strings.Builder
	names map[string]string
}

func newContractWorld(t *testing.T) *contractWorld {
	t.Helper()
	return &contractWorld{t: t, files: map[string]string{}, names: map[string]string{}}
}

func (w *contractWorld) write(format string, args ...any) { fmt.Fprintf(&w.b, format, args...) }

// name registers path to be written as placeholder in the transcript.
func (w *contractWorld) name(path, placeholder string) { w.names[path] = placeholder }

func (w *contractWorld) text() string {
	text := w.b.String()
	paths := make([]string, 0, len(w.names))
	for p := range w.names {
		paths = append(paths, p)
	}
	sort.Slice(paths, func(i, j int) bool { return len(paths[i]) > len(paths[j]) })
	for _, p := range paths {
		text = strings.ReplaceAll(text, p, w.names[p])
	}
	return text
}

func (w *contractWorld) readFile(path string) ([]byte, error) {
	if content, ok := w.files[path]; ok {
		return []byte(content), nil
	}
	return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
}

// The fixed places of the in-memory file system.
const (
	contractFile  = "/virtual/skills/_shared/contract.md"
	registryFile  = "/virtual/project/.atl/skill-registry.md"
	contractEntry = "skills/_shared/minimalism-contract.md"
)

// contractDoc is a contract document with the given frontmatter lines.
func contractDoc(frontmatter ...string) string {
	return "---\n" + strings.Join(frontmatter, "\n") + "\n---\n# A contract\n\nThe body is not read.\n"
}

// The frontmatter lines of a well-formed contract.
const (
	fmApplies  = "applies_to_phases: [sdd-tasks, sdd-apply]"
	fmExcluded = "excluded_phases: [sdd-propose, sdd-spec, sdd-design, sdd-verify, sdd-archive]"
	fmInject   = `injection_point: "## Skills to load before work"`
)

// agentInput is the PreToolUse input of the Agent tool for a sub-agent of the given type.
func agentInput(subagentType, prompt string) string {
	b, _ := json.Marshal(map[string]any{
		"tool_name":  "Agent",
		"tool_input": map[string]any{"description": "a sub-agent", "subagent_type": subagentType, "prompt": prompt},
	})
	return string(b)
}

// The prompts the gate is shown: one with no skills section, one with it and another skill,
// and one that already names the contract (so a strip has something to remove).
const (
	promptPlain      = "Do the phase."
	promptWithHeader = "Do the phase.\n\n## Skills to load before work\n/skills/other/SKILL.md\n"
	promptWithEntry  = "Do the phase.\n\n## Skills to load before work\n/skills/other/SKILL.md\n" + contractEntry + "\n"
)

// gate records one run of 'gate-task' with the given stdin and arguments.
func (w *contractWorld) gate(label, stdin string, args ...string) {
	w.t.Helper()
	var stdout, stderr bytes.Buffer
	gateTaskCore(args, strings.NewReader(stdin), &stdout, &stderr, w.readFile)
	w.write("$ gate-task %s\n# %s\n--- stdin ---\n%s\n--- stdout ---\n%s--- stderr ---\n%s\n",
		strings.Join(args, " "), label, ensureNewline(stdin), ensureNewline(stdout.String()), ensureNewline(stderr.String()))
}

// gateMatrix records the four runs that show what a contract does to a sub-agent prompt:
// an applies-to phase without the skills section, one with it, an excluded phase that
// carries the contract, and a phase the contract names nowhere.
func (w *contractWorld) gateMatrix(args ...string) {
	w.t.Helper()
	w.gate("sdd-apply, no skills section", agentInput("sdd-apply", promptPlain), args...)
	w.gate("sdd-tasks, with a skills section", agentInput("sdd-tasks", promptWithHeader), args...)
	w.gate("sdd-propose carrying the contract", agentInput("sdd-propose", promptWithEntry), args...)
	w.gate("sdd-explore, in neither list", agentInput("sdd-explore", promptWithEntry), args...)
}

// propagate records one run of 'propagate'. The registry is written back into the
// in-memory file system, and printed when it changed.
func (w *contractWorld) propagate(label string, args ...string) {
	w.t.Helper()
	var stdout, stderr bytes.Buffer
	exitCode := -1
	before := w.files[registryFile]
	writeFile := func(path string, data []byte, _ os.FileMode) error {
		w.files[path] = string(data)
		return nil
	}
	runPropagateCore(args, &stdout, &stderr, w.readFile, writeFile, func(code int) { exitCode = code })
	exit := "none"
	if exitCode >= 0 {
		exit = fmt.Sprint(exitCode)
	}
	w.write("$ propagate %s\n# %s\nexit: %s\n--- stdout ---\n%s--- stderr ---\n%s", strings.Join(args, " "), label, exit, ensureNewline(stdout.String()), ensureNewline(stderr.String()))
	if after, ok := w.files[registryFile]; ok && after != before {
		w.write("--- registry after ---\n%s", ensureNewline(after))
	} else {
		w.write("--- registry unchanged ---\n")
	}
	w.write("\n")
}

// goldenRegistry is a registry with a Shared Contracts table and a foreign marker block.
const goldenRegistry = "# Skill Registry\n\n### Shared Contracts\n\n| Artifact | Path | Description |\n|----------|------|-------------|\n" +
	"| pre-sdd-contracts | skills/_shared/pre-sdd-contracts.md | Shared contracts |\n" +
	"<!-- BEGIN: some-other-scope (auto-generated) -->\n| other | skills/_shared/other.md | Foreign row |\n<!-- END: some-other-scope -->\n\n### Other section\n\ntext\n"

func (w *contractWorld) withRegistry(contractContent string) {
	w.files[registryFile] = goldenRegistry
	w.files[contractFile] = contractContent
}

// checkContract records the contract check of the status verb for the contract in the
// in-memory file system.
func (w *contractWorld) checkContract(label string) {
	res := checkContract(contractFile, w.readFile)
	w.write("# %s\nlabel: %s\nok: %v\nnote: %s\n\n", label, res.label, res.ok, res.note)
}

// opencodeInstall records 'runtime install --target opencode' with a temporary overlay whose
// minimalism contract and optional OO-quality contract are the given text, then the
// prompt configuration it wrote.
func (w *contractWorld) opencodeInstall(minimalism, oo string) {
	w.t.Helper()
	overlay, configRoot := w.t.TempDir(), w.t.TempDir()
	shared := filepath.Join(overlay, "skills", "_shared")
	if err := os.MkdirAll(shared, 0o755); err != nil {
		w.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shared, "minimalism-contract.md"), []byte(minimalism), 0o644); err != nil {
		w.t.Fatal(err)
	}
	if oo != "" {
		if err := os.WriteFile(filepath.Join(shared, "oo-quality-contract.md"), []byte(oo), 0o644); err != nil {
			w.t.Fatal(err)
		}
	}
	w.t.Setenv("LABDRIAN_OVERLAY_DIR", overlay)
	w.name(overlay, "<OVERLAY>")
	w.name(configRoot, "<CONFIG>")

	var stdout, stderr bytes.Buffer
	exitCode := -1
	runRuntimeCore(noPi(), noGit(), []string{"install", "--target", "opencode", "--config-root", configRoot}, &stdout, &stderr, func(code int) { exitCode = code })
	exit := "none"
	if exitCode >= 0 {
		exit = fmt.Sprint(exitCode)
	}
	w.write("$ runtime install --target opencode --config-root <CONFIG>\nexit: %s\n--- stdout ---\n%s--- stderr ---\n%s", exit, ensureNewline(stdout.String()), ensureNewline(stderr.String()))
	data, err := os.ReadFile(filepath.Join(configRoot, "labdrian-runtime-parity.json"))
	if err != nil {
		w.write("--- prompt_config ---\n(no configuration written)\n")
		return
	}
	var config struct {
		PromptConfig json.RawMessage `json:"prompt_config"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		w.t.Fatalf("decode the configuration: %v", err)
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, config.PromptConfig, "", "  "); err != nil {
		w.t.Fatalf("indent the prompt configuration: %v", err)
	}
	w.write("--- prompt_config ---\n%s\n", pretty.String())
}

// contractGoldenCase is one scenario and the golden file its transcript is compared with.
type contractGoldenCase struct {
	name string
	run  func(w *contractWorld)
}

// A trusted work context for an application-code unit written in TypeScript with an
// activation the OO-quality contract asks for.
const (
	workContextMatching    = `{"trusted":true,"languages":["TypeScript"],"activations":["review"],"work_kinds":["application-code"]}`
	workContextOtherLang   = `{"trusted":true,"languages":["go"],"activations":["review"],"work_kinds":["application-code"]}`
	workContextUntrusted   = `{"trusted":false,"languages":["typescript"],"activations":["review"],"work_kinds":["application-code"]}`
	workContextNoWorkKinds = `{"trusted":true,"languages":["typescript"],"activations":["review"]}`
	workContextWrongKind   = `{"trusted":true,"languages":["typescript"],"activations":["review"],"work_kinds":["documentation"]}`
	workContextOtherAct    = `{"trusted":true,"languages":["typescript"],"activations":["planning"],"work_kinds":["application-code"]}`
)

var ooContract = contractDoc(
	"applies_to_phases: [sdd-design, sdd-tasks, sdd-apply]",
	"excluded_phases: [sdd-propose, sdd-spec, sdd-archive, sdd-verify]",
	fmInject,
	"language_context: [typescript, nestjs]",
	"activation_context: [oo-domain-design, domain-heavy-application-code, review]",
)

func contractGoldenCases() []contractGoldenCase {
	fileArgs := []string{"--contract-file", contractFile, "--contract-path", contractEntry}
	return []contractGoldenCase{
		// ---- gate-task ----------------------------------------------------------------
		{"gate-task-file-contract", func(w *contractWorld) {
			w.files[contractFile] = contractDoc(fmApplies, fmExcluded, fmInject)
			w.gateMatrix(fileArgs...)
			w.gate("sdd-apply that already names the contract", agentInput("sdd-apply", promptWithEntry), fileArgs...)
			w.gate("a model is echoed", `{"tool_name":"Agent","tool_input":{"description":"d","subagent_type":"sdd-apply","prompt":"p","model":"sonnet"}}`, fileArgs...)
			w.gate("the default contract path", agentInput("sdd-apply", promptPlain), "--contract-file", contractFile)
		}},
		{"gate-task-embedded-contract", func(w *contractWorld) {
			w.gateMatrix("--embedded-contract", "anti-generic-design")
			w.gate("an explicit path for the embedded contract", agentInput("sdd-apply", promptPlain), "--embedded-contract", "anti-generic-design", "--contract-path", "/abs/anti-generic-design.md")
			w.gate("an unknown embedded contract", agentInput("sdd-apply", promptPlain), "--embedded-contract", "no-such-contract")
		}},
		{"gate-task-injection-point", func(w *contractWorld) {
			for _, point := range []string{`injection_point: "## Custom header"`, `injection_point: '## Single quoted'`, `injection_point: ## Bare header`, `injection_point:`} {
				w.files[contractFile] = contractDoc(fmApplies, fmExcluded, point)
				w.gate("with "+point, agentInput("sdd-apply", "Do it.\n\n## Custom header\n"), fileArgs...)
			}
		}},
		{"gate-task-context-contract", func(w *contractWorld) {
			w.files[contractFile] = ooContract
			w.gate("a matching trusted context", agentInput("sdd-apply", promptPlain), append(fileArgs, "--work-context-json", workContextMatching)...)
			w.gate("another language", agentInput("sdd-apply", promptPlain), append(fileArgs, "--work-context-json", workContextOtherLang)...)
			w.gate("an untrusted context", agentInput("sdd-apply", promptPlain), append(fileArgs, "--work-context-json", workContextUntrusted)...)
			w.gate("no work kinds", agentInput("sdd-apply", promptPlain), append(fileArgs, "--work-context-json", workContextNoWorkKinds)...)
			w.gate("work that is not application code", agentInput("sdd-apply", promptPlain), append(fileArgs, "--work-context-json", workContextWrongKind)...)
			w.gate("an activation the contract does not ask for", agentInput("sdd-apply", promptPlain), append(fileArgs, "--work-context-json", workContextOtherAct)...)
			w.gate("no context at all", agentInput("sdd-apply", promptPlain), fileArgs...)
			w.gate("an excluded phase strips whatever the context", agentInput("sdd-propose", promptWithEntry), append(fileArgs, "--work-context-json", workContextOtherLang)...)
			w.gate("a context that is not JSON", agentInput("sdd-apply", promptPlain), append(fileArgs, "--work-context-json", "not json")...)
			w.files["/virtual/context.json"] = workContextMatching
			w.gate("a context file", agentInput("sdd-apply", promptPlain), append(fileArgs, "--work-context-file", "/virtual/context.json")...)
			w.gate("both forms of the context", agentInput("sdd-apply", promptPlain), append(fileArgs, "--work-context-json", workContextMatching, "--work-context-file", "/virtual/context.json")...)
			w.gate("a context file that is missing", agentInput("sdd-apply", promptPlain), append(fileArgs, "--work-context-file", "/virtual/missing.json")...)
		}},
		{"gate-task-refuses-a-contract-it-cannot-use", func(w *contractWorld) {
			input := agentInput("sdd-apply", promptPlain)
			w.gate("no contract source", input)
			w.gate("a contract file that is missing", input, fileArgs...)
			for _, c := range []struct{ label, content string }{
				{"an empty file", ""},
				{"no frontmatter", "no frontmatter here"},
				{"one delimiter only", "---\napplies_to_phases: [sdd-apply]\n"},
				{"no applies_to_phases", contractDoc(fmExcluded, fmInject)},
				{"an empty applies_to_phases list", contractDoc("applies_to_phases: []", fmInject)},
				{"an applies_to_phases with no value", contractDoc("applies_to_phases:", fmInject)},
				// A scope that reads and a context that does not: the gate reads both (ParseBoth), so
				// the warning names the context's fault.
				{"a language_context that is not a list", contractDoc(fmApplies, fmInject, "language_context: typescript")},
				{"an activation_context that is not a list", contractDoc(fmApplies, fmInject, "activation_context: review")},
				{"a context_operator", contractDoc(fmApplies, fmInject, "context_operator: prompt_contains")},
			} {
				w.files[contractFile] = c.content
				w.gate(c.label, input, fileArgs...)
			}
		}},
		{"gate-task-refuses-an-input-it-cannot-use", func(w *contractWorld) {
			w.files[contractFile] = contractDoc(fmApplies, fmExcluded, fmInject)
			for _, in := range []struct{ label, stdin string }{
				{"empty input", ""},
				{"not JSON", "not json"},
				{"no tool_input", `{"tool_name":"Agent"}`},
				{"a tool_input that is not an object", `{"tool_name":"Agent","tool_input":"text"}`},
				{"no subagent_type", `{"tool_name":"Agent","tool_input":{"description":"d","prompt":"p"}}`},
				{"no prompt", `{"tool_name":"Agent","tool_input":{"description":"d","subagent_type":"sdd-apply"}}`},
			} {
				w.gate(in.label, in.stdin, fileArgs...)
			}
		}},
		{"gate-task-frontmatter-shapes", func(w *contractWorld) {
			input := agentInput("sdd-apply", promptPlain)
			for _, c := range []struct{ label, content string }{
				{"keys indented", "---\n  applies_to_phases: [sdd-apply]\n\t" + fmExcluded + "\n---\n"},
				{"CRLF line endings", "---\r\napplies_to_phases: [sdd-apply]\r\nexcluded_phases: [sdd-propose]\r\n---\r\n# c\r\n"},
				{"a later key wins over an earlier one", contractDoc("applies_to_phases: [sdd-tasks]", "applies_to_phases: [sdd-apply]")},
				{"a preamble before the delimiters", "preamble text\n---\napplies_to_phases: [sdd-apply]\n---\nbody\n"},
				{"a delimiter inside a value is part of the value", contractDoc("injection_point: \"## a --- b\"", "applies_to_phases: [sdd-apply]")},
				{"spaces around items", contractDoc("applies_to_phases: [ sdd-apply ,  sdd-tasks ]")},
				{"an empty item between commas", contractDoc("applies_to_phases: [sdd-apply, , sdd-tasks]")},
				{"a key that merely starts with the name", contractDoc("applies_to_phases_extra: [sdd-apply]", "applies_to_phases: [sdd-tasks]")},
			} {
				w.files[contractFile] = c.content
				w.gate(c.label, input, fileArgs...)
			}
		}},
		// ---- the lists a lenient parser accepts and a strict one refuses --------------
		{"gate-task-phases-without-brackets-accepted-today", func(w *contractWorld) {
			w.files[contractFile] = contractDoc("applies_to_phases: sdd-tasks, sdd-apply", fmExcluded, fmInject)
			w.gateMatrix(fileArgs...)
		}},
		{"gate-task-phases-unclosed-bracket-accepted-today", func(w *contractWorld) {
			w.files[contractFile] = contractDoc("applies_to_phases: [sdd-tasks, sdd-apply", fmExcluded, fmInject)
			w.gateMatrix(fileArgs...)
		}},
		{"gate-task-phases-quoted-items-accepted-today", func(w *contractWorld) {
			w.files[contractFile] = contractDoc(`applies_to_phases: ["sdd-tasks", 'sdd-apply']`, `excluded_phases: ["sdd-propose"]`, fmInject)
			w.gateMatrix(fileArgs...)
		}},
		{"gate-task-excluded-without-brackets-accepted-today", func(w *contractWorld) {
			w.files[contractFile] = contractDoc(fmApplies, "excluded_phases: sdd-propose, sdd-spec", fmInject)
			w.gateMatrix(fileArgs...)
		}},
		{"gate-task-excluded-with-no-value-accepted-today", func(w *contractWorld) {
			w.files[contractFile] = contractDoc(fmApplies, "excluded_phases:", fmInject)
			w.gateMatrix(fileArgs...)
		}},
		{"gate-task-phases-and-excluded-with-trailing-text-accepted-today", func(w *contractWorld) {
			w.files[contractFile] = contractDoc("applies_to_phases: [sdd-tasks, sdd-apply] # the phases", "excluded_phases: [sdd-propose] # not these", fmInject)
			w.gateMatrix(fileArgs...)
		}},
		// ---- the context metadata ------------------------------------------------------
		{"gate-task-malformed-language-context", func(w *contractWorld) {
			w.files[contractFile] = contractDoc(fmApplies, fmExcluded, fmInject, "language_context: typescript", "activation_context: [review]")
			w.gate("a matching context", agentInput("sdd-apply", promptPlain), append(fileArgs, "--work-context-json", workContextMatching)...)
			w.gate("an excluded phase", agentInput("sdd-propose", promptWithEntry), append(fileArgs, "--work-context-json", workContextMatching)...)
		}},
		{"gate-task-malformed-activation-context", func(w *contractWorld) {
			w.files[contractFile] = contractDoc(fmApplies, fmExcluded, fmInject, "language_context: [typescript]", "activation_context: review")
			w.gate("a matching context", agentInput("sdd-apply", promptPlain), append(fileArgs, "--work-context-json", workContextMatching)...)
			w.gate("an excluded phase", agentInput("sdd-propose", promptWithEntry), append(fileArgs, "--work-context-json", workContextMatching)...)
		}},
		{"gate-task-unsupported-context-operator", func(w *contractWorld) {
			w.files[contractFile] = contractDoc(fmApplies, fmExcluded, fmInject, "language_context: [typescript]", "context_operator: prompt_contains")
			w.gate("a matching context", agentInput("sdd-apply", promptPlain), append(fileArgs, "--work-context-json", workContextMatching)...)
			w.gate("an excluded phase", agentInput("sdd-propose", promptWithEntry), append(fileArgs, "--work-context-json", workContextMatching)...)
		}},
		{"gate-task-context-lists-quotes-and-case", func(w *contractWorld) {
			w.files[contractFile] = contractDoc(fmApplies, fmExcluded, fmInject, `language_context: ["TypeScript", 'NestJS']`, "activation_context: [ Review , ]")
			w.gate("a matching context", agentInput("sdd-apply", promptPlain), append(fileArgs, "--work-context-json", workContextMatching)...)
			w.gate("another language", agentInput("sdd-apply", promptPlain), append(fileArgs, "--work-context-json", workContextOtherLang)...)
		}},
		{"gate-task-empty-context-lists", func(w *contractWorld) {
			w.files[contractFile] = contractDoc(fmApplies, fmExcluded, fmInject, "language_context: []", "activation_context: []")
			w.gate("no context given", agentInput("sdd-apply", promptPlain), fileArgs...)
		}},
		// ---- propagate -----------------------------------------------------------------
		{"propagate-embedded-contract", func(w *contractWorld) {
			w.files[registryFile] = goldenRegistry
			args := []string{"--registry", registryFile, "--embedded-contract", "anti-generic-design"}
			w.propagate("the block is inserted", args...)
			w.propagate("the same run again is silent", args...)
			w.propagate("an explicit path for the row", append(args, "--contract-path", "custom/anti-generic-design.md")...)
		}},
		{"propagate-file-contract", func(w *contractWorld) {
			w.withRegistry(contractDoc(fmApplies, fmExcluded, fmInject))
			args := []string{"--registry", registryFile, "--contract-file", contractFile, "--contract-path", contractEntry}
			w.propagate("the block is inserted", args...)
			w.propagate("the same run again is silent", args...)
			w.files[contractFile] = contractDoc("applies_to_phases: [sdd-design]", "excluded_phases: [sdd-apply]", fmInject)
			w.propagate("a changed contract updates the block", args...)
			w.files[contractFile] = contractDoc("applies_to_phases: [sdd-design]", fmInject)
			w.propagate("a contract with no excluded phases", args...)
		}},
		{"propagate-replaces-an-unscoped-row", func(w *contractWorld) {
			w.withRegistry(contractDoc(fmApplies, fmExcluded, fmInject))
			w.files[registryFile] = "# Skill Registry\n\n### Shared Contracts\n\n| Artifact | Path | Description |\n|---|---|---|\n| minimalism-contract | old/path.md | an old unscoped row |\n"
			w.propagate("the unscoped row is replaced", "--registry", registryFile, "--contract-file", contractFile, "--contract-path", contractEntry)
		}},
		{"propagate-refuses-what-it-cannot-do", func(w *contractWorld) {
			w.withRegistry(contractDoc(fmApplies, fmExcluded, fmInject))
			w.propagate("no registry flag", "--contract-file", contractFile)
			w.propagate("no contract source", "--registry", registryFile)
			w.propagate("a contract file that is missing", "--registry", registryFile, "--contract-file", "/virtual/missing.md")
			w.propagate("an unknown embedded contract", "--registry", registryFile, "--embedded-contract", "no-such-contract")
			for _, c := range []struct{ label, content string }{
				{"an empty contract", ""},
				{"a contract with no frontmatter", "no frontmatter here"},
				{"a contract with one delimiter", "---\napplies_to_phases: [sdd-apply]\n"},
				{"a contract with no applies_to_phases", contractDoc(fmExcluded, fmInject)},
				{"a contract with an empty applies_to_phases list", contractDoc("applies_to_phases: []")},
				{"a contract with an applies_to_phases that has no value", contractDoc("applies_to_phases:")},
			} {
				w.files[contractFile] = c.content
				w.propagate(c.label, "--registry", registryFile, "--contract-file", contractFile)
			}
		}},
		{"propagate-registry-states", func(w *contractWorld) {
			w.files[contractFile] = contractDoc(fmApplies, fmExcluded, fmInject)
			args := []string{"--registry", registryFile, "--contract-file", contractFile}
			w.propagate("no registry: a clean no-op", args...)
			w.propagate("no registry, one is required", append(args, "--require-registry")...)
			w.files[registryFile] = ""
			w.propagate("an empty registry", args...)
			w.files[registryFile] = " \n\t\n"
			w.propagate("a registry of white space", args...)
		}},
		// ---- propagate: the lists a lenient parser accepts and a strict one refuses ---
		{"propagate-phases-without-brackets-accepted-today", func(w *contractWorld) {
			w.withRegistry(contractDoc("applies_to_phases: sdd-tasks, sdd-apply", fmExcluded, fmInject))
			w.propagate("the row is built from the phases", "--registry", registryFile, "--contract-file", contractFile)
		}},
		{"propagate-phases-unclosed-bracket-accepted-today", func(w *contractWorld) {
			w.withRegistry(contractDoc("applies_to_phases: [sdd-tasks, sdd-apply", fmExcluded, fmInject))
			w.propagate("the row is built from the phases", "--registry", registryFile, "--contract-file", contractFile)
		}},
		{"propagate-phases-quoted-items-accepted-today", func(w *contractWorld) {
			w.withRegistry(contractDoc(`applies_to_phases: ["sdd-tasks", 'sdd-apply']`, `excluded_phases: ["sdd-propose"]`, fmInject))
			w.propagate("the row is built from the phases", "--registry", registryFile, "--contract-file", contractFile)
		}},
		{"propagate-excluded-without-brackets-accepted-today", func(w *contractWorld) {
			w.withRegistry(contractDoc(fmApplies, "excluded_phases: sdd-propose, sdd-spec", fmInject))
			w.propagate("the row is built from the phases", "--registry", registryFile, "--contract-file", contractFile)
		}},
		{"propagate-excluded-with-no-value-accepted-today", func(w *contractWorld) {
			w.withRegistry(contractDoc(fmApplies, "excluded_phases:", fmInject))
			w.propagate("the row is built from the phases", "--registry", registryFile, "--contract-file", contractFile)
		}},
		{"propagate-ignores-context-metadata-accepted-today", func(w *contractWorld) {
			args := []string{"--registry", registryFile, "--contract-file", contractFile}
			for _, c := range []struct{ label, line string }{
				{"a language_context that is not a list", "language_context: typescript"},
				{"an activation_context that is not a list", "activation_context: review"},
				{"a context_operator", "context_operator: prompt_contains"},
			} {
				w.withRegistry(contractDoc(fmApplies, fmExcluded, fmInject, c.line))
				w.propagate(c.label, args...)
			}
		}},
		// ---- the contract check of the status verb -----------------------------------
		{"status-contract-check", func(w *contractWorld) {
			w.checkContract("a contract that is missing")
			for _, c := range []struct{ label, content string }{
				{"a well-formed contract", contractDoc(fmApplies, fmExcluded, fmInject)},
				{"an empty file", ""},
				{"no frontmatter", "no frontmatter here"},
				{"no applies_to_phases", contractDoc(fmExcluded)},
				{"an empty applies_to_phases list", contractDoc("applies_to_phases: []")},
				{"phases without brackets (accepted today)", contractDoc("applies_to_phases: sdd-tasks, sdd-apply")},
				{"an unclosed list (accepted today)", contractDoc("applies_to_phases: [sdd-tasks")},
				{"an excluded list with no brackets (accepted today)", contractDoc(fmApplies, "excluded_phases: sdd-propose")},
				{"a malformed language_context (accepted today)", contractDoc(fmApplies, "language_context: typescript")},
				{"a context_operator (accepted today)", contractDoc(fmApplies, "context_operator: prompt_contains")},
			} {
				w.files[contractFile] = c.content
				w.checkContract(c.label)
			}
		}},
		// ---- runtime install --target opencode ---------------------------------------
		{"runtime-opencode-reads-the-contracts-of-the-overlay", func(w *contractWorld) {
			w.opencodeInstall(contractDoc(fmApplies, fmExcluded, fmInject), ooContract)
		}},
		{"runtime-opencode-without-the-oo-contract", func(w *contractWorld) {
			w.opencodeInstall(contractDoc(fmApplies, fmExcluded, fmInject), "")
		}},
		{"runtime-opencode-refuses-a-minimalism-contract-it-cannot-use", func(w *contractWorld) {
			w.opencodeInstall("no frontmatter here", "")
			w.opencodeInstall(contractDoc(fmExcluded, fmInject), "")
		}},
		{"runtime-opencode-refuses-an-oo-contract-it-cannot-use", func(w *contractWorld) {
			valid := contractDoc(fmApplies, fmExcluded, fmInject)
			w.opencodeInstall(valid, "no frontmatter here")
			w.opencodeInstall(valid, contractDoc(fmExcluded, fmInject, "language_context: [typescript]"))
			w.opencodeInstall(valid, contractDoc(fmApplies, fmInject, "language_context: typescript"))
			w.opencodeInstall(valid, contractDoc(fmApplies, fmInject, "activation_context: review"))
			w.opencodeInstall(valid, contractDoc(fmApplies, fmInject, "context_operator: prompt_contains"))
		}},
		{"runtime-opencode-context-lists-quotes-and-case", func(w *contractWorld) {
			valid := contractDoc(fmApplies, fmExcluded, fmInject)
			w.opencodeInstall(valid, contractDoc(fmApplies, fmInject, `language_context: ["TypeScript", 'NestJS']`, "activation_context: [ Review , ]"))
		}},
		{"runtime-opencode-minimalism-context-metadata-accepted-today", func(w *contractWorld) {
			w.opencodeInstall(contractDoc(fmApplies, fmExcluded, fmInject, "language_context: typescript"), "")
			w.opencodeInstall(contractDoc(fmApplies, fmExcluded, fmInject, "context_operator: prompt_contains"), "")
		}},
		{"runtime-opencode-phases-without-brackets-accepted-today", func(w *contractWorld) {
			w.opencodeInstall(contractDoc("applies_to_phases: sdd-tasks, sdd-apply", fmExcluded, fmInject), "")
		}},
		{"runtime-opencode-refuses-oo-phases-without-brackets", func(w *contractWorld) {
			valid := contractDoc(fmApplies, fmExcluded, fmInject)
			w.opencodeInstall(valid, contractDoc("applies_to_phases: sdd-design, sdd-apply", "excluded_phases: sdd-propose", fmInject, "language_context: [typescript]"))
		}},
		{"runtime-opencode-phases-quoted-items-accepted-today", func(w *contractWorld) {
			w.opencodeInstall(contractDoc(`applies_to_phases: ["sdd-tasks", 'sdd-apply']`, `excluded_phases: ["sdd-propose"]`, fmInject), "")
		}},
		{"runtime-opencode-excluded-with-no-value-accepted-today", func(w *contractWorld) {
			w.opencodeInstall(contractDoc(fmApplies, "excluded_phases:", fmInject), "")
		}},
	}
}

var contractGoldenFileName = regexp.MustCompile(`[^a-z0-9-]+`)

// TestContractGolden runs every case and compares its transcript with its golden file.
func TestContractGolden(t *testing.T) {
	for _, tc := range contractGoldenCases() {
		t.Run(tc.name, func(t *testing.T) {
			w := newContractWorld(t)
			tc.run(w)
			checkContractGolden(t, tc.name, w.text())
		})
	}
}

func checkContractGolden(t *testing.T, name, got string) {
	t.Helper()
	if contractGoldenFileName.MatchString(name) {
		t.Fatalf("case name %q is not a file name", name)
	}
	path := filepath.Join("testdata", "contract-golden", name+".golden")
	if *updateContractGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden file: %v (record it with -update-contract-golden)", err)
	}
	if diff := goldenDifference(name, got, string(want)); diff != "" {
		t.Fatal(diff)
	}
}
