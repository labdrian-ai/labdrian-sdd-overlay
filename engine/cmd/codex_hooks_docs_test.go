package main

import (
	"os"
	"strings"
	"testing"
)

func TestCodexHookCommandsAreDocumented(t *testing.T) {
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	help, err := os.ReadFile("../../bin/lib/usage.sh")
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{"usage": captureUsage(t), "README": string(readme)} {
		for _, want := range []string{"merge-codex-hooks --hooks <path> --hook-command <binary-path>", "uninstall-codex-hooks --hooks <path> --hook-command <binary-path>", "startup|resume|clear|compact", "config.toml", "/hooks", "key t"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s missing %q", name, want)
			}
		}
	}
	for _, want := range []string{"CODEX_HOME", "SessionStart", "config.toml", "/hooks, key t"} {
		if !strings.Contains(string(help), want) {
			t.Errorf("wrapper help missing %q", want)
		}
	}
}
