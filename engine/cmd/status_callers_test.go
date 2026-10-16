package main

// `status` reads its arguments strictly (parseStatusArgs): what it does not know is a usage error. So
// the callers of the verb in this repository must pass it only what it accepts, and a caller that is
// added, or one whose arguments change, must be seen. This test holds the callers there are, each with
// the exact arguments it passes, and fails on a call it does not know.
//
// The callers of the engine's `status` in the repository (Phase 9, batch 46):
//
//   - bin/labdrian-overlay, status-hooks: `exec "$ENGINE_BINARY" status`, with no arguments;
//   - bin/labdrian-overlay, doctor (doctor_report_registry): `"$ENGINE_BINARY" status --only registry
//     --cwd "$dir"`, the registry of one project;
//   - the tests of this package, which call runStatus or run the built binary with `status` and the
//     arguments they mean to try (status_run_test.go, status_golden_test.go, dispatch_routes_test.go);
//   - nothing else: the TUI runs the verbs of the overlay script (`status`, which is the script's own
//     verb for the targets, and `status-hooks`), never the engine; the hooks that install-hooks writes
//     into settings.json call propagate, gate-task, sync-trigger, review-receipt, shaper, projection and
//     skills guard-hook, not status; tools/, .github/, pi/, skills/ and agents/ do not call it.

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// statusCallInScript finds the arguments of each call of the engine's status verb in the overlay
// script: the words after `"$ENGINE_BINARY" status` up to the end of the command (a closing
// parenthesis, a pipe, the end of the line), without a redirection of a stream.
var statusCallInScript = regexp.MustCompile(`"\$ENGINE_BINARY" status((?: [^\s)|]+)*)`)

func TestEveryCallerOfStatusInTheRepositoryPassesOnlyWhatItAccepts(t *testing.T) {
	var found []string
	for _, m := range statusCallInScript.FindAllStringSubmatch(overlaytest.ScriptText(t), -1) {
		var words []string
		for _, w := range strings.Fields(m[1]) {
			if !strings.Contains(w, ">") { // 2>&1 is the script's, not an argument
				words = append(words, w)
			}
		}
		found = append(found, strings.Join(words, " "))
	}
	// The calls the script makes: status-hooks passes nothing and doctor the registry. Their order is the order
	// of the modules that make them, which is nobody's business here.
	sort.Strings(found)
	want := []string{``, `--only registry --cwd "$dir"`}
	if !reflect.DeepEqual(found, want) {
		t.Fatalf("the overlay script calls the engine's status with %q, want %q: a caller was added or changed, so add it to this test (and to the ledger) and check that status accepts what it passes", found, want)
	}

	for _, call := range found {
		dir := t.TempDir()
		args := strings.Fields(strings.ReplaceAll(call, `"$dir"`, dir))
		got, err := parseStatusArgs(args)
		if err != nil {
			t.Errorf("status refuses the arguments %q that the overlay script passes it: %v", call, err)
			continue
		}
		if call == "" && got != (statusArgs{}) {
			t.Errorf("status-hooks passes no argument and status read %+v", got)
		}
		if call != "" && got != (statusArgs{only: onlyRegistry, cwd: dir}) {
			t.Errorf("the doctor passes %q and status read %+v, want the registry of %s", call, got, dir)
		}
	}
}

// Nothing else in the repository calls the engine's status verb: the places that could (the TUI, the
// tools, the workflows of CI, the Pi package, the skills and agents, the other module) are read for a
// line that names the engine and then status.
func TestNoOtherPlaceInTheRepositoryCallsTheEnginesStatus(t *testing.T) {
	call := regexp.MustCompile(`(gentle-ai-overlay|ENGINE_BINARY"?|\bengine)\s+status\b`)
	root := filepath.Join("..", "..")
	for _, dir := range []string{"tui", "tools", ".github", "pi", "skills", "agents", "longterm-mem", "identity", "prespec-malandra"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // a place the repository does not have
			}
			if d.IsDir() {
				if d.Name() == "node_modules" || d.Name() == "testdata" || d.Name() == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			if info, err := d.Info(); err != nil || info.Size() > 1<<20 {
				return nil
			}
			switch filepath.Ext(path) {
			case ".go", ".sh", ".ts", ".js", ".md", ".yml", ".yaml", ".json", ".toml":
			default:
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			isDoc := filepath.Ext(path) == ".md"
			for i, line := range strings.Split(string(body), "\n") {
				// A comment in code that speaks of the verb (the TUI says what exit 2 of it means) is no
				// call; a line of a document that shows one is a call to know.
				if t := strings.TrimSpace(line); !isDoc && (strings.HasPrefix(t, "//") || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "*")) {
					continue
				}
				if call.MatchString(line) {
					t.Errorf("%s:%d calls (or documents a call of) the engine's status: %q; add it to the callers of status_callers_test.go", path, i+1, strings.TrimSpace(line))
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
