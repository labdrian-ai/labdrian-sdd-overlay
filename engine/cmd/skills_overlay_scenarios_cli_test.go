package main

// Scenarios that cross the verbs that write an overlay with the ones that read it or install from
// it, which only the composition of the program can run: the verbs are use cases, and a test of
// one cannot call another.

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// A writer asks the locker whether the registry is there before it asks for the lock: a lock
// file made beside a registry that is not there is litter in a directory that is not an overlay.
func TestAWriterAsksTheLockerWhetherTheRegistryIsThereBeforeItLocksIt(t *testing.T) {
	w := newOverlayWorld(t)
	locker := &holdingLocker{unseen: map[string]error{w.reg: fmt.Errorf("the locker cannot see it")}}
	r := w.run(skillsAdd, w.deps(locker, os.ReadFile), append([]string{"add", "newbie"}, w.flags()...)...)
	want := fmt.Sprintf("error: skills add: reading registry %q: the locker cannot see it; nothing was locked and nothing was changed\n", w.reg)
	if r.code() != 1 || r.stderr != want {
		t.Errorf("add = exit %d, stderr %q, want exit 1 and %q", r.code(), r.stderr, want)
	}
	if got := locker.log(); len(got) != 0 {
		t.Errorf("lock events = %v, want none: the verb refuses before it asks for a lock", got)
	}
	if got, _ := os.ReadFile(w.reg); string(got) != overlayRegistryOf("existing") {
		t.Errorf("the registry was changed to %q", got)
	}
}

// A writer that is about to fail because there is no registry must not first create a lock file
// beside the registry it did not find. With no --registry the registry is ./skills.registry.yaml
// in the working directory, wherever a raw engine call happens to be made.
func TestAWriterWithoutARegistryLocksNothing(t *testing.T) {
	for _, tc := range overlayVerbCases {
		t.Run(tc.name, func(t *testing.T) {
			w := newOverlayWorld(t)
			old, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Chdir(t.TempDir()); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := os.Chdir(old); err != nil {
					t.Errorf("restoring the working directory: %v", err)
				}
			})
			locker := &holdingLocker{}
			// The verb's own arguments, and no registry: the default is the working directory's.
			r := w.run(tc.verb, w.deps(locker, os.ReadFile), append(append([]string{tc.word}, tc.extra...), "--manifest", w.man, "--source-root", w.root)...)
			if r.code() != 1 || r.stdout != "" {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 1 and nothing on stdout", r.code(), r.stdout, r.stderr)
			}
			for _, want := range []string{"skills " + tc.name, "reading registry", "skills.registry.yaml", "nothing was locked"} {
				if !strings.Contains(r.stderr, want) {
					t.Errorf("stderr %q does not contain %q", r.stderr, want)
				}
			}
			if got := locker.log(); len(got) != 0 {
				t.Errorf("lock events = %v, want none", got)
			}
		})
	}
}

// An approve of a global skill that starts while an install is copying waits for it: the install
// is parked after it read the registry, holding the shared lock, and the approve can only be
// granted its exclusive one once the install has let go of it.
func TestAnApproveStartedDuringAnInstallWaitsForTheInstallToFinish(t *testing.T) {
	w := newOverlayWorld(t)
	project := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, w.reg, overlayRegistryOf("existing")+`  - id: proj
    path: proj
    source:
      type: custom
    install:
      defaultScope: project
      allowedProjects:
        - p
      targets:
        - claude
    lifecycle:
      updateStrategy: overlay-only
`)
	writeTestFile(t, w.man, overlayManifestOf("existing", "proj"))
	writeTestFile(t, filepath.Join(w.root, "proj", "SKILL.md"), overlaySkillMD("proj"))
	gate := newReadGate(t, w.reg)
	locker := &exclusionLocker{blocked: gate.release}

	deps := w.deps(locker, gate.readFile)
	deps.Tree = skillsTree()
	deps.Cwd = func() (string, error) { return project, nil }
	deps.Identity = newProjectIdentity()
	installDone := make(chan verbRun, 1)
	go func() {
		var out, errOut strings.Builder
		var r verbRun
		skills.SkillsCoreAt("install", []string{"install", "--registry", w.reg, "--source-root", w.root, "--project-id", "p"}, deps, &out, &errOut, func(c int) { r.exits = append(r.exits, c) })
		r.stdout, r.stderr = out.String(), errOut.String()
		installDone <- r
	}()
	<-gate.arrived
	approved := runSkillsVerb(skillsApprove, w.deps(locker, os.ReadFile), append([]string{"approve", "--id", "existing", "--approver", "reviewer"}, w.flags()...)...)
	gate.release()
	installed := <-installDone

	for name, r := range map[string]verbRun{"install": installed, "approve": approved} {
		if r.code() != 0 {
			t.Errorf("%s: exit %d, stderr=%q", name, r.code(), r.stderr)
		}
	}
	if _, err := os.Stat(filepath.Join(project, ".claude", "skills", "proj", "SKILL.md")); err != nil {
		t.Errorf("the project skill was not installed: %v", err)
	}
	events := locker.events()
	index := func(want string) int {
		for i, e := range events {
			if e == want {
				return i
			}
		}
		return -1
	}
	released, granted := index("released shared .registry.yaml.lock"), index("granted exclusive .registry.yaml.lock")
	if released < 0 || granted < 0 || released > granted {
		t.Errorf("lock events %v: the approve was granted its exclusive lock before the install let go of its shared one", events)
	}
}

