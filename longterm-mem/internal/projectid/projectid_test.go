package projectid_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/projectid"
)

// Every test here builds its own throwaway repositories under t.TempDir()
// and runs git inside those. Nothing touches the repository this module
// lives in. Production code never shells out (exec_allowlist_test.go), but
// a _test.go file may, and building a real linked worktree is the only
// honest way to prove the property under test.

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// newRepo creates an initialized repository with one commit at parent/name
// and returns its path.
func newRepo(t *testing.T, parent, name string) string {
	t.Helper()
	root := filepath.Join(parent, name)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", root, err)
	}
	git(t, root, "init", "-q", "-b", "main")
	git(t, root, "commit", "-q", "--allow-empty", "-m", "init")
	return root
}

// addWorktree creates a linked worktree of repo at path and returns path.
func addWorktree(t *testing.T, repo, path string) string {
	t.Helper()
	git(t, repo, "worktree", "add", "-q", "-b", filepath.Base(path), path)
	return path
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func resolve(t *testing.T, dir string) projectid.Identity {
	t.Helper()
	id, err := projectid.Resolve(dir)
	if err != nil {
		t.Fatalf("Resolve(%s): unexpected error: %v", dir, err)
	}
	return id
}

// --- (a) the property: main checkout and linked worktree agree, per rule ---

func TestResolve_MainCheckoutAndWorktreeAgree_Declared(t *testing.T) {
	tmp := t.TempDir()
	repo := newRepo(t, tmp, "repo")
	write(t, filepath.Join(repo, projectid.DeclaredFileName), "acme-widgets\n")
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-q", "-m", "declare")
	wt := addWorktree(t, repo, filepath.Join(tmp, "wt"))

	main, linked := resolve(t, repo), resolve(t, wt)
	if main.Project != linked.Project {
		t.Fatalf("declared rule fragments across worktrees: main=%q worktree=%q", main.Project, linked.Project)
	}
	if main.Project != "acme-widgets" {
		t.Fatalf("declared identity = %q, want %q", main.Project, "acme-widgets")
	}
	if main.Rule != projectid.RuleDeclared || linked.Rule != projectid.RuleDeclared {
		t.Fatalf("rule = %q/%q, want %q for both", main.Rule, linked.Rule, projectid.RuleDeclared)
	}
}

// TestResolve_DeclarationIsRepositoryWide pins the property in BOTH
// directions, which the test above cannot reach.
//
// `git worktree add -b` branches off the current HEAD, so a declaration
// committed before the worktree exists is visible from both roots no matter
// how the rule is written. The dangerous shapes are the asymmetric ones: the
// file on one root and not the other. The first implementation searched the
// caller's worktree first and fell back to the main checkout, which made the
// worktree-only case resolve `declared` while the main checkout resolved
// `remote` -- two identities for one repository, produced by the fix itself.
//
// Searching two roots can only ever be one-directional, and whichever
// direction is missed fragments. So the declaration is read from the main
// checkout alone, and this test asserts agreement whichever root holds the
// file.
func TestResolve_DeclarationIsRepositoryWide(t *testing.T) {
	for _, tc := range []struct {
		name    string
		onMain  bool
		wantVia projectid.Rule
	}{
		{name: "declared only on the main checkout", onMain: true, wantVia: projectid.RuleDeclared},
		{name: "declared only on the linked worktree", onMain: false, wantVia: projectid.RuleRemote},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			repo := newRepo(t, tmp, "repo")
			// An origin remote gives the chain a SECOND rule to fall to, so a
			// disagreement shows up as two different identities rather than
			// as one lookup simply failing.
			git(t, repo, "remote", "add", "origin", "https://github.com/acme/widgets.git")
			wt := addWorktree(t, repo, filepath.Join(tmp, "wt"))

			root := repo
			if !tc.onMain {
				root = wt
			}
			// Untracked on purpose: committing it would put the file on both
			// branches and dissolve the asymmetry this test exists to create.
			write(t, filepath.Join(root, projectid.DeclaredFileName), "chosen-name\n")

			main, linked := resolve(t, repo), resolve(t, wt)
			if main.Project != linked.Project {
				t.Fatalf("a declaration on one root fragments the repository: main=%q (%s) worktree=%q (%s)",
					main.Project, main.Rule, linked.Project, linked.Rule)
			}
			if main.Rule != tc.wantVia {
				t.Errorf("resolved via %q, want %q -- the declaration belongs to the repository, so only the main checkout's copy counts",
					main.Rule, tc.wantVia)
			}
		})
	}
}

