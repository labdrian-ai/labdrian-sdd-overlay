package shelltest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/enginetest"
)

func TestCodexHooksShellLifecycle(t *testing.T) {
	overlayScript(t) // require bash
	hooks, err := filepath.Abs("../../bin/lib/hooks.sh")
	if err != nil {
		t.Fatal(err)
	}
	binary := enginetest.Binary(t)
	for _, present := range []bool{false, true} {
		t.Run(map[bool]string{false: "absent", true: "present"}[present], func(t *testing.T) {
			home := t.TempDir()
			codex := filepath.Join(home, "codex-home")
			bin := filepath.Join(home, "bin")
			if err := os.MkdirAll(bin, 0700); err != nil {
				t.Fatal(err)
			}
			installed := filepath.Join(home, ".claude", "bin", "gentle-ai-overlay")
			if err := os.MkdirAll(filepath.Dir(installed), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(binary, installed); err != nil {
				t.Fatal(err)
			}
			if present {
				if err := os.MkdirAll(codex, 0700); err != nil {
					t.Fatal(err)
				}
			}
			// Building is stubbed, the binary being tested was built by enginetest. No installer can run.
			if err := os.WriteFile(filepath.Join(bin, "go"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
				t.Fatal(err)
			}
			script := `set -eu
source "$HOOKS"
info() { :; }
warn() { :; }
die() { printf '%s\n' "$*" >&2; exit 1; }
engine_binary_is_stale() { return 1; }
cmd_install_hooks
`
			run := func(body string) string {
				t.Helper()
				cmd := exec.Command("bash", "-c", body)
				cmd.Env = []string{"HOME=" + home, "CODEX_HOME=" + codex, "HOOKS=" + hooks, "ENGINE_SRC=" + home, "ENGINE_BINARY=" + installed, "PATH=" + bin + ":/usr/bin:/bin", "OVERLAY_SKIP_TOOL_INSTALL=1"}
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("%v\n%s", err, out)
				}
				return string(out)
			}
			out := run(script)
			path := filepath.Join(codex, "hooks.json")
			if !present {
				if _, err := os.Stat(codex); !os.IsNotExist(err) {
					t.Fatalf("created codex home: %v", err)
				}
				if strings.Contains(out, "Codex memory hooks") || strings.Contains(out, "merge-codex-hooks:") {
					t.Fatal(out)
				}
			} else {
				data, err := os.ReadFile(path)
				if err != nil || !strings.Contains(string(data), "projection hook") {
					t.Fatalf("%s %v", data, err)
				}
				for _, event := range []string{"SessionStart", "PreToolUse", "PostToolUse"} {
					if !strings.Contains(string(data), "--event "+event) {
						t.Fatalf("missing %s: %s", event, data)
					}
				}
				if !strings.Contains(out, "Codex memory hooks (SessionStart, PreToolUse, PostToolUse) installed") ||
					!strings.Contains(out, "trust each new handler once") || !strings.Contains(out, "/hooks, key t") ||
					!strings.Contains(out, "two more trusts") || !strings.Contains(out, "no trust state or config.toml is changed") {
					t.Fatal(out)
				}
				if _, err := os.Stat(filepath.Join(codex, "config.toml")); !os.IsNotExist(err) {
					t.Fatalf("created Codex configuration: %v", err)
				}
				// status is delegated to the same engine; a missing Claude contract can make it exit 1.
				status := run(strings.Replace(script, "cmd_install_hooks", "(cmd_status_hooks) || true", 1))
				if !strings.Contains(status, "[OK  ] hook: Codex memory (SessionStart, PreToolUse, PostToolUse)") || !strings.Contains(status, "trust") {
					t.Fatal(status)
				}
			}
			uninstall := strings.Replace(script, "cmd_install_hooks", "cmd_uninstall_hooks", 1)
			out = run(uninstall)
			if present {
				data, err := os.ReadFile(path)
				if err != nil || strings.Contains(string(data), "projection hook") {
					t.Fatalf("uninstall: %s %v", data, err)
				}
			} else if strings.Contains(out, "Codex memory hooks") || strings.Contains(out, "uninstall-codex-hooks:") {
				t.Fatal(out)
			}
			if _, err := os.Stat(filepath.Join(home, ".codex")); !os.IsNotExist(err) {
				t.Fatalf("created default home: %v", err)
			}
		})
	}
}
