package main

// Tests of the adapter of `skills uninstall` (skills_install_cli.go): what it tells, what it leaves
// on disk, the words of each refusal, and the one lock it takes. What the planner decides is tested
// in engine/skills (uninstall_plan_test.go).

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// installed is the world after `skills install` put the project skill in the project.
func installedWorld(t *testing.T) (installCLIWorld, skills.Deps) {
	t.Helper()
	w := newInstallCLIWorld(t)
	deps := w.deps(noopOverlayLocker{})
	if r := w.run(skillsInstall, deps, w.args("install")...); r.code() != 0 {
		t.Fatalf("install: exit %d, stderr %q", r.code(), r.stderr)
	}
	return w, deps
}

func TestUninstallRemovesWhatInstallWroteAndSaysEachFile(t *testing.T) {
	w, deps := installedWorld(t)

	r := w.run(skillsUninstall, deps, "uninstall", "proj")

	want := "removed: .claude/skills/proj/SKILL.md\nremoved: .agents/skills/proj/SKILL.md\nuninstalled: proj\n"
	if r.stdout != want || r.stderr != "" || !reflect.DeepEqual(r.exits, []int{0}) {
		t.Errorf("exit %v, stdout %q, stderr %q, want stdout %q", r.exits, r.stdout, r.stderr, want)
	}
	tree := w.tree()
	for path := range tree {
		if strings.Contains(path, "skills/proj") {
			t.Errorf("%s is still in the project", path)
		}
	}
	lock, err := skills.ParseProjectLock([]byte(tree[".labdrian/procedural-skills.lock.json"]))
	if err != nil || len(lock.Installs) != 0 {
		t.Errorf("the project lock = %+v, %v; want it there with no install record", lock, err)
	}
	if again := w.run(skillsInstall, deps, w.args("install")...); again.stdout != "installed: proj\n" {
		t.Errorf("install after uninstall = %q, want the skill installed again", again.stdout)
	}
}

func TestUninstallDryRunSaysWhatItWouldRemoveAndChangesNothing(t *testing.T) {
	w, deps := installedWorld(t)
	before := w.tree()

	r := w.run(skillsUninstall, deps, "uninstall", "--dry-run", "proj")

	want := "plan: .claude/skills/proj/SKILL.md\nplan: .agents/skills/proj/SKILL.md\nplan: .labdrian/procedural-skills.lock.json\n"
	if r.stdout != want || r.stderr != "" || !reflect.DeepEqual(r.exits, []int{0}) {
		t.Errorf("exit %v, stdout %q, stderr %q, want stdout %q", r.exits, r.stdout, r.stderr, want)
	}
	if !reflect.DeepEqual(before, w.tree()) {
		t.Error("a dry run changed the project")
	}
}

func TestUninstallRefusesAnEditedFileNamingItAndRemovesNothing(t *testing.T) {
	w, deps := installedWorld(t)
	writeTestFile(t, filepath.Join(w.project, ".claude", "skills", "proj", "SKILL.md"), "my edit\n")
	before := w.tree()

	r := w.run(skillsUninstall, deps, "uninstall", "proj")

	want := "error: skills uninstall: .claude/skills/proj/SKILL.md was edited since skills install wrote it (its bytes no longer match the install record); restore it or move it away, then run uninstall again\n" +
		"error: skills uninstall: refused, so nothing was removed\n"
	if r.code() != 1 || r.stdout != "" || r.stderr != want {
		t.Errorf("exit %d, stdout %q, stderr %q, want %q", r.code(), r.stdout, r.stderr, want)
	}
	if !reflect.DeepEqual(before, w.tree()) {
		t.Error("a refused uninstall changed the project")
	}
}

func TestUninstallKeepsAndNamesADirectoryThatHoldsFilesInstallDidNotWrite(t *testing.T) {
	w, deps := installedWorld(t)
	writeTestFile(t, filepath.Join(w.project, ".agents", "skills", "proj", "notes.md"), "mine\n")

	r := w.run(skillsUninstall, deps, "uninstall", "proj")

	want := "removed: .claude/skills/proj/SKILL.md\nremoved: .agents/skills/proj/SKILL.md\n" +
		"note: kept .agents/skills/proj: it holds files skills install did not write\nuninstalled: proj\n"
	if r.stdout != want || r.code() != 0 {
		t.Errorf("exit %d, stdout %q, stderr %q, want stdout %q", r.code(), r.stdout, r.stderr, want)
	}
	if got := w.tree()[".agents/skills/proj/notes.md"]; got != "mine\n" {
		t.Errorf("the unrecorded file holds %q", got)
	}
}

