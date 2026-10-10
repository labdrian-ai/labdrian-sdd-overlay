//go:build unix

package main

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator"
)

// The golden files under testdata/status-golden record what 'status' (the doctor of an
// installation, which 'labdrian doctor' runs) prints and exits with, for the installations it
// can find: the binary, the hooks and guards in Claude's settings.json, the minimalism contract
// and the registry of the project it is run in. They were recorded from the program as it was
// before Phase 9 unit H30 (docs/architecture/hexagonal-target.md) moved what status decides
// out of cmd into the status package, and a change to a byte of any of them fails here.
// Rewrite them deliberately with
//
//	go test ./cmd -run TestStatusGolden -update-status-golden
//
// and read the diff before committing it.
//
// Each case runs the built program (the engine binary, built once for the whole test run) in a
// throwaway world whose home, project and PATH are inside it, with no other variable of the test
// process in its environment, so no case reads or writes anything of the machine. The world is a
// directory with a neutral name: the program tells its own hook entries apart by words in their
// commands, and a path that carried one would make a hook appear that is not there.
//
// What depends on the machine, and what is done about it. These tests are for Unix systems (the
// build tag): they use permission bits, symbolic links and the words the system gives for an error
// ("not a directory", "is a directory"), which are those of Linux and Darwin and are recorded as
// they are. The bits are the ones stat reports, so the cases hold for root too, and the cases that
// need a read to fail make a directory of the file, which fails for root as well. The words of the
// JSON library and the usage text of the program are not the verb's and change with the Go release
// or with a verb added elsewhere, so the transcript writes them as <JSON ERROR> and <USAGE>
// (hideForeignWords); the words around them stay pinned.
var updateStatusGolden = flag.Bool("update-status-golden", false, "rewrite the golden files of the status verb")

var (
	// jsonErrorText is what encoding/json says of a settings file or a manifest it cannot decode.
	jsonErrorText = regexp.MustCompile(`(?:json: )?cannot unmarshal \w+ into Go value of type [^\n]*?\{\}|invalid character '[^']*' [^\n]*?string|unexpected end of JSON input`)
	// usageText is the usage the program prints after a command line it refuses, up to the end of
	// the stderr it is in: the blank line that closes the record, before the next label or the end.
	usageText = regexp.MustCompile(`(?s)Usage:\n.*?\n\n(# |\z)`)
)

// hideForeignWords writes the words of the JSON library and the usage text of the program as
// placeholders, in a transcript.
func hideForeignWords(text string) string {
	text = jsonErrorText.ReplaceAllString(text, "<JSON ERROR>")
	return usageText.ReplaceAllString(text, "<USAGE>\n\n$1")
}

// Where the installation lives in a world, relative to the world.
const (
	worldHome       = "home"
	worldBinary     = "home/.claude/bin/gentle-ai-overlay"
	worldSettings   = "home/.claude/settings.json"
	worldContract   = "home/.claude/skills/_shared/minimalism-contract.md"
	projectRegistry = ".atl/skill-registry.md"
)

// newBinaryWorld makes a world for the built program: a directory of its own, a home in it, a
// PATH that holds nothing (no case starts a program, and none can reach a pi or git of the
// machine) and nothing else in the environment.
func newBinaryWorld(t *testing.T) *registryWorld {
	t.Helper()
	dir := neutralDir(t)
	w := &registryWorld{t: t, bin: reviewReceiptBinary(t), dir: dir, names: map[string]string{}, filter: hideForeignWords}
	w.name(dir, "<WORLD>")
	w.mkdir("nobin")
	w.mkdir(worldHome)
	w.env = []string{"HOME=" + w.path(worldHome), "PATH=" + w.path("nobin")}
	return w
}

// setupRun runs the program without recording it, and fails the case when it does not exit 0.
func (w *registryWorld) setupRun(cwd string, args ...string) {
	w.t.Helper()
	code, stdout, stderr, err := runWithin(registryRunTimeout, w.bin, w.path(cwd), w.env, args)
	if err != nil || code != 0 {
		w.t.Fatalf("setup %v: exit %d, error %v, stdout %q, stderr %q", args, code, err, stdout, stderr)
	}
}

// editJSON changes the JSON object in rel with edit and writes it back, indented.
func (w *registryWorld) editJSON(rel string, edit func(root map[string]any)) {
	w.t.Helper()
	var root map[string]any
	if err := json.Unmarshal([]byte(w.read(rel)), &root); err != nil {
		w.t.Fatalf("%s is not a JSON object: %v", rel, err)
	}
	edit(root)
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		w.t.Fatal(err)
	}
	w.put(rel, string(data))
}

