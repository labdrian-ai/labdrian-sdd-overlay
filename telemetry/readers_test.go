package telemetry

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/iotest"
)

func fixture(t *testing.T, root, name, data string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

const stamp = "2026-10-20T13:57:50.903Z"

func claude(id string, output int) string {
	b, _ := json.Marshal(map[string]any{"type": "assistant", "timestamp": stamp, "sessionId": "session", "uuid": "fallback", "message": map[string]any{"id": id, "model": "claude-sonnet-5-5", "usage": map[string]int{"input_tokens": 3, "cache_read_input_tokens": 4, "cache_creation_input_tokens": 5, "output_tokens": output}}})
	return string(b) + "\n"
}

func pi(id, cost string) string {
	if cost != "" {
		cost = `,"cost":` + cost
	}
	return `{"type":"message","id":"fallback","timestamp":"` + stamp + `","message":{"role":"assistant","provider":"openai-codex","model":"gpt-6-luna","responseId":"` + id + `","usage":{"input":3,"output":6,"cacheRead":4,"cacheWrite":5,"reasoning":2,"totalTokens":20` + cost + `}}}` + "\n"
}

type reader func(string, func(Call)) (Stats, error)

func collect(t *testing.T, read reader, root string) ([]Call, Stats) {
	t.Helper()
	var calls []Call
	s, err := read(root, func(c Call) { calls = append(calls, c) })
	if err != nil {
		t.Fatal(err)
	}
	return calls, s
}

func TestClaudeDedupeLast(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "project/session.jsonl", claude("msg", 1)+claude("msg", 6))
	calls, s := collect(t, ReadClaude, root)
	want := Call{Source: "claude", Project: "project", SessionID: "session", Provider: "anthropic", Model: "claude-sonnet-5-5", Role: "main", Day: "2026-10-20", Input: 3, CacheRead: 4, CacheWrite: 5, Output: 6}
	if len(calls) != 1 || !reflect.DeepEqual(calls[0], want) || s != (Stats{Files: 1, Read: 1, Lines: 2, Calls: 1, Duplicates: 1}) {
		t.Fatalf("calls=%+v stats=%+v", calls, s)
	}
}

func TestClaudeUUIDFallback(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "p/session.jsonl", claude("", 1)+claude("", 7))
	calls, s := collect(t, ReadClaude, root)
	if len(calls) != 1 || calls[0].Output != 7 || s.Duplicates != 1 {
		t.Fatalf("%+v %+v", calls, s)
	}
}

func TestClaudeSyntheticSkipped(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "p/session.jsonl", claude("real", 1)+strings.ReplaceAll(claude("fake", 2), "claude-sonnet-5-5", "<synthetic>"))
	calls, s := collect(t, ReadClaude, root)
	if len(calls) != 1 || calls[0].Output != 1 || s.Skipped != 1 {
		t.Fatalf("%+v %+v", calls, s)
	}
}

func TestClaudeSidechain(t *testing.T) {
	for _, path := range []string{"p/session/subagents/agent-a.jsonl", "p/session.jsonl"} {
		t.Run(path, func(t *testing.T) {
			root := t.TempDir()
			line := claude("m", 1)
			if path == "p/session.jsonl" {
				line = strings.Replace(line, `"type":"assistant"`, `"isSidechain":true,"type":"assistant"`, 1)
			}
			fixture(t, root, path, line)
			calls, _ := collect(t, ReadClaude, root)
			if len(calls) != 1 || calls[0].Role != "sub" {
				t.Fatalf("%+v", calls)
			}
		})
	}
}

func TestClaudeAgentType(t *testing.T) {
	for _, meta := range []string{`{"agentType":"Explore","description":"private"}`, "", `{bad`, `{"agentType":"Explore"} garbage`} {
		t.Run(meta, func(t *testing.T) {
			root := t.TempDir()
			fixture(t, root, "p/s/subagents/agent-a.jsonl", claude("m", 1))
			if meta != "" {
				fixture(t, root, "p/s/subagents/agent-a.meta.json", meta)
			}
			calls, s := collect(t, ReadClaude, root)
			want := ""
			if meta == `{"agentType":"Explore","description":"private"}` {
				want = "Explore"
			}
			if len(calls) != 1 || calls[0].AgentType != want || s.Files != 1 || s.UnreadableFiles != 0 {
				t.Fatalf("%+v %+v", calls, s)
			}
		})
	}
}

func TestProjectFromDirectory(t *testing.T) {
	for _, read := range []reader{ReadClaude, ReadPi} {
		root := t.TempDir()
		fixture(t, root, "project-name/ts_session.jsonl", claude("m", 1)+pi("r", ""))
		calls, _ := collect(t, read, root)
		if len(calls) != 1 || calls[0].Project != "project-name" || calls[0].SessionID != "session" {
			t.Fatalf("%+v", calls)
		}
	}
}

