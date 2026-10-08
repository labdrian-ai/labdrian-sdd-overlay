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

// projectRetirementFixture is a real, agent-owned registration used by the
// retirement tests. Building it through the registration planner keeps the
// retirement tests honest about the lock hash, target layout and ownership
// evidence they consume.
type projectRetirementFixture struct {
	root     string
	id       string
	lockPath string
	lockData []byte
	targets  []string
}

func newProjectRetirementFixture(t *testing.T, id string) projectRetirementFixture {
	t.Helper()
	in := registerInput(t, id)
	plan, err := PlanProjectRegister(in)
	if err != nil {
		t.Fatalf("plan initial registration: %v", err)
	}
	var stdout, stderr bytes.Buffer
	if err := ExecuteProjectPlan(plan, newFakeProjectFS(nil), &stdout, &stderr); err != nil {
		t.Fatalf("execute initial registration: %v (stderr %q)", err, stderr.String())
	}
	lockData, err := os.ReadFile(plan.Lock.Abs)
	if err != nil {
		t.Fatalf("read registered lock: %v", err)
	}
	targets := make([]string, 0, len(plan.Writes))
	for _, w := range plan.Writes {
		targets = append(targets, w.Abs)
	}
	return projectRetirementFixture{
		root:     in.ProjectRoot,
		id:       id,
		lockPath: plan.Lock.Abs,
		lockData: lockData,
		targets:  targets,
	}
}

func (f projectRetirementFixture) input() RetireInput {
	return RetireInput{
		ProjectRoot: f.root,
		ID:          f.id,
		LockData:    f.lockData,
		LockExists:  true,
		Registry:    Registry{Version: "1"},
		ReadFile:    os.ReadFile,
		ReadDir:     os.ReadDir,
		Stat:        os.Stat,
		ResolvePath: fsresolve.KeepingMissing,
	}
}

func TestPlanAndExecuteProjectRetireRemovesTargetsAndLockEntry(t *testing.T) {
	f := newProjectRetirementFixture(t, "tidy-worktree")
	plan, err := PlanProjectRetire(f.input())
	if err != nil {
		t.Fatalf("unexpected retirement refusal: %v", err)
	}
	if plan.ID != f.id {
		t.Errorf("plan.ID = %q, want %q", plan.ID, f.id)
	}
	if len(plan.Deletes) != len(f.targets) {
		t.Fatalf("plan.Deletes = %d, want %d", len(plan.Deletes), len(f.targets))
	}
	if len(plan.DeleteWrites) != len(f.targets) {
		t.Fatalf("plan.DeleteWrites = %d, want %d", len(plan.DeleteWrites), len(f.targets))
	}
	if plan.Lock.Backup == nil {
		t.Fatal("retirement must capture the existing lock for rollback")
	}

	var stdout, stderr bytes.Buffer
	if err := ExecuteProjectRetirePlan(plan, newFakeProjectFS(nil), &stdout, &stderr); err != nil {
		t.Fatalf("execute retirement: %v (stderr %q)", err, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("successful retirement printed stderr: %q", stderr.String())
	}
	for _, target := range f.targets {
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Errorf("retirement left target %q; stat error = %v", target, err)
		}
	}
	lockData, err := os.ReadFile(f.lockPath)
	if err != nil {
		t.Fatalf("read lock after retirement: %v", err)
	}
	lock, err := ParseProjectLock(lockData)
	if err != nil {
		t.Fatalf("parse lock after retirement: %v", err)
	}
	if len(lock.Skills) != 0 {
		t.Fatalf("lock after retirement contains %d entries, want none: %+v", len(lock.Skills), lock.Skills)
	}
	for _, rel := range plan.Deletes {
		if !strings.Contains(stdout.String(), "removed: "+rel+"\n") {
			t.Errorf("stdout = %q, want removed line for %q", stdout.String(), rel)
		}
	}
}

func TestPlanProjectRetireRefusesHumanOwnedSkill(t *testing.T) {
	f := newProjectRetirementFixture(t, "tidy-worktree")
	if err := os.WriteFile(f.targets[0], []byte("human-owned edit\n"), 0o644); err != nil {
		t.Fatalf("human edit: %v", err)
	}
	before := snapshotTree(t, f.root)

	plan, err := PlanProjectRetire(f.input())
	if err == nil {
		t.Fatalf("expected human-owned refusal, got plan %+v", plan)
	}
	if !strings.Contains(err.Error(), "human-owned") {
		t.Errorf("refusal %q does not report human ownership", err)
	}
	if !strings.Contains(err.Error(), "hash-mismatch") {
		t.Errorf("refusal %q does not name the ownership reason", err)
	}
	if len(plan.DeleteWrites) != 0 || plan.Lock.Rel != "" {
		t.Errorf("refusal returned a non-zero plan: %+v", plan)
	}
	assertSameTree(t, before, snapshotTree(t, f.root))
}