// dropHooks removes, from the entries under event in a settings object, those whose commands
// contain every one of words.
func dropHooks(root map[string]any, event string, words ...string) {
	hooks, _ := root["hooks"].(map[string]any)
	entries, _ := hooks[event].([]any)
	var kept []any
	for _, e := range entries {
		if !strings.Contains(commandsOf(e), words[0]) || (len(words) > 1 && !strings.Contains(commandsOf(e), words[1])) {
			kept = append(kept, e)
		}
	}
	hooks[event] = kept
}

// commandsOf joins the commands of the inner hooks of one entry.
func commandsOf(entry any) string {
	var commands []string
	if em, ok := entry.(map[string]any); ok {
		inner, _ := em["hooks"].([]any)
		for _, ih := range inner {
			if m, ok := ih.(map[string]any); ok {
				if c, ok := m["command"].(string); ok {
					commands = append(commands, c)
				}
			}
		}
	}
	return strings.Join(commands, "\n")
}

// A well-formed contract and a registry that holds both scoped blocks.
const (
	statusContract = "---\napplies_to_phases: [sdd-tasks, sdd-apply]\n---\n# A contract\n\nThe body is not read.\n"
)

var statusRegistryBoth = "# Registry\n\n" + propagator.BeginMarker + "\nrow\n" + propagator.EndMarker + "\n" +
	propagator.AntiGenericDesignBeginMarker + "\nrow\n" + propagator.AntiGenericDesignEndMarker + "\n"

// complete puts a complete installation in the world: the hooks the installer writes, an
// executable binary, a contract that parses and a registry with both scoped blocks. status
// finds nothing wrong with it.
func (w *registryWorld) complete() {
	w.t.Helper()
	w.mkdir("home/.claude")
	w.setupRun(".", "merge-settings", "--settings", w.path(worldSettings), "--hook-command", w.path(worldBinary))
	w.putMode(worldBinary, "#!/bin/sh\n", 0o755)
	w.put(worldContract, statusContract)
	w.put(projectRegistry, statusRegistryBoth)
}

// status records one run of 'status' in the project directory of the world, saying what the
// run shows.
func (w *registryWorld) status(label string) {
	w.t.Helper()
	w.label("%s", label)
	w.run("status")
}

// settingsCases is the list of settings.json texts a case runs status over, each with the words
// that say what it is.
type settingsText struct{ label, text string }

func (w *registryWorld) statusOverSettings(variants []settingsText) {
	w.t.Helper()
	for _, v := range variants {
		w.put(worldSettings, v.text)
		w.status(v.label)
	}
}

