package gitfs_test

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/identity/gitfs"
	"github.com/labdrian-ai/labdrian-sdd-overlay/identity/resolve"
)

// The adapter's refusals end with the way on, which names the flag the caller passes in.
const flag = "--project-id"

func put(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func originConfig(url string) string {
	return "[core]\n\tbare = false\n[remote \"origin\"]\n\turl = " + url + "\n"
}

// mainCheckout is a repository named demo whose .git directory holds config, made by hand: no git
// runs.
func mainCheckout(t *testing.T, config string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "demo")
	put(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")
	put(t, filepath.Join(root, ".git", "config"), config)
	return root
}

// linkedWorktree makes dir a linked worktree of the repository at main, the way git leaves one.
func linkedWorktree(t *testing.T, main, dir, name string) {
	t.Helper()
	gitDir := filepath.Join(main, ".git", "worktrees", name)
	put(t, filepath.Join(gitDir, "HEAD"), "ref: refs/heads/"+name+"\n")
	put(t, filepath.Join(gitDir, "commondir"), "../..\n")
	put(t, filepath.Join(dir, ".git"), "gitdir: "+gitDir+"\n")
}

func resolver(r gitfs.Repositories) resolve.Resolver { return resolve.New(r) }

func resolveIn(t *testing.T, dir string) resolve.Identity {
	t.Helper()
	id, err := resolver(gitfs.Repositories{ExplicitFlag: flag}).Resolve(dir, "")
	if err != nil {
		t.Fatalf("Resolve(%q): %v", dir, err)
	}
	return id
}

func realpath(t *testing.T, p string) string {
	t.Helper()
	real, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return real
}

// ---- where the repository is ----------------------------------------------------------

func TestLocateFindsAMainCheckoutFromItsRootAndFromBelow(t *testing.T) {
	root := mainCheckout(t, originConfig("https://github.com/acme/demo.git"))
	below := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(below, 0o755); err != nil {
		t.Fatal(err)
	}
	want := resolve.Repository{WorktreeRoot: realpath(t, root), MainWorktreeRoot: realpath(t, root), CommonDir: realpath(t, filepath.Join(root, ".git"))}
	for _, dir := range []string{root, below} {
		got, err := gitfs.Repositories{}.Locate(dir)
		if err != nil || got != want {
			t.Errorf("Locate(%q) = %+v, %v, want %+v", dir, got, err, want)
		}
	}
}

func TestLocateFollowsALinkedWorktreeToTheCommonDirectoryAndTheMainCheckout(t *testing.T) {
	main := mainCheckout(t, originConfig("https://github.com/acme/demo.git"))
	worktree := filepath.Join(t.TempDir(), "feature-x")
	linkedWorktree(t, main, worktree, "feature-x")

	got, err := gitfs.Repositories{}.Locate(worktree)
	want := resolve.Repository{WorktreeRoot: realpath(t, worktree), MainWorktreeRoot: realpath(t, main), CommonDir: realpath(t, filepath.Join(main, ".git"))}
	if err != nil || got != want {
		t.Errorf("Locate = %+v, %v, want %+v", got, err, want)
	}
}

func TestLocateSaysADirectoryOutsideARepositoryIsInNone(t *testing.T) {
	_, err := gitfs.Repositories{}.Locate(t.TempDir())
	if !errors.Is(err, resolve.ErrNotARepository) {
		t.Errorf("Locate = %v, want ErrNotARepository", err)
	}
}

// A directory reached through a link is the directory it links to, so the name of the main
// checkout is the same however the working directory was reached.
func TestADirectoryReachedThroughALinkIsTheRealOne(t *testing.T) {
	root := mainCheckout(t, "[core]\n")
	link := filepath.Join(t.TempDir(), "a-link")
	if err := os.Symlink(root, link); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	if id := resolveIn(t, link); id.Project != "demo" {
		t.Errorf("Resolve through a link = %q, want demo", id.Project)
	}
}

// ---- the chain over the disk: the owner's order (C23, decision 1 of M2) ---------------

func TestTheChainOverTheDiskNamesTheProjectInTheOwnersOrder(t *testing.T) {
	withOrigin := mainCheckout(t, originConfig("git@github.com:acme/demo.git"))
	noOrigin := mainCheckout(t, "[core]\n\tbare = false\n")
	declared := mainCheckout(t, originConfig("git@github.com:acme/demo.git"))
	put(t, filepath.Join(declared, resolve.DeclaredFileName), "declared-name\n")
	belowNoOrigin := filepath.Join(noOrigin, "sub")
	if err := os.MkdirAll(belowNoOrigin, 0o755); err != nil {
		t.Fatal(err)
	}
	worktreeNoOrigin := filepath.Join(t.TempDir(), "feature-x")
	linkedWorktree(t, noOrigin, worktreeNoOrigin, "feature-x")
	worktreeDeclared := filepath.Join(t.TempDir(), "feature-y")
	linkedWorktree(t, declared, worktreeDeclared, "feature-y")
	plain := filepath.Join(t.TempDir(), "plain")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name     string
		dir      string
		explicit string
		want     string
		source   resolve.Source
	}{
		{"the explicit id beats the declaration and the origin", declared, "given", "given", resolve.SourceExplicit},
		{"the declaration beats the origin", declared, "", "declared-name", resolve.SourceDeclared},
		{"a linked worktree reads the main checkout's declaration", worktreeDeclared, "", "declared-name", resolve.SourceDeclared},
		{"the origin beats the directory", withOrigin, "", "github.com/acme/demo", resolve.SourceRemote},
		{"no origin is the main checkout's name", noOrigin, "", "demo", resolve.SourceDirectory},
		{"below the root is the main checkout's name, not the subdirectory's", belowNoOrigin, "", "demo", resolve.SourceDirectory},
		{"a linked worktree is the main checkout's name, not its own", worktreeNoOrigin, "", "demo", resolve.SourceDirectory},
		{"outside a repository it is the directory's name", plain, "", "plain", resolve.SourceDirectory},
	} {
		t.Run(tt.name, func(t *testing.T) {
			id, err := resolver(gitfs.Repositories{}).Resolve(tt.dir, tt.explicit)
			if err != nil || id.Project != tt.want || id.Source != tt.source {
				t.Errorf("Resolve = %q via %s, %v, want %q via %s", id.Project, id.Source, err, tt.want, tt.source)
			}
		})
	}
}

