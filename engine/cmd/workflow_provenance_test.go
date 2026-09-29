package main

// Fixture repos here are hand-built plain directories and files, never a
// real git repository and never the git binary: observeProvenance must work
// (or fail soft) without one.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFixtureFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("os.MkdirAll(%q) = %v, want nil", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("os.WriteFile(%q) = %v, want nil", path, err)
	}
}

func TestObserveProvenanceRejectsRelativeOrEmptyCwd(t *testing.T) {
	for _, cwd := range []string{"", "relative/path", "."} {
		p := observeProvenance(cwd)
		if p.WorktreeRoot != "" || p.GitHead != "" {
			t.Fatalf("observeProvenance(%q) = %+v, want empty Provenance", cwd, p)
		}
	}
}

func TestObserveProvenanceDetachedHead(t *testing.T) {
	root := t.TempDir()
	head := strings.Repeat("a", 40)
	writeFixtureFile(t, filepath.Join(root, ".git", "HEAD"), head+"\n")

	p := observeProvenance(root)
	if p.WorktreeRoot != root {
		t.Fatalf("WorktreeRoot = %q, want %q", p.WorktreeRoot, root)
	}
	if p.GitHead != head {
		t.Fatalf("GitHead = %q, want %q", p.GitHead, head)
	}
}

func TestObserveProvenanceWalksUpFromSubdirectory(t *testing.T) {
	root := t.TempDir()
	head := strings.Repeat("b", 64)
	writeFixtureFile(t, filepath.Join(root, ".git", "HEAD"), head+"\n")
	sub := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatalf("os.MkdirAll() = %v, want nil", err)
	}

	p := observeProvenance(sub)
	if p.WorktreeRoot != root {
		t.Fatalf("WorktreeRoot = %q, want %q (found by walking up from %q)", p.WorktreeRoot, root, sub)
	}
	if p.GitHead != head {
		t.Fatalf("GitHead = %q, want %q", p.GitHead, head)
	}
}

