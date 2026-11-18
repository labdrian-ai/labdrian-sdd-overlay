package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An install made before the registry refresh hook existed is reported as partial by `runtime status`
// and `runtime sync-check` (the one place the lifecycle state of the Claude hooks is read), with the
// fix named, and nothing else refuses it: `runtime update` and `runtime install` run on it and
// write the entry (labdrian-orchestrator-opencode T30 commit d, correction 3).
func TestAnInstallWithoutTheRegistryRefreshHookIsPartialAndUpdateRepairsIt(t *testing.T) {
	t.Setenv("LABDRIAN_OVERLAY_DIR", writeMinimalismOverlayFixture(t))
	t.Setenv("HOME", t.TempDir())
	configRoot := t.TempDir()
	run := func(args ...string) (int, string, string) {
		var out, errBuf bytes.Buffer
		code := -1
		runRuntimeCore(testDeps(), noPi(), noGit(), append(args, "--target", "claude", "--config-root", configRoot),
			&out, &errBuf, func(c int) { code = c })
		return code, out.String(), errBuf.String()
	}

	if code, out, errText := run("install"); code != 0 {
		t.Fatalf("install = %d\nstdout=%q\nstderr=%q", code, out, errText)
	}
	if code, out, _ := run("status"); code != 0 || !strings.Contains(out, "[claude] status: supported") {
		t.Fatalf("status of a complete install = %d %q, want supported", code, out)
	}

	settingsPath := filepath.Join(configRoot, "settings.json")
	dropRegistryRefresh(t, settingsPath)

	code, out, errText := run("status")
	if code != 1 || !strings.Contains(out, "[claude] status: partial") || !strings.Contains(out, "labdrian install-hooks") || errText != "" {
		t.Errorf("status without the entry = %d stdout=%q stderr=%q, want partial naming install-hooks and exit 1", code, out, errText)
	}
	if code, out, errText := run("update"); code != 0 {
		t.Fatalf("update on an install without the entry = %d\nstdout=%q\nstderr=%q, want it to run", code, out, errText)
	}
	if code, out, _ := run("status"); code != 0 || !strings.Contains(out, "[claude] status: supported") {
		t.Errorf("status after update = %d %q, want supported", code, out)
	}
}

// dropRegistryRefresh removes the registry refresh entry from the UserPromptSubmit list of the
// settings file, as an install made before the hook existed lacks it.
func dropRegistryRefresh(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	hooks := root["hooks"].(map[string]any)
	var kept []any
	for _, e := range hooks["UserPromptSubmit"].([]any) {
		if !strings.Contains(string(mustJSON(t, e)), "registry refresh") {
			kept = append(kept, e)
		}
	}
	if len(kept) == len(hooks["UserPromptSubmit"].([]any)) {
		t.Fatal("the install holds no registry refresh entry; the case proves nothing")
	}
	hooks["UserPromptSubmit"] = kept
	if err := os.WriteFile(path, mustJSON(t, root), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return data
}
