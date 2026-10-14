package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The golden files under testdata/register-golden pin what `register` and `unregister` print, exit with and
// leave in the runtimes' configuration (and in install-state.json) when they are run over a machine: the targets a
// --target names or "all" expands to, what is skipped, what is refused, what cannot be resolved, and the exit code
// that is chosen among the outcomes of several targets. They were recorded from the program as it stood before the
// orchestration of the two commands (target expansion, the pi probe, the defaults, the loop over the targets, the
// choice of the exit code) moved out of the command package into a use case (Phase 9, L10), and that move must not
// alter one byte of them.
//
// Each scenario is a machine of its own: a HOME the test made, with no XDG_CONFIG_HOME, CODEX_HOME or STATE_DIR
// (so every default path is under it), and the files the scenario puts there. What a scenario ran, and what was
// left on the machine, is rendered with the paths of the machine masked.
//
// To rewrite them after a change that is MEANT to alter the output:
//
//	go test ./cmd/longterm-mem -run TestRegisterGolden -update-promote-golden
//
// and read the diff before committing it.

// registerFingerprint is the digest install-state.json records for an entry. The entry holds the path of the binary,
// which is under the machine's home unless a scenario gives one, so the digest differs from one run to the next and
// is masked by where it is; the entry it covers is in the golden, and the digest of an entry is held by the tests
// of internal/register.
var registerFingerprint = regexp.MustCompile(`"fingerprint": "[0-9a-f]{64}"`)

// registerWorld is a machine for `register` and `unregister` to act on.
type registerWorld struct {
	home string
}

