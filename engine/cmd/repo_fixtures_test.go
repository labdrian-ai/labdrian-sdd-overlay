package main

// The fixture repositories the commands' tests use are engine/repotest's: hand-built plain
// directories and files, never a real git repository and never the git binary, because the
// commands find a repository by reading its files (engine/gitfs). These are the names this
// package's tests have always called them by.

import (
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/repotest"
)

func writeFixtureFile(t *testing.T, path, content string) {
	t.Helper()
	repotest.WriteFile(t, path, content)
}

func wantRepoKey(t *testing.T, commonDir string) string {
	t.Helper()
	return repotest.WantKey(t, commonDir)
}

func fixtureRepo(t *testing.T, name string) string {
	t.Helper()
	return repotest.Repo(t, name)
}

func fixtureLinkedWorktree(t *testing.T, mainRoot, name string) string {
	t.Helper()
	return repotest.LinkedWorktree(t, mainRoot, name)
}

func mustRepoKey(t *testing.T, cwd string) string {
	t.Helper()
	return repotest.MustKey(t, newRepoLocator(), cwd)
}
