package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func settingsRoot(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]interface{}
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestMergeSettingsKeepsAForeignAgentValueInTheFileAndWritesTheBackup(t *testing.T) {
	const before = `{"agent":"someone-else","model":"opus"}`
	_, path := mergeMainAgent(t, before)

	root := settingsRoot(t, path)
	if root["agent"] != "someone-else" || root["model"] != "opus" {
		t.Errorf("the file holds agent=%v model=%v, want the foreign values kept", root["agent"], root["model"])
	}
	if _, ok := root["hooks"]; !ok {
		t.Error("the hooks were not installed beside the foreign agent")
	}
	backup, err := os.ReadFile(path + ".bak")
	if err != nil || string(backup) != before {
		t.Errorf("backup = %q, %v; want the bytes the file held before", backup, err)
	}
}

func TestUninstallHooksRemovesTheForemanKeyAndKeepsAForeignValue(t *testing.T) {
	for _, tc := range []struct {
		name, agent string
		kept        bool
	}{{"owned", "foreman", false}, {"foreign", "someone-else", true}} {
		t.Run(tc.name, func(t *testing.T) {
			_, path := mergeMainAgent(t, `{"agent":"`+tc.agent+`","model":"opus"}`)
			p := newCapturedProcess()
			runUninstallHooks(p.process, []string{"--settings", path, "--hook-command", mainAgentHookCommand})

			root := settingsRoot(t, path)
			if _, ok := root["agent"]; ok != tc.kept {
				t.Errorf("agent present = %v, want %v", ok, tc.kept)
			}
			if tc.kept && root["agent"] != tc.agent {
				t.Errorf("agent = %v, want %q kept", root["agent"], tc.agent)
			}
			if root["model"] != "opus" {
				t.Error("a foreign key was lost")
			}
			if p.out.String() != "uninstall-hooks: hooks removed successfully\n" || p.err.String() != "" || len(p.exits) != 0 {
				t.Errorf("stdout = %q, stderr = %q, exits = %v", p.out.String(), p.err.String(), p.exits)
			}
		})
	}
}

func TestStatusReportsAMissingAndAConflictingAgentKeyOnTheCheckLine(t *testing.T) {
	for _, tc := range []struct{ name, settings, want string }{
		{"missing", `{"model":"opus"}`, `[WARN] settings: "agent" (main session runs as the foreman) — "agent" key missing in %s; the main session does not run as the foreman; run 'labdrian install-hooks'; restart Claude Code to load hook changes`},
		{"conflict", `{"agent":"someone-else"}`, `[WARN] settings: "agent" (main session runs as the foreman) — "agent" is "someone-else", not "foreman"; the overlay never overwrites it, so the main session does not run as the foreman; set it to "foreman" or remove the key, then run 'labdrian install-hooks'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			path := home + "/.claude/settings.json"
			if err := os.MkdirAll(home+"/.claude", 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(tc.settings), 0o600); err != nil {
				t.Fatal(err)
			}
			d := testDeps()
			d.getenv = environmentOf(map[string]string{"HOME": home})
			d.getwd = func() (string, error) { return "", os.ErrNotExist }
			p := newCapturedProcess()
			runStatus(p.process, d, nil)

			want := strings.Replace(tc.want, "%s", path, 1)
			if !strings.Contains(p.out.String(), want+"\n") {
				t.Errorf("the report has no line %q:\n%s", want, p.out.String())
			}
		})
	}
}
