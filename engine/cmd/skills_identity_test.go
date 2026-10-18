package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// The program names a project with the shared resolver (identity/resolve, C23 M2), in the owner's
// order of 2026-10-18: what the person said with --project-id, then the file .longterm-mem-project
// at the root of the main checkout, then the origin remote of the repository, then the name of the
// main checkout's root, and outside a repository the name of the directory. A checkout whose origin
// is github.com/acme/demo is that project, whatever its directory is called.
func TestTheProgramNamesAProjectWithTheSharedResolver(t *testing.T) {
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	origin := "[core]\n\tbare = false\n[remote \"origin\"]\n\turl = git@github.com:acme/demo.git\n"

	root := filepath.Join(t.TempDir(), "renamed-checkout")
	write(filepath.Join(root, ".git", "config"), origin)

	declared := filepath.Join(t.TempDir(), "declared-checkout")
	write(filepath.Join(declared, ".git", "config"), origin)
	write(filepath.Join(declared, ".longterm-mem-project"), "chosen\n")

	noOrigin := filepath.Join(t.TempDir(), "no-origin")
	write(filepath.Join(noOrigin, ".git", "config"), "[core]\n")
	below := filepath.Join(noOrigin, "sub", "dir")
	if err := os.MkdirAll(below, 0o755); err != nil {
		t.Fatal(err)
	}

	plain := filepath.Join(t.TempDir(), "plain-directory")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}

	for name, tc := range map[string]struct {
		q    skills.ProjectQuery
		want skills.ProjectID
	}{
		"what the person said, first":                      {skills.ProjectQuery{Dir: declared, Explicit: "given"}, "given"},
		"the declared file, before the origin":             {skills.ProjectQuery{Dir: declared}, "chosen"},
		"the origin, not the directory":                    {skills.ProjectQuery{Dir: root}, "github.com/acme/demo"},
		"the main checkout's name, not the subdirectory's": {skills.ProjectQuery{Dir: below}, "no-origin"},
		"the directory with no repository":                 {skills.ProjectQuery{Dir: plain}, "plain-directory"},
	} {
		t.Run(name, func(t *testing.T) {
			id, ok, err := newProjectIdentity().Identify(tc.q)
			if err != nil || !ok || id != tc.want {
				t.Errorf("newProjectIdentity().Identify(%+v) = %q, %v, %v, want %q", tc.q, id, ok, err, tc.want)
			}
		})
	}
}
