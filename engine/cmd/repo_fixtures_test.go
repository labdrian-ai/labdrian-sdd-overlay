package main

// Fixture repos here are hand-built plain directories and files, never a real git repository and
// never the git binary: the commands find a repository by reading its files (engine/gitfs).

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
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

var repoKeyShape = regexp.MustCompile(`^[0-9a-f]{64}$`)

// wantRepoKey is the key the design specifies for a git common directory: the
// lowercase hex SHA-256 of its symlink-resolved path. The tests compute it
// independently of the adapter that finds it, from the fixture they built.
func wantRepoKey(t *testing.T, commonDir string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(commonDir)
	if err != nil {
		t.Fatalf("filepath.EvalSymlinks(%q) = %v, want nil", commonDir, err)
	}
	sum := sha256.Sum256([]byte(resolved))
	return hex.EncodeToString(sum[:])
}

// fixtureRepo builds a hand-made plain repository (a .git directory with a
// HEAD) named name under a fresh temporary directory, and returns its root.
func fixtureRepo(t *testing.T, name string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), name)
	writeFixtureFile(t, filepath.Join(root, ".git", "HEAD"), strings.Repeat("a", 40)+"\n")
	return root
}

// fixtureLinkedWorktree adds a linked worktree called name to the repository
// at mainRoot, in the layout `git worktree add` creates: the worktree's .git
// file points at .git/worktrees/<name>, whose commondir file leads back to the
// main .git. It returns the worktree root.
func fixtureLinkedWorktree(t *testing.T, mainRoot, name string) string {
	t.Helper()
	worktreeGitDir := filepath.Join(mainRoot, ".git", "worktrees", name)
	writeFixtureFile(t, filepath.Join(worktreeGitDir, "HEAD"), strings.Repeat("b", 40)+"\n")
	writeFixtureFile(t, filepath.Join(worktreeGitDir, "commondir"), "../..\n")
	worktree := filepath.Join(t.TempDir(), name)
	writeFixtureFile(t, filepath.Join(worktree, ".git"), "gitdir: "+worktreeGitDir+"\n")
	return worktree
}

func mustRepoKey(t *testing.T, cwd string) string {
	t.Helper()
	key, ok := newRepoLocator().RepoKey(cwd)
	if !ok {
		t.Fatalf("RepoKey(%q) = _, false, want a key", cwd)
	}
	if !repoKeyShape.MatchString(key) {
		t.Fatalf("RepoKey(%q) = %q, want 64 lowercase hex characters", cwd, key)
	}
	return key
}
