package skills

// ExecuteInstallPlan is all or nothing. Every test here injects one failure into the
// filesystem the executor works through (fakeProjectFS delegates to the real one over
// a temporary directory and fails a single call) and compares the whole tree with the
// one before the run: bytes, permission bits, and directories.

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var pdfV2 = map[string]string{"SKILL.md": "# pdf v2\n", "NOTES.md": "notes\n"} // drops references/guide.md

// updateScenario is an installed skill (v1) and the plan that moves it to v2: a
// replaced file, a created file, and a removed file, in each runtime directory, and
// the lock rewritten.
func updateScenario(t *testing.T) (*ownFixture, InstallPlan, map[string]string) {
	t.Helper()
	f := newOwnFixture(t)
	f.install(skill("pdf", pdfV1))
	plan, refusals := f.plan(skill("pdf", pdfV2))
	if len(refusals) != 0 {
		t.Fatalf("setup refused: %v", refusals)
	}
	if len(plan.Writes) != 4 || len(plan.Deletes) != 2 || plan.Lock.Rel == "" {
		t.Fatalf("setup plan has %d writes, %d deletes, lock %q; want 4, 2, and the lock", len(plan.Writes), len(plan.Deletes), plan.Lock.Rel)
	}
	return f, plan, f.snapshot()
}

func (f *ownFixture) abs(rel string) string { return filepath.Join(f.root, filepath.FromSlash(rel)) }

// failOnce fails the first call of op on want and no other, so that the undoing of
// what that call was part of can be seen to work. With lands set, a rename still
// happens and then reports failure, as on a filesystem that loses the reply.
func failOnce(op, want string, err error, lands bool) func(*fakeProjectFS, string, string) error {
	fired := false
	return func(f *fakeProjectFS, gotOp, gotPath string) error {
		if fired || gotOp != op || gotPath != want {
			return nil
		}
		fired = true
		f.after = lands
		return err
	}
}

func TestExecuteInstallPlan_CommitsWritesThenRemovalsThenTheLockLast(t *testing.T) {
	f, plan, _ := updateScenario(t)
	fsys := newFakeProjectFS(nil)

	if err := ExecuteInstallPlan(plan, f.root, fsys); err != nil {
		t.Fatalf("%v", err)
	}

	var committed []string
	for _, entry := range fsys.log {
		if op, path, _ := strings.Cut(entry, " "); op == "rename" || op == "remove" {
			committed = append(committed, op+" "+strings.TrimPrefix(path, f.root+"/"))
		}
	}
	// Files in path order within a runtime directory, the claude directory first;
	// then the removals; then the lock; and only then the emptied directories, which
	// are not part of the rollback.
	want := []string{
		"rename .claude/skills/pdf/NOTES.md", "rename .claude/skills/pdf/SKILL.md",
		"rename .agents/skills/pdf/NOTES.md", "rename .agents/skills/pdf/SKILL.md",
		"remove .claude/skills/pdf/references/guide.md", "remove .agents/skills/pdf/references/guide.md",
		"rename " + ProjectLockRelPath,
		"remove .claude/skills/pdf/references", "remove .agents/skills/pdf/references",
	}
	if strings.Join(committed, "\n") != strings.Join(want, "\n") {
		t.Errorf("commit order:\n%s\nwant:\n%s", strings.Join(committed, "\n"), strings.Join(want, "\n"))
	}
}

