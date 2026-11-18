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
	if want := "roles skill-paths --skills-root " + skillsRootFixture; calls[1].Args != want {
		t.Errorf("the verb should run as %q, got %q", want, calls[1].Args)
	}
	if calls[1].Raw != listedPrompt {
		t.Errorf("the verb should be handed the prompt as the agent wrote it, as plain text, got %q", calls[1].Raw)
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
	old := newFakeEngine(t, nil)
	old.failEventWith(t, skillsEvent, 1, "error: roles: unknown verb \"skill-paths\" (expected validate, next, resume, append, or match-shaper)\n")

	for name, run := range map[string]func() []byte{
		"missing": func() []byte {
			return runPluginWithEngine(t, pluginPath, filepath.Join(t.TempDir(), "no-engine"), launchOf("subagent", "builder", listedPrompt))
		},
		"timeout": func() []byte {
			return runPluginWithEnv(t, pluginPath, []string{engineEnvironment + "=" + hung.path, "LABDRIAN_ENGINE_TIMEOUT_MS=400"}, launchOf("subagent", "builder", listedPrompt))
		},
		"empty answer": func() []byte {
			return runPluginWithEngine(t, pluginPath, silent.path, launchOf("subagent", "builder", listedPrompt))
		},
		"engine too old": func() []byte {
			return runPluginWithEngine(t, pluginPath, old.path, launchOf("subagent", "builder", listedPrompt))
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := decodeLaunch(t, run())
			if got.Thrown == nil || got.Thrown.Name != "Tool.Error" || !strings.HasPrefix(got.Thrown.Message, "this launch was denied: its skill list could not be checked: ") {
				t.Fatalf("a launch whose skill list the engine cannot check must be denied with a sentence naming it, got %#v", got.Thrown)
			}
			if got.Input["prompt"] != listedPrompt {
				t.Fatalf("a denied launch keeps its prompt, got %#v", got.Input)
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
