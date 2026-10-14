package projectidfs_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/projectid"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/projectidfs"
)

// These cases ask the adapter its three questions directly, over repositories laid out by hand (the files git
// keeps, written by the test, with no git process): what Repository it hands the rules for each layout, what it
// says of a declaration and of a configuration, and the words of its errors. That the rules then give the same
// identity from a main checkout and from a worktree git itself made is shown by the other files of this package.

// realTree is a fresh temporary directory named by its resolved path (the adapter answers in resolved paths).
func realTree(t *testing.T) string {
	t.Helper()
	real, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return real
}

func mkdir(t *testing.T, parts ...string) string {
	t.Helper()
	path := filepath.Join(parts...)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLocate_AMainCheckoutIsItsOwnMainRootWithItsGitDirectoryAsCommonDir(t *testing.T) {
	root := realTree(t)
	mkdir(t, root, ".git")
	below := mkdir(t, root, "module", "promote")

	for _, dir := range []string{root, below} {
		got, err := projectidfs.Inspector{}.Locate(dir)

		want := projectid.Repository{WorktreeRoot: root, MainWorktreeRoot: root, CommonDir: filepath.Join(root, ".git")}
		if err != nil || got != want {
			t.Errorf("Locate(%s) = %+v, %v; want %+v", dir, got, err, want)
		}
	}
}

// A .git that is a LINK to the real git directory is a layout git supports. The working tree is where the link is
// (not where the git directory is), which is where the declaration is read from; the common directory is the
// directory the link leads to, resolved.
func TestLocate_AGitDirectoryThatIsALinkKeepsTheMainRootWhereTheLinkIs(t *testing.T) {
	root := realTree(t)
	repo := mkdir(t, root, "repo")
	elsewhere := mkdir(t, root, "elsewhere", "git-directory")
	if err := os.Symlink(elsewhere, filepath.Join(repo, ".git")); err != nil {
		t.Fatal(err)
	}

	got, err := projectidfs.Inspector{}.Locate(repo)

	want := projectid.Repository{WorktreeRoot: repo, MainWorktreeRoot: repo, CommonDir: elsewhere}
	if err != nil || got != want {
		t.Errorf("Locate = %+v, %v; want %+v", got, err, want)
	}
}

// A link that leads nowhere is no .git at all: the walk goes on above it, as it does for a directory with none.
func TestLocate_ADanglingGitLinkIsNoRepository(t *testing.T) {
	dir := realTree(t)
	if err := os.Symlink(filepath.Join(dir, "gone"), filepath.Join(dir, ".git")); err != nil {
		t.Fatal(err)
	}

	_, err := projectidfs.Inspector{}.Locate(dir)

	if !errors.Is(err, projectid.ErrNotARepository) {
		t.Errorf("Locate = %v, want ErrNotARepository", err)
	}
}

// A linked worktree's .git is a file naming the worktree's own git directory, whose commondir file points back
// at the shared one RELATIVELY ("../.."). The common directory comes out absolute and clean, the one of the main
// checkout, and the main root is its parent.
func TestLocate_ALinkedWorktreeFindsTheCommonDirThroughItsRelativePointer(t *testing.T) {
	root := realTree(t)
	common := mkdir(t, root, "main", ".git")
	gitDir := mkdir(t, common, "worktrees", "feature")
	write(t, filepath.Join(gitDir, "commondir"), "../..\n")
	worktree := mkdir(t, root, "feature")
	write(t, filepath.Join(worktree, ".git"), "gitdir: "+gitDir+"\n")

	got, err := projectidfs.Inspector{}.Locate(worktree)

	want := projectid.Repository{WorktreeRoot: worktree, MainWorktreeRoot: filepath.Join(root, "main"), CommonDir: common}
	if err != nil || got != want {
		t.Errorf("Locate = %+v, %v; want %+v", got, err, want)
	}
}

// A .git file may name its git directory relative to the working tree, as a submodule's does; with no commondir
// file in it, that directory IS the common directory.
func TestLocate_ASubmoduleIsItsOwnCommonDir(t *testing.T) {
	root := realTree(t)
	modules := mkdir(t, root, ".git", "modules", "vendored")
	sub := mkdir(t, root, "vendored")
	write(t, filepath.Join(sub, ".git"), "gitdir: ../.git/modules/vendored\n")

	got, err := projectidfs.Inspector{}.Locate(sub)

	want := projectid.Repository{WorktreeRoot: sub, MainWorktreeRoot: filepath.Dir(modules), CommonDir: modules}
	if err != nil || got != want {
		t.Errorf("Locate = %+v, %v; want %+v", got, err, want)
	}
}

// The directory asked about is canonicalized first, so the same repository reached through a link answers the
// same, and an empty directory means the working directory.
func TestLocate_AnswersInResolvedPathsWhateverWayTheDirectoryIsNamed(t *testing.T) {
	root := realTree(t)
	repo := mkdir(t, root, "repo")
	mkdir(t, repo, ".git")
	link := filepath.Join(root, "link")
	if err := os.Symlink(repo, link); err != nil {
		t.Fatal(err)
	}
	want := projectid.Repository{WorktreeRoot: repo, MainWorktreeRoot: repo, CommonDir: filepath.Join(repo, ".git")}

	for _, dir := range []string{repo, link, filepath.Join(link, "..", "repo")} {
		if got, err := (projectidfs.Inspector{}).Locate(dir); err != nil || got != want {
			t.Errorf("Locate(%s) = %+v, %v; want %+v", dir, got, err, want)
		}
	}
	t.Chdir(repo)
	if got, err := (projectidfs.Inspector{}).Locate(""); err != nil || got != want {
		t.Errorf("Locate(\"\") = %+v, %v; want the repository of the working directory, %+v", got, err, want)
	}
}

// Where there is no .git above the directory the answer is the named error with the directory in its words (it
// is what an operator reads), and where there is one that names nothing it is the same named error with the
// file; a pointer to a directory that is not there is an error of its own, not a repository that is not one.
func TestLocate_NamesWhatIsWrongWithoutGuessing(t *testing.T) {
	for _, tc := range []struct {
		name      string
		layout    func(t *testing.T, dir string)
		notARepo  bool
		wantWords func(dir string) string
	}{
		{"no .git above", func(t *testing.T, dir string) {}, true,
			func(dir string) string { return "projectid: directory is not inside a git repository: " + dir }},
		{"a .git file that is no pointer", func(t *testing.T, dir string) { write(t, filepath.Join(dir, ".git"), "not a pointer\n") }, true,
			func(dir string) string {
				return "projectid: " + filepath.Join(dir, ".git") + " does not name a git directory: projectid: directory is not inside a git repository"
			}},
		{"a .git file that names an empty directory", func(t *testing.T, dir string) { write(t, filepath.Join(dir, ".git"), "gitdir:   \n") }, true,
			func(dir string) string {
				return "projectid: " + filepath.Join(dir, ".git") + " names an empty git directory: projectid: directory is not inside a git repository"
			}},
		{"a .git file that names a directory that is not there", func(t *testing.T, dir string) {
			write(t, filepath.Join(dir, ".git"), "gitdir: "+filepath.Join(dir, "gone")+"\n")
		}, false, func(dir string) string {
			return "projectid: resolving " + filepath.Join(dir, "gone") + ": lstat " + filepath.Join(dir, "gone") + ": no such file or directory"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := realTree(t)
			tc.layout(t, dir)

			_, err := projectidfs.Inspector{}.Locate(dir)

			if err == nil || errors.Is(err, projectid.ErrNotARepository) != tc.notARepo {
				t.Fatalf("Locate = %v; want an error that is-not-a-repository = %v", err, tc.notARepo)
			}
			if want := tc.wantWords(dir); err.Error() != want {
				t.Errorf("the error is %q, want %q", err, want)
			}
		})
	}
}

// A .git that cannot be looked at (a directory the process may not search) is not "no .git": the walk does not
// go on above it as though the repository were not there, which would resolve the repository that happens to
// contain this one. The error names the file.
func TestLocate_AGitEntryThatCannotBeInspectedIsAnErrorNotNoRepository(t *testing.T) {
	root := realTree(t)
	locked := mkdir(t, root, "locked")
	mkdir(t, root, ".git") // a repository above, which a walk that went on would find
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	if _, err := os.ReadDir(locked); err == nil {
		t.Skip("directory permissions are not enforced for this process (root or an equivalent capability): the entry cannot be made unreadable")
	}

	_, err := projectidfs.Inspector{}.Locate(locked)

	if err == nil || errors.Is(err, projectid.ErrNotARepository) {
		t.Fatalf("Locate = %v; want an error that is not 'not a repository'", err)
	}
	if want := "projectid: inspecting " + filepath.Join(locked, ".git") + ": "; !strings.HasPrefix(err.Error(), want) {
		t.Errorf("the error is %q, want it to begin %q", err, want)
	}
}

// A directory that is not there cannot be located, and is no repository either: the error says it could not be
// resolved.
func TestLocate_ADirectoryThatIsNotThereCannotBeResolved(t *testing.T) {
	gone := filepath.Join(realTree(t), "gone")

	_, err := projectidfs.Inspector{}.Locate(gone)

	if err == nil || errors.Is(err, projectid.ErrNotARepository) || !strings.HasPrefix(err.Error(), "projectid: resolving "+gone+": ") {
		t.Errorf("Locate = %v; want a resolution error that names %s", err, gone)
	}
}

func TestDeclaredProject_IsTheFileAsItIsOrNothingAtAll(t *testing.T) {
	root := realTree(t)
	inspector := projectidfs.Inspector{}

	if text, found, err := inspector.DeclaredProject(root); text != "" || found || err != nil {
		t.Errorf("DeclaredProject of a root with no declaration = %q, %v, %v; want nothing, no error", text, found, err)
	}

	write(t, filepath.Join(root, projectid.DeclaredFileName), "  chosen-name \r\nsecond line")
	if text, found, err := inspector.DeclaredProject(root); text != "  chosen-name \r\nsecond line" || !found || err != nil {
		t.Errorf("DeclaredProject = %q, %v, %v; want the text untouched (the rules judge it)", text, found, err)
	}
}

func TestDeclaredProject_AnUnreadableDeclarationIsAnErrorInTheWordsTheRulesHaveAlwaysUsed(t *testing.T) {
	root := realTree(t)
	path := mkdir(t, root, projectid.DeclaredFileName)

	text, found, err := projectidfs.Inspector{}.DeclaredProject(root)

	if err == nil || text != "" || found {
		t.Fatalf("DeclaredProject = %q, %v, %v; want an error and nothing else", text, found, err)
	}
	if want := "projectid: reading " + path + ": read " + path + ": is a directory"; err.Error() != want {
		t.Errorf("the error is %q, want %q", err, want)
	}
}

func TestGitConfig_IsTheFileAsItIsAndEmptyWhenItCannotBeRead(t *testing.T) {
	common := mkdir(t, realTree(t), ".git")
	inspector := projectidfs.Inspector{}

	if got := inspector.GitConfig(common); got != "" {
		t.Errorf("GitConfig of a repository with no config = %q, want nothing", got)
	}

	const config = "[remote \"origin\"]\n\turl = https://github.com/acme/widgets.git\n"
	write(t, filepath.Join(common, "config"), config)
	if got := inspector.GitConfig(common); got != config {
		t.Errorf("GitConfig = %q, want the file untouched", got)
	}

	if err := os.Remove(filepath.Join(common, "config")); err != nil {
		t.Fatal(err)
	}
	mkdir(t, common, "config")
	if got := inspector.GitConfig(common); got != "" {
		t.Errorf("GitConfig of a config that is a directory = %q, want nothing", got)
	}
}
