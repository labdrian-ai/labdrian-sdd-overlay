package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Required fields from the pre/post-tool-use.command.input.schema.json schemas at
// https://raw.githubusercontent.com/openai/codex/rust-v0.148.0/codex-rs/hooks/schema/generated/.
// These are schema-shaped fixtures, not live-runtime coverage. transcript_path is nullable.
func codexSourceInput(t *testing.T, cwd, event, tool string, input any) string {
	t.Helper()
	fields := map[string]any{
		"session_id": "session-1", "turn_id": "turn-1", "agent_id": "subagent-1", "cwd": cwd,
		"hook_event_name": event, "model": "gpt-5.4", "permission_mode": "default",
		"tool_name": tool, "tool_use_id": "toolu_1", "transcript_path": nil, "tool_input": input,
	}
	if event == "PostToolUse" {
		fields["tool_response"] = map[string]any{"text": "response-content-canary"}
	}
	data, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func sourceHook(w phase7World, event, input string, flags ...string) phase7Run {
	args := append([]string{"projection", "hook", "--event", event}, flags...)
	return w.engine(w.dir, input, args...)
}

func sourceSidecar(w phase7World) string {
	return filepath.Join(w.state, "labdrian", "workflows", "proj-1", "wf-1.memory.jsonl")
}

func sourceRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// Compare complete serialized records, not just the adapted tool field. Only the
// wall-clock timestamp is taken from the record; processes intentionally have no clock seam.
func sourceRecord(t *testing.T, line []byte, tool, source, project string, ids string) {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(line, &fields); err != nil {
		t.Fatalf("record %q: %v", line, err)
	}
	sum := sha256.Sum256([]byte(`{"text":"response-content-canary"}`))
	idsJSON := "[]"
	if ids != "" {
		idsJSON = "[" + fmt.Sprintf("%q", ids) + "]"
	}
	want := fmt.Sprintf(`{"version":1,"at":%s,"tool":%q,"source":%q,"project":%q,"goal":"","ids":%s,"skill":"","call_id":"toolu_1","session":"session-1","response_sha256":%q,"response_bytes":34}`+"\n",
		fields["at"], tool, source, project, idsJSON, hex.EncodeToString(sum[:]))
	if string(line) != want {
		t.Errorf("sidecar bytes = %s; want %s", line, want)
	}
}

func TestCodexSourceProcesses(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs engine processes")
	}
	binary := phase6BuildEngineBinary(t)
	t.Run("PreToolUseAdaptsBeforeGate", func(t *testing.T) {
		w := newPhase7World(t, binary)
		w.bindRunning("proj-1", "wf-1", "sdd")
		before, tree := w.stateSnapshot(), snapshotTree(t, w.state)
		for _, name := range []string{"query", "get"} {
			input := map[string]any{"project": "proj-1", "query": "q", "engram_id": 7}
			claude := sourceHook(w, "PreToolUse", gateInput(t, w.repo, "session-1", "mcp__longterm-mem__"+name, input))
			assertDenied(t, "Claude reference", claude.asHook(), `does not include source "longterm-mem"`)
			got := sourceHook(w, "PreToolUse", codexSourceInput(t, w.repo, "PreToolUse", "mcp__longterm_mem__"+name, input), "--source", "codex")
			if got != claude {
				t.Errorf("%s: Codex %+v != Claude %+v", name, got, claude)
			}
		}
		assertSilent(t, "allowed Engram read", sourceHook(w, "PreToolUse", codexSourceInput(t, w.repo, "PreToolUse", "mcp__engram__mem_search", queryArgs("proj-1")), "--source", "codex").asHook())
		// The input cwd wins over the process cwd, and query arguments are never rewritten.
		assertDenied(t, "project unchanged", sourceHook(w, "PreToolUse", codexSourceInput(t, w.repo, "PreToolUse", "mcp__engram__mem_search", queryArgs("other")), "--source", "codex").asHook(), `project "other"`)
		if !reflect.DeepEqual(before, w.stateSnapshot()) || !reflect.DeepEqual(tree, snapshotTree(t, w.state)) {
			t.Error("gate changed state")
		}
	})
	t.Run("PostToolUseAdaptsBeforeEvidence", func(t *testing.T) {
		w := newPhase7World(t, binary)
		w.bindRunning("proj-1", "wf-1", "odd")
		log := phase6WorkflowLogPath(w.state, "proj-1", "wf-1")
		before := sourceRead(t, log)
		var accumulated []byte
		for _, tt := range []struct{ tool, adapted, source, project, id string }{
			{"mcp__engram__mem_get_observation", "mcp__engram__mem_get_observation", "engram", "", "7"},
			{"mcp__longterm_mem__query", "mcp__longterm-mem__query", "longterm-mem", "proj-1", ""},
			{"mcp__longterm_mem__get", "mcp__longterm-mem__get", "longterm-mem", "", "7"},
		} {
			in := codexSourceInput(t, w.repo, "PostToolUse", tt.tool, map[string]any{"project": "proj-1", "id": 7})
			assertSilent(t, tt.tool, sourceHook(w, "PostToolUse", in, "--source", "codex").asHook())
			data := sourceRead(t, sourceSidecar(w))
			if !bytes.HasPrefix(data, accumulated) || bytes.Count(data[len(accumulated):], []byte("\n")) != 1 {
				t.Fatalf("not exactly one append: %q -> %q", accumulated, data)
			}
			sourceRecord(t, data[len(accumulated):], tt.adapted, tt.source, tt.project, tt.id)
			accumulated = data
		}
		if !bytes.Equal(before, sourceRead(t, log)) {
			t.Error("evidence changed workflow log")
		}
	})
	t.Run("ExistingEventsKeepDefaultAndExplicitClaude", func(t *testing.T) {
		w := newPhase7World(t, binary)
		w.bindRunning("proj-1", "wf-1", "sdd")
		for event, in := range map[string]string{
			"PreToolUse":       gateInput(t, w.repo, "session-1", "mcp__longterm-mem__query", queryArgs("proj-1")),
			"PostToolUse":      postToolUseInput(t, w.repo, "mcp__engram__mem_search", queryArgs("proj-1"), map[string]any{"text": "response-content-canary"}),
			"SessionStart":     sessionStartInput(t, w.repo, "startup"),
			"UserPromptSubmit": hookInput(t, w.repo, "session-1", "p"),
		} {
			t.Run(event, func(t *testing.T) {
				before := w.stateSnapshot()
				want := sourceHook(w, event, in)
				got := sourceHook(w, event, in, "--source", "claude")
				if got != want || want.code != 0 || want.stderr != "" {
					t.Fatalf("default %+v; explicit Claude %+v", want, got)
				}
				if event == "PostToolUse" {
					assertSilent(t, event, want.asHook())
					lines := bytes.SplitAfter(sourceRead(t, sourceSidecar(w)), []byte("\n"))
					if len(lines) != 3 {
						t.Fatalf("want two records, got %q", lines)
					}
					for _, line := range lines[:2] {
						sourceRecord(t, line, "mcp__engram__mem_search", "engram", "proj-1", "")
					}
				} else {
					if want.stdout == "" || !reflect.DeepEqual(before, w.stateSnapshot()) {
						t.Fatalf("no output or state changed: %+v", want)
					}
					if event != "PreToolUse" && sourceHook(w, event, in, "--source", "codex") != want {
						t.Error("source changed a non-tool event")
					}
				}
			})
		}
		// Neither an omitted flag nor explicit Claude may adapt Codex spellings.
		for _, event := range []string{"PreToolUse", "PostToolUse"} {
			before := w.stateSnapshot()
			for _, flags := range [][]string{nil, {"--source", "claude"}} {
				assertSilent(t, "Claude underscore passthrough", sourceHook(w, event, codexSourceInput(t, w.repo, event, "mcp__longterm_mem__query", queryArgs("other")), flags...).asHook())
			}
			if !reflect.DeepEqual(before, w.stateSnapshot()) {
				t.Error("Claude underscore input changed state")
			}
		}
	})
	t.Run("BadSourceIsUsageErrorWithoutWrites", func(t *testing.T) {
		w := newPhase7World(t, binary)
		w.bindRunning("proj-1", "wf-1", "odd")
		before := w.stateSnapshot()
		for _, event := range []string{"PreToolUse", "PostToolUse", "SessionStart", "UserPromptSubmit"} {
			for _, source := range []string{"other", "", "Codex"} {
				r := sourceHook(w, event, codexSourceInput(t, w.repo, event, "mcp__engram__mem_search", queryArgs("proj-1")), "--source", source)
				want := fmt.Sprintf("error: projection hook: unsupported --source %q (supported: claude, codex)\n", source)
				if r.code != 1 || r.stdout != "" || r.stderr != want {
					t.Errorf("%s %q: %+v; want usage error %q", event, source, r, want)
				}
			}
		}
		if !reflect.DeepEqual(before, w.stateSnapshot()) {
			t.Error("bad source wrote state")
		}
	})
}