func (w *registerWorld) write(t *testing.T, rel, content string) {
	t.Helper()
	path := filepath.Join(w.home, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

// directory makes a directory where a runtime's file should be, so that reading the file fails.
func (w *registerWorld) directory(t *testing.T, rel string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(w.home, filepath.FromSlash(rel)), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
}

const (
	registerEmptyClaude   = `{"mcpServers": {}}` + "\n"
	registerEmptyOpencode = `{"mcp": {}}` + "\n"
	registerEmptyCodex    = "# the user's own settings\nmodel = \"o3\"\n"
	registerEmptyPi       = `{"mcpServers": {}}` + "\n"

	// registerForeignClaude holds an entry of the name longterm-mem registers under that longterm-mem did not write.
	registerForeignClaude   = `{"mcpServers": {"longterm-mem": {"type": "stdio", "command": "/someone/elses/binary", "args": ["mcp"]}}}` + "\n"
	registerForeignOpencode = `{"mcp": {"longterm-mem": {"type": "local", "command": ["/someone/elses/binary", "mcp"]}}}` + "\n"
)

// registerFiles puts the configuration of the runtimes a machine has on it.
type registerFiles struct{ claude, opencode, codex bool }

func (f registerFiles) put(t *testing.T, w *registerWorld) {
	t.Helper()
	if f.claude {
		w.write(t, ".claude.json", registerEmptyClaude)
	}
	if f.opencode {
		w.write(t, ".config/opencode/opencode.json", registerEmptyOpencode)
	}
	if f.codex {
		w.write(t, ".codex/config.toml", registerEmptyCodex)
	}
}

// installPi builds the package pi registers in and lists it in pi's settings, which is what a machine that has pi
// installed looks like to the probe.
func installPi(t *testing.T, w *registerWorld) {
	t.Helper()
	w.write(t, ".labdrian-overlay/pi/labdrian-pi/mcp.json", registerEmptyPi)
	w.write(t, ".pi/agent/settings.json", `{"packages": ["`+filepath.Join(w.home, ".labdrian-overlay", "pi", "labdrian-pi")+`"]}`+"\n")
}

type registerScenario struct {
	name string
	// setup puts the files of the machine.
	setup func(t *testing.T, w *registerWorld)
	// homeless runs the commands with no HOME at all, so that nothing derived from it can be resolved.
	homeless bool
	// xdg is the XDG_CONFIG_HOME of the run, under the machine's home; empty leaves it unset.
	xdg      string
	commands [][]string
}

func registerScenarios() []registerScenario {
	register := func(args ...string) []string { return append([]string{"register"}, args...) }
	unregister := func(args ...string) []string { return append([]string{"unregister"}, args...) }
	all := registerFiles{true, true, true}
	flags := []string{"--state-dir", "{home}/state", "--binary", "/opt/bin/longterm-mem"}
	withFlags := func(args ...string) []string { return append(append([]string(nil), args...), flags...) }

	return []registerScenario{
		// register: what is refused before any target is touched.
		{name: "01-register-with-no-target", commands: [][]string{register()}},
		{name: "02-register-an-unknown-target", commands: [][]string{register("--target", "bogus")}},
		{name: "03-register-all-with-a-config-root", commands: [][]string{register("--target", "all", "--config-root", "{home}/elsewhere")}},
		{name: "04-register-pi-when-it-is-not-installed", commands: [][]string{register("--target", "pi")}},
		{name: "05-register-an-unresolvable-state-directory", homeless: true, commands: [][]string{register("--target", "claude", "--config-root", "{home}", "--binary", "/opt/bin/longterm-mem")}},
		{name: "06-register-an-unresolvable-binary", homeless: true, commands: [][]string{register("--target", "claude", "--config-root", "{home}", "--state-dir", "{home}/state")}},

		// register: one target.
		{name: "07-register-claude", setup: registerFiles{claude: true}.put, commands: [][]string{register("--target", "claude")}},
		{name: "08-register-claude-twice", setup: registerFiles{claude: true}.put, commands: [][]string{register("--target", "claude"), register("--target", "claude")}},
		{name: "09-register-codex-and-opencode", setup: all.put, commands: [][]string{register("--target", "codex"), register("--target", "opencode")}},
		{name: "10-register-pi-when-it-is-installed", setup: installPi, commands: [][]string{register("--target", "pi")}},
		{name: "11-register-a-runtime-that-has-no-configuration", commands: [][]string{register("--target", "claude")}},
		{name: "12-register-over-a-config-that-cannot-be-read", setup: func(t *testing.T, w *registerWorld) { w.directory(t, ".claude.json") }, commands: [][]string{register("--target", "claude")}},
		{name: "13-register-over-an-entry-that-is-not-ours", setup: func(t *testing.T, w *registerWorld) { w.write(t, ".claude.json", registerForeignClaude) }, commands: [][]string{register("--target", "claude")}},
		{name: "14-register-with-every-path-given", setup: func(t *testing.T, w *registerWorld) { w.write(t, "elsewhere/.claude.json", registerEmptyClaude) }, commands: [][]string{register("--target", "claude", "--config-root", "{home}/elsewhere", "--state-dir", "{home}/state", "--binary", "/opt/bin/longterm-mem")}},

		// register all: what a machine has is registered, what it does not is skipped without a word of failure.
		{name: "15-register-all-on-a-machine-with-none", commands: [][]string{register("--target", "all")}},
		{name: "16-register-all-with-claude-and-codex", setup: registerFiles{claude: true, codex: true}.put, commands: [][]string{register("--target", "all")}},
		{name: "17-register-all-with-pi-installed", setup: func(t *testing.T, w *registerWorld) { all.put(t, w); installPi(t, w) }, commands: [][]string{register("--target", "all")}},
		{name: "18-register-all-with-pi-listed-but-not-built", setup: func(t *testing.T, w *registerWorld) {
			all.put(t, w)
			w.write(t, ".pi/agent/settings.json", `{"packages": ["`+filepath.Join(w.home, ".labdrian-overlay", "pi", "labdrian-pi")+`"]}`+"\n")
		}, commands: [][]string{register("--target", "all")}},

		// register all: one outcome among several decides the exit code, a failure before a path that could not be
		// resolved before a conflict, and every target is attempted whatever the others did.
		{name: "19-register-all-with-a-conflict", setup: func(t *testing.T, w *registerWorld) {
			all.put(t, w)
			w.write(t, ".claude.json", registerForeignClaude)
		}, commands: [][]string{register("--target", "all")}},
		{name: "20-register-all-with-a-conflict-and-a-failure", setup: func(t *testing.T, w *registerWorld) {
			all.put(t, w)
			w.write(t, ".claude.json", registerForeignClaude)
			w.write(t, ".config/opencode/opencode.json", `{"mcp": `)
		}, commands: [][]string{register("--target", "all")}},
		{name: "21-register-all-with-an-unresolvable-root-and-a-conflict", homeless: true, xdg: "{home}/xdg", setup: func(t *testing.T, w *registerWorld) {
			w.write(t, "xdg/opencode/opencode.json", registerForeignOpencode)
		}, commands: [][]string{register(withFlags("--target", "all")...)}},
		{name: "22-register-all-with-an-unresolvable-root-and-a-failure", homeless: true, xdg: "{home}/xdg", setup: func(t *testing.T, w *registerWorld) {
			w.write(t, "xdg/opencode/opencode.json", `{"mcp": `)
		}, commands: [][]string{register(withFlags("--target", "all")...)}},
		{name: "23-register-all-with-roots-that-cannot-be-resolved", homeless: true, commands: [][]string{register(withFlags("--target", "all")...)}},

		// unregister: refused before any target is touched, or with nothing to do.
		{name: "24-unregister-with-no-target", commands: [][]string{unregister()}},
		{name: "25-unregister-an-unknown-target", commands: [][]string{unregister("--target", "bogus")}},
		{name: "26-unregister-all-with-a-config-root", commands: [][]string{unregister("--target", "all", "--config-root", "{home}/elsewhere")}},
		{name: "27-unregister-an-unresolvable-state-directory", homeless: true, commands: [][]string{unregister("--target", "claude", "--config-root", "{home}")}},
		{name: "28-unregister-all-on-a-machine-with-none", commands: [][]string{unregister("--target", "all")}},
		{name: "29-unregister-all-on-a-machine-with-configs-and-no-entries", setup: all.put, commands: [][]string{unregister("--target", "all")}},

		// unregister: what register put is taken away, and what it did not put is left and reported.
		{name: "30-unregister-what-register-all-put", setup: all.put, commands: [][]string{register("--target", "all"), unregister("--target", "all")}},
		{name: "31-unregister-claude-twice", setup: registerFiles{claude: true}.put, commands: [][]string{register("--target", "claude"), unregister("--target", "claude"), unregister("--target", "claude")}},
		{name: "32-unregister-pi", setup: installPi, commands: [][]string{register("--target", "pi"), unregister("--target", "pi")}},
		{name: "33-unregister-an-entry-that-is-not-ours", setup: func(t *testing.T, w *registerWorld) { w.write(t, ".claude.json", registerForeignClaude) }, commands: [][]string{unregister("--target", "claude")}},
		{name: "34-unregister-all-with-an-entry-that-is-not-ours-and-a-failure", setup: func(t *testing.T, w *registerWorld) {
			w.write(t, ".claude.json", registerForeignClaude)
			w.directory(t, ".config/opencode/opencode.json")
		}, commands: [][]string{unregister("--target", "all")}},
		{name: "35-unregister-all-with-an-entry-that-is-not-ours", setup: func(t *testing.T, w *registerWorld) {
			all.put(t, w)
			w.write(t, ".claude.json", registerForeignClaude)
		}, commands: [][]string{unregister("--target", "all")}},
		{name: "36-unregister-all-with-roots-that-cannot-be-resolved", homeless: true, commands: [][]string{unregister("--target", "all", "--state-dir", "{home}/state")}},
		{name: "37-unregister-all-with-an-unresolvable-root-and-a-failure", homeless: true, xdg: "{home}/xdg", setup: func(t *testing.T, w *registerWorld) {
			w.directory(t, "xdg/opencode/opencode.json")
		}, commands: [][]string{unregister("--target", "all", "--state-dir", "{home}/state")}},
		{name: "38-unregister-claude-with-every-path-given", setup: func(t *testing.T, w *registerWorld) { w.write(t, "elsewhere/.claude.json", registerEmptyClaude) }, commands: [][]string{
			register("--target", "claude", "--config-root", "{home}/elsewhere", "--state-dir", "{home}/state", "--binary", "/opt/bin/longterm-mem"),
			unregister("--target", "claude", "--config-root", "{home}/elsewhere", "--state-dir", "{home}/state"),
		}},

		// pi registers in the package directory it is given, which is a configuration root of its own.
		{name: "39-register-pi-in-a-package-directory-that-is-given", setup: func(t *testing.T, w *registerWorld) {
			w.write(t, "package/mcp.json", registerEmptyPi)
			w.write(t, ".pi/agent/settings.json", `{"packages": ["`+filepath.Join(w.home, "package")+`"]}`+"\n")
		}, commands: [][]string{register("--target", "pi", "--config-root", "{home}/package")}},
		{name: "40-register-pi-in-a-package-directory-that-is-given-and-not-listed", setup: func(t *testing.T, w *registerWorld) {
			w.write(t, "package/mcp.json", registerEmptyPi)
		}, commands: [][]string{register("--target", "pi", "--config-root", "{home}/package")}},
		{name: "41-register-pi-when-the-package-directory-cannot-be-told", homeless: true, commands: [][]string{register(withFlags("--target", "pi")...)}},
	}
}

func TestRegisterGolden(t *testing.T) {
	goldenDir, err := filepath.Abs(filepath.Join("testdata", "register-golden"))
	if err != nil {
		t.Fatalf("resolve the golden directory: %v", err)
	}
	for _, scenario := range registerScenarios() {
		t.Run(scenario.name, func(t *testing.T) {
			fixedUmask(t)
			w := &registerWorld{home: realTempDir(t)}
			t.Setenv("HOME", w.home)
			t.Setenv("XDG_CONFIG_HOME", "")
			t.Setenv("CODEX_HOME", "")
			t.Setenv("STATE_DIR", "")
			if scenario.xdg != "" {
				t.Setenv("XDG_CONFIG_HOME", strings.ReplaceAll(scenario.xdg, "{home}", w.home))
			}
			t.Chdir(t.TempDir())
			if scenario.setup != nil {
				scenario.setup(t, w)
			}
			if scenario.homeless {
				t.Setenv("HOME", "")
			}

			var out strings.Builder
			for _, args := range scenario.commands {
				args = append([]string(nil), args...)
				for i := range args {
					args[i] = strings.ReplaceAll(args[i], "{home}", w.home)
				}
				var code int
				stdout, stderr := captureStreams(t, func() { code = run(args) })
				fmt.Fprintf(&out, "args: %s\nexit: %d\n--- stdout ---\n%s--- stderr ---\n%s", strings.Join(args, " "), code, stdout, stderr)
			}
			out.WriteString("--- machine ---\n" + renderPromoteGoldenTree(t, w.home))
			checkPromoteGolden(t, goldenDir, scenario.name, registerFingerprint.ReplaceAllString(strings.ReplaceAll(out.String(), w.home, "<home>"), `"fingerprint": "<sha256>"`))
		})
	}
}
