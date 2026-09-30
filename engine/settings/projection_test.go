package settings_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
)

// projectionEntry is one owned projection hook entry read back from settings.
type projectionEntry struct {
	event   string
	matcher string
	command string
	raw     map[string]interface{}
}

// projectionEntries returns every entry carrying the projection identity and
// the test binary, in file order, across all event keys.
func projectionEntries(t *testing.T, root map[string]interface{}) []projectionEntry {
	t.Helper()
	var out []projectionEntry
	hooks, _ := root["hooks"].(map[string]interface{})
	for _, key := range []string{"UserPromptSubmit", "PreToolUse", "SessionEnd", "Stop"} {
		entries, _ := hooks[key].([]interface{})
		for _, e := range entries {
			em, _ := e.(map[string]interface{})
			if em == nil || !entryContainsBinarySubstring(e, testHookCommand) || !entryContainsBinarySubstring(e, settings.LabdrianProjectionIdentity) {
				continue
			}
			matcher, _ := em["matcher"].(string)
			inner, _ := em["hooks"].([]interface{})
			var command string
			if len(inner) == 1 {
				ih, _ := inner[0].(map[string]interface{})
				command, _ = ih["command"].(string)
			}
			out = append(out, projectionEntry{event: key, matcher: matcher, command: command, raw: em})
		}
	}
	return out
}

func mustJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func installFresh(t *testing.T) (string, *settings.Merger) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.json")
	m := buildMerger(t, path)
	if err := m.Install(); err != nil {
		t.Fatalf("Install: %v", err)
	}
	return path, m
}

func TestInstall_AddsTheProjectionFamily(t *testing.T) {
	path, _ := installFresh(t)
	got := projectionEntries(t, parseJSON(t, path))
	if len(got) != 3 {
		t.Fatalf("projection entries = %d, want 3: %+v", len(got), got)
	}
	want := []struct{ event, matcher, hookEvent string }{
		{"UserPromptSubmit", "", "UserPromptSubmit"},
		{"PreToolUse", settings.ProjectionEditToolMatcher, "PreToolUse"},
		{"PreToolUse", settings.ProjectionMemoryQueryMatcher, "PreToolUse"},
	}
	for i, w := range want {
		e := got[i]
		if e.event != w.event || e.matcher != w.matcher {
			t.Errorf("entry %d = %s/%q, want %s/%q", i, e.event, e.matcher, w.event, w.matcher)
		}
		wantTail := testHookCommand + " projection hook --event " + w.hookEvent
		if !strings.Contains(e.command, wantTail) {
			t.Errorf("entry %d command %q does not run %q", i, e.command, wantTail)
		}
		if _, has := e.raw["matcher"]; has != (w.matcher != "") {
			t.Errorf("entry %d matcher key presence = %v, want %v", i, has, w.matcher != "")
		}
	}
}

func TestProjectionMatchersAreTheDocumentedOnes(t *testing.T) {
	if settings.ProjectionEditToolMatcher != "Write|Edit|MultiEdit|NotebookEdit" {
		t.Errorf("ProjectionEditToolMatcher = %q", settings.ProjectionEditToolMatcher)
	}
	if settings.ProjectionMemoryQueryMatcher != `^mcp__([A-Za-z0-9-]+_)*longterm-mem__query$` {
		t.Errorf("ProjectionMemoryQueryMatcher = %q", settings.ProjectionMemoryQueryMatcher)
	}
}

func TestProjectionCommandNeverBlocksAndSkipsAMissingBinary(t *testing.T) {
	path, _ := installFresh(t)
	for _, e := range projectionEntries(t, parseJSON(t, path)) {
		// Missing binary: command -v fails, so the || true arm makes the hook a
		// no-op. The projection hooks decide by JSON output, never by exit code
		// 2, so a masked exit status cannot hide a denial.
		wantPrefix := "command -v " + testHookCommand + " >/dev/null 2>&1 && " + testHookCommand + " projection hook --event "
		if !strings.HasPrefix(e.command, wantPrefix) || !strings.HasSuffix(e.command, " || true") {
			t.Errorf("command %q is not the guarded never-blocking form", e.command)
		}
	}
}