// A declaration a linked worktree's branch carries alone names nothing.
func TestADeclarationOnlyInALinkedWorktreeIsNotRead(t *testing.T) {
	main := mainCheckout(t, "[core]\n")
	worktree := filepath.Join(t.TempDir(), "feature-x")
	linkedWorktree(t, main, worktree, "feature-x")
	put(t, filepath.Join(worktree, resolve.DeclaredFileName), "only-here\n")
	if id := resolveIn(t, worktree); id.Project != "demo" || id.Source != resolve.SourceDirectory {
		t.Errorf("Resolve = %+v, want the main checkout's name", id)
	}
}

func TestADeclarationThatCannotBeReadIsAnError(t *testing.T) {
	root := mainCheckout(t, originConfig("git@github.com:acme/demo.git"))
	// A directory where the file should be cannot be read as one.
	if err := os.MkdirAll(filepath.Join(root, resolve.DeclaredFileName), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := resolver(gitfs.Repositories{}).Resolve(root, "")
	if err == nil || !strings.Contains(err.Error(), resolve.DeclaredFileName) {
		t.Errorf("Resolve = %v, want an error naming the declaration", err)
	}
}

func TestABrokenDeclarationIsRefused(t *testing.T) {
	root := mainCheckout(t, originConfig("git@github.com:acme/demo.git"))
	put(t, filepath.Join(root, resolve.DeclaredFileName), "one\ntwo\n")
	if _, err := resolver(gitfs.Repositories{}).Resolve(root, ""); !errors.Is(err, resolve.ErrDeclaredInvalid) {
		t.Errorf("Resolve = %v, want ErrDeclaredInvalid", err)
	}
}

// ---- the origin, read as a file -------------------------------------------------------

func TestTheOriginIsReadThroughARelativeGitdirPointer(t *testing.T) {
	base := t.TempDir()
	main := filepath.Join(base, "main")
	put(t, filepath.Join(main, ".git", "HEAD"), "ref: refs/heads/main\n")
	put(t, filepath.Join(main, ".git", "config"), originConfig("https://github.com/acme/demo.git"))
	put(t, filepath.Join(main, ".git", "worktrees", "wt", "HEAD"), "ref: refs/heads/wt\n")
	put(t, filepath.Join(main, ".git", "worktrees", "wt", "commondir"), "../..")
	put(t, filepath.Join(base, "wt", ".git"), "gitdir: ../main/.git/worktrees/wt")

	if id := resolveIn(t, filepath.Join(base, "wt")); id.Project != "github.com/acme/demo" {
		t.Errorf("Resolve through a relative pointer = %q, want github.com/acme/demo", id.Project)
	}
}

func TestAGitDirectoryWithNoCommondirIsTheCommonDirectory(t *testing.T) {
	base := t.TempDir()
	put(t, filepath.Join(base, "modules", "sub", "HEAD"), "ref: refs/heads/main\n")
	put(t, filepath.Join(base, "modules", "sub", "config"), originConfig("https://github.com/acme/sub.git"))
	put(t, filepath.Join(base, "sub", ".git"), "gitdir: "+filepath.Join(base, "modules", "sub")+"\n")

	if id := resolveIn(t, filepath.Join(base, "sub")); id.Project != "github.com/acme/sub" {
		t.Errorf("Resolve for a submodule = %q, want github.com/acme/sub", id.Project)
	}
}

func TestNoOriginFallsThroughToTheDirectory(t *testing.T) {
	noOrigin := mainCheckout(t, "[core]\n\tbare = false\n")
	localOrigin := mainCheckout(t, originConfig("/srv/git/demo.git"))
	noConfig := filepath.Join(t.TempDir(), "demo")
	put(t, filepath.Join(noConfig, ".git", "HEAD"), "ref: refs/heads/main\n")

	for name, dir := range map[string]string{"a repository with no origin": noOrigin, "an origin with no host": localOrigin, "a git directory with no config": noConfig} {
		if id := resolveIn(t, dir); id.Project != "demo" || id.Source != resolve.SourceDirectory {
			t.Errorf("%s: Resolve = %q via %s, want demo via the directory", name, id.Project, id.Source)
		}
	}
}

// A pointer is trusted only as far as it leads to a git directory, which always has a HEAD. One
// that leads nowhere, or to a directory that is no git directory (so that its config is not a
// repository's), is a repository that could not be read: an error that names the file, the
// directory it names and how to go on, and the chain does not go on to name the project by the
// directory.
func TestAPointerThatLeadsToNoGitDirectoryIsAnError(t *testing.T) {
	hostileConfig := originConfig("https://github.com/evil/trap.git")
	wantReason := map[string]string{
		"the git directory is gone": "it has no HEAD",
		"the HEAD is a directory":   "its HEAD is a directory",
	}
	for name, build := range map[string]func(t *testing.T) (dir, gitFile, named string){
		"the git directory is gone": func(t *testing.T) (string, string, string) {
			dir := filepath.Join(t.TempDir(), "demo")
			gone := filepath.Join(t.TempDir(), "gone")
			put(t, filepath.Join(dir, ".git"), "gitdir: "+gone+"\n")
			return dir, filepath.Join(dir, ".git"), gone
		},
		"the directory is no git directory, whatever its config says": func(t *testing.T) (string, string, string) {
			dir := filepath.Join(t.TempDir(), "demo")
			notGit := filepath.Join(t.TempDir(), "notgit")
			put(t, filepath.Join(notGit, "config"), hostileConfig)
			put(t, filepath.Join(dir, ".git"), "gitdir: "+notGit+"\n")
			return dir, filepath.Join(dir, ".git"), notGit
		},
		"the common directory is no git directory, whatever its config says": func(t *testing.T) (string, string, string) {
			base := t.TempDir()
			dir := filepath.Join(base, "demo")
			gitDir := filepath.Join(base, "wt-git")
			notGit := filepath.Join(base, "elsewhere")
			put(t, filepath.Join(gitDir, "HEAD"), "ref: refs/heads/wt\n")
			put(t, filepath.Join(gitDir, "commondir"), notGit+"\n")
			put(t, filepath.Join(notGit, "config"), hostileConfig)
			put(t, filepath.Join(dir, ".git"), "gitdir: "+gitDir+"\n")
			return dir, filepath.Join(dir, ".git"), notGit
		},
		"the HEAD is a directory": func(t *testing.T) (string, string, string) {
			dir := filepath.Join(t.TempDir(), "demo")
			gitDir := filepath.Join(t.TempDir(), "gitdir")
			if err := os.MkdirAll(filepath.Join(gitDir, "HEAD"), 0o755); err != nil {
				t.Fatal(err)
			}
			put(t, filepath.Join(gitDir, "config"), hostileConfig)
			put(t, filepath.Join(dir, ".git"), "gitdir: "+gitDir+"\n")
			return dir, filepath.Join(dir, ".git"), gitDir
		},
		"the common directory is gone": func(t *testing.T) (string, string, string) {
			base := t.TempDir()
			dir := filepath.Join(base, "demo")
			gitDir := filepath.Join(base, "wt-git")
			put(t, filepath.Join(gitDir, "HEAD"), "ref: refs/heads/wt\n")
			put(t, filepath.Join(gitDir, "commondir"), "../gone\n")
			put(t, filepath.Join(dir, ".git"), "gitdir: "+gitDir+"\n")
			return dir, filepath.Join(dir, ".git"), filepath.Join(base, "gone")
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir, gitFile, named := build(t)

			id, err := resolver(gitfs.Repositories{ExplicitFlag: flag}).Resolve(dir, "")
			if err == nil || id != (resolve.Identity{}) {
				t.Fatalf("Resolve = %+v, %v, want an error", id, err)
			}
			if errors.Is(err, resolve.ErrNotARepository) {
				t.Errorf("the error %q says the directory is in no repository; it is a repository that could not be read", err)
			}
			for _, want := range []string{gitFile, named, flag, wantReason[name]} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("the error %q does not say %q", err, want)
				}
			}
			// A person who names the project does not need to look.
			if id, err := resolver(gitfs.Repositories{}).Resolve(dir, "given"); err != nil || id.Project != "given" {
				t.Errorf("Resolve with an explicit id = %+v, %v, want it answered as given", id, err)
			}
		})
	}
}