func TestObserveProvenanceSymbolicRefResolvedFromLooseRefFile(t *testing.T) {
	root := t.TempDir()
	head := strings.Repeat("c", 40)
	writeFixtureFile(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")
	writeFixtureFile(t, filepath.Join(root, ".git", "refs", "heads", "main"), head+"\n")

	p := observeProvenance(root)
	if p.GitHead != head {
		t.Fatalf("GitHead = %q, want %q (resolved from the loose ref file)", p.GitHead, head)
	}
}

func TestObserveProvenanceSymbolicRefResolvedFromPackedRefs(t *testing.T) {
	root := t.TempDir()
	head := strings.Repeat("d", 40)
	writeFixtureFile(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")
	// No loose refs/heads/main file: only packed-refs names it.
	writeFixtureFile(t, filepath.Join(root, ".git", "packed-refs"),
		"# pack-refs with: peeled fully-peeled sorted\n"+
			strings.Repeat("e", 40)+" refs/heads/other\n"+
			head+" refs/heads/main\n"+
			"^"+strings.Repeat("f", 40)+"\n")

	p := observeProvenance(root)
	if p.GitHead != head {
		t.Fatalf("GitHead = %q, want %q (resolved from packed-refs)", p.GitHead, head)
	}
}

func TestObserveProvenanceLinkedWorktreeGitfile(t *testing.T) {
	worktree := filepath.Join(t.TempDir(), "worktree")
	actualGitDir := filepath.Join(t.TempDir(), "main-repo", ".git", "worktrees", "worktree")
	head := strings.Repeat("1", 40)
	writeFixtureFile(t, filepath.Join(actualGitDir, "HEAD"), head+"\n")
	writeFixtureFile(t, filepath.Join(worktree, ".git"), "gitdir: "+actualGitDir+"\n")

	p := observeProvenance(worktree)
	if p.WorktreeRoot != worktree {
		t.Fatalf("WorktreeRoot = %q, want %q (this worktree's own root, not the main repo)", p.WorktreeRoot, worktree)
	}
	if p.GitHead != head {
		t.Fatalf("GitHead = %q, want %q", p.GitHead, head)
	}
}

// TestObserveProvenanceLinkedWorktreeSymbolicRefUsesCommonDir mirrors the
// layout `git worktree add` creates: the worktree's gitdir holds a symbolic
// HEAD and a commondir file, while the branch ref (loose or packed) lives in
// the main repository's .git.
func TestObserveProvenanceLinkedWorktreeSymbolicRefUsesCommonDir(t *testing.T) {
	for _, packed := range []bool{false, true} {
		root := t.TempDir()
		mainGit := filepath.Join(root, "main-repo", ".git")
		worktreeGitDir := filepath.Join(mainGit, "worktrees", "wt")
		worktree := filepath.Join(root, "wt")
		head := strings.Repeat("2", 40)
		writeFixtureFile(t, filepath.Join(worktreeGitDir, "HEAD"), "ref: refs/heads/feat/x\n")
		writeFixtureFile(t, filepath.Join(worktreeGitDir, "commondir"), "../..\n")
		if packed {
			writeFixtureFile(t, filepath.Join(mainGit, "packed-refs"), "# pack-refs with: peeled\n"+head+" refs/heads/feat/x\n")
		} else {
			writeFixtureFile(t, filepath.Join(mainGit, "refs", "heads", "feat", "x"), head+"\n")
		}
		writeFixtureFile(t, filepath.Join(worktree, ".git"), "gitdir: "+worktreeGitDir+"\n")

		if p := observeProvenance(worktree); p.GitHead != head {
			t.Errorf("packed=%v: GitHead = %q, want %q resolved through commondir", packed, p.GitHead, head)
		}
	}
}

func TestObserveProvenanceMissingGitYieldsEmpty(t *testing.T) {
	dir := t.TempDir()
	p := observeProvenance(dir)
	if p.WorktreeRoot != "" || p.GitHead != "" {
		t.Fatalf("observeProvenance(%q) = %+v, want empty Provenance (no .git anywhere above it)", dir, p)
	}
}

func TestObserveProvenanceMalformedGitfileYieldsEmpty(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, filepath.Join(root, ".git"), "not a gitdir pointer\n")

	p := observeProvenance(root)
	if p.WorktreeRoot != "" || p.GitHead != "" {
		t.Fatalf("observeProvenance() = %+v, want empty Provenance for a malformed .git file", p)
	}
}

func TestObserveProvenanceGitfilePointingNowhereYieldsEmpty(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, filepath.Join(root, ".git"), "gitdir: "+filepath.Join(root, "does-not-exist")+"\n")

	p := observeProvenance(root)
	if p.WorktreeRoot != "" || p.GitHead != "" {
		t.Fatalf("observeProvenance() = %+v, want empty Provenance when the gitdir target does not exist", p)
	}
}

func TestObserveProvenanceUnreadableHeadYieldsEmptyHeadOnly(t *testing.T) {
	root := t.TempDir()
	// .git exists as a directory but HEAD is missing entirely.
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatalf("os.MkdirAll() = %v, want nil", err)
	}

	p := observeProvenance(root)
	if p.WorktreeRoot != root {
		t.Fatalf("WorktreeRoot = %q, want %q (the worktree is still observed even without HEAD)", p.WorktreeRoot, root)
	}
	if p.GitHead != "" {
		t.Fatalf("GitHead = %q, want empty when HEAD is missing", p.GitHead)
	}
}

func TestObserveProvenanceHeadNotHexYieldsEmptyHead(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, filepath.Join(root, ".git", "HEAD"), "garbage\n")

	p := observeProvenance(root)
	if p.GitHead != "" {
		t.Fatalf("GitHead = %q, want empty for a HEAD that is neither a hex object id nor a ref line", p.GitHead)
	}
}

