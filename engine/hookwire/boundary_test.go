package hookwire_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The hook format has one home. Every JSON tag of a field of the hook protocol, the ones Claude
// Code sends and the ones it reads, is a struct tag of engine/hookwire and of no other
// package: a policy that decodes a hook input itself, or builds a hook answer itself, is the
// wire format in a policy, which is what Phase 9 unit H14 removed. This test reads the
// production code of the module and fails on a tag of the protocol anywhere else.
//
// The tags are the ones that exist only in the protocol. "command" is not among them: it is
// the name of a field of a Bash input, but also of the entries of a settings file, which are
// not the hook format.
var protocolTags = []string{
	"hook_event_name", "tool_name", "tool_input", "tool_use_id", "subagent_type", "file_path", "notebook_path",
	"hookEventName", "hookSpecificOutput", "permissionDecision", "permissionDecisionReason",
	"additionalContext", "systemMessage", "updatedInput",
}

var protocolTag = regexp.MustCompile("json:\"(" + strings.Join(protocolTags, "|") + ")[\",]")

// owedWire lists the files that still hold a tag of the protocol outside hookwire, each owed to
// the slice of H14 that moves it. It only shrinks: the slice that moves a file deletes its row,
// and a row whose file no longer holds a tag fails the test, so none can be left behind. A new
// tag is not added here to make the test pass; it is written in hookwire.
var owedWire = map[string]string{
	"reviewreceipt/hook.go":   "H14 slice 2",
	"shaper/guard.go":         "H14 slice 2",
	"skills/approve_guard.go": "H14 slice 2",
}

func TestNoHookWireFormatOutsideHookwire(t *testing.T) {
	root := ".."
	found := map[string][]string{}
	checked := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "hookwire", "testdata", ".git", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		checked++
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		for i, line := range strings.Split(string(data), "\n") {
			if protocolTag.MatchString(line) {
				found[rel] = append(found[rel], strings.TrimSpace(line)+"  (line "+strconv.Itoa(i+1)+")")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 50 {
		t.Fatalf("only %d source files were read: the walk is broken", checked)
	}

	files := make([]string, 0, len(found))
	for f := range found {
		files = append(files, f)
	}
	sort.Strings(files)
	for _, f := range files {
		if _, owed := owedWire[f]; !owed {
			t.Errorf("%s holds a tag of the hook protocol outside engine/hookwire:\n\t%s\nthe hook format is written in hookwire only", f, strings.Join(found[f], "\n\t"))
		}
	}
	for f, unit := range owedWire {
		if _, still := found[f]; !still {
			t.Errorf("owedWire lists %s (owed to %s), which holds no tag of the hook protocol any more: delete the row", f, unit)
		}
	}
}

// hookwire is the protocol and nothing of a policy: it imports no package of the module, so a
// policy can change without it and a runtime with another hook format can be given another
// adapter with the same policies. engine/cmd maps its values to those of each policy.
func TestHookwireImportsNothingOfTheModule(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		checked++
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		for _, imp := range parsed.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(path, "github.com/labdrian-ai/") {
				t.Errorf("%s imports %s: hookwire knows no package of the module", file, path)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no source file of hookwire was checked")
	}
}
