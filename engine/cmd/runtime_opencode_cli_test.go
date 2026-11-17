package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These run `engine runtime install|status|uninstall --target opencode` the way the program does,
// from the environment of a sandbox: a HOME of its own, a PATH with one of the two MCP programs, the
// overlay checkout of the fixture. Nothing is passed as --config-root, so the wiring of the registered
// adapter is what finds the config directory, the absolute path of the servers and the skills
// directory of ~/.agents.

// openCodeSandbox is the world of one run: the home, the overlay and the PATH directory.
type openCodeSandbox struct {
	home, overlay, bin string
}

func newOpenCodeSandbox(t *testing.T) openCodeSandbox {
	t.Helper()
	s := openCodeSandbox{home: t.TempDir(), overlay: writeMinimalismOverlayFixture(t), bin: t.TempDir()}
	for path, text := range map[string]string{
		filepath.Join(s.overlay, "output-styles", "labdrian.md"):             "---\nname: labdrian\n---\n# Style body\n",
		filepath.Join(s.overlay, "skills", "role-chain", "SKILL.md"):         "---\nname: role-chain\n---\n",
		filepath.Join(s.home, ".agents", "skills", "role-chain", "SKILL.md"): "a copy that shadows the overlay skill",
		filepath.Join(s.bin, "memorycard"):                                   "#!/bin/sh\n",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		mode := os.FileMode(0o644)
		if filepath.Base(path) == "memorycard" {
			mode = 0o755
		}
		if err := os.WriteFile(path, []byte(text), mode); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func (s openCodeSandbox) run(t *testing.T, action string) (stdout string, exit int) {
	t.Helper()
	d := testDeps()
	d.getenv = environmentOf(map[string]string{"HOME": s.home, "PATH": s.bin, "LABDRIAN_OVERLAY_DIR": s.overlay})
	var out, errBuf bytes.Buffer
	exit = -1
	runRuntimeCore(d, noPi(), noGit(), []string{action, "--target", "opencode"}, &out, &errBuf, func(code int) { exit = code })
	if errBuf.Len() != 0 {
		t.Errorf("runtime %s wrote on stderr: %q", action, errBuf.String())
	}
	return out.String(), exit
}

func readFileText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestRuntimeCLIOpenCodeInstallStatusUninstallInASandboxHome(t *testing.T) {
	s := newOpenCodeSandbox(t)
	config := filepath.Join(s.home, ".config", "opencode")
	owner := "{\n    \"share\": \"manual\"\n}\n"
	if err := os.MkdirAll(config, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "opencode.json"), []byte(owner), 0o644); err != nil {
		t.Fatal(err)
	}

	out, exit := s.run(t, "install")

	if exit != 0 {
		t.Fatalf("install exit %d\n%s", exit, out)
	}
	for _, want := range []string{
		"[opencode] install: restart_required",
		"opencode.json written to " + filepath.Join(config, "opencode.json"),
		"codegraph not found on PATH",
		"AGENTS.md written to " + filepath.Join(config, "AGENTS.md"),
		filepath.Join(s.home, ".agents", "skills") + ": role-chain",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("install output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "memorycard not found") {
		t.Errorf("install output says memorycard was not found though the PATH has it:\n%s", out)
	}
	installed := readFileText(t, filepath.Join(config, "opencode.json"))
	if !strings.HasPrefix(installed, "{\n    \"share\": \"manual\"") || !strings.Contains(installed, filepath.Join(s.bin, "memorycard")) || !strings.Contains(installed, `"default_agent": "foreman"`) {
		t.Errorf("opencode.json = %s", installed)
	}
	if text := readFileText(t, filepath.Join(config, "AGENTS.md")); !strings.Contains(text, "# Style body") || strings.Contains(text, "name: labdrian") {
		t.Errorf("AGENTS.md = %s", text)
	}
	if _, err := os.Stat(filepath.Join(config, "skills")); !os.IsNotExist(err) {
		t.Errorf("a skills directory was created in the OpenCode config (stat err %v)", err)
	}
	if text := readFileText(t, filepath.Join(s.home, ".agents", "skills", "role-chain", "SKILL.md")); text != "a copy that shadows the overlay skill" {
		t.Errorf("the shadowing skill of the owner was touched: %q", text)
	}

	status, statusExit := s.run(t, "status")
	if !strings.Contains(status, "[opencode] status: restart_required") || statusExit != 1 || !strings.Contains(status, ": role-chain") {
		t.Errorf("status = exit %d\n%s", statusExit, status)
	}

	out, exit = s.run(t, "uninstall")
	if exit != 0 || !strings.Contains(out, "[opencode] uninstall: restart_required") {
		t.Fatalf("uninstall exit %d\n%s", exit, out)
	}
	if got := readFileText(t, filepath.Join(config, "opencode.json")); got != owner {
		t.Errorf("uninstall did not give back the owner's opencode.json: %q", got)
	}
	if _, err := os.Stat(filepath.Join(config, "AGENTS.md")); !os.IsNotExist(err) {
		t.Errorf("AGENTS.md was created by the install and is still there (stat err %v)", err)
	}
}

func TestRuntimeCLIOpenCodeRefusedConfigStillWritesTheRest(t *testing.T) {
	s := newOpenCodeSandbox(t)
	config := filepath.Join(s.home, ".config", "opencode")
	if err := os.MkdirAll(config, 0o755); err != nil {
		t.Fatal(err)
	}
	broken := `{"mcp": {} } trailing`
	if err := os.WriteFile(filepath.Join(config, "opencode.json"), []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}

	out, exit := s.run(t, "install")

	if exit == 0 || !strings.Contains(out, "[opencode] install: partial") || !strings.Contains(out, "OpenCode config not written") {
		t.Errorf("install exit %d, want a partial result naming the config:\n%s", exit, out)
	}
	if got := readFileText(t, filepath.Join(config, "opencode.json")); got != broken {
		t.Errorf("the refused file changed: %q", got)
	}
	if !strings.Contains(readFileText(t, filepath.Join(config, "AGENTS.md")), "labdrian:opencode-instructions") {
		t.Errorf("AGENTS.md was not written")
	}
}
