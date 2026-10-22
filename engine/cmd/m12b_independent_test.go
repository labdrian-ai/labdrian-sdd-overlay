package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// ADR decisions 1/2/3: use installed commands, including shell quoting, with an
// independently authored envelope and a goal-narrowed workflow that includes longterm-mem.
func TestM12bIndependentInstalledGoalAndMetadata(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs isolated engine processes")
	}
	binary := phase6BuildEngineBinary(t)
	quoted := filepath.Join(t.TempDir(), "engine path's")
	data, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(quoted, data, 0700); err != nil {
		t.Fatal(err)
	}
	w := newPhase7World(t, quoted)
	goal := writeMemoryTestFile(t, w.dir, "goal.json", memoryTestGoalV3JSON("proj-1", "goal-1", memoryTestDirectiveJSON("goal", "engram", "longterm-mem")))
	w.verb("proj-1", "wf-1", "create", "--goal", goal, "--profile", "odd")
	w.engine(w.repo, "", "workflow", "bind", "--project", "proj-1", "--workflow", "wf-1").must(t, "bind")
	hooks := filepath.Join(t.TempDir(), "hooks.json")
	w.engine(w.dir, "", "merge-codex-hooks", "--hooks", hooks, "--hook-command", quoted).must(t, "install")
	bytes, err := os.ReadFile(hooks)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Hooks map[string][]struct {
			Matcher string
			Hooks   []struct{ Command string }
		}
	}
	if err := json.Unmarshal(bytes, &doc); err != nil {
		t.Fatal(err)
	}
	envelope := func(event, tool string, args any) string {
		fields := map[string]any{"cwd": w.repo, "hook_event_name": event, "tool_name": tool, "tool_input": args, "tool_use_id": "call-1", "session_id": "session-1", "agent_id": "ignored-agent"}
		if event == "PostToolUse" {
			fields["tool_response"] = map[string]any{"text": "PRIVATE-response-canary"}
		}
		out, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}
	installed := func(event, tool string, args any) phase7Run {
		groups := doc.Hooks[event]
		if len(groups) != 1 || len(groups[0].Hooks) != 1 {
			t.Fatalf("%s groups: %+v", event, groups)
		}
		if !regexp.MustCompile(groups[0].Matcher).MatchString(tool) {
			t.Fatalf("matcher misses %s", tool)
		}
		return w.run(w.dir, envelope(event, tool, args), "sh", "-c", groups[0].Hooks[0].Command)
	}
	// Seed the very same sidecar through Claude first.
	seed := envelope("PostToolUse", "mcp__engram__mem_search", map[string]any{"project": "proj-1"})
	assertSilent(t, "Claude seed", w.engine(w.dir, seed, "projection", "hook", "--event", "PostToolUse").asHook())
	sidecar := filepath.Join(w.state, "labdrian", "workflows", "proj-1", "wf-1.memory.jsonl")
	prefix, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name string
		args map[string]any
		deny bool
	}{
		{"allowed", map[string]any{"project": "proj-1", "goal": "goal-1"}, false},
		{"other project", map[string]any{"project": "other", "goal": "goal-1"}, true},
		{"no goal", map[string]any{"project": "proj-1"}, true},
		{"empty goal", map[string]any{"project": "proj-1", "goal": ""}, true},
		{"nonstring goal", map[string]any{"project": "proj-1", "goal": 7}, true},
		{"other goal", map[string]any{"project": "proj-1", "goal": "other"}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			before := w.stateSnapshot()
			got := installed("PreToolUse", "mcp__longterm_mem__query", tt.args)
			reference := w.engine(w.dir, envelope("PreToolUse", "mcp__longterm-mem__query", tt.args), "projection", "hook", "--event", "PreToolUse")
			if got != reference {
				t.Fatalf("Codex=%+v Claude=%+v", got, reference)
			}
			if tt.deny {
				assertDenied(t, tt.name, got.asHook(), "permits only")
			} else {
				assertSilent(t, tt.name, got.asHook())
			}
			if !reflect.DeepEqual(before, w.stateSnapshot()) {
				t.Fatal("gate changed state/evidence")
			}
		})
	}
	args := map[string]any{"project": "proj-1", "goal": "goal-1"}
	assertSilent(t, "Codex evidence", installed("PostToolUse", "mcp__longterm_mem__query", args).asHook())
	after, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(after), string(prefix)) {
		t.Fatal("lost Claude prefix")
	}
	var record struct {
		Tool, Source, Project, Goal, CallID, Session, ResponseSHA256 string
		ResponseBytes                                                int64
	}
	var fields map[string]json.RawMessage
	line := after[len(prefix):]
	if err := json.Unmarshal(line, &fields); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(line, &record); err != nil {
		t.Fatal(err)
	}
	// Field names with underscores are deliberately read independently of Go field naming.
	var hash, call string
	var size int
	json.Unmarshal(fields["response_sha256"], &hash)
	json.Unmarshal(fields["response_bytes"], &size)
	json.Unmarshal(fields["call_id"], &call)
	response := []byte(`{"text":"PRIVATE-response-canary"}`)
	sum := sha256.Sum256(response)
	if hash != hex.EncodeToString(sum[:]) || size != len(response) || call != "call-1" || record.Session != "session-1" || record.Tool != "mcp__longterm-mem__query" || record.Source != "longterm-mem" || record.Project != "proj-1" || record.Goal != "goal-1" || strings.Contains(string(after), "PRIVATE-response-canary") {
		t.Fatalf("metadata: %s", line)
	}
	info, err := os.Stat(sidecar)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("sidecar mode %v %v", info, err)
	}
}
