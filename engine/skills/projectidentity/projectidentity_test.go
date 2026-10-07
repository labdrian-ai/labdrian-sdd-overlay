package projectidentity_test

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/projectidentity"
)

// vectorsDir holds the recorded vectors of the identity module (Phase 9, D2): the same files
// longterm-mem runs through its own reader of a repository.
const vectorsDir = "../../../identity/testdata"

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

// mainCheckout is a repository whose .git directory holds config, made by hand: no git runs.
func mainCheckout(t *testing.T, config string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "demo")
	put(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")
	put(t, filepath.Join(root, ".git", "config"), config)
	return root
}

func identify(t *testing.T, source skills.ProjectIdentity, q skills.ProjectQuery) (skills.ProjectID, bool) {
	t.Helper()
	id, ok, err := source.Identify(q)
	if err != nil {
		t.Fatalf("Identify(%+v): %v", q, err)
	}
	return id, ok
}

// ---- explicit and the directory name --------------------------------------------------

func TestExplicitAnswersOnlyWhenTheIdWasGiven(t *testing.T) {
	if id, ok := identify(t, projectidentity.Explicit{}, skills.ProjectQuery{Dir: "/p/demo", Explicit: "given"}); !ok || id != "given" {
		t.Errorf("Explicit with an id = %q, %v, want it answered as given", id, ok)
	}
	if id, ok := identify(t, projectidentity.Explicit{}, skills.ProjectQuery{Dir: "/p/demo"}); ok || id != "" {
		t.Errorf("Explicit with no id = %q, %v, want no answer", id, ok)
	}
}

func TestDirectoryNameAlwaysAnswersWithTheNameOfTheDirectory(t *testing.T) {
	for dir, want := range map[string]skills.ProjectID{"/p/demo": "demo", "/p/demo/": "demo", "/": "/", "relative/dir": "dir", "": "."} {
		if id, ok := identify(t, projectidentity.DirectoryName{}, skills.ProjectQuery{Dir: dir, Explicit: "ignored"}); !ok || id != want {
			t.Errorf("DirectoryName(%q) = %q, %v, want %q", dir, id, ok, want)
		}
	}
}

// ---- the origin remote, read as a file ------------------------------------------------

func TestGitOriginAnswersTheNormalizedOriginOfTheRepository(t *testing.T) {
	root := mainCheckout(t, originConfig("git@github.com:acme/demo.git"))
	if id, ok := identify(t, projectidentity.GitOrigin{}, skills.ProjectQuery{Dir: root}); !ok || id != "github.com/acme/demo" {
		t.Errorf("GitOrigin = %q, %v, want github.com/acme/demo", id, ok)
	}
}

func TestGitOriginIgnoresTheExplicitIdAndLeavesItToTheChain(t *testing.T) {
	root := mainCheckout(t, originConfig("git@github.com:acme/demo.git"))
	if id, _ := identify(t, projectidentity.GitOrigin{}, skills.ProjectQuery{Dir: root, Explicit: "given"}); id != "github.com/acme/demo" {
		t.Errorf("GitOrigin = %q, want the origin whatever was given", id)
	}
}

func TestGitOriginFindsTheRepositoryFromADirectoryBelowItsRoot(t *testing.T) {
	root := mainCheckout(t, originConfig("https://github.com/acme/demo.git"))
	below := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(below, 0o755); err != nil {
		t.Fatal(err)
	}
	if id, ok := identify(t, projectidentity.GitOrigin{}, skills.ProjectQuery{Dir: below}); !ok || id != "github.com/acme/demo" {
		t.Errorf("GitOrigin from below the root = %q, %v, want github.com/acme/demo", id, ok)
	}
}

func TestGitOriginFollowsALinkedWorktreeToTheCommonDirectory(t *testing.T) {
	main := mainCheckout(t, originConfig("https://github.com/acme/demo.git"))
	worktree := filepath.Join(t.TempDir(), "feature-x")
	gitDir := filepath.Join(main, ".git", "worktrees", "feature-x")
	put(t, filepath.Join(gitDir, "HEAD"), "ref: refs/heads/feature-x\n")
	put(t, filepath.Join(gitDir, "commondir"), "../..\n")
	put(t, filepath.Join(worktree, ".git"), "gitdir: "+gitDir+"\n")

	if id, ok := identify(t, projectidentity.GitOrigin{}, skills.ProjectQuery{Dir: worktree}); !ok || id != "github.com/acme/demo" {
		t.Errorf("GitOrigin in a linked worktree = %q, %v, want the origin of the repository it belongs to", id, ok)
	}
}

