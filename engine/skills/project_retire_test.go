package skills

import (
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
	if err := ExecuteProjectPlan(plan, newFakeProjectFS(nil)); err != nil {
		t.Fatalf("execute initial registration: %v", err)
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

	if err := ExecuteProjectRetirePlan(plan, newFakeProjectFS(nil)); err != nil {
		t.Fatalf("execute retirement: %v", err)
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
	// What a person is told afterwards is the commit order of the retirement: the removals, then
	// the lock.
	committed := ProjectRetireCommitOrder(plan)
	if len(committed) != len(plan.Deletes)+1 {
		t.Fatalf("ProjectRetireCommitOrder lists %d writes, want the %d removals and the lock", len(committed), len(plan.Deletes))
	}
	for i, rel := range plan.Deletes {
		if committed[i].Rel != rel {
			t.Errorf("ProjectRetireCommitOrder[%d] = %q, want %q", i, committed[i].Rel, rel)
		}
	}
	if last := committed[len(committed)-1]; last.Rel != ProjectLockRelPath {
		t.Errorf("ProjectRetireCommitOrder ends with %q, want the lock %q", last.Rel, ProjectLockRelPath)
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
	err = ExecuteProjectRetirePlan(plan, fsys)
	if err == nil {
		t.Fatal("injected mid-retirement failure must fail")
	}
	if !errors.Is(err, errInjected) {
		t.Errorf("error %v does not carry injected cause", err)
	}
	if errors.Is(err, ErrRollbackIncomplete) {
		t.Fatalf("retirement rollback must succeed: %v", err)
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
	err = ExecuteProjectRetirePlan(plan, fsys)
	if err == nil {
		t.Fatal("a rollback failure must fail the retirement")
	}
	if !errors.Is(err, ErrRollbackIncomplete) {
		t.Fatalf("error %v must report incomplete rollback", err)
	}
	if !strings.HasPrefix(err.Error(), "project-register: rollback incomplete: "+first.Rel+" (after ") {
		t.Errorf("error = %q, want it worded as project-register always worded it, naming %q", err, first.Rel)
	}
	if got := unrestoredBy(err); len(got) != 1 || got[0] != first.Rel {
		t.Errorf("the error names %q as not restored, want exactly the repo-relative %q", got, first.Rel)
	}
}

// TestPlanProjectRetireRefusesTargetsThatResolveToOneFile: two targets of the skill, or a target and
// the lock, that name one physical file cannot both be removed and restored. The retirement is
// refused, naming both.
func TestPlanProjectRetireRefusesTargetsThatResolveToOneFile(t *testing.T) {
	f := newProjectRetirementFixture(t, "tidy-worktree")
	if _, err := PlanProjectRetire(f.input()); err != nil {
		t.Fatalf("precondition: the input must retire cleanly, got %v", err)
	}
	cases := []struct {
		name     string
		path, as string
		want     []string
	}{
		{"two_targets", f.targets[1], f.targets[0], []string{projectTargets[0].Dir, projectTargets[1].Dir}},
		{"target_and_lock", f.lockPath, f.targets[0], []string{projectTargets[0].Dir, ProjectLockRelPath}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := f.input()
			in.ResolvePath = resolvingAsOneFile(tc.path, tc.as)

			plan, err := PlanProjectRetire(in)
			if err == nil || !strings.HasPrefix(err.Error(), "project-retire: targets ") || !strings.HasSuffix(err.Error(), "resolve to the same file") {
				t.Fatalf("PlanProjectRetire = %+v, %v, want a project-retire refusal that says the targets resolve to the same file", plan, err)
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("refusal %q does not name %q", err, want)
				}
			}
		})
	}
}

// TestPlanProjectRetireKeepsTheModeTheLockHasNow: the retirement rewrites the lock at the mode it
// has on disk, not at the mode a registration writes, so a lock a project keeps private stays so.
func TestPlanProjectRetireKeepsTheModeTheLockHasNow(t *testing.T) {
	f := newProjectRetirementFixture(t, "tidy-worktree")
	if err := os.Chmod(f.lockPath, 0o600); err != nil {
		t.Fatal(err)
	}

	plan, err := PlanProjectRetire(f.input())
	if err != nil {
		t.Fatalf("unexpected retirement refusal: %v", err)
	}
	if plan.Lock.Mode != 0o600 {
		t.Errorf("the planned lock has mode %04o, want the 0600 it has now", plan.Lock.Mode)
	}
}

// TestExecuteProjectRetireRollbackRestoresTheModeTheLockHasAtCommit: the mode the lock had when the
// plan was built is a fact as old as the plan. A person may change it before the retirement runs,
// and a rollback must put the lock back as it was found at the moment of the commit, as a failed
// registration does.
func TestExecuteProjectRetireRollbackRestoresTheModeTheLockHasAtCommit(t *testing.T) {
	f := newProjectRetirementFixture(t, "tidy-worktree")
	plan, err := PlanProjectRetire(f.input())
	if err != nil {
		t.Fatalf("plan retirement: %v", err)
	}
	if plan.Lock.Mode == 0o600 {
		t.Fatal("precondition: the plan must have been built while the lock was not 0600")
	}
	if err := os.Chmod(f.lockPath, 0o600); err != nil {
		t.Fatal(err)
	}

	commits := 0
	fsys := newFakeProjectFS(func(_ *fakeProjectFS, op, path string) error {
		if op == "rename" && path == plan.Lock.Abs {
			if commits++; commits == 1 { // the commit; the rollback's own rename goes through
				return errInjected
			}
		}
		return nil
	})
	err = ExecuteProjectRetirePlan(plan, fsys)
	if !errors.Is(err, errInjected) || errors.Is(err, ErrRollbackIncomplete) {
		t.Fatalf("error = %v, want the injected failure and a complete rollback", err)
	}
	info, statErr := os.Stat(f.lockPath)
	if statErr != nil {
		t.Fatalf("the lock must be back: %v", statErr)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("the restored lock has mode %04o, want the 0600 it had at the commit", got)
	}
}

// TestExecuteProjectRetireLockThatCannotBeInspectedBeforeTheCommitIsLeftAlone: the lock is looked at
// right before its rename, to learn the mode a rollback would restore. If that fails, the rename
// never ran: the retirement returns that error, the removed targets are put back, the old lock is
// not touched (no rename onto it, no rewrite of it) and the staged lock is gone.
func TestExecuteProjectRetireLockThatCannotBeInspectedBeforeTheCommitIsLeftAlone(t *testing.T) {
	f := newProjectRetirementFixture(t, "tidy-worktree")
	plan, err := PlanProjectRetire(f.input())
	if err != nil {
		t.Fatalf("plan retirement: %v", err)
	}
	before := snapshotTree(t, f.root)

	fsys := newFakeProjectFS(func(f *fakeProjectFS, op, path string) error {
		// The targets are removed by now: this is the look at the lock before its commit.
		if op == "stat" && path == plan.Lock.Abs && f.n["remove"] > 0 {
			return errInjected
		}
		return nil
	})
	err = ExecuteProjectRetirePlan(plan, fsys)
	if !errors.Is(err, errInjected) || errors.Is(err, ErrRollbackIncomplete) {
		t.Fatalf("error = %v, want the injected failure and a complete rollback", err)
	}
	if !strings.Contains(err.Error(), "project-retire: inspecting") {
		t.Errorf("error = %q, want it worded as the inspection before the commit", err)
	}
	assertSameTree(t, before, snapshotTree(t, f.root))
	for _, entry := range fsys.log {
		if entry == "rename "+plan.Lock.Abs {
			t.Errorf("the lock was renamed onto (%q) although it could not be inspected first", entry)
		}
	}
	staged := 0
	for _, entry := range fsys.log {
		if strings.HasPrefix(entry, "writetemp "+filepath.Dir(plan.Lock.Abs)) {
			staged++
		}
	}
	if staged != 1 {
		t.Errorf("the lock directory was written %d times, want once (the staging): no restore of the lock was due", staged)
	}
}
