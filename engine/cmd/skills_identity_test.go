package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// The chain the program builds names a project in the owner's order (Phase 9, decision Q8): what
// the person said with --project-id, then the origin remote of the repository, then the name of
// the directory. A checkout whose origin is github.com/acme/demo is that project, whatever its
// directory is called.
func TestTheProgramNamesAProjectByItsOriginBeforeItsDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "renamed-checkout")
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := "[core]\n\tbare = false\n[remote \"origin\"]\n\turl = git@github.com:acme/demo.git\n"
	if err := os.WriteFile(filepath.Join(root, ".git", "config"), []byte(config), 0o644); err != nil {
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
		"the origin, not the directory":    {skills.ProjectQuery{Dir: root}, "github.com/acme/demo"},
		"what the person said, first":      {skills.ProjectQuery{Dir: root, Explicit: "given"}, "given"},
		"the directory with no repository": {skills.ProjectQuery{Dir: plain}, "plain-directory"},
	} {
		t.Run(name, func(t *testing.T) {
			id, ok, err := newProjectIdentity().Identify(tc.q)
			if err != nil || !ok || id != tc.want {
				t.Errorf("newProjectIdentity().Identify(%+v) = %q, %v, %v, want %q", tc.q, id, ok, err, tc.want)
			}
		})
	}
}