func TestExecuteInstallPlan_ALoneFailureAnywhereRestoresTheTreeExactly(t *testing.T) {
	boom := errors.New("injected failure")
	for name, inject := range map[string]func(f *ownFixture) func(*fakeProjectFS, string, string) error{
		"the first replacement cannot be staged": func(f *ownFixture) func(*fakeProjectFS, string, string) error {
			return failOnce("writetemp", f.abs(".claude/skills/pdf"), boom, false)
		},
		"the first rename fails": func(f *ownFixture) func(*fakeProjectFS, string, string) error {
			return failOnce("rename", f.abs(".claude/skills/pdf/NOTES.md"), boom, false)
		},
		"the last write fails": func(f *ownFixture) func(*fakeProjectFS, string, string) error {
			return failOnce("rename", f.abs(".agents/skills/pdf/SKILL.md"), boom, false)
		},
		"the first removal fails": func(f *ownFixture) func(*fakeProjectFS, string, string) error {
			return failOnce("remove", f.abs(".claude/skills/pdf/references/guide.md"), boom, false)
		},
		"the second removal fails": func(f *ownFixture) func(*fakeProjectFS, string, string) error {
			return failOnce("remove", f.abs(".agents/skills/pdf/references/guide.md"), boom, false)
		},
		"the lock cannot be committed": func(f *ownFixture) func(*fakeProjectFS, string, string) error {
			return failOnce("rename", f.abs(ProjectLockRelPath), boom, false)
		},
		"the lock rename lands and still reports failure": func(f *ownFixture) func(*fakeProjectFS, string, string) error {
			return failOnce("rename", f.abs(ProjectLockRelPath), boom, true)
		},
		"a replacement lands and still reports failure": func(f *ownFixture) func(*fakeProjectFS, string, string) error {
			return failOnce("rename", f.abs(".agents/skills/pdf/NOTES.md"), boom, true)
		},
		"a removal lands and still reports failure": func(f *ownFixture) func(*fakeProjectFS, string, string) error {
			guide := f.abs(".agents/skills/pdf/references/guide.md")
			fired := false
			return func(fsys *fakeProjectFS, op, path string) error {
				if fired || op != "remove" || path != guide {
					return nil
				}
				fired = true
				_ = fsys.real.Remove(path)
				return boom
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			f, plan, before := updateScenario(t)

			err := ExecuteInstallPlan(plan, f.root, newFakeProjectFS(inject(f)))

			if err == nil || !strings.Contains(err.Error(), "injected failure") {
				t.Fatalf("error = %v, want the injected failure", err)
			}
			if strings.Contains(err.Error(), "project-register") {
				t.Errorf("error %q names another verb", err)
			}
			assertSameTree(t, before, f.snapshot())
		})
	}
}

// A first install that fails part way leaves no skill directory, no runtime directory
// it created, and no lock.
func TestExecuteInstallPlan_AFailedFirstInstallLeavesNothingBehind(t *testing.T) {
	boom := errors.New("injected failure")
	for name, fail := range map[string]func(f *ownFixture) func(*fakeProjectFS, string, string) error{
		"a rename in the second runtime": func(f *ownFixture) func(*fakeProjectFS, string, string) error {
			return failAt("rename", f.abs(".agents/skills/pdf/SKILL.md"), boom)
		},
		"the lock": func(f *ownFixture) func(*fakeProjectFS, string, string) error {
			return failAt("rename", f.abs(ProjectLockRelPath), boom)
		},
		"a directory that is half made": func(f *ownFixture) func(*fakeProjectFS, string, string) error {
			return failPartialMkdir(f.abs(".agents/skills/pdf/references"), boom)
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newOwnFixture(t)
			plan, refusals := f.plan(skill("pdf", pdfV1))
			if len(refusals) != 0 {
				t.Fatal(refusals)
			}
			before := f.snapshot()

			if err := ExecuteInstallPlan(plan, f.root, newFakeProjectFS(fail(f))); err == nil {
				t.Fatal("the install succeeded")
			}

			assertSameTree(t, before, f.snapshot())
			for _, dir := range []string{".claude", ".agents", ".labdrian"} {
				if f.exists(dir) {
					t.Errorf("%s was left behind", dir)
				}
			}
		})
	}
}