func TestPlanProjectRetireVerifiesAbsorbedIntoExistence(t *testing.T) {
	tests := []struct {
		name       string
		configure  func(t *testing.T, in *RetireInput)
		wantTarget string
		wantError  string
	}{
		{
			name: "global registry match is existence lookup",
			configure: func(t *testing.T, in *RetireInput) {
				in.AbsorbedInto = "absorber"
				in.Registry = Registry{Version: "1", Skills: []Entry{{
					ID: "global-skill", Path: "skills/absorber",
				}}}
			},
			wantTarget: "absorber",
		},
		{
			name: "project lock entry",
			configure: func(t *testing.T, in *RetireInput) {
				in.AbsorbedInto = "absorber"
				lock, err := ParseProjectLock(in.LockData)
				if err != nil {
					t.Fatalf("parse fixture lock: %v", err)
				}
				lock.Skills = append(lock.Skills, ProjectLockEntry{
					ID: "absorber", Provenance: "procedural",
					Candidate: "procedural/candidates/repeated-success/absorber",
					SHA256:    "not-used-for-existence", Revision: 1,
					Targets: []string{".claude/skills/absorber/SKILL.md"},
				})
				in.LockData, err = SerializeProjectLock(lock)
				if err != nil {
					t.Fatalf("serialize fixture lock: %v", err)
				}
			},
			wantTarget: "absorber",
		},
		{
			name: "unverified target is named",
			configure: func(t *testing.T, in *RetireInput) {
				in.AbsorbedInto = "missing-absorber"
			},
			wantError: `absorbed-into target "missing-absorber" is not verified`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newProjectRetirementFixture(t, "tidy-worktree")
			in := f.input()
			tc.configure(t, &in)
			plan, err := PlanProjectRetire(in)
			if tc.wantError != "" {
				if err == nil {
					t.Fatalf("expected refusal, got plan %+v", plan)
				}
				if !strings.Contains(err.Error(), tc.wantError) {
					t.Errorf("refusal %q does not name %q", err, tc.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected absorbed-into refusal: %v", err)
			}
			if plan.AbsorbedInto != tc.wantTarget {
				t.Errorf("plan.AbsorbedInto = %q, want %q", plan.AbsorbedInto, tc.wantTarget)
			}
		})
	}
}

func TestExecuteProjectRetireRollbackRestoresPreRetirementTree(t *testing.T) {
	f := newProjectRetirementFixture(t, "tidy-worktree")
	plan, err := PlanProjectRetire(f.input())
	if err != nil {
		t.Fatalf("plan retirement: %v", err)
	}
	before := snapshotTree(t, f.root)
	failedDelete := plan.DeleteWrites[1].Abs

	fsys := newFakeProjectFS(func(f *fakeProjectFS, op, path string) error {
		if op == "remove" && path == failedDelete {
			return errInjected
		}
		return nil
	})
	var stdout, stderr bytes.Buffer
	err = ExecuteProjectRetirePlan(plan, fsys, &stdout, &stderr)
	if err == nil {
		t.Fatal("injected mid-retirement failure must fail")
	}
	if !errors.Is(err, errInjected) {
		t.Errorf("error %v does not carry injected cause", err)
	}
	if errors.Is(err, ErrRollbackIncomplete) {
		t.Fatalf("retirement rollback must succeed: %v (stderr %q)", err, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("failed retirement printed removed lines: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("successful retirement rollback printed stderr: %q", stderr.String())
	}
	assertSameTree(t, before, snapshotTree(t, f.root))
}

func TestExecuteProjectRetireRollbackFailureReportsRelativePath(t *testing.T) {
	f := newProjectRetirementFixture(t, "tidy-worktree")
	plan, err := PlanProjectRetire(f.input())
	if err != nil {
		t.Fatalf("plan retirement: %v", err)
	}
	first := plan.DeleteWrites[0]
	failedDelete := plan.DeleteWrites[1].Abs

	fsys := newFakeProjectFS(func(f *fakeProjectFS, op, path string) error {
		if op == "remove" && path == failedDelete {
			return errInjected
		}
		if op == "writetemp" && path == filepath.Dir(first.Abs) {
			return errors.New("rollback write failed")
		}
		return nil
	})
	var stdout, stderr bytes.Buffer
	err = ExecuteProjectRetirePlan(plan, fsys, &stdout, &stderr)
	if err == nil {
		t.Fatal("a rollback failure must fail the retirement")
	}
	if !errors.Is(err, ErrRollbackIncomplete) {
		t.Fatalf("error %v must report incomplete rollback", err)
	}
	want := "error: rollback incomplete: " + first.Rel + "\n"
	if !strings.Contains(stderr.String(), want) {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
	if strings.Contains(stderr.String(), f.root) {
		t.Errorf("stderr = %q contains an absolute path", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("failed retirement printed removed lines: %q", stdout.String())
	}
}