// A HEAD that cannot be looked at is told as that, whatever the system says: the stat is injected,
// so the test does not depend on how a platform refuses one.
func TestAHeadThatCannotBeLookedAtIsAnError(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "demo")
	gitDir := filepath.Join(base, "gitdir")
	put(t, filepath.Join(gitDir, "HEAD"), "ref: refs/heads/main\n")
	put(t, filepath.Join(gitDir, "config"), originConfig("https://github.com/acme/demo.git"))
	gitFile := filepath.Join(dir, ".git")
	put(t, gitFile, "gitdir: "+gitDir+"\n")
	head := filepath.Join(realpath(t, gitDir), "HEAD")
	denied := &fs.PathError{Op: "stat", Path: head, Err: fs.ErrPermission}
	source := gitfs.Repositories{ExplicitFlag: flag, Stat: func(path string) (fs.FileInfo, error) {
		if path == head || path == filepath.Join(gitDir, "HEAD") {
			return nil, denied
		}
		return os.Stat(path)
	}}

	_, err := resolver(source).Resolve(dir, "")
	if err == nil || !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("Resolve = %v, want an error wrapping fs.ErrPermission", err)
	}
	for _, want := range []string{gitFile, "whose HEAD cannot be looked at", flag} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error %q does not say %q", err, want)
		}
	}
}