func TestCodexSourceUsageDoesNotReadStdin(t *testing.T) {
	for _, flags := range [][]string{{"--source", "other"}, {"--source"}} {
		stdin := &endlessReader{}
		var out, errBuf bytes.Buffer
		var codes []int
		runProjectionCore(testDeps(), append([]string{"hook", "--event", "PostToolUse"}, flags...), t.TempDir(), stdin, &out, &errBuf, func(code int) { codes = append(codes, code) })
		want := "error: projection hook: unsupported --source \"other\" (supported: claude, codex)\n"
		if len(flags) == 1 {
			want = "error: projection hook: --source requires a value (supported: claude, codex)\n"
		}
		if stdin.read != 0 || out.Len() != 0 || errBuf.String() != want || !reflect.DeepEqual(codes, []int{1}) {
			t.Errorf("read=%d stdout=%q stderr=%q codes=%v", stdin.read, out.String(), errBuf.String(), codes)
		}
	}
}

func TestCodexSourceHelp(t *testing.T) {
	readme := sourceRead(t, filepath.Join("..", "..", "README.md"))
	for name, text := range map[string]string{"usage": captureUsage(t), "README": string(readme)} {
		if !strings.Contains(text, "[--source claude|codex]") || !strings.Contains(text, "default: claude") {
			t.Errorf("%s does not document source flag/default", name)
		}
	}
}
