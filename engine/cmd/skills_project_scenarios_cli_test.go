package main

// Scenarios that cross the verbs that install into a project with the ones that register skills in
// it, which only the composition of the program can run: the verbs are use cases, and a test of one
// cannot call another.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// The procedural verbs share the project lock with install and must carry its records along.
func TestProjectRegisterKeepsTheInstallRecordsAndInstallStillOwnsItsSkill(t *testing.T) {
	w := newInstallCLIWorld(t)
	deps := w.deps(noopOverlayLocker{})
	if r := w.run(skillsInstall, deps, w.args("install")...); r.code() != 0 {
		t.Fatalf("install: exit %d, stderr %q", r.code(), r.stderr)
	}
	regPath := filepath.Join(t.TempDir(), "skills.registry.yaml")
	writeTestFile(t, regPath, registryOf("alpha"))
	draft := filepath.Join(t.TempDir(), "SKILL.md")
	writeTestFile(t, draft, goldenProjectDraft("tidy-worktree"))

	var out, errOut strings.Builder
	code := -1
	runSkillsCore(testDeps(), "project-register", []string{"project-register", "--project-root", w.project,
		"--candidate", "procedural/candidates/repeated-success/tidy-worktree", "--registry", regPath, draft}, &out, &errOut, func(c int) { code = c })
	if code != 0 {
		t.Fatalf("project-register: exit %d, stderr %q", code, errOut.String())
	}

	data, err := os.ReadFile(skills.ProjectLockPath(w.project))
	if err != nil {
		t.Fatal(err)
	}
	lock, err := skills.ParseProjectLock(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(lock.Skills) != 1 || lock.Skills[0].ID != "tidy-worktree" || len(lock.Installs) != 1 || lock.Installs[0].ID != "proj" {
		t.Errorf("lock = %+v, want the procedural entry and the install record", lock)
	}
	if r := w.run(skillsInstall, deps, w.args("install")...); r.code() != 0 || r.stdout != "unchanged: proj\n" {
		t.Errorf("install after project-register: exit %d, stdout %q, stderr %q, want unchanged", r.code(), r.stdout, r.stderr)
	}
}
