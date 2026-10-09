package main

// The fixture repositories the commands' tests use are engine/repotest's: hand-built plain
// directories and files, never a real git repository and never the git binary, because the
// commands find a repository by reading its files (engine/gitfs). These are the names this
// package's tests have always called them by.

import (
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/repotest"
)

// writeFixtureFile writes content to path, making the directories above it.
func writeFixtureFile(t *testing.T, path, content string) {
	t.Helper()
	repotest.WriteFile(t, path, content)
}

// wantRepoKey is the key the design specifies for a git common directory: the hex SHA-256 of its
// symbolic-link-resolved path, computed here from the fixture and not by the code under test.
func wantRepoKey(t *testing.T, commonDir string) string {
	t.Helper()
	return repotest.WantKey(t, commonDir)
}

// fixtureRepo builds a plain repository called name under a fresh temporary directory: a .git
// directory holding a HEAD of forty a's. It returns the root.
func fixtureRepo(t *testing.T, name string) string {
	t.Helper()
	return repotest.Repo(t, name)
}

// fixtureLinkedWorktree adds a linked worktree called name to the repository at mainRoot, in the
// layout `git worktree add` makes: a .git file that points at .git/worktrees/<name>, whose
// commondir leads back to the main .git and whose HEAD is forty b's. It returns the worktree root.
func fixtureLinkedWorktree(t *testing.T, mainRoot, name string) string {
	t.Helper()
	return repotest.LinkedWorktree(t, mainRoot, name)
}

// mustRepoKey asks the locator of the commands for the key of the repository that holds cwd, and
// fails the test when there is none or the answer is not 64 lowercase hexadecimal digits.
func mustRepoKey(t *testing.T, cwd string) string {
	t.Helper()
	return repotest.MustKey(t, newRepoLocator(), cwd)
}
