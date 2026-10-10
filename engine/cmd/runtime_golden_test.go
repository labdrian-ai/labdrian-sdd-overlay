//go:build unix

package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The golden files under testdata/runtime-golden record what 'runtime <action>' prints and exits
// with: status, install, update and uninstall, for each runtime (claude, codex, opencode, pi)
// and for all of them, in every state the installation can be found in, and the same actions on
// the longterm-mem component, and what every bad command line is told. They were recorded from
// the program as it was before Phase 9 unit H30 (docs/architecture/hexagonal-target.md) moved the
// rule that turns the results of the targets into an exit code out of cmd and gave the options of
// the command a type of their own, and a change to a byte of any of them fails here. Rewrite them
// deliberately with
//
//	go test ./cmd -run TestRuntimeGolden -update-runtime-golden
//
// and read the diff before committing it.
//
// Each case runs the built program in a world as status_golden_test.go describes: a home and a
// PATH inside a directory with a neutral name, and no other variable of the test process. The
// PATH holds nothing, so no case can start a pi or a git of the machine, and the Pi runtime is
// only ever asked what it can tell without a program: the Pi cases cannot install it. What depends
// on the machine, and the words of the JSON library and the usage text that the transcript writes
// as placeholders, are described in status_golden_test.go.
var updateRuntimeGolden = flag.Bool("update-runtime-golden", false, "rewrite the golden files of the runtime verb")

// runtime records one run of 'runtime <args>' and says what it shows.
func (w *registryWorld) runtime(label string, args ...string) {
	w.t.Helper()
	w.label("%s", label)
	w.run(append([]string{"runtime"}, args...)...)
}

// eachTarget records the same action on every target the command knows, then on all of them.
func (w *registryWorld) eachTarget(action string) {
	w.t.Helper()
	for _, target := range []string{"claude", "codex", "opencode", "pi", "all"} {
		w.runtime(action+" "+target, action, "--target", target)
	}
}

// files records the names under the home of the world: what the actions left.
func (w *registryWorld) files(label string) {
	w.t.Helper()
	w.label("%s", label)
	w.treeOf(worldHome, false)
}

// contractOverlay makes an overlay checkout of the world that holds the contracts the OpenCode plugin is built from.
func (w *registryWorld) contractOverlay() string {
	w.t.Helper()
	w.put("overlay/skills/_shared/minimalism-contract.md", contractDoc(fmApplies, fmExcluded, fmInject))
	return w.path("overlay")
}