func TestInstall_ProjectionFamilyIsIdempotent(t *testing.T) {
	path, m := installFresh(t)
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Install(); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(path)
	if string(first) != string(second) {
		t.Errorf("second Install changed the file")
	}
	if n := len(projectionEntries(t, parseJSON(t, path))); n != 3 {
		t.Errorf("projection entries after two installs = %d, want 3", n)
	}
}

// foreignSettings holds entries that share our event keys, our binary path, and
// even our matchers, but carry no projection identity.
func foreignSettings() (string, []interface{}, []interface{}) {
	ups := []interface{}{
		map[string]interface{}{"hooks": []interface{}{map[string]interface{}{"type": "command", "command": "/opt/other/ups --flag"}}},
		map[string]interface{}{"hooks": []interface{}{map[string]interface{}{"type": "command", "command": testHookCommand + " something-else"}}},
	}
	pre := []interface{}{
		map[string]interface{}{"matcher": settings.ProjectionEditToolMatcher, "hooks": []interface{}{map[string]interface{}{"type": "command", "command": "/opt/other/edit-guard", "timeout": float64(5)}}},
		map[string]interface{}{"matcher": settings.ProjectionMemoryQueryMatcher, "hooks": []interface{}{map[string]interface{}{"type": "command", "command": "/opt/other/mem-guard"}}},
		map[string]interface{}{"matcher": "Bash", "hooks": []interface{}{map[string]interface{}{"type": "command", "command": testHookCommand + " other verb"}}},
	}
	doc := map[string]interface{}{
		"model": "opus",
		"hooks": map[string]interface{}{"UserPromptSubmit": ups, "PreToolUse": pre, "Notification": []interface{}{map[string]interface{}{"hooks": []interface{}{}}}},
	}
	b, _ := json.Marshal(doc)
	return string(b), ups, pre
}

func TestProjectionFamilyPreservesForeignEntriesByteForByte(t *testing.T) {
	doc, ups, pre := foreignSettings()
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	m := buildMerger(t, path)

	check := func(stage string) {
		t.Helper()
		root := parseJSON(t, path)
		hooks := root["hooks"].(map[string]interface{})
		for _, c := range []struct {
			key  string
			want []interface{}
		}{{"UserPromptSubmit", ups}, {"PreToolUse", pre}} {
			have := map[string]bool{}
			for _, e := range hooks[c.key].([]interface{}) {
				have[mustJSON(t, e)] = true
			}
			for _, f := range c.want {
				if !have[mustJSON(t, f)] {
					t.Errorf("%s: foreign %s entry lost or altered: %s", stage, c.key, mustJSON(t, f))
				}
			}
		}
		if root["model"] != "opus" || hooks["Notification"] == nil {
			t.Errorf("%s: foreign top-level keys lost: %v", stage, root)
		}
	}

	if err := m.Install(); err != nil {
		t.Fatal(err)
	}
	check("after Install")
	if n := len(projectionEntries(t, parseJSON(t, path))); n != 3 {
		t.Errorf("projection entries = %d, want 3 next to same-matcher foreign entries", n)
	}
	if err := m.Uninstall(); err != nil {
		t.Fatal(err)
	}
	check("after Uninstall")
	if n := len(projectionEntries(t, parseJSON(t, path))); n != 0 {
		t.Errorf("projection entries survive Uninstall: %d", n)
	}
}

func TestUninstall_RemovesOnlyTheProjectionFamilyWhenTheRestIsGone(t *testing.T) {
	path, m := installFresh(t)
	root := parseJSON(t, path)
	hooks := root["hooks"].(map[string]interface{})
	// Keep only the projection entries plus one foreign entry per key.
	for _, key := range []string{"UserPromptSubmit", "PreToolUse"} {
		kept := []interface{}{map[string]interface{}{"hooks": []interface{}{map[string]interface{}{"type": "command", "command": "/x/foreign-" + key}}}}
		for _, e := range hooks[key].([]interface{}) {
			if entryContainsBinarySubstring(e, settings.LabdrianProjectionIdentity) {
				kept = append(kept, e)
			}
		}
		hooks[key] = kept
	}
	b, _ := json.Marshal(root)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := m.Uninstall(); err != nil {
		t.Fatal(err)
	}
	after := parseJSON(t, path)
	if n := len(projectionEntries(t, after)); n != 0 {
		t.Errorf("projection entries left: %d", n)
	}
	for _, key := range []string{"UserPromptSubmit", "PreToolUse"} {
		entries := after["hooks"].(map[string]interface{})[key].([]interface{})
		if len(entries) != 1 || !strings.Contains(mustJSON(t, entries[0]), "/x/foreign-"+key) {
			t.Errorf("%s = %s, want only the foreign entry", key, mustJSON(t, entries))
		}
	}
}

