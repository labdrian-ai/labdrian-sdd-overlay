package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexHookVerbsAndInstalledSessionStart(t *testing.T) {
	binary := phase6BuildEngineBinary(t)
	w := newPhase7World(t, binary)
	home := filepath.Join(w.home, "codex")
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "hooks.json")
	original := `{"future" : 1e+09,"hooks":{"SessionStart":[{ "hooks": [{"type":"command","command":"gentle-ai skill-registry refresh"}]},{"hooks":[{"type":"command","command":"moshi notify"}]}]}}`
	writeFixtureFile(t, path, original)
	// A binary path with both shell metacharacters exercises the existing quote helper.
	installed := filepath.Join(t.TempDir(), "engine's directory", "engine")
	if err := os.MkdirAll(filepath.Dir(installed), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(binary, installed); err != nil {
		t.Fatal(err)
	}
	invoke := func(verb string) phase7Run {
		return w.engine(w.dir, "", verb, "--hooks", path, "--hook-command", installed)
	}
	assert := func(r phase7Run, code int, out, err string) {
		t.Helper()
		if r.code != code || r.stdout != out || r.stderr != err {
			t.Fatalf("got %+v want code=%d stdout=%q stderr=%q", r, code, out, err)
		}
	}
	assert(invoke("merge-codex-hooks"), 0, "merge-codex-hooks: hooks installed successfully\n", "")
	first := mustReadFile(t, path)
	backup := mustReadFile(t, path+".bak")
	if backup != original {
		t.Fatal("backup differs")
	}
	stat, _ := os.Stat(path)
	assert(invoke("merge-codex-hooks"), 0, "merge-codex-hooks: hooks installed successfully\n", "")
	stat2, _ := os.Stat(path)
	if first != mustReadFile(t, path) || backup != mustReadFile(t, path+".bak") || !stat.ModTime().Equal(stat2.ModTime()) {
		t.Fatal("second merge rewrote file or backup")
	}
	doc := phase7ReadSettings(t, path)
	groups := doc["hooks"].(map[string]any)["SessionStart"].([]any)
	if len(groups) != 3 {
		t.Fatalf("groups: %v", groups)
	}
	command := groups[2].(map[string]any)["hooks"].([]any)[0].(map[string]any)["command"].(string)
	// All required fields from rust-v0.148.0/session-start.command.input.schema.json.
	input := func(cwd, source string) string {
		data, _ := json.Marshal(map[string]any{"cwd": cwd, "hook_event_name": "SessionStart", "model": "synthetic-model", "permission_mode": "default", "session_id": "synthetic-session", "source": source, "transcript_path": nil})
		return string(data)
	}
	assert(w.run(w.dir, input(w.repo, "startup"), "/bin/sh", "-c", command), 0, "", "")
	w.bindRunning("proj-1", "wf-1", "odd", "authorize")
	// Availability is real but deterministic: this world's PATH contains no source binaries and its repo has no lock.
	block := strings.Replace(oddInstructions, "\n- Engram reads", "\n- Do not read engram: it is unavailable in this session (the engram program was not found).\n- Do not read longterm-mem: it is unavailable in this session (the longterm-mem program was not found).\n- Do not read procedural-skills: it is unavailable in this session (no procedural skills lock: none installed).\n- Engram reads", 1)
	expected, _ := json.Marshal(struct {
		Output struct {
			Event   string `json:"hookEventName"`
			Context string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}{Output: struct {
		Event   string `json:"hookEventName"`
		Context string `json:"additionalContext"`
	}{"SessionStart", block}})
	for _, source := range []string{"startup", "resume", "clear", "compact"} {
		assert(w.run(w.dir, input(w.repo, source), "/bin/sh", "-c", command), 0, string(expected)+"\n", "")
	}
	assert(invoke("uninstall-codex-hooks"), 0, "uninstall-codex-hooks: hooks removed successfully\n", "")
	after := mustReadFile(t, path)
	if strings.Contains(after, "projection hook") || !strings.Contains(after, "gentle-ai skill-registry refresh") || !strings.Contains(after, "moshi notify") {
		t.Fatal(after)
	}
	assert(invoke("uninstall-codex-hooks"), 0, "uninstall-codex-hooks: hooks removed successfully\n", "")
	if after != mustReadFile(t, path) {
		t.Fatal("second uninstall changed bytes")
	}
	writeFixtureFile(t, path, `{"hooks":[]}`)
	beforeBackup := mustReadFile(t, path+".bak")
	for _, verb := range []string{"merge-codex-hooks", "uninstall-codex-hooks"} {
		assert(invoke(verb), 2, "", "error: "+verb+": "+path+": hooks must be an object (not modified)\n")
		if mustReadFile(t, path) != `{"hooks":[]}` || beforeBackup != mustReadFile(t, path+".bak") {
			t.Fatal("malformed file changed")
		}
	}
	writeFixtureFile(t, path, "")
	for _, verb := range []string{"merge-codex-hooks", "uninstall-codex-hooks"} {
		assert(invoke(verb), 2, "", "error: "+verb+": "+path+": empty JSON document (not modified)\n")
		if mustReadFile(t, path) != "" || beforeBackup != mustReadFile(t, path+".bak") {
			t.Fatal("empty malformed file or backup changed")
		}
	}
	missing := filepath.Join(home, "new.json")
	assert(w.engine(w.dir, "", "merge-codex-hooks", "--hooks", missing, "--hook-command", installed), 0, "merge-codex-hooks: hooks installed successfully\n", "")
	if _, err := os.Stat(missing + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("new file backup: %v", err)
	}
	for _, verb := range []string{"merge-codex-hooks", "uninstall-codex-hooks"} {
		assert(w.engine(w.dir, "", verb), 1, "", "error: "+verb+": --hooks and --hook-command are required\n")
		assert(w.engine(w.dir, "", verb, "--unknown"), 1, "", "error: "+verb+": unknown argument \"--unknown\"\n")
	}
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestStatusCodexPresenceAndAbsence(t *testing.T) {
	home := t.TempDir()
	d := testDeps()
	d.getenv = environmentOf(map[string]string{"HOME": home, "CODEX_HOME": filepath.Join(home, "custom-codex")})
	d.getwd = func() (string, error) { return home, nil }
	check := func() string {
		p := newCapturedProcess()
		run(p.process, d, []string{"status"})
		if p.err.String() != "" {
			t.Fatal(p.err.String())
		}
		return p.out.String()
	}
	if out := check(); strings.Contains(out, "hook: Codex") {
		t.Fatal(out)
	}
	path := filepath.Join(home, "custom-codex", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if out := check(); !strings.Contains(out, "[WARN] hook: Codex memory (SessionStart, PreToolUse, PostToolUse)") {
		t.Fatalf("CODEX_HOME=%q: %s", d.env("CODEX_HOME"), out)
	}
	p := newCapturedProcess()
	run(p.process, d, []string{"merge-codex-hooks", "--hooks", path, "--hook-command", filepath.Join(home, ".claude/bin/gentle-ai-overlay")})
	if out := check(); !strings.Contains(out, "[OK  ] hook: Codex memory (SessionStart, PreToolUse, PostToolUse)") || !strings.Contains(out, "trust") || !strings.Contains(out, "/hooks") || !strings.Contains(out, "key t") {
		t.Fatal(out)
	}
}