// When putting a file back fails too, the error says so and names the path.
func TestExecuteInstallPlan_ReportsARollbackThatCouldNotFinish(t *testing.T) {
	f, plan, _ := updateScenario(t)
	boom := errors.New("injected failure")
	skillRenames := 0
	fsys := newFakeProjectFS(func(_ *fakeProjectFS, op, path string) error {
		// The lock cannot be committed, and putting the replaced SKILL.md back (the
		// second rename onto its path) fails too.
		if op == "rename" && path == f.abs(".claude/skills/pdf/SKILL.md") {
			skillRenames++
			if skillRenames == 2 {
				return boom
			}
		}
		if op == "rename" && path == f.abs(ProjectLockRelPath) {
			return boom
		}
		return nil
	})

	err := ExecuteInstallPlan(plan, f.root, fsys)

	if !isRollbackIncomplete(err) {
		t.Fatalf("error = %v, want a rollback-incomplete error", err)
	}
	if !slices.Contains(unrestoredBy(err), ".claude/skills/pdf/SKILL.md") {
		t.Errorf("the error names %q as not restored, want it to include the path that could not be restored", unrestoredBy(err))
	}
}

func isRollbackIncomplete(err error) bool { return errors.Is(err, ErrRollbackIncomplete) }

// The plan was built from what was on disk; if a path it meant to create appears
// before the write, the install stops instead of writing over it.
func TestExecuteInstallPlan_RefusesAPathThatAppearedAfterThePlanWasBuilt(t *testing.T) {
	f := newOwnFixture(t)
	plan, refusals := f.plan(skill("pdf", pdfV1))
	if len(refusals) != 0 {
		t.Fatal(refusals)
	}
	f.write(".agents/skills/pdf/SKILL.md", "appeared meanwhile\n")
	before := f.snapshot()

	err := ExecuteInstallPlan(plan, f.root, testProjectFS())

	if err == nil || !strings.Contains(err.Error(), ".agents/skills/pdf/SKILL.md") || !strings.Contains(err.Error(), "skills install") {
		t.Fatalf("error = %v, want a refusal naming the path, worded for install", err)
	}
	assertSameTree(t, before, f.snapshot())
}

// The executor is the one project-register uses. It words its failures for the verb
// that ran it, from a name it is given, and never names the verb it was written for
// or the other one of the pair: an adopt that fails says "skills adopt".
func TestExecuteInstallPlan_WordsEveryFailureForTheVerbThatRanIt(t *testing.T) {
	boom := errors.New("injected failure")
	for _, verb := range []string{"install", "adopt"} {
		// A plan for the verb over a project where its one destination is the lock (adopt
		// writes only that) or the skill file in the second runtime (install).
		plan := func(t *testing.T) (*ownFixture, InstallPlan) {
			f := newOwnFixture(t)
			if verb == "adopt" {
				f.preexisting("pdf", pdfV1, ".claude", ".agents")
				p, refusals := f.adoptPlan(skill("pdf", pdfV1))
				if len(refusals) != 0 {
					t.Fatalf("setup refused: %v", refusals)
				}
				return f, p
			}
			p, refusals := f.plan(skill("pdf", pdfV1))
			if len(refusals) != 0 {
				t.Fatalf("setup refused: %v", refusals)
			}
			return f, p
		}
		mine := "skills " + verb + ": "

		t.Run(verb+"/a destination that appeared after the plan", func(t *testing.T) {
			f, p := plan(t)
			appeared := ProjectLockRelPath
			if verb == "install" {
				appeared = ".agents/skills/pdf/SKILL.md"
			}
			f.write(appeared, "appeared meanwhile\n")

			err := ExecuteInstallPlan(p, f.root, testProjectFS())

			if err == nil || !strings.HasPrefix(err.Error(), mine) || !strings.Contains(err.Error(), appeared) {
				t.Fatalf("error = %v, want a refusal that starts %q and names %s", err, mine, appeared)
			}
			if strings.Contains(err.Error(), "project-register") {
				t.Errorf("error %q names the verb the executor was written for", err)
			}
		})

		t.Run(verb+"/a rollback that could not finish", func(t *testing.T) {
			f, p := plan(t)
			lock := f.abs(ProjectLockRelPath)
			fsys := newFakeProjectFS(func(_ *fakeProjectFS, op, path string) error {
				if path == lock && (op == "rename" || op == "remove") {
					return boom
				}
				return nil
			})

			err := ExecuteInstallPlan(p, f.root, fsys)

			if !isRollbackIncomplete(err) {
				t.Fatalf("error = %v, want a rollback-incomplete error", err)
			}
			if !strings.HasPrefix(err.Error(), mine+"rollback incomplete") || strings.Contains(err.Error(), "project-register") {
				t.Errorf("error = %q, want it to start %q and to name no other verb", err, mine+"rollback incomplete")
			}
			if !strings.Contains(err.Error(), mine+"committing") {
				t.Errorf("error = %q, want the cause worded for %q too", err, verb)
			}
		})
	}
}