func TestHasProjectionHooks_ReportsMissingAndDriftedParts(t *testing.T) {
	path, _ := installFresh(t)
	full := parseJSON(t, path)
	if !settings.HasProjectionHooks(full, testHookCommand) {
		t.Fatalf("HasProjectionHooks = false after Install; missing %v", settings.MissingProjectionHookParts(full, testHookCommand))
	}
	if parts := settings.MissingProjectionHookParts(full, testHookCommand); len(parts) != 0 {
		t.Fatalf("missing parts after Install = %v", parts)
	}
	if !settings.HasSupportedClaudeLifecycleState(full, testHookCommand) {
		t.Fatalf("lifecycle state unsupported after a full Install")
	}

	mutate := func(fn func(hooks map[string]interface{})) map[string]interface{} {
		root := parseJSON(t, path)
		fn(root["hooks"].(map[string]interface{}))
		return root
	}
	dropWhere := func(hooks map[string]interface{}, key string, drop func(map[string]interface{}) bool) {
		var kept []interface{}
		for _, e := range hooks[key].([]interface{}) {
			em := e.(map[string]interface{})
			if entryContainsBinarySubstring(e, settings.LabdrianProjectionIdentity) && drop(em) {
				continue
			}
			kept = append(kept, e)
		}
		hooks[key] = kept
	}
	cases := []struct {
		name string
		root map[string]interface{}
		want string
	}{
		{"user prompt entry removed", mutate(func(h map[string]interface{}) {
			dropWhere(h, "UserPromptSubmit", func(map[string]interface{}) bool { return true })
		}), "UserPromptSubmit"},
		{"edit matcher removed", mutate(func(h map[string]interface{}) {
			dropWhere(h, "PreToolUse", func(e map[string]interface{}) bool { return e["matcher"] == settings.ProjectionEditToolMatcher })
		}), settings.ProjectionEditToolMatcher},
		{"memory matcher removed", mutate(func(h map[string]interface{}) {
			dropWhere(h, "PreToolUse", func(e map[string]interface{}) bool { return e["matcher"] == settings.ProjectionMemoryQueryMatcher })
		}), settings.ProjectionMemoryQueryMatcher},
		{"memory matcher narrowed", mutate(func(h map[string]interface{}) {
			for _, e := range h["PreToolUse"].([]interface{}) {
				em := e.(map[string]interface{})
				if em["matcher"] == settings.ProjectionMemoryQueryMatcher {
					em["matcher"] = "mcp__longterm-mem__query"
				}
			}
		}), "drifted"},
		{"command edited", mutate(func(h map[string]interface{}) {
			for _, e := range h["UserPromptSubmit"].([]interface{}) {
				if entryContainsBinarySubstring(e, settings.LabdrianProjectionIdentity) {
					ih := e.(map[string]interface{})["hooks"].([]interface{})[0].(map[string]interface{})
					ih["command"] = ih["command"].(string) + " --extra"
				}
			}
		}), "drifted"},
	}
	for _, c := range cases {
		if settings.HasProjectionHooks(c.root, testHookCommand) {
			t.Errorf("%s: HasProjectionHooks = true", c.name)
		}
		if settings.HasSupportedClaudeLifecycleState(c.root, testHookCommand) {
			t.Errorf("%s: lifecycle state reported supported", c.name)
		}
		joined := strings.Join(settings.MissingProjectionHookParts(c.root, testHookCommand), "|")
		if !strings.Contains(joined, c.want) {
			t.Errorf("%s: missing parts %q do not name %q", c.name, joined, c.want)
		}
	}
	if settings.HasProjectionHooks(map[string]interface{}{}, testHookCommand) {
		t.Errorf("empty settings reported as having the projection hooks")
	}
}