func statusGoldenCases() []registryGoldenCase {
	return []registryGoldenCase{
		{"status-in-an-empty-home", func(w *registryWorld) {
			w.status("nothing is installed: every hard check fails, the registry of a project that has none is quiet")
		}},
		{"status-of-a-complete-install", func(w *registryWorld) {
			w.complete()
			w.status("everything in place: exit 0")
			w.mkdir("project/deeper")
			w.label("run from a directory with no registry of its own: quiet, and exit 0")
			w.runIn("project/deeper", "status")
			w.put("project/.atl/skill-registry.md", statusRegistryBoth)
			w.label("run from a project whose registry holds both blocks")
			w.runIn("project", "status")
		}},
		{"status-binary-states", func(w *registryWorld) {
			w.complete()
			// A file whose mode forbids writing is replaced, not written over.
			for _, step := range []struct {
				mode  os.FileMode
				label string
			}{
				{0o644, "the binary exists but has no execute bit: FAIL, exit 1"},
				{0o100, "the execute bit of the owner alone is enough"},
				{0o010, "the execute bit of the group alone is enough too: any bit counts"},
			} {
				w.removeAll(worldBinary)
				w.putMode(worldBinary, "#!/bin/sh\n", step.mode)
				w.status(step.label)
			}
			w.removeAll(worldBinary)
			w.mkdir(worldBinary)
			w.status("a directory in its place has execute bits, so it is OK")
			w.removeAll(worldBinary)
			w.symlink(w.path("nowhere"), worldBinary)
			w.status("a link to nothing is not found")
			w.removeAll(worldBinary)
			w.removeAll("home/.claude/bin")
			w.put("home/.claude/bin", "this is a file, not a directory\n")
			w.status("a file where the directory of the binary should be: the error of the system is the note")
			w.removeAll("home/.claude/bin")
			w.status("no binary and no directory: not found")
		}},
		{"status-settings-that-are-not-an-object", func(w *registryWorld) {
			w.complete()
			w.statusOverSettings([]settingsText{
				{"a file that is not JSON", "{not json"},
				{"an empty file", ""},
				{"only white space", " \n\t\n"},
				{"the JSON value null: it parses, and holds nothing", "null"},
				{"an array", "[]"},
				{"a string", `"text"`},
				{"a number", "7"},
				{"a truncated object", `{"hooks": {`},
			})
			w.removeAll(worldSettings)
			w.mkdir(worldSettings)
			w.status("a directory where the file should be: the file cannot be read")
		}},
		{"status-settings-without-the-hooks", func(w *registryWorld) {
			w.complete()
			bin := w.path(worldBinary)
			entry := func(matcher, command string) string {
				return `{"matcher":` + matcher + `,"hooks":[{"type":"command","command":"` + command + `"}]}`
			}
			w.statusOverSettings([]settingsText{
				{"an empty object", `{}`},
				{"hooks is null", `{"hooks":null}`},
				{"hooks is an array", `{"hooks":[]}`},
				{"hooks is a string", `{"hooks":"x"}`},
				{"hooks is an empty object: the keys are there, the entries are not", `{"hooks":{}}`},
				{"only entries of someone else", `{"hooks":{"UserPromptSubmit":[` + entry(`"x"`, "echo hi") + `],"PreToolUse":[` + entry(`"Agent"`, "echo hi") + `]}}`},
				{"entries that are not objects", `{"hooks":{"UserPromptSubmit":["x",1,null],"PreToolUse":[1,"Agent"]}}`},
				{"events that are not arrays", `{"hooks":{"UserPromptSubmit":{},"PreToolUse":"x","SessionEnd":3}}`},
				{"an inner hooks list that is not an array", `{"hooks":{"UserPromptSubmit":[{"hooks":"x"}],"PreToolUse":[{"matcher":"Agent","hooks":{}}]}}`},
				{"commands that are not strings", `{"hooks":{"UserPromptSubmit":[{"hooks":[{"command":7},null,"x"]}]}}`},
				{"the matcher is compared as it is written: agent and 5 are not Agent",
					`{"hooks":{"PreToolUse":[` + entry(`"agent"`, bin) + `,` + entry(`5`, bin) + `,` + entry(`null`, bin) + `]}}`},
				{"the binary in the two base hooks and nothing else: the other families are WARN",
					`{"hooks":{"UserPromptSubmit":[` + entry(`""`, bin+" propagate") + `],"PreToolUse":[` + entry(`"Agent"`, bin+" gate-task") + `]}}`},
				{"the binary in SessionEnd and in Bash, without the words of their families",
					`{"hooks":{"SessionEnd":[` + entry(`""`, bin+" other") + `],"PreToolUse":[` + entry(`"Bash"`, bin+" other") + `]}}`},
				{"the words of the families without the binary",
					`{"hooks":{"SessionEnd":[` + entry(`""`, "other sync-trigger") + `],"PreToolUse":[` + entry(`"Bash"`, "other review-receipt") + `]}}`},
				{"the binary and the words in different inner hooks of one entry",
					`{"hooks":{"SessionEnd":[{"hooks":[{"command":"` + bin + `"},{"command":"sync-trigger"}]}]}}`},
			})
		}},
		{"status-families-missing-or-drifted", func(w *registryWorld) {
			w.complete()
			full := w.read(worldSettings)
			variant := func(label string, edit func(root map[string]any)) {
				w.put(worldSettings, full)
				w.editJSON(worldSettings, edit)
				w.status(label)
			}
			variant("no SessionEnd entry: WARN", func(r map[string]any) { delete(r["hooks"].(map[string]any), "SessionEnd") })
			variant("no review-receipt entry: WARN", func(r map[string]any) { dropHooks(r, "PreToolUse", "review-receipt") })
			variant("no deny rule: the shaper guard is WARN", func(r map[string]any) { delete(r, "permissions") })
			variant("hooks are disabled altogether: the guards say so", func(r map[string]any) { r["disableAllHooks"] = true })
			variant("hooks are not disabled when the key is false", func(r map[string]any) { r["disableAllHooks"] = false })
			variant("no shaper guard entries", func(r map[string]any) { dropHooks(r, "PreToolUse", "shaper guard-hook") })
			variant("no projection entries", func(r map[string]any) {
				dropHooks(r, "PreToolUse", "projection hook")
				dropHooks(r, "UserPromptSubmit", "projection hook")
			})
			variant("a projection entry that was edited", func(r map[string]any) {
				entries := r["hooks"].(map[string]any)["UserPromptSubmit"].([]any)
				for _, e := range entries {
					if strings.Contains(commandsOf(e), "projection hook") {
						e.(map[string]any)["hooks"].([]any)[0].(map[string]any)["command"] = commandsOf(e) + " # edited"
					}
				}
			})
			variant("a projection entry twice", func(r map[string]any) {
				hooks := r["hooks"].(map[string]any)
				entries := hooks["UserPromptSubmit"].([]any)
				hooks["UserPromptSubmit"] = append(entries, entries[len(entries)-1])
			})
			variant("no approve guard entries", func(r map[string]any) { dropHooks(r, "PreToolUse", "skills guard-hook") })
			variant("an approve guard entry that was edited", func(r map[string]any) {
				for _, e := range r["hooks"].(map[string]any)["PreToolUse"].([]any) {
					if strings.Contains(commandsOf(e), "skills guard-hook") {
						e.(map[string]any)["hooks"].([]any)[0].(map[string]any)["command"] = commandsOf(e) + " # edited"
					}
				}
			})
			variant("no gate-task entries for Agent: FAIL", func(r map[string]any) { dropHooks(r, "PreToolUse", "gate-task") })
			variant("no UserPromptSubmit key at all: FAIL", func(r map[string]any) { delete(r["hooks"].(map[string]any), "UserPromptSubmit") })
		}},
		{"status-contract-states", func(w *registryWorld) {
			w.complete()
			set := func(label, text string) {
				w.put(worldContract, text)
				w.status(label)
			}
			set("an empty contract", "")
			set("no frontmatter", "# A contract\n")
			set("frontmatter that is not closed", "---\napplies_to_phases: [a]\n")
			set("no applies_to_phases", "---\nname: x\n---\nbody\n")
			set("an empty list of phases", "---\napplies_to_phases: []\n---\nbody\n")
			set("phases without brackets", "---\napplies_to_phases: sdd-tasks\n---\nbody\n")
			set("the other keys are not read", "---\napplies_to_phases: [sdd-tasks]\nactivation: nonsense\n---\nbody\n")
			w.removeAll(worldContract)
			w.mkdir(worldContract)
			w.status("a directory where the contract should be: it cannot be read")
			w.removeAll(worldContract)
			w.status("no contract: FAIL")
		}},
		{"status-registry-states", func(w *registryWorld) {
			w.complete()
			set := func(label, text string) {
				w.put(projectRegistry, text)
				w.status(label)
			}
			set("an empty registry: FAIL, and the note says an empty registry is not zero skills", "")
			set("only white space: the same", " \n\t\n")
			set("text with neither block: WARN names both", "# Registry\n")
			set("only the minimalism block: WARN names the other", "# Registry\n"+propagator.BeginMarker+"\n")
			set("only the design block: WARN names the other", "# Registry\n"+propagator.AntiGenericDesignBeginMarker+"\n")
			set("the begin markers are enough, the end markers are not read", propagator.BeginMarker+propagator.AntiGenericDesignBeginMarker)
			set("both blocks", statusRegistryBoth)
			w.removeAll(projectRegistry)
			w.mkdir(projectRegistry)
			w.status("a directory where the registry should be: it cannot be read, FAIL")
			w.removeAll(projectRegistry)
			w.status("no registry: quiet")
		}},
		{"status-with-no-home", func(w *registryWorld) {
			// Without a home the places are relative, so they are the world's own: the program is
			// started in the world and finds .claude there.
			w.setenv("HOME", "")
			w.status("HOME is empty and nothing is installed under the project directory")
			w.mkdir(".claude")
			w.setupRun(".", "merge-settings", "--settings", ".claude/settings.json", "--hook-command", ".claude/bin/gentle-ai-overlay")
			w.putMode(".claude/bin/gentle-ai-overlay", "#!/bin/sh\n", 0o755)
			w.put(".claude/skills/_shared/minimalism-contract.md", statusContract)
			w.put(projectRegistry, statusRegistryBoth)
			w.status("HOME is empty and the installation is under the project directory: exit 0")
			w.env = []string{"PATH=" + w.path("nobin")}
			w.status("HOME is not set at all: the same")
		}},
	}
}

// TestStatusGolden runs every case and compares its transcript with its golden file.
func TestStatusGolden(t *testing.T) {
	seen := map[string]bool{}
	for _, tc := range statusGoldenCases() {
		if seen[tc.name] {
			t.Errorf("two cases are named %q", tc.name)
		}
		seen[tc.name] = true
		t.Run(tc.name, func(t *testing.T) {
			w := newBinaryWorld(t)
			tc.run(w)
			checkGoldenIn(t, "status-golden", tc.name, w.text(), updateStatusGolden, "-update-status-golden")
		})
	}
	entries, err := os.ReadDir(filepath.Join("testdata", "status-golden"))
	if err != nil {
		// Only the recording itself may find the directory missing; a directory that was deleted
		// or misnamed must fail and not skip.
		if *updateStatusGolden && os.IsNotExist(err) {
			return
		}
		t.Fatalf("the golden directory testdata/status-golden cannot be read: %v", err)
	}
	for _, e := range entries {
		if name := strings.TrimSuffix(e.Name(), ".golden"); !seen[name] {
			t.Errorf("testdata/status-golden/%s belongs to no case", e.Name())
		}
	}
}
