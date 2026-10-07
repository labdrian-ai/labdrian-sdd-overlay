package main

// The verbs that sit behind a use case are reached through the table of the program, which wires the
// real adapters and the real locker (the project verbs through runSkillsCore itself): each verb the table names is the verb the
// person typed, and what it tells is its own. The adapters themselves are tested one by one in
// skills_install_cli_test.go and skills_project_cli_test.go.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// throughTheProgram runs one command of `engine skills` as the program does.
func throughTheProgram(verb string, args []string) verbRun {
	var out, errOut strings.Builder
	var r verbRun
	runSkillsCore(verb, args, &out, &errOut, func(c int) { r.exits = append(r.exits, c) })
	r.stdout, r.stderr = out.String(), errOut.String()
	return r
}

// throughTheTable runs a verb as the program does, by the name the table gives it and with the
// adapters and the locker the program wires, except for the working directory, which is the one the
// test names: a test must not change the directory of the process.
func throughTheTable(verb, cwd string, args []string) verbRun {
	deps := newSkillsDeps()
	deps.Cwd = func() (string, error) { return cwd, nil }
	return runSkillsVerb(skillsCLIVerbs[verb], deps, args...)
}

func TestInstallAndAdoptAreEachTheVerbTheTableNamesThem(t *testing.T) {
	t.Run("install writes what the registry admits", func(t *testing.T) {
		w := newInstallCLIWorld(t)

		r := throughTheTable("install", w.project, w.args("install"))

		if r.code() != 0 || r.stdout != "installed: proj\n" || r.stderr != "" {
			t.Errorf("exit %d, stdout %q, stderr %q, want installed: proj", r.code(), r.stdout, r.stderr)
		}
		if _, err := os.Stat(filepath.Join(w.project, ".claude", "skills", "proj", "SKILL.md")); err != nil {
			t.Errorf("install through the program wrote no skill: %v", err)
		}
	})
	t.Run("adopt records what is already there and writes nothing", func(t *testing.T) {
		w := newInstallCLIWorld(t)
		w.copyByHand(overlaySkillMD("proj"))
		before := w.tree()

		r := throughTheTable("adopt", w.project, w.args("adopt"))

		if r.code() != 0 || r.stdout != "adopted: proj\n" || r.stderr != "" {
			t.Errorf("exit %d, stdout %q, stderr %q, want adopted: proj", r.code(), r.stdout, r.stderr)
		}
		for rel, content := range before {
			if got, ok := w.tree()[rel]; !ok || got != content {
				t.Errorf("adopt changed %s", rel)
			}
		}
	})
}

func TestTheProjectVerbsAreEachTheVerbTheTableNamesThem(t *testing.T) {
	w := newProjectCLIWorld(t)

	r := throughTheProgram("project-register", w.registerArgs())
	if r.code() != 0 || !strings.HasPrefix(r.stdout, "wrote: .claude/skills/"+projectTidyID+"/SKILL.md\n") || !strings.HasSuffix(r.stdout, skills.PiTrustNote+"\n") {
		t.Fatalf("project-register: exit %d, stdout %q, stderr %q", r.code(), r.stdout, r.stderr)
	}
	r = throughTheProgram("project-status", w.statusArgs(projectTidyID))
	if want := projectTidyID + " rev:1 owner:agent superseded-by:-\n"; r.code() != 0 || r.stdout != want || r.stderr != "" {
		t.Errorf("project-status: exit %d, stdout %q, stderr %q, want %q", r.code(), r.stdout, r.stderr, want)
	}
	r = throughTheProgram("project-revise", w.reviseArgs("--dry-run"))
	if r.code() != 0 || strings.Count(r.stdout, "plan: ") != 3 || r.stderr != "" {
		t.Errorf("project-revise: exit %d, stdout %q, stderr %q, want the plan of three paths", r.code(), r.stdout, r.stderr)
	}
	r = throughTheProgram("project-retire", w.retireArgs(projectTidyID, "--reason", "human-request"))
	want := "removed: .claude/skills/" + projectTidyID + "/SKILL.md\nremoved: .agents/skills/" + projectTidyID + "/SKILL.md\n"
	if r.code() != 0 || r.stdout != want || r.stderr != "" {
		t.Errorf("project-retire: exit %d, stdout %q, stderr %q, want %q", r.code(), r.stdout, r.stderr, want)
	}
}

// --dry-run of project-retire tells the files it would remove, as the lines the agent reads, and
// removes nothing.
func TestProjectRetireDryRunTellsThePlanAndRemovesNothing(t *testing.T) {
	w := newProjectCLIWorld(t)
	w.registered()
	before := w.tree()

	r := w.run(skillsProjectRetire, w.deps(noopOverlayLocker{}), w.retireArgs(projectTidyID, "--dry-run")...)

	want := "plan: .claude/skills/" + projectTidyID + "/SKILL.md\nplan: .agents/skills/" + projectTidyID + "/SKILL.md\nplan: " + skills.ProjectLockRelPath + "\n"
	if r.code() != 0 || r.stdout != want || r.stderr != "" {
		t.Errorf("exit %d, stdout %q, stderr %q, want exit 0 and %q", r.code(), r.stdout, r.stderr, want)
	}
	if got := w.tree(); len(got) != len(before) {
		t.Errorf("a dry run changed the project: %v, want %v", got, before)
	}
	if got := w.lockIDs(); len(got) != 1 || got[0] != projectTidyID {
		t.Errorf("a dry run changed the lock: it lists %v", got)
	}
}

// A project lock that stays taken is exit 2 with a retry message for every verb that takes it, and
// the project is not changed.
func TestEveryProjectVerbExits2WhenTheProjectLockStaysTaken(t *testing.T) {
	for _, v := range projectVerbs {
		t.Run(v.name, func(t *testing.T) {
			w := newProjectCLIWorld(t)
			locker := &holdingLocker{failOn: map[string]error{w.root: busyAtErr{at: w.root}}}
			args := map[string][]string{
				"project-register": w.registerArgs(), "project-revise": w.reviseArgs(),
				"project-retire": w.retireArgs("x"), "project-status": w.statusArgs(""),
			}[v.name]
			before := w.tree()

			r := w.run(v.run, w.deps(locker), args...)

			if r.code() != skills.ExitBusy || r.stdout != "" || !strings.Contains(r.stderr, "skills "+v.name) || !strings.Contains(r.stderr, "retry") {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 2 and a retry message naming the verb", r.code(), r.stdout, r.stderr)
			}
			if got := w.tree(); len(got) != len(before) {
				t.Errorf("a busy lock changed the project: %v", got)
			}
		})
	}
}
