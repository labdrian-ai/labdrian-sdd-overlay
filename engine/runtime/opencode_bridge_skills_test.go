package runtime_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/hookwire"
	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

// T43 commit b (S8 L117 b): on a `subagent` launch the bridge hands the prompt to `roles skill-paths
// --skills-root <root>` and launches with what it prints, after the gate and before the frozen context
// (so the diff appended to a reviewer brief is never read as a skill list). The verb's refusals and any
// failure of it deny the launch. A config with no skills_root (an older install) leaves the prompt as it is.

const (
	skillsRootFixture = "/overlay/skills"
	// skillsEvent is the event the stand-in files `roles skill-paths` under.
	skillsEvent  = "SkillPaths"
	listedPrompt = "Build T1.\n\n## Skills to load before work\ntest-first\n"
	pathsPrompt  = "Build T1.\n\n## Skills to load before work\n/overlay/skills/test-first/SKILL.md\n"
)

// skillsFixture installs the plugin with a config that names the skills root, and with the contract
// entries of included (the phases the minimalism contract is injected for).
func skillsFixture(t *testing.T, included ...string) string {
	t.Helper()
	root, pluginPath := writePluginFixture(t)
	config := minimalismConfig(pluginPath, root)
	config["skills_root"] = skillsRootFixture
	if included != nil {
		config["prompt_config"].(map[string]any)["included_phases"] = included
	}
	writeJSONFile(t, filepath.Join(root, "labdrian-runtime-parity.json"), config)
	return pluginPath
}

func TestOpenCodeBridgeLaunchesARoleWithTheSkillPathsTheVerbPrinted(t *testing.T) {
	requireNode(t)
	engine := newFakeEngine(t, map[string][]byte{skillsEvent: []byte(pathsPrompt)})
	pluginPath := skillsFixture(t)

	got := decodeLaunch(t, runPluginWithEngine(t, pluginPath, engine.path, launchOf("subagent", "builder", listedPrompt)))
	if got.Thrown != nil || got.Input["prompt"] != pathsPrompt || got.Input["agent"] != "builder" {
		t.Fatalf("the launch should carry the verb's stdout as its prompt, got %#v", got)
	}
	calls := engine.calls(t)
	if len(calls) != 2 || calls[0].Event != eventPreToolUse || calls[1].Event != skillsEvent {
		t.Fatalf("the gate should be asked first, then the verb, got %#v", calls)
	}
	if want := "roles skill-paths --skills-root " + skillsRootFixture + " --agent builder"; calls[1].Args != want {
		t.Errorf("the verb should run as %q, got %q", want, calls[1].Args)
	}
	if calls[1].Raw != listedPrompt {
		t.Errorf("the verb should be handed the prompt as the agent wrote it, as plain text, got %q", calls[1].Raw)
	}
}

// T60 correction B (S8 L173): the plugin hands the verb the launch's agent, which decides whether a prose
// skill is added; a launch with no agent field hands it none.
func TestOpenCodeBridgeHandsTheVerbTheAgentOfALaunchAndNoneWhenThereIsNone(t *testing.T) {
	requireNode(t)
	pluginPath := skillsFixture(t)

	engine := newFakeEngine(t, map[string][]byte{skillsEvent: []byte(reviewerBrief), frozenEvent: []byte(frozenReply)})
	got := decodeLaunch(t, runPluginWithEngine(t, pluginPath, engine.path, launchOf("subagent", "reviewer", reviewerBrief)))
	if got.Thrown != nil {
		t.Fatalf("the launch should not be denied, got %#v", got.Thrown)
	}
	calls := engine.calls(t)
	if want := "roles skill-paths --skills-root " + skillsRootFixture + " --agent reviewer"; len(calls) < 2 || calls[1].Args != want {
		t.Errorf("a reviewer launch should run the verb as %q, got %#v", want, calls)
	}

	noAgent := newFakeEngine(t, map[string][]byte{skillsEvent: []byte(pathsPrompt)})
	call := `const call = { tool: "subagent", sessionID: "ses_1", id: "call_1", input: { prompt: ` + `"x" } };
let thrown = null;
try { await beforeHook(call); } catch (error) { thrown = { name: error.name, message: error.message, fields: error.fields }; }
console.log(JSON.stringify({ thrown, input: call.input }));`
	if got := decodeLaunch(t, runPluginWithEngine(t, pluginPath, noAgent.path, call)); got.Thrown != nil {
		t.Fatalf("a launch with no agent should not be denied, got %#v", got.Thrown)
	}
	calls = noAgent.calls(t)
	if want := "roles skill-paths --skills-root " + skillsRootFixture; len(calls) < 2 || calls[1].Args != want {
		t.Errorf("a launch with no agent should run the verb as %q, got %#v", want, calls)
	}
}

