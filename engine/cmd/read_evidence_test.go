package main

// Tests for 'projection hook --event PostToolUse' (C23 M7,
// docs/architecture/adr-c23-m7-read-evidence.md): a memory read that ran under a bound workflow is
// one line of the workflow's memory-read evidence, and a line that cannot be written is one
// warning for the agent.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func postToolUseInput(t *testing.T, cwd, tool string, toolInput, response any) string {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"session_id": "session-1", "transcript_path": "/tmp/transcript.jsonl", "cwd": cwd,
		"hook_event_name": "PostToolUse", "tool_name": tool, "tool_input": toolInput,
		"tool_response": response, "tool_use_id": "toolu_1",
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func (e hookEnv) evidenceFile(project, wf string) string {
	return filepath.Join(e.state, "labdrian", "workflows", project, wf+".memory.jsonl")
}

func TestPostToolUseRecordsAMemoryReadOfTheBoundWorkflow(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "odd")
	args := []string{"hook", "--event", "PostToolUse"}
	in := postToolUseInput(t, e.repo, "mcp__plugin_engram_engram__mem_get_observation", map[string]any{"id": 4216}, map[string]any{"text": "a record"})
	assertSilent(t, "a recorded read", runProjectionArgsWith(e.deps, args, in, e.dir))

	data, err := os.ReadFile(e.evidenceFile("proj-1", "wf-1"))
	if err != nil {
		t.Fatalf("no evidence: %v", err)
	}
	var got map[string]any
	if strings.Count(string(data), "\n") != 1 || json.Unmarshal(data, &got) != nil {
		t.Fatalf("evidence = %q, want one JSON line", data)
	}
	sum := sha256.Sum256([]byte(`{"text":"a record"}`))
	for key, want := range map[string]any{
		"version": 1.0, "tool": "mcp__plugin_engram_engram__mem_get_observation", "source": "engram",
		"call_id": "toolu_1", "session": "session-1", "response_sha256": hex.EncodeToString(sum[:]), "response_bytes": 19.0,
	} {
		if got[key] != want {
			t.Errorf("%s = %#v, want %#v", key, got[key], want)
		}
	}
	if ids, _ := got["ids"].([]any); len(ids) != 1 || ids[0] != "4216" {
		t.Errorf("ids = %#v, want [\"4216\"]", got["ids"])
	}
	if _, err := os.Stat(e.evidenceFile("proj-1", "wf-1")); err != nil {
		t.Error(err)
	}
}

func TestPostToolUseRecordsNothingWithoutAReadToRecord(t *testing.T) {
	e := newHookEnv(t)
	e.create(t, "proj-1", "wf-1", "odd")
	args := []string{"hook", "--event", "PostToolUse"}
	read := postToolUseInput(t, e.repo, "mcp__engram__mem_search", map[string]any{"project": "proj-1"}, "ok")
	assertSilent(t, "nothing bound", runProjectionArgsWith(e.deps, args, read, e.dir))

	mustBindOK(t, e.repo, "proj-1", "wf-1")
	assertSilent(t, "a write", runProjectionArgsWith(e.deps, args, postToolUseInput(t, e.repo, "mcp__engram__mem_save", map[string]any{"title": "x"}, "ok"), e.dir))
	assertSilent(t, "an edit", runProjectionArgsWith(e.deps, args, postToolUseInput(t, e.repo, "Edit", map[string]any{"file_path": "/x"}, "ok"), e.dir))
	assertSilent(t, "input that is not JSON", runProjectionArgsWith(e.deps, args, "not json", e.dir))
	assertSilent(t, "another event's input", runProjectionArgsWith(e.deps, args, sessionStartInput(t, e.repo, "startup"), e.dir))
	if _, err := os.Stat(e.evidenceFile("proj-1", "wf-1")); !os.IsNotExist(err) {
		t.Errorf("evidence was written: %v", err)
	}
}

func TestPostToolUseWarnsTheAgentWhenTheEvidenceCannotBeWritten(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "odd")
	if err := os.MkdirAll(e.evidenceFile("proj-1", "wf-1"), 0o700); err != nil {
		t.Fatal(err)
	}
	in := postToolUseInput(t, e.repo, "mcp__engram__mem_search", map[string]any{"project": "proj-1"}, "ok")
	r := runProjectionArgsWith(e.deps, []string{"hook", "--event", "PostToolUse"}, in, e.dir)
	var out struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if len(r.codes) != 1 || r.codes[0] != 0 || r.stderr != "" || json.Unmarshal([]byte(r.stdout), &out) != nil {
		t.Fatalf("exits %v, stdout %q, stderr %q", r.codes, r.stdout, r.stderr)
	}
	if out.HookSpecificOutput.HookEventName != "PostToolUse" ||
		!strings.Contains(out.HookSpecificOutput.AdditionalContext, "mcp__engram__mem_search) was not recorded for workflow wf-1 of project proj-1") {
		t.Errorf("reply = %+v", out)
	}
}
