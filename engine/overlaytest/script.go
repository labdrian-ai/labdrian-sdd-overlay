package overlaytest

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The script is not one file. bin/labdrian-overlay is the entry point: it finds itself, sources the
// modules in bin/lib/ in the order its _modules array lists them, and dispatches the command. A test that
// runs the script needs none of this (it runs the entry point and the entry point does the rest), but a
// test that reads what the script says, or copies it, has to take the modules with it, and takes them from
// here, so that the list of them is the script's own and no test keeps one.

// modulesArray finds the array of module names in the entry point: one name to a line.
var modulesArray = regexp.MustCompile(`(?s)\n_modules=\(\n(.*?)\n\)\n`)

// LibDir is the directory of the modules of the script at script: lib/ beside it.
func LibDir(script string) string {
	return filepath.Join(filepath.Dir(script), "lib")
}

// ModulePath is the file of the module called name, for the script at script.
func ModulePath(script, name string) string {
	return filepath.Join(LibDir(script), name+".sh")
}

// ScriptModules is the names of the modules the entry point at script sources, in the order it sources
// them: the words of its _modules array. An entry point with no such array has none.
func ScriptModules(t testing.TB, script string) []string {
	t.Helper()
	text, err := os.ReadFile(script)
	if err != nil {
		t.Fatalf("read the entry point of the script: %v", err)
	}
	m := modulesArray.FindSubmatch(text)
	if m == nil {
		return nil
	}
	return strings.Fields(string(m[1]))
}

// ScriptFilesOf is the files that make up the script at script: the entry point itself, then the file of
// each module it sources, in that order. It fails the test when a module the entry point lists is not
// there, as the script itself does when it is run.
func ScriptFilesOf(t testing.TB, script string) []string {
	t.Helper()
	files := []string{script}
	for _, name := range ScriptModules(t, script) {
		path := ModulePath(script, name)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("the entry point lists the module %q and %s is not there: %v", name, path, err)
		}
		files = append(files, path)
	}
	return files
}

// ScriptFiles is ScriptFilesOf for the script of this checkout.
func ScriptFiles(t testing.TB) []string {
	t.Helper()
	return ScriptFilesOf(t, Script(t))
}

// ScriptTextOf is what the script at script says: the text of ScriptFilesOf one after the other.
func ScriptTextOf(t testing.TB, script string) string {
	t.Helper()
	var b strings.Builder
	for _, path := range ScriptFilesOf(t, script) {
		text, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		b.Write(text)
		if len(text) > 0 && text[len(text)-1] != '\n' {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// ScriptText is what the script of this checkout says. A test that looks for a function, a message or a
// call in the script looks here and not in the entry point alone, which holds the loader and the dispatch
// and little else.
func ScriptText(t testing.TB) string {
	t.Helper()
	return ScriptTextOf(t, Script(t))
}

// CopyScriptOf copies the script at src, the entry point and the modules it lists, to bin/ under root, as
// they stand in a clone, and returns the path of the copy of the entry point. The copy is a script a test
// may break (take a module away) or run from another place.
func CopyScriptOf(t testing.TB, src, root string) string {
	t.Helper()
	dst := filepath.Join(root, "bin", filepath.Base(src))
	for _, from := range ScriptFilesOf(t, src) {
		rel, err := filepath.Rel(filepath.Dir(src), from)
		if err != nil {
			t.Fatalf("path of %s below the script: %v", from, err)
		}
		to := filepath.Join(filepath.Dir(dst), rel)
		data, err := os.ReadFile(from)
		if err != nil {
			t.Fatalf("read %s: %v", from, err)
		}
		info, err := os.Stat(from)
		if err != nil {
			t.Fatalf("stat %s: %v", from, err)
		}
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(to), err)
		}
		if err := os.WriteFile(to, data, info.Mode().Perm()); err != nil {
			t.Fatalf("write %s: %v", to, err)
		}
	}
	return dst
}

// CopyScript is CopyScriptOf for the script of this checkout.
func CopyScript(t testing.TB, root string) string {
	t.Helper()
	return CopyScriptOf(t, Script(t), root)
}
