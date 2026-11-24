package main

// Every step of the role-chain skill that a command's text sends the reader to is a line of the skill: the heading
// `## <Name>` or a line that starts `- <Name>, ` (T4b, `4b9c3789c2e0-4.polisher`, L86 choice 2, widened by L90 choice 6 from
// the stop hook's reason to every string of the commands; the bullet form by L94, because the Classify step is one line of
// Binding). The stop hook asked for "the Classify step of the role-chain skill" while no such step existed, and no check
// tied the words to the skill: this is the known-error check of the class. It reads the string literals of the non-test
// files of the package, not the reasons they make, so a step named in any branch of any text is held.
//
// Bound: a phrase split across two concatenated literals is not read. The one phrase today sits in one literal, and a
// literal that keeps `role-chain skill` without the phrase fails, so a reworded reference cannot slip past.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestEveryRoleChainStepTheCommandsNameIsALineOfTheSkill(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	phrase := regexp.MustCompile(`the (\w+) step of the role-chain skill`)
	named := map[string]string{} // step name -> a file that names it
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(parsed, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			text, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Fatalf("%s: %v", file, err)
			}
			if !strings.Contains(text, "role-chain skill") {
				return true
			}
			matches := phrase.FindAllStringSubmatch(text, -1)
			if len(matches) == 0 {
				t.Errorf("%s: a string says %q and names no step as %q: %q", file, "role-chain skill", "the <Name> step of the role-chain skill", text)
			}
			for _, m := range matches {
				named[m[1]] = file
			}
			return true
		})
	}
	if len(named) == 0 {
		t.Fatal("no string of the commands names a step of the role-chain skill, so this check reads nothing")
	}
	skill, err := os.ReadFile(filepath.Join("..", "..", "skills", "role-chain", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(skill), "\n")
	for name, file := range named {
		found := false
		for _, line := range lines {
			if line == "## "+name || strings.HasPrefix(line, "- "+name+", ") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s names the %s step of the role-chain skill, and skills/role-chain/SKILL.md has no line `## %s` and none that starts `- %s, `", file, name, name, name)
		}
	}
}