func TestOpenCodeBridgeDeniesALaunchTheVerbRefusesWithItsLine(t *testing.T) {
	requireNode(t)
	const line = `error: roles skill-paths: line 3 "## Skills to load before work: test-first, go-test" is not the header alone: write "## Skills to load before work" alone on its line, then one skill id or absolute SKILL.md path per line`
	for name, prompt := range map[string]string{
		"the colon form": "Build.\n\n## Skills to load before work: test-first, go-test\n",
		"an unknown id":  listedPrompt,
	} {
		t.Run(name, func(t *testing.T) {
			engine := newFakeEngine(t, map[string][]byte{skillsEvent: []byte(pathsPrompt)})
			engine.failEventWith(t, skillsEvent, 2, line+"\n")
			got := decodeLaunch(t, runPluginWithEngine(t, skillsFixture(t), engine.path, launchOf("subagent", "builder", prompt)))
			if got.Thrown == nil || got.Thrown.Name != "Tool.Error" || got.Thrown.Message != line || got.Thrown.Fields["message"] != line {
				t.Fatalf("the verb's refusal should throw a Tool.Error({message: %q}), got %#v", line, got.Thrown)
			}
			if got.Input["prompt"] != prompt {
				t.Fatalf("a refused launch keeps the prompt the agent wrote, got %#v", got.Input)
			}
		})
	}
}

func TestOpenCodeBridgeDeniesALaunchWhenTheEngineCannotCheckItsSkillList(t *testing.T) {
	requireNode(t)
	pluginPath := skillsFixture(t)
	hung := newFakeEngine(t, map[string][]byte{skillsEvent: []byte(pathsPrompt)})
	hung.hangEventFor(t, skillsEvent, 2)
	silent := newFakeEngine(t, nil)

	const unchecked = "this launch was denied: its skill list could not be checked: "
	for name, tc := range map[string]struct {
		run  func() []byte
		want string // the whole message, or "" when only the prefix is checked (Node words the spawn error)
	}{
		"missing": {run: func() []byte {
			return runPluginWithEngine(t, pluginPath, filepath.Join(t.TempDir(), "no-engine"), launchOf("subagent", "builder", listedPrompt))
		}},
		"timeout": {run: func() []byte {
			return runPluginWithEnv(t, pluginPath, []string{engineEnvironment + "=" + hung.path, "LABDRIAN_ENGINE_TIMEOUT_MS=400"}, launchOf("subagent", "builder", listedPrompt))
		}, want: unchecked + "the engine did not answer within 0.4 seconds"},
		"empty answer": {run: func() []byte {
			return runPluginWithEngine(t, pluginPath, silent.path, launchOf("subagent", "builder", listedPrompt))
		}, want: unchecked + "the engine printed nothing"},
	} {
		t.Run(name, func(t *testing.T) {
			got := decodeLaunch(t, tc.run())
			if got.Thrown == nil || got.Thrown.Name != "Tool.Error" || !strings.HasPrefix(got.Thrown.Message, unchecked) {
				t.Fatalf("a launch whose skill list the engine cannot check must be denied with a sentence naming it, got %#v", got.Thrown)
			}
			if got.Input["prompt"] != listedPrompt {
				t.Fatalf("a denied launch keeps its prompt, got %#v", got.Input)
			}
			// T60 correction D (S8 L177, -5 F2): a failure with no exit code names no exit and no remedy.
			for _, banned := range []string{olderEngineRemedy, "exited 1"} {
				if strings.Contains(got.Thrown.Message, banned) {
					t.Errorf("a failure with no exit code must not hold %q, got %q", banned, got.Thrown.Message)
				}
			}
			if tc.want != "" && got.Thrown.Message != tc.want {
				t.Errorf("the denial should be exactly %q, got %q", tc.want, got.Thrown.Message)
			}
		})
	}
}

// olderEngineRemedy is the command the denial of an exit 1 offers for an engine older than the plugin.
const olderEngineRemedy = "labdrian apply --target opencode"

// exitOneDenial is the exact sentence a launch is denied with when the verb exits 1 (S8 L177): what the engine
// reported (binary, exit, line), the remedy for an older engine offered on a condition, the cause never claimed.
func exitOneDenial(enginePath, line string) string {
	return "this launch was denied: its skill list could not be checked: " + line + "; the engine " + enginePath +
		" exited 1 on this plugin's call: if that engine is older than the plugin, run `" + olderEngineRemedy +
		"`, or rebuild the engine " + engineEnvironment + " names"
}

