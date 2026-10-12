package skillstalefs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pathOf joins directories the way a PATH is written on this platform.
func pathOf(dirs ...string) string {
	return strings.Join(dirs, string(os.PathListSeparator))
}

func TestResolvesCommand_FindsAFileInAnyDirectoryOfThePath(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	writeProjectFile(t, second, "tool", "#!/bin/sh\n")

	if !(PathCommands{PathEnv: pathOf(first, second)}).ResolvesCommand("tool") {
		t.Error("a file in the second directory of the PATH was not found")
	}
	if (PathCommands{PathEnv: pathOf(first)}).ResolvesCommand("tool") {
		t.Error("a file that is in no directory of the PATH was found")
	}
}

func TestResolvesCommand_DoesNotCountADirectoryOfThatName(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "tool"), 0o755); err != nil {
		t.Fatal(err)
	}

	if (PathCommands{PathEnv: dir}).ResolvesCommand("tool") {
		t.Error("a directory named like the command was taken for the command")
	}
}

// An empty command has nothing to look for, so it is never reported missing.
func TestResolvesCommand_AnEmptyCommandResolves(t *testing.T) {
	if !(PathCommands{PathEnv: t.TempDir()}).ResolvesCommand("") {
		t.Error("an empty command was reported missing")
	}
}

// An empty entry of a PATH, and an empty PATH, mean the working directory, as a shell reads them.
func TestResolvesCommand_AnEmptyEntryIsTheWorkingDirectory(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)
	writeProjectFile(t, cwd, "tool", "#!/bin/sh\n")

	for name, pathEnv := range map[string]string{"empty PATH": "", "empty entry": pathOf(t.TempDir(), "")} {
		if !(PathCommands{PathEnv: pathEnv}).ResolvesCommand("tool") {
			t.Errorf("%s: a file in the working directory was not found", name)
		}
	}
}

// The command is a path under each directory: one with a separator in it is looked for below them.
func TestResolvesCommand_ANameWithASeparatorIsLookedForBelowEachDirectory(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "bin/tool", "#!/bin/sh\n")

	if !(PathCommands{PathEnv: dir}).ResolvesCommand("bin/tool") {
		t.Error("bin/tool below a directory of the PATH was not found")
	}
}
