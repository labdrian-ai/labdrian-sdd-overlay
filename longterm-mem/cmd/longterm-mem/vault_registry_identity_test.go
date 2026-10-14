package main

import (
	"strings"
	"testing"
)

// The vault registry is one of the two stores the identity of a project is taken from (machineStores): a name the
// registry has a row for is a name the memory already lives under, and a repository that derives it adopts it in
// place of the URL-shaped name its remote also derives. The row of the default project is the one a machine has
// before anyone adds another, so what a command that resolves the project from the working directory makes of a
// repository whose remote is named like the default project depends on whether the registry is there.
//
// A person runs these commands more than once. The three runs below are `status` with no --project, in a
// repository whose origin is github.com/acme/labdrian-sdd-overlay, on a machine with no registry file.
func TestVaultRegistry_ARepositoryNamedLikeTheDefaultProjectAcrossRuns(t *testing.T) {
	w := newIdentityWorld(t)
	w.repo(t)
	w.git(t, "remote", "add", "origin", "https://github.com/acme/"+registryDefaultProject+".git")
	t.Setenv("LONGTERM_MEM_VAULT", "")
	t.Chdir(w.root)

	// What the first run says is the name it minted, and what the vault of that name is: the registry has no row
	// for the URL-shaped name, so the vault is not configured. By the second run the first one has made the
	// registry, with the row of the default project, and the plain name is the one the memory lives under.
	for run, want := range []struct {
		exit    int
		project string
		verb    string
	}{
		{exitVaultNotConfigured, "github.com/acme/" + registryDefaultProject, "resolved"},
		{exitOK, registryDefaultProject, "adopted"},
		{exitOK, registryDefaultProject, "adopted"},
	} {
		code, _, stderr := w.run(t, []string{"status"})

		if code != want.exit {
			t.Errorf("run %d: exit %d, want %d\n%s", run+1, code, want.exit, stderr)
		}
		line := want.verb + ` "` + want.project + `" from the working directory`
		if !strings.Contains(stderr, line) {
			t.Errorf("run %d: stderr does not say it %s\n%s", run+1, line, stderr)
		}
	}
}
