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