func TestGitOriginFollowsARelativeGitdirPointer(t *testing.T) {
	base := t.TempDir()
	main := filepath.Join(base, "main")
	put(t, filepath.Join(main, ".git", "HEAD"), "ref: refs/heads/main\n")
	put(t, filepath.Join(main, ".git", "config"), originConfig("https://github.com/acme/demo.git"))
	put(t, filepath.Join(main, ".git", "worktrees", "wt", "HEAD"), "ref: refs/heads/wt\n")
	put(t, filepath.Join(main, ".git", "worktrees", "wt", "commondir"), "../..")
	put(t, filepath.Join(base, "wt", ".git"), "gitdir: ../main/.git/worktrees/wt")

	if id, ok := identify(t, projectidentity.GitOrigin{}, skills.ProjectQuery{Dir: filepath.Join(base, "wt")}); !ok || id != "github.com/acme/demo" {
		t.Errorf("GitOrigin through a relative pointer = %q, %v, want github.com/acme/demo", id, ok)
	}
}

func TestGitOriginTreatsAGitDirectoryWithNoCommondirAsTheCommonDirectory(t *testing.T) {
	base := t.TempDir()
	put(t, filepath.Join(base, "modules", "sub", "HEAD"), "ref: refs/heads/main\n")
	put(t, filepath.Join(base, "modules", "sub", "config"), originConfig("https://github.com/acme/sub.git"))
	put(t, filepath.Join(base, "sub", ".git"), "gitdir: "+filepath.Join(base, "modules", "sub")+"\n")

	if id, ok := identify(t, projectidentity.GitOrigin{}, skills.ProjectQuery{Dir: filepath.Join(base, "sub")}); !ok || id != "github.com/acme/sub" {
		t.Errorf("GitOrigin for a submodule = %q, %v, want github.com/acme/sub", id, ok)
	}
}

func TestGitOriginHasNoAnswerWhereThereIsNoOrigin(t *testing.T) {
	plain := t.TempDir()
	noOrigin := mainCheckout(t, "[core]\n\tbare = false\n")
	localOrigin := mainCheckout(t, originConfig("/srv/git/demo.git"))
	noConfig := filepath.Join(t.TempDir(), "demo")
	put(t, filepath.Join(noConfig, ".git", "HEAD"), "ref: refs/heads/main\n")

	for name, dir := range map[string]string{"a directory that is no repository": plain, "a repository with no origin": noOrigin, "an origin with no host": localOrigin, "a git directory with no config": noConfig} {
		if id, ok := identify(t, projectidentity.GitOrigin{}, skills.ProjectQuery{Dir: dir}); ok || id != "" {
			t.Errorf("%s: GitOrigin = %q, %v, want no answer", name, id, ok)
		}
	}
}

// A pointer is trusted only as far as it leads to a git directory, which always has a HEAD. One
// that leads nowhere (a worktree or a submodule that was moved or removed since), or to a directory
// that is no git directory (so that its config is not a repository's), is a repository the source
// could not read: an error that names the file, the directory it names and how to go on, which is
// to name the project, and the chain does not go on to name it by the directory.
func TestGitOriginSaysItCannotTellWhenAPointerLeadsToNoGitDirectory(t *testing.T) {
	hostileConfig := originConfig("https://github.com/evil/trap.git")
	// Each case says its own reason, not another's.
	wantReason := map[string]string{
		"the git directory is gone":    "it has no HEAD",
		"the HEAD is a directory":      "its HEAD is a directory",
		"the HEAD cannot be looked at": "whose HEAD cannot be looked at",
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
		"the HEAD cannot be looked at": func(t *testing.T) (string, string, string) {
			dir := filepath.Join(t.TempDir(), "demo")
			gitDir := filepath.Join(t.TempDir(), "gitdir")
			// A HEAD that links to itself cannot be looked at, whoever asks (root included).
			if err := os.MkdirAll(gitDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("HEAD", filepath.Join(gitDir, "HEAD")); err != nil {
				t.Skipf("no symlink: %v", err)
			}
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

			id, ok, err := projectidentity.GitOrigin{}.Identify(skills.ProjectQuery{Dir: dir})
			if err == nil || ok || id != "" {
				t.Fatalf("GitOrigin = %q, %v, %v, want an error", id, ok, err)
			}
			for _, want := range []string{gitFile, named, "--project-id"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("the error %q does not say %q", err, want)
				}
			}
			chained := projectidentity.Chain(projectidentity.GitOrigin{}, projectidentity.DirectoryName{})
			if id, ok, err := chained.Identify(skills.ProjectQuery{Dir: dir}); err == nil || ok || id != "" {
				t.Errorf("Chain = %q, %v, %v, want the error, not the name of the directory", id, ok, err)
			}
			if want := wantReason[name]; want != "" && !strings.Contains(err.Error(), want) {
				t.Errorf("the error %q does not say %q", err, want)
			}
			// A person who names the project does not need to look.
			withTheID := projectidentity.Chain(projectidentity.Explicit{}, projectidentity.GitOrigin{}, projectidentity.DirectoryName{})
			if id, ok, err := withTheID.Identify(skills.ProjectQuery{Dir: dir, Explicit: "given"}); err != nil || !ok || id != "given" {
				t.Errorf("Chain with --project-id = %q, %v, %v, want it answered as given", id, ok, err)
			}
		})
	}
}

