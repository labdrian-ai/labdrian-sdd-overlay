package skills

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pathguard/fsresolve"
)

// projectRevisionFixture is a registered project-tier skill plus a revised
// draft outside its project root. The fixture deliberately creates the
// registered state through the real registration planner and executor, so
// revision tests exercise the same lock and target layout as the CLI.
type projectRevisionFixture struct {
	root      string
	lockPath  string
	first     string
	second    string
	oldSkill  []byte
	oldLock   []byte
	draftPath string
	draftData []byte
}

func newProjectRevisionFixture(t *testing.T, id string) projectRevisionFixture {
	t.Helper()
	in := registerInput(t, id)
	plan, err := PlanProjectRegister(in)
	if err != nil {
		t.Fatalf("plan initial registration: %v", err)
	}
	if err := ExecuteProjectPlan(plan, newFakeProjectFS(nil)); err != nil {
		t.Fatalf("execute initial registration: %v", err)
	}

	oldSkill, err := os.ReadFile(plan.Writes[0].Abs)
	if err != nil {
		t.Fatalf("read registered skill: %v", err)
	}
	oldLock, err := os.ReadFile(plan.Lock.Abs)
	if err != nil {
		t.Fatalf("read registered lock: %v", err)
	}
	draftData := []byte(strings.Replace(string(in.DraftData),
		"Use when a worktree must be handed over clean.",
		"Recheck a worktree before handing it to a reviewer.", 1))
	draftPath := filepath.Join(filepath.Dir(in.DraftPath), "revision-SKILL.md")
	if err := os.WriteFile(draftPath, draftData, 0o644); err != nil {
		t.Fatalf("write revision draft: %v", err)
	}

	return projectRevisionFixture{
		root:      in.ProjectRoot,
		lockPath:  plan.Lock.Abs,
		first:     plan.Writes[0].Abs,
		second:    plan.Writes[1].Abs,
		oldSkill:  oldSkill,
		oldLock:   oldLock,
		draftPath: draftPath,
		draftData: draftData,
	}
}

func (f projectRevisionFixture) input() ReviseInput {
	return ReviseInput{
		ProjectRoot:  f.root,
		DraftPath:    f.draftPath,
		DraftData:    f.draftData,
		CandidateKey: testCandidateKey,
		LockData:     f.oldLock,
		LockExists:   true,
		ReadFile:     os.ReadFile,
		ReadDir:      os.ReadDir,
		Stat:         os.Stat,
		ResolvePath:  fsresolve.KeepingMissing,
	}
}

func TestPlanProjectRevise_RefusesHumanOwnedReasonsWithoutWrites(t *testing.T) {
	cases := []struct {
		name   string
		reason string
		mutate func(t *testing.T, f projectRevisionFixture)
	}{
		{
			name:   "hash-mismatch",
			reason: "hash-mismatch",
			mutate: func(t *testing.T, f projectRevisionFixture) {
				if err := os.WriteFile(f.first, append(append([]byte{}, f.oldSkill...), []byte("human edit\n")...), 0o644); err != nil {
					t.Fatalf("human edit: %v", err)
				}
			},
		},
		{
			name:   "missing",
			reason: "missing",
			mutate: func(t *testing.T, f projectRevisionFixture) {
				if err := os.Remove(f.first); err != nil {
					t.Fatalf("remove target: %v", err)
				}
			},
		},
		{
			name:   "extra-entry",
			reason: "extra-entry",
			mutate: func(t *testing.T, f projectRevisionFixture) {
				if err := os.WriteFile(filepath.Join(filepath.Dir(f.first), "README.md"), []byte("human note\n"), 0o644); err != nil {
					t.Fatalf("write extra entry: %v", err)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newProjectRevisionFixture(t, "tidy-worktree")
			tc.mutate(t, f)
			before := snapshotTree(t, f.root)

			plan, err := PlanProjectRevise(f.input())
			if err == nil {
				t.Fatalf("expected %s refusal, got plan %+v", tc.reason, plan)
			}
			if !strings.Contains(err.Error(), tc.reason) {
				t.Errorf("refusal %q does not name %q", err, tc.reason)
			}
			if !strings.Contains(err.Error(), "human-owned") {
				t.Errorf("refusal %q does not report human ownership", err)
			}
			assertSameTree(t, before, snapshotTree(t, f.root))
		})
	}
}

func TestPlanProjectRevise_HashMatchBumpsRevisionAndHash(t *testing.T) {
	f := newProjectRevisionFixture(t, "tidy-worktree")
	plan, err := PlanProjectRevise(f.input())
	if err != nil {
		t.Fatalf("unexpected revision refusal: %v", err)
	}
	if plan.ID != "tidy-worktree" {
		t.Errorf("plan.ID = %q, want tidy-worktree", plan.ID)
	}
	if plan.Revision != 2 {
		t.Errorf("plan.Revision = %d, want 2", plan.Revision)
	}
	if plan.SHA256 != HashSkill(plan.Writes[0].Data) {
		t.Errorf("plan hash = %q, want hash of stamped revision bytes %q", plan.SHA256, HashSkill(plan.Writes[0].Data))
	}
	if plan.SHA256 == HashSkill(f.oldSkill) {
		t.Error("revision hash must change when the draft body changes")
	}
	for i, w := range plan.Writes {
		if w.Backup == nil || !bytes.Equal(w.Backup, f.oldSkill) {
			t.Errorf("Writes[%d].Backup must capture the old target bytes", i)
		}
	}
	if plan.Lock.Backup == nil || !bytes.Equal(plan.Lock.Backup, f.oldLock) {
		t.Error("revision lock write must capture the old lock bytes")
	}

	if err := ExecuteProjectRevisePlan(plan, newFakeProjectFS(nil)); err != nil {
		t.Fatalf("execute revision: %v", err)
	}
	gotLock, err := os.ReadFile(f.lockPath)
	if err != nil {
		t.Fatalf("read revised lock: %v", err)
	}
	lock, err := ParseProjectLock(gotLock)
	if err != nil {
		t.Fatalf("parse revised lock: %v", err)
	}
	if len(lock.Skills) != 1 || lock.Skills[0].Revision != 2 || lock.Skills[0].SHA256 != plan.SHA256 {
		t.Errorf("revised lock entry = %+v, want revision 2 and hash %q", lock.Skills, plan.SHA256)
	}
	for _, target := range []string{f.first, f.second} {
		data, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("read revised target %q: %v", target, err)
		}
		if HashSkill(data) != plan.SHA256 {
			t.Errorf("revised target %q has hash %q, want %q", target, HashSkill(data), plan.SHA256)
		}
	}
}

func TestExecuteProjectRevisePlan_RollbackRestoresBackups(t *testing.T) {
	f := newProjectRevisionFixture(t, "tidy-worktree")
	plan, err := PlanProjectRevise(f.input())
	if err != nil {
		t.Fatalf("unexpected revision refusal: %v", err)
	}
	before := snapshotTree(t, f.root)
	order := planOrder(plan)
	failed := false
	fsys := newFakeProjectFS(func(f *fakeProjectFS, op, path string) error {
		if op == "rename" && path == order[1].Abs && !failed {
			failed = true
			return errInjected
		}
		return nil
	})
	if err := ExecuteProjectRevisePlan(plan, fsys); err == nil {
		t.Fatal("injected mid-revision failure must fail")
	} else if errors.Is(err, ErrRollbackIncomplete) {
		t.Fatalf("revision rollback must succeed: %v", err)
	}
	assertSameTree(t, before, snapshotTree(t, f.root))
}
