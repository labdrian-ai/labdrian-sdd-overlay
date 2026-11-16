package main

import (
	"os"
	"path/filepath"
	"testing"
)

const mainAgentHookCommand = "/test/.claude/bin/gentle-ai-overlay"

func mergeMainAgent(t *testing.T, before string) (*capturedProcess, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.json")
	if before != "" {
		if err := os.WriteFile(path, []byte(before), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	p := newCapturedProcess()
	runMergeSettings(p.process, []string{"--settings", path, "--hook-command", mainAgentHookCommand})
	return p, path
}

func TestMergeSettingsSaysWhenAForeignAgentValueKeepsTheForemanOff(t *testing.T) {
	p, path := mergeMainAgent(t, `{"agent":"someone-else"}`)
	want := "notice: " + path + " holds \"agent\": \"someone-else\"; it was left as it is, so the foreman main session was not enabled (set it to \"foreman\" or remove the key, then run install-hooks again)\n"
	if p.err.String() != want {
		t.Errorf("stderr = %q, want %q", p.err.String(), want)
	}
	if p.out.String() != "merge-settings: hooks installed successfully\n" || len(p.exits) != 0 {
		t.Errorf("stdout = %q, exits = %v; the install still succeeds", p.out.String(), p.exits)
	}
}

func TestMergeSettingsPrintsANonStringAgentValueAsJSONInTheNotice(t *testing.T) {
	p, path := mergeMainAgent(t, `{"agent":{"name":"x"}}`)
	want := "notice: " + path + " holds \"agent\": {\"name\":\"x\"}; it was left as it is, so the foreman main session was not enabled (set it to \"foreman\" or remove the key, then run install-hooks again)\n"
	if p.err.String() != want {
		t.Errorf("stderr = %q, want %q", p.err.String(), want)
	}
}

// A value with <, > or & is printed as the file holds it, not HTML-escaped.
func TestMergeSettingsPrintsTheAgentValueUnescaped(t *testing.T) {
	p, path := mergeMainAgent(t, `{"agent":"a<b>&c"}`)
	want := "notice: " + path + " holds \"agent\": \"a<b>&c\"; it was left as it is, so the foreman main session was not enabled (set it to \"foreman\" or remove the key, then run install-hooks again)\n"
	if p.err.String() != want {
		t.Errorf("stderr = %q, want %q", p.err.String(), want)
	}
}

func TestMergeSettingsSaysNothingExtraWhenTheForemanIsSet(t *testing.T) {
	for _, before := range []string{"", `{"agent":"foreman"}`, `{"model":"opus"}`} {
		p, _ := mergeMainAgent(t, before)
		if p.err.String() != "" {
			t.Errorf("before %q: stderr = %q, want nothing", before, p.err.String())
		}
	}
}
