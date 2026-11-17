package runtime_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The tests stub the modules OpenCode hands a plugin, and a stub that exports something the real
// module does not hides a plugin that cannot load (the 2.2.7 `import { Tool }`, which the real
// module never exported). OpenCode 2.2.7's @opencode/plugin/promise/tool exports exactly CallID and
// Error (probed in /home/labdrian/.cache/t15-oc-T9SG/probe-exports.json); every stub of it in the
// engine's tests must export those names and no others.
var (
	toolStubLine   = regexp.MustCompile(`"tool\.js":\s*` + "`([^`]*)`")
	exportedName   = regexp.MustCompile(`\bexport\s+(?:class|const|let|var|function)\s+([A-Za-z_$][\w$]*)`)
	realToolExport = []string{"CallID", "Error"}
)

func TestEveryStubOfThePluginToolModuleExportsExactlyWhatTheRealModuleDoes(t *testing.T) {
	var stubs int
	err := filepath.WalkDir("..", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range toolStubLine.FindAllStringSubmatch(string(source), -1) {
			stubs++
			var got []string
			for _, name := range exportedName.FindAllStringSubmatch(match[1], -1) {
				got = append(got, name[1])
			}
			sort.Strings(got)
			if strings.Join(got, ",") != strings.Join(realToolExport, ",") {
				t.Errorf("%s stubs @opencode/plugin/promise/tool with exports %v, want %v", path, got, realToolExport)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if stubs < 2 {
		t.Errorf("found %d stubs of the tool module under engine/, want the two known ones (runtime and cmd tests): the guard no longer sees them", stubs)
	}
}
