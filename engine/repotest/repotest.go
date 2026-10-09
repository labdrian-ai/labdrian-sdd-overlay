// Package repotest holds the hand-made repositories the tests of the code that finds a repository
// share (engine/gitfs and engine/cmd). A fixture is a plain directory with a .git directory or a
// .git file in it, never a real git repository and never the git binary: the code under test
// reads files and must work, or fail soft, without one. It is test support, imported by test files
// only.
package repotest

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// KeyShape is what a repository key looks like: 64 lowercase hexadecimal digits.
var KeyShape = regexp.MustCompile(`^[0-9a-f]{64}$`)

// KeyLocator is what MustKey asks for a key: the RepoKey method of the locator under test.
type KeyLocator interface {
	RepoKey(dir string) (string, bool)
}

// WriteFile writes content to path, making the directories above it.
func WriteFile(t testing.TB, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("os.MkdirAll(%q) = %v, want nil", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("os.WriteFile(%q) = %v, want nil", path, err)
	}
}

// WantKey is the key the design specifies for a git common directory: the lowercase hex SHA-256
// of its symlink-resolved path. The tests compute it independently of the code that finds it,
// from the fixture they built.
func WantKey(t testing.TB, commonDir string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(commonDir)
	if err != nil {
		t.Fatalf("filepath.EvalSymlinks(%q) = %v, want nil", commonDir, err)
	}
	sum := sha256.Sum256([]byte(resolved))
	return hex.EncodeToString(sum[:])
}

// Repo builds a hand-made plain repository (a .git directory with a HEAD) named name under a
// fresh temporary directory, and returns its root.
func Repo(t testing.TB, name string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), name)
	WriteFile(t, filepath.Join(root, ".git", "HEAD"), strings.Repeat("a", 40)+"\n")
	return root
}

// LinkedWorktree adds a linked worktree called name to the repository at mainRoot, in the layout
// `git worktree add` creates: the worktree's .git file points at .git/worktrees/<name>, whose
// commondir file leads back to the main .git. It returns the worktree root.
func LinkedWorktree(t testing.TB, mainRoot, name string) string {
	t.Helper()
	worktreeGitDir := filepath.Join(mainRoot, ".git", "worktrees", name)
	WriteFile(t, filepath.Join(worktreeGitDir, "HEAD"), strings.Repeat("b", 40)+"\n")
	WriteFile(t, filepath.Join(worktreeGitDir, "commondir"), "../..\n")
	worktree := filepath.Join(t.TempDir(), name)
	WriteFile(t, filepath.Join(worktree, ".git"), "gitdir: "+worktreeGitDir+"\n")
	return worktree
}

// MustKey asks the locator for the key of the repository that holds cwd and fails the test when
// there is none or when the key is not of the shape of a key.
func MustKey(t testing.TB, locator KeyLocator, cwd string) string {
	t.Helper()
	key, ok := locator.RepoKey(cwd)
	if !ok {
		t.Fatalf("RepoKey(%q) = _, false, want a key", cwd)
	}
	if !KeyShape.MatchString(key) {
		t.Fatalf("RepoKey(%q) = %q, want 64 lowercase hex characters", cwd, key)
	}
	return key
}