// A config that is there and cannot be read is a source that could not tell, which is not the same
// as one with no answer: the chain must not go on to name the project by something less.
func TestGitOriginSaysItCannotTellWhenTheConfigCannotBeRead(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("a file without permissions does not stop root")
	}
	root := mainCheckout(t, originConfig("https://github.com/acme/demo.git"))
	config := filepath.Join(root, ".git", "config")
	if err := os.Chmod(config, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(config, 0o644) })

	id, ok, err := projectidentity.GitOrigin{}.Identify(skills.ProjectQuery{Dir: root})
	if err == nil || ok || id != "" {
		t.Fatalf("GitOrigin = %q, %v, %v, want the error of the read", id, ok, err)
	}
	if !strings.Contains(err.Error(), config) {
		t.Errorf("the error %q does not name %s", err, config)
	}
}

// A .git that is a file and holds no gitdir line is a repository the source could not read, not a
// directory that is no repository: it is an error that names the file, as an unreadable one is,
// and the chain does not go on to name the project by the directory.
func TestGitOriginSaysItCannotTellWhenTheGitFileIsNoPointer(t *testing.T) {
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

			id, ok, err := projectidentity.GitOrigin{}.Identify(skills.ProjectQuery{Dir: dir})
			if err == nil || ok || id != "" {
				t.Fatalf("GitOrigin = %q, %v, %v, want an error", id, ok, err)
			}
			if !strings.Contains(err.Error(), gitFile) {
				t.Errorf("the error %q does not name %s", err, gitFile)
			}
			chained := projectidentity.Chain(projectidentity.GitOrigin{}, projectidentity.DirectoryName{})
			if id, ok, err := chained.Identify(skills.ProjectQuery{Dir: dir}); err == nil || ok || id != "" {
				t.Errorf("Chain = %q, %v, %v, want the error, not the name of the directory", id, ok, err)
			}
		})
	}
}

// A .git directory whose existence cannot be told, because a directory above it cannot be read, is
// an error that names the directory and says how to go on, which is to name the project.
func TestGitOriginSaysWhichDirectoryItCouldNotReadAndHowToGoOn(t *testing.T) {
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

	id, ok, err := projectidentity.GitOrigin{}.Identify(skills.ProjectQuery{Dir: below})
	if err == nil || ok || id != "" {
		t.Fatalf("GitOrigin = %q, %v, %v, want an error", id, ok, err)
	}
	// The cause is told by the kind of error and not by the words of one system: the words of a
	// refused permission differ between platforms.
	if !errors.Is(err, fs.ErrPermission) {
		t.Errorf("the error %q does not wrap fs.ErrPermission", err)
	}
	for _, want := range []string{below, "--project-id"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error %q does not say %q", err, want)
		}
	}
	// A person who names the project does not need to look.
	chained := projectidentity.Chain(projectidentity.Explicit{}, projectidentity.GitOrigin{}, projectidentity.DirectoryName{})
	if id, ok, err := chained.Identify(skills.ProjectQuery{Dir: below, Explicit: "given"}); err != nil || !ok || id != "given" {
		t.Errorf("Chain with --project-id = %q, %v, %v, want it answered as given", id, ok, err)
	}
}