func TestMalformedLineNotFatal(t *testing.T) {
	for _, read := range []reader{ReadClaude, ReadPi} {
		root := t.TempDir()
		fixture(t, root, "p/ts_session.jsonl", "{bad \"usage\":\n"+claude("m", 1)+pi("r", ""))
		calls, s := collect(t, read, root)
		if len(calls) != 1 || s.Lines != 3 || s.Skipped != 2 || s.UnreadableFiles != 0 {
			t.Fatalf("%+v %+v", calls, s)
		}
	}
}

func TestLargeLine(t *testing.T) {
	for _, tc := range []struct {
		read reader
		line string
	}{{ReadClaude, claude("m", 1)}, {ReadPi, pi("r", "")}} {
		root := t.TempDir()
		line := strings.Replace(tc.line, `{`, `{"private":"`+strings.Repeat("x", 2<<20)+`",`, 1)
		fixture(t, root, "p/ts_session.jsonl", strings.TrimSuffix(line, "\n"))
		calls, s := collect(t, tc.read, root)
		if len(calls) != 1 || s.Lines != 1 {
			t.Fatalf("%+v %+v", calls, s)
		}
	}
}

func TestMissingRoot(t *testing.T) {
	for _, read := range []reader{ReadClaude, ReadPi} {
		s, err := read(filepath.Join(t.TempDir(), "missing"), func(Call) { t.Fatal("unexpected call") })
		if err == nil || s != (Stats{}) {
			t.Fatalf("stats=%+v err=%v", s, err)
		}
	}
}

func TestPiCost(t *testing.T) {
	for _, cost := range []string{"0.25", `{"total":0.25}`, ""} {
		t.Run(cost, func(t *testing.T) {
			root := t.TempDir()
			fixture(t, root, "p/ts_session.jsonl", pi("r", cost))
			calls, s := collect(t, ReadPi, root)
			want := Call{Source: "pi", Project: "p", SessionID: "session", Provider: "openai-codex", Model: "gpt-6-luna", Role: "main", Day: "2026-10-20", Input: 3, CacheRead: 4, CacheWrite: 5, Output: 6, Reasoning: 2}
			if cost != "" {
				v := 0.25
				want.CostUSD = &v
			}
			if len(calls) != 1 || !reflect.DeepEqual(calls[0], want) || s != (Stats{Files: 1, Read: 1, Lines: 1, Calls: 1}) {
				t.Fatalf("%+v %+v", calls, s)
			}
		})
	}
}

func TestPiDedupeFirst(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "p/a_session.jsonl", pi("response", "0.25")+pi("response", "0.5"))
	fixture(t, root, "q/b_session.jsonl", pi("response", "0.75"))
	calls, s := collect(t, ReadPi, root)
	if len(calls) != 1 || calls[0].CostUSD == nil || *calls[0].CostUSD != 0.25 || s.Duplicates != 2 {
		t.Fatalf("%+v %+v", calls, s)
	}
}

// Lines without a response ID fall back to their line ID, which is only
// unique within one session file.
func TestPiLineIDScopedToFile(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "p/a_session.jsonl", pi("", "0.25")+pi("", "0.5"))
	fixture(t, root, "q/b_session.jsonl", pi("", "0.75"))
	calls, s := collect(t, ReadPi, root)
	if len(calls) != 2 || *calls[0].CostUSD != 0.25 || *calls[1].CostUSD != 0.75 || s.Duplicates != 1 {
		t.Fatalf("%+v %+v", calls, s)
	}
}

// A file that fails mid-read contributes no calls, and its IDs do not hide
// copies in later files.
func TestPartialFileDropped(t *testing.T) {
	for _, tc := range []struct {
		source, line string
	}{{"claude", claude("m", 1)}, {"pi", pi("r", "")}} {
		var s Stats
		r := io.MultiReader(strings.NewReader(tc.line), iotest.ErrReader(errors.New("disk")))
		rows, ok := readFile(r, tc.source, func(rows map[string]entry, e entry) bool { rows[e.id] = e; return false }, &s)
		if ok || rows != nil || s.Lines != 1 {
			t.Fatalf("%s: %v %+v %+v", tc.source, ok, rows, s)
		}
	}
}

func TestUnreadableDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads any directory")
	}
	for _, read := range []reader{ReadClaude, ReadPi} {
		root := t.TempDir()
		fixture(t, root, "p/ts_session.jsonl", `{"type":"user"}`+"\n")
		locked := filepath.Join(root, "q")
		if err := os.Mkdir(locked, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(locked, 0700) })
		calls, s := collect(t, read, root)
		if len(calls) != 0 || s != (Stats{Files: 1, Read: 1, Lines: 1, Skipped: 1, UnreadableFiles: 1}) {
			t.Fatalf("%+v %+v", calls, s)
		}
	}
}