// T60 correction D (S8 L177, replacing correction C's test of L175): the verb exits 1 for a usage error
// (an engine older than the plugin does not know `--agent`) and for any other failure of the verb, so the
// denial states the exit and offers the remedy on a condition; it never claims an older engine as the cause.
func TestOpenCodeBridgeDeniesAnExitOneWithTheRemedyForAnOlderEngineAndNoClaimOfIt(t *testing.T) {
	requireNode(t)
	for name, line := range map[string]string{
		"an unknown flag":                       `error: roles skill-paths: unknown flag "--agent"`,
		"a skills root that is not a directory": "error: roles skill-paths: --skills-root /overlay/skills is not a directory",
		"an unread prompt":                      "error: roles skill-paths: read the prompt: input/output error",
		"an unwritten stdout":                   "error: roles skill-paths: write to stdout: broken pipe",
		"an unknown verb":                       `error: roles: unknown verb "skill-paths" (expected validate, next, resume, append, or match-shaper)`,
		"no line from the engine":               "the engine ended with exit 1",
	} {
		t.Run(name, func(t *testing.T) {
			engine := newFakeEngine(t, map[string][]byte{skillsEvent: []byte(pathsPrompt)})
			message := ""
			if !strings.HasPrefix(line, "the engine ended with") {
				message = line + "\n"
			}
			engine.failEventWith(t, skillsEvent, 1, message)
			got := decodeLaunch(t, runPluginWithEngine(t, skillsFixture(t), engine.path, launchOf("subagent", "builder", listedPrompt)))
			if want := exitOneDenial(engine.path, line); got.Thrown == nil || got.Thrown.Name != "Tool.Error" || got.Thrown.Message != want {
				t.Fatalf("an exit 1 should be denied with exactly %q, got %#v", want, got.Thrown)
			}
			for _, banned := range []string{"does not accept this plugin's call", "as an engine older than the plugin does"} {
				if strings.Contains(got.Thrown.Message, banned) {
					t.Errorf("the denial must not claim the cause, it holds %q: %q", banned, got.Thrown.Message)
				}
			}
			if got.Input["prompt"] != listedPrompt {
				t.Errorf("a denied launch keeps its prompt, got %#v", got.Input)
			}
		})
	}
}

// An exit that is not the verb's refusal (exit 2 without the verb's prefix, or another number) keeps today's
// sentence: the line as the engine said it, no exit named, no remedy (S8 L175; L177, -5 F2).
func TestOpenCodeBridgeKeepsTheSentenceForAnExitTwoThatIsNotTheVerbsLine(t *testing.T) {
	requireNode(t)
	for name, tc := range map[string]struct {
		code int
		line string
	}{
		"exit 2 that is not the verb's line": {2, "error: something else entirely"},
		"exit 3 with the verb's prefix":      {3, "error: roles skill-paths: something"},
		"exit 3 with another line":           {3, "error: something else entirely"},
	} {
		t.Run(name, func(t *testing.T) {
			engine := newFakeEngine(t, map[string][]byte{skillsEvent: []byte(pathsPrompt)})
			engine.failEventWith(t, skillsEvent, tc.code, tc.line+"\n")
			got := decodeLaunch(t, runPluginWithEngine(t, skillsFixture(t), engine.path, launchOf("subagent", "builder", listedPrompt)))
			if want := "this launch was denied: its skill list could not be checked: " + tc.line; got.Thrown == nil || got.Thrown.Message != want {
				t.Fatalf("exit %d should be denied with %q, got %#v", tc.code, want, got.Thrown)
			}
		})
	}
}

func TestOpenCodeBridgeLeavesTheFrozenContextOfAReviewerAsTheVerbPrintedIt(t *testing.T) {
	requireNode(t)
	// The diff holds the header and a variant of it, as a diff of this very file would.
	frozen := reviewerBrief + "\n## Frozen context (base aaaa, head bbbb)\nchanged paths: a.go\n```diff\n+## Skills to load before work\n+## Skills to load before work: test-first\n```\n"
	engine := newFakeEngine(t, map[string][]byte{skillsEvent: []byte(reviewerBrief), frozenEvent: []byte(frozen)})

	got := decodeLaunch(t, runPluginWithEngine(t, skillsFixture(t), engine.path, launchOf("subagent", "reviewer", reviewerBrief)))
	if got.Thrown != nil || got.Input["prompt"] != frozen {
		t.Fatalf("the reviewer should reach its launch exactly as review frozen-context printed it, got %#v", got)
	}
	calls := engine.calls(t)
	if len(calls) != 3 || calls[0].Event != eventPreToolUse || calls[1].Event != skillsEvent || calls[2].Event != frozenEvent {
		t.Fatalf("the gate, the skill list and the frozen context should be asked in that order, got %#v", calls)
	}
	if calls[1].Raw != reviewerBrief || calls[2].Raw != reviewerBrief {
		t.Fatalf("both verbs should be handed the brief alone, the diff being made after the skill list is checked, got %q and %q", calls[1].Raw, calls[2].Raw)
	}
}

