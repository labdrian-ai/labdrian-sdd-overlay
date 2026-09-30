package skills

// ExecuteInstallPlan is all or nothing. Every test here injects one failure into the
// filesystem the executor works through (fakeProjectFS delegates to the real one over
// a temporary directory and fails a single call) and compares the whole tree with the
// one before the run: bytes, permission bits, and directories.

import (
	"bytes"
	"errors"
	"path/filepath"
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
	var stderr bytes.Buffer

	if err := ExecuteInstallPlan(plan, f.root, fsys, &stderr); err != nil {
		t.Fatalf("%v (stderr %q)", err, stderr.String())
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
			var stderr bytes.Buffer

			err := ExecuteInstallPlan(plan, f.root, newFakeProjectFS(inject(f)), &stderr)

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
			var stderr bytes.Buffer

			if err := ExecuteInstallPlan(plan, f.root, newFakeProjectFS(fail(f)), &stderr); err == nil {
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
	var stderr bytes.Buffer

	err := ExecuteInstallPlan(plan, f.root, fsys, &stderr)

	if !isRollbackIncomplete(err) {
		t.Fatalf("error = %v, want a rollback-incomplete error", err)
	}
	if !strings.Contains(stderr.String(), ".claude/skills/pdf/SKILL.md") {
		t.Errorf("stderr %q does not name the path that could not be restored", stderr.String())
	}
}

func isRollbackIncomplete(err error) bool {
	for depth := 0; err != nil && depth < maxErrorChain; depth++ {
		if err == ErrRollbackIncomplete {
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

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
	var stderr bytes.Buffer

	err := ExecuteInstallPlan(plan, f.root, osProjectFS{}, &stderr)

	if err == nil || !strings.Contains(err.Error(), ".agents/skills/pdf/SKILL.md") || !strings.Contains(err.Error(), "skills install") {
		t.Fatalf("error = %v, want a refusal naming the path, worded for install", err)
	}
	assertSameTree(t, before, f.snapshot())
}

func TestExecuteInstallPlan_AnEmptyPlanTouchesNothing(t *testing.T) {
	f := newOwnFixture(t)
	fsys := newFakeProjectFS(nil)
	if err := ExecuteInstallPlan(InstallPlan{}, f.root, fsys, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(fsys.log) != 0 {
		t.Errorf("an empty plan made calls: %v", fsys.log)
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
