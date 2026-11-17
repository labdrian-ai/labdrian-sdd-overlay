package main

// The session events of the OpenCode hook bridge (S10, T11 commit b) against the real engine: each
// answer the plugin hands OpenCode is the one the engine gives Claude Code for the same event.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// hookAnswer is the part of a hook's answer the bridge reads.
type hookAnswer struct {
	SystemMessage string `json:"systemMessage"`
	Specific      struct {
		AdditionalContext string `json:"additionalContext"`
	} `json:"hookSpecificOutput"`
}

func decodeHookAnswer(t *testing.T, stdout string) hookAnswer {
	t.Helper()
	var answer hookAnswer
	if err := json.Unmarshal([]byte(stdout), &answer); err != nil {
		t.Fatalf("the reference answer %q is not a hook answer: %v", stdout, err)
	}
	return answer
}

// pushedTexts decodes what the plugin pushed onto the system prompt: the texts of its text parts.
func pushedTexts(t *testing.T, out []byte) []string {
	t.Helper()
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(out, &parts); err != nil {
		t.Fatalf("decode what the plugin pushed: %v\nraw=%q", err, out)
	}
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part.Type != "text" {
			t.Errorf("the plugin pushed a part of type %q, want text", part.Type)
		}
		texts = append(texts, part.Text)
	}
	return texts
}

func TestOpenCodeBridgeSessionEventsAgainstTheEngine(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs engine processes")
	}
	binary := phase6BuildEngineBinary(t)
	pluginPath := bridgeConfigRoot(t)

	t.Run("SessionStartPutsTheBoundWorkflowsMemoryBlockIntoTheSystemPrompt", func(t *testing.T) {
		w := newPhase7World(t, binary)
		w.bindRunning("proj-1", "wf-1", "odd")
		reference := w.engine(w.dir, `{"hook_event_name":"SessionStart","session_id":"reference","cwd":`+jsonString(t, w.repo)+`}`, "projection", "hook", "--event", "SessionStart")
		want := decodeHookAnswer(t, reference.stdout).Specific.AdditionalContext
		if want == "" {
			t.Fatalf("the reference SessionStart of a bound workflow should carry its memory instructions, got %q", reference.stdout)
		}
		out := bridgeRun(t, w, pluginPath, `const system = [];
await hooks.get("context")({ sessionID: "session-1", system });
console.log(JSON.stringify(system));`)
		if got := pushedTexts(t, out); len(got) != 1 || got[0] != want {
			t.Errorf("the system prompt should get the block the engine gives Claude Code,\n got %q\nwant %q", got, want)
		}
	})

	t.Run("UserPromptSubmitTellsTheSessionTheWorkflowIsPaused", func(t *testing.T) {
		w := newPhase7World(t, binary)
		w.bindRunning("proj-1", "wf-1", "sdd")
		w.verb("proj-1", "wf-1", "pause")
		reference := decodeHookAnswer(t, w.promptHook(w.repo).stdout)
		var want []string
		for _, line := range []string{reference.SystemMessage, reference.Specific.AdditionalContext} {
			if line != "" {
				want = append(want, line)
			}
		}
		if len(want) == 0 {
			t.Fatal("the reference prompt hook of a paused workflow should tell the session so")
		}
		out := bridgeRun(t, w, pluginPath, `const prompt = { sessionID: "session-1", prompt: { text: "please continue with the task" } };
await hooks.get("prompt")(prompt);
const system = [];
await hooks.get("context")({ sessionID: "session-1", system });
console.log(JSON.stringify({ text: prompt.prompt.text, system }));`)
		var got struct {
			Text   string `json:"text"`
			System []any  `json:"system"`
		}
		if err := json.Unmarshal(out, &got); err != nil {
			t.Fatalf("decode result: %v\nraw=%q", err, out)
		}
		if wantText := "please continue with the task\n\n" + strings.Join(want, "\n"); got.Text != wantText {
			t.Errorf("the model should read what the engine tells Claude Code at a prompt, after the user's text,\n got %q\nwant %q", got.Text, wantText)
		}
		for _, part := range got.System {
			for _, line := range want {
				if strings.Contains(fmt.Sprint(part), line) {
					t.Errorf("the answer to a prompt must not enter the system prompt, found %q in it", line)
				}
			}
		}
	})

	t.Run("StopForcesAnotherTurnWithTheReasonTheEngineGivesClaudeCode", func(t *testing.T) {
		s := newStopWorld(t)
		env := append(append([]string{}, s.env...), "HOME="+t.TempDir(), "XDG_STATE_HOME="+s.home, "PATH="+os.Getenv("PATH"))
		reference := exec.Command(binary, "review", "stop-hook")
		reference.Dir = s.root
		reference.Env = env
		reference.Stdin = strings.NewReader(stopInput("reference-session", s.root, false))
		blocked, err := reference.Output()
		if err != nil {
			t.Fatalf("run the reference stop hook: %v", err)
		}
		var want struct {
			Decision string `json:"decision"`
			Reason   string `json:"reason"`
		}
		if err := json.Unmarshal(blocked, &want); err != nil || want.Decision != "block" || want.Reason == "" {
			t.Fatalf("the reference stop hook should block a branch with a commit no review covers, got %q (%v)", blocked, err)
		}

		out := bridgeRunIn(t, binary, s.root, env, pluginPath, `const idle = hooks.get("experimental.idle");
await idle({ sessionID: "opencode-session", background: [] });
await idle({ sessionID: "opencode-session", background: [] });
console.log(JSON.stringify(synthetic));`)
		var sent []struct {
			SessionID string `json:"sessionID"`
			Text      string `json:"text"`
		}
		if err := json.Unmarshal(out, &sent); err != nil {
			t.Fatalf("decode result: %v\nraw=%q", err, out)
		}
		// The engine asks once for each session and commit: the second stop of the session, with the
		// stop hook already active, is let through.
		if len(sent) != 1 || sent[0].SessionID != "opencode-session" || sent[0].Text != want.Reason {
			t.Errorf("the first stop should be blocked with the engine's reason and the second let through, got %#v", sent)
		}
		if _, err := os.Stat(filepath.Join(s.home, "labdrian")); err != nil {
			t.Errorf("the engine should have remembered its question in the state home: %v", err)
		}
	})
}
