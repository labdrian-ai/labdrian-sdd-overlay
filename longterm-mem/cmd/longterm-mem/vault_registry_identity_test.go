package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The vault registry is one of the two stores the identity of a project is taken from (machineStores): a name the
// registry has a row for is a name the memory already lives under, and a repository that derives it adopts it in
// place of the URL-shaped name its remote also derives. The row of the default project is the one a machine has
// before anyone adds another, so what a command that resolves the project from the working directory makes of a
// repository whose remote is named like the default project depends on what the registry holds.
//
// A person runs these commands more than once. The three runs below are `status` with no --project, in a
// repository whose origin is github.com/acme/labdrian-sdd-overlay, on a machine with no registry file. `status`
// only reads, so it makes no registry (the owner's decision of 2026-10-14), and a registry nobody has written is
// taken to hold the row a machine starts with, so every run says what the second and the third said before the
// decision, when the first run's registry was what made the plain name the one the memory lives under: the name
// of the default project, adopted, and its vault. The first run used to mint the URL-shaped name and exit 3 for it.
func TestVaultRegistry_ARepositoryNamedLikeTheDefaultProjectAcrossRuns(t *testing.T) {
	w := newIdentityWorld(t)
	w.repo(t)
	w.git(t, "remote", "add", "origin", "https://github.com/acme/"+registryDefaultProject+".git")
	t.Setenv("LONGTERM_MEM_VAULT", "")
	t.Chdir(w.root)

	for run := 1; run <= 3; run++ {
		code, _, stderr := w.run(t, []string{"status"})

		if code != exitOK {
			t.Errorf("run %d: exit %d, want %d\n%s", run, code, exitOK, stderr)
		}
		line := `adopted "` + registryDefaultProject + `" from the working directory`
		if !strings.Contains(stderr, line) {
			t.Errorf("run %d: stderr does not say it %s\n%s", run, line, stderr)
		}
	}
	if _, err := os.Stat(filepath.Join(w.home, ".labdrian-overlay", "vaults.json")); err == nil {
		t.Errorf("a registry exists after three runs of status, which only reads")
	}
}