func TestNonAssistantIgnored(t *testing.T) {
	for _, tc := range []struct {
		read reader
		line string
	}{{ReadClaude, claude("m", 1)}, {ReadPi, pi("r", "")}} {
		root := t.TempDir()
		other := strings.ReplaceAll(tc.line, "assistant", "user")
		fixture(t, root, "p/ts_session.jsonl", other+`{"type":"message","message":{"role":"assistant"}}`+"\n"+tc.line)
		calls, s := collect(t, tc.read, root)
		if len(calls) != 1 || s.Skipped != 2 {
			t.Fatalf("%+v %+v", calls, s)
		}
	}
}

// A line that would be a call but has no usable ID cannot be deduplicated: it
// is dropped and counted as NoID, never as a duplicate of another such line,
// while lines that are not calls stay Skipped.
func TestNoIDCounted(t *testing.T) {
	for _, tc := range []struct {
		name       string
		read       reader
		call, noID string
	}{
		{"claude", ReadClaude, claude("m", 1), strings.Replace(claude("", 1), `,"uuid":"fallback"`, "", 1)},
		{"pi", ReadPi, pi("r", ""), strings.Replace(pi("", ""), `"id":"fallback",`, "", 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			fixture(t, root, "p/ts_session.jsonl", tc.noID+tc.noID+`{"type":"user"}`+"\n"+tc.call)
			calls, s := collect(t, tc.read, root)
			if len(calls) != 1 || s != (Stats{Files: 1, Read: 1, Lines: 4, Calls: 1, Skipped: 1, NoID: 2}) {
				t.Fatalf("%+v %+v", calls, s)
			}
		})
	}
}

func TestUnreadableFile(t *testing.T) {
	for _, read := range []reader{ReadClaude, ReadPi} {
		root := t.TempDir()
		if err := os.Symlink(filepath.Join(root, "absent"), filepath.Join(root, "broken.jsonl")); err != nil {
			t.Fatal(err)
		}
		calls, s := collect(t, read, root)
		if len(calls) != 0 || s != (Stats{Files: 1, UnreadableFiles: 1}) {
			t.Fatalf("%+v %+v", calls, s)
		}
	}
}

func TestTimestampTieBreak(t *testing.T) {
	for _, tc := range []struct {
		read reader
		a, b string
	}{{ReadClaude, claude("a", 1), claude("b", 2)}, {ReadPi, pi("a", "1"), pi("b", "2")}} {
		root := t.TempDir()
		fixture(t, root, "p/ts_s.jsonl", tc.b+tc.a)
		calls, _ := collect(t, tc.read, root)
		if len(calls) != 2 || (calls[0].Source == "claude" && calls[0].Output != 1) || (calls[0].Source == "pi" && *calls[0].CostUSD != 1) {
			t.Fatalf("ID tie-break: %+v", calls)
		}
	}
}

func TestDeterminism(t *testing.T) {
	for _, tc := range []struct {
		read reader
		a, b string
	}{{ReadClaude, claude("a", 1), claude("b", 2)}, {ReadPi, pi("a", "1"), pi("b", "2")}} {
		root := t.TempDir()
		fixture(t, root, "z/ts_s.jsonl", strings.ReplaceAll(tc.a, stamp, "invalid"))
		fixture(t, root, "a/ts_s.jsonl", tc.b+strings.ReplaceAll(tc.a, stamp, "2026-10-21T00:00:00Z"))
		first, s := collect(t, tc.read, root)
		second, s2 := collect(t, tc.read, root)
		want := 3
		if first != nil && first[0].Source == "pi" {
			want = 2
		}
		if len(first) != want || !reflect.DeepEqual(first, second) || s != s2 {
			t.Fatalf("%+v / %+v", first, second)
		}
		if first[0].Day != "2026-10-20" || first[1].Day != "2026-10-21" {
			t.Fatalf("order: %+v", first)
		}
		if want == 3 && first[2].Day != "" {
			t.Fatal("invalid day must be empty")
		}
	}
}

func TestReadOnly(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "p/ts_s.jsonl", claude("m", 1)+pi("r", ""))
	fixture(t, root, "p/s/subagents/agent-a.meta.json", `{"agentType":"Explore"}`)
	type snapshot struct {
		Hash  [32]byte
		Size  int64
		Mode  os.FileMode
		Mtime string
	}
	snap := func() map[string]snapshot {
		result := map[string]snapshot{}
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			result[path] = snapshot{sha256.Sum256(data), info.Size(), info.Mode(), info.ModTime().String()}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	before := snap()
	for _, read := range []reader{ReadClaude, ReadPi} {
		calls, _ := collect(t, read, root)
		if len(calls) != 1 {
			t.Fatalf("%+v", calls)
		}
	}
	if !reflect.DeepEqual(before, snap()) {
		t.Fatal("reader modified files")
	}
}