func TestInstall_RepairsADriftedProjectionFamilyWithoutDuplicates(t *testing.T) {
	path, m := installFresh(t)
	root := parseJSON(t, path)
	hooks := root["hooks"].(map[string]interface{})
	for _, e := range hooks["PreToolUse"].([]interface{}) {
		em := e.(map[string]interface{})
		if em["matcher"] == settings.ProjectionMemoryQueryMatcher {
			em["matcher"] = "mcp__longterm-mem__query"
		}
	}
	// A duplicated user-prompt entry is drift too.
	for _, e := range hooks["UserPromptSubmit"].([]interface{}) {
		if entryContainsBinarySubstring(e, settings.LabdrianProjectionIdentity) {
			hooks["UserPromptSubmit"] = append(hooks["UserPromptSubmit"].([]interface{}), e)
			break
		}
	}
	b, _ := json.Marshal(root)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := m.Install(); err != nil {
		t.Fatal(err)
	}
	after := parseJSON(t, path)
	if !settings.HasProjectionHooks(after, testHookCommand) {
		t.Fatalf("still drifted: %v", settings.MissingProjectionHookParts(after, testHookCommand))
	}
	if n := len(projectionEntries(t, after)); n != 3 {
		t.Errorf("projection entries after repair = %d, want 3", n)
	}
}

func TestInstall_AddsTheProjectionFamilyToAnOlderInstall(t *testing.T) {
	path, m := installFresh(t)
	root := parseJSON(t, path)
	hooks := root["hooks"].(map[string]interface{})
	for _, key := range []string{"UserPromptSubmit", "PreToolUse"} {
		var kept []interface{}
		for _, e := range hooks[key].([]interface{}) {
			if !entryContainsBinarySubstring(e, settings.LabdrianProjectionIdentity) {
				kept = append(kept, e)
			}
		}
		hooks[key] = kept
	}
	b, _ := json.Marshal(root)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if settings.HasSupportedClaudeLifecycleState(parseJSON(t, path), testHookCommand) {
		t.Fatal("an install without the projection family reported supported")
	}
	before := parseJSON(t, path)
	if err := m.Install(); err != nil {
		t.Fatal(err)
	}
	after := parseJSON(t, path)
	if !settings.HasSupportedClaudeLifecycleState(after, testHookCommand) {
		t.Fatalf("re-running Install did not complete the state")
	}
	// Every pre-existing entry is still there, unchanged.
	for _, key := range []string{"UserPromptSubmit", "PreToolUse", "SessionEnd"} {
		have := map[string]bool{}
		for _, e := range after["hooks"].(map[string]interface{})[key].([]interface{}) {
			have[mustJSON(t, e)] = true
		}
		for _, e := range before["hooks"].(map[string]interface{})[key].([]interface{}) {
			if !have[mustJSON(t, e)] {
				t.Errorf("%s: older entry changed: %s", key, mustJSON(t, e))
			}
		}
	}
	if reflect.DeepEqual(before, after) {
		t.Errorf("Install changed nothing")
	}
}

// withProjectionFamily adds exactly the projection entries Install writes to a
// hand-built root. It takes them from a real Install into a temp file, because
// the family is only complete when its entries are exactly what Install writes.
func withProjectionFamily(root map[string]interface{}, hookCommand string) map[string]interface{} {
	tmp, err := os.MkdirTemp("", "projection-family-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmp)
	p := filepath.Join(tmp, "settings.json")
	if err := settings.NewMerger(p, hookCommand).Install(); err != nil {
		panic(err)
	}
	data, _ := os.ReadFile(p)
	var full map[string]interface{}
	if err := json.Unmarshal(data, &full); err != nil {
		panic(err)
	}
	hooks := root["hooks"].(map[string]interface{})
	for _, key := range []string{"UserPromptSubmit", "PreToolUse"} {
		existing, _ := hooks[key].([]interface{})
		for _, e := range full["hooks"].(map[string]interface{})[key].([]interface{}) {
			if entryContainsBinarySubstring(e, settings.LabdrianProjectionIdentity) {
				existing = append(existing, e)
			}
		}
		hooks[key] = existing
	}
	return root
}