func TestObserveProvenanceUnresolvableSymbolicRefYieldsEmptyHead(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/does-not-exist\n")

	p := observeProvenance(root)
	if p.WorktreeRoot != root {
		t.Fatalf("WorktreeRoot = %q, want %q", p.WorktreeRoot, root)
	}
	if p.GitHead != "" {
		t.Fatalf("GitHead = %q, want empty when the ref resolves nowhere", p.GitHead)
	}
}

// TestObserveProvenanceFollowsSymlinkedGitDirectory covers the decision
// that a symlinked .git IS followed, matching git's own behavior: git
// itself does not care whether .git is a plain directory or a symlink to
// one, so this helper must not either.
func TestObserveProvenanceFollowsSymlinkedGitDirectory(t *testing.T) {
	root := t.TempDir()
	actual := filepath.Join(t.TempDir(), "actual-git-dir")
	head := strings.Repeat("3", 40)
	writeFixtureFile(t, filepath.Join(actual, "HEAD"), head+"\n")
	if err := os.Symlink(actual, filepath.Join(root, ".git")); err != nil {
		t.Skipf("os.Symlink() = %v, symlinks unsupported here", err)
	}

	p := observeProvenance(root)
	if p.WorktreeRoot != root {
		t.Fatalf("WorktreeRoot = %q, want %q", p.WorktreeRoot, root)
	}
	if p.GitHead != head {
		t.Fatalf("GitHead = %q, want %q (a symlinked .git directory must be followed)", p.GitHead, head)
	}
}

// TestObserveProvenanceFollowsSymlinkedGitfile covers a symlink named .git
// that itself points at a gitdir-pointer FILE (rather than a directory),
// the shape a hand-rolled or unusual worktree tool might produce.
func TestObserveProvenanceFollowsSymlinkedGitfile(t *testing.T) {
	root := t.TempDir()
	actualGitDir := filepath.Join(t.TempDir(), "main-repo", ".git", "worktrees", "worktree")
	head := strings.Repeat("4", 40)
	writeFixtureFile(t, filepath.Join(actualGitDir, "HEAD"), head+"\n")
	gitfile := filepath.Join(t.TempDir(), "real.gitfile")
	writeFixtureFile(t, gitfile, "gitdir: "+actualGitDir+"\n")
	if err := os.Symlink(gitfile, filepath.Join(root, ".git")); err != nil {
		t.Skipf("os.Symlink() = %v, symlinks unsupported here", err)
	}

	p := observeProvenance(root)
	if p.WorktreeRoot != root {
		t.Fatalf("WorktreeRoot = %q, want %q", p.WorktreeRoot, root)
	}
	if p.GitHead != head {
		t.Fatalf("GitHead = %q, want %q (a symlinked .git pointer file must be followed)", p.GitHead, head)
	}
}

// TestObserveProvenanceGitdirPointerRelativeToBase covers readGitdirPointer's
// relative-target branch: the common on-disk shape `git worktree add`
// actually produces, where the "gitdir:" line names a path relative to the
// worktree root rather than an absolute one.
func TestObserveProvenanceGitdirPointerRelativeToBase(t *testing.T) {
	root := t.TempDir()
	worktree := filepath.Join(root, "wt")
	actualGitDir := filepath.Join(root, "main-repo", ".git", "worktrees", "wt")
	head := strings.Repeat("5", 40)
	writeFixtureFile(t, filepath.Join(actualGitDir, "HEAD"), head+"\n")
	rel, err := filepath.Rel(worktree, actualGitDir)
	if err != nil {
		t.Fatalf("filepath.Rel() = %v, want nil", err)
	}
	writeFixtureFile(t, filepath.Join(worktree, ".git"), "gitdir: "+rel+"\n")

	p := observeProvenance(worktree)
	if p.WorktreeRoot != worktree {
		t.Fatalf("WorktreeRoot = %q, want %q", p.WorktreeRoot, worktree)
	}
	if p.GitHead != head {
		t.Fatalf("GitHead = %q, want %q (resolved from a relative gitdir pointer)", p.GitHead, head)
	}
}
