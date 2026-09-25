package gitprov

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const testHead = "0123456789abcdef0123456789abcdef01234567"

// fakeRepo is a git layout on disk plus the answers a fake runner gives for
// it. Tests mutate answers to model each refusal.
type fakeRepo struct {
	root    string
	answers map[string]string
	fail    map[string]error
	calls   []call
}

type call struct {
	dir  string
	env  []string
	args []string
}

func (f *fakeRepo) run(dir string, env []string, args []string) ([]byte, error) {
	f.calls = append(f.calls, call{dir: dir, env: append([]string(nil), env...), args: append([]string(nil), args...)})
	key := strings.Join(args, " ")
	if err, ok := f.fail[key]; ok {
		return nil, err
	}
	out, ok := f.answers[key]
	if !ok {
		return nil, errors.New("unexpected argv: " + key)
	}
	return []byte(out), nil
}

func (f *fakeRepo) observer(environ ...string) Observer {
	return Observer{Run: f.run, Environ: environ}
}

// newMainRepo builds a main worktree whose .git is a directory.
func newMainRepo(t *testing.T) *fakeRepo {
	t.Helper()
	root := resolvedTempDir(t)
	gitDir := filepath.Join(root, ".git")
	if err := os.Mkdir(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	return &fakeRepo{
		root: root,
		answers: map[string]string{
			"rev-parse --is-bare-repository":   "false\n",
			"rev-parse --is-inside-work-tree":  "true\n",
			"rev-parse --show-toplevel":        root + "\n",
			"rev-parse --absolute-git-dir":     gitDir + "\n",
			"rev-parse --git-common-dir":       ".git\n",
			"rev-parse --verify HEAD^{commit}": testHead + "\n",
		},
		fail: map[string]error{},
	}
}

// newLinkedRepo builds a linked worktree: root/.git is a file pointing at
// common/worktrees/wt, whose gitdir file points back at root/.git.
func newLinkedRepo(t *testing.T) (*fakeRepo, string, string) {
	t.Helper()
	base := resolvedTempDir(t)
	common := filepath.Join(base, "main", ".git")
	wtGitDir := filepath.Join(common, "worktrees", "wt")
	root := filepath.Join(base, "wt")
	for _, d := range []string{wtGitDir, root} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	writeFile(t, filepath.Join(root, ".git"), "gitdir: "+wtGitDir+"\n")
	writeFile(t, filepath.Join(wtGitDir, "gitdir"), filepath.Join(root, ".git")+"\n")
	return &fakeRepo{
		root: root,
		answers: map[string]string{
			"rev-parse --is-bare-repository":   "false\n",
			"rev-parse --is-inside-work-tree":  "true\n",
			"rev-parse --show-toplevel":        root + "\n",
			"rev-parse --absolute-git-dir":     wtGitDir + "\n",
			"rev-parse --git-common-dir":       common + "\n",
			"rev-parse --verify HEAD^{commit}": testHead + "\n",
		},
		fail: map[string]error{},
	}, common, wtGitDir
}

// newGitfileRepo builds a non-linked gitfile layout: root/.git is a file
// pointing at a separate main git dir that is its own common dir, the shape
// of both a forged gitfile and a separate-git-dir or submodule layout. The
// core.worktree lookup fails as git does when the key is unset.
func newGitfileRepo(t *testing.T) (*fakeRepo, string) {
	t.Helper()
	base := resolvedTempDir(t)
	common := filepath.Join(base, "main", ".git")
	root := filepath.Join(base, "forged")
	for _, d := range []string{common, root} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	writeFile(t, filepath.Join(root, ".git"), "gitdir: "+common+"\n")
	return &fakeRepo{
		root: root,
		answers: map[string]string{
			"rev-parse --is-bare-repository":   "false\n",
			"rev-parse --is-inside-work-tree":  "true\n",
			"rev-parse --show-toplevel":        root + "\n",
			"rev-parse --absolute-git-dir":     common + "\n",
			"rev-parse --git-common-dir":       common + "\n",
			"rev-parse --verify HEAD^{commit}": testHead + "\n",
		},
		fail: map[string]error{
			"config --local --get core.worktree": errors.New("exit status 1"),
		},
	}, common
}

func resolvedTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve temp dir: %v", err)
	}
	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestObserveRecordsMainWorktreeProvenance(t *testing.T) {
	repo := newMainRepo(t)

	got, err := repo.observer("HOME=/home/x").Observe(repo.root)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	want := Observation{
		Toplevel:  repo.root,
		GitDir:    filepath.Join(repo.root, ".git"),
		CommonDir: filepath.Join(repo.root, ".git"),
		Head:      testHead,
		Linked:    false,
	}
	if got != want {
		t.Errorf("Observe = %+v, want %+v", got, want)
	}
}