// The real file system, where a HEAD that links to itself cannot be looked at whoever asks.
func TestAHeadThatLinksToItselfIsAnError(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "demo")
	loop := filepath.Join(base, "loop")
	if err := os.MkdirAll(loop, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("HEAD", filepath.Join(loop, "HEAD")); err != nil {
		t.Skipf("no symlinks to make a HEAD that links to itself: %v", err)
	}
	put(t, filepath.Join(dir, ".git"), "gitdir: "+loop+"\n")

	_, err := resolver(gitfs.Repositories{ExplicitFlag: flag}).Resolve(dir, "")
	if err == nil || !strings.Contains(err.Error(), "whose HEAD cannot be looked at") || !strings.Contains(err.Error(), flag) {
		t.Errorf("error = %v, want it to say the HEAD cannot be looked at and how to go on", err)
	}
}

// A config that is there and cannot be read is an error, not a repository with no remote.
func TestAConfigThatCannotBeReadIsAnError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("a file without permissions does not stop root")
	}
	root := mainCheckout(t, originConfig("https://github.com/acme/demo.git"))
	config := filepath.Join(root, ".git", "config")
	if err := os.Chmod(config, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(config, 0o644) })

	id, err := resolver(gitfs.Repositories{}).Resolve(root, "")
	if err == nil || id != (resolve.Identity{}) {
		t.Fatalf("Resolve = %+v, %v, want the error of the read", id, err)
	}
	if !strings.Contains(err.Error(), realpath(t, config)) {
		t.Errorf("the error %q does not name %s", err, config)
	}
}

// A .git that is a file and holds no gitdir line is a repository that could not be read, not a
// directory that is no repository.
func TestAGitFileThatIsNoPointerIsAnError(t *testing.T) {
	for name, content := range map[string]string{
		"text that is no pointer":    "this is not a git file\n",
		"an empty file":              "",
		"a gitdir line with no path": "gitdir:\n",
		"a path with no gitdir key":  "/srv/git/demo.git\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "demo")
			gitFile := filepath.Join(dir, ".git")
			put(t, gitFile, content)

			id, err := resolver(gitfs.Repositories{}).Resolve(dir, "")
			if err == nil || id != (resolve.Identity{}) || errors.Is(err, resolve.ErrNotARepository) {
				t.Fatalf("Resolve = %+v, %v, want an error that is not ErrNotARepository", id, err)
			}
			if !strings.Contains(err.Error(), realpath(t, dir)) {
				t.Errorf("the error %q does not name the file in %s", err, dir)
			}
		})
	}
}