func TestExecuteInstallPlan_AnEmptyPlanTouchesNothing(t *testing.T) {
	f := newOwnFixture(t)
	fsys := newFakeProjectFS(nil)
	if err := ExecuteInstallPlan(InstallPlan{}, f.root, fsys); err != nil {
		t.Fatal(err)
	}
	if len(fsys.log) != 0 {
		t.Errorf("an empty plan made calls: %v", fsys.log)
	}
}

// A skill's own directory is never pruned, even when removing the plan's files leaves
// it empty. The function guarantees that by itself, through the strict containment
// test it asks: the root is not within itself. A plan never empties a skill directory
// (every skill keeps its SKILL.md), so these plans are built by hand; the guarantee
// must not depend on that.
func TestPruneEmptyDirs_NeverRemovesASkillDirectoryItself(t *testing.T) {
	for name, deleted := range map[string]string{
		"a file directly in the skill directory": "old.md",
		"a file two levels down":                 "a/b/old.md",
	} {
		t.Run(name, func(t *testing.T) {
			f := newOwnFixture(t)
			skillDir := f.abs(".claude/skills/pdf")
			if err := os.MkdirAll(filepath.Join(skillDir, filepath.Dir(filepath.FromSlash(deleted))), 0o755); err != nil {
				t.Fatal(err)
			}
			plan := InstallPlan{
				Deletes: []ProjectWrite{{Rel: ".claude/skills/pdf/" + deleted, Abs: filepath.Join(skillDir, filepath.FromSlash(deleted))}},
				Dirs:    []string{skillDir},
			}

			pruneEmptyDirs(testProjectFS(), plan)

			if !f.exists(".claude/skills/pdf") {
				t.Error("the skill directory itself was removed")
			}
			if !f.exists(".claude/skills") {
				t.Error("the runtime's skills directory was removed")
			}
			if strings.Contains(deleted, "/") && f.exists(".claude/skills/pdf/a") {
				t.Error("the emptied directory inside the skill was not pruned")
			}
		})
	}
}

// Emptied directories are pruned only inside an installed skill, never the skill's
// own directory, and never one that still holds something.
func TestExecuteInstallPlan_PrunesOnlyDirectoriesItEmptied(t *testing.T) {
	f := newOwnFixture(t)
	f.install(skill("pdf", map[string]string{"SKILL.md": "x", "a/b/c.md": "deep", "a/keep.md": "kept"}))
	f.write(".claude/skills/pdf/a/mine.txt", "a person's file in a/")

	f.install(skill("pdf", map[string]string{"SKILL.md": "x"}))

	if f.exists(".claude/skills/pdf/a/b") {
		t.Error("the emptied directory a/b was not pruned")
	}
	if !f.exists(".claude/skills/pdf/a/mine.txt") || !f.exists(".claude/skills/pdf/a") {
		t.Error("a directory that still holds a person's file was removed")
	}
	if f.exists(".agents/skills/pdf/a") {
		t.Error("the other runtime's emptied directory a was not pruned")
	}
	if !f.exists(".claude/skills/pdf/SKILL.md") || !f.exists(".agents/skills/pdf/SKILL.md") {
		t.Error("a skill directory was pruned")
	}
}