func TestOpenCodeBridgeLeavesALaunchAloneWhenTheConfigNamesNoSkillsRoot(t *testing.T) {
	requireNode(t)
	for name, prompt := range map[string]string{"a list": listedPrompt, "the colon form": "Build.\n## Skills to load before work: test-first\n"} {
		t.Run(name, func(t *testing.T) {
			engine := newFakeEngine(t, map[string][]byte{skillsEvent: []byte(pathsPrompt)})
			got := decodeLaunch(t, runPluginWithEngine(t, bridgeFixture(t), engine.path, launchOf("subagent", "builder", prompt)))
			if got.Thrown != nil || got.Input["prompt"] != prompt || len(got.Input) != 2 {
				t.Fatalf("an older install leaves the prompt byte-identical, got %#v", got)
			}
			if calls := engine.calls(t); len(calls) != 1 || calls[0].Event != eventPreToolUse {
				t.Fatalf("only the gate should be asked, got %#v", calls)
			}
		})
	}
}

func TestOpenCodeBridgeDoesNotCheckTheSkillListOfADeniedLaunchOrOfAnotherTool(t *testing.T) {
	requireNode(t)
	pluginPath := skillsFixture(t)

	denying := newFakeEngine(t, map[string][]byte{eventPreToolUse: encoded(t, hookwire.PreToolUseReply{Deny: true, Reason: "paused"}), skillsEvent: []byte(pathsPrompt)})
	got := decodeLaunch(t, runPluginWithEngine(t, pluginPath, denying.path, launchOf("subagent", "builder", listedPrompt)))
	if got.Thrown == nil || got.Thrown.Message != "paused" || got.Input["prompt"] != listedPrompt {
		t.Fatalf("a launch the gate denies is not rewritten, got %#v", got)
	}
	if calls := denying.calls(t); len(calls) != 1 {
		t.Fatalf("a denied launch should not run the verb, got %#v", calls)
	}

	other := newFakeEngine(t, map[string][]byte{skillsEvent: []byte(pathsPrompt)})
	got = decodeLaunch(t, runPluginWithEngine(t, pluginPath, other.path, launchOf("shell", "builder", listedPrompt)))
	if got.Thrown != nil || got.Input["prompt"] != listedPrompt {
		t.Fatalf("another tool is left as it was, got %#v", got)
	}
	if calls := other.calls(t); len(calls) != 1 {
		t.Fatalf("only the gate should be asked, got %#v", calls)
	}
}

func TestOpenCodeBridgeInjectsTheContractAfterTheSkillList(t *testing.T) {
	requireNode(t)
	const contract = "skills/_shared/minimalism-contract.md"
	engine := newFakeEngine(t, map[string][]byte{skillsEvent: []byte(pathsPrompt)})

	got := decodeLaunch(t, runPluginWithEngine(t, skillsFixture(t, "builder"), engine.path, launchOf("subagent", "builder", listedPrompt)))
	prompt, _ := got.Input["prompt"].(string)
	if got.Thrown != nil || !strings.HasPrefix(prompt, pathsPrompt[:strings.Index(pathsPrompt, "\n/overlay")]) || !strings.Contains(prompt, "/overlay/skills/test-first/SKILL.md") || !strings.Contains(prompt, contract) {
		t.Fatalf("the contract should be added to the prompt the verb printed, got %#v", got)
	}
	if calls := engine.calls(t); len(calls) != 2 || calls[1].Raw != listedPrompt {
		t.Fatalf("the verb should be handed the prompt before the contract is added, got %#v", calls)
	}
}

func TestOpenCodeInstallWritesTheSkillsRootOfTheOverlayItReads(t *testing.T) {
	root := t.TempDir()
	overlay := shippedOverlay(t)
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{OverlayDir: overlay})
	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}

	data, err := os.ReadFile(filepath.Join(root, "labdrian-runtime-parity.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		SkillsRoot string `json:"skills_root"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(overlay, "skills"); config.SkillsRoot != want {
		t.Fatalf("skills_root = %q, want the absolute skills directory of the overlay, %q", config.SkillsRoot, want)
	}
}