func TestUninstallRefusesWhatItCannotUninstall(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(w installCLIWorld)
		args  []string
		want  string
	}{
		{"no id", func(installCLIWorld) {}, []string{"uninstall"},
			"error: skills uninstall requires a <id> argument\n"},
		{"two ids", func(installCLIWorld) {}, []string{"uninstall", "a", "b"},
			"error: skills uninstall: unexpected extra argument \"b\" (uninstall accepts exactly one <id>)\n"},
		{"a flag it does not read", func(installCLIWorld) {}, []string{"uninstall", "--project-id", "p", "proj"},
			"unknown flag"},
		{"a project with no lock", func(installCLIWorld) {}, []string{"uninstall", "proj"},
			"error: skills uninstall: proj has no install record in .labdrian/procedural-skills.lock.json, so nothing installed it here and nothing is removed\n"},
		{"a skill copied by hand", func(w installCLIWorld) { w.copyByHand(overlaySkillMD("proj")) }, []string{"uninstall", "proj"},
			"proj has no install record"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newInstallCLIWorld(t)
			tc.setup(w)
			before := w.tree()

			r := w.run(skillsUninstall, w.deps(noopOverlayLocker{}), tc.args...)

			if r.code() != 1 || r.stdout != "" || !strings.Contains(r.stderr, tc.want) {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 1 and stderr holding %q", r.code(), r.stdout, r.stderr, tc.want)
			}
			if !reflect.DeepEqual(before, w.tree()) {
				t.Error("the project changed")
			}
		})
	}
}

// project-status lists every install record too, after the procedural skills, with how many files
// it records, and an id names an installed skill as it names a procedural one.
func TestProjectStatusListsTheInstallRecords(t *testing.T) {
	w, deps := installedWorld(t)
	status := func(extra ...string) verbRun {
		return w.run(skillsProjectStatus, deps, append([]string{"project-status", "--project-root", w.project, "--registry", w.reg}, extra...)...)
	}

	for _, tc := range []struct {
		name  string
		extra []string
	}{{"every skill", nil}, {"the installed one by its id", []string{"proj"}}} {
		r := status(tc.extra...)
		if r.stdout != "proj installed files:1\n" || r.stderr != "" || !reflect.DeepEqual(r.exits, []int{0}) {
			t.Errorf("%s: exit %v, stdout %q, stderr %q, want %q", tc.name, r.exits, r.stdout, r.stderr, "proj installed files:1\n")
		}
	}
	if r := status("ghost"); r.code() != 1 || !strings.Contains(r.stderr, `skill "ghost" is not in the project lock`) {
		t.Errorf("an id the lock does not have: exit %d, stderr %q", r.code(), r.stderr)
	}
	w.run(skillsUninstall, deps, "uninstall", "proj")
	if r := status(); r.stdout != "" || r.code() != 0 {
		t.Errorf("after uninstall: exit %d, stdout %q, want nothing listed", r.code(), r.stdout)
	}
}

// uninstall takes the project lock, exclusive, on the directory it works in, and no overlay lock:
// it reads no registry.
func TestUninstallTakesOnlyTheProjectLockExclusive(t *testing.T) {
	got := skills.ProjectLocks("uninstall", "/p")
	if len(got) != 1 || got[0].Mode != skills.LockExclusive || !got[0].Dir || got[0].Path != "/p" {
		t.Errorf("ProjectLocks(uninstall) = %+v, want one exclusive lock on the directory /p", got)
	}
	if overlay := skills.OverlayLocks("uninstall", "/r/skills.registry.yaml"); len(overlay) != 0 {
		t.Errorf("OverlayLocks(uninstall) = %+v, want none", overlay)
	}
}

func TestUninstallRefusesBeforeLockingWhenItCannotNameItsProjectDirectory(t *testing.T) {
	w := newInstallCLIWorld(t)
	deps := w.depsAt(noopOverlayLocker{}, func() (string, error) { return "relative", nil })

	r := w.run(skillsUninstall, deps, "uninstall", "proj")

	if r.code() != 1 || !strings.Contains(r.stderr, "cannot resolve the project directory it works in") || !strings.Contains(r.stderr, "nothing was removed") {
		t.Errorf("exit %d, stderr %q", r.code(), r.stderr)
	}
	if _, err := os.Stat(skills.ProjectLockPath(w.project)); !os.IsNotExist(err) {
		t.Errorf("a project lock appeared: %v", err)
	}
}