// This is the scenario an upstream merge creates: a baseline skill that fails the hard lint and
// whose bytes changed. validate asks for an approval; approve gives it; validate then passes.
func TestABaselineSkillThatFailsTheHardLintIsAskedForAnApprovalByValidateAndApproveResolvesIt(t *testing.T) {
	w := newOverlayWorld(t)
	writeTestFile(t, w.reg, overlayRegistryOf(baselineSkillID))
	writeTestFile(t, w.man, overlayManifestOf(baselineSkillID))
	if err := os.RemoveAll(w.root); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(w.root, baselineSkillID, "SKILL.md"), overBudgetSkillMD(baselineSkillID))
	deps := w.deps(noopOverlayLocker{}, os.ReadFile)
	deps.Tree = skillsTree()
	validate := func() verbRun {
		return runSkillsVerb(skillsValidate, deps, append([]string{"validate"}, w.flags()...)...)
	}

	before := validate()
	if before.code() != 1 || !strings.Contains(before.stderr, "differs from the grandfathered baseline") || !strings.Contains(before.stderr, baselineSkillID) {
		t.Fatalf("validate before approve: exit %d, stderr %q, want the changed baseline skill asked for an approval", before.code(), before.stderr)
	}
	approved := runSkillsVerb(skillsApprove, deps, append([]string{"approve", "--id", baselineSkillID, "--approver", "reviewer"}, w.flags()...)...)
	if approved.code() != 0 || !strings.Contains(approved.stderr, "warning: [lint:body-hard-budget]") {
		t.Fatalf("approve: exit %d, stderr %q, want success with the finding as a warning", approved.code(), approved.stderr)
	}
	after := validate()
	if after.code() != 0 || !strings.Contains(after.stdout, "global skill approvals verified (1 skills: 1 approved, 0 grandfathered)") {
		t.Errorf("validate after approve: exit %d, stdout %q, stderr %q, want the skill approved by its record", after.code(), after.stdout, after.stderr)
	}
}

// add is deliberately unchanged by approval: it still refuses a hard lint finding, so a baseline
// skill that fails the lint cannot be registered again by add, approved or not. Baseline skills are
// already registered, so the only way to meet this is to remove one and add it back; the rewrite of
// these skills, which brings each one within the budget, is the way out. This test states the limit.
func TestAddStillRefusesABaselineSkillThatFailsTheHardLintEvenWhenApproved(t *testing.T) {
	w := newOverlayWorld(t)
	w.skill(baselineSkillID, overBudgetSkillMD(baselineSkillID))
	before := w.snapshot()
	r := w.run(skillsAdd, w.deps(noopOverlayLocker{}, os.ReadFile), append([]string{"add", baselineSkillID}, w.flags()...)...)
	if r.code() != 1 || !strings.Contains(r.stderr, "[lint:body-hard-budget]") {
		t.Fatalf("add: exit %d, stderr %q, want a refusal with the lint finding", r.code(), r.stderr)
	}
	if !reflect.DeepEqual(before, w.snapshot()) {
		t.Error("a refused add changed files")
	}
}