func runtimeGoldenCases() []registryGoldenCase {
	return []registryGoldenCase{
		{"runtime-status-in-an-empty-home", func(w *registryWorld) {
			w.runtime("with no --target the runtime is opencode", "status")
			w.eachTarget("status")
			w.runtime("a target is trimmed and the case matters", "status", "--target", " claude ")
			w.runtime("a target in capitals is not one", "status", "--target", "Claude")
		}},
		{"runtime-lifecycle-of-claude", func(w *registryWorld) {
			for _, step := range []string{"status", "install", "status", "install", "update", "status", "uninstall", "status", "uninstall"} {
				w.runtime(step+" claude", step, "--target", "claude")
			}
			w.files("what is left")
		}},
		{"runtime-lifecycle-of-codex", func(w *registryWorld) {
			for _, step := range []string{"status", "install", "status", "install", "update", "status", "uninstall", "status", "uninstall"} {
				w.runtime(step+" codex", step, "--target", "codex")
			}
			w.files("what is left")
		}},
		{"runtime-lifecycle-of-all-the-targets", func(w *registryWorld) {
			w.eachTarget("install")
			w.runtime("status all after installing: codex alone is partial and opencode and pi are not installed", "status", "--target", "all")
			w.eachTarget("update")
			w.eachTarget("uninstall")
			w.runtime("status all after uninstalling", "status", "--target", "all")
			w.files("what is left")
		}},
		{"runtime-opencode-and-pi-with-an-overlay", func(w *registryWorld) {
			w.eachTarget("install")
			w.setenv("LABDRIAN_OVERLAY_DIR", w.contractOverlay())
			w.setenv("OVERLAY_DIR", w.path("overlay"))
			w.runtime("opencode with the overlay named", "install", "--target", "opencode")
			w.runtime("opencode again", "install", "--target", "opencode")
			w.runtime("status opencode", "status", "--target", "opencode")
			w.runtime("update opencode", "update", "--target", "opencode")
			w.runtime("status opencode", "status", "--target", "opencode")
			w.runtime("pi with an overlay that holds no registry", "status", "--target", "pi")
			w.runtime("install pi", "install", "--target", "pi")
			w.runtime("update pi", "update", "--target", "pi")
			w.runtime("uninstall pi", "uninstall", "--target", "pi")
			w.runtime("status all", "status", "--target", "all")
			w.runtime("uninstall opencode", "uninstall", "--target", "opencode")
			w.runtime("status opencode", "status", "--target", "opencode")
			w.files("what is left")
		}},
		{"runtime-claude-with-settings-that-cannot-be-used", func(w *registryWorld) {
			for _, v := range []settingsText{
				{"a file that is not JSON", "{not json"},
				{"the JSON value null", "null"},
				{"an array", "[]"},
				{"hooks is an array", `{"hooks":[]}`},
				{"an event that is not an array", `{"hooks":{"PreToolUse":"x"}}`},
				{"an empty file", ""},
			} {
				w.removeAll("home/.claude")
				w.put(worldSettings, v.text)
				w.label("%s", v.label)
				for _, action := range []string{"status", "install", "update", "uninstall"} {
					w.runtime(action+" claude", action, "--target", "claude")
					w.treeOf("home/.claude", false)
				}
			}
			w.removeAll("home/.claude")
			w.mkdir(worldSettings)
			w.label("a directory where settings.json should be")
			for _, action := range []string{"status", "install", "update", "uninstall"} {
				w.runtime(action+" claude", action, "--target", "claude")
			}
		}},
		{"runtime-claude-families-present-and-absent", func(w *registryWorld) {
			w.runtime("install claude", "install", "--target", "claude")
			full := w.read(worldSettings)
			variant := func(label string, edit func(root map[string]any)) {
				w.put(worldSettings, full)
				w.editJSON(worldSettings, edit)
				w.runtime(label, "status", "--target", "claude")
			}
			w.runtime("the complete install", "status", "--target", "claude")
			variant("no SessionEnd entry", func(r map[string]any) { delete(r["hooks"].(map[string]any), "SessionEnd") })
			variant("no review-receipt entry", func(r map[string]any) { dropHooks(r, "PreToolUse", "review-receipt") })
			variant("no deny rule", func(r map[string]any) { delete(r, "permissions") })
			variant("hooks disabled", func(r map[string]any) { r["disableAllHooks"] = true })
			variant("no shaper guard entries", func(r map[string]any) { dropHooks(r, "PreToolUse", "shaper guard-hook") })
			variant("no projection entries", func(r map[string]any) {
				dropHooks(r, "PreToolUse", "projection hook")
				dropHooks(r, "UserPromptSubmit", "projection hook")
			})
			variant("no approve guard entries", func(r map[string]any) { dropHooks(r, "PreToolUse", "skills guard-hook") })
			variant("no gate-task entries", func(r map[string]any) { dropHooks(r, "PreToolUse", "gate-task") })
			variant("no hooks at all", func(r map[string]any) { delete(r, "hooks") })
			w.put(worldSettings, full)
			w.editJSON(worldSettings, func(r map[string]any) { dropHooks(r, "PreToolUse", "review-receipt") })
			w.runtime("update brings the missing entry back", "update", "--target", "claude")
			w.runtime("status after it", "status", "--target", "claude")
			w.put(worldSettings, `{"model":"x","hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"echo mine"}]}]},"permissions":{"deny":["Bash(rm *)"]}}`)
			w.runtime("install into settings that have entries of their own", "install", "--target", "claude")
			w.runtime("uninstall leaves them", "uninstall", "--target", "claude")
			w.show(worldSettings)
			w.runtime("status after uninstalling", "status", "--target", "claude")
		}},
		{"runtime-codex-manifest-states", func(w *registryWorld) {
			manifest := "home/.codex/labdrian-runtime-lifecycle.json"
			w.runtime("install codex", "install", "--target", "codex")
			good := w.read(manifest)
			for _, v := range []settingsText{
				{"the manifest the install wrote", good},
				{"not JSON", "{not json"},
				{"empty", ""},
				{"null", "null"},
				{"managed by someone else", strings.Replace(good, "labdrian-sdd-overlay", "somebody-else", 1)},
				{"another version", strings.Replace(good, "2026-07-05-codex-runtime-lifecycle", "1999-01-01-old", 1)},
				{"another config root", strings.Replace(good, `"config_root": "`, `"config_root": "/elsewhere`, 1)},
				{"no fields", "{}"},
			} {
				w.put(manifest, v.text)
				w.label("%s", v.label)
				for _, action := range []string{"status", "update", "uninstall"} {
					w.runtime(action+" codex", action, "--target", "codex")
					w.put(manifest, v.text)
				}
			}
			w.removeAll("home/.codex")
			w.mkdir(manifest)
			w.runtime("a directory where the manifest should be", "status", "--target", "codex")
			w.runtime("install over it", "install", "--target", "codex")
		}},
		{"runtime-longterm-mem-component", func(w *registryWorld) {
			state := w.path("state")
			w.setenv("STATE_DIR", state)
			for _, action := range []string{"status", "install", "update", "uninstall"} {
				w.runtime(action+" with nothing installed", action, "--component", "longterm-mem")
			}
			w.putMode("state/bin/longterm-mem", "#!/bin/sh\n", 0o755)
			w.mkdir("home/.claude")
			w.mkdir("home/.codex")
			w.mkdir("home/.config/opencode")
			for _, action := range []string{"status", "install", "status", "update", "uninstall", "status"} {
				w.runtime(action+" with the binary and the runtimes", action, "--component", "longterm-mem", "--state-dir", state)
			}
			w.runtime("the target is not read by the component", "status", "--component", "longterm-mem", "--target", "pi", "--state-dir", state)
			w.runtime("the config root is not read by the component", "status", "--component", "longterm-mem", "--config-root", w.path("elsewhere"), "--state-dir", state)
			w.putMode("state/bin/longterm-mem", "#!/bin/sh\n", 0o644)
			w.runtime("the binary is not executable", "install", "--component", "longterm-mem", "--state-dir", state)
			w.files("what is left")
			w.treeOf("state", false)
		}},
		{"runtime-bad-command-lines", func(w *registryWorld) {
			// The usage text follows most of these; hideForeignWords writes it as <USAGE>, here and
			// in every case of this file, so a verb added elsewhere does not change what is pinned.
			for _, args := range [][]string{
				{},
				{"--target", "claude"},
				{"-x"},
				{"restart"},
				{"rollback"},
				{"status", "--target"},
				{"status", "--target", "vim"},
				{"status", "--config-root"},
				{"status", "--state-dir"},
				{"status", "--component"},
				{"status", "--component", "nonsense"},
				{"status", "--unknown"},
				{"status", "extra"},
				{"status", "--target", "claude", "extra"},
				{"update", "--component", "longterm-mem"},
				{"status", "--component", "runtime-parity", "--component", "longterm-mem", "--component", "runtime-parity", "--target", "claude"},
				{"capabilities", "--bogus"},
				{"probe", "--bogus"},
			} {
				w.runtime("runtime "+strings.Join(args, " "), args...)
			}
		}},
		{"runtime-roots-come-from-the-environment", func(w *registryWorld) {
			w.setenv("XDG_CONFIG_HOME", w.path("xdg"))
			w.setenv("CODEX_HOME", w.path("codex-home"))
			w.setenv("STATE_DIR", w.path("state"))
			w.runtime("install claude, codex and opencode with the roots the environment names", "install", "--target", "all")
			w.treeOf(".", false)
			w.runtime("status all", "status", "--target", "all")
			w.runtime("--config-root wins over the environment", "install", "--target", "codex", "--config-root", w.path("given"))
			w.treeOf("given", false)
			w.runtime("status of the root that was given", "status", "--target", "codex", "--config-root", w.path("given"))
			w.setenv("HOME", "  "+w.path(worldHome)+"  ")
			w.runtime("a HOME with space around it is trimmed", "status", "--target", "claude")
			w.setenv("HOME", "")
			w.runtime("no HOME: there is no default root", "status", "--target", "claude")
		}},
	}
}

// TestRuntimeGolden runs every case and compares its transcript with its golden file.
func TestRuntimeGolden(t *testing.T) {
	seen := map[string]bool{}
	for _, tc := range runtimeGoldenCases() {
		if seen[tc.name] {
			t.Errorf("two cases are named %q", tc.name)
		}
		seen[tc.name] = true
		t.Run(tc.name, func(t *testing.T) {
			w := newBinaryWorld(t)
			tc.run(w)
			checkGoldenIn(t, "runtime-golden", tc.name, w.text(), updateRuntimeGolden, "-update-runtime-golden")
		})
	}
	entries, err := os.ReadDir(filepath.Join("testdata", "runtime-golden"))
	if err != nil {
		if *updateRuntimeGolden && os.IsNotExist(err) {
			return
		}
		t.Fatalf("the golden directory testdata/runtime-golden cannot be read: %v", err)
	}
	for _, e := range entries {
		if name := strings.TrimSuffix(e.Name(), ".golden"); !seen[name] {
			t.Errorf("testdata/runtime-golden/%s belongs to no case", e.Name())
		}
	}
}
