package settings

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestShellQuoteLeavesASafePathAloneAndQuotesTheRest(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"/opt/labdrian/bin/overlay", "/opt/labdrian/bin/overlay"},
		{"/home/a-b_c.d/bin/x+y", "/home/a-b_c.d/bin/x+y"},
		{"/p with space/bin", `'/p with space/bin'`},
		{"/p/it's/bin", `'/p/it'\''s/bin'`},
		{"/p/$HOME/bin", `'/p/$HOME/bin'`},
		{"", `''`},
	} {
		if got := shellQuote(c.in); got != c.want {
			t.Errorf("shellQuote(%q) = %s, want %s", c.in, got, c.want)
		}
	}
}

// trickyBinary is a path the unquoted commands could not run: a space and an apostrophe.
func trickyBinary(t *testing.T) (path, marker string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "my dir it's here")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	marker = filepath.Join(t.TempDir(), "ran")
	path = filepath.Join(dir, "overlay")
	script := "#!/bin/sh\necho ran >> " + shellQuote(marker) + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path, marker
}

// Every installed entry runs the binary when its path has a space and an apostrophe in it.
func TestEveryInstalledEntryRunsABinaryWhoseDirectoryHasASpaceAndAnApostrophe(t *testing.T) {
	for _, family := range installedFamilies {
		for _, spec := range family.specs {
			t.Run(family.label(spec), func(t *testing.T) {
				binary, marker := trickyBinary(t)
				entry := family.build(binary, spec)
				cmd := entry["hooks"].([]interface{})[0].(map[string]interface{})["command"].(string)

				run := exec.Command("sh", "-c", cmd)
				run.Env = append(os.Environ(), "CLAUDE_PROJECT_DIR="+t.TempDir())
				run.Stdin = strings.NewReader("{}")
				_ = run.Run() // the exit code is the hook's own; only that it ran matters here
				if _, err := os.Stat(marker); err != nil {
					t.Errorf("the binary did not run: %v\ncommand: %s", err, cmd)
				}
			})
		}
	}
}

// A path that needed no quoting is written exactly as before, so no installed machine drifts.
func TestAPathNeedingNoQuotesWritesTheCommandsAsBefore(t *testing.T) {
	for _, family := range installedFamilies {
		for _, spec := range family.specs {
			cmd := family.build(toyBinary, spec)["hooks"].([]interface{})[0].(map[string]interface{})["command"].(string)
			if !strings.Contains(cmd, "command -v "+toyBinary+" ") || strings.Contains(cmd, "'"+toyBinary) {
				t.Errorf("%s: %s", family.label(spec), cmd)
			}
		}
	}
}

// An entry written before the quoting names the path bare. It is still ours, and the next merge
// replaces it with the quoted one instead of adding a second entry.
func TestAnEntryWrittenWithTheBarePathIsOursAndIsReplacedByTheQuotedOne(t *testing.T) {
	const binary = "/my dir/it's here/overlay"
	for _, family := range installedFamilies {
		t.Run(family.identity, func(t *testing.T) {
			hooks := map[string]interface{}{}
			for _, spec := range family.specs {
				old := family.build(binary, spec)
				cmd := old["hooks"].([]interface{})[0].(map[string]interface{})["command"].(string)
				cmd = strings.ReplaceAll(cmd, shellQuote(binary), binary)
				old["hooks"].([]interface{})[0].(map[string]interface{})["command"] = cmd
				hooks[spec.event] = append(listOf(hooks[spec.event]), interface{}(old))
			}
			foreign := command("echo foreign")
			for event := range hooks {
				hooks[event] = append(listOf(hooks[event]), interface{}(foreign))
			}

			if !family.merge(hooks, binary) {
				t.Fatal("merge() reported no change for bare-path entries")
			}
			for _, event := range family.events() {
				got := listOf(hooks[event])
				owned, foreigners := 0, 0
				for _, e := range got {
					if family.owns(e, binary) {
						owned++
					} else if reflect.DeepEqual(e, interface{}(foreign)) {
						foreigners++
					}
				}
				if owned != len(family.specsFor(event)) || foreigners != 1 {
					t.Errorf("%s = %v: %d owned, %d foreign", event, got, owned, foreigners)
				}
			}
			if family.merge(hooks, binary) {
				t.Error("a second merge() changed the file")
			}
		})
	}
}

// Ownership is by substring, as uninstall always had it: an entry that runs a longer binary name
// but names our path and our identity is taken for ours. Uninstall removes it, and since the
// legacy families repair, a merge replaces it too. Pinned so the two cannot disagree.
func TestAnEntryNamingOurPathInALongerBinaryNameIsIsReplacedByMerge(t *testing.T) {
	lookalike := command(toyBinary + "-extra propagate --contract-file minimalism-contract.md")
	hooks := map[string]interface{}{"PreToolUse": list(lookalike)}

	if !minimalismFamily.owns(lookalike, toyBinary) {
		t.Fatal("the lookalike is not taken for ours, so this test no longer pins what it says")
	}
	if !minimalismFamily.merge(hooks, toyBinary) {
		t.Fatal("merge() left the lookalike")
	}
	for _, e := range listOf(hooks["PreToolUse"]) {
		if reflect.DeepEqual(e, interface{}(lookalike)) {
			t.Errorf("PreToolUse = %v: the lookalike is still there", hooks["PreToolUse"])
		}
	}
}

func listOf(v interface{}) []interface{} { l, _ := v.([]interface{}); return l }