func TestObserveRecordsLinkedWorktreeProvenance(t *testing.T) {
	repo, common, wtGitDir := newLinkedRepo(t)

	got, err := repo.observer().Observe(repo.root)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	want := Observation{Toplevel: repo.root, GitDir: wtGitDir, CommonDir: common, Head: testHead, Linked: true}
	if got != want {
		t.Errorf("Observe = %+v, want %+v", got, want)
	}
}

func TestObserveAcceptsSymlinkedRootResolvingToToplevel(t *testing.T) {
	repo := newMainRepo(t)
	link := filepath.Join(resolvedTempDir(t), "link")
	if err := os.Symlink(repo.root, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	got, err := repo.observer().Observe(link)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if got.Toplevel != repo.root {
		t.Errorf("Toplevel = %q, want %q", got.Toplevel, repo.root)
	}
	if repo.calls[0].dir != repo.root {
		t.Errorf("runner dir = %q, want resolved root %q", repo.calls[0].dir, repo.root)
	}
}

func TestObserveRunsFixedArgvInResolvedRoot(t *testing.T) {
	repo := newMainRepo(t)

	if _, err := repo.observer().Observe(repo.root); err != nil {
		t.Fatalf("Observe: %v", err)
	}
	want := [][]string{
		{"rev-parse", "--is-bare-repository"},
		{"rev-parse", "--is-inside-work-tree"},
		{"rev-parse", "--show-toplevel"},
		{"rev-parse", "--absolute-git-dir"},
		{"rev-parse", "--git-common-dir"},
		{"rev-parse", "--verify", "HEAD^{commit}"},
	}
	var got [][]string
	for _, c := range repo.calls {
		got = append(got, c.args)
		if c.dir != repo.root {
			t.Errorf("call %v ran in %q, want %q", c.args, c.dir, repo.root)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("argv = %v, want %v", got, want)
	}
}

func TestObserveScrubsGitEnvironmentFromRunner(t *testing.T) {
	repo := newMainRepo(t)
	environ := []string{
		"HOME=/home/x",
		"PATH=/usr/bin",
		"GIT_CEILING_DIRECTORIES=/",
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=core.fsmonitor",
		"GIT_CONFIG_VALUE_0=/tmp/evil",
		"GIT_CONFIG_PARAMETERS='core.fsmonitor'='/tmp/evil'",
		"GIT_CONFIG_GLOBAL=/tmp/evil",
		"GIT_EXEC_PATH=/tmp/evil",
		"GIT_OBJECT_DIRECTORY=/tmp/evil",
		"GIT_INDEX_FILE=/tmp/evil",
	}

	if _, err := repo.observer(environ...).Observe(repo.root); err != nil {
		t.Fatalf("Observe: %v", err)
	}
	want := []string{"HOME=/home/x", "PATH=/usr/bin"}
	for _, c := range repo.calls {
		if !reflect.DeepEqual(c.env, want) {
			t.Errorf("call %v env = %v, want %v", c.args, c.env, want)
		}
	}
}

func TestObserveRefusesRepositoryRedirectingEnvironment(t *testing.T) {
	for _, name := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR"} {
		t.Run(name, func(t *testing.T) {
			repo := newMainRepo(t)

			_, err := repo.observer("HOME=/home/x", name+"=/elsewhere").Observe(repo.root)
			if err == nil {
				t.Fatal("Observe succeeded with a repository-redirecting variable present")
			}
			if !strings.Contains(err.Error(), name) {
				t.Errorf("error %q does not name %s", err, name)
			}
			if len(repo.calls) != 0 {
				t.Errorf("runner was called %d times; want refusal before any git invocation", len(repo.calls))
			}
		})
	}
}

func TestObserveFailsClosedOnAmbiguousRepositories(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(t *testing.T, r *fakeRepo)
		want   string
	}{
		{"bare_repository", func(t *testing.T, r *fakeRepo) {
			r.answers["rev-parse --is-bare-repository"] = "true\n"
		}, "bare"},
		{"outside_work_tree", func(t *testing.T, r *fakeRepo) {
			r.answers["rev-parse --is-inside-work-tree"] = "false\n"
		}, "work tree"},
		{"subdirectory_root", func(t *testing.T, r *fakeRepo) {
			r.answers["rev-parse --show-toplevel"] = filepath.Dir(r.root) + "\n"
		}, "toplevel"},
		{"relative_toplevel", func(t *testing.T, r *fakeRepo) {
			r.answers["rev-parse --show-toplevel"] = "repo\n"
		}, "absolute"},
		{"multiline_output", func(t *testing.T, r *fakeRepo) {
			r.answers["rev-parse --show-toplevel"] = r.root + "\n" + r.root + "\n"
		}, "single line"},
		{"empty_output", func(t *testing.T, r *fakeRepo) {
			r.answers["rev-parse --absolute-git-dir"] = "\n"
		}, "empty"},
		{"git_dir_differs_from_dot_git", func(t *testing.T, r *fakeRepo) {
			other := filepath.Join(r.root, "other")
			if err := os.Mkdir(other, 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			r.answers["rev-parse --absolute-git-dir"] = other + "\n"
			r.answers["rev-parse --git-common-dir"] = other + "\n"
		}, ".git"},
		{"common_dir_differs_without_linked_layout", func(t *testing.T, r *fakeRepo) {
			other := filepath.Join(r.root, "other")
			if err := os.Mkdir(other, 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			r.answers["rev-parse --git-common-dir"] = other + "\n"
		}, "worktrees"},
		{"unborn_head", func(t *testing.T, r *fakeRepo) {
			r.fail["rev-parse --verify HEAD^{commit}"] = errors.New("exit status 128")
		}, "HEAD"},
		{"malformed_head", func(t *testing.T, r *fakeRepo) {
			r.answers["rev-parse --verify HEAD^{commit}"] = "ABCDEF\n"
		}, "HEAD"},
		{"missing_git_binary", func(t *testing.T, r *fakeRepo) {
			r.fail["rev-parse --is-bare-repository"] = exec.ErrNotFound
		}, "git"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMainRepo(t)
			tc.mutate(t, repo)

			got, err := repo.observer().Observe(repo.root)
			if err == nil {
				t.Fatalf("Observe succeeded with %+v; want refusal", got)
			}
			if got != (Observation{}) {
				t.Errorf("Observe returned partial observation %+v alongside error", got)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

func TestObserveFailsClosedOnLinkedWorktreeMismatch(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(t *testing.T, r *fakeRepo, common, wtGitDir string)
		want   string
	}{
		{"back_pointer_names_another_worktree", func(t *testing.T, r *fakeRepo, common, wtGitDir string) {
			otherDotGit := filepath.Join(filepath.Dir(r.root), "other", ".git")
			if err := os.MkdirAll(filepath.Dir(otherDotGit), 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			writeFile(t, otherDotGit, "gitdir: "+wtGitDir+"\n")
			writeFile(t, filepath.Join(wtGitDir, "gitdir"), otherDotGit+"\n")
		}, "linked worktree mismatch"},
		{"back_pointer_missing", func(t *testing.T, r *fakeRepo, common, wtGitDir string) {
			if err := os.Remove(filepath.Join(wtGitDir, "gitdir")); err != nil {
				t.Fatalf("remove: %v", err)
			}
		}, filepath.Join("worktrees", "wt", "gitdir")},
		{"dot_git_file_names_another_git_dir", func(t *testing.T, r *fakeRepo, common, wtGitDir string) {
			other := filepath.Join(common, "worktrees", "other")
			if err := os.MkdirAll(other, 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			writeFile(t, filepath.Join(r.root, ".git"), "gitdir: "+other+"\n")
		}, "not the reported git dir"},
		{"dot_git_file_malformed", func(t *testing.T, r *fakeRepo, common, wtGitDir string) {
			writeFile(t, filepath.Join(r.root, ".git"), "not a gitdir line\n")
		}, "malformed pointer"},
		{"git_dir_outside_common_worktrees", func(t *testing.T, r *fakeRepo, common, wtGitDir string) {
			stray := filepath.Join(filepath.Dir(common), "stray")
			if err := os.MkdirAll(stray, 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			writeFile(t, filepath.Join(stray, "gitdir"), filepath.Join(r.root, ".git")+"\n")
			writeFile(t, filepath.Join(r.root, ".git"), "gitdir: "+stray+"\n")
			r.answers["rev-parse --absolute-git-dir"] = stray + "\n"
		}, "is not under"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo, common, wtGitDir := newLinkedRepo(t)
			tc.mutate(t, repo, common, wtGitDir)

			got, err := repo.observer().Observe(repo.root)
			if err == nil {
				t.Fatalf("Observe succeeded with %+v; want refusal", got)
			}
			if got != (Observation{}) {
				t.Errorf("Observe returned partial observation %+v alongside error", got)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

func TestObserveFailsClosedOnForgedGitfile(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(t *testing.T, r *fakeRepo, common string)
		want   string
	}{
		{"gitfile_names_main_git_dir_without_core_worktree", func(t *testing.T, r *fakeRepo, common string) {}, "core.worktree"},
		{"core_worktree_names_another_directory", func(t *testing.T, r *fakeRepo, common string) {
			delete(r.fail, "config --local --get core.worktree")
			r.answers["config --local --get core.worktree"] = filepath.Dir(common) + "\n"
		}, "core.worktree"},
		{"core_worktree_multiline", func(t *testing.T, r *fakeRepo, common string) {
			delete(r.fail, "config --local --get core.worktree")
			r.answers["config --local --get core.worktree"] = r.root + "\n" + r.root + "\n"
		}, "single line"},
		{"core_worktree_missing_directory", func(t *testing.T, r *fakeRepo, common string) {
			delete(r.fail, "config --local --get core.worktree")
			r.answers["config --local --get core.worktree"] = filepath.Join(r.root, "missing") + "\n"
		}, "core.worktree"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo, common := newGitfileRepo(t)
			tc.mutate(t, repo, common)

			got, err := repo.observer().Observe(repo.root)
			if err == nil {
				t.Fatalf("Observe succeeded with %+v; want refusal", got)
			}
			if got != (Observation{}) {
				t.Errorf("Observe returned partial observation %+v alongside error", got)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

func TestObserveAcceptsGitfileWhoseCoreWorktreeNamesToplevel(t *testing.T) {
	cases := []struct {
		name  string
		value func(r *fakeRepo, common string) string
	}{
		{"absolute", func(r *fakeRepo, common string) string { return r.root }},
		{"relative_to_git_dir", func(r *fakeRepo, common string) string {
			rel, err := filepath.Rel(common, r.root)
			if err != nil {
				t.Fatalf("rel: %v", err)
			}
			return rel
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo, common := newGitfileRepo(t)
			delete(repo.fail, "config --local --get core.worktree")
			repo.answers["config --local --get core.worktree"] = tc.value(repo, common) + "\n"

			got, err := repo.observer().Observe(repo.root)
			if err != nil {
				t.Fatalf("Observe: %v", err)
			}
			want := Observation{Toplevel: repo.root, GitDir: common, CommonDir: common, Head: testHead, Linked: false}
			if got != want {
				t.Errorf("Observe = %+v, want %+v", got, want)
			}
		})
	}
}

func TestObserveRefusesCopiedLinkedWorktreeGitfile(t *testing.T) {
	repo, _, _ := newLinkedRepo(t)
	copied := filepath.Join(filepath.Dir(repo.root), "copied")
	if err := os.Mkdir(copied, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(repo.root, ".git"))
	if err != nil {
		t.Fatalf("read gitfile: %v", err)
	}
	writeFile(t, filepath.Join(copied, ".git"), string(data))
	repo.root = copied
	repo.answers["rev-parse --show-toplevel"] = copied + "\n"

	got, err := repo.observer().Observe(copied)
	if err == nil {
		t.Fatalf("Observe succeeded with %+v; want refusal", got)
	}
	if !strings.Contains(err.Error(), "linked worktree mismatch") {
		t.Errorf("error %q does not mention the linked worktree mismatch", err)
	}
}

func TestObserveRejectsInvalidRootAndRunner(t *testing.T) {
	repo := newMainRepo(t)
	cases := []struct {
		name string
		obs  Observer
		root string
	}{
		{"empty_root", repo.observer(), ""},
		{"relative_root", repo.observer(), "repo"},
		{"missing_root", repo.observer(), filepath.Join(repo.root, "missing")},
		{"nil_runner", Observer{}, repo.root},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := tc.obs.Observe(tc.root); err == nil {
				t.Fatalf("Observe(%q) = %+v; want refusal", tc.root, got)
			}
		})
	}
}

func TestObserveFailsClosedWhenGitIsNotOnPath(t *testing.T) {
	root := resolvedTempDir(t)
	t.Setenv("PATH", "")

	if got, err := Observe(root); err == nil {
		t.Fatalf("Observe = %+v; want refusal without a git binary", got)
	}
}

// TestObserveMatchesRealGitRepository is the one optional integration test:
// it drives a real git binary and skips when none is installed.
func TestObserveMatchesRealGitRepository(t *testing.T) {
	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git binary not available")
	}
	home := resolvedTempDir(t)
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	// Clear ambient GIT_* so the setup git commands and Observe both run
	// against the temp repositories; t.Setenv restores them afterwards.
	for _, kv := range os.Environ() {
		if name, _, _ := strings.Cut(kv, "="); strings.HasPrefix(name, "GIT_") {
			t.Setenv(name, "")
			os.Unsetenv(name)
		}
	}
	base := resolvedTempDir(t)
	main := filepath.Join(base, "main")
	wt := filepath.Join(base, "wt")
	gitRun := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command(gitBin, append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.invalid", "-c", "commit.gpgsign=false"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.Mkdir(main, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	gitRun(main, "init", "-q")
	gitRun(main, "commit", "-q", "--allow-empty", "-m", "init")
	gitRun(main, "worktree", "add", "-q", wt)

	t.Run("main_worktree", func(t *testing.T) {
		got, err := Observe(main)
		if err != nil {
			t.Fatalf("Observe: %v", err)
		}
		if got.Toplevel != main || got.GitDir != filepath.Join(main, ".git") || got.Linked {
			t.Errorf("Observe = %+v", got)
		}
		if len(got.Head) != 40 && len(got.Head) != 64 {
			t.Errorf("Head = %q, want a full object name", got.Head)
		}
	})
	t.Run("linked_worktree", func(t *testing.T) {
		got, err := Observe(wt)
		if err != nil {
			t.Fatalf("Observe: %v", err)
		}
		if got.Toplevel != wt || got.CommonDir != filepath.Join(main, ".git") || !got.Linked {
			t.Errorf("Observe = %+v", got)
		}
	})
	t.Run("subdirectory_root_refused", func(t *testing.T) {
		sub := filepath.Join(main, "sub")
		if err := os.Mkdir(sub, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if got, err := Observe(sub); err == nil {
			t.Fatalf("Observe(subdir) = %+v; want refusal", got)
		}
	})
	t.Run("forged_gitfile_naming_main_git_dir_refused", func(t *testing.T) {
		forged := filepath.Join(base, "forged")
		if err := os.Mkdir(forged, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		writeFile(t, filepath.Join(forged, ".git"), "gitdir: "+filepath.Join(main, ".git")+"\n")
		if got, err := Observe(forged); err == nil {
			t.Fatalf("Observe(forged) = %+v; want refusal", got)
		}
	})
	t.Run("copied_linked_gitfile_refused", func(t *testing.T) {
		copied := filepath.Join(base, "copied")
		if err := os.Mkdir(copied, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		data, err := os.ReadFile(filepath.Join(wt, ".git"))
		if err != nil {
			t.Fatalf("read gitfile: %v", err)
		}
		writeFile(t, filepath.Join(copied, ".git"), string(data))
		if got, err := Observe(copied); err == nil {
			t.Fatalf("Observe(copied) = %+v; want refusal", got)
		}
	})
	t.Run("separate_git_dir_without_core_worktree_refused", func(t *testing.T) {
		sepWT := filepath.Join(base, "sepwt")
		gitRun(base, "init", "-q", "--separate-git-dir="+filepath.Join(base, "sep.git"), sepWT)
		gitRun(sepWT, "commit", "-q", "--allow-empty", "-m", "init")
		if got, err := Observe(sepWT); err == nil {
			t.Fatalf("Observe(separate git dir) = %+v; want refusal", got)
		}
	})
	t.Run("separate_git_dir_with_core_worktree_accepted", func(t *testing.T) {
		sepWT := filepath.Join(base, "sepwt2")
		sepGit := filepath.Join(base, "sep2.git")
		gitRun(base, "init", "-q", "--separate-git-dir="+sepGit, sepWT)
		gitRun(sepWT, "commit", "-q", "--allow-empty", "-m", "init")
		gitRun(sepWT, "config", "core.worktree", sepWT)
		got, err := Observe(sepWT)
		if err != nil {
			t.Fatalf("Observe: %v", err)
		}
		if got.Toplevel != sepWT || got.GitDir != sepGit || got.CommonDir != sepGit || got.Linked {
			t.Errorf("Observe = %+v", got)
		}
	})
	t.Run("bare_repository_refused", func(t *testing.T) {
		bare := filepath.Join(base, "bare.git")
		gitRun(base, "init", "-q", "--bare", bare)
		if got, err := Observe(bare); err == nil {
			t.Fatalf("Observe(bare) = %+v; want refusal", got)
		}
	})
}
