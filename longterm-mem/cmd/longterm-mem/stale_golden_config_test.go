package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// `git config --list --show-scope` prints "<scope>\t<key>=<value>": every line of a pinned world is local.
func TestNonLocalScopes_NamesEveryEntryThatIsNotTheRepositorys(t *testing.T) {
	out := "local\tcore.bare=false\nglobal\tuser.name=somebody\nsystem\tinit.defaultbranch=trunk\ncommand\tcommit.gpgsign=false\nlocal\tcore.filemode=true\n"

	got, entries := nonLocalScopes(out)

	want := []string{"global\tuser.name=somebody", "system\tinit.defaultbranch=trunk", "command\tcommit.gpgsign=false"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("nonLocalScopes = %q, want %q", got, want)
	}
	if entries != 5 {
		t.Errorf("nonLocalScopes counted %d entries, want 5", entries)
	}
	if got, entries := nonLocalScopes("local\tcore.bare=false\n"); len(got) != 0 || entries != 1 {
		t.Errorf("nonLocalScopes of a local entry = %q, %d; want none of them foreign, and one entry", got, entries)
	}
}

// A listing with no entry counts none: the world sets entries of its own, so that is what a git that was not
// asked, or a directory that is no repository, looks like, and the world's test fails on it.
func TestConfigListings_CountNothingForNothing(t *testing.T) {
	if got, entries := nonLocalScopes(""); len(got) != 0 || entries != 0 {
		t.Errorf("nonLocalScopes of nothing = %q, %d; want none", got, entries)
	}
	if got, entries := foreignOrigins("\n", t.TempDir()); len(got) != 0 || entries != 0 {
		t.Errorf("foreignOrigins of nothing = %q, %d; want none", got, entries)
	}
}

// `--show-origin` names the file an entry comes from as git writes it: relative to where git ran or absolute,
// whichever the version and the platform choose. Both are the repository's own file when they resolve to it.
func TestForeignOrigins_ResolvesTheFileAsGitNamesIt(t *testing.T) {
	root := realTempDir(t)
	own := filepath.Join(root, ".git", "config")
	if err := os.MkdirAll(filepath.Dir(own), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(own, []byte("[core]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := "file:.git/config\tcore.bare=false\n" +
		"file:" + own + "\tcore.filemode=true\n" +
		"file:/home/somebody/.gitconfig\tuser.name=somebody\n" +
		"file:/etc/gitconfig\tinit.defaultbranch=trunk\n" +
		"command line:\tcommit.gpgsign=false\n"

	got, entries := foreignOrigins(out, root)

	want := []string{
		"file:/home/somebody/.gitconfig\tuser.name=somebody",
		"file:/etc/gitconfig\tinit.defaultbranch=trunk",
		"command line:\tcommit.gpgsign=false",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("foreignOrigins = %q, want %q", got, want)
	}
	if entries != 5 {
		t.Errorf("foreignOrigins counted %d entries, want 5", entries)
	}
}