// The same refusal, reached in a way that does not depend on permissions, so that it is proved for
// root too: a .git that links to itself cannot be looked at, whoever asks. The cause is kept, and
// the way on is the same.
func TestGitOriginSaysWhichDirectoryItCouldNotLookAtWhateverTheUser(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(".git", filepath.Join(dir, ".git")); err != nil {
		t.Skipf("cannot make a symbolic link here: %v", err)
	}

	id, ok, err := projectidentity.GitOrigin{}.Identify(skills.ProjectQuery{Dir: dir})

	if err == nil || ok || id != "" {
		t.Fatalf("GitOrigin = %q, %v, %v, want an error", id, ok, err)
	}
	for _, want := range []string{dir, "--project-id"} {
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
			id, ok := identify(t, projectidentity.GitOrigin{}, skills.ProjectQuery{Dir: mainCheckout(t, originConfig(v.URL))})
			if id != skills.ProjectID(v.Want) || ok != (v.Want != "") {
				t.Errorf("origin %q gave %q, %v, want %q", v.URL, id, ok, v.Want)
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
			id, ok := identify(t, projectidentity.GitOrigin{}, skills.ProjectQuery{Dir: mainCheckout(t, v.Config)})
			if id != skills.ProjectID(v.Want) || ok != (v.Want != "") {
				t.Errorf("config %q gave %q, %v, want %q", v.Config, id, ok, v.Want)
			}
		})
	}
}

func readVectors(t *testing.T, name string, into any) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(vectorsDir, name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}

// ---- the chain ------------------------------------------------------------------------

type answer struct {
	id  skills.ProjectID
	ok  bool
	err error
	// asked counts how many times the source was asked.
	asked *int
}

func (a answer) Identify(skills.ProjectQuery) (skills.ProjectID, bool, error) {
	if a.asked != nil {
		*a.asked++
	}
	return a.id, a.ok, a.err
}

func TestChainAsksInOrderAndTheFirstAnswerWins(t *testing.T) {
	var first, second, third int
	chain := projectidentity.Chain(
		answer{asked: &first},
		answer{id: "second", ok: true, asked: &second},
		answer{id: "third", ok: true, asked: &third},
	)
	if id, ok := identify(t, chain, skills.ProjectQuery{Dir: "/p"}); !ok || id != "second" {
		t.Errorf("Chain = %q, %v, want the answer of the second source", id, ok)
	}
	if first != 1 || second != 1 || third != 0 {
		t.Errorf("sources asked %d, %d, %d times, want 1, 1, 0: the chain stops at the first answer", first, second, third)
	}
}

func TestChainHasNoAnswerWhenNoSourceHasOne(t *testing.T) {
	if id, ok := identify(t, projectidentity.Chain(answer{}, answer{}), skills.ProjectQuery{}); ok || id != "" {
		t.Errorf("Chain = %q, %v, want no answer", id, ok)
	}
	if id, ok := identify(t, projectidentity.Chain(), skills.ProjectQuery{}); ok || id != "" {
		t.Errorf("an empty Chain = %q, %v, want no answer", id, ok)
	}
}

func TestChainStopsAtASourceThatCannotTell(t *testing.T) {
	boom := errors.New("cannot tell")
	var later int
	chain := projectidentity.Chain(answer{err: boom}, answer{id: "later", ok: true, asked: &later})
	id, ok, err := chain.Identify(skills.ProjectQuery{})
	if !errors.Is(err, boom) || ok || id != "" || later != 0 {
		t.Errorf("Chain = %q, %v, %v (later asked %d), want the error and no further question", id, ok, err, later)
	}
}

// The order the owner chose (Phase 9, Q8): what the person said, then what the repository says of
// itself, then what the directory is called.
func TestTheChainOfTheOwnerNamesTheProjectInTheOrderQ8Gives(t *testing.T) {
	chain := projectidentity.Chain(projectidentity.Explicit{}, projectidentity.GitOrigin{}, projectidentity.DirectoryName{})
	withOrigin := mainCheckout(t, originConfig("git@github.com:acme/demo.git"))
	noOrigin := mainCheckout(t, "[core]\n\tbare = false\n")

	for name, tc := range map[string]struct {
		q    skills.ProjectQuery
		want skills.ProjectID
	}{
		"explicit beats the origin and the directory":   {skills.ProjectQuery{Dir: withOrigin, Explicit: "given"}, "given"},
		"the origin beats the directory":                {skills.ProjectQuery{Dir: withOrigin}, "github.com/acme/demo"},
		"the directory when there is nothing else":      {skills.ProjectQuery{Dir: noOrigin}, "demo"},
		"the directory when it is no repository at all": {skills.ProjectQuery{Dir: filepath.Join(t.TempDir(), "plain")}, "plain"},
	} {
		if id, ok := identify(t, chain, tc.q); !ok || id != tc.want {
			t.Errorf("%s: Chain = %q, %v, want %q", name, id, ok, tc.want)
		}
	}
}
