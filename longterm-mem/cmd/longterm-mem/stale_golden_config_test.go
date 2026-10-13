package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
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

// A repository whose own config file is not there has nothing an entry could be said to come from: every entry
// is foreign, including one that names the path the file would have. (The path of an entry that does not exist
// cannot be resolved either, so no entry could have matched a file that is not there.)
func TestForeignOrigins_NoConfigFileOfItsOwnMeansEveryEntryIsForeign(t *testing.T) {
	root := realTempDir(t)
	out := "file:.git/config\tcore.bare=false\n" +
		"file:" + filepath.Join(root, ".git", "config") + "\tcore.filemode=true\n"

	got, entries := foreignOrigins(out, root)

	want := []string{"file:.git/config\tcore.bare=false", "file:" + filepath.Join(root, ".git", "config") + "\tcore.filemode=true"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("foreignOrigins = %q, want %q", got, want)
	}
	if entries != 2 {
		t.Errorf("foreignOrigins counted %d entries, want 2", entries)
	}
}

// Git writes a path with a quote, a backslash or a control character in it between double quotes, with a
// backslash before each (the way it quotes a name in `git status`). A repository in such a directory still has
// the file as its own.
func TestForeignOrigins_ReadsAPathGitQuotes(t *testing.T) {
	root := filepath.Join(realTempDir(t), `a "quoted" dir`)
	own := filepath.Join(root, ".git", "config")
	if err := os.MkdirAll(filepath.Dir(own), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(own, []byte("[core]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := "file:" + strconv.Quote(own) + "\tcore.bare=false\n" +
		"file:\"/home/some\\tbody/.gitconfig\"\tuser.name=somebody\n"

	got, entries := foreignOrigins(out, root)

	want := []string{"file:\"/home/some\\tbody/.gitconfig\"\tuser.name=somebody"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("foreignOrigins = %q, want %q", got, want)
	}
	if entries != 2 {
		t.Errorf("foreignOrigins counted %d entries, want 2", entries)
	}
}

// The way the world asks when git cannot say the scope of an entry (before 2.26) is run here against the git of
// the host, end to end: the listing git makes of the world with --show-origin, read as the helper reads it,
// finds the same entries the scopes do, and none of them foreign. Before this the fallback was only ever
// exercised on text.
func TestStaleWorld_TheOriginsOfTheConfigurationAgreeWithItsScopes(t *testing.T) {
	w := newSkillsStaleWorld(t, true)
	w.history(t)

	byScope, scopeEntries := nonLocalScopes(w.gitOutput(t, "config", "--list", "--show-scope"))
	byOrigin, originEntries := w.foreignConfigByOrigin(t)

	if originEntries == 0 {
		t.Fatal("git lists no entry with --show-origin: the comparison would prove nothing")
	}
	if originEntries != scopeEntries {
		t.Errorf("--show-origin lists %d entries, --show-scope %d", originEntries, scopeEntries)
	}
	if len(byScope) != 0 || len(byOrigin) != 0 {
		t.Errorf("foreign entries: by scope %q, by origin %q; want none", byScope, byOrigin)
	}
}