// A .git directory whose existence cannot be told, because a directory above it cannot be read, is
// an error that names the directory and says how to go on.
func TestADirectoryThatCannotBeReadIsAnErrorThatSaysHowToGoOn(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("a directory without permissions does not stop root")
	}
	locked := filepath.Join(t.TempDir(), "locked")
	below := filepath.Join(locked, "project")
	if err := os.MkdirAll(below, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o755) })

	_, err := resolver(gitfs.Repositories{ExplicitFlag: flag}).Resolve(below, "")
	if err == nil || !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("Resolve = %v, want an error wrapping fs.ErrPermission", err)
	}
	for _, want := range []string{below, flag} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error %q does not say %q", err, want)
		}
	}
	if id, err := resolver(gitfs.Repositories{}).Resolve(below, "given"); err != nil || id.Project != "given" {
		t.Errorf("Resolve with an explicit id = %+v, %v, want it answered as given", id, err)
	}
}

// The same refusal, reached in a way that does not depend on permissions: a .git that links to
// itself cannot be looked at, whoever asks. The cause is kept once.
func TestAGitThatLinksToItselfIsAnErrorWhateverTheUser(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(".git", filepath.Join(dir, ".git")); err != nil {
		t.Skipf("cannot make a symbolic link here: %v", err)
	}

	_, err := resolver(gitfs.Repositories{ExplicitFlag: flag}).Resolve(dir, "")
	if err == nil {
		t.Fatal("Resolve gave no error")
	}
	for _, want := range []string{realpath(t, dir), flag} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error %q does not say %q", err, want)
		}
	}
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		t.Errorf("the error %q carries the path error of the system twice", err)
	}
	if errors.Unwrap(err) == nil {
		t.Errorf("the error %q does not keep its cause", err)
	}
}

// Without a flag to name, a refusal says nothing about one.
func TestARefusalNamesNoFlagWhenThereIsNone(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "demo")
	put(t, filepath.Join(dir, ".git"), "gitdir: "+filepath.Join(t.TempDir(), "gone")+"\n")
	_, err := resolver(gitfs.Repositories{}).Resolve(dir, "")
	if err == nil || strings.Contains(err.Error(), "give ") {
		t.Errorf("Resolve = %v, want an error with no way on through a flag", err)
	}
}

// ---- the recorded vectors of the identity module ----------------------------------------

func TestEveryRecordedRemoteVectorGivesTheRecordedIdentity(t *testing.T) {
	var vectors []struct {
		URL  string `json:"url"`
		Want string `json:"want"`
	}
	readVectors(t, "remote-vectors.json", &vectors)
	if len(vectors) < 30 {
		t.Fatalf("only %d vectors: the file was truncated", len(vectors))
	}
	for _, v := range vectors {
		v := v
		t.Run(v.URL, func(t *testing.T) {
			id := resolveIn(t, mainCheckout(t, originConfig(v.URL)))
			want, source := v.Want, resolve.SourceRemote
			if want == "" {
				want, source = "demo", resolve.SourceDirectory
			}
			if id.Project != want || id.Source != source {
				t.Errorf("origin %q gave %q via %s, want %q via %s", v.URL, id.Project, id.Source, want, source)
			}
		})
	}
}

func TestEveryRecordedOriginConfigVectorGivesTheRecordedIdentity(t *testing.T) {
	var vectors []struct {
		Config string `json:"config"`
		Want   string `json:"want"`
	}
	readVectors(t, "origin-vectors.json", &vectors)
	if len(vectors) < 20 {
		t.Fatalf("only %d vectors: the file was truncated", len(vectors))
	}
	for _, v := range vectors {
		v := v
		t.Run(strings.ReplaceAll(v.Config, "\n", "|"), func(t *testing.T) {
			id := resolveIn(t, mainCheckout(t, v.Config))
			want := v.Want
			if want == "" {
				want = "demo"
			}
			if id.Project != want {
				t.Errorf("config %q gave %q, want %q", v.Config, id.Project, want)
			}
		})
	}
}

func readVectors(t *testing.T, name string, into any) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}
