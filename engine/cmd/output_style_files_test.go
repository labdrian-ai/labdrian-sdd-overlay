package main

import (
	"os"
	"strings"
	"testing"
)

// The "outputStyle" key through the public commands (T27, L57's ownership rules), as main_agent_*_test.go
// does for "agent": what merge-settings and uninstall-hooks leave in the file, and what they say.

const installedOK = "merge-settings: hooks installed successfully\n"

func foreignStyleNoticeLine(path, value string) string {
	return "notice: " + path + " holds \"outputStyle\": " + value + "; it was left as it is, so the labdrian output style was not enabled (set it to \"labdrian\" or remove the key, then run install-hooks again)\n"
}

func TestMergeSettingsWritesTheOutputStyleKeyAndSaysNothingExtra(t *testing.T) {
	for _, before := range []string{"", `{"model":"opus"}`, `{"outputStyle":"labdrian"}`} {
		p, path := mergeMainAgent(t, before)
		if got := settingsRoot(t, path)["outputStyle"]; got != "labdrian" {
			t.Errorf("before %q: outputStyle = %v, want labdrian", before, got)
		}
		if p.err.String() != "" || p.out.String() != installedOK || len(p.exits) != 0 {
			t.Errorf("before %q: stdout = %q, stderr = %q, exits = %v", before, p.out.String(), p.err.String(), p.exits)
		}
	}
}

func TestMergeSettingsKeepsAForeignOutputStyleAndSaysSoOnStderr(t *testing.T) {
	for _, tc := range []struct{ file, value string }{
		{`{"outputStyle":"Gentleman","model":"opus"}`, `"Gentleman"`},
		{`{"outputStyle":{"name":"x"}}`, `{"name":"x"}`},
		{`{"outputStyle":"a<b>&c"}`, `"a<b>&c"`},
		{`{"outputStyle":null}`, `null`},
	} {
		p, path := mergeMainAgent(t, tc.file)
		if want := foreignStyleNoticeLine(path, tc.value); p.err.String() != want {
			t.Errorf("file %s: stderr = %q, want %q", tc.file, p.err.String(), want)
		}
		if p.out.String() != installedOK || len(p.exits) != 0 {
			t.Errorf("file %s: stdout = %q, exits = %v; the install still succeeds", tc.file, p.out.String(), p.exits)
		}
		backup, err := os.ReadFile(path + ".bak")
		if err != nil || string(backup) != tc.file {
			t.Errorf("file %s: backup = %q, %v; want the bytes the file held before", tc.file, backup, err)
		}
	}
	_, path := mergeMainAgent(t, `{"outputStyle":"Gentleman","model":"opus"}`)
	root := settingsRoot(t, path)
	if root["outputStyle"] != "Gentleman" || root["model"] != "opus" || root["agent"] != "foreman" {
		t.Errorf("the file holds outputStyle=%v model=%v agent=%v, want the foreign values kept and the agent written", root["outputStyle"], root["model"], root["agent"])
	}
}

// Both keys foreign: the agent notice, then the style notice, one line each.
func TestMergeSettingsSaysBothNoticesAgentFirst(t *testing.T) {
	p, path := mergeMainAgent(t, `{"agent":"someone-else","outputStyle":"Gentleman"}`)
	want := "notice: " + path + " holds \"agent\": \"someone-else\"; it was left as it is, so the foreman main session was not enabled (set it to \"foreman\" or remove the key, then run install-hooks again)\n" +
		foreignStyleNoticeLine(path, `"Gentleman"`)
	if p.err.String() != want {
		t.Errorf("stderr = %q, want %q", p.err.String(), want)
	}
}

func TestUninstallHooksRemovesTheOutputStyleKeyOnlyWhenItIsLabdrian(t *testing.T) {
	for _, tc := range []struct {
		name, style string
		kept        bool
	}{{"owned", "labdrian", false}, {"foreign", "Gentleman", true}} {
		t.Run(tc.name, func(t *testing.T) {
			_, path := mergeMainAgent(t, `{"outputStyle":"`+tc.style+`","model":"opus"}`)
			p := newCapturedProcess()
			runUninstallHooks(p.process, []string{"--settings", path, "--hook-command", mainAgentHookCommand})

			root := settingsRoot(t, path)
			if _, ok := root["outputStyle"]; ok != tc.kept {
				t.Errorf("outputStyle present = %v, want %v", ok, tc.kept)
			}
			if tc.kept && root["outputStyle"] != tc.style {
				t.Errorf("outputStyle = %v, want %q kept", root["outputStyle"], tc.style)
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

func TestStatusReportsAMissingAndAConflictingOutputStyleOnTheCheckLine(t *testing.T) {
	for _, tc := range []struct{ name, settings, want string }{
		{"missing", `{"model":"opus"}`, `[WARN] settings: "outputStyle" (labdrian output style) — "outputStyle" key missing in %s; the labdrian output style is not enabled; run 'labdrian install-hooks'; restart Claude Code to load hook changes`},
		{"conflict", `{"outputStyle":"Gentleman"}`, `[WARN] settings: "outputStyle" (labdrian output style) — "outputStyle" is "Gentleman", not "labdrian"; the overlay never overwrites it, so the labdrian output style is not enabled; set it to "labdrian" or remove the key, then run 'labdrian install-hooks'`},
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