func TestResolve_MainCheckoutAndWorktreeAgree_Remote(t *testing.T) {
	tmp := t.TempDir()
	repo := newRepo(t, tmp, "repo")
	git(t, repo, "remote", "add", "origin", "git@github.com:acme/widgets.git")
	wt := addWorktree(t, repo, filepath.Join(tmp, "wt"))

	main, linked := resolve(t, repo), resolve(t, wt)
	if main.Project != linked.Project {
		t.Fatalf("remote rule fragments across worktrees: main=%q worktree=%q", main.Project, linked.Project)
	}
	if main.Rule != projectid.RuleRemote || linked.Rule != projectid.RuleRemote {
		t.Fatalf("rule = %q/%q, want %q for both", main.Rule, linked.Rule, projectid.RuleRemote)
	}
	if main.Project != "github.com/acme/widgets" {
		t.Fatalf("remote identity = %q, want %q", main.Project, "github.com/acme/widgets")
	}
}

func TestResolve_MainCheckoutAndWorktreeAgree_CommonDir(t *testing.T) {
	tmp := t.TempDir()
	repo := newRepo(t, tmp, "repo") // no declared file, no remote
	wt := addWorktree(t, repo, filepath.Join(tmp, "wt"))

	main, linked := resolve(t, repo), resolve(t, wt)
	if main.Project != linked.Project {
		t.Fatalf("common-dir rule fragments across worktrees: main=%q worktree=%q", main.Project, linked.Project)
	}
	if main.Rule != projectid.RuleCommonDir || linked.Rule != projectid.RuleCommonDir {
		t.Fatalf("rule = %q/%q, want %q for both", main.Rule, linked.Rule, projectid.RuleCommonDir)
	}
}

// --- (b) the measured trap: git's own answer is relative from the main
// checkout and absolute from a worktree; keeping either raw fragments. ---

func TestResolve_CommonDirIsAbsoluteAndSymlinkFree(t *testing.T) {
	tmp := t.TempDir()
	repo := newRepo(t, tmp, "repo")
	wt := addWorktree(t, repo, filepath.Join(tmp, "wt"))

	// The trap, re-measured here rather than assumed: git answers
	// relatively from the main checkout and absolutely from the worktree.
	fromMain := git(t, repo, "rev-parse", "--git-common-dir")
	fromWorktree := git(t, wt, "rev-parse", "--git-common-dir")
	if fromMain == fromWorktree {
		t.Skipf("git no longer reproduces the relative/absolute asymmetry (%q vs %q); the assertions below still stand on their own", fromMain, fromWorktree)
	}

	main, linked := resolve(t, repo), resolve(t, wt)
	if main.Project != linked.Project {
		t.Fatalf("common-dir identity fragments: main=%q worktree=%q (git answered %q vs %q)", main.Project, linked.Project, fromMain, fromWorktree)
	}
	if !filepath.IsAbs(main.Project) {
		t.Fatalf("common-dir identity %q is not absolute; git's own relative answer must never survive as a key", main.Project)
	}
	if strings.Contains(main.Project, "..") {
		t.Fatalf("common-dir identity %q still carries an unresolved %q segment", main.Project, "..")
	}
	wantReal, err := filepath.EvalSymlinks(filepath.Join(repo, ".git"))
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	if main.Project != wantReal {
		t.Fatalf("common-dir identity = %q, want the realpath %q", main.Project, wantReal)
	}
}

// TWIN: symlinks. A worktree reached through a symlinked path, and a
// repository reached through a symlinked parent, must still agree with the
// real paths. A test using only real paths would pass with the symlink
// resolution missing entirely.
func TestResolve_SymlinkedPathsAgree(t *testing.T) {
	tmp := t.TempDir()
	realParent := filepath.Join(tmp, "real")
	if err := os.MkdirAll(realParent, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	repo := newRepo(t, realParent, "repo")
	wt := addWorktree(t, repo, filepath.Join(tmp, "wt"))

	linkParent := filepath.Join(tmp, "link-parent")
	if err := os.Symlink(realParent, linkParent); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	linkWT := filepath.Join(tmp, "link-wt")
	if err := os.Symlink(wt, linkWT); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	want := resolve(t, repo).Project
	for _, dir := range []string{
		filepath.Join(linkParent, "repo"), // repo through a symlinked parent
		linkWT,                            // worktree through a symlinked path
		wt,
	} {
		if got := resolve(t, dir).Project; got != want {
			t.Fatalf("Resolve(%s) = %q, want %q -- symlinked paths must not fragment", dir, got, want)
		}
	}
}

// --- (c) remote normalization ---

func TestResolve_RemoteSpellingsCollapse(t *testing.T) {
	spellings := []string{
		"git@github.com:acme/widgets.git",
		"https://github.com/acme/widgets",
		"https://github.com/acme/widgets.git",
		"ssh://git@github.com/acme/widgets.git",
		"https://github.com/acme/widgets/",
	}
	const want = "github.com/acme/widgets"

	for _, url := range spellings {
		tmp := t.TempDir()
		repo := newRepo(t, tmp, "repo")
		git(t, repo, "remote", "add", "origin", url)
		got := resolve(t, repo)
		if got.Project != want {
			t.Errorf("remote %q normalized to %q, want %q", url, got.Project, want)
		}
		if got.Rule != projectid.RuleRemote {
			t.Errorf("remote %q resolved by rule %q, want %q", url, got.Rule, projectid.RuleRemote)
		}
	}
}

// --- (d) chain order ---

func TestResolve_ChainOrder(t *testing.T) {
	t.Run("declared beats remote", func(t *testing.T) {
		tmp := t.TempDir()
		repo := newRepo(t, tmp, "repo")
		git(t, repo, "remote", "add", "origin", "https://github.com/acme/widgets.git")
		write(t, filepath.Join(repo, projectid.DeclaredFileName), "chosen-name")

		got := resolve(t, repo)
		if got.Rule != projectid.RuleDeclared || got.Project != "chosen-name" {
			t.Fatalf("got %q via %q, want %q via %q -- the declared file is the only rule a human controls and must win", got.Project, got.Rule, "chosen-name", projectid.RuleDeclared)
		}
	})

	t.Run("remote beats common dir", func(t *testing.T) {
		tmp := t.TempDir()
		repo := newRepo(t, tmp, "repo")
		git(t, repo, "remote", "add", "origin", "https://github.com/acme/widgets.git")

		got := resolve(t, repo)
		if got.Rule != projectid.RuleRemote {
			t.Fatalf("got %q via %q, want rule %q", got.Project, got.Rule, projectid.RuleRemote)
		}
	})
}

func TestResolve_DeclaredGarbageIsRejected(t *testing.T) {
	for name, content := range map[string]string{
		"empty":      "",
		"whitespace": "   \n\t ",
		"multiline":  "one\ntwo\n",
	} {
		t.Run(name, func(t *testing.T) {
			tmp := t.TempDir()
			repo := newRepo(t, tmp, "repo")
			git(t, repo, "remote", "add", "origin", "https://github.com/acme/widgets.git")
			write(t, filepath.Join(repo, projectid.DeclaredFileName), content)

			id, err := projectid.Resolve(repo)
			if !errors.Is(err, projectid.ErrDeclaredInvalid) {
				t.Fatalf("Resolve with declared content %q = (%+v, %v), want ErrDeclaredInvalid -- a broken declaration must be reported, not silently skipped", content, id, err)
			}
		})
	}
}

// --- (e) different repositories never collide ---

func TestResolve_DifferentRepositoriesDiffer(t *testing.T) {
	tmp := t.TempDir()

	t.Run("common dir", func(t *testing.T) {
		a, b := newRepo(t, tmp, "a"), newRepo(t, tmp, "b")
		if resolve(t, a).Project == resolve(t, b).Project {
			t.Fatalf("two distinct repositories collided on %q", resolve(t, a).Project)
		}
	})

	t.Run("remote", func(t *testing.T) {
		a, b := newRepo(t, tmp, "ra"), newRepo(t, tmp, "rb")
		git(t, a, "remote", "add", "origin", "https://github.com/acme/widgets.git")
		git(t, b, "remote", "add", "origin", "https://github.com/acme/gadgets.git")
		if resolve(t, a).Project == resolve(t, b).Project {
			t.Fatalf("two distinct remotes collided on %q", resolve(t, a).Project)
		}
	})
}

// --- (f) not a repository: a named failure, never "" ---

func TestResolve_NotARepository(t *testing.T) {
	dir := t.TempDir()
	id, err := projectid.Resolve(dir)
	if !errors.Is(err, projectid.ErrNotARepository) {
		t.Fatalf("Resolve(%s) = (%+v, %v), want ErrNotARepository", dir, id, err)
	}
	if id.Project != "" {
		t.Fatalf("failed resolution still produced project %q; an empty or junk project name must never reach Engram", id.Project)
	}
}

func TestResolve_MissingDirectoryIsNamedFailure(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")
	if _, err := projectid.Resolve(dir); err == nil {
		t.Fatal("Resolve on a nonexistent directory returned no error")
	}
}

// --- (g) correspondence check ---

func TestCheckCorrespondence(t *testing.T) {
	tmp := t.TempDir()
	repo := newRepo(t, tmp, "repo")
	write(t, filepath.Join(repo, projectid.DeclaredFileName), "acme-widgets")

	t.Run("matching project is silent", func(t *testing.T) {
		c, err := projectid.CheckCorrespondence(repo, "acme-widgets")
		if err != nil {
			t.Fatalf("CheckCorrespondence: %v", err)
		}
		if !c.Match {
			t.Fatalf("CheckCorrespondence(%q) reported a mismatch against canonical %q", "acme-widgets", c.Canonical.Project)
		}
		if c.Warning() != "" {
			t.Fatalf("a matching project still produced a warning: %q", c.Warning())
		}
	})

	t.Run("mismatching project warns and names both", func(t *testing.T) {
		c, err := projectid.CheckCorrespondence(repo, "some-other-project")
		if err != nil {
			t.Fatalf("CheckCorrespondence: %v", err)
		}
		if c.Match {
			t.Fatal("CheckCorrespondence reported a match for a project that is not this repository's identity")
		}
		w := c.Warning()
		if !strings.Contains(w, "some-other-project") || !strings.Contains(w, "acme-widgets") {
			t.Fatalf("warning %q must name both the given project and the canonical identity", w)
		}
	})

	t.Run("outside a repository there is nothing to check", func(t *testing.T) {
		_, err := projectid.CheckCorrespondence(t.TempDir(), "whatever")
		if !errors.Is(err, projectid.ErrNotARepository) {
			t.Fatalf("CheckCorrespondence outside a repository = %v, want ErrNotARepository", err)
		}
	})

	t.Run("the repository's own directory name matches a path-derived identity", func(t *testing.T) {
		// A path- or remote-derived identity is never a string an
		// operator types. Warning on every such call would be pure
		// noise, so the repository's own name counts as corresponding.
		plain := newRepo(t, tmp, "plain-repo")
		c, err := projectid.CheckCorrespondence(plain, "plain-repo")
		if err != nil {
			t.Fatalf("CheckCorrespondence: %v", err)
		}
		if !c.Match {
			t.Fatalf("--project plain-repo mismatched its own repository, canonical %q via %q", c.Canonical.Project, c.Canonical.Rule)
		}
		c, err = projectid.CheckCorrespondence(plain, "unrelated")
		if err != nil {
			t.Fatalf("CheckCorrespondence: %v", err)
		}
		if c.Match {
			t.Fatal("an unrelated project name matched a path-derived identity")
		}
	})
}

// --- (f) shapes a real repository can have that must not read as "not a repository" ---

// A `.git` that is a SYMLINK to the real git directory is a layout git
// itself supports (a checkout whose metadata lives on another volume). The
// discovery walk lstats `.git` deliberately, so it can tell a main
// checkout's directory from a linked worktree's file -- but an lstat of a
// symlink reports neither, so the symlink fell into the worktree branch and
// was parsed for a "gitdir:" line it does not have. The whole repository
// then resolved as ErrNotARepository: not a wrong identity, no identity.
func TestResolve_SymlinkedGitDirectory(t *testing.T) {
	tmp := t.TempDir()
	root := newRepo(t, tmp, "widgets")
	write(t, filepath.Join(root, projectid.DeclaredFileName), "chosen-name\n")

	real := filepath.Join(tmp, "widgets-gitdir")
	if err := os.Rename(filepath.Join(root, ".git"), real); err != nil {
		t.Fatalf("moving the git directory aside: %v", err)
	}
	if err := os.Symlink(real, filepath.Join(root, ".git")); err != nil {
		t.Fatalf("symlinking the git directory back: %v", err)
	}

	id := resolve(t, root)
	if id.Project != "chosen-name" || id.Rule != projectid.RuleDeclared {
		t.Fatalf("a symlinked .git must resolve like a plain one: got %q (%s)", id.Project, id.Rule)
	}
}

// A remote whose url was repointed with an older line left in place is a
// shape git resolves by a rule worth pinning: for `remote.<name>.url` git
// takes the FIRST value, not the last, and fetches from it. That is the
// opposite of the last-wins rule config uses for ordinary single-valued
// keys, and getting it backwards would key the project on a url git itself
// never contacts. This test asserts the identity against git's own answer
// rather than against a hardcoded string, so it stays honest if that rule
// ever moves.
func TestResolve_OriginURLMatchesTheOneGitUses(t *testing.T) {
	tmp := t.TempDir()
	root := newRepo(t, tmp, "widgets")
	git(t, root, "remote", "add", "origin", "https://github.com/acme/widgets.git")
	config := filepath.Join(root, ".git", "config")
	raw, err := os.ReadFile(config)
	if err != nil {
		t.Fatalf("reading %s: %v", config, err)
	}
	write(t, config, strings.Replace(string(raw),
		"\turl = https://github.com/acme/widgets.git",
		"\turl = https://github.com/acme/widgets.git\n\turl = https://github.com/acme/superseded.git", 1))

	want := projectid.NormalizeRemote(git(t, root, "remote", "get-url", "origin"))
	if want != "github.com/acme/widgets" {
		t.Fatalf("fixture did not produce the shape under test: git reports %q", want)
	}

	id := resolve(t, root)
	if id.Project != want {
		t.Fatalf("identity must key on the url git itself uses: got %q, git uses %q", id.Project, want)
	}
}
